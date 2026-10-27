import { Graph, layout } from '@dagrejs/dagre';
import type { Edge, Node } from '@xyflow/react';

export type LayoutDirection = 'LR' | 'TB';

export interface LayoutOptions {
  direction?: LayoutDirection;
  nodeWidth?: number;
  nodeHeight?: number;
}

/**
 * layoutGraph menghitung posisi node dengan dagre.
 * Fungsi murni: tidak mengubah input, mengembalikan salinan node dengan posisi baru.
 */
export function layoutGraph<N extends Node>(nodes: N[], edges: Edge[], options: LayoutOptions = {}): N[] {
  const { direction = 'LR', nodeWidth = 200, nodeHeight = 72 } = options;
  const g = new Graph();
  g.setGraph({ rankdir: direction, nodesep: 40, ranksep: 80 });
  g.setDefaultEdgeLabel(() => ({}));

  for (const node of nodes) {
    g.setNode(node.id, { width: node.measured?.width ?? nodeWidth, height: node.measured?.height ?? nodeHeight });
  }
  for (const edge of edges) {
    g.setEdge(edge.source, edge.target);
  }
  layout(g);

  return nodes.map((node) => {
    const pos = g.node(node.id);
    const width = node.measured?.width ?? nodeWidth;
    const height = node.measured?.height ?? nodeHeight;
    return { ...node, position: { x: pos.x - width / 2, y: pos.y - height / 2 } };
  });
}
