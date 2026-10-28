import { isActiveElementState, type ElementInstance, type Incident } from './process-instance';

/** Class marker status token; sama dengan MarkerClass token-* di @workflow/bpmn-viewer. */
export type TokenMarker = 'token-active' | 'token-completed' | 'token-incident';

export const TOKEN_MARKERS: TokenMarker[] = ['token-active', 'token-completed', 'token-incident'];

export interface ElementTokenState {
  marker: TokenMarker;
  /** Jumlah element instance yang masih aktif pada elemen ini. */
  activeCount: number;
}

/**
 * Element instance tipe PROCESS (scope root) tidak digambar sebagai marker: id-nya sama dengan root
 * plane bpmn-js sehingga marker akan mewarnai seluruh diagram.
 */
const ROOT_ELEMENT_TYPES = new Set(['PROCESS']);

/**
 * tokenStates menghitung status token per id elemen dari element instance dan incident.
 * Prioritas: incident (belum resolved) > aktif > selesai. Elemen yang belum pernah dilewati token
 * tidak muncul di hasil.
 */
export function tokenStates(
  elementInstances: ElementInstance[],
  incidents: Incident[],
): Record<string, ElementTokenState> {
  const knownIncidents = new Map(incidents.map((i) => [i.key, i]));
  const incidentElements = new Set<string>();
  for (const incident of incidents) {
    if (!incident.resolved && incident.elementId) incidentElements.add(incident.elementId);
  }

  const result: Record<string, ElementTokenState> = {};
  for (const ei of elementInstances) {
    if (!ei.elementId || ROOT_ELEMENT_TYPES.has(ei.elementType)) continue;
    const entry = (result[ei.elementId] ??= { marker: 'token-completed', activeCount: 0 });
    if (!isActiveElementState(ei.state)) continue;
    entry.activeCount += 1;
    // Element instance aktif yang menunjuk incident terbuka (atau yang tidak ada di daftar) = incident.
    const incident = ei.incidentKey ? knownIncidents.get(ei.incidentKey) : undefined;
    if (ei.incidentKey && (!incident || !incident.resolved)) incidentElements.add(ei.elementId);
  }

  for (const [elementId, entry] of Object.entries(result)) {
    if (incidentElements.has(elementId)) entry.marker = 'token-incident';
    else if (entry.activeCount > 0) entry.marker = 'token-active';
  }
  // Incident pada elemen tanpa element instance (data parsial) tetap ditandai.
  for (const elementId of incidentElements) {
    result[elementId] ??= { marker: 'token-incident', activeCount: 0 };
  }
  return result;
}

/** elementIdsWithMarker mengembalikan id elemen dengan marker tertentu, terurut. */
export function elementIdsWithMarker(states: Record<string, ElementTokenState>, marker: TokenMarker): string[] {
  return Object.entries(states)
    .filter(([, s]) => s.marker === marker)
    .map(([id]) => id)
    .sort();
}

/** activeTokenBadges: id elemen → jumlah token aktif, hanya untuk elemen yang punya token aktif. */
export function activeTokenBadges(states: Record<string, ElementTokenState>): Record<string, number> {
  return Object.fromEntries(
    Object.entries(states)
      .filter(([, s]) => s.activeCount > 0)
      .map(([id, s]) => [id, s.activeCount]),
  );
}
