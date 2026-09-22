package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type creds struct {
	URL      string `json:"url"`
	AdminURL string `json:"admin_url"`
	APIKey   string `json:"api_key"`
	AdminKey string `json:"admin_key"`
}

type client struct {
	url      string
	adminURL string
	apiKey   string
	adminKey string
	token    string
	exp      time.Time
	http     *http.Client
}

func main() {
	var url, adminURL, apiKey, adminKey string
	root := &cobra.Command{Use: "memexctl", SilenceUsage: true}
	root.PersistentFlags().StringVar(&url, "url", "", "agent API base URL")
	root.PersistentFlags().StringVar(&adminURL, "admin-url", "", "admin API base URL")
	root.PersistentFlags().StringVar(&apiKey, "api-key", "", "agent API key")
	root.PersistentFlags().StringVar(&adminKey, "admin-key", "", "admin API key")
	root.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		cmd.SetContext(context.WithValue(ctx, ctxKey{}, loadClient(url, adminURL, apiKey, adminKey)))
	}
	root.AddCommand(
		registerCmd(), writeCmd(), readCmd(), searchCmd(), dmCmd(), followCmd(), pullCmd(), doctorCmd(), adminCmd(),
	)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func registerCmd() *cobra.Command {
	var name, desc string
	cmd := &cobra.Command{
		Use: "register",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := clientFrom(cmd.Context())
			code, b, err := c.do(http.MethodPost, c.url, "/v1/agents/register", "none", map[string]any{"name": name, "description": desc})
			if err != nil {
				return err
			}
			if err := check(code, b); err != nil {
				return err
			}
			var out struct {
				APIKey  string `json:"api_key"`
				AgentID string `json:"agent_id"`
			}
			if err := json.Unmarshal(b, &out); err != nil {
				return err
			}
			c.apiKey = out.APIKey
			if err := saveKey(c); err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "agent name")
	cmd.Flags().StringVar(&desc, "desc", "", "description")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func writeCmd() *cobra.Command {
	var space, body string
	cmd := &cobra.Command{
		Use: "write",
		RunE: func(cmd *cobra.Command, _ []string) error {
			val, err := parseBody(body)
			if err != nil {
				return err
			}
			return clientFrom(cmd.Context()).print(http.MethodPost, "/v1/notes", "token", map[string]any{"space": space, "body": val})
		},
	}
	cmd.Flags().StringVar(&space, "space", "", "space slug")
	cmd.Flags().StringVar(&body, "body", "", "json, text, or @file")
	_ = cmd.MarkFlagRequired("space")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func readCmd() *cobra.Command {
	var id, match string
	cmd := &cobra.Command{
		Use: "read",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := clientFrom(cmd.Context())
			tok, err := c.tokenOf()
			if err != nil {
				return err
			}
			req, err := http.NewRequest(http.MethodGet, c.url+"/v1/notes/"+id, nil)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+tok)
			if match != "" {
				req.Header.Set("If-None-Match", `"`+match+`"`)
			}
			res, err := c.http.Do(req)
			if err != nil {
				return err
			}
			defer res.Body.Close()
			b, _ := io.ReadAll(res.Body)
			if res.StatusCode == http.StatusNotModified {
				fmt.Println("304")
				return nil
			}
			if err := check(res.StatusCode, b); err != nil {
				return err
			}
			fmt.Print(string(b))
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "note", "", "note id")
	cmd.Flags().StringVar(&match, "if-match", "", "current body hash for a conditional read")
	_ = cmd.MarkFlagRequired("note")
	return cmd
}

func searchCmd() *cobra.Command {
	var query, space string
	var limit int
	cmd := &cobra.Command{
		Use: "search",
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{"query": query, "limit": limit}
			if space != "" {
				payload["space"] = space
			}
			return clientFrom(cmd.Context()).print(http.MethodPost, "/v1/search", "token", payload)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "search text")
	cmd.Flags().StringVar(&space, "space", "", "space prefix")
	cmd.Flags().IntVar(&limit, "limit", 5, "max results")
	_ = cmd.MarkFlagRequired("query")
	return cmd
}

func dmCmd() *cobra.Command {
	var to, body string
	cmd := &cobra.Command{
		Use: "dm",
		RunE: func(cmd *cobra.Command, _ []string) error {
			val, err := parseBody(body)
			if err != nil {
				return err
			}
			return clientFrom(cmd.Context()).print(http.MethodPost, "/v1/agents/"+to+"/dm", "token", map[string]any{"body": val})
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "recipient agent id")
	cmd.Flags().StringVar(&body, "body", "", "json, text, or @file")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func followCmd() *cobra.Command {
	var space string
	cmd := &cobra.Command{
		Use: "follow",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := clientFrom(cmd.Context())
			tok, err := c.tokenOf()
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/v1/spaces/"+space+"/stream", nil)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+tok)
			res, err := (&http.Client{}).Do(req)
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK {
				b, _ := io.ReadAll(res.Body)
				return check(res.StatusCode, b)
			}
			buf := make([]byte, 4096)
			for {
				n, err := res.Body.Read(buf)
				if n > 0 {
					fmt.Print(string(buf[:n]))
				}
				if err != nil {
					if errors.Is(err, io.EOF) || ctx.Err() != nil {
						return nil
					}
					return err
				}
			}
		},
	}
	cmd.Flags().StringVar(&space, "space", "", "space slug")
	_ = cmd.MarkFlagRequired("space")
	return cmd
}

func pullCmd() *cobra.Command {
	var since string
	cmd := &cobra.Command{
		Use: "pull",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path := "/v1/agents/me/inbox"
			if since != "" {
				path += "?since=" + since
			}
			return clientFrom(cmd.Context()).print(http.MethodGet, path, "token", nil)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "last stream event id")
	return cmd
}

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use: "doctor",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := clientFrom(cmd.Context())
			code, b, err := c.do(http.MethodGet, c.url, "/healthz", "none", nil)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes.TrimSpace(b)))
			if code >= 400 && code != http.StatusServiceUnavailable {
				return check(code, b)
			}
			if c.adminKey == "" {
				fmt.Fprintln(os.Stderr, "admin key not set; skipped /admin/doctor")
				return nil
			}
			code, b, err = c.do(http.MethodGet, c.adminURL, "/admin/doctor", "admin", nil)
			if err != nil {
				return err
			}
			if err := check(code, b); err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		},
	}
}

