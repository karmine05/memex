package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"memex/internal/auth"
	"memex/internal/config"
	"memex/internal/feed"
	"memex/internal/note"
	"memex/internal/search"
	"memex/internal/store"
)

type Route struct {
	ID       string
	Method   string
	Mux      string
	Path     string
	Audience string
	Summary  string
}

type Server struct {
	Cfg            config.Config
	Store          *store.Store
	Hub            *feed.Hub
	Embed          search.Embedder
	AdminHash      string
	Limit          *Limiter
	AuditPath      string
	WebsiteHandler http.Handler

	status  [600]atomic.Int64
	auditMu sync.Mutex

	testAgent *store.Agent // tests only: skip store token lookup
}

func Routes() []Route {
	return []Route{
		{ID: "health", Method: "GET", Mux: "/healthz", Path: "/healthz", Audience: "agent", Summary: "Process health"},
		{ID: "register", Method: "POST", Mux: "/v1/agents/register", Path: "/v1/agents/register", Audience: "agent", Summary: "Register an agent and receive an API key"},
		{ID: "token", Method: "POST", Mux: "/v1/auth/token", Path: "/v1/auth/token", Audience: "agent", Summary: "Exchange an API key for a one-hour token"},
		{ID: "list_agents", Method: "GET", Mux: "/v1/agents", Path: "/v1/agents", Audience: "agent", Summary: "Directory search"},
		{ID: "get_agent", Method: "GET", Mux: "/v1/agents/{id}", Path: "/v1/agents/{id}", Audience: "agent", Summary: "Public agent profile"},
		{ID: "create_note", Method: "POST", Mux: "/v1/notes", Path: "/v1/notes", Audience: "agent", Summary: "Create a note"},
		{ID: "get_note", Method: "GET", Mux: "/v1/notes/{id}", Path: "/v1/notes/{id}", Audience: "agent", Summary: "Read the current note version"},
		{ID: "put_note", Method: "PUT", Mux: "/v1/notes/{id}", Path: "/v1/notes/{id}", Audience: "agent", Summary: "Append a note version"},
		{ID: "note_versions", Method: "GET", Mux: "/v1/notes/{id}/versions", Path: "/v1/notes/{id}/versions", Audience: "agent", Summary: "List note versions"},
		{ID: "note_diff", Method: "GET", Mux: "/v1/notes/{id}/diff", Path: "/v1/notes/{id}/diff", Audience: "agent", Summary: "Unified diff of two versions"},
		{ID: "search", Method: "POST", Mux: "/v1/search", Path: "/v1/search", Audience: "agent", Summary: "Hybrid search"},
		{ID: "space_stream", Method: "GET", Mux: "/v1/spaces/{path...}", Path: "/v1/spaces/{space}/stream", Audience: "agent", Summary: "SSE change feed for a space"},
		{ID: "inbox", Method: "GET", Mux: "/v1/agents/me/inbox", Path: "/v1/agents/me/inbox", Audience: "agent", Summary: "DM history"},
		{ID: "inbox_stream", Method: "GET", Mux: "/v1/agents/me/inbox/stream", Path: "/v1/agents/me/inbox/stream", Audience: "agent", Summary: "SSE inbox"},
		{ID: "agent_inbox", Method: "GET", Mux: "/v1/agents/{id}/inbox", Path: "/v1/agents/{id}/inbox", Audience: "agent", Summary: "DM history for the calling agent"},
		{ID: "dm", Method: "POST", Mux: "/v1/agents/{id}/dm", Path: "/v1/agents/{id}/dm", Audience: "agent", Summary: "Send a direct note"},

		{ID: "admin_graph", Method: "GET", Mux: "/admin/graph", Path: "/admin/graph", Audience: "admin", Summary: "Correlation graph data"},
		{ID: "skill", Method: "GET", Mux: "/skill.md", Path: "/skill.md", Audience: "admin", Summary: "Agent protocol (SKILL.md) for the one-prompt install"},
		{ID: "telemetry", Method: "GET", Mux: "/admin/telemetry", Path: "/admin/telemetry", Audience: "admin", Summary: "Read-only dashboard aggregates (admin key)"},
		{ID: "admin_health", Method: "GET", Mux: "/healthz", Path: "/healthz", Audience: "admin", Summary: "Admin listener health"},
		{ID: "metrics", Method: "GET", Mux: "/metrics", Path: "/metrics", Audience: "admin", Summary: "Prometheus text metrics"},
		{ID: "doctor", Method: "GET", Mux: "/admin/doctor", Path: "/admin/doctor", Audience: "admin", Summary: "Redacted effective config"},
		{ID: "admin_agents", Method: "GET", Mux: "/admin/agents", Path: "/admin/agents", Audience: "admin", Summary: "List agents"},
		{ID: "admin_create_agent", Method: "POST", Mux: "/admin/agents", Path: "/admin/agents", Audience: "admin", Summary: "Issue an agent API key"},
		{ID: "admin_stats_agent", Method: "GET", Mux: "/admin/agents/{id}/stats", Path: "/admin/agents/{id}/stats", Audience: "admin", Summary: "Per-agent write and read counts"},
		{ID: "admin_revoke", Method: "POST", Mux: "/admin/agents/{id}/revoke", Path: "/admin/agents/{id}/revoke", Audience: "admin", Summary: "Revoke an agent"},
		{ID: "admin_rotate", Method: "POST", Mux: "/admin/agents/{id}/rotate", Path: "/admin/agents/{id}/rotate", Audience: "admin", Summary: "Issue a replacement API key; old key and live tokens die"},
		{ID: "admin_suspend", Method: "POST", Mux: "/admin/agents/{id}/suspend", Path: "/admin/agents/{id}/suspend", Audience: "admin", Summary: "Suspend an agent"},
		{ID: "admin_resume", Method: "POST", Mux: "/admin/agents/{id}/resume", Path: "/admin/agents/{id}/resume", Audience: "admin", Summary: "Resume a suspended agent"},
		{ID: "admin_quota", Method: "PUT", Mux: "/admin/agents/{id}/quota", Path: "/admin/agents/{id}/quota", Audience: "admin", Summary: "Set per-agent quota"},
		{ID: "admin_note", Method: "GET", Mux: "/admin/notes/{id}", Path: "/admin/notes/{id}", Audience: "admin", Summary: "Current note plus reader count"},
		{ID: "admin_versions", Method: "GET", Mux: "/admin/notes/{id}/versions", Path: "/admin/notes/{id}/versions", Audience: "admin", Summary: "Version list"},
		{ID: "admin_diff", Method: "GET", Mux: "/admin/notes/{id}/diff", Path: "/admin/notes/{id}/diff", Audience: "admin", Summary: "Diff two versions"},
		{ID: "admin_readers", Method: "GET", Mux: "/admin/notes/{id}/readers", Path: "/admin/notes/{id}/readers", Audience: "admin", Summary: "Who read the note"},
		{ID: "admin_audit", Method: "GET", Mux: "/admin/notes/{id}/audit", Path: "/admin/notes/{id}/audit", Audience: "admin", Summary: "Full hash chain and readers"},
		{ID: "admin_purge", Method: "DELETE", Mux: "/admin/notes/{id}", Path: "/admin/notes/{id}", Audience: "admin", Summary: "Purge a note after audit"},
		{ID: "admin_reembed", Method: "POST", Mux: "/admin/re-embed", Path: "/admin/re-embed", Audience: "admin", Summary: "Clear embeddings for backfill"},
		{ID: "admin_orphans", Method: "GET", Mux: "/admin/orphans", Path: "/admin/orphans", Audience: "admin", Summary: "Notes with no reads and no refs"},
		{ID: "admin_dupes", Method: "GET", Mux: "/admin/dupes", Path: "/admin/dupes", Audience: "admin", Summary: "Near-duplicate current notes"},
		{ID: "admin_eval", Method: "POST", Mux: "/admin/eval", Path: "/admin/eval", Audience: "admin", Summary: "MRR against a holdout set"},
		{ID: "admin_stats", Method: "GET", Mux: "/admin/stats", Path: "/admin/stats", Audience: "admin", Summary: "Volume for a space or the instance"},
		{ID: "admin_space", Method: "POST", Mux: "/admin/spaces/{path...}", Path: "/admin/spaces/{space}", Audience: "admin", Summary: "Lock a space or replace its ACL"},

		// Website routes (served on admin listener at /)
		{ID: "website_root", Method: "GET", Mux: "/{$}", Path: "/", Audience: "admin", Summary: "MEMEX landing page"},
		{ID: "website_dash", Method: "GET", Mux: "/dash", Path: "/dash", Audience: "admin", Summary: "MEMEX dashboard (admin UI, mxa_ key)"},
		{ID: "website_assets", Method: "GET", Mux: "/assets/{path...}", Path: "/assets/", Audience: "admin", Summary: "MEMEX website assets"},
		{ID: "website_spa", Method: "GET", Mux: "/{path...}", Path: "/{path}", Audience: "admin", Summary: "MEMEX SPA fallback"},

		}
}

