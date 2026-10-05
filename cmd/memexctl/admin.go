package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"memex/internal/note"
)

func adminCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "admin"}
	cmd.AddCommand(
		adminAgents(), adminCreate(), adminStats(), adminSimple("revoke", "/revoke"),
		adminSimple("rotate", "/rotate"), adminSimple("suspend", "/suspend"), adminSimple("resume", "/resume"),
		adminNote(), adminDiff(), adminReaders(), adminAudit(), adminVerify(), adminPurge(),
		adminReembed(), adminOrphans(), adminDupes(), adminEval(), adminSpace(), adminVolume(),
		adminSkill(),
	)
	return cmd
}

// adminSkill prints the agent protocol (SKILL.md) from the admin port, the
// same source the admin webUI uses to build a one-prompt install.
func adminSkill() *cobra.Command {
	return &cobra.Command{
		Use:   "skill",
		Short: "Print the agent protocol for onboarding (admin port)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := clientFrom(cmd.Context())
			code, b, err := c.do(http.MethodGet, c.adminURL, "/skill.md", "none", nil)
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

func adminAgents() *cobra.Command {
	return &cobra.Command{
		Use: "agents",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return clientFrom(cmd.Context()).print(http.MethodGet, "/admin/agents", "admin", nil)
		},
	}
}

func adminCreate() *cobra.Command {
	var name, desc string
	cmd := &cobra.Command{
		Use: "create-agent-key",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return clientFrom(cmd.Context()).print(http.MethodPost, "/admin/agents", "admin", map[string]any{"name": name, "description": desc})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "agent name")
	cmd.Flags().StringVar(&desc, "desc", "", "description")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func adminStats() *cobra.Command {
	return &cobra.Command{
		Use:  "agent-stats",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clientFrom(cmd.Context()).print(http.MethodGet, "/admin/agents/"+args[0]+"/stats", "admin", nil)
		},
	}
}

func adminSimple(use, suffix string) *cobra.Command {
	return &cobra.Command{
		Use:  use,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clientFrom(cmd.Context()).print(http.MethodPost, "/admin/agents/"+args[0]+suffix, "admin", map[string]any{})
		},
	}
}

func adminNote() *cobra.Command {
	return &cobra.Command{
		Use:  "note",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clientFrom(cmd.Context()).print(http.MethodGet, "/admin/notes/"+args[0], "admin", nil)
		},
	}
}

func adminDiff() *cobra.Command {
	var from, to string
	cmd := &cobra.Command{
		Use:  "diff",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/admin/notes/" + args[0] + "/diff?from=" + from + "&to=" + to
			return clientFrom(cmd.Context()).print(http.MethodGet, path, "admin", nil)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "start hash")
	cmd.Flags().StringVar(&to, "to", "", "end hash")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

func adminReaders() *cobra.Command {
	return &cobra.Command{
		Use:  "readers",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clientFrom(cmd.Context()).print(http.MethodGet, "/admin/notes/"+args[0]+"/readers", "admin", nil)
		},
	}
}

func adminAudit() *cobra.Command {
	return &cobra.Command{
		Use:  "audit",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clientFrom(cmd.Context()).print(http.MethodGet, "/admin/notes/"+args[0]+"/audit", "admin", nil)
		},
	}
}

func adminVerify() *cobra.Command {
	return &cobra.Command{
		Use:  "verify",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := clientFrom(cmd.Context())
			code, b, err := c.do(http.MethodGet, c.adminURL, "/admin/notes/"+args[0]+"/audit", "admin", nil)
			if err != nil {
				return err
			}
			if err := check(code, b); err != nil {
				return err
			}
			var resp struct {
				Versions []struct {
					Version  int64           `json:"version"`
					Body     json.RawMessage `json:"body"`
					BodyHash string          `json:"body_hash"`
					PrevHash string          `json:"prev_hash"`
				} `json:"versions"`
			}
			if err := json.Unmarshal(b, &resp); err != nil {
				return err
			}
			items := make([]note.ChainItem, len(resp.Versions))
			for i, v := range resp.Versions {
				stored, err := note.DecodeBody(v.Body)
				if err != nil {
					return err
				}
				items[i] = note.ChainItem{Version: v.Version, Body: stored, BodyHash: v.BodyHash, PrevHash: v.PrevHash}
			}
			if err := note.Verify(items); err != nil {
				return err
			}
			fmt.Printf("ok %s versions=%d\n", args[0], len(items))
			return nil
		},
	}
}

