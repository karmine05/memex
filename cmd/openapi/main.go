package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"memex/internal/api"
)

func main() {
	out := flag.String("o", "docs/openapi.json", "openapi output path")
	md := flag.String("md", "docs/api.md", "markdown output path")
	flag.Parse()
	if err := syncSkill(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := write(*out, api.OpenAPI()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := write(*md, api.APIDocs()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// syncSkill copies the canonical docs/SKILL.md into the embedded copy the
// server serves. docs/SKILL.md is the source of truth; the embedded file is
// a build artifact kept in lockstep by this target and guarded by
// TestSkillMDMatchesDocs.
func syncSkill() error {
	b, err := os.ReadFile("docs/SKILL.md")
	if err != nil {
		return fmt.Errorf("read docs/SKILL.md: %w", err)
	}
	return write("internal/api/skill.md", b)
}

func write(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}
