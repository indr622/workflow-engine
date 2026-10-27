import { create } from 'zustand';
import {
  applyEdgeChanges,
  applyNodeChanges,
  layoutGraph,
  type CardNodeType,
  type Edge,
  type EdgeChange,
  type NodeChange,
} from '@workflow/flow-canvas';
import { sampleProcessMap, type ProcessKind, type ProcessMap } from '../domain/process-map';

const toneByKind: Record<ProcessKind, CardNodeType['data']['tone']> = {
  process: 'default',
  'call-activity': 'secondary',
  decision: 'success',
};

/** toGraph memetakan domain ProcessMap ke node/edge React Flow yang sudah di-layout. */
export function toGraph(map: ProcessMap): { nodes: CardNodeType[]; edges: Edge[] } {
  const nodes: CardNodeType[] = map.processes.map((p) => ({
    id: p.id,
    type: 'card',
    position: { x: 0, y: 0 },
    data: { title: p.name, description: `v${p.version}`, status: p.kind, tone: toneByKind[p.kind] },
  }));
  const edges: Edge[] = map.links.map((l) => ({ id: `${l.from}-${l.to}`, source: l.from, target: l.to }));
  return { nodes: layoutGraph(nodes, edges, { direction: 'LR' }), edges };
}

interface ProcessMapState {
  nodes: CardNodeType[];
  edges: Edge[];
  onNodesChange: (changes: NodeChange<CardNodeType>[]) => void;
  onEdgesChange: (changes: EdgeChange[]) => void;
  relayout: () => void;
}

/** State graf adalah state klien (posisi, seleksi), maka disimpan di Zustand. */
export const useProcessMapStore = create<ProcessMapState>()((set, get) => ({
  ...toGraph(sampleProcessMap()),
  onNodesChange: (changes) => set({ nodes: applyNodeChanges(changes, get().nodes) }),
  onEdgesChange: (changes) => set({ edges: applyEdgeChanges(changes, get().edges) }),
  relayout: () => set({ nodes: layoutGraph(get().nodes, get().edges, { direction: 'LR' }) }),
}));