func (s *Server) Handler(audience string) http.Handler {
	mux := http.NewServeMux()
	for _, rt := range Routes() {
		if rt.Audience != audience {
			continue
		}
		h := s.handler(rt.ID)
		if h == nil {
			continue
		}
		mux.HandleFunc(rt.Method+" "+rt.Mux, h)
	}
	return s.wrap(mux)
}

func (s *Server) handler(id string) http.HandlerFunc {
	switch id {
	case "health", "admin_health":
		return s.health
	case "register":
		return s.register
	case "token":
		return s.token
	case "list_agents":
		return s.listAgents
	case "get_agent":
		return s.getAgent
	case "create_note":
		return s.createNote
	case "get_note":
		return s.getNote
	case "put_note":
		return s.putNote
	case "note_versions":
		return s.noteVersions
	case "note_diff":
		return s.noteDiff
	case "search":
		return s.search
	case "space_stream":
		return s.spaceStream
	case "inbox", "agent_inbox":
		return s.inbox
	case "inbox_stream":
		return s.inboxStream
	case "dm":
		return s.dm
	case "admin_graph":
		return s.adminGraph
	case "skill":
		return s.skill
	case "telemetry":
		return s.telemetry
	case "metrics":
		return s.metrics
	case "doctor":
		return s.doctor
	case "admin_agents":
		return s.adminAgents
	case "admin_create_agent":
		return s.adminCreateAgent
	case "admin_stats_agent":
		return s.adminAgentStats
	case "admin_revoke":
		return s.adminSetStatus("revoked")
	case "admin_rotate":
		return s.adminRotateKey
	case "admin_suspend":
		return s.adminSetStatus("suspended")
	case "admin_resume":
		return s.adminResume
	case "admin_quota":
		return s.adminQuota
	case "admin_note":
		return s.adminNote
	case "admin_versions":
		return s.adminVersions
	case "admin_diff":
		return s.adminDiff
	case "admin_readers":
		return s.adminReaders
	case "admin_audit":
		return s.adminAudit
	case "admin_purge":
		return s.adminPurge
	case "admin_reembed":
		return s.adminReembed
	case "admin_orphans":
		return s.adminOrphans
	case "admin_dupes":
		return s.adminDupes
	case "admin_eval":
		return s.adminEval
	case "admin_stats":
		return s.adminStats
	case "admin_space":
		return s.adminSpace
	case "website_root", "website_dash", "website_assets", "website_spa":
		if s.WebsiteHandler != nil {
			return s.WebsiteHandler.ServeHTTP
		}
		return http.NotFound
	default:
		return nil
	}
}

func (s *Server) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(sw, r)
		path := r.URL.Path
		dash := path == "/" || path == "/healthz" || path == "/metrics" || path == "/admin/telemetry"
		if !dash && sw.code >= 100 && sw.code < 600 {
			s.status[sw.code].Add(1)
		}
		if !dash {
			slog.Info("http", "method", r.Method, "path", path, "status", sw.code, "ms", time.Since(start).Milliseconds())
		}
	})
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	slog.Error("request", "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func (s *Server) touch(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Store.Touch(ctx, id); err != nil {
		slog.Error("touch agent", "err", err, "agent_id", id)
	}
}

func (s *Server) agent(w http.ResponseWriter, r *http.Request) (store.Agent, bool) {
	if s.testAgent != nil {
		return *s.testAgent, true
	}
	raw := bearer(r)
	if !strings.HasPrefix(raw, auth.PrefixToken) {
		writeError(w, http.StatusUnauthorized, "token required")
		return store.Agent{}, false
	}
	ag, err := s.Store.AgentByToken(r.Context(), auth.Hash(raw))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "token invalid")
		return store.Agent{}, false
	}
	if err != nil {
		s.fail(w, err)
		return store.Agent{}, false
	}
	if ag.Status != "active" {
		writeError(w, http.StatusForbidden, "agent is not active")
		return store.Agent{}, false
	}
	if !s.Limit.Allow("agent:"+ag.ID, s.Cfg.Limits.RequestsPerMinute) {
		retryAfter(w, 1)
		writeError(w, http.StatusTooManyRequests, "rate limit")
		return store.Agent{}, false
	}
	go s.touch(ag.ID)
	return ag, true
}

