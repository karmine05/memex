//go:build integration

package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"memex/internal/store"
)

// fakeEmbedder rejects any text containing the poison marker, so one row
// can fail while its siblings succeed; down rejects everything to model
// an embedder outage.
type fakeEmbedder struct{ down bool }

func (f fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if f.down {
		return nil, errors.New("embedder down")
	}
	out := make([][]float32, len(texts))
	for i, tx := range texts {
		if strings.Contains(tx, "poison") {
			return nil, errors.New("context length exceeded")
		}
		v := make([]float32, 768)
		v[0] = 1
		out[i] = v
	}
	return out, nil
}

func (fakeEmbedder) Healthy(context.Context) error { return nil }

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("MEMEX_TEST_DSN")
	if dsn == "" {
		t.Fatal("MEMEX_TEST_DSN is required")
	}
	ctx := context.Background()
	if err := store.ResetDatabase(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `
		INSERT INTO agents (agent_id, name, key_hash)
		VALUES ('11111111-1111-4111-8111-111111111111', 'worker-test', 'seed-key-hash')`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `INSERT INTO spaces (space_id, acl_json) VALUES ('worker-test', '{}')`); err != nil {
		t.Fatal(err)
	}
	return st
}

func seedVersion(t *testing.T, st *store.Store, noteID, body string) {
	t.Helper()
	_, err := st.Pool.Exec(context.Background(), `
		INSERT INTO note_versions (note_id, version, agent_id, space_id, body, body_hash)
		VALUES ($1::uuid, 1, '11111111-1111-4111-8111-111111111111', 'worker-test', $2, 'seed-hash')`, noteID, body)
	if err != nil {
		t.Fatal(err)
	}
}

func embedState(t *testing.T, st *store.Store, noteID string) (embedded bool, attempts int) {
	t.Helper()
	err := st.Pool.QueryRow(context.Background(),
		`SELECT embedding IS NOT NULL, embed_attempts FROM note_versions WHERE note_id=$1::uuid`, noteID).
		Scan(&embedded, &attempts)
	if err != nil {
		t.Fatal(err)
	}
	return
}

// TestEmbedBatchIsolatesPoisonRow is the L7 check: one persistently
// failing row in a batch must not hold the other rows, and a failed row
// burns an attempt only when siblings proved the embedder was up.
func TestEmbedBatchIsolatesPoisonRow(t *testing.T) {
	st := openTestStore(t)
	const (
		good1 = "22222222-2222-4222-8222-222222222222"
		bad   = "33333333-3333-4333-8333-333333333333"
		good2 = "44444444-4444-4444-8444-444444444444"
	)
	seedVersion(t, st, good1, "first healthy body")
	seedVersion(t, st, bad, "poison body over the context window")
	seedVersion(t, st, good2, "second healthy body")

	ctx := context.Background()
	batch, err := st.PendingEmbeddings(ctx, 16)
	if err != nil || len(batch) != 3 {
		t.Fatalf("pending %d (err %v), want 3", len(batch), err)
	}
	if err := embedBatch(ctx, st, fakeEmbedder{}, &embedLoop{}, batch); err != nil {
		t.Fatalf("batch with one poison row must not error: %v", err)
	}
	if e, a := embedState(t, st, good1); !e || a != 0 {
		t.Fatalf("good1 embedded=%v attempts=%d", e, a)
	}
	if e, a := embedState(t, st, bad); e || a != 1 {
		t.Fatalf("bad embedded=%v attempts=%d, want 0/1", e, a)
	}
	if e, a := embedState(t, st, good2); !e || a != 0 {
		t.Fatalf("good2 embedded=%v attempts=%d", e, a)
	}

	// Second pass: only the poison row is pending. A lone failure without
	// a sibling success is indistinguishable from an outage, so it must
	// surface the error and burn no further attempts.
	again, err := st.PendingEmbeddings(ctx, 16)
	if err != nil || len(again) != 1 || again[0].NoteID != bad {
		t.Fatalf("second pending = %d rows (err %v), want the one bad row", len(again), err)
	}
	if err := embedBatch(ctx, st, fakeEmbedder{}, &embedLoop{}, again); err == nil {
		t.Fatal("lone failing row must return the embedder error")
	}
	if e, a := embedState(t, st, bad); e || a != 1 {
		t.Fatalf("bad embedded=%v attempts=%d after lone failure, want 0/1", e, a)
	}
}

// TestEmbedBatchOutageRecordsNoAttempts: a full embedder outage must not
// dead-letter healthy rows — the operator's Ollama restart cannot cost
// the backlog three cycles of attempts.
func TestEmbedBatchOutageRecordsNoAttempts(t *testing.T) {
	st := openTestStore(t)
	const (
		a = "55555555-5555-4555-8555-555555555555"
		b = "66666666-6666-4666-8666-666666666666"
	)
	seedVersion(t, st, a, "healthy one")
	seedVersion(t, st, b, "healthy two")

	ctx := context.Background()
	batch, err := st.PendingEmbeddings(ctx, 16)
	if err != nil || len(batch) != 2 {
		t.Fatalf("pending %d (err %v), want 2", len(batch), err)
	}
	if err := embedBatch(ctx, st, fakeEmbedder{down: true}, &embedLoop{}, batch); err == nil {
		t.Fatal("outage must surface as an error so the loop backs off")
	}
	for _, id := range []string{a, b} {
		if e, at := embedState(t, st, id); e || at != 0 {
			t.Fatalf("%s embedded=%v attempts=%d after outage, want 0/0", id, e, at)
		}
	}
	if rows, _ := st.PendingEmbeddings(ctx, 16); len(rows) != 2 {
		t.Fatalf("backlog shrank after outage: %d rows", len(rows))
	}
}
