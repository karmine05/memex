import { useEffect } from 'react';
import { useAppStore } from '../store';
import { api } from '../utils/api';
import { AgentsPane } from './AgentsPane';
import { GraphPane } from './GraphPane';
import { InspectorPane } from './InspectorPane';
import { TopBar } from './TopBar';
import { CommandPalette } from './CommandPalette';

// The admin listener has no SSE endpoints: /admin/telemetry and /admin/graph
// return plain JSON. Poll instead — a failed poll marks the connection
// disconnected, a successful one refreshes the dashboard.
const TELEMETRY_INTERVAL_MS = 3000;
const GRAPH_INTERVAL_MS = 30000;

export function Dashboard() {
  const { setConnectionStatus, setAgents, setGraph, setActivity, isZenMode } = useAppStore();

  // Telemetry poll
  useEffect(() => {
    let alive = true;
    const load = async () => {
      try {
        const data = await api.getTelemetry();
        if (!alive) return;
        setAgents(data.agents);
        setActivity(data.activity);
        setConnectionStatus('connected');
      } catch {
        if (alive) setConnectionStatus('disconnected');
      }
    };
    load();
    const timer = setInterval(load, TELEMETRY_INTERVAL_MS);
    return () => { alive = false; clearInterval(timer); };
  }, [setAgents, setActivity, setConnectionStatus]);

  // Graph poll
  useEffect(() => {
    let alive = true;
    const load = () => {
      api.getGraph()
        .then(data => { if (alive) setGraph(data); })
        .catch(() => {});
    };
    load();
    const timer = setInterval(load, GRAPH_INTERVAL_MS);
    return () => { alive = false; clearInterval(timer); };
  }, [setGraph]);

  return (
    <div className="h-full w-full flex flex-col" data-zen={isZenMode}>
      <TopBar />
      <div className="flex-1 flex overflow-hidden">
        <AgentsPane />
        <GraphPane />
        <InspectorPane />
      </div>
      <CommandPalette />
    </div>
  );
}
