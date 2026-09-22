package store

import (
	"context"
	"fmt"
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