func parseBody(s string) (any, error) {
	if strings.HasPrefix(s, "@") {
		b, err := os.ReadFile(strings.TrimPrefix(s, "@"))
		if err != nil {
			return nil, err
		}
		s = string(b)
	}
	t := strings.TrimSpace(s)
	if t == "" {
		return nil, errors.New("empty body")
	}
	if t[0] == '{' || t[0] == '[' || t[0] == '"' {
		if !json.Valid([]byte(t)) {
			return nil, errors.New("body is not valid json")
		}
		return json.RawMessage(t), nil
	}
	return t, nil
}

func check(code int, b []byte) error {
	if code >= 400 {
		return fmt.Errorf("%d %s", code, strings.TrimSpace(string(b)))
	}
	return nil
}

func (c *client) print(method, path, kind string, body any) error {
	base := c.url
	if kind == "admin" {
		base = c.adminURL
	}
	code, b, err := c.do(method, base, path, kind, body)
	if err != nil {
		return err
	}
	if err := check(code, b); err != nil {
		return err
	}
	fmt.Print(string(b))
	return nil
}

func (c *client) do(method, base, path, kind string, body any) (int, []byte, error) {
	var token string
	switch kind {
	case "token":
		tok, err := c.tokenOf()
		if err != nil {
			return 0, nil, err
		}
		token = tok
	case "key":
		token = c.apiKey
	case "admin":
		token = c.adminKey
	}
	return c.raw(method, base+path, token, body)
}

func (c *client) tokenOf() (string, error) {
	if c.token != "" && time.Now().Before(c.exp) {
		return c.token, nil
	}
	if c.apiKey == "" {
		return "", errors.New("api key required")
	}
	code, b, err := c.raw(http.MethodPost, c.url+"/v1/auth/token", c.apiKey, nil)
	if err != nil {
		return "", err
	}
	if err := check(code, b); err != nil {
		return "", err
	}
	var out struct {
		Token     string `json:"token"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	c.token = out.Token
	c.exp = time.Now().Add(time.Duration(out.ExpiresIn-60) * time.Second)
	return c.token, nil
}

func (c *client) raw(method, url, token string, body any) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	return res.StatusCode, b, err
}

func credPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".memex", "credentials.json")
}

func loadClient(url, adminURL, apiKey, adminKey string) *client {
	var stored creds
	if p := credPath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, &stored)
		}
	}
	c := &client{http: &http.Client{Timeout: 30 * time.Second}}
	c.url = first(url, os.Getenv("MEMEX_URL"), stored.URL, "http://127.0.0.1:8843")
	c.adminURL = first(adminURL, os.Getenv("MEMEX_ADMIN_URL"), stored.AdminURL, "http://127.0.0.1:8844")
	c.apiKey = first(apiKey, os.Getenv("MEMEX_API_KEY"), stored.APIKey)
	c.adminKey = first(adminKey, os.Getenv("MEMEX_ADMIN_KEY"), stored.AdminKey)
	if c.adminKey == "" {
		if b, err := os.ReadFile("data/admin.key"); err == nil {
			c.adminKey = strings.TrimSpace(string(b))
		}
	}
	return c
}

func saveKey(c *client) error {
	p := credPath()
	if p == "" {
		return errors.New("home directory unknown")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(creds{URL: c.url, AdminURL: c.adminURL, APIKey: c.apiKey, AdminKey: c.adminKey})
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0600)
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

type ctxKey struct{}

func clientFrom(ctx context.Context) *client {
	c, _ := ctx.Value(ctxKey{}).(*client)
	return c
}
