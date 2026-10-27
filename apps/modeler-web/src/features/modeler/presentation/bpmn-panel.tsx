import { useEffect, useRef } from 'react';
import { Workflow } from 'lucide-react';
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@workflow/ui';
import { BpmnCanvas, type BpmnCanvasHandle } from '@workflow/bpmn-viewer';
import { DeployToolbar, IssuesPanel, elementIdsWithIssues, useDeploymentUiStore } from '../../deployment';

/**
 * BpmnPanel menampilkan kanvas bpmn-js (satu-satunya editor BPMN XML) dengan toolbar deploy.
 * Issue dari fitur deployment digambar sebagai marker; issue terpilih disorot di kanvas.
 */
export function BpmnPanel() {
  const canvasRef = useRef<BpmnCanvasHandle>(null);
  const issues = useDeploymentUiStore((s) => s.issues);
  const selected = useDeploymentUiStore((s) => s.selectedIssue);

  useEffect(() => {
    canvasRef.current?.setMarkers('issue-error', elementIdsWithIssues(issues, 'error'));
    canvasRef.current?.setMarkers('issue-warning', elementIdsWithIssues(issues, 'warning'));
  }, [issues]);

  useEffect(() => {
    if (selected?.elementId) {
      canvasRef.current?.selectElement(selected.elementId);
    }
  }, [selected]);

  return (
    <Card className="h-full">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Workflow className="size-4" /> BPMN modeler
        </CardTitle>
        <CardDescription>bpmn-js: BPMN 2.0 XML yang kompatibel Camunda Modeler</CardDescription>
        <CardAction>
          <DeployToolbar getXml={() => canvasRef.current?.saveXML() ?? Promise.reject(new Error('canvas not ready'))} />
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="h-[420px]">
          <BpmnCanvas ref={canvasRef} mode="modeler" />
        </div>
        <IssuesPanel />
      </CardContent>
    </Card>
  );
}
