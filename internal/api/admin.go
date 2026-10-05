package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"memex/internal/auth"
	"memex/internal/note"
	"memex/internal/store"
)

func (s *Server) adminAgents(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	list, err := s.Store.ListAgents(r.Context(), r.URL.Query().Get("q"), true)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]agentBody, 0, len(list))
	for _, a := range list {
		out = append(out, agentView(a))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) adminCreateAgent(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	var req registerReq
	if err := readJSON(w, r, &req, 32*1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	s.issueAgent(w, r.Context(), req)
}

func (s *Server) adminAgentStats(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	id := r.PathValue("id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	if _, err := s.Store.AgentByID(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		s.fail(w, err)
		return
	}
	st, err := s.Store.AgentStats(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) adminSetStatus(status string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(w, r) {
			return
		}
		id := r.PathValue("id")
		if !validUUID(id) {
			writeError(w, http.StatusBadRequest, "invalid agent id")
			return
		}
		if err := s.Store.SetStatus(r.Context(), id, status); errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		} else if err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"agent_id": id, "status": status})
	}
}

func (s *Server) adminResume(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	id := r.PathValue("id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	if err := s.Store.Resume(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agent_id": id, "status": "active"})
}

// adminRotateKey issues a replacement key for an agent: same identity, new
// secret. The old key and every live token stop working immediately.
func (s *Server) adminRotateKey(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	id := r.PathValue("id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	key, err := auth.NewAPIKey()
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Store.RotateKey(r.Context(), id, auth.Hash(key)); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		s.fail(w, err)
		return
	}
	// The key is already live; a failed audit write must not swallow the
	// only copy of it from the response.
	if err := s.audit("rotate", map[string]any{"agent_id": id}); err != nil {
		slog.Warn("audit rotate", "agent_id", id, "err", err)
	}
	writeJSON(w, http.StatusOK, keyBody{APIKey: key, AgentID: id})
}

func (s *Server) adminQuota(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	id := r.PathValue("id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	var raw json.RawMessage
	if err := readJSON(w, r, &raw, 8*1024); err != nil || !json.Valid(raw) || len(raw) == 0 || raw[0] != '{' {
		writeError(w, http.StatusBadRequest, "quota object required")
		return
	}
	if err := s.Store.SetQuota(r.Context(), id, raw); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agent_id": id, "quota": raw})
}

func (s *Server) adminNote(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	id := r.PathValue("id")
	v, ok := s.adminLoad(w, r, id)
	if !ok {
		return
	}
	n, err := s.Store.ReaderCount(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"note": toNote(v, nil, true), "readers": n})
}

func (s *Server) adminLoad(w http.ResponseWriter, r *http.Request, id string) (store.Version, bool) {
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid note id")
		return store.Version{}, false
	}
	v, err := s.Store.Latest(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return store.Version{}, false
	}
	if err != nil {
		s.fail(w, err)
		return store.Version{}, false
	}
	return v, true
}

func (s *Server) adminVersions(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	id := r.PathValue("id")
	if _, ok := s.adminLoad(w, r, id); !ok {
		return
	}
	s.writeVersions(w, r, id)
}

func (s *Server) adminDiff(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	id := r.PathValue("id")
	if _, ok := s.adminLoad(w, r, id); !ok {
		return
	}
	s.writeDiff(w, r, id)
}

