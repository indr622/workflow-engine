import 'bpmn-js/dist/assets/diagram-js.css';
import 'bpmn-js/dist/assets/bpmn-js.css';
import 'bpmn-js/dist/assets/bpmn-font/css/bpmn.css';

import { useEffect, useRef } from 'react';
import BpmnModeler from 'bpmn-js/lib/Modeler';
import BpmnViewer from 'bpmn-js/lib/NavigatedViewer';
import { cn } from '@workflow/ui';

/** Diagram BPMN kosong dengan satu start event, kompatibel Camunda Modeler. */
export const EMPTY_DIAGRAM = `<?xml version="1.0" encoding="UTF-8"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL"
  xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI"
  xmlns:dc="http://www.omg.org/spec/DD/20100524/DC"
  id="Definitions_1" targetNamespace="http://bpmn.io/schema/bpmn">
  <bpmn:process id="Process_1" isExecutable="true">
    <bpmn:startEvent id="StartEvent_1" />
  </bpmn:process>
  <bpmndi:BPMNDiagram id="BPMNDiagram_1">
    <bpmndi:BPMNPlane id="BPMNPlane_1" bpmnElement="Process_1">
      <bpmndi:BPMNShape id="StartEvent_1_di" bpmnElement="StartEvent_1">
        <dc:Bounds x="180" y="160" width="36" height="36" />
      </bpmndi:BPMNShape>
    </bpmndi:BPMNPlane>
  </bpmndi:BPMNDiagram>
</bpmn:definitions>`;

export interface BpmnCanvasProps {
  /** XML BPMN 2.0 yang ditampilkan. */
  xml?: string;
  /** "modeler" untuk mengedit, "viewer" untuk tampilan baca saja (Operate). */
  mode?: 'modeler' | 'viewer';
  className?: string;
  onError?: (error: Error) => void;
}

/**
 * BpmnCanvas membungkus bpmn-js untuk React.
 * BPMN XML adalah satu-satunya format diagram yang dikenal engine.
 */
export function BpmnCanvas({ xml = EMPTY_DIAGRAM, mode = 'modeler', className, onError }: BpmnCanvasProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return undefined;

    const Ctor = mode === 'modeler' ? BpmnModeler : BpmnViewer;
    const instance = new Ctor({ container });
    instance.importXML(xml).catch((e: unknown) => onError?.(e instanceof Error ? e : new Error(String(e))));

    return () => instance.destroy();
  }, [xml, mode, onError]);

  return <div ref={containerRef} className={cn('h-full w-full rounded-xl border bg-white', className)} />;
}
