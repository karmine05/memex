package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"memex/internal/auth"
	"memex/internal/config"
)

func TestAdminKeyRequiredOnAdminAPI(t *testing.T) {
	s := &Server{
		Cfg: config.Default(), AdminHash: auth.Hash("mxa_unit_test"),
		Limit: NewLimiter(),
	}
	admin := s.Handler("admin")

	get := func(key string) int {
		req := httptest.NewRequest(http.MethodGet, "/admin/doctor", nil)
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		rr := httptest.NewRecorder()
		admin.ServeHTTP(rr, req)
		return rr.Code
	}
	if code := get(""); code != http.StatusUnauthorized {
		t.Fatalf("missing key: status %d, want 401", code)
	}
	if code := get("mxa_wrong"); code != http.StatusUnauthorized {
		t.Fatalf("wrong key: status %d, want 401", code)
	}
	if code := get("mxa_unit_test"); code != http.StatusOK {
		t.Fatalf("valid key: status %d, want 200", code)
	}
}

func TestEveryRouteHasAHandler(t *testing.T) {
	s := &Server{Cfg: config.Default(), Limit: NewLimiter()}
	for _, rt := range Routes() {
		if s.handler(rt.ID) == nil {
			t.Fatalf("missing handler %s", rt.ID)
		}
	}
}

func TestGraphHTMLHasZenMode(t *testing.T) {
	for _, part := range []string{
		"body.zen .panel", // css: hide chrome
		`id="zen-btn"`,    // entry button
		"function setZen", // toggle + fullscreen wiring
	} {
		if !bytes.Contains(graphHTML, []byte(part)) {
			t.Fatalf("graph.html missing zen mode piece: %q", part)
		}
	}
}

func TestAdminRoutesAreAbsentFromAgentListener(t *testing.T) {
	s := &Server{Cfg: config.Default(), Limit: NewLimiter(), WebsiteHandler: WebsiteHandler()}
	h := s.Handler("agent")
	req := httptest.NewRequest(http.MethodGet, "/admin/agents", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("agent listener /admin/agents status %d", rr.Code)
	}
	// Agent listener does NOT serve website (website is admin-only)
	home := httptest.NewRequest(http.MethodGet, "/", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, home)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("agent home should be 404, got %d", rr.Code)
	}

	admin := s.Handler("admin")
	// Admin listener serves React app at /
	rr = httptest.NewRecorder()
	admin.ServeHTTP(rr, home)
	if rr.Code != http.StatusOK || len(rr.Body.Bytes()) < 100 {
		t.Fatalf("admin home %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("admin home content-type %q", ct)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("MEMEX — Shared Memory for Agents")) {
		t.Fatalf("admin home missing React app content")
	}

	// /dash serves the same SPA shell; the client asks for the mxa_ key.
	dash := httptest.NewRequest(http.MethodGet, "/dash", nil)
	rr = httptest.NewRecorder()
	admin.ServeHTTP(rr, dash)
	if rr.Code != http.StatusOK || !bytes.Contains(rr.Body.Bytes(), []byte("MEMEX — Shared Memory for Agents")) {
		t.Fatalf("admin dash %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	s.Handler("agent").ServeHTTP(rr, dash)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("agent dash status %d", rr.Code)
	}

	// Admin API endpoints are on the admin listener; gated routes answer 401
	// without the mxa_ key instead of 404.
	req = httptest.NewRequest(http.MethodGet, "/admin/telemetry", nil)
	rr = httptest.NewRecorder()
	admin.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("telemetry status %d: %s", rr.Code, rr.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/admin/telemetry", nil)
	rr = httptest.NewRecorder()
	s.Handler("agent").ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("agent telemetry status %d", rr.Code)
	}
}

func TestSkillMDIsKeylessOnAdminOnly(t *testing.T) {
	s := &Server{Cfg: config.Default(), Limit: NewLimiter()}
	// admin listener: keyless, serves the protocol as markdown
	admin := s.Handler("admin")
	req := httptest.NewRequest(http.MethodGet, "/skill.md", nil)
	rr := httptest.NewRecorder()
	admin.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin skill.md %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/markdown; charset=utf-8" {
		t.Fatalf("skill.md content-type %q", ct)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("Agent Protocol")) {
		t.Fatalf("skill.md body missing protocol: %q", rr.Body.String()[:100])
	}
	// agent listener: no instructions-by-URL (RULES.md 4.5)
	agent := s.Handler("agent")
	rr = httptest.NewRecorder()
	agent.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("agent skill.md status %d", rr.Code)
	}
}

func TestAdminHomeServesGraph(t *testing.T) {
	s := &Server{Cfg: config.Default(), Limit: NewLimiter(), WebsiteHandler: WebsiteHandler()}
	admin := s.Handler("admin")

	// Admin home at / serves the React app
	home := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	admin.ServeHTTP(rr, home)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin home %d: %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("admin home content-type %q", ct)
	}
	// Should contain React app content
	if !bytes.Contains(rr.Body.Bytes(), []byte("MEMEX — Shared Memory for Agents")) {
		t.Fatalf("admin home missing React app content: %s", rr.Body.String()[:200])
	}

	met := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr = httptest.NewRecorder()
	admin.ServeHTTP(rr, met)
	if rr.Code != http.StatusOK {
		t.Fatalf("metrics %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/plain; version=0.0.4" {
		t.Fatalf("metrics content-type %q body %q", ct, rr.Body.String())
	}
}