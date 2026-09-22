package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"memex/internal/auth"
	"memex/internal/note"
	"memex/internal/store"
)

type agentBody struct {
	AgentID     string          `json:"agent_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Card        json.RawMessage `json:"card"`
	Status      string          `json:"status"`
	LastActive  *string         `json:"last_active"`
}

type keyBody struct {
	APIKey  string `json:"api_key"`
	AgentID string `json:"agent_id"`
}

type tokenBody struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"`
}

func agentView(a store.Agent) agentBody {
	card := json.RawMessage(a.Card)
	if len(card) == 0 {
		card = json.RawMessage(`{}`)
	}
	var last *string
	if a.LastActive != nil {
		s := a.LastActive.UTC().Format(time.RFC3339Nano)
		last = &s
	}
	return agentBody{
		AgentID: a.ID, Name: a.Name, Description: a.Description,
		Card: card, Status: a.Status, LastActive: last,
	}
}

type registerReq struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Card        json.RawMessage `json:"card"`
	Invite      string          `json:"invite"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if !s.Limit.Allow("ip:"+clientIP(r), 30) {
		retryAfter(w, 1)
		writeError(w, http.StatusTooManyRequests, "rate limit")
		return
	}
	var req registerReq
	if err := readJSON(w, r, &req, 32*1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !s.registrationOK(w, r, req.Invite) {
		return
	}
	s.issueAgent(w, r.Context(), req)
}

func (s *Server) registrationOK(w http.ResponseWriter, r *http.Request, invite string) bool {
	switch s.Cfg.Registration {
	case "open":
		return true
	case "invite":
		if s.Cfg.ValidInvite(invite) {
			return true
		}
		writeError(w, http.StatusForbidden, "invite required")
		return false
	default:
		if s.isAdmin(r) {
			return true
		}
		writeError(w, http.StatusForbidden, "bootstrap key required")
		return false
	}
}

func (s *Server) issueAgent(w http.ResponseWriter, ctx context.Context, req registerReq) {
	if !note.ValidName(req.Name) {
		writeError(w, http.StatusBadRequest, "invalid name")
		return
	}
	if len(req.Card) > 0 && !json.Valid(req.Card) {
		writeError(w, http.StatusBadRequest, "invalid card")
		return
	}
	id, err := note.NewID(time.Now())
	if err != nil {
		s.fail(w, err)
		return
	}
	key, err := auth.NewAPIKey()
	if err != nil {
		s.fail(w, err)
		return
	}
	err = s.Store.CreateAgent(ctx, store.Agent{
		ID: id, Name: req.Name, Description: req.Description, Card: req.Card, KeyHash: auth.Hash(key),
	})
	if errors.Is(err, store.ErrExists) {
		writeError(w, http.StatusConflict, "name taken")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, keyBody{APIKey: key, AgentID: id})
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	if !s.Limit.Allow("ip:"+clientIP(r), 60) {
		retryAfter(w, 1)
		writeError(w, http.StatusTooManyRequests, "rate limit")
		return
	}
	key := bearer(r)
	if !stringsHasPrefix(key, auth.PrefixKey) {
		writeError(w, http.StatusUnauthorized, "api key required")
		return
	}
	ag, err := s.Store.AgentByKeyHash(r.Context(), auth.Hash(key))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "api key invalid")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	if ag.Status != "active" {
		writeError(w, http.StatusForbidden, "agent is not active")
		return
	}
	tok, err := auth.NewToken()
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Store.InsertToken(r.Context(), auth.Hash(tok), ag.ID); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tokenBody{Token: tok, ExpiresIn: 3600})
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.agent(w, r); !ok {
		return
	}
	list, err := s.Store.ListAgents(r.Context(), r.URL.Query().Get("q"), false)
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

func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.agent(w, r); !ok {
		return
	}
	id := r.PathValue("id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	ag, err := s.Store.AgentByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || ag.Status == "revoked" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, agentView(ag))
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
