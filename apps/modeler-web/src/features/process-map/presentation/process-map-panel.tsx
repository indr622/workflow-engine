import { Network } from 'lucide-react';
import { Button, Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@workflow/ui';
import { FlowCanvas } from '@workflow/flow-canvas';
import { useProcessMapStore } from '../application/process-map.store';

/** ProcessMapPanel menggambar peta dependensi proses dengan React Flow. */
export function ProcessMapPanel() {
  const { nodes, edges, onNodesChange, onEdgesChange, relayout } = useProcessMapStore();

  return (
    <Card className="h-full">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Network className="size-4" /> Process map
        </CardTitle>
        <CardDescription>React Flow: graf non-BPMN (call activity dan decision)</CardDescription>
        <CardAction>
          <Button variant="outline" size="sm" onClick={relayout}>
            Auto layout
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="h-[420px]">
        <FlowCanvas nodes={nodes} edges={edges} onNodesChange={onNodesChange} onEdgesChange={onEdgesChange} />
      </CardContent>
    </Card>
  );
}
