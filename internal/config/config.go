package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const Version = "1.0.0"

type Config struct {
	Mode         string
	Listen       string
	AdminListen  string
	DB           string
	DataDir      string
	Registration string
	Embed        Embed
	Search       Search
	Limits       Limits
	Invites      []string
}

type Embed struct {
	Provider string
	Model    string
	URL      string
	APIKey   string
	Dim      int
}

type Search struct {
	Vector   float64
	Lexical  float64
	Recency  float64
	HalfLife time.Duration
}

type Limits struct {
	NotesPerHour      int     `json:"notes_per_hour"`
	BytesPerHour      int64   `json:"bytes_per_hour"`
	NoteMaxBytes      int     `json:"note_max_bytes"`
	HistoryMaxBytes   int64   `json:"note_history_max"`
	RequestsPerMinute float64 `json:"requests_per_minute"`
}

func Default() Config {
	return Config{
		Mode:         "private",
		Listen:       "127.0.0.1:8843",
		AdminListen:  "127.0.0.1:8844",
		DB:           "postgres://memex:memex@127.0.0.1:5433/memex?sslmode=disable",
		DataDir:      "data",
		Registration: "bootstrap",
		Embed: Embed{
			Provider: "ollama",
			Model:    "nomic-embed-text",
			URL:      "http://127.0.0.1:11434",
			Dim:      768,
		},
		Search: Search{
			Vector:   0.5,
			Lexical:  0.3,
			Recency:  0.2,
			HalfLife: 72 * time.Hour,
		},
		Limits: Limits{
			NotesPerHour:      30,
			BytesPerHour:      30 * 1024 * 1024,
			NoteMaxBytes:      64 * 1024,
			HistoryMaxBytes:   4 * 1024 * 1024,
			RequestsPerMinute: 100,
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("config %s: %w", path, err)
		}
		if err := applyFile(&cfg, b); err != nil {
			return Config{}, fmt.Errorf("config %s: %w", path, err)
		}
	}
	applyEnv(&cfg)
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	switch c.Mode {
	case "private", "public":
	default:
		return fmt.Errorf("mode %q", c.Mode)
	}
	switch c.Registration {
	case "bootstrap", "invite", "open":
	default:
		return fmt.Errorf("registration %q", c.Registration)
	}
	if c.Registration == "open" && os.Getenv("MEMEX_ALLOW_OPEN") != "true" {
		return fmt.Errorf("open registration lets anyone who can reach the port register an agent; set MEMEX_ALLOW_OPEN=true to confirm")
	}
	switch c.Embed.Provider {
	case "ollama", "openai", "none":
	default:
		return fmt.Errorf("embed provider %q", c.Embed.Provider)
	}
	if c.Search.HalfLife <= 0 {
		return fmt.Errorf("recency half-life must be positive")
	}
	if c.Limits.NoteMaxBytes <= 0 || c.Limits.RequestsPerMinute <= 0 {
		return fmt.Errorf("limits must be positive")
	}
	return nil
}

func (c Config) ValidInvite(code string) bool {
	if code == "" {
		return false
	}
	for _, inv := range c.Invites {
		if subtleEq(inv, code) {
			return true
		}
	}
	return false
}

func subtleEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func RedactDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.User == nil {
		return dsn
	}
	u.User = url.UserPassword(u.User.Username(), "redacted")
	return u.String()
}

func applyEnv(c *Config) {
	set := func(key string, dst *string) {
		if v := os.Getenv(key); v != "" {
			*dst = v
		}
	}
	set("MEMEX_MODE", &c.Mode)
	set("MEMEX_LISTEN", &c.Listen)
	set("MEMEX_ADMIN_LISTEN", &c.AdminListen)
	set("MEMEX_DB", &c.DB)
	set("MEMEX_DATA_DIR", &c.DataDir)
	set("MEMEX_REGISTRATION", &c.Registration)
	set("MEMEX_EMBED_PROVIDER", &c.Embed.Provider)
	set("MEMEX_EMBED_URL", &c.Embed.URL)
	set("MEMEX_EMBED_MODEL", &c.Embed.Model)
	set("MEMEX_EMBED_API_KEY", &c.Embed.APIKey)
}

func applyFile(c *Config, b []byte) error {
	trim := strings.TrimSpace(string(b))
	var m map[string]any
	var err error
	if strings.HasPrefix(trim, "{") {
		err = json.Unmarshal(b, &m)
	} else {
		m, err = parseYAML(trim)
	}
	if err != nil {
		return err
	}
	return fill(c, m)
}

