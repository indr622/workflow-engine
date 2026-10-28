import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { BpmnCanvas, type BpmnCanvasHandle } from '@workflow/bpmn-viewer';
import { useInstanceUiStore } from '../application/instance.store';
import type { ProcessInstanceDetail } from '../domain/process-instance';
import { activeTokenBadges, elementIdsWithMarker, TOKEN_MARKERS, tokenStates } from '../domain/token-markers';

export interface InstanceViewerProps {
  /** XML BPMN definition yang dijalankan instance. */
  xml: string;
  detail: ProcessInstanceDetail;
}

/**
 * InstanceViewer menggambar diagram definition (bpmn-js viewer) dengan marker status token
 * (aktif/selesai/incident) dan badge jumlah token aktif per elemen. Klik elemen memilihnya.
 */
export function InstanceViewer({ xml, detail }: InstanceViewerProps) {
  const canvasRef = useRef<BpmnCanvasHandle>(null);
  const [hasLayout, setHasLayout] = useState(true);
  const selected = useInstanceUiStore((s) => s.selectedElementId);
  const selectElement = useInstanceUiStore((s) => s.selectElement);

  const states = useMemo(
    () => tokenStates(detail.elementInstances, detail.incidents),
    [detail.elementInstances, detail.incidents],
  );

  useEffect(() => {
    for (const marker of TOKEN_MARKERS) {
      canvasRef.current?.setMarkers(marker, elementIdsWithMarker(states, marker));
    }
    canvasRef.current?.setBadges('token-count', activeTokenBadges(states));
  }, [states]);

  useEffect(() => {
    if (selected) canvasRef.current?.selectElement(selected);
  }, [selected]);

  const onImported = useCallback((info: { hasLayout: boolean }) => setHasLayout(info.hasLayout), []);

  return (
    <div className="space-y-2">
      {!hasLayout && (
        <p className="text-sm text-muted-foreground">
          This definition has no diagram layout (BPMNDI); token state is listed below instead.
        </p>
      )}
      <div className="h-[480px]">
        <BpmnCanvas ref={canvasRef} mode="viewer" xml={xml} onImported={onImported} onElementClick={selectElement} />
      </div>
      <div className="flex flex-wrap gap-4 text-xs text-muted-foreground">
        <Legend className="border-green-600 bg-green-50" label="active token" />
        <Legend className="border-neutral-500 bg-neutral-100" label="completed" />
        <Legend className="border-red-600 bg-red-50" label="incident" />
      </div>
    </div>
  );
}

function Legend({ className, label }: { className: string; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className={`inline-block size-3 rounded-sm border-2 ${className}`} />
      {label}
    </span>
  );
}
