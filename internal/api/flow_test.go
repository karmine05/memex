//go:build integration

package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"memex/internal/auth"
	"memex/internal/config"
	"memex/internal/feed"
	"memex/internal/search"
	"memex/internal/store"
)

func TestFlow(t *testing.T) {
	dsn := os.Getenv("MEMEX_TEST_DSN")
	if dsn == "" {
		t.Fatal("MEMEX_TEST_DSN is required")
	}
	ctx := context.Background()
	resetDB(t, dsn)
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	hub := feed.New()
	go hub.Serve(ctx, dsn)
	select {
	case <-hub.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("listen did not become ready")
	}
	adminKey, err := auth.NewAdminKey()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Registration = "bootstrap"
	cfg.Embed.Provider = "none"
	srv := &Server{
		Cfg: cfg, Store: st, Hub: hub, Embed: search.None{},
		AdminHash: auth.Hash(adminKey), Limit: NewLimiter(),
		AuditPath: t.TempDir() + "/audit.log",
	}
	agent := httptest.NewServer(srv.Handler("agent"))
	t.Cleanup(agent.Close)
	admin := httptest.NewServer(srv.Handler("admin"))
	t.Cleanup(admin.Close)

	var logs bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))

	reg := post(t, agent.URL+"/v1/agents/register", "", map[string]any{"name": "nope"})
	if reg.StatusCode != 403 {
		t.Fatalf("bootstrap gate %d", reg.StatusCode)
	}

	created := post(t, admin.URL+"/admin/agents", adminKey, map[string]any{"name": "alpha", "description": "first"})
	if created.StatusCode != 201 {
		t.Fatalf("create %d %s", created.StatusCode, created.Body)
	}
	var key struct {
		APIKey  string `json:"api_key"`
		AgentID string `json:"agent_id"`
	}
	mustJSON(t, created.Body, &key)
	if !strings.HasPrefix(key.APIKey, "mxk_") {
		t.Fatal(key.APIKey)
	}

	// The one-prompt install depends on the protocol being keyless on the
	// admin port (webUI/CLI fetch it) and absent from the agent port.
	skill := get(t, admin.URL+"/skill.md", "", "")
	if skill.StatusCode != 200 || skill.Header.Get("Content-Type") != "text/markdown; charset=utf-8" ||
		!strings.Contains(skill.Body, "Agent Protocol") {
		t.Fatalf("skill.md %d %q", skill.StatusCode, skill.Body[:min(120, len(skill.Body))])
	}
	agentSkill := get(t, agent.URL+"/skill.md", "", "")
	if agentSkill.StatusCode != http.StatusNotFound {
		t.Fatalf("agent skill.md %d", agentSkill.StatusCode)
	}

	var stored string
	if err := st.Pool.QueryRow(ctx, `SELECT key_hash FROM agents WHERE agent_id=$1::uuid`, key.AgentID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == key.APIKey || stored != auth.Hash(key.APIKey) {
		t.Fatal("key was not stored as a hash")
	}

	tokRes := post(t, agent.URL+"/v1/auth/token", key.APIKey, nil)
	if tokRes.StatusCode != 200 {
		t.Fatalf("token %d %s", tokRes.StatusCode, tokRes.Body)
	}
	var tok struct {
		Token string `json:"token"`
	}
	mustJSON(t, tokRes.Body, &tok)

	body := map[string]any{"space": "lab", "body": map[string]any{"topic": "lab/unique", "status": "answer", "answer": "zzqxmemextoken fixes the pod"}}
	noteRes := post(t, agent.URL+"/v1/notes", tok.Token, body)
	if noteRes.StatusCode != 201 {
		t.Fatalf("note %d %s", noteRes.StatusCode, noteRes.Body)
	}
	var wrote noteBody
	mustJSON(t, noteRes.Body, &wrote)

	bad := put(t, agent.URL+"/v1/notes/"+wrote.NoteID, tok.Token, map[string]any{"body": map[string]any{"answer": "other"}, "base_hash": "deadbeef"})
	if bad.StatusCode != 409 {
		t.Fatalf("conflict %d %s", bad.StatusCode, bad.Body)
	}
	var conflict conflictBody
	mustJSON(t, bad.Body, &conflict)
	if conflict.BodyHash != wrote.BodyHash {
		t.Fatalf("conflict hash %s", conflict.BodyHash)
	}
	good := put(t, agent.URL+"/v1/notes/"+wrote.NoteID, tok.Token, map[string]any{
		"body": map[string]any{"answer": "zzqxmemextoken still fixes the pod", "status": "answer"}, "base_hash": conflict.BodyHash,
	})
	if good.StatusCode != 200 {
		t.Fatalf("retry %d %s", good.StatusCode, good.Body)
	}
	var updated noteBody
	mustJSON(t, good.Body, &updated)

	got := get(t, agent.URL+"/v1/notes/"+wrote.NoteID, tok.Token, "")
	if got.StatusCode != 200 || got.Header.Get("ETag") != `"`+updated.BodyHash+`"` {
		t.Fatalf("get %d etag %s body %s", got.StatusCode, got.Header.Get("ETag"), got.Body)
	}
	cached := get(t, agent.URL+"/v1/notes/"+wrote.NoteID, tok.Token, updated.BodyHash)
	if cached.StatusCode != 304 {
		t.Fatalf("etag %d", cached.StatusCode)
	}

	found := post(t, agent.URL+"/v1/search", tok.Token, map[string]any{"query": "zzqxmemextoken", "limit": 5})
	if found.StatusCode != 200 {
		t.Fatalf("search %d %s", found.StatusCode, found.Body)
	}
	var results struct {
		Results []noteBody `json:"results"`
	}
	mustJSON(t, found.Body, &results)
	if len(results.Results) == 0 || results.Results[0].NoteID != wrote.NoteID {
		t.Fatalf("search results %+v", results.Results)
	}
	var readers int
	if err := st.Pool.QueryRow(ctx, `SELECT count(DISTINCT agent_id) FROM note_reads WHERE note_id=$1::uuid`, wrote.NoteID).Scan(&readers); err != nil {
		t.Fatal(err)
	}
	if readers != 1 {
		t.Fatalf("readers %d", readers)
	}

	beta := post(t, admin.URL+"/admin/agents", adminKey, map[string]any{"name": "beta", "description": "second"})
	if beta.StatusCode != 201 {
		t.Fatalf("beta %d %s", beta.StatusCode, beta.Body)
	}
	var key2 struct {
		APIKey  string `json:"api_key"`
		AgentID string `json:"agent_id"`
	}
	mustJSON(t, beta.Body, &key2)
	tok2 := post(t, agent.URL+"/v1/auth/token", key2.APIKey, nil)
	var tokB struct {
		Token string `json:"token"`
	}
	mustJSON(t, tok2.Body, &tokB)
	dm := post(t, agent.URL+"/v1/agents/"+key.AgentID+"/dm", tokB.Token, map[string]any{"body": map[string]any{"status": "note", "answer": "ping from beta"}})
	if dm.StatusCode != 201 {
		t.Fatalf("dm %d %s", dm.StatusCode, dm.Body)
	}
	inbox := get(t, agent.URL+"/v1/agents/me/inbox", tok.Token, "")
	if inbox.StatusCode != 200 || !strings.Contains(inbox.Body, "ping from beta") {
		t.Fatalf("inbox %d %s", inbox.StatusCode, inbox.Body)
	}
	stranger := get(t, agent.URL+"/v1/notes/"+dmNoteID(t, dm.Body), tok.Token, "")
	// alpha is a participant, so this is 200. A third agent is checked below.
	if stranger.StatusCode != 200 {
		t.Fatalf("participant read %d", stranger.StatusCode)
	}

	gamma := post(t, admin.URL+"/admin/agents", adminKey, map[string]any{"name": "gamma", "description": "third"})
	var key3 struct {
		APIKey  string `json:"api_key"`
		AgentID string `json:"agent_id"`
	}
	mustJSON(t, gamma.Body, &key3)
	tok3res := post(t, agent.URL+"/v1/auth/token", key3.APIKey, nil)
	var tokC struct {
		Token string `json:"token"`
	}
	mustJSON(t, tok3res.Body, &tokC)
	denied := get(t, agent.URL+"/v1/notes/"+dmNoteID(t, dm.Body), tokC.Token, "")
	if denied.StatusCode != 403 {
		t.Fatalf("stranger dm read %d %s", denied.StatusCode, denied.Body)
	}

	// Key rotation: same identity, new secret; old key and live tokens die.
	rotated := post(t, admin.URL+"/admin/agents/"+key3.AgentID+"/rotate", adminKey, map[string]any{})
	if rotated.StatusCode != 200 {
		t.Fatalf("rotate %d %s", rotated.StatusCode, rotated.Body)
	}
	var rotatedKey struct {
		APIKey  string `json:"api_key"`
		AgentID string `json:"agent_id"`
	}
	mustJSON(t, rotated.Body, &rotatedKey)
	if rotatedKey.AgentID != key3.AgentID || rotatedKey.APIKey == key3.APIKey || !strings.HasPrefix(rotatedKey.APIKey, "mxk_") {
		t.Fatalf("rotate response %+v", rotatedKey)
	}
	if old := post(t, agent.URL+"/v1/auth/token", key3.APIKey, nil); old.StatusCode != 401 {
		t.Fatalf("old key still works after rotate: %d", old.StatusCode)
	}
	if live := get(t, agent.URL+"/v1/agents", tokC.Token, ""); live.StatusCode != 401 {
		t.Fatalf("old token still works after rotate: %d", live.StatusCode)
	}
	fresh := post(t, agent.URL+"/v1/auth/token", rotatedKey.APIKey, nil)
	if fresh.StatusCode != 200 {
		t.Fatalf("new key rejected %d %s", fresh.StatusCode, fresh.Body)
	}
	if missing := post(t, admin.URL+"/admin/agents/00000000-0000-7000-8000-000000000000/rotate", adminKey, map[string]any{}); missing.StatusCode != 404 {
		t.Fatalf("rotate missing agent %d", missing.StatusCode)
	}
	auditLine, err := os.ReadFile(srv.AuditPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(auditLine), `"rotate"`) || !strings.Contains(string(auditLine), key3.AgentID) {
		t.Fatalf("rotate not audited: %s", auditLine)
	}

	streamCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, agent.URL+"/v1/spaces/lab/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.Token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	reader := bufio.NewReader(res.Body)
	if err := waitFor(reader, "connected"); err != nil {
		t.Fatal(err)
	}
	extra := post(t, agent.URL+"/v1/notes", tok.Token, map[string]any{"space": "lab", "body": "stream-event-body"})
	if extra.StatusCode != 201 {
		t.Fatalf("stream note %d %s", extra.StatusCode, extra.Body)
	}
	var streamed noteBody
	mustJSON(t, extra.Body, &streamed)
	if err := waitFor(reader, streamed.NoteID); err != nil {
		t.Fatal(err)
	}

	audit := get(t, admin.URL+"/admin/notes/"+wrote.NoteID+"/audit", adminKey, "")
	if audit.StatusCode != 200 || !strings.Contains(audit.Body, `"ok":true`) {
		t.Fatalf("audit %d %s", audit.StatusCode, audit.Body)
	}
	if _, err := st.Pool.Exec(ctx, `UPDATE note_versions SET body = body || 'x' WHERE note_id=$1::uuid AND version=1`, wrote.NoteID); err != nil {
		t.Fatal(err)
	}
	broken := get(t, admin.URL+"/admin/notes/"+wrote.NoteID+"/audit", adminKey, "")
	if !strings.Contains(broken.Body, `"ok":false`) {
		t.Fatalf("tamper not detected %s", broken.Body)
	}

	// Revoking an agent removes it from operational views: no graph node or
	// edge may reference it, and recent activity drops its rows. beta has a
	// "sent" edge (its DM), an "used" edge from alpha's inbox read, and the
	// read below adds a beta→alpha "used" edge — all must disappear.
	if r := get(t, agent.URL+"/v1/notes/"+wrote.NoteID, tokB.Token, ""); r.StatusCode != 200 {
		t.Fatalf("beta read %d %s", r.StatusCode, r.Body)
	}
	if rev := post(t, admin.URL+"/admin/agents/"+key2.AgentID+"/revoke", adminKey, map[string]any{}); rev.StatusCode != 200 {
		t.Fatalf("revoke %d %s", rev.StatusCode, rev.Body)
	}
	gv := struct {
		Nodes []struct {
			ID string `json:"id"`
		} `json:"nodes"`
		Edges []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"edges"`
	}{}
	graphRes := get(t, admin.URL+"/admin/graph", adminKey, "")
	if graphRes.StatusCode != 200 {
		t.Fatalf("graph %d %s", graphRes.StatusCode, graphRes.Body)
	}
	mustJSON(t, graphRes.Body, &gv)
	for _, n := range gv.Nodes {
		if n.ID == key2.AgentID {
			t.Fatal("revoked agent still in graph nodes")
		}
	}
	for _, e := range gv.Edges {
		if e.From == key2.AgentID || e.To == key2.AgentID {
			t.Fatalf("revoked agent still referenced by edge %+v", e)
		}
	}
	telemetry := get(t, admin.URL+"/admin/telemetry", adminKey, "")
	if strings.Contains(telemetry.Body, `"beta"`) {
		t.Fatal("revoked agent still in telemetry activity")
	}

	if strings.Contains(logs.String(), key.APIKey) || strings.Contains(logs.String(), tok.Token) ||
		strings.Contains(logs.String(), rotatedKey.APIKey) {
		t.Fatal("secret appeared in logs")
	}
}

func resetDB(t *testing.T, dsn string) {
	t.Helper()
	if err := store.ResetDatabase(context.Background(), dsn); err != nil {
		t.Fatal(err)
	}
}

type response struct {
	StatusCode int
	Body       string
	Header     http.Header
}

func post(t *testing.T, url, token string, body any) response {
	t.Helper()
	return call(t, http.MethodPost, url, token, body)
}

func put(t *testing.T, url, token string, body any) response {
	t.Helper()
	return call(t, http.MethodPut, url, token, body)
}

func get(t *testing.T, url, token, etag string) response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", `"`+etag+`"`)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return response{StatusCode: res.StatusCode, Body: string(b), Header: res.Header}
}

func call(t *testing.T, method, url, token string, body any) response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return response{StatusCode: res.StatusCode, Body: string(b), Header: res.Header}
}

func mustJSON(t *testing.T, raw string, dst any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), dst); err != nil {
		t.Fatalf("json %s: %v", raw, err)
	}
}

func dmNoteID(t *testing.T, raw string) string {
	t.Helper()
	var n noteBody
	mustJSON(t, raw, &n)
	return n.NoteID
}

func waitFor(r *bufio.Reader, needle string) error {
	deadline := time.Now().Add(4 * time.Second)
	var buf bytes.Buffer
	for time.Now().Before(deadline) {
		line, err := r.ReadString('\n')
		buf.WriteString(line)
		if strings.Contains(buf.String(), needle) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return io.EOF
}
