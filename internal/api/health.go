package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"memex/internal/config"
	"memex/internal/store"
)

type healthBody struct {
	Status        string `json:"status"`
	Version       string `json:"version"`
	DB            string `json:"db"`
	Embedder      string `json:"embedder"`
	Streams       int    `json:"streams"`
	NoteVersions  int64  `json:"note_versions"`
	PendingEmbeds int64  `json:"pending_embeds"`
}

func (s *Server) healthInfo(ctx context.Context) healthBody {
	body := healthBody{Status: "ok", Version: config.Version, DB: "ok", Embedder: "ok"}
	if s.Store == nil {
		body.DB = "down"
	} else if err := s.Store.Pool.Ping(ctx); err != nil {
		body.DB = "down"
	} else if n, p, err := s.Store.Counts(ctx); err == nil {
		body.NoteVersions = n
		body.PendingEmbeds = p
	}
	if s.Embed == nil {
		body.Embedder = "degraded"
	} else if err := s.Embed.Healthy(ctx); err != nil {
		body.Embedder = "degraded"
	}
	if s.Hub != nil {
		body.Streams = s.Hub.Subscribers()
	}
	if body.DB != "ok" {
		body.Status = "down"
	}
	return body
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	body := s.healthInfo(ctx)
	code := http.StatusOK
	if body.DB != "ok" {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, body)
}

// telemetry is keyless: read-only aggregates for the admin dashboard, served
// only on the loopback-bound admin listener (same trust boundary as
// /admin/graph, /healthz, and /metrics).
func (s *Server) telemetry(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil {
		writeError(w, http.StatusServiceUnavailable, "db down")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	st, err := s.Store.Stats(ctx, "")
	if err != nil {
		s.fail(w, err)
		return
	}
	agents, err := s.Store.ListAgents(ctx, "", false)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]agentBody, 0, len(agents))
	for _, a := range agents {
		out = append(out, agentView(a))
	}
	act, err := s.Store.RecentActivity(ctx, 40)
	if err != nil {
		s.fail(w, err)
		return
	}
	if act == nil {
		act = []store.Activity{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"health":   s.healthInfo(ctx),
		"stats":    st,
		"agents":   out,
		"activity": act,
	})
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	for code := 100; code < 600; code++ {
		n := s.status[code].Load()
		if n == 0 {
			continue
		}
		b.WriteString("memex_http_responses_total{code=\"")
		b.WriteString(strconv.Itoa(code))
		b.WriteString("\"} ")
		b.WriteString(strconv.FormatInt(n, 10))
		b.WriteByte('\n')
	}
	if s.Hub != nil {
		b.WriteString("memex_streams ")
		b.WriteString(strconv.Itoa(s.Hub.Subscribers()))
		b.WriteByte('\n')
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte(b.String()))
}

func (s *Server) doctor(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":         s.Cfg.Mode,
		"listen":       s.Cfg.Listen,
		"admin_listen": s.Cfg.AdminListen,
		"db":           config.RedactDSN(s.Cfg.DB),
		"registration": s.Cfg.Registration,
		"embed": map[string]any{
			"provider": s.Cfg.Embed.Provider,
			"model":    s.Cfg.Embed.Model,
			"url":      s.Cfg.Embed.URL,
			"dim":      s.Cfg.Embed.Dim,
		},
		"search": map[string]any{
			"vector":   s.Cfg.Search.Vector,
			"lexical":  s.Cfg.Search.Lexical,
			"recency":  s.Cfg.Search.Recency,
			"halflife": s.Cfg.Search.HalfLife.String(),
		},
		"limits": s.Cfg.Limits,
	})
}
