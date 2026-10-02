import { create } from 'zustand';
import { immer } from 'zustand/middleware/immer';
import type { Agent, GraphData, ConnectionState } from '../types';

interface AppState {
  // Connection
  connection: ConnectionState;
  setConnectionStatus: (status: ConnectionState['status']) => void;

  // Agents
  agents: Agent[];
  setAgents: (agents: Agent[]) => void;
  addAgent: (agent: Agent) => void;
  updateAgent: (id: string, updates: Partial<Agent>) => void;
  removeAgent: (id: string) => void;

  // Graph
  graph: GraphData | null;
  setGraph: (graph: GraphData) => void;

  // Inspector
  selectedNodeId: string | null;
  setSelectedNodeId: (id: string | null) => void;

  // UI
  isZenMode: boolean;
  setZenMode: (enabled: boolean) => void;
  inspectorWidth: number;
  setInspectorWidth: (width: number) => void;
  agentsWidth: number;
  setAgentsWidth: (width: number) => void;

  // Command palette
  isCommandPaletteOpen: boolean;
  setCommandPaletteOpen: (open: boolean) => void;

  // Create-agent modal
  isCreateAgentOpen: boolean;
  setCreateAgentOpen: (open: boolean) => void;
}

export const useAppStore = create<AppState>()(
  immer((set) => ({
    connection: { status: 'connecting' },
    setConnectionStatus: (status) => set((state) => { state.connection.status = status; }),

    agents: [],
    setAgents: (agents) => set((state) => { state.agents = agents; }),
    addAgent: (agent) => set((state) => { state.agents.push(agent); }),
    updateAgent: (id, updates) => set((state) => {
      const idx = state.agents.findIndex(a => a.agent_id === id);
      if (idx >= 0) Object.assign(state.agents[idx], updates);
    }),
    removeAgent: (id) => set((state) => {
      state.agents = state.agents.filter(a => a.agent_id !== id);
    }),

    graph: null,
    setGraph: (graph) => set((state) => { state.graph = graph; }),

    selectedNodeId: null,
    setSelectedNodeId: (id) => set((state) => { state.selectedNodeId = id; }),

    isZenMode: false,
    setZenMode: (enabled) => set((state) => { state.isZenMode = enabled; }),
    inspectorWidth: 320,
    setInspectorWidth: (width) => set((state) => { state.inspectorWidth = width; }),
    agentsWidth: 280,
    setAgentsWidth: (width) => set((state) => { state.agentsWidth = width; }),

    isCommandPaletteOpen: false,
    setCommandPaletteOpen: (open) => set((state) => { state.isCommandPaletteOpen = open; }),

    isCreateAgentOpen: false,
    setCreateAgentOpen: (open) => set((state) => { state.isCreateAgentOpen = open; }),
  }))
);
