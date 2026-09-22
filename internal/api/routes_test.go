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
}
