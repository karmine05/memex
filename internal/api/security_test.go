//go:build integration

package api

import (
	"context"
	"strings"
	"testing"

	"memex/internal/store"
)

// TestMemexCanReadPolicy pins the SQL-side ACL policy to the Go policy
// (note.Can, unit-tested in internal/note) for every shape the admin
// endpoint accepts, plus the malformed shapes the 0001 function treated
// as open. Migration 0002 rewrites the function; these rows fail on the
// old one.
func TestMemexCanReadPolicy(t *testing.T) {
	_, _, _, st, _ := newHarness(t)
	ctx := context.Background()
	can := func(acl any, agent string) bool {
		var out bool
		if err := st.Pool.QueryRow(ctx, `SELECT memex_can_read($1::jsonb, $2)`, acl, agent).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	cases := []struct {
		name  string
		acl   any // nil is JSON null
		agent string
		want  bool
	}{
		{"null acl is open", nil, "anyone", true},
		{"empty object is open", `{}`, "anyone", true},
		{"read list denies stranger", `{"read":["a","b"],"write":null}`, "c", false},
		{"read list allows listed reader", `{"read":["a","b"],"write":null}`, "a", true},
		{"read list with write null, writer listed", `{"read":["a"],"write":null}`, "a", true},
		{"malformed read value fails closed", `{"read":"x"}`, "c", false},
		{"malformed write value fails closed", `{"read":["a"],"write":5}`, "a", false},
		{"both malformed fails closed", `{"read":1,"write":{}}`, "a", false},
		{"empty read list is open", `{"read":[]}`, "c", true},
		{"star read list is open", `{"read":["*"]}`, "c", true},
		{"explicit writer can read", `{"read":["a"],"write":["b"]}`, "b", true},
		{"stranger denied despite non-empty write list", `{"read":["a"],"write":["b"]}`, "c", false},
		{"empty write list is not a read grant", `{"read":["a"],"write":[]}`, "c", false},
		{"star write list grants read", `{"read":["a"],"write":["*"]}`, "c", true},
	}
	for _, tc := range cases {
		if got := can(tc.acl, tc.agent); got != tc.want {
			t.Fatalf("%s: memex_can_read(%v, %q) = %v, want %v", tc.name, tc.acl, tc.agent, got, tc.want)
		}
	}
}

// TestInboxExcludesExtraSegments pins the L1 fix: inbox predicates match
// exactly the two-segment dm/<a>/<b> shape. A 3+ segment "dm/..." space
// is an ordinary open space an attacker can create, and must not be able
// to inject into a victim's inbox.
func TestInboxExcludesExtraSegments(t *testing.T) {
	_, agent, admin, _, adminKey := newHarness(t)

	mkAgent := func(name string) (agentID, apiKey string) {
		res := post(t, admin.URL+"/admin/agents", adminKey, map[string]any{"name": name})
		if res.StatusCode != 201 {
			t.Fatalf("%s %d %s", name, res.StatusCode, res.Body)
		}
		var k struct {
			APIKey  string `json:"api_key"`
			AgentID string `json:"agent_id"`
		}
		mustJSON(t, res.Body, &k)
		return k.AgentID, k.APIKey
	}
	victimID, victimKey := mkAgent("victim")
	attackerID, attackerKey := mkAgent("attacker")

	token := func(apiKey string) string {
		res := post(t, agent.URL+"/v1/auth/token", apiKey, nil)
		if res.StatusCode != 200 {
			t.Fatalf("token %d %s", res.StatusCode, res.Body)
		}
		var out struct {
			Token string `json:"token"`
		}
		mustJSON(t, res.Body, &out)
		return out.Token
	}
	victimTok, attackerTok := token(victimKey), token(attackerKey)

	// A 3-segment dm/... space is not a DM (creation refuses only the
	// two-segment form), so it is creatable as an ordinary open space —
	// and the old inbox predicate would have matched it.
	space := "dm/" + victimID + "/" + attackerID + "/inject"
	evil := post(t, agent.URL+"/v1/notes", attackerTok, map[string]any{"space": space, "body": map[string]any{"topic": "sneaky", "status": "note", "answer": "inbox-injection-payload"}})
	if evil.StatusCode != 201 {
		t.Fatalf("3-segment dm space %d %s", evil.StatusCode, evil.Body)
	}

	// A genuine DM from attacker to victim must still reach the inbox.
	dm := post(t, agent.URL+"/v1/agents/"+victimID+"/dm", attackerTok, map[string]any{"body": map[string]any{"status": "note", "answer": "legit-dm-payload"}})
	if dm.StatusCode != 201 {
		t.Fatalf("dm %d %s", dm.StatusCode, dm.Body)
	}

	inbox := get(t, agent.URL+"/v1/agents/me/inbox", victimTok, "")
	if inbox.StatusCode != 200 {
		t.Fatalf("inbox %d %s", inbox.StatusCode, inbox.Body)
	}
	if !strings.Contains(inbox.Body, "legit-dm-payload") {
		t.Fatalf("legit DM missing from inbox: %s", inbox.Body)
	}
	if strings.Contains(inbox.Body, "inbox-injection-payload") {
		t.Fatal("3-segment dm/... space leaked into inbox")
	}
}

// TestUpdateConflictDoesNotLeakToExcludedAgent pins the L5 ordering fix:
// a stale-base_hash PUT by an agent the space ACL excludes must answer
// 403, not 409 — the 409 body carries the current body_hash.
func TestUpdateConflictDoesNotLeakToExcludedAgent(t *testing.T) {
	_, agent, admin, _, adminKey := newHarness(t)

	alpha := post(t, admin.URL+"/admin/agents", adminKey, map[string]any{"name": "alpha"})
	beta := post(t, admin.URL+"/admin/agents", adminKey, map[string]any{"name": "beta"})
	var kA, kB struct {
		APIKey  string `json:"api_key"`
		AgentID string `json:"agent_id"`
	}
	mustJSON(t, alpha.Body, &kA)
	mustJSON(t, beta.Body, &kB)
	tok := func(apiKey string) string {
		res := post(t, agent.URL+"/v1/auth/token", apiKey, nil)
		var out struct {
			Token string `json:"token"`
		}
		mustJSON(t, res.Body, &out)
		return out.Token
	}
	alphaTok, betaTok := tok(kA.APIKey), tok(kB.APIKey)

	noteRes := post(t, agent.URL+"/v1/notes", alphaTok, map[string]any{"space": "vault", "body": map[string]any{"status": "note", "answer": "restricted content"}})
	if noteRes.StatusCode != 201 {
		t.Fatalf("note %d %s", noteRes.StatusCode, noteRes.Body)
	}
	var wrote noteBody
	mustJSON(t, noteRes.Body, &wrote)

	acl := post(t, admin.URL+"/admin/spaces/vault/acl", adminKey, map[string]any{"read": []string{kA.AgentID}, "write": []string{kA.AgentID}})
	if acl.StatusCode != 200 {
		t.Fatalf("acl %d %s", acl.StatusCode, acl.Body)
	}

	// Excluded agent with a stale base hash: 403, and no hash in the body.
	denied := put(t, agent.URL+"/v1/notes/"+wrote.NoteID, betaTok, map[string]any{"body": map[string]any{"answer": "pried open"}, "base_hash": "deadbeef"})
	if denied.StatusCode != 403 {
		t.Fatalf("excluded update = %d, want 403: %s", denied.StatusCode, denied.Body)
	}
	if strings.Contains(denied.Body, wrote.BodyHash) {
		t.Fatalf("403 body leaked current hash: %s", denied.Body)
	}
	// The writer still gets the real 409 with the conflict hash.
	conflict := put(t, agent.URL+"/v1/notes/"+wrote.NoteID, alphaTok, map[string]any{"body": map[string]any{"answer": "mine"}, "base_hash": "deadbeef"})
	if conflict.StatusCode != 409 || !strings.Contains(conflict.Body, wrote.BodyHash) {
		t.Fatalf("writer conflict = %d: %s", conflict.StatusCode, conflict.Body)
	}
}

// TestEmbedDeadLetterAndReset pins the L7 store side: a persistently
// failing row is skipped after MaxEmbedAttempts, the pending count
// excludes dead rows, and re-embed resets the counter.
func TestEmbedDeadLetterAndReset(t *testing.T) {
	_, agent, admin, st, adminKey := newHarness(t)
	ctx := context.Background()

	res := post(t, admin.URL+"/admin/agents", adminKey, map[string]any{"name": "alpha"})
	var k struct {
		APIKey string `json:"api_key"`
	}
	mustJSON(t, res.Body, &k)
	tokRes := post(t, agent.URL+"/v1/auth/token", k.APIKey, nil)
	var tok struct {
		Token string `json:"token"`
	}
	mustJSON(t, tokRes.Body, &tok)
	noteRes := post(t, agent.URL+"/v1/notes", tok.Token, map[string]any{"space": "lab", "body": map[string]any{"status": "note", "answer": "needs embedding"}})
	if noteRes.StatusCode != 201 {
		t.Fatalf("note %d %s", noteRes.StatusCode, noteRes.Body)
	}
	var wrote noteBody
	mustJSON(t, noteRes.Body, &wrote)

	pending := func() []store.Version {
		rows, err := st.PendingEmbeddings(ctx, 10)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	has := func(rows []store.Version, id string) bool {
		for _, r := range rows {
			if r.NoteID == id {
				return true
			}
		}
		return false
	}

	if !has(pending(), wrote.NoteID) {
		t.Fatal("fresh row not pending")
	}
	for i := 0; i < store.MaxEmbedAttempts; i++ {
		if err := st.RecordEmbedAttempt(ctx, wrote.NoteID, 1); err != nil {
			t.Fatal(err)
		}
	}
	if has(pending(), wrote.NoteID) {
		t.Fatal("row still pending at the attempt cap")
	}
	if _, p, err := st.Counts(ctx); err != nil || p != 0 {
		t.Fatalf("pending count after dead-letter = %d (err %v), want 0", p, err)
	}
	if n, err := st.ClearEmbeddings(ctx, nil); err != nil || n == 0 {
		t.Fatalf("re-embed cleared %d rows (err %v)", n, err)
	}
	if !has(pending(), wrote.NoteID) {
		t.Fatal("row not pending again after re-embed reset")
	}
}
