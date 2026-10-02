import { useAppStore } from '../store';
import { api } from '../utils/api';
import { useEffect, useState } from 'react';
import type { Agent, AgentStats } from '../types';
import { formatDistanceToNow } from 'date-fns';

// Graph nodes ARE agents (see store.Graph), so the inspector shows the
// selected agent's profile plus its per-agent write/read stats.
export function InspectorPane() {
  const { selectedNodeId, agents, inspectorWidth, setInspectorWidth, isZenMode } = useAppStore();

  if (isZenMode) return null;

  const agent = selectedNodeId ? agents.find(a => a.agent_id === selectedNodeId) ?? null : null;

  return (
    <aside
      className="flex flex-col glass border-l border-border/50 shrink-0 overflow-hidden"
      style={{ width: inspectorWidth, minWidth: 280, maxWidth: 600 }}
      role="complementary"
      aria-label="Agent Inspector"
    >
      <div
        className="w-1 cursor-col-resize bg-border/50 hover:bg-accent/50 transition-colors self-stretch"
        onMouseDown={(e) => {
          e.preventDefault();
          const startX = e.clientX;
          const startWidth = inspectorWidth;
          const onMouseMove = (e: MouseEvent) => {
            const newWidth = Math.max(280, Math.min(600, startWidth - (e.clientX - startX)));
            setInspectorWidth(newWidth);
          };
          const onMouseUp = () => {
            window.removeEventListener('mousemove', onMouseMove);
            window.removeEventListener('mouseup', onMouseUp);
          };
          window.addEventListener('mousemove', onMouseMove);
          window.addEventListener('mouseup', onMouseUp);
        }}
        aria-label="Resize inspector panel"
        role="separator"
        tabIndex={0}
      />

      <div className="flex flex-col h-full">
        {!selectedNodeId ? (
          <EmptyInspector />
        ) : (
          <>
            <div className="p-3 border-b border-border/50 glass bg-bg/50 shrink-0">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2 min-w-0">
                  <span className="font-mono text-xs text-accent">Agent</span>
                  <code className="font-mono text-sm text-text bg-border/50 px-2 py-0.5 rounded truncate">
                    {agent ? agent.name : selectedNodeId.slice(0, 12)}
                  </code>
                </div>
              </div>
            </div>
            <div className="flex-1 overflow-y-auto scrollable p-4">
              {agent ? (
                <AgentDetail key={agent.agent_id} agent={agent} />
              ) : (
                <div className="text-textMuted text-center py-8 text-sm">Agent not found</div>
              )}
            </div>
          </>
        )}
      </div>
    </aside>
  );
}

function EmptyInspector() {
  return (
    <div className="flex-1 flex items-center justify-center p-8 text-center">
      <div className="max-w-xs">
        <svg className="w-20 h-20 mx-auto mb-4 opacity-20" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
        </svg>
        <p className="font-ui font-medium text-lg text-text">Select an agent</p>
        <p className="text-textMuted text-sm mt-1">Click an agent in the list or the graph legend to inspect it</p>
      </div>
    </div>
  );
}

function AgentDetail({ agent }: { agent: Agent }) {
  const [stats, setStats] = useState<AgentStats | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    let alive = true;
    api.getAgentStats(agent.agent_id)
      .then(s => { if (alive) setStats(s); })
      .catch(err => { if (alive) setError(err instanceof Error ? err.message : 'Failed to load stats'); });
    return () => { alive = false; };
  }, [agent.agent_id]);

  const statusConfig = {
    active: { class: 'badge-ok', label: 'Active' },
    suspended: { class: 'badge-warn', label: 'Suspended' },
    revoked: { class: 'badge-bad', label: 'Revoked' },
  } as const;

  return (
    <div className="space-y-4">
      {error && <div className="text-bad text-sm" role="alert">{error}</div>}
      <div>
        <label className="label">Status</label>
        <span className={statusConfig[agent.status].class}>{statusConfig[agent.status].label}</span>
      </div>
      <div>
        <label className="label">ID</label>
        <code className="font-mono text-xs text-text break-all bg-border/50 p-2 rounded block">{agent.agent_id}</code>
      </div>
      {agent.description && (
        <div>
          <label className="label">Description</label>
          <p className="text-sm text-text">{agent.description}</p>
        </div>
      )}
      <div>
        <label className="label">Last Active</label>
        <span className="font-mono text-sm">
          {agent.last_active ? formatDistanceToNow(new Date(agent.last_active), { addSuffix: true }) : 'Never'}
        </span>
      </div>

      <div>
        <label className="label">Activity</label>
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
          <label className="label">Spaces</label>
          <div className="flex flex-wrap gap-1">
            {stats.spaces.map(space => (
              <code key={space} className="font-mono text-xs text-textMuted bg-border/50 px-2 py-0.5 rounded">{space}</code>
            ))}
          </div>
        </div>
      )}
    </div>
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
