package note

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"memex/internal/jcs"
)

const (
	MaxVersionBytes = 64 * 1024
	MaxHistoryBytes = 4 * 1024 * 1024
)

var (
	ErrBodyRequired = errors.New("body required")
	ErrBodyTooLarge = errors.New("body exceeds 64KB")
	ErrBadJSON      = errors.New("invalid json body")
)

// NewID returns a UUIDv7. Callers pass the clock so request code owns time.Now.
func NewID(now time.Time) (string, error) {
	var b [16]byte
	ms := uint64(now.UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	if _, err := rand.Read(b[6:]); err != nil {
		return "", fmt.Errorf("uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// DecodeBody unwraps the JSON value agents send as `body`.
// A JSON string is plain text. Objects and arrays stay JSON text.
func DecodeBody(raw json.RawMessage) (string, error) {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return "", ErrBodyRequired
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", fmt.Errorf("body: %w", err)
		}
		return s, nil
	}
	return string(raw), nil
}

// Canonicalize applies JCS when the text is a JSON object or array.
// Other text is returned unchanged so equal prose hashes equal.
func Canonicalize(body string) (string, error) {
	t := strings.TrimSpace(body)
	if t == "" {
		return body, nil
	}
	if t[0] != '{' && t[0] != '[' {
		return body, nil
	}
	if !json.Valid([]byte(t)) {
		return body, nil
	}
	out, err := jcs.Transform([]byte(t))
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrBadJSON, err.Error())
	}
	return string(out), nil
}

// Normalize stores the canonical bytes and the hash of those exact bytes.
func Normalize(raw json.RawMessage) (string, string, error) {
	text, err := DecodeBody(raw)
	if err != nil {
		return "", "", err
	}
	canon, err := Canonicalize(text)
	if err != nil {
		return "", "", err
	}
	if len(canon) > MaxVersionBytes {
		return "", "", ErrBodyTooLarge
	}
	return canon, HashBytes(canon), nil
}

func HashBytes(canon string) string {
	sum := sha256.Sum256([]byte(canon))
	return hex.EncodeToString(sum[:])
}

// Hash recomputes the content hash. Canonicalize is idempotent on JCS output.
func Hash(body string) (string, error) {
	canon, err := Canonicalize(body)
	if err != nil {
		return "", err
	}
	return HashBytes(canon), nil
}

type ChainItem struct {
	Version  int64
	Body     string
	BodyHash string
	PrevHash string
}

// Verify walks a note's version chain. The error names the first broken version.
func Verify(items []ChainItem) error {
	if len(items) == 0 {
		return errors.New("empty chain")
	}
	prev := ""
	for i, it := range items {
		if it.Version != int64(i+1) {
			return fmt.Errorf("version %d: gap", it.Version)
		}
		h, err := Hash(it.Body)
		if err != nil {
			return fmt.Errorf("version %d: %w", it.Version, err)
		}
		if h != it.BodyHash {
			return fmt.Errorf("version %d: hash mismatch", it.Version)
		}
		if it.PrevHash != prev {
			return fmt.Errorf("version %d: prev_hash mismatch", it.Version)
		}
		prev = it.BodyHash
	}
	return nil
}

// DMSpace is the synthetic space for a pair of agents. Order does not matter.
func DMSpace(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return "dm/" + a + "/" + b
}

func DMParticipants(space string) (string, string, bool) {
	rest, ok := strings.CutPrefix(space, "dm/")
	if !ok {
		return "", "", false
	}
	a, b, ok := strings.Cut(rest, "/")
	if !ok || a == "" || b == "" || strings.Contains(b, "/") {
		return "", "", false
	}
	return a, b, true
}

func IsDM(space string) bool {
	_, _, ok := DMParticipants(space)
	return ok
}

func ValidSpace(s string) bool {
	if len(s) == 0 || len(s) > 200 {
		return false
	}
	if strings.Contains(s, "..") || strings.HasPrefix(s, "/") || strings.HasSuffix(s, "/") {
		return false
	}
	for i, r := range s {
		if !spaceRune(r) {
			return false
		}
		if i == 0 && (r == '.' || r == '_' || r == '/' || r == '-') {
			return false
		}
	}
	return true
}

func spaceRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
	case r >= 'A' && r <= 'Z':
	case r >= '0' && r <= '9':
	case r == '.' || r == '_' || r == '/' || r == '-':
	default:
		return false
	}
	return true
}

func ValidName(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-':
		default:
			return false
		}
		if i == 0 && (r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

type aclDoc struct {
	Read   []string `json:"read"`
	Write  []string `json:"write"`
	Locked bool     `json:"locked"`
}

func parseACL(raw []byte) (aclDoc, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return aclDoc{}, nil
	}
	var doc aclDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return aclDoc{}, fmt.Errorf("acl: %w", err)
	}
	return doc, nil
}

func listed(ids []string, agentID string) bool {
	if len(ids) == 0 {
		return true
	}
	for _, id := range ids {
		if id == "*" || id == agentID {
			return true
		}
	}
	return false
}

// Can reports whether agentID may read or write under this ACL.
// An empty ACL is the default-open memory network.
func Can(raw []byte, agentID string, write bool) bool {
	doc, err := parseACL(raw)
	if err != nil {
		return false
	}
	if write && doc.Locked {
		return false
	}
	list := doc.Read
	if write {
		list = doc.Write
	}
	if listed(list, agentID) {
		return true
	}
	if !write && listed(doc.Write, agentID) {
		return true
	}
	return false
}

func Locked(raw []byte) bool {
	doc, err := parseACL(raw)
	if err != nil {
		return true
	}
	return doc.Locked
}

// DMACL is the participant-only ACL stored on a DM space.
func DMACL(a, b string) []byte {
	doc := aclDoc{Read: []string{a, b}, Write: []string{a, b}}
	bts, err := json.Marshal(doc)
	if err != nil {
		return []byte(`{"read":[],"write":[]}`)
	}
	return bts
}

// LockACL keeps the existing lists and marks the space read-only.
func LockACL(raw []byte) ([]byte, error) {
	doc, err := parseACL(raw)
	if err != nil {
		return nil, err
	}
	doc.Locked = true
	return json.Marshal(doc)
}
