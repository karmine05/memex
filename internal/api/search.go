package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"memex/internal/search"
	"memex/internal/store"
)

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	ag, ok := s.agent(w, r)
	if !ok {
		return
	}
	var req struct {
		Query  string `json:"query"`
		Space  string `json:"space"`
		Limit  int    `json:"limit"`
		Since  string `json:"since"`
		Bodies *bool  `json:"bodies"`
	}
	if err := readJSON(w, r, &req, 16*1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "query required")
		return
	}
	if req.Limit <= 0 {
		req.Limit = 5
	}
	if req.Limit > 50 {
		req.Limit = 50
	}
	var since *time.Time
	if req.Since != "" {
		t, err := time.Parse(time.RFC3339, req.Since)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since must be RFC3339")
			return
		}
		since = &t
	}
	withBody := true
	if req.Bodies != nil {
		withBody = *req.Bodies
	}
	q := store.SearchQuery{
		Text: req.Query, Space: req.Space, AgentID: ag.ID, Since: since, Limit: req.Limit,
		VectorW: s.Cfg.Search.Vector, LexicalW: s.Cfg.Search.Lexical, RecencyW: s.Cfg.Search.Recency,
		HalfLife: s.Cfg.Search.HalfLife.Seconds(),
	}
	if s.Embed != nil {
		ectx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		vecs, err := s.Embed.Embed(ectx, []string{req.Query})
		cancel()
		if err != nil {
			slog.Warn("search embed degraded", "err", err)
		} else if lit, err := search.VectorLiteral(vecs[0]); err == nil {
			q.Vector = lit
		}
	}
	hits, err := s.Store.Search(r.Context(), q)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]noteBody, 0, len(hits))
	for _, h := range hits {
		score := h.Score
		if err := s.Store.RecordRead(r.Context(), h.Version.NoteID, h.Version.Version, ag.ID); err != nil {
			s.fail(w, err)
			return
		}
		out = append(out, toNote(h.Version, &score, withBody))
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out})
}
