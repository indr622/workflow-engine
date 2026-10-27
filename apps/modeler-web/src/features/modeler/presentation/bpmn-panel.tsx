import { Workflow } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@workflow/ui';
import { BpmnCanvas } from '@workflow/bpmn-viewer';

/** BpmnPanel menampilkan kanvas bpmn-js; satu-satunya editor BPMN XML. */
export function BpmnPanel() {
  return (
    <Card className="h-full">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Workflow className="size-4" /> BPMN modeler
        </CardTitle>
        <CardDescription>bpmn-js: BPMN 2.0 XML yang kompatibel Camunda Modeler</CardDescription>
      </CardHeader>
      <CardContent className="h-[420px]">
        <BpmnCanvas mode="modeler" />
      </CardContent>
    </Card>
  );
}
