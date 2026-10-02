import { useEffect, useState, useRef } from 'react';
import { useAppStore } from '../store';
import type { Agent } from '../types';

interface CommandItem {
  id: string;
  label: string;
  description: string;
  category: 'agent' | 'action';
  action: () => void;
  keywords: string[];
}

export function CommandPalette() {
  const {
    isCommandPaletteOpen, setCommandPaletteOpen, agents,
    selectedNodeId, setSelectedNodeId, setZenMode, isZenMode, setCreateAgentOpen,
  } = useAppStore();
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [items, setItems] = useState<CommandItem[]>([]);
  const inputRef = useRef<HTMLInputElement>(null);

  // Build command items
  useEffect(() => {
    const newItems: CommandItem[] = [
      ...agents.map((agent: Agent) => ({
        id: `agent-${agent.agent_id}`,
        label: agent.name,
        description: `Agent · ${agent.status} · ${agent.agent_id.slice(0, 8)}`,
        category: 'agent' as const,
        action: () => setSelectedNodeId(agent.agent_id),
        keywords: [agent.name, agent.agent_id, agent.status],
      })),
      {
        id: 'create-agent',
        label: 'Create Agent',
        description: 'Initialize a new agent and get its API key',
        category: 'action' as const,
        action: () => setCreateAgentOpen(true),
        keywords: ['create', 'new', 'agent', 'init', 'initialize'],
      },
      {
        id: 'zen-mode',
        label: 'Toggle Zen Mode',
        description: 'Full-screen ambient graph view',
        category: 'action' as const,
        action: () => setZenMode(!isZenMode),
        keywords: ['zen', 'fullscreen', 'ambient', 'graph'],
      },
    ];

    if (selectedNodeId) {
      newItems.push({
        id: `inspect-${selectedNodeId}`,
        label: `Inspect ${selectedNodeId.slice(0, 12)}`,
        description: 'Open the inspector for the selected node',
        category: 'action' as const,
        action: () => setSelectedNodeId(selectedNodeId),
        keywords: ['inspect', 'view', 'detail', selectedNodeId],
      });
    }

    setItems(newItems);
  }, [agents, selectedNodeId, setSelectedNodeId, setZenMode, isZenMode, setCreateAgentOpen]);

  // Filter items
  const filteredItems = items
    .filter((item) => {
      if (!query) return true;
      const q = query.toLowerCase();
      return (
        item.label.toLowerCase().includes(q) ||
        item.description.toLowerCase().includes(q) ||
        item.keywords.some((k) => k.toLowerCase().includes(q))
      );
    })
    .slice(0, 10);

  const closePalette = () => {
    setCommandPaletteOpen(false);
    setQuery('');
    setSelectedIndex(0);
  };

  // Handle keyboard
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!isCommandPaletteOpen) return;

      switch (e.key) {
        case 'Escape':
          e.preventDefault();
          closePalette();
          break;
        case 'ArrowDown':
          e.preventDefault();
          setSelectedIndex((i) => Math.min(i + 1, filteredItems.length - 1));
          break;
        case 'ArrowUp':
          e.preventDefault();
          setSelectedIndex((i) => Math.max(i - 1, 0));
          break;
        case 'Enter':
          e.preventDefault();
          if (filteredItems[selectedIndex]) {
            filteredItems[selectedIndex].action();
            closePalette();
          }
          break;
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isCommandPaletteOpen, filteredItems, selectedIndex]);

  // Focus input on open
  useEffect(() => {
    if (isCommandPaletteOpen) {
      setQuery('');
      setSelectedIndex(0);
      setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [isCommandPaletteOpen]);

  if (!isCommandPaletteOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-20 px-4" role="dialog" aria-modal="true" aria-label="Command palette">
      <div className="absolute inset-0 bg-bg/80 backdrop-blur-sm" onClick={closePalette} />
      <div className="panel-strong w-full max-w-2xl animate-in relative" style={{ maxHeight: '60vh' }}>
        <div className="flex items-center gap-2 mb-4">
          <svg className="w-5 h-5 text-textMuted flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            ref={inputRef}
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Type a command or search…"
            className="input flex-1 bg-transparent border-0 focus:ring-0 text-lg font-mono"
            autoFocus
            aria-label="Command palette search"
          />
          <kbd className="font-mono text-xs text-textMuted px-2 py-0.5 bg-border/50 rounded">⌘K</kbd>
        </div>

        {filteredItems.length === 0 ? (
          <div className="text-center py-8 text-textMuted">
            <svg className="w-12 h-12 mx-auto mb-3 opacity-30" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9.172 16.172a4 4 0 015.656 0M9 10h.01M15 10h.01M21 12a9 9 0 11-18 0 9 9 0 0114 0z" /></svg>
            <p>No commands match</p>
          </div>
        ) : (
          <ul className="space-y-1 max-h-96 overflow-y-auto scrollable" role="listbox">
            {filteredItems.map((item, index) => (
              <li
                key={item.id}
                role="option"
                aria-selected={index === selectedIndex}
                className={`flex items-center gap-3 px-3 py-2 rounded-lg cursor-pointer transition-colors ${
                  index === selectedIndex ? 'bg-accentDim' : 'hover:bg-border/50'
                }`}
                onClick={() => {
                  item.action();
                  closePalette();
                }}
                onMouseEnter={() => setSelectedIndex(index)}
              >
                <span className={`badge ${item.category === 'agent' ? 'badge-ok' : 'badge-warn'}`}>
                  {item.category}
                </span>
                <div className="flex-1 min-w-0">
                  <div className="font-mono text-sm truncate">{item.label}</div>
                  <div className="text-textMuted text-xs truncate">{item.description}</div>
                </div>
              </li>
            ))}
          </ul>
        )}

        <div className="mt-4 pt-4 border-t border-border/50 flex items-center justify-between text-xs text-textMuted">
          <span>↑↓ navigate · ↵ select · Esc close</span>
          <span className="font-mono">{filteredItems.length} result{filteredItems.length !== 1 ? 's' : ''}</span>
        </div>
      </div>
    </div>
  );
}
