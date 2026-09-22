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
		return err
	}
	for i := range kept {
		lit, err := search.VectorLiteral(vecs[i])
		if err != nil {
			return err
		}
		if err := st.SetEmbedding(ctx, kept[i].NoteID, kept[i].Version, lit); err != nil {
			return err
		}
	}
	return nil
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
