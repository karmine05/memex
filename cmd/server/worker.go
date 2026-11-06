package main

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"memex/internal/search"
	"memex/internal/store"
)

type embedLoop struct {
	mu   sync.Mutex
	next time.Time
	busy sync.Map
}

func (e *embedLoop) ready() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return !time.Now().Before(e.next)
}

func (e *embedLoop) pause(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	t := time.Now().Add(d)
	if t.After(e.next) {
		e.next = t
	}
}

func runEmbeddings(ctx context.Context, st *store.Store, emb search.Embedder) {
	loop := &embedLoop{}
	sem := make(chan struct{}, 4)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if !loop.ready() {
				continue
			}
			rows, err := st.PendingEmbeddings(ctx, 16)
			if err != nil {
				slog.Error("pending embeds", "err", err)
				loop.pause(5 * time.Second)
				continue
			}
			if len(rows) == 0 {
				continue
			}
			select {
			case sem <- struct{}{}:
				go func(batch []store.Version) {
					defer func() { <-sem }()
					if err := embedBatch(ctx, st, emb, loop, batch); err != nil {
						slog.Error("embed batch", "err", err)
						loop.pause(5 * time.Second)
					}
				}(append([]store.Version(nil), rows...))
			default:
			}
		}
	}
}

func embedBatch(ctx context.Context, st *store.Store, emb search.Embedder, loop *embedLoop, batch []store.Version) error {
	texts := make([]string, 0, len(batch))
	kept := make([]store.Version, 0, len(batch))
	keys := make([]string, 0, len(batch))
	for _, v := range batch {
		key := v.NoteID + ":" + strconv.FormatInt(v.Version, 10)
		if _, loaded := loop.busy.LoadOrStore(key, true); loaded {
			continue
		}
		kept = append(kept, v)
		keys = append(keys, key)
		texts = append(texts, search.EmbedInput(v.Body))
	}
	defer func() {
		for _, key := range keys {
			loop.busy.Delete(key)
		}
	}()
	if len(kept) == 0 {
		return nil
	}
	vecs, err := emb.Embed(ctx, texts)
	if err != nil {
		return fallbackPerRow(ctx, st, emb, kept, texts, err)
	}
	var lastErr error
	failed := 0
	for i := range kept {
		if err := persistEmbedding(ctx, st, kept[i], vecs[i]); err != nil {
			failed++
			lastErr = err
			slog.Error("persist embed", "note", kept[i].NoteID, "version", kept[i].Version, "err", err)
		}
	}
	// Every persist failed: the DB, not the embedder, is the problem.
	// Surface it so the loop backs off instead of spinning at tick rate.
	if failed == len(kept) {
		return lastErr
	}
	return nil
}

// fallbackPerRow retries a rejected batch one row at a time, so a single
// poison row (over-context body, model error) cannot hold the other
// fifteen. A failed row only burns an attempt when sibling rows
// succeeded in the same pass — that proves the embedder was up and the
// failure is about that row's content, which is what the
// MaxEmbedAttempts dead-letter is for. On a full outage nothing is
// recorded, the backlog stays intact, and the batch error makes the loop
// back off.
func fallbackPerRow(ctx context.Context, st *store.Store, emb search.Embedder, kept []store.Version, texts []string, batchErr error) error {
	sawSuccess := false
	var failed []store.Version
	for i := range kept {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		vecs, err := emb.Embed(ctx, texts[i:i+1])
		if err != nil {
			failed = append(failed, kept[i])
			continue
		}
		sawSuccess = true
		if err := persistEmbedding(ctx, st, kept[i], vecs[0]); err != nil {
			slog.Error("persist embed", "note", kept[i].NoteID, "version", kept[i].Version, "err", err)
		}
	}
	if sawSuccess {
		for _, v := range failed {
			if err := st.RecordEmbedAttempt(ctx, v.NoteID, v.Version); err != nil {
				slog.Warn("record embed attempt", "err", err)
			}
		}
		return nil
	}
	return batchErr
}

func persistEmbedding(ctx context.Context, st *store.Store, v store.Version, vec []float32) error {
	lit, err := search.VectorLiteral(vec)
	if err != nil {
		return err
	}
	return st.SetEmbedding(ctx, v.NoteID, v.Version, lit)
}

func maintain(ctx context.Context, st *store.Store) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := st.DeleteExpiredTokens(ctx); err != nil {
				slog.Error("expire tokens", "err", err)
			}
			if err := st.TrimStreams(ctx); err != nil {
				slog.Error("trim streams", "err", err)
			}
		}
	}
}