func fill(c *Config, m map[string]any) error {
	if v, ok := str(m, "mode"); ok {
		c.Mode = v
	}
	if v, ok := str(m, "listen"); ok {
		c.Listen = v
	}
	if v, ok := str(m, "admin_listen"); ok {
		c.AdminListen = v
	}
	if v, ok := str(m, "db"); ok {
		c.DB = v
	}
	if v, ok := str(m, "data_dir"); ok {
		c.DataDir = v
	}
	if emb, ok := m["embed"].(map[string]any); ok {
		if v, ok := str(emb, "provider"); ok {
			c.Embed.Provider = v
		}
		if v, ok := str(emb, "model"); ok {
			c.Embed.Model = v
		}
		if v, ok := str(emb, "url"); ok {
			c.Embed.URL = v
		}
		if v, ok := str(emb, "api_key"); ok {
			c.Embed.APIKey = v
		}
		if v, ok := num(emb, "dim"); ok {
			c.Embed.Dim = int(v)
		}
	}
	if s, ok := m["search"].(map[string]any); ok {
		if w, ok := s["weights"].(map[string]any); ok {
			if v, ok := num(w, "vector"); ok {
				c.Search.Vector = v
			}
			if v, ok := num(w, "lexical"); ok {
				c.Search.Lexical = v
			}
			if v, ok := num(w, "recency"); ok {
				c.Search.Recency = v
			}
		}
		if v, ok := str(s, "recency_halflife"); ok {
			d, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("recency_halflife: %w", err)
			}
			c.Search.HalfLife = d
		}
	}
	if lim, ok := m["limits"].(map[string]any); ok {
		if v, ok := num(lim, "notes_per_hour"); ok {
			c.Limits.NotesPerHour = int(v)
		}
		if v, ok := num(lim, "bytes_per_hour"); ok {
			c.Limits.BytesPerHour = int64(v)
		}
		if v, ok := num(lim, "note_max_bytes"); ok {
			c.Limits.NoteMaxBytes = int(v)
		}
		if v, ok := num(lim, "note_history_max"); ok {
			c.Limits.HistoryMaxBytes = int64(v)
		}
		if v, ok := num(lim, "requests_per_minute"); ok {
			c.Limits.RequestsPerMinute = v
		}
	}
	if pub, ok := m["public"].(map[string]any); ok {
		if v, ok := str(pub, "registration"); ok {
			c.Registration = v
		}
		if list, ok := pub["invites"].([]any); ok {
			c.Invites = c.Invites[:0]
			for _, item := range list {
				s, ok := item.(string)
				if !ok {
					return fmt.Errorf("invite code is not a string")
				}
				c.Invites = append(c.Invites, s)
			}
		}
	}
	if c.Mode == "public" && c.Registration == "bootstrap" {
		c.Registration = "invite"
	}
	if c.Mode == "private" && !envSet("MEMEX_REGISTRATION") && c.Registration == "invite" && len(c.Invites) == 0 {
		c.Registration = "bootstrap"
	}
	return nil
}

func envSet(k string) bool { return os.Getenv(k) != "" }

func str(m map[string]any, k string) (string, bool) {
	v, ok := m[k]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func num(m map[string]any, k string) (float64, bool) {
	v, ok := m[k]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

type yline struct {
	indent int
	text   string
}

func parseYAML(src string) (map[string]any, error) {
	var lines []yline
	for _, raw := range strings.Split(src, "\n") {
		if i := strings.Index(raw, " #"); i >= 0 && !inQuote(raw[:i]) {
			raw = raw[:i]
		}
		trim := strings.TrimSpace(raw)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		lines = append(lines, yline{indent: indent, text: trim})
	}
	m, n, err := parseMap(lines, 0)
	if err != nil {
		return nil, err
	}
	if n != len(lines) {
		return nil, fmt.Errorf("yaml trailing content at %q", lines[n].text)
	}
	return m, nil
}

func inQuote(s string) bool {
	return strings.Count(s, `"`)%2 == 1
}

func parseMap(lines []yline, indent int) (map[string]any, int, error) {
	m := map[string]any{}
	i := 0
	for i < len(lines) {
		if lines[i].indent < indent {
			break
		}
		if lines[i].indent != indent {
			return nil, i, fmt.Errorf("yaml indent at %q", lines[i].text)
		}
		key, val, has := splitKV(lines[i].text)
		if key == "" {
			return nil, i, fmt.Errorf("yaml key at %q", lines[i].text)
		}
		if !has {
			if i+1 >= len(lines) || lines[i+1].indent <= indent {
				m[key] = map[string]any{}
				i++
				continue
			}
			if strings.HasPrefix(lines[i+1].text, "- ") {
				list, n, err := parseList(lines[i+1:], lines[i+1].indent)
				if err != nil {
					return nil, i, err
				}
				m[key] = list
				i = i + 1 + n
				continue
			}
			child, n, err := parseMap(lines[i+1:], lines[i+1].indent)
			if err != nil {
				return nil, i, err
			}
			m[key] = child
			i = i + 1 + n
			continue
		}
		m[key] = scalar(val)
		i++
	}
	return m, i, nil
}

func parseList(lines []yline, indent int) ([]any, int, error) {
	var out []any
	i := 0
	for i < len(lines) {
		if lines[i].indent < indent {
			break
		}
		if lines[i].indent != indent || !strings.HasPrefix(lines[i].text, "- ") {
			return nil, i, fmt.Errorf("yaml list at %q", lines[i].text)
		}
		out = append(out, scalar(strings.TrimSpace(lines[i].text[2:])))
		i++
	}
	return out, i, nil
}

func splitKV(text string) (string, string, bool) {
	if strings.HasPrefix(text, "- ") {
		return "", "", false
	}
	i := strings.Index(text, ":")
	if i <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(text[:i])
	rest := strings.TrimSpace(text[i+1:])
	if rest == "" || rest == "|" || rest == ">" {
		return key, "", false
	}
	return key, rest, true
}

func scalar(v string) any {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		u, err := strconv.Unquote(v)
		if err == nil {
			return u
		}
	}
	if v == "true" {
		return "true"
	}
	if v == "false" {
		return "false"
	}
	if v == "null" {
		return nil
	}
	if n, err := strconv.ParseFloat(v, 64); err == nil {
		return n
	}
	return v
}
