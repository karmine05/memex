package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestYAMLOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memex.yml")
	body := []byte(`
mode: public
listen: ":8843"
embed:
  provider: openai
  model: nomic-embed-text
  url: http://embed.internal
search:
  weights:
    vector: 0.5
    lexical: 0.3
    recency: 0.2
  recency_halflife: 72h
limits:
  notes_per_hour: 10
public:
  registration: invite
  invites:
    - alpha
    - beta
`)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != "public" || cfg.Registration != "invite" || cfg.Embed.Provider != "openai" {
		t.Fatalf("%+v", cfg)
	}
	if cfg.Search.HalfLife != 72*time.Hour || cfg.Limits.NotesPerHour != 10 {
		t.Fatalf("%+v", cfg.Search)
	}
	if !cfg.ValidInvite("alpha") || cfg.ValidInvite("nope") {
		t.Fatal("invite")
	}
	if RedactDSN(cfg.DB) == cfg.DB && cfg.DB != "" {
		t.Fatal("dsn should redact a password")
	}
}

func TestOpenRegistrationRequiresExplicitOptIn(t *testing.T) {
	os.Unsetenv("MEMEX_ALLOW_OPEN")
	t.Cleanup(func() { os.Unsetenv("MEMEX_ALLOW_OPEN") })
	dir := t.TempDir()
	for _, mode := range []string{"public", "private"} {
		path := filepath.Join(dir, mode+".yml")
		if err := os.WriteFile(path, []byte("mode: "+mode+"\npublic:\n  registration: open\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("%s+open must refuse to boot without MEMEX_ALLOW_OPEN=true", mode)
		}
	}
	t.Setenv("MEMEX_ALLOW_OPEN", "true")
	cfg, err := Load(filepath.Join(dir, "public.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Registration != "open" {
		t.Fatalf("opt-in should load open registration: %+v", cfg)
	}
}
