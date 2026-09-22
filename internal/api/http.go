package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"memex/internal/store"
)

type errBody struct {
	Error string `json:"error"`
}

type conflictBody struct {
	Error    string `json:"error"`
	BodyHash string `json:"body_hash"`
	PrevHash string `json:"prev_hash"`
}

type noteBody struct {
	NoteID    string          `json:"note_id"`
	Version   int64           `json:"version"`
	Space     string          `json:"space"`
	AgentID   string          `json:"agent_id"`
	Body      json.RawMessage `json:"body,omitempty"`
	BodyHash  string          `json:"body_hash"`
	PrevHash  string          `json:"prev_hash"`
	CreatedAt string          `json:"created_at"`
	Score     *float64        `json:"score,omitempty"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		http.Error(w, `{"error":"encode"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(buf.Bytes())
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, errBody{Error: msg})
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if len(h) < len(p) || !strings.EqualFold(h[:len(p)], p) {
		return ""
	}
	return strings.TrimSpace(h[len(p):])
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any, max int64) error {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		return errors.New("content type")
	}
	r.Body = http.MaxBytesReader(w, r.Body, max)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return err
		}
		return err
	}
	return nil
}

func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func rawBody(stored string) json.RawMessage {
	if json.Valid([]byte(stored)) {
		return json.RawMessage(stored)
	}
	b, err := json.Marshal(stored)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return b
}

func toNote(v store.Version, score *float64, withBody bool) noteBody {
	n := noteBody{
		NoteID:    v.NoteID,
		Version:   v.Version,
		Space:     v.SpaceID,
		AgentID:   v.AgentID,
		BodyHash:  v.BodyHash,
		PrevHash:  v.PrevHash,
		CreatedAt: v.CreatedAt.UTC().Format(time.RFC3339Nano),
		Score:     score,
	}
	if withBody {
		n.Body = rawBody(v.Body)
	}
	return n
}

func retryAfter(w http.ResponseWriter, seconds int) {
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
}

type statusWriter struct {
	http.ResponseWriter
	code  int
	wrote bool
}

func (s *statusWriter) WriteHeader(code int) {
	if s.wrote {
		return
	}
	s.wrote = true
	s.code = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if !s.wrote {
		s.WriteHeader(http.StatusOK)
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }
