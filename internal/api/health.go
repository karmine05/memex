package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"memex/internal/config"
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

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	body := healthBody{Status: "ok", Version: config.Version, DB: "ok", Embedder: "ok"}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
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
	code := http.StatusOK
	if body.DB != "ok" {
		body.Status = "down"
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, body)
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
