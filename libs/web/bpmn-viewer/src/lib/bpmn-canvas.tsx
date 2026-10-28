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
export type MarkerClass = 'issue-error' | 'issue-warning' | 'token-active' | 'token-completed' | 'token-incident';

const MARKER_CLASSES: MarkerClass[] = [
  'issue-error',
  'issue-warning',
  'token-active',
  'token-completed',
  'token-incident',
];

/** Jenis badge overlay yang disediakan tema kanvas. */
export type BadgeType = 'token-count';

const BADGE_TYPES: BadgeType[] = ['token-count'];

/** BpmnCanvasHandle adalah API imperatif kanvas untuk fitur di sekitarnya. */
export interface BpmnCanvasHandle {
  /** Mengembalikan XML diagram saat ini (hanya mode modeler yang bisa berubah). */
  saveXML(): Promise<string>;
  /** Memuat ulang diagram dari XML. */
  importXML(xml: string): Promise<void>;
  /** Mengganti seluruh marker kelas tertentu dengan daftar id elemen baru. */
  setMarkers(markerClass: MarkerClass, elementIds: string[]): void;
  /** Mengganti seluruh badge overlay jenis tertentu; key = id elemen, value = teks badge. */
  setBadges(badgeType: BadgeType, badges: Record<string, string | number>): void;
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
  /** Dipanggil setelah import; hasLayout=false bila XML tidak punya diagram interchange (BPMNDI). */
  onImported?: (info: { hasLayout: boolean; warnings: number }) => void;
  /** Dipanggil saat pengguna mengklik elemen diagram (bukan root plane). */
  onElementClick?: (elementId: string) => void;
  ref?: Ref<BpmnCanvasHandle>;
}

type Instance = InstanceType<typeof BpmnModeler> | InstanceType<typeof BpmnViewer>;

interface CanvasService {
  addMarker(id: string, cls: string): void;
  removeMarker(id: string, cls: string): void;
  getRootElement(): { id: string };
}
interface RegistryService {
  get(id: string): unknown;
  getAll(): unknown[];
}
interface OverlaysService {
  add(id: string, type: string, overlay: { position: Record<string, number>; html: HTMLElement }): string;
  remove(filter: { type: string }): void;
}

/** applyMarkers memasang marker untuk id yang ada di diagram; id yang tidak dikenal diabaikan. */
function applyMarkers(instance: Instance, markerClass: MarkerClass, add: string[], remove: string[]) {
  const canvas = instance.get('canvas') as CanvasService;
  const registry = instance.get('elementRegistry') as RegistryService;
  for (const id of remove) {
    if (registry.get(id)) canvas.removeMarker(id, markerClass);
  }
  for (const id of add) {
    if (registry.get(id)) canvas.addMarker(id, markerClass);
  }
}

/** applyBadges mengganti semua overlay jenis tertentu dengan badge baru. */
function applyBadges(instance: Instance, badgeType: BadgeType, badges: Record<string, string | number>) {
  const overlays = instance.get('overlays') as OverlaysService;
  const registry = instance.get('elementRegistry') as RegistryService;
  overlays.remove({ type: badgeType });
  for (const [id, text] of Object.entries(badges)) {
    if (!registry.get(id)) continue;
    const html = document.createElement('span');
    html.className = `bpmn-badge bpmn-badge-${badgeType}`;
    html.textContent = String(text);
    overlays.add(id, badgeType, { position: { bottom: 12, left: -8 }, html });
  }
}

/** reapplyAll memasang ulang semua marker dan badge yang diminta, dipakai setelah import. */
function reapplyAll(
  instance: Instance,
  markers: Record<MarkerClass, string[]>,
  badges: Record<BadgeType, Record<string, string | number>>,
) {
  for (const cls of MARKER_CLASSES) applyMarkers(instance, cls, markers[cls], []);
  for (const type of BADGE_TYPES) applyBadges(instance, type, badges[type]);
}

/**
 * BpmnCanvas membungkus bpmn-js untuk React.
 * BPMN XML adalah satu-satunya format diagram yang dikenal engine.
 */
export function BpmnCanvas({
  xml = EMPTY_DIAGRAM,
  mode = 'modeler',
  className,
  onError,
  onImported,
  onElementClick,
  ref,
}: BpmnCanvasProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const instanceRef = useRef<Instance | null>(null);
  // Marker dan badge yang diminta disimpan agar bisa dipasang ulang setelah import (XML berubah).
  const markersRef = useRef<Record<MarkerClass, string[]>>(
    Object.fromEntries(MARKER_CLASSES.map((c) => [c, [] as string[]])) as Record<MarkerClass, string[]>,
  );
  const badgesRef = useRef<Record<BadgeType, Record<string, string | number>>>(
    Object.fromEntries(BADGE_TYPES.map((t) => [t, {}])) as Record<BadgeType, Record<string, string | number>>,
  );
  const onElementClickRef = useRef(onElementClick);
  useEffect(() => {
    onElementClickRef.current = onElementClick;
  }, [onElementClick]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return undefined;

    const Ctor = mode === 'modeler' ? BpmnModeler : BpmnViewer;
    const instance = new Ctor({ container });
    instanceRef.current = instance;
    const eventBus = instance.get('eventBus') as {
      on(event: string, cb: (e: { element: { id: string } }) => void): void;
    };
    eventBus.on('element.click', (e) => {
      const root = (instance.get('canvas') as CanvasService).getRootElement();
      if (e.element.id !== root.id) onElementClickRef.current?.(e.element.id);
    });
    instance
      .importXML(xml)
      .then(({ warnings }) => {
        // Tanpa BPMNDI, registry hanya berisi root plane: tidak ada shape yang bisa digambar.
        const registry = instance.get('elementRegistry') as RegistryService;
        reapplyAll(instance, markersRef.current, badgesRef.current);
        onImported?.({ hasLayout: registry.getAll().length > 1, warnings: warnings.length });
      })
      .catch((e: unknown) => onError?.(e instanceof Error ? e : new Error(String(e))));

    return () => {
      instance.destroy();
      instanceRef.current = null;
    };
  }, [xml, mode, onError, onImported]);

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
        const instance = instanceRef.current;
        if (!instance) return;
        await instance.importXML(next);
        reapplyAll(instance, markersRef.current, badgesRef.current);
      },
      setMarkers(markerClass, elementIds) {
        const previous = markersRef.current[markerClass];
        markersRef.current[markerClass] = [...elementIds];
        const instance = instanceRef.current;
        if (instance) applyMarkers(instance, markerClass, elementIds, previous);
      },
      setBadges(badgeType, badges) {
        badgesRef.current[badgeType] = { ...badges };
        const instance = instanceRef.current;
        if (instance) applyBadges(instance, badgeType, badges);
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
