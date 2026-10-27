import type { Edge, Node } from '@xyflow/react';
import { layoutGraph } from './layout';

describe('layoutGraph', () => {
  it('places a target node to the right of its source in LR direction', () => {
    const nodes: Node[] = [
      { id: 'a', position: { x: 0, y: 0 }, data: {} },
      { id: 'b', position: { x: 0, y: 0 }, data: {} },
    ];
    const edges: Edge[] = [{ id: 'a-b', source: 'a', target: 'b' }];

    const [a, b] = layoutGraph(nodes, edges, { direction: 'LR' });

    expect(b?.position.x).toBeGreaterThan(a?.position.x ?? Infinity);
    expect(nodes[0]?.position).toEqual({ x: 0, y: 0 });
  });
});
