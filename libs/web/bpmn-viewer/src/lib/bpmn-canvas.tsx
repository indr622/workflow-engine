import 'bpmn-js/dist/assets/diagram-js.css';
import 'bpmn-js/dist/assets/bpmn-js.css';
import 'bpmn-js/dist/assets/bpmn-font/css/bpmn.css';

import { useEffect, useImperativeHandle, useRef, type Ref } from 'react';
import BpmnModeler from 'bpmn-js/lib/Modeler';
import BpmnViewer from 'bpmn-js/lib/NavigatedViewer';
import { cn } from '@workflow/ui';

/** Diagram BPMN kosong dengan satu start event, kompatibel Camunda Modeler. */
export const EMPTY_DIAGRAM = `<?xml version="1.0" encoding="UTF-8"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL"
  xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI"
  xmlns:dc="http://www.omg.org/spec/DD/20100524/DC"
  xmlns:zeebe="http://camunda.org/schema/zeebe/1.0"
  xmlns:modeler="http://camunda.org/schema/modeler/1.0"
  id="Definitions_1" targetNamespace="http://bpmn.io/schema/bpmn"
  modeler:executionPlatform="Camunda Cloud" modeler:executionPlatformVersion="8.8.0">
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

/** Class marker yang disediakan tema kanvas. */
export type MarkerClass = 'issue-error' | 'issue-warning';

/** BpmnCanvasHandle adalah API imperatif kanvas untuk fitur di sekitarnya. */
export interface BpmnCanvasHandle {
  /** Mengembalikan XML diagram saat ini (hanya mode modeler yang bisa berubah). */
  saveXML(): Promise<string>;
  /** Memuat ulang diagram dari XML. */
  importXML(xml: string): Promise<void>;
  /** Mengganti seluruh marker kelas tertentu dengan daftar id elemen baru. */
  setMarkers(markerClass: MarkerClass, elementIds: string[]): void;
  /** Memilih elemen dan menggesernya ke tengah kanvas; aman bila id tidak ada. */
  selectElement(elementId: string): void;
}

export interface BpmnCanvasProps {
  /** XML BPMN 2.0 yang ditampilkan. */
  xml?: string;
  /** "modeler" untuk mengedit, "viewer" untuk tampilan baca saja (Operate). */
  mode?: 'modeler' | 'viewer';
  className?: string;
  onError?: (error: Error) => void;
  ref?: Ref<BpmnCanvasHandle>;
}

type Instance = InstanceType<typeof BpmnModeler> | InstanceType<typeof BpmnViewer>;

/**
 * BpmnCanvas membungkus bpmn-js untuk React.
 * BPMN XML adalah satu-satunya format diagram yang dikenal engine.
 */
export function BpmnCanvas({ xml = EMPTY_DIAGRAM, mode = 'modeler', className, onError, ref }: BpmnCanvasProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const instanceRef = useRef<Instance | null>(null);
  const markersRef = useRef<Record<MarkerClass, string[]>>({ 'issue-error': [], 'issue-warning': [] });

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return undefined;

    const Ctor = mode === 'modeler' ? BpmnModeler : BpmnViewer;
    const instance = new Ctor({ container });
    instanceRef.current = instance;
    instance.importXML(xml).catch((e: unknown) => onError?.(e instanceof Error ? e : new Error(String(e))));

    return () => {
      instance.destroy();
      instanceRef.current = null;
    };
  }, [xml, mode, onError]);

  useImperativeHandle(
    ref,
    () => ({
      async saveXML() {
        const instance = instanceRef.current;
        if (!instance) throw new Error('BPMN canvas is not mounted');
        const { xml: out } = await instance.saveXML({ format: true });
        return out ?? '';
      },
      async importXML(next: string) {
        await instanceRef.current?.importXML(next);
      },
      setMarkers(markerClass, elementIds) {
        const instance = instanceRef.current;
        if (!instance) return;
        const canvas = instance.get('canvas') as {
          addMarker(id: string, cls: string): void;
          removeMarker(id: string, cls: string): void;
        };
        const registry = instance.get('elementRegistry') as { get(id: string): unknown };
        for (const id of markersRef.current[markerClass]) {
          if (registry.get(id)) canvas.removeMarker(id, markerClass);
        }
        const applied: string[] = [];
        for (const id of elementIds) {
          if (registry.get(id)) {
            canvas.addMarker(id, markerClass);
            applied.push(id);
          }
        }
        markersRef.current[markerClass] = applied;
      },
      selectElement(elementId) {
        const instance = instanceRef.current;
        if (!instance) return;
        const registry = instance.get('elementRegistry') as { get(id: string): unknown };
        const element = registry.get(elementId);
        if (!element) return;
        (instance.get('selection') as { select(el: unknown): void }).select(element);
        (instance.get('canvas') as { scrollToElement(el: unknown): void }).scrollToElement(element);
      },
    }),
    [],
  );

  return <div ref={containerRef} className={cn('bpmn-canvas h-full w-full rounded-xl border bg-white', className)} />;
}
