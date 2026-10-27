import { sampleProcessMap } from '../domain/process-map';
import { toGraph } from './process-map.store';

describe('toGraph', () => {
  it('creates one card node per process and one edge per link', () => {
    const { nodes, edges } = toGraph(sampleProcessMap());
    expect(nodes).toHaveLength(4);
    expect(edges.map((e) => e.id)).toContain('order-payment');
    expect(nodes.every((n) => n.type === 'card')).toBe(true);
  });
});
