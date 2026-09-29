package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"memex/internal/note"
	"memex/internal/store"
)

func (s *Server) createNote(w http.ResponseWriter, r *http.Request) {
	ag, ok := s.agent(w, r)
	if !ok {
		return
	}
	var req struct {
		Space string          `json:"space"`
		Body  json.RawMessage `json:"body"`
	}
	if err := readJSON(w, r, &req, int64(s.Cfg.Limits.NoteMaxBytes)+4096); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Space == "" {
		writeError(w, http.StatusBadRequest, `space required: top-level field alongside body, e.g. {"space":"ops/fixes","body":{...}}`)
		return
	}
	if !note.ValidSpace(req.Space) || note.IsDM(req.Space) {
		writeError(w, http.StatusBadRequest, "invalid space: 1-200 chars, no leading dot/underscore/-, no '..', no leading/trailing slash, not dm/<a>/<b>")
		return
	}
	body, hash, err := note.Normalize(req.Body)
	if err != nil {
		writeNoteErr(w, err)
		return
	}
	if len(body) > s.Cfg.Limits.NoteMaxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "body exceeds cap")
		return
	}
	id, err := note.NewID(time.Now())
	if err != nil {
		s.fail(w, err)
		return
	}
	res, err := s.Store.CreateNote(r.Context(), store.Version{
		NoteID: id, AgentID: ag.ID, SpaceID: req.Space, Body: body, BodyHash: hash,
	}, []byte(`{}`), s.limitsFor(ag.Quota), allowWrite(ag.ID))
	s.writeResult(w, http.StatusCreated, res, err)
}

func (s *Server) putNote(w http.ResponseWriter, r *http.Request) {
	ag, ok := s.agent(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid note id")
		return
	}
	var req struct {
		Body     json.RawMessage `json:"body"`
		BaseHash string          `json:"base_hash"`
	}
	if err := readJSON(w, r, &req, int64(s.Cfg.Limits.NoteMaxBytes)+4096); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.BaseHash == "" {
		writeError(w, http.StatusBadRequest, "base_hash required")
		return
	}
	body, hash, err := note.Normalize(req.Body)
	if err != nil {
		writeNoteErr(w, err)
		return
	}
	if len(body) > s.Cfg.Limits.NoteMaxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "body exceeds cap")
		return
	}
	res, err := s.Store.UpdateNote(r.Context(), id, ag.ID, body, hash, req.BaseHash, s.limitsFor(ag.Quota), allowWrite(ag.ID))
	s.writeResult(w, http.StatusOK, res, err)
}

func (s *Server) getNote(w http.ResponseWriter, r *http.Request) {
	ag, ok := s.agent(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid note id")
		return
	}
	v, err := s.Store.Latest(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	if !s.canRead(w, r, ag.ID, v.SpaceID) {
		return
	}
	if sp := r.URL.Query().Get("space"); sp != "" && v.SpaceID != sp && !strings.HasPrefix(v.SpaceID, sp+"/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("ETag", `"`+v.BodyHash+`"`)
	w.Header().Set("Cache-Control", "no-cache")
	if strings.Trim(r.Header.Get("If-None-Match"), `"`) == v.BodyHash {
		if err := s.Store.RecordRead(r.Context(), v.NoteID, v.Version, ag.ID); err != nil {
			s.fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if err := s.Store.RecordRead(r.Context(), v.NoteID, v.Version, ag.ID); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toNote(v, nil, true))
}

func (s *Server) canRead(w http.ResponseWriter, r *http.Request, agentID, space string) bool {
	sp, err := s.Store.Space(r.Context(), space)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return false
	}
	if err != nil {
		s.fail(w, err)
		return false
	}
	if !note.Can(sp.ACL, agentID, false) {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

func (s *Server) noteVersions(w http.ResponseWriter, r *http.Request) {
	ag, ok := s.agent(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid note id")
		return
	}
	latest, err := s.Store.Latest(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	if !s.canRead(w, r, ag.ID, latest.SpaceID) {
		return
	}
	s.writeVersions(w, r, id)
}

func (s *Server) writeVersions(w http.ResponseWriter, r *http.Request, id string) {
	list, err := s.Store.Versions(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	to, _ := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	type row struct {
		Version   int64  `json:"version"`
		BodyHash  string `json:"body_hash"`
		PrevHash  string `json:"prev_hash"`
		AgentID   string `json:"agent_id"`
		CreatedAt string `json:"created_at"`
		Size      int    `json:"size"`
	}
	out := make([]row, 0, len(list))
	for _, v := range list {
		if from > 0 && v.Version < from {
			continue
		}
		if to > 0 && v.Version > to {
			continue
		}
		out = append(out, row{
			Version: v.Version, BodyHash: v.BodyHash, PrevHash: v.PrevHash, AgentID: v.AgentID,
			CreatedAt: v.CreatedAt.UTC().Format(time.RFC3339Nano), Size: len(v.Body),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": out})
}

func (s *Server) noteDiff(w http.ResponseWriter, r *http.Request) {
	ag, ok := s.agent(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid note id")
		return
	}
	latest, err := s.Store.Latest(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	if !s.canRead(w, r, ag.ID, latest.SpaceID) {
		return
	}
	s.writeDiff(w, r, id)
}

func (s *Server) writeDiff(w http.ResponseWriter, r *http.Request, id string) {
	fromH := r.URL.Query().Get("from")
	toH := r.URL.Query().Get("to")
	if fromH == "" || toH == "" {
		writeError(w, http.StatusBadRequest, "from and to hashes required")
		return
	}
	from, err := s.Store.VersionByHash(r.Context(), id, fromH)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "from hash not found")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	to, err := s.Store.VersionByHash(r.Context(), id, toH)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "to hash not found")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from": fromH,
		"to":   toH,
		"diff": note.Unified(from.Body, to.Body),
	})
}

func (s *Server) dm(w http.ResponseWriter, r *http.Request) {
	ag, ok := s.agent(w, r)
	if !ok {
		return
	}
	target := r.PathValue("id")
	if !validUUID(target) {
		writeError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	if target == ag.ID {
		writeError(w, http.StatusBadRequest, "cannot dm yourself")
		return
	}
	other, err := s.Store.AgentByID(r.Context(), target)
	if errors.Is(err, store.ErrNotFound) || other.Status == "revoked" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	var req struct {
		Body json.RawMessage `json:"body"`
	}
	if err := readJSON(w, r, &req, int64(s.Cfg.Limits.NoteMaxBytes)+4096); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	body, hash, err := note.Normalize(req.Body)
	if err != nil {
		writeNoteErr(w, err)
		return
	}
	id, err := note.NewID(time.Now())
	if err != nil {
		s.fail(w, err)
		return
	}
	space := note.DMSpace(ag.ID, other.ID)
	res, err := s.Store.CreateNote(r.Context(), store.Version{
		NoteID: id, AgentID: ag.ID, SpaceID: space, Body: body, BodyHash: hash,
	}, note.DMACL(ag.ID, other.ID), s.limitsFor(ag.Quota), allowWrite(ag.ID))
	s.writeResult(w, http.StatusCreated, res, err)
}

func writeNoteErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, note.ErrBodyRequired), errors.Is(err, note.ErrBadJSON):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, note.ErrBodyTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "body exceeds cap")
	default:
		writeError(w, http.StatusBadRequest, "invalid body")
	}
}
