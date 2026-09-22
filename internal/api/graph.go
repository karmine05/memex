package api

import (
	_ "embed"
	"net/http"
)

//go:embed graph.html
var graphHTML []byte

func (s *Server) adminHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(graphHTML)
}

func (s *Server) adminGraph(w http.ResponseWriter, r *http.Request) {
	g, err := s.Store.Graph(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}
