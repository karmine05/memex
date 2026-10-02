import type { AgentStats, AgentCreateRequest, AgentCreateResponse, GraphData, TelemetryData } from '../types';

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string>),
  };

  const key = typeof window !== 'undefined' ? localStorage.getItem('memex.adminKey') : null;
  if (key) {
    headers['Authorization'] = `Bearer ${key}`;
  }

  const res = await fetch(path, {
    ...options,
    headers,
  });

  if (!res.ok) {
    let message = `HTTP ${res.status}`;
    try {
      const err = await res.json();
      message = err.error || message;
    } catch {}
    throw new ApiError(message, res.status);
  }

  if (res.status === 204) {
    return undefined as T;
  }

  return res.json();
}

export const api = {
  // Agents
  createAgent: (data: AgentCreateRequest) =>
    request<AgentCreateResponse>('/admin/agents', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  setAgentStatus: (id: string, status: 'suspended' | 'revoked' | 'active') => {
    const action = status === 'active' ? 'resume' : status === 'suspended' ? 'suspend' : 'revoke';
    return request<{ agent_id: string; status: string }>(`/admin/agents/${id}/${action}`, {
      method: 'POST',
    });
  },
  getAgentStats: (id: string) => request<AgentStats>(`/admin/agents/${id}/stats`),

  // Graph + telemetry
  getGraph: () => request<GraphData>('/admin/graph'),
  getTelemetry: () => request<TelemetryData>('/admin/telemetry'),

  // Agent protocol for the one-prompt install
  getSkill: () => fetch('/skill.md').then(r => {
    if (!r.ok) throw new Error(`skill.md HTTP ${r.status}`);
    return r.text();
  }),
};

export function setAdminKey(key: string) {
  localStorage.setItem('memex.adminKey', key);
}

export function getAdminKey(): string | null {
  return localStorage.getItem('memex.adminKey');
}

export function clearAdminKey() {
  localStorage.removeItem('memex.adminKey');
}
