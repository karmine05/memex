package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"memex/internal/config"
)

func TestEveryRouteHasAHandler(t *testing.T) {
	s := &Server{Cfg: config.Default(), Limit: NewLimiter()}
	for _, rt := range Routes() {
		if s.handler(rt.ID) == nil {
			t.Fatalf("missing handler %s", rt.ID)
		}
	}
}

func TestAdminRoutesAreAbsentFromAgentListener(t *testing.T) {
	s := &Server{Cfg: config.Default(), Limit: NewLimiter()}
	h := s.Handler("agent")
	req := httptest.NewRequest(http.MethodGet, "/admin/agents", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("agent listener status %d", rr.Code)
	}
	home := httptest.NewRequest(http.MethodGet, "/", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, home)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("agent home status %d", rr.Code)
	}
	admin := s.Handler("admin")
	rr = httptest.NewRecorder()
	admin.ServeHTTP(rr, home)
	if rr.Code != http.StatusOK || len(rr.Body.Bytes()) < 100 {
		t.Fatalf("admin home %d", rr.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/admin/telemetry", nil)
	rr = httptest.NewRecorder()
	admin.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("telemetry status %d: %s", rr.Code, rr.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/admin/telemetry", nil)
	rr = httptest.NewRecorder()
	s.Handler("agent").ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("agent telemetry status %d", rr.Code)
	}
}

func TestAdminHomeIsNotACatchAll(t *testing.T) {
	s := &Server{Cfg: config.Default(), Limit: NewLimiter()}
	admin := s.Handler("admin")

	home := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	admin.ServeHTTP(rr, home)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin home %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("admin home content-type %q", ct)
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

	bogus := httptest.NewRequest(http.MethodGet, "/admin/metrics", nil)
	rr = httptest.NewRecorder()
	admin.ServeHTTP(rr, bogus)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unknown admin path %d body %s", rr.Code, rr.Body.String())
	}
}
