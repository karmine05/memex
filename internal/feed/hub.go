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

// sub is one live SSE subscription, bound to the agent that opened it so
// revocation, suspension, rotation, and space ACL changes can tear it down.
type sub struct {
	ch    chan Event
	agent string
	done  chan struct{}
	once  sync.Once
}

func (s *sub) stop() { s.once.Do(func() { close(s.done) }) }

type Hub struct {
	mu    sync.Mutex
	subs  map[string]map[*sub]struct{}
	ready chan struct{}
	once  sync.Once
}

func New() *Hub {
	return &Hub{
		subs:  map[string]map[*sub]struct{}{},
		ready: make(chan struct{}),
	}
}

func (h *Hub) Ready() <-chan struct{} { return h.ready }

// Subscribe registers under key for the owning agent. The returned done
// channel is closed when the subscription is cancelled by any path
// (client disconnect, agent lifecycle, or space ACL change).
func (h *Hub) Subscribe(key, agent string) (<-chan Event, <-chan struct{}, func()) {
	s := &sub{ch: make(chan Event, 256), agent: agent, done: make(chan struct{})}
	h.mu.Lock()
	if h.subs[key] == nil {
		h.subs[key] = map[*sub]struct{}{}
	}
	h.subs[key][s] = struct{}{}
	h.mu.Unlock()
	cancel := func() {
		h.mu.Lock()
		if m := h.subs[key]; m != nil {
			delete(m, s)
		}
		h.mu.Unlock()
		s.stop()
	}
	return s.ch, s.done, cancel
}

// CancelAgent tears down every stream owned by the agent. Called on
// revocation, suspension, and key rotation: an identity that no longer
// exists must not keep receiving change events on old connections.
func (h *Hub) CancelAgent(agent string) {
	h.mu.Lock()
	var dead []*sub
	for key, m := range h.subs {
		for s := range m {
			if s.agent == agent {
				delete(h.subs[key], s)
				dead = append(dead, s)
			}
		}
	}
	h.mu.Unlock()
	for _, s := range dead {
		s.stop()
	}
}

// CancelSpace stops every space stream on the given space, used after an
// ACL replace: the old grant no longer describes who may see the feed.
func (h *Hub) CancelSpace(space string) {
	key := "space:" + space
	h.mu.Lock()
	dead := h.subs[key]
	delete(h.subs, key)
	h.mu.Unlock()
	for s := range dead {
		s.stop()
	}
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
	for s := range h.subs[key] {
		ch := s.ch
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
