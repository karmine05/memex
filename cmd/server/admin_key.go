package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"memex/internal/auth"
)

func loadAdminKey(dir string) (string, error) {
	if v := strings.TrimSpace(os.Getenv("MEMEX_ADMIN_KEY")); v != "" {
		if !strings.HasPrefix(v, auth.PrefixAdmin) {
			return "", fmt.Errorf("MEMEX_ADMIN_KEY must start with %s", auth.PrefixAdmin)
		}
		return v, nil
	}
	path := filepath.Join(dir, "admin.key")
	b, err := os.ReadFile(path)
	if err == nil {
		key := strings.TrimSpace(string(b))
		if !strings.HasPrefix(key, auth.PrefixAdmin) {
			return "", fmt.Errorf("%s does not contain an admin key", path)
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	key, err := auth.NewAdminKey()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(key+"\n"), 0600); err != nil {
		return "", err
	}
	fmt.Fprintf(os.Stderr, "admin key written to %s\n", path)
	return key, nil
}
