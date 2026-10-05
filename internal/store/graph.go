package store

import (
	"context"
	"fmt"
	"sort"
	"time"
)

type GraphNode struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type GraphEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// Graph links agents by use. "used" means the reader fetched the writer's note.
// "sent" means a direct message. "cited" means a note body names the other agent's note.
func (s *Store) Graph(ctx context.Context) (Graph, error) {
	g := Graph{Nodes: []GraphNode{}, Edges: []GraphEdge{}}
	rows, err := s.Pool.Query(ctx, `
		SELECT agent_id::text, name, status FROM agents
		WHERE status <> 'revoked'
		ORDER BY name`)
	if err != nil {
		return g, fmt.Errorf("graph agents: %w", err)
	}
	for rows.Next() {
		var n GraphNode
		if err := rows.Scan(&n.ID, &n.Name, &n.Status); err != nil {
			rows.Close()
			return g, err
		}
		g.Nodes = append(g.Nodes, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return g, err
	}
	edges, err := s.graphEdges(ctx, `
		SELECT r.agent_id::text, nv.agent_id::text, count(DISTINCT r.note_id)::int
		FROM note_reads r
		JOIN note_versions nv ON nv.note_id = r.note_id AND nv.version = r.version
		WHERE r.agent_id <> nv.agent_id
		GROUP BY 1, 2`, "used")
	if err != nil {
		return g, err
	}
	g.Edges = append(g.Edges, edges...)
	dmRows, err := s.Pool.Query(ctx, `
		SELECT agent_id::text, space_id, count(*)::int
		FROM note_versions
		WHERE space_id LIKE 'dm/%'
		GROUP BY 1, 2`)
	if err != nil {
		return g, fmt.Errorf("graph dm: %w", err)
	}
	for dmRows.Next() {
		var from, space string
		var n int
		if err := dmRows.Scan(&from, &space, &n); err != nil {
			dmRows.Close()
			return g, err
		}
		a, b, ok := dmPair(space)
		if !ok {
			continue
		}
		to := b
		if from == b {
			to = a
		}
		if from == to {
			continue
		}
		g.Edges = append(g.Edges, GraphEdge{From: from, To: to, Kind: "sent", Count: n})
	}
	dmRows.Close()
	if err := dmRows.Err(); err != nil {
		return g, err
	}
	cited, err := s.graphEdges(ctx, `
		SELECT writer.agent_id::text, cited.agent_id::text, count(DISTINCT writer.note_id)::int
		FROM note_versions writer
		JOIN note_current wc ON wc.note_id = writer.note_id AND wc.version = writer.version
		JOIN note_current cc ON writer.note_id <> cc.note_id
		JOIN note_versions cited ON cited.note_id = cc.note_id AND cited.version = cc.version
		WHERE writer.agent_id <> cited.agent_id
		  AND position(cited.note_id::text in writer.body) > 0
		GROUP BY 1, 2`, "cited")
	if err != nil {
		return g, err
	}
	g.Edges = append(g.Edges, cited...)

	// Revoked agents are operationally removed: nodes already exclude them,
	// so drop edges that reference one. Without this a revoked agent leaves
	// edges pointing at nodes the dashboard never renders.
	active := make(map[string]bool, len(g.Nodes))
	for _, n := range g.Nodes {
		active[n.ID] = true
	}
	kept := g.Edges[:0]
	for _, e := range g.Edges {
		if active[e.From] && active[e.To] {
			kept = append(kept, e)
		}
	}
	g.Edges = kept
	return g, nil
}

func (s *Store) graphEdges(ctx context.Context, q, kind string) ([]GraphEdge, error) {
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("graph %s: %w", kind, err)
	}
	defer rows.Close()
	var out []GraphEdge
	for rows.Next() {
		var e GraphEdge
		if err := rows.Scan(&e.From, &e.To, &e.Count); err != nil {
			return nil, err
		}
		e.Kind = kind
		out = append(out, e)
	}
	return out, rows.Err()
}

