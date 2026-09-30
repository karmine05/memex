package api

import (
	"fmt"
	"sort"
	"strings"
)

// OpenAPI is generated from Routes. make api writes this to docs/openapi.json.
func OpenAPI() []byte {
	type op struct {
		method  string
		summary string
		auth    string
	}
	byPath := map[string][]op{}
	for _, rt := range Routes() {
		auth := "bearer"
		if rt.ID == "health" || rt.ID == "admin_health" || rt.ID == "metrics" || rt.ID == "telemetry" || rt.ID == "register" || rt.ID == "skill" {
			auth = "none"
		}
		if rt.ID == "token" {
			auth = "apiKey"
		}
		byPath[rt.Path] = append(byPath[rt.Path], op{method: strings.ToLower(rt.Method), summary: rt.Summary, auth: auth})
	}
	if _, ok := byPath["/admin/spaces/{space}/lock"]; !ok {
		byPath["/admin/spaces/{space}/lock"] = []op{{method: "post", summary: "Lock a space read-only", auth: "bearer"}}
		byPath["/admin/spaces/{space}/acl"] = []op{{method: "post", summary: "Replace a space ACL", auth: "bearer"}}
		delete(byPath, "/admin/spaces/{space}")
	}
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var b strings.Builder
	b.WriteString("{\n  \"openapi\": \"3.0.3\",\n  \"info\": {\"title\": \"memex\", \"version\": \"1.0.0\"},\n")
	b.WriteString("  \"components\": {\"securitySchemes\": {\"bearer\": {\"type\": \"http\", \"scheme\": \"bearer\"}}},\n")
	b.WriteString("  \"paths\": {\n")
	for i, p := range paths {
		ops := byPath[p]
		sort.Slice(ops, func(a, c int) bool { return ops[a].method < ops[c].method })
		fmt.Fprintf(&b, "    %q: {\n", p)
		for j, op := range ops {
			fmt.Fprintf(&b, "      %q: {\"summary\": %q, \"responses\": {\"200\": {\"description\": \"ok\"}}", op.method, op.summary)
			if op.auth != "none" {
				b.WriteString(", \"security\": [{\"bearer\": []}]")
			}
			b.WriteString("}")
			if j+1 < len(ops) {
				b.WriteString(",")
			}
			b.WriteByte('\n')
		}
		b.WriteString("    }")
		if i+1 < len(paths) {
			b.WriteString(",")
		}
		b.WriteByte('\n')
	}
	b.WriteString("  }\n}\n")
	return []byte(b.String())
}

// APIDocs is the human index of the same routes.
func APIDocs() []byte {
	var b strings.Builder
	b.WriteString("# memex API\n\n")
	b.WriteString("Compact JSON. Agent routes use `Authorization: Bearer <token>` except register and token exchange. Admin routes use an `mxa_` key on the admin listener.\n\n")
	b.WriteString("| Method | Path | Listener | Summary |\n|---|---|---|---|\n")
	for _, rt := range Routes() {
		fmt.Fprintf(&b, "| `%s` | `%s` | %s | %s |\n", rt.Method, rt.Path, rt.Audience, rt.Summary)
	}
	b.WriteString("\n`POST /admin/spaces/{space}/lock` and `POST /admin/spaces/{space}/acl` are the two actions behind the space route.\n")
	b.WriteString("\nErrors are `{\"error\":\"...\"}`. A stale `base_hash` returns 409 with `body_hash` and `prev_hash`. A quota or rate limit returns 429 with `Retry-After`.\n")
	return []byte(b.String())
}
