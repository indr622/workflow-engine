import '@xyflow/react/dist/style.css';
import {
  Background,
  Controls,
  ReactFlow,
  type Edge,
  type Node,
  type NodeTypes,
  type OnEdgesChange,
  type OnNodesChange,
} from '@xyflow/react';
import { cn } from '@workflow/ui';
import { CardNode } from './card-node';

const defaultNodeTypes: NodeTypes = { card: CardNode };

export interface FlowCanvasProps<N extends Node = Node, E extends Edge = Edge> {
  nodes: N[];
  edges: E[];
  onNodesChange?: OnNodesChange<N>;
  onEdgesChange?: OnEdgesChange<E>;
  nodeTypes?: NodeTypes;
  className?: string;
  readOnly?: boolean;
}

/**
 * FlowCanvas membungkus React Flow dengan tema shadcn.
 * Aplikasi tidak meng-import @xyflow/react langsung; state node/edge dikelola store fitur (Zustand).
 */
export function FlowCanvas<N extends Node = Node, E extends Edge = Edge>({
  nodes,
  edges,
  onNodesChange,
  onEdgesChange,
  nodeTypes,
  className,
  readOnly = false,
}: FlowCanvasProps<N, E>) {
  return (
    <div className={cn('h-full w-full rounded-xl border bg-background', className)}>
      <ReactFlow<N, E>
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        nodeTypes={{ ...defaultNodeTypes, ...nodeTypes }}
        nodesDraggable={!readOnly}
        nodesConnectable={!readOnly}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        proOptions={{ hideAttribution: true }}
      >
        <Background gap={16} />
        <Controls showInteractive={!readOnly} />
      </ReactFlow>
    </div>
  );
}