func (s *Server) adminReaders(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	id := r.PathValue("id")
	if _, ok := s.adminLoad(w, r, id); !ok {
		return
	}
	readers, err := s.Store.Readers(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	n, err := s.Store.ReaderCount(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	type row struct {
		AgentID string `json:"agent_id"`
		Version int64  `json:"version"`
		ReadAt  string `json:"read_at"`
	}
	out := make([]row, 0, len(readers))
	for _, rd := range readers {
		out = append(out, row{AgentID: rd.AgentID, Version: rd.Version, ReadAt: rd.ReadAt.UTC().Format(time.RFC3339Nano)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": n, "reads": out})
}

func (s *Server) adminAudit(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	id := r.PathValue("id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid note id")
		return
	}
	versions, err := s.Store.Versions(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(versions) == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	readers, err := s.Store.Readers(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	items := make([]note.ChainItem, len(versions))
	body := make([]noteBody, len(versions))
	for i, v := range versions {
		items[i] = note.ChainItem{Version: v.Version, Body: v.Body, BodyHash: v.BodyHash, PrevHash: v.PrevHash}
		body[i] = toNote(v, nil, true)
	}
	resp := map[string]any{"note_id": id, "versions": body, "ok": true}
	if err := note.Verify(items); err != nil {
		resp["ok"] = false
		resp["error"] = err.Error()
	}
	type rd struct {
		AgentID string `json:"agent_id"`
		Version int64  `json:"version"`
		ReadAt  string `json:"read_at"`
	}
	rs := make([]rd, 0, len(readers))
	for _, x := range readers {
		rs = append(rs, rd{AgentID: x.AgentID, Version: x.Version, ReadAt: x.ReadAt.UTC().Format(time.RFC3339Nano)})
	}
	resp["reads"] = rs
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) adminPurge(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	if r.URL.Query().Get("confirm") != "yes" {
		writeError(w, http.StatusBadRequest, "confirm=yes required")
		return
	}
	id := r.PathValue("id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid note id")
		return
	}
	versions, err := s.Store.Versions(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(versions) == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	chain := make([]map[string]any, len(versions))
	for i, v := range versions {
		chain[i] = map[string]any{"version": v.Version, "body_hash": v.BodyHash, "prev_hash": v.PrevHash, "agent_id": v.AgentID, "body": v.Body}
	}
	if err := s.audit("purge", map[string]any{"note_id": id, "chain": chain}); err != nil {
		s.fail(w, err)
		return
	}
	if _, err := s.Store.PurgeNote(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"purged": id, "versions": len(versions)})
}

func (s *Server) adminReembed(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	var req struct {
		After string `json:"after"`
	}
	if r.ContentLength != 0 {
		if err := readJSON(w, r, &req, 4096); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
	}
	var after *time.Time
	if req.After != "" {
		t, err := time.Parse(time.RFC3339, req.After)
		if err != nil {
			writeError(w, http.StatusBadRequest, "after must be RFC3339")
			return
		}
		after = &t
	}
	n, err := s.Store.ClearEmbeddings(r.Context(), after)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": n})
}

func (s *Server) adminOrphans(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	age := r.URL.Query().Get("older_than")
	if age == "" {
		age = "720h"
	}
	d, err := parseAge(age)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad older_than")
		return
	}
	list, err := s.Store.Orphans(r.Context(), time.Now().Add(-d))
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]noteBody, 0, len(list))
	for _, v := range list {
		out = append(out, toNote(v, nil, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{"notes": out})
}

func (s *Server) adminDupes(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	list, err := s.Store.Dupes(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	if list == nil {
		list = []store.Dupe{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"dupes": list})
}

func (s *Server) adminEval(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	var req struct {
		Holdout []struct {
			Query  string `json:"query"`
			NoteID string `json:"note_id"`
			Space  string `json:"space"`
		} `json:"holdout"`
	}
	if err := readJSON(w, r, &req, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(req.Holdout) == 0 {
		writeError(w, http.StatusBadRequest, "holdout required")
		return
	}
	var sum float64
	hits := 0
	for _, item := range req.Holdout {
		q := store.SearchQuery{
			Text: item.Query, Space: item.Space, AgentID: "", Limit: 5,
			LexicalW: s.Cfg.Search.Lexical, RecencyW: s.Cfg.Search.Recency,
			VectorW: s.Cfg.Search.Vector, HalfLife: s.Cfg.Search.HalfLife.Seconds(),
		}
		found, err := s.Store.Search(r.Context(), q)
		if err != nil {
			s.fail(w, err)
			return
		}
		for i, h := range found {
			if h.Version.NoteID == item.NoteID {
				sum += 1 / float64(i+1)
				hits++
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mrr": sum / float64(len(req.Holdout)), "n": len(req.Holdout), "hits": hits,
	})
}

func (s *Server) adminStats(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	st, err := s.Store.Stats(r.Context(), r.URL.Query().Get("space"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) adminSpace(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	rest := r.PathValue("path")
	switch {
	case strings.HasSuffix(rest, "/lock"):
		space := strings.TrimSuffix(rest, "/lock")
		if !note.ValidSpace(space) {
			writeError(w, http.StatusBadRequest, "invalid space")
			return
		}
		sp, err := s.Store.Space(r.Context(), space)
		raw := []byte(`{}`)
		if err == nil {
			raw = sp.ACL
		} else if !errors.Is(err, store.ErrNotFound) {
			s.fail(w, err)
			return
		}
		locked, err := note.LockACL(raw)
		if err != nil {
			s.fail(w, err)
			return
		}
		if err := s.Store.SetSpaceACL(r.Context(), space, locked); err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"space": space, "locked": true})
	case strings.HasSuffix(rest, "/acl"):
		space := strings.TrimSuffix(rest, "/acl")
		if !note.ValidSpace(space) {
			writeError(w, http.StatusBadRequest, "invalid space")
			return
		}
		var raw json.RawMessage
		if err := readJSON(w, r, &raw, 16*1024); err != nil || len(raw) == 0 || raw[0] != '{' {
			writeError(w, http.StatusBadRequest, "acl object required")
			return
		}
		if err := s.Store.SetSpaceACL(r.Context(), space, raw); err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"space": space, "acl": raw})
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}
