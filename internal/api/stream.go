package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"memex/internal/feed"
	"memex/internal/note"
	"memex/internal/store"
)

func (s *Server) spaceStream(w http.ResponseWriter, r *http.Request) {
	ag, ok := s.agent(w, r)
	if !ok {
		return
	}
	rest := r.PathValue("path")
	if !strings.HasSuffix(rest, "/stream") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	space := strings.TrimSuffix(rest, "/stream")
	if space == "" || strings.Contains(space, "..") {
		writeError(w, http.StatusBadRequest, "invalid space")
		return
	}
	sp, err := s.Store.Space(r.Context(), space)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, err)
		return
	}
	if err == nil && !note.Can(sp.ACL, ag.ID, false) {
		// A restricted space answers exactly like a nonexistent one: the
		// same 200 SSE shape, no replay, no live subscription. 403-vs-200
		// would enumerate which agent pairs have exchanged DMs (space names
		// are deterministic), and a live subscription would leak per-change
		// metadata the read ACL withholds.
		s.serveQuietStream(w, r)
		return
	}
	s.serveStream(w, r, "space:"+space, ag.ID, func(after int64) ([]store.StreamRow, error) {
		return s.Store.EventsAfter(r.Context(), space, after, 200)
	})
}

func (s *Server) inboxStream(w http.ResponseWriter, r *http.Request) {
	ag, ok := s.agent(w, r)
	if !ok {
		return
	}
	s.serveStream(w, r, "inbox:"+ag.ID, ag.ID, func(after int64) ([]store.StreamRow, error) {
		return s.Store.InboxEvents(r.Context(), ag.ID, after, 200)
	})
}

// serveQuietStream is the uniform empty stream: same 200/SSE shape as a
// live one, but with no hub subscription and no replay, so it cannot
// disclose existence or per-change metadata of spaces the caller may not
// read. It exists so subscribe-before-exist (open spaces) and ACL-denied
// spaces are indistinguishable on the wire.
func (s *Server) serveQuietStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	rc := http.NewResponseController(w)
	fmt.Fprintf(w, ": connected\n\n")
	_ = rc.Flush()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	max := time.NewTimer(streamMaxLifetime)
	defer max.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-max.C:
			return
		case <-ping.C:
			fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
			_ = rc.Flush()
		}
	}
}

func (s *Server) inbox(w http.ResponseWriter, r *http.Request) {
	ag, ok := s.agent(w, r)
	if !ok {
		return
	}
	if id := r.PathValue("id"); id != "" && id != ag.ID && id != "me" {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	sinceRaw := r.URL.Query().Get("since")
	latest := sinceRaw == ""
	var since int64
	if !latest {
		n, err := strconv.ParseInt(sinceRaw, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since must be an event id")
			return
		}
		since = n
	}
	notes, err := s.Store.InboxNotes(r.Context(), ag.ID, since, limit, latest)
	if err != nil {
		s.fail(w, err)
		return
	}
	if ims := r.Header.Get("If-Modified-Since"); ims != "" && len(notes) == 0 {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	out := make([]noteBody, 0, len(notes))
	for _, n := range notes {
		if err := s.Store.RecordRead(r.Context(), n.NoteID, n.Version, ag.ID); err != nil {
			s.fail(w, err)
			return
		}
		out = append(out, toNote(n, nil, true))
	}
	writeJSON(w, http.StatusOK, map[string]any{"notes": out})
}

// streamMaxLifetime bounds one SSE connection. It matches the one-hour
// token TTL: a revoked or rotated agent gets no stream longer than its
// credentials, and well-behaved clients resume via Last-Event-ID.
const streamMaxLifetime = time.Hour

func (s *Server) serveStream(w http.ResponseWriter, r *http.Request, key, agent string, replay func(int64) ([]store.StreamRow, error)) {
	if s.Hub == nil {
		writeError(w, http.StatusServiceUnavailable, "stream unavailable")
		return
	}
	var after int64
	if h := r.Header.Get("Last-Event-ID"); h != "" {
		n, err := strconv.ParseInt(h, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad Last-Event-ID")
			return
		}
		after = n
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	rc := http.NewResponseController(w)
	ch, done, cancel := s.Hub.Subscribe(key, agent)
	defer cancel()
	fmt.Fprintf(w, ": connected\n\n")
	_ = rc.Flush()
	rows, err := replay(after)
	if err != nil {
		slog.Error("stream replay", "err", err)
		return
	}
	seen := map[int64]struct{}{}
	for _, row := range rows {
		seen[row.ID] = struct{}{}
		writeSSE(w, row)
	}
	_ = rc.Flush()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	max := time.NewTimer(streamMaxLifetime)
	defer max.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-done:
			return
		case <-max.C:
			return
		case ev := <-ch:
			if ev.ID <= after {
				continue
			}
			if _, ok := seen[ev.ID]; ok {
				delete(seen, ev.ID)
				continue
			}
			writeFeed(w, ev)
			_ = rc.Flush()
		case <-ping.C:
			fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
			_ = rc.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, row store.StreamRow) {
	ev := feed.Event{
		ID: row.ID, NoteID: row.NoteID, Version: row.Version, BodyHash: row.BodyHash,
		AgentID: row.AgentID, Space: row.SpaceID, CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	writeFeed(w, ev)
}

func writeFeed(w http.ResponseWriter, ev feed.Event) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "id: %d\nevent: note\ndata: %s\n\n", ev.ID, b)
}
