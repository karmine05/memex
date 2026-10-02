import { useEffect, useState } from 'react';
import { useAppStore } from '../store';
import { ApiError, api, clearAdminKey, getAdminKey, setAdminKey } from '../utils/api';
import { AdminKeyModal } from './AdminKeyModal';
import { AgentCard } from './AgentCard';
import { GraphPane } from './GraphPane';
import { TopBar } from './TopBar';
import { CommandPalette } from './CommandPalette';

// The admin listener has no SSE endpoints: /admin/telemetry and /admin/graph
// return plain JSON. Poll instead — a failed poll marks the connection
// disconnected, a successful one refreshes the dashboard.
const TELEMETRY_INTERVAL_MS = 3000;
const GRAPH_INTERVAL_MS = 30000;

export function Dashboard() {
  // /dash is gated by the mxa_ admin key. The modal validates it against
  // /admin/agents, which returns 401 while the gate is closed.
  const [needsAdminKey, setNeedsAdminKey] = useState(!getAdminKey());
  const [adminKeyValue, setAdminKeyValue] = useState('');
  const { setConnectionStatus, setAgents, setGraph, setActivity, isZenMode } = useAppStore();

  // A 401 mid-session means the key was rotated/revoked: drop it and re-lock.
  const handleAuthError = (err: unknown): boolean => {
    if (err instanceof ApiError && err.status === 401) {
      clearAdminKey();
      setNeedsAdminKey(true);
      return true;
    }
    return false;
  };

  // Telemetry poll
  useEffect(() => {
    if (needsAdminKey) return;
    let alive = true;
    const load = async () => {
      try {
        const data = await api.getTelemetry();
        if (!alive) return;
        setAgents(data.agents);
        setActivity(data.activity);
        setConnectionStatus('connected');
      } catch (err) {
        if (!alive) return;
        if (!handleAuthError(err)) setConnectionStatus('disconnected');
      }
    };
    load();
    const timer = setInterval(load, TELEMETRY_INTERVAL_MS);
    return () => { alive = false; clearInterval(timer); };
  }, [needsAdminKey, setAgents, setActivity, setConnectionStatus]);

  // Graph poll
  useEffect(() => {
    if (needsAdminKey) return;
    let alive = true;
    const load = () => {
      api.getGraph()
        .then(data => { if (alive) setGraph(data); })
        .catch((err) => { if (alive) handleAuthError(err); });
    };
    load();
    const timer = setInterval(load, GRAPH_INTERVAL_MS);
    return () => { alive = false; clearInterval(timer); };
  }, [needsAdminKey, setGraph]);

  const handleAdminKeySubmit = (key: string) => {
    setAdminKey(key);
    setAdminKeyValue(key);
    setNeedsAdminKey(false);
  };

  if (needsAdminKey) {
    return (
      <AdminKeyModal
        onSubmit={handleAdminKeySubmit}
        value={adminKeyValue}
        onChange={setAdminKeyValue}
      />
    );
  }

  return (
    <div className="h-full w-full flex flex-col" data-zen={isZenMode}>
      <TopBar />
      <div className="flex-1 flex overflow-hidden">
        <GraphPane />
      </div>
      <AgentCard />
      <CommandPalette />
    </div>
  );
}