func dmPair(space string) (string, string, bool) {
	const prefix = "dm/"
	if len(space) <= len(prefix) || space[:len(prefix)] != prefix {
		return "", "", false
	}
	rest := space[len(prefix):]
	for i := 0; i < len(rest); i++ {
		if rest[i] == '/' {
			a, b := rest[:i], rest[i+1:]
			if a == "" || b == "" || containsSlash(b) {
				return "", "", false
			}
			return a, b, true
		}
	}
	return "", "", false
}

func containsSlash(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return true
		}
	}
	return false
}

// Activity is one named agent event for the admin dashboard.
type Activity struct {
	Kind  string `json:"kind"`
	Actor string `json:"actor"`
	Other string `json:"other,omitempty"`
	Space string `json:"space,omitempty"`
	At    string `json:"at"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
}

func (s *Store) RecentActivity(ctx context.Context, limit int) ([]Activity, error) {
	if limit < 1 {
		limit = 40
	}
	if limit > 80 {
		limit = 80
	}
	names := map[string]string{}
	nr, err := s.Pool.Query(ctx, `SELECT agent_id::text, name FROM agents`)
	if err != nil {
		return nil, fmt.Errorf("activity names: %w", err)
	}
	for nr.Next() {
		var id, name string
		if err := nr.Scan(&id, &name); err != nil {
			nr.Close()
			return nil, err
		}
		names[id] = name
	}
	nr.Close()
	if err := nr.Err(); err != nil {
		return nil, err
	}

	type row struct {
		kind, actor, other, space, from, to string
		at                                  time.Time
	}
	var raw []row

	writes, err := s.Pool.Query(ctx, `
		SELECT nv.agent_id::text, a.name, nv.space_id, nv.created_at
		FROM note_versions nv
		JOIN agents a ON a.agent_id = nv.agent_id
		WHERE a.status <> 'revoked'
		ORDER BY nv.created_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("activity writes: %w", err)
	}
	for writes.Next() {
		var id, name, space string
		var at time.Time
		if err := writes.Scan(&id, &name, &space, &at); err != nil {
			writes.Close()
			return nil, err
		}
		r := row{kind: "wrote", actor: name, space: space, from: id, at: at}
		if a, b, ok := dmPair(space); ok {
			r.kind = "sent"
			if id == a {
				r.to = b
			} else {
				r.to = a
			}
			r.other = names[r.to]
		}
		raw = append(raw, r)
	}
	writes.Close()
	if err := writes.Err(); err != nil {
		return nil, err
	}

	reads, err := s.Pool.Query(ctx, `
		SELECT r.agent_id::text, ra.name, nv.agent_id::text, wa.name, r.read_at
		FROM note_reads r
		JOIN agents ra ON ra.agent_id = r.agent_id
		JOIN note_versions nv ON nv.note_id = r.note_id AND nv.version = r.version
		JOIN agents wa ON wa.agent_id = nv.agent_id
		WHERE r.agent_id <> nv.agent_id
		  AND ra.status <> 'revoked' AND wa.status <> 'revoked'
		ORDER BY r.read_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("activity reads: %w", err)
	}
	for reads.Next() {
		var from, actor, to, other string
		var at time.Time
		if err := reads.Scan(&from, &actor, &to, &other, &at); err != nil {
			reads.Close()
			return nil, err
		}
		raw = append(raw, row{kind: "used", actor: actor, other: other, from: from, to: to, at: at})
	}
	reads.Close()
	if err := reads.Err(); err != nil {
		return nil, err
	}

	sort.Slice(raw, func(i, j int) bool { return raw[i].at.After(raw[j].at) })
	if len(raw) > limit {
		raw = raw[:limit]
	}
	out := make([]Activity, 0, len(raw))
	for _, r := range raw {
		out = append(out, Activity{
			Kind: r.kind, Actor: r.actor, Other: r.other, Space: r.space,
			At: r.at.UTC().Format(time.RFC3339Nano), From: r.from, To: r.to,
		})
	}
	return out, nil
}
