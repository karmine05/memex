import { useState, useRef } from 'react';
import { useAppStore } from '../store';
import { api } from '../utils/api';
import type { Agent } from '../types';
import { formatDistanceToNow } from 'date-fns';

const statusConfig = {
  active: { class: 'badge-ok', dot: 'bg-ok', label: 'Active' },
  suspended: { class: 'badge-warn', dot: 'bg-warn', label: 'Suspended' },
  revoked: { class: 'badge-bad', dot: 'bg-bad', label: 'Revoked' },
} as const;

export function AgentsPane() {
  const { agents, agentsWidth, setAgentsWidth, updateAgent, selectedNodeId, setSelectedNodeId, isZenMode } = useAppStore();
  const [filter, setFilter] = useState('');
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const resizeRef = useRef<HTMLDivElement>(null);

  const filteredAgents = agents.filter(a =>
    a.name.toLowerCase().includes(filter.toLowerCase()) ||
    a.agent_id.toLowerCase().includes(filter.toLowerCase())
  );

  const handleStatusChange = async (agent: Agent, newStatus: 'active' | 'suspended' | 'revoked') => {
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

  const startResize = (e: React.MouseEvent) => {
    e.preventDefault();
    const startX = e.clientX;
    const startWidth = agentsWidth;

    const onMouseMove = (e: MouseEvent) => {
      const newWidth = Math.max(200, Math.min(500, startWidth + (e.clientX - startX)));
      setAgentsWidth(newWidth);
    };

    const onMouseUp = () => {
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
    };

    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
  };

  if (isZenMode) return null;

  return (
    <aside
      className="flex flex-col glass border-r border-border/50 shrink-0 overflow-hidden"
      style={{ width: agentsWidth, minWidth: 200, maxWidth: 500 }}
      role="complementary"
      aria-label="Agents"
    >
      <div className="p-3 border-b border-border/50 flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <h3 className="font-ui font-semibold text-sm tracking-wider text-textMuted uppercase">Agents</h3>
          <span className="badge badge-dim">{agents.length}</span>
        </div>
        <input
          type="search"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="Filter agents…"
          className="input text-xs py-1.5"
          aria-label="Filter agents"
        />
      </div>

      <div className="flex-1 overflow-y-auto scrollable p-2" role="list" aria-label="Agent list">
        {filteredAgents.length === 0 ? (
          <div className="text-center py-8 text-textMuted text-sm">
            {filter ? 'No agents match' : 'No agents yet'}
          </div>
        ) : (
          <ul className="space-y-1" role="list">
            {filteredAgents.map((agent) => (
              <AgentItem
                key={agent.agent_id}
                agent={agent}
                isSelected={selectedNodeId === agent.agent_id}
                isExpanded={expandedId === agent.agent_id}
                onSelect={() => {
                  setSelectedNodeId(agent.agent_id);
                  setExpandedId(expandedId === agent.agent_id ? null : agent.agent_id);
                }}
                onStatusChange={handleStatusChange}
              />
            ))}
          </ul>
        )}
      </div>

      <div
        ref={resizeRef}
        className="w-1 cursor-col-resize bg-border/50 hover:bg-accent/50 transition-colors"
        onMouseDown={startResize}
        aria-label="Resize agents panel"
        role="separator"
        tabIndex={0}
        onKeyDown={(e) => {
          if (e.key === 'ArrowLeft') setAgentsWidth(Math.max(200, agentsWidth - 20));
          if (e.key === 'ArrowRight') setAgentsWidth(Math.min(500, agentsWidth + 20));
        }}
      />
    </aside>
  );
}

function AgentItem({
  agent,
  isSelected,
  isExpanded,
  onSelect,
  onStatusChange,
}: {
  agent: Agent;
  isSelected: boolean;
  isExpanded: boolean;
  onSelect: () => void;
  onStatusChange: (agent: Agent, status: 'active' | 'suspended' | 'revoked') => void;
}) {
  const config = statusConfig[agent.status];

  return (
    <li
      role="listitem"
      className={`group relative flex flex-col rounded-xl transition-colors ${
        isSelected ? 'bg-accentDim ring-1 ring-accent/50' : 'hover:bg-border/30'
      }`}
    >
      <button
        onClick={onSelect}
        className={`flex items-center gap-2 p-2 rounded-lg w-full text-left focus-visible:ring-2 focus-visible:ring-accent ${
          isSelected ? 'bg-accentDim' : ''
        }`}
        aria-pressed={isSelected}
        aria-expanded={isExpanded}
      >
        <span className={`relative flex-shrink-0 w-2 h-2 rounded-full ${config.dot}`} aria-hidden="true" />
        <span className="font-mono text-sm truncate flex-1">{agent.name}</span>
        <span className={config.class} aria-label={`Status: ${config.label}`}>{config.label}</span>
      </button>

      {isExpanded && (
        <div className="px-2 pb-2 space-y-3 border-l-2 border-border/50 ml-3 animate-in">
          <div className="grid grid-cols-1 gap-2 text-xs">
            <div>
              <span className="text-textMuted">ID</span>
              <code className="font-mono text-text break-all block mt-0.5">{agent.agent_id}</code>
            </div>
            <div>
              <span className="text-textMuted">Last Active</span>
              <span className="font-mono text-text block mt-0.5">
                {agent.last_active ? formatDistanceToNow(new Date(agent.last_active), { addSuffix: true }) : 'Never'}
              </span>
            </div>
            {agent.description && (
              <div>
                <span className="text-textMuted">Description</span>
                <span className="text-text block mt-0.5">{agent.description}</span>
              </div>
            )}
          </div>

          <div className="flex gap-2 pt-1">
            {agent.status !== 'revoked' && (
              <>
                <button
                  onClick={() => onStatusChange(agent, agent.status === 'active' ? 'suspended' : 'active')}
                  className={`btn-secondary flex-1 text-xs ${agent.status === 'active' ? 'bg-warn/20 text-warn hover:bg-warn/30' : 'bg-ok/20 text-ok hover:bg-ok/30'}`}
                >
                  {agent.status === 'active' ? 'Suspend' : 'Resume'}
                </button>
                <button
                  onClick={() => onStatusChange(agent, 'revoked')}
                  className="btn-danger flex-1 text-xs"
                >
                  Revoke
                </button>
              </>
            )}
            {agent.status === 'revoked' && (
              <span className="flex-1 text-center text-textMuted text-xs py-2">Revoked</span>
            )}
          </div>
        </div>
      )}
    </li>
  );
}
