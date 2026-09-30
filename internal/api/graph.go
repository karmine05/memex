package api

import (
	_ "embed"
	"net/http"
)

//go:embed graph.html
var graphHTML []byte

//go:embed skill.md
var skillMD []byte

func (s *Server) adminHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(graphHTML)
}

// skill is keyless, like /admin/telemetry and /admin/graph: it serves the
// agent protocol to the operator's browser on the loopback admin listener so
// the webUI can build a one-prompt install. It is deliberately NOT on the
// agent listener (RULES.md 4.5: no agent-facing docs-by-URL).
func (s *Server) skill(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(skillMD)
}

func (s *Server) adminGraph(w http.ResponseWriter, r *http.Request) {
	g, err := s.Store.Graph(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}
