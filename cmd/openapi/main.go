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
	if err := write(*out, api.OpenAPI()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := write(*md, api.APIDocs()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func write(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}
