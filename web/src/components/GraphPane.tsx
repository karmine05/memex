import { useAppStore } from '../store';
import { Graph3D } from './Graph3D';
import type { GraphData } from '../types';

export function GraphPane() {
  const { graph, activity, selectedNodeId, setSelectedNodeId, isZenMode } = useAppStore();

  if (isZenMode) {
    return (
      <div className="fixed inset-0 z-50 bg-bg" role="main" aria-label="Zen mode - fullscreen graph">
        <Graph3D graph={graph} activity={activity} onNodeClick={setSelectedNodeId} />
        <div className="fixed bottom-4 right-4 glass px-4 py-2 rounded-lg text-sm text-textMuted">
          Double-click or press Z to exit
        </div>
      </div>
    );
  }

  return (
    <main
      className="flex-1 flex flex-col relative overflow-hidden"
      role="main"
      aria-label="Memory Graph"
    >
      <div className="flex items-center justify-between p-3 border-b border-border/50 glass bg-bg/50 shrink-0">
        <h3 className="font-ui font-semibold text-sm tracking-wider text-textMuted uppercase">Memory Graph</h3>
        <span className="font-mono text-xs text-textMuted">
          {graph ? `${graph.edges.length} link${graph.edges.length !== 1 ? 's' : ''}` : '—'}
        </span>
      </div>

      <div className="flex-1 relative overflow-hidden">
        <Graph3D graph={graph} activity={activity} onNodeClick={setSelectedNodeId} />
        {graph && graph.nodes.length === 0 && (
          <div className="absolute inset-0 flex items-center justify-center text-textMuted pointer-events-none">
            <div className="text-center p-8">
              <svg className="w-16 h-16 mx-auto mb-4 opacity-30" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
              </svg>
              <p className="font-mono text-lg">No agents connected</p>
              <p className="text-sm mt-1">Click "Init Agent" to create one</p>
            </div>
          </div>
        )}
        {graph && graph.nodes.length > 0 && (
          <GraphLegend nodes={graph.nodes} selectedId={selectedNodeId} onSelect={setSelectedNodeId} />
        )}
      </div>
    </main>
  );
}

function GraphLegend({
  nodes,
  selectedId,
  onSelect,
}: {
  nodes: GraphData['nodes'];
  selectedId: string | null;
  onSelect: (id: string) => void;
}) {
  return (
    <div className="absolute top-4 left-4 glass p-3 rounded-xl max-h-60 overflow-auto pointer-events-auto" style={{ maxWidth: 200 }}>
      <div className="font-mono text-xs text-textMuted uppercase tracking-wider mb-2">Agents</div>
      <div className="space-y-1">
        {nodes.slice(0, 20).map((node) => (
          <button
            key={node.id}
            className={`flex items-center gap-2 px-2 py-1 rounded-lg text-xs cursor-pointer transition-colors w-full text-left ${
              selectedId === node.id ? 'bg-accentDim' : 'hover:bg-border/50'
            }`}
            onClick={() => onSelect(node.id)}
            aria-pressed={selectedId === node.id}
          >
            <span className={`w-1.5 h-1.5 rounded-full ${node.status === 'active' ? 'bg-ok' : 'bg-warn'}`} />
            <span className="truncate font-mono">{node.name}</span>
          </button>
        ))}
        {nodes.length > 20 && (
          <div className="text-textMuted text-xs px-1">+{nodes.length - 20} more</div>
        )}
      </div>
    </div>
  );
}
