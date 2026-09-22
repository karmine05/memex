package feed

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

type Event struct {
	ID        int64  `json:"-"`
	NoteID    string `json:"note_id"`
	Version   int64  `json:"version"`
	BodyHash  string `json:"body_hash"`
	AgentID   string `json:"agent_id"`
	Space     string `json:"space"`
	CreatedAt string `json:"created_at"`
}

type Hub struct {
	mu    sync.Mutex
	subs  map[string]map[chan Event]struct{}
	ready chan struct{}
	once  sync.Once
}

func New() *Hub {
	return &Hub{
		subs:  map[string]map[chan Event]struct{}{},
		ready: make(chan struct{}),
	}
}

func (h *Hub) Ready() <-chan struct{} { return h.ready }

func (h *Hub) Subscribe(key string) (<-chan Event, func()) {
	ch := make(chan Event, 256)
	h.mu.Lock()
	if h.subs[key] == nil {
		h.subs[key] = map[chan Event]struct{}{}
	}
	h.subs[key][ch] = struct{}{}
	h.mu.Unlock()
	cancel := func() {
		h.mu.Lock()
		delete(h.subs[key], ch)
		h.mu.Unlock()
	}
	return ch, cancel
}

func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, m := range h.subs {
		n += len(m)
	}
	return n
}

func (h *Hub) Publish(ev Event) {
	h.send("space:"+ev.Space, ev)
	if a, b, ok := dmPair(ev.Space); ok {
		h.send("inbox:"+a, ev)
		if a != b {
			h.send("inbox:"+b, ev)
		}
	}
}

func (h *Hub) send(key string, ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[key] {
		select {
		case ch <- ev:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- ev:
			default:
			}
		}
	}
}

func dmPair(space string) (string, string, bool) {
	rest, ok := strings.CutPrefix(space, "dm/")
	if !ok {
		return "", "", false
	}
	a, b, ok := strings.Cut(rest, "/")
	if !ok || a == "" || b == "" || strings.Contains(b, "/") {
		return "", "", false
	}
	return a, b, true
}

type notifyPayload struct {
	ID        int64  `json:"id"`
	SpaceID   string `json:"space_id"`
	NoteID    string `json:"note_id"`
	Version   int64  `json:"version"`
	BodyHash  string `json:"body_hash"`
	AgentID   string `json:"agent_id"`
	CreatedAt string `json:"created_at"`
}

func (h *Hub) Serve(ctx context.Context, dsn string) {
	for {
		if ctx.Err() != nil {
			return
		}
		err := h.listenOnce(ctx, dsn)
		if ctx.Err() != nil {
			return
		}
		slog.Error("stream listen", "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func (h *Hub) listenOnce(ctx context.Context, dsn string) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `LISTEN memex_stream`); err != nil {
		return err
	}
	h.once.Do(func() { close(h.ready) })
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		var p notifyPayload
		if err := json.Unmarshal([]byte(n.Payload), &p); err != nil {
			slog.Error("stream payload", "err", err)
			continue
		}
		h.Publish(Event{
			ID: p.ID, NoteID: p.NoteID, Version: p.Version, BodyHash: p.BodyHash,
			AgentID: p.AgentID, Space: p.SpaceID, CreatedAt: p.CreatedAt,
		})
	}
}
