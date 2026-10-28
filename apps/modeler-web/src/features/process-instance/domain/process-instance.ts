/**
 * Domain process instance di sisi UI. Murni TypeScript: tanpa React, tanpa transport.
 * Semua key adalah string UUIDv7 (ADR-0011). Fitur ini akan dipindah ke operate-web di Fase 6,
 * jadi domain dan infrastructure sengaja tidak bergantung pada fitur lain.
 */

export type ProcessInstanceState = 'active' | 'completed' | 'terminated' | 'unspecified';

export type ElementInstanceState =
  'activating' | 'activated' | 'completing' | 'completed' | 'terminating' | 'terminated' | 'unspecified';

export interface ProcessInstance {
  key: string;
  definitionKey: string;
  bpmnProcessId: string;
  version: number;
  state: ProcessInstanceState;
  hasIncident: boolean;
  startTime: Date | null;
  endTime: Date | null;
}

export interface ElementInstance {
  key: string;
  flowScopeKey: string;
  elementId: string;
  /** Tipe elemen BPMN dari engine, misalnya "SERVICE_TASK" atau "PROCESS". */
  elementType: string;
  state: ElementInstanceState;
  /** Kosong bila tidak ada incident. */
  incidentKey: string;
  startTime: Date | null;
  endTime: Date | null;
}

export interface Variable {
  scopeKey: string;
  name: string;
  /** Nilai dalam JSON. */
  value: string;
}

export interface Incident {
  key: string;
  elementInstanceKey: string;
  elementId: string;
  errorType: string;
  errorMessage: string;
  resolved: boolean;
  createdAt: Date | null;
}

export interface Job {
  key: string;
  type: string;
  elementId: string;
  elementInstanceKey: string;
  retries: number;
  /** CREATED, COMPLETED, CANCELED (string di Fase 2, lihat rencana 7.2). */
  state: string;
}

export interface ProcessInstanceDetail {
  instance: ProcessInstance;
  elementInstances: ElementInstance[];
  variables: Variable[];
  incidents: Incident[];
  jobs: Job[];
}

/** Definisi yang dijalankan: key versi tertentu, atau bpmnProcessId dengan versi (0 = terbaru). */
export type DefinitionSelector =
  { kind: 'key'; definitionKey: string } | { kind: 'bpmnProcess'; bpmnProcessId: string; version: number };

export interface CreateInstanceInput {
  definition: DefinitionSelector;
  /** Objek JSON; string kosong berarti {}. */
  variables: string;
}

export interface CreatedInstance {
  key: string;
  definitionKey: string;
  bpmnProcessId: string;
  version: number;
  state: ProcessInstanceState;
}

export interface ListInstancesParams {
  bpmnProcessId?: string;
  definitionKey?: string;
  state?: ProcessInstanceState;
  onlyWithIncident?: boolean;
  pageSize?: number;
  pageToken?: string;
}

export interface InstancesPage {
  items: ProcessInstance[];
  nextPageToken: string;
}

/** Port keluar fitur process-instance; diimplementasikan di infrastructure. */
export interface ProcessInstanceRepository {
  create(input: CreateInstanceInput): Promise<CreatedInstance>;
  get(key: string): Promise<ProcessInstanceDetail>;
  list(params: ListInstancesParams): Promise<InstancesPage>;
  cancel(key: string): Promise<void>;
  completeJob(jobKey: string, variables: string): Promise<void>;
}

/** isActiveElementState: element instance masih memegang token. */
export function isActiveElementState(state: ElementInstanceState): boolean {
  return state === 'activating' || state === 'activated' || state === 'completing' || state === 'terminating';
}

/** isOpenJob: job masih menunggu dikerjakan worker (atau diselesaikan manual). */
export function isOpenJob(job: Job): boolean {
  return job.state === 'CREATED';
}

/** isFinalInstance: instance tidak akan berubah lagi, polling boleh berhenti. */
export function isFinalInstance(instance: ProcessInstance): boolean {
  return instance.state === 'completed' || instance.state === 'terminated';
}
