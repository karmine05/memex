package api

import (
	"bytes"
	"os"
	"testing"
)

func TestGeneratedDocsMatchRoutes(t *testing.T) {
	spec, err := os.ReadFile("../../docs/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(spec), bytes.TrimSpace(OpenAPI())) {
		t.Fatal("docs/openapi.json is stale; run make api")
	}
	md, err := os.ReadFile("../../docs/api.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(md), bytes.TrimSpace(APIDocs())) {
		t.Fatal("docs/api.md is stale; run make api")
	}
}

func TestSkillMDMatchesDocs(t *testing.T) {
	// The embedded skill.md is a build artifact of docs/SKILL.md. Keep the
	// served protocol in lockstep with the canonical file.
	docs, err := os.ReadFile("../../docs/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(docs, skillMD) {
		t.Fatal("internal/api/skill.md is stale; run make api")
	}
}
