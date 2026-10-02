import { useRef, useEffect } from 'react';
import { useAppStore } from '../store';
import type { GraphData } from '../types';

export function GraphPane() {
  const { graph, selectedNodeId, setSelectedNodeId, isZenMode } = useAppStore();
  const canvasRef = useRef<HTMLCanvasElement>(null);

  // Ambient starfield background. Re-runs when zen mode swaps the canvas so
  // the rAF loop never paints a detached element.
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const cleanup = initStarfield(canvas);
    return cleanup;
  }, [isZenMode]);

  if (isZenMode) {
    return (
      <div className="fixed inset-0 z-50 bg-bg" role="main" aria-label="Zen mode - fullscreen graph">
        <canvas ref={canvasRef} className="w-full h-full" />
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
        <canvas ref={canvasRef} className="absolute inset-0 w-full h-full" style={{ touchAction: 'none' }} />
        {graph && graph.nodes.length === 0 && (
          <div className="absolute inset-0 flex items-center justify-center text-textMuted">
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
    <div className="absolute bottom-4 left-4 glass p-3 rounded-xl max-h-60 overflow-auto" style={{ maxWidth: 200 }}>
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

function initStarfield(canvas: HTMLCanvasElement): () => void {
  const ctx = canvas.getContext('2d');
  if (!ctx) return () => {};

  const DPR = Math.min(window.devicePixelRatio || 1, 2);

  function resize() {
    const rect = canvas.getBoundingClientRect();
    canvas.width = Math.round(rect.width * DPR);
    canvas.height = Math.round(rect.height * DPR);
    canvas.style.width = rect.width + 'px';
    canvas.style.height = rect.height + 'px';
  }

  resize();
  window.addEventListener('resize', resize);

  const stars = Array.from({ length: 200 }, () => ({
    x: Math.random() * canvas.width,
    y: Math.random() * canvas.height,
    size: 0.5 + Math.random() * 1.5,
    speed: 0.1 + Math.random() * 0.3,
    opacity: 0.2 + Math.random() * 0.5,
  }));

  let animationId = 0;

  function animate() {
    const c = ctx;
    if (!c) return;
    c.clearRect(0, 0, canvas.width, canvas.height);

    // Subtle grid
    c.strokeStyle = 'rgba(111, 211, 255, 0.03)';
    c.lineWidth = 1;
    const gridSize = 50 * DPR;
    for (let i = 0; i <= canvas.width; i += gridSize) {
      c.beginPath();
      c.moveTo(i, 0);
      c.lineTo(i, canvas.height);
      c.stroke();
    }
    for (let i = 0; i <= canvas.height; i += gridSize) {
      c.beginPath();
      c.moveTo(0, i);
      c.lineTo(canvas.width, i);
      c.stroke();
    }

    // Drifting particles
    for (const star of stars) {
      star.y -= star.speed * DPR;
      if (star.y < 0) {
        star.y = canvas.height;
        star.x = Math.random() * canvas.width;
      }
      c.fillStyle = `rgba(111, 211, 255, ${star.opacity})`;
      c.beginPath();
      c.arc(star.x, star.y, star.size * DPR, 0, Math.PI * 2);
      c.fill();
    }

    // Central glow
    const centerX = canvas.width / 2;
    const centerY = canvas.height / 2;
    const gradient = c.createRadialGradient(centerX, centerY, 0, centerX, centerY, Math.min(canvas.width, canvas.height) * 0.4);
    gradient.addColorStop(0, 'rgba(111, 211, 255, 0.08)');
    gradient.addColorStop(0.5, 'rgba(111, 211, 255, 0.02)');
    gradient.addColorStop(1, 'rgba(111, 211, 255, 0)');
    c.fillStyle = gradient;
    c.fillRect(0, 0, canvas.width, canvas.height);

    animationId = requestAnimationFrame(animate);
  }

  animate();

  return () => {
    cancelAnimationFrame(animationId);
    window.removeEventListener('resize', resize);
  };
}
