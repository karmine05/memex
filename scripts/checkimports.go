package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type pkg struct {
	ImportPath  string
	Imports     []string
	TestImports []string
}

func main() {
	out, err := exec.Command("go", "list", "-json", "./...").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "go list: %v\n", err)
		os.Exit(1)
	}
	dec := json.NewDecoder(strings.NewReader(string(out)))
	failed := false
	for dec.More() {
		var p pkg
		if err := dec.Decode(&p); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if !strings.HasPrefix(p.ImportPath, "memex") {
			continue
		}
		for _, imp := range append(p.Imports, p.TestImports...) {
			if !allowed(p.ImportPath, imp) {
				fmt.Printf("forbidden import %s -> %s\n", p.ImportPath, imp)
				failed = true
			}
		}
	}
	if failed {
		os.Exit(1)
	}
}

func allowed(pkg, imp string) bool {
	if isStd(imp) {
		return true
	}
	if strings.HasPrefix(imp, "github.com/jackc/pgx/") {
		return strings.HasPrefix(pkg, "memex/internal/store") || strings.HasPrefix(pkg, "memex/internal/feed") || strings.HasPrefix(pkg, "memex/cmd/")
	}
	if strings.HasPrefix(imp, "github.com/spf13/") {
		return pkg == "memex/cmd/memexctl"
	}
	if strings.HasPrefix(imp, "memex/") {
		return edge(pkg, imp)
	}
	return false
}

func isStd(imp string) bool {
	return !strings.Contains(imp, ".")
}

func edge(pkg, imp string) bool {
	switch {
	case strings.HasPrefix(pkg, "memex/cmd/"):
		return strings.HasPrefix(imp, "memex/internal/") || imp == "memex/migrations"
	case pkg == "memex/internal/api":
		switch imp {
		case "memex/internal/note", "memex/internal/store", "memex/internal/search",
			"memex/internal/feed", "memex/internal/auth", "memex/internal/config":
			return true
		}
		return false
	case pkg == "memex/internal/note":
		return imp == "memex/internal/jcs"
	case pkg == "memex/internal/store":
		return imp == "memex/migrations"
	default:
		return false
	}
}
