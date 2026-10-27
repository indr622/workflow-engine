import { Handle, Position, type Node, type NodeProps } from '@xyflow/react';
import { Badge, Card, CardDescription, CardHeader, CardTitle, cn } from '@workflow/ui';

export type CardNodeData = {
  title: string;
  description?: string;
  status?: string;
  tone?: 'default' | 'success' | 'destructive' | 'secondary';
};

export type CardNodeType = Node<CardNodeData, 'card'>;

/** CardNode adalah node React Flow yang digambar dengan komponen Card shadcn. */
export function CardNode({ data, selected }: NodeProps<CardNodeType>) {
  return (
    <Card className={cn('w-[200px] gap-2 py-3 shadow-sm', selected && 'ring-2 ring-ring')}>
      <Handle type="target" position={Position.Left} />
      <CardHeader className="px-3">
        <CardTitle className="text-sm">{data.title}</CardTitle>
        {data.description && <CardDescription className="text-xs">{data.description}</CardDescription>}
        {data.status && (
          <Badge variant={data.tone ?? 'secondary'} className="mt-1">
            {data.status}
          </Badge>
        )}
      </CardHeader>
      <Handle type="source" position={Position.Right} />
    </Card>
  );
}