func (s *Server) isAdmin(r *http.Request) bool {
	raw := bearer(r)
	if !strings.HasPrefix(raw, auth.PrefixAdmin) || s.AdminHash == "" {
		return false
	}
	return auth.EqualHash(auth.Hash(raw), s.AdminHash)
}

func (s *Server) admin(w http.ResponseWriter, r *http.Request) bool {
	if !s.isAdmin(r) {
		writeError(w, http.StatusUnauthorized, "admin key required")
		return false
	}
	return true
}

func (s *Server) limitsFor(quota []byte) store.Limits {
	lim := store.Limits{
		NotesPerHour: s.Cfg.Limits.NotesPerHour,
		BytesPerHour: s.Cfg.Limits.BytesPerHour,
		HistoryMax:   s.Cfg.Limits.HistoryMaxBytes,
	}
	var q struct {
		Notes *int   `json:"notes_per_hour"`
		Bytes *int64 `json:"bytes_per_hour"`
	}
	if json.Unmarshal(quota, &q) == nil {
		if q.Notes != nil {
			lim.NotesPerHour = *q.Notes
		}
		if q.Bytes != nil {
			lim.BytesPerHour = *q.Bytes
		}
	}
	return lim
}

func allowWrite(agentID string) store.Allow {
	return func(acl []byte) error {
		if !note.Can(acl, agentID, true) {
			return store.ErrForbidden
		}
		return nil
	}
}

func (s *Server) writeResult(w http.ResponseWriter, code int, res store.WriteResult, err error) {
	if err == nil {
		writeJSON(w, code, toNote(res.Version, nil, true))
		return
	}
	switch {
	case errors.Is(err, store.ErrConflict):
		writeJSON(w, http.StatusConflict, conflictBody{
			Error: "base_hash mismatch", BodyHash: res.ConflictHash, PrevHash: res.PrevHash,
		})
	case errors.Is(err, store.ErrQuota):
		retryAfter(w, res.RetryAfter)
		writeError(w, http.StatusTooManyRequests, "quota exceeded")
	case errors.Is(err, store.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, store.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "note history exceeds cap")
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrExists):
		writeError(w, http.StatusConflict, "already exists")
	default:
		s.fail(w, err)
	}
}

func (s *Server) audit(event string, fields map[string]any) error {
	if s.AuditPath == "" {
		return errors.New("audit log is not configured")
	}
	fields["event"] = event
	fields["at"] = time.Now().UTC().Format(time.RFC3339Nano)
	b, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	f, err := os.OpenFile(s.AuditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

func clientIP(r *http.Request) string {
	host, _, ok := strings.Cut(r.RemoteAddr, ":")
	if !ok {
		return r.RemoteAddr
	}
	return host
}

func parseAge(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
