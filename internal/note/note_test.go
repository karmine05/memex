package note

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestReorderedJSONHashesEqual(t *testing.T) {
	left := json.RawMessage(`{"b":1,"a":2,"nested":{"z":true,"y":false}}`)
	right := json.RawMessage("{\n  \"nested\": {\"y\": false, \"z\": true},\n  \"a\": 2,\n  \"b\": 1\n}")
	lb, lh, err := Normalize(left)
	if err != nil {
		t.Fatal(err)
	}
	rb, rh, err := Normalize(right)
	if err != nil {
		t.Fatal(err)
	}
	if lh != rh {
		t.Fatalf("hash %s != %s\n%s\n%s", lh, rh, lb, rb)
	}
	if lb != rb {
		t.Fatalf("canonical bytes differ\n%s\n%s", lb, rb)
	}
	again, err := Hash(lb)
	if err != nil {
		t.Fatal(err)
	}
	if again != lh {
		t.Fatal("canonicalize is not idempotent")
	}
}

func TestPlainTextIsNotJSON(t *testing.T) {
	body, hash, err := Normalize(json.RawMessage(`"not an object"`))
	if err != nil {
		t.Fatal(err)
	}
	if body != "not an object" {
		t.Fatal(body)
	}
	if hash == "" {
		t.Fatal("empty hash")
	}
}

func TestJCSNumbers(t *testing.T) {
	raw := json.RawMessage(`{"numbers":[333333333.33333329,1E30,4.50,2e-3,0.000000000000000000000000001]}`)
	got, _, err := Normalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"numbers":[333333333.3333333,1e+30,4.5,0.002,1e-27]}`
	if got != want {
		t.Fatalf("got %s", got)
	}
}

func TestUpdateChainDetectsTamper(t *testing.T) {
	b1, h1, err := Normalize(json.RawMessage(`{"status":"question","answer":"why"}`))
	if err != nil {
		t.Fatal(err)
	}
	b2, h2, err := Normalize(json.RawMessage(`{"answer":"because","status":"answer"}`))
	if err != nil {
		t.Fatal(err)
	}
	chain := []ChainItem{
		{Version: 1, Body: b1, BodyHash: h1, PrevHash: ""},
		{Version: 2, Body: b2, BodyHash: h2, PrevHash: h1},
	}
	if err := Verify(chain); err != nil {
		t.Fatal(err)
	}
	chain[1].Body = strings.ReplaceAll(b2, "because", "tampered")
	if err := Verify(chain); err == nil {
		t.Fatal("tamper was accepted")
	}
}

func TestUnifiedDiffShowsChangedLine(t *testing.T) {
	got := Unified("alpha\nbeta\n", "alpha\ngamma\n")
	if !strings.Contains(got, "\n beta\n") || !strings.Contains(got, "\n-beta\n") && !strings.Contains(got, "\n-beta\n") {
		if !strings.Contains(got, "-beta\n") || !strings.Contains(got, "+gamma\n") || !strings.Contains(got, " alpha\n") {
			t.Fatalf("diff:\n%s", got)
		}
	}
	if Unified("same", "same") != "" {
		t.Fatal("equal inputs should be empty")
	}
}

func TestDMSpaceIsSorted(t *testing.T) {
	if DMSpace("bbb", "aaa") != "dm/aaa/bbb" {
		t.Fatal(DMSpace("bbb", "aaa"))
	}
	if DMSpace("bbb", "aaa") != DMSpace("aaa", "bbb") {
		t.Fatal("order")
	}
	a, b, ok := DMParticipants("dm/aaa/bbb")
	if !ok || a != "aaa" || b != "bbb" {
		t.Fatal(a, b, ok)
	}
}

func TestACL(t *testing.T) {
	if !Can([]byte(`{}`), "a", true) {
		t.Fatal("open write")
	}
	raw := DMACL("a", "b")
	if Can(raw, "c", false) {
		t.Fatal("stranger read")
	}
	if !Can(raw, "b", true) {
		t.Fatal("participant write")
	}
	locked, err := LockACL([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if Can(locked, "a", true) {
		t.Fatal("locked write")
	}
	if !Can(locked, "a", false) {
		t.Fatal("locked read")
	}
}

func TestNewIDLooksLikeUUID(t *testing.T) {
	id, err := NewID(time.UnixMilli(1_700_000_000_000))
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 36 || id[14] != '7' {
		t.Fatal(id)
	}
}

func TestSpaceAndName(t *testing.T) {
	if !ValidSpace("ops/fixes") || ValidSpace("../etc") || ValidSpace("/ops") || !ValidSpace("team/a/b") {
		t.Fatal("space")
	}
	if !ValidName("ops-bot") || ValidName("") || ValidName("has space") {
		t.Fatal("name")
	}
}
