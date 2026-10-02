import { useEffect, useState } from 'react';
import { useAppStore } from '../store';
import { api } from '../utils/api';
import type { Agent, AgentStats } from '../types';
import { formatDistanceToNow } from 'date-fns';

const statusConfig = {
  active: { class: 'badge-ok', dot: 'bg-ok', label: 'Active' },
  suspended: { class: 'badge-warn', dot: 'bg-warn', label: 'Suspended' },
  revoked: { class: 'badge-bad', dot: 'bg-bad', label: 'Revoked' },
} as const;

// Floating glass "playing card" with the selected agent's details.
// Renders only while a specific agent is selected; auto-hides on Escape,
// on close, or when the agent leaves the roster.
export function AgentCard() {
  const { selectedNodeId, setSelectedNodeId, agents } = useAppStore();
  const agent = selectedNodeId ? agents.find(a => a.agent_id === selectedNodeId) ?? null : null;

  useEffect(() => {
    if (!selectedNodeId) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setSelectedNodeId(null);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [selectedNodeId, setSelectedNodeId]);

  // Auto-hide if the selection no longer resolves to a live agent.
  useEffect(() => {
    if (selectedNodeId && agents.length > 0 && !agents.some(a => a.agent_id === selectedNodeId)) {
      setSelectedNodeId(null);
    }
  }, [selectedNodeId, agents, setSelectedNodeId]);

  if (!selectedNodeId || !agent) return null;

  return (
    <aside
      className="fixed top-16 right-4 z-50 w-80 max-w-[calc(100vw-2rem)] animate-cardIn"
      aria-label={`Agent details: ${agent.name}`}
    >
      <AgentCardBody key={agent.agent_id} agent={agent} onClose={() => setSelectedNodeId(null)} />
    </aside>
  );
}

function AgentCardBody({ agent, onClose }: { agent: Agent; onClose: () => void }) {
  const { updateAgent } = useAppStore();
  const [stats, setStats] = useState<AgentStats | null>(null);
  const [error, setError] = useState('');
  const [idCopied, setIdCopied] = useState(false);

  useEffect(() => {
    let alive = true;
    api.getAgentStats(agent.agent_id)
      .then(s => { if (alive) setStats(s); })
      .catch(err => { if (alive) setError(err instanceof Error ? err.message : 'Failed to load stats'); });
    return () => { alive = false; };
  }, [agent.agent_id]);

  const handleStatusChange = async (newStatus: 'active' | 'suspended' | 'revoked') => {
    if (newStatus === 'revoked') {
      if (!confirm(`Revoke agent "${agent.name}"? This cannot be undone.`)) return;
    }
    try {
      await api.setAgentStatus(agent.agent_id, newStatus);
      updateAgent(agent.agent_id, { status: newStatus });
    } catch (err) {
      alert(`Failed: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
  };

  const copyId = async () => {
    try {
      await navigator.clipboard.writeText(agent.agent_id);
    } catch {
      const ta = document.createElement('textarea');
      ta.value = agent.agent_id;
      document.body.appendChild(ta);
      ta.select();
      try { document.execCommand('copy'); } catch {}
      ta.remove();
    }
    setIdCopied(true);
    setTimeout(() => setIdCopied(false), 1200);
  };

  const config = statusConfig[agent.status];

  return (
    <article className="glass-strong rounded-2xl overflow-hidden shadow-2xl">
      <div className="h-1 bg-gradient-to-r from-accent/70 via-accent/25 to-transparent" aria-hidden="true" />

      <header className="p-4 pb-3 border-b border-border/50 flex items-center gap-3">
        <span
          className="flex-shrink-0 w-10 h-10 rounded-xl flex items-center justify-center font-ui font-bold text-lg text-bg"
          style={{ background: 'linear-gradient(135deg, rgba(111,211,255,0.9), #0aa2d6)' }}
          aria-hidden="true"
        >
          {agent.name.charAt(0).toUpperCase() || '?'}
        </span>
        <div className="min-w-0 flex-1">
          <div className="font-ui font-semibold text-sm truncate text-text" title={agent.name}>{agent.name}</div>
          <span className={`${config.class} mt-1`}>{config.label}</span>
        </div>
        <button
          onClick={onClose}
          className="btn-ghost p-1.5 rounded-lg -mr-1"
          aria-label="Close agent details"
          title="Close (Esc)"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" /></svg>
        </button>
      </header>

      <div className="p-4 space-y-4 scrollable overflow-y-auto max-h-[min(50vh,24rem)]">
        {error && <div className="text-bad text-sm" role="alert">{error}</div>}

        <div>
          <span className="label">ID</span>
          <div className="flex items-center gap-1.5">
            <code className="font-mono text-xs text-text break-all bg-bg/60 border border-border/50 px-2 py-1.5 rounded-lg flex-1">{agent.agent_id}</code>
            <button type="button" onClick={copyId} className="btn-secondary text-[11px] px-2 py-1.5 uppercase tracking-wider" aria-label="Copy agent ID">
              {idCopied ? 'Copied' : 'Copy'}
            </button>
          </div>
        </div>

        {agent.description && (
          <div>
            <span className="label">Description</span>
            <p className="text-sm text-text leading-snug">{agent.description}</p>
          </div>
        )}

        <div className="grid grid-cols-2 gap-3">
          <div>
            <span className="label">Last Active</span>
            <span className="font-mono text-sm text-text">
              {agent.last_active ? formatDistanceToNow(new Date(agent.last_active), { addSuffix: true }) : 'Never'}
            </span>
          </div>
        </div>

        <div>
          <span className="label">Activity</span>
          {stats ? (
            <div className="grid grid-cols-3 gap-2">
              <StatCard label="Notes" value={stats.notes.toLocaleString()} />
              <StatCard label="Reads" value={stats.reads.toLocaleString()} />
              <StatCard label="Bytes" value={formatBytes(stats.bytes)} />
            </div>
          ) : (
            <div className="text-textMuted text-sm">Loading…</div>
          )}
        </div>

        {stats && stats.spaces.length > 0 && (
          <div>
            <span className="label">Spaces</span>
            <div className="flex flex-wrap gap-1">
              {stats.spaces.map(space => (
                <code key={space} className="font-mono text-xs text-textMuted bg-border/50 px-2 py-0.5 rounded">{space}</code>
              ))}
            </div>
          </div>
        )}
      </div>

      <footer className="p-3 border-t border-border/50 flex gap-2">
        {agent.status !== 'revoked' ? (
          <>
            <button
              onClick={() => handleStatusChange(agent.status === 'active' ? 'suspended' : 'active')}
              className={`btn-secondary flex-1 text-xs ${agent.status === 'active' ? 'bg-warn/20 text-warn hover:bg-warn/30' : 'bg-ok/20 text-ok hover:bg-ok/30'}`}
            >
              {agent.status === 'active' ? 'Suspend' : 'Resume'}
            </button>
            <button
              onClick={() => handleStatusChange('revoked')}
              className="btn-danger flex-1 text-xs"
            >
              Revoke
            </button>
          </>
        ) : (
          <span className="flex-1 text-center text-textMuted text-xs py-1.5">Revoked — this agent can no longer authenticate</span>
        )}
      </footer>
    </article>
  );
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="glass p-2 rounded-lg text-center">
      <div className="font-mono text-sm text-accent">{value}</div>
      <div className="text-textMuted text-xs">{label}</div>
    </div>
  );
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}
