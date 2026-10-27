/**
 * Domain deployment di sisi UI. Murni TypeScript: tanpa React, tanpa transport.
 */

export type IssueSeverity = 'error' | 'warning';

export interface ValidationIssue {
  code: string;
  severity: IssueSeverity;
  message: string;
  resourceName: string;
  processId: string;
  elementId: string;
  line: number;
  column: number;
}

export interface ProcessDefinition {
  /** Key int64 ditampilkan sebagai string agar presisi tidak hilang. */
  key: string;
  bpmnProcessId: string;
  version: number;
  versionTag: string;
  name: string;
  resourceName: string;
  deploymentKey: string;
  createdAt: Date;
  duplicate: boolean;
}

export interface DeployOutcome {
  deploymentKey: string;
  definitions: ProcessDefinition[];
  warnings: ValidationIssue[];
}

export interface ValidateOutcome {
  valid: boolean;
  issues: ValidationIssue[];
}

export interface ResourceInput {
  name: string;
  content: string;
}

export interface ListDefinitionsParams {
  bpmnProcessId?: string;
  latestOnly?: boolean;
  pageSize?: number;
  pageToken?: string;
}

export interface DefinitionsPage {
  items: ProcessDefinition[];
  nextPageToken: string;
}

/** DeploymentRejectedError: engine menolak deploy; membawa semua issue. */
export class DeploymentRejectedError extends Error {
  constructor(readonly issues: ValidationIssue[]) {
    super(`deployment rejected with ${issues.filter((i) => i.severity === 'error').length} error(s)`);
    this.name = 'DeploymentRejectedError';
  }
}

/** Port keluar fitur deployment; diimplementasikan di infrastructure. */
export interface DeploymentRepository {
  deploy(resources: ResourceInput[]): Promise<DeployOutcome>;
  validate(resources: ResourceInput[]): Promise<ValidateOutcome>;
  list(params: ListDefinitionsParams): Promise<DefinitionsPage>;
  get(key: string): Promise<{ definition: ProcessDefinition; bpmnXml: string }>;
}

/** blockingIssues mengembalikan issue yang membuat deploy ditolak. */
export function blockingIssues(issues: ValidationIssue[]): ValidationIssue[] {
  return issues.filter((i) => i.severity === 'error');
}

/** elementIdsWithIssues mengembalikan id elemen unik yang punya issue dengan severity tertentu. */
export function elementIdsWithIssues(issues: ValidationIssue[], severity: IssueSeverity): string[] {
  return [...new Set(issues.filter((i) => i.severity === severity && i.elementId).map((i) => i.elementId))];
}

/** sortIssues: error dulu, lalu urut posisi di file. */
export function sortIssues(issues: ValidationIssue[]): ValidationIssue[] {
  return [...issues].sort((a, b) => {
    if (a.severity !== b.severity) return a.severity === 'error' ? -1 : 1;
    return a.line - b.line || a.column - b.column || a.code.localeCompare(b.code);
  });
}