func adminPurge() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:  "purge",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("purge deletes the note; pass --yes")
			}
			return clientFrom(cmd.Context()).print(http.MethodDelete, "/admin/notes/"+args[0]+"?confirm=yes", "admin", nil)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm purge")
	return cmd
}

func adminReembed() *cobra.Command {
	var after string
	cmd := &cobra.Command{
		Use: "re-embed",
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			if after != "" {
				body["after"] = after
			}
			return clientFrom(cmd.Context()).print(http.MethodPost, "/admin/re-embed", "admin", body)
		},
	}
	cmd.Flags().StringVar(&after, "after", "", "RFC3339 timestamp")
	return cmd
}

func adminOrphans() *cobra.Command {
	var after string
	cmd := &cobra.Command{
		Use: "orphans",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return clientFrom(cmd.Context()).print(http.MethodGet, "/admin/orphans?older_than="+after, "admin", nil)
		},
	}
	cmd.Flags().StringVar(&after, "after", "30d", "age threshold, for example 30d or 720h")
	return cmd
}

func adminDupes() *cobra.Command {
	return &cobra.Command{
		Use: "dupes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return clientFrom(cmd.Context()).print(http.MethodGet, "/admin/dupes", "admin", nil)
		},
	}
}

func adminEval() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use: "eval",
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			b = bytes.TrimSpace(b)
			if len(b) > 0 && b[0] == '[' {
				var holdout any
				if err := json.Unmarshal(b, &holdout); err != nil {
					return err
				}
				return clientFrom(cmd.Context()).print(http.MethodPost, "/admin/eval", "admin", map[string]any{"holdout": holdout})
			}
			var wrapped struct {
				Holdout json.RawMessage `json:"holdout"`
			}
			if err := json.Unmarshal(b, &wrapped); err != nil {
				return err
			}
			return clientFrom(cmd.Context()).print(http.MethodPost, "/admin/eval", "admin", map[string]any{"holdout": wrapped.Holdout})
		},
	}
	cmd.Flags().StringVar(&path, "holdout", "", "json file of {query, note_id} pairs or a raw array")
	_ = cmd.MarkFlagRequired("holdout")
	return cmd
}

func adminSpace() *cobra.Command {
	cmd := &cobra.Command{Use: "space"}
	var readers, writers string
	lock := &cobra.Command{
		Use:  "lock",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clientFrom(cmd.Context()).print(http.MethodPost, "/admin/spaces/"+args[0]+"/lock", "admin", map[string]any{})
		},
	}
	acl := &cobra.Command{
		Use:  "acl",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"read": splitCSV(readers), "write": splitCSV(writers)}
			return clientFrom(cmd.Context()).print(http.MethodPost, "/admin/spaces/"+args[0]+"/acl", "admin", body)
		},
	}
	acl.Flags().StringVar(&readers, "read", "", "comma-separated agent ids")
	acl.Flags().StringVar(&writers, "write", "", "comma-separated agent ids")
	cmd.AddCommand(lock, acl)
	return cmd
}

func adminVolume() *cobra.Command {
	var space string
	cmd := &cobra.Command{
		Use: "stats",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path := "/admin/stats"
			if space != "" {
				path += "?space=" + space
			}
			return clientFrom(cmd.Context()).print(http.MethodGet, path, "admin", nil)
		},
	}
	cmd.Flags().StringVar(&space, "space", "", "optional space")
	return cmd
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
