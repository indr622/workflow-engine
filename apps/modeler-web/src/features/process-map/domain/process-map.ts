/**
 * Domain peta dependensi process definition (proses induk memanggil sub-proses lewat call activity).
 * Murni TypeScript, tanpa React maupun React Flow.
 */

export type ProcessKind = 'process' | 'call-activity' | 'decision';

export interface ProcessRef {
  id: string;
  name: string;
  kind: ProcessKind;
  version: number;
}

export interface ProcessLink {
  from: string;
  to: string;
}

export interface ProcessMap {
  processes: ProcessRef[];
  links: ProcessLink[];
}

/** sampleProcessMap adalah data contoh Fase 0 sampai engine menyediakan API deployment. */
export function sampleProcessMap(): ProcessMap {
  return {
    processes: [
      { id: 'order', name: 'Order fulfillment', kind: 'process', version: 3 },
      { id: 'payment', name: 'Payment', kind: 'call-activity', version: 2 },
      { id: 'shipping', name: 'Shipping', kind: 'call-activity', version: 5 },
      { id: 'risk', name: 'Risk check', kind: 'decision', version: 1 },
    ],
    links: [
      { from: 'order', to: 'payment' },
      { from: 'order', to: 'shipping' },
      { from: 'payment', to: 'risk' },
    ],
  };
}
