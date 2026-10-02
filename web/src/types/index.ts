export interface Agent {
  agent_id: string;
  name: string;
  description: string | null;
  card: unknown;
  status: 'active' | 'suspended' | 'revoked';
  last_active: string | null;
}

export interface AgentCreateRequest {
  name: string;
  description?: string;
}

export interface AgentCreateResponse {
  agent_id: string;
  api_key: string;
}

export interface AgentStats {
  notes: number;
  bytes: number;
  reads: number;
  spaces: string[];
}

export interface GraphNode {
  id: string;
  name: string;
  status: 'active' | 'suspended' | 'revoked';
}

export interface GraphEdge {
  from: string;
  to: string;
  kind: 'used' | 'sent' | 'cited' | 'wrote';
  count: number;
}

export interface GraphData {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface TelemetryHealth {
  status: string;
  version: string;
  db: string;
  embedder: string;
  streams: number;
  note_versions: number;
  pending_embeds: number;
}

export interface TelemetryActivity {
  at: string;
  kind: 'used' | 'sent' | 'cited' | 'wrote';
  actor: string;
  other?: string;
  from?: string;
  to?: string;
  space?: string;
}

export interface TelemetryData {
  health: TelemetryHealth;
  stats: unknown;
  agents: Agent[];
  activity: TelemetryActivity[];
}

export interface ConnectionState {
  status: 'connecting' | 'connected' | 'disconnected';
}
