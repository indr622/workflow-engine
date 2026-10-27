import { create } from 'zustand';
import { sortIssues, type ValidationIssue } from '../domain/deployment';

/**
 * State klien fitur deployment: issue hasil deploy/validasi terakhir dan issue yang dipilih.
 * Data dari engine (daftar definition) tidak disimpan di sini, tetapi di TanStack Query.
 */
interface DeploymentUiState {
  issues: ValidationIssue[];
  selectedIssue: ValidationIssue | null;
  lastDeploymentKey: string | null;
  setIssues: (issues: ValidationIssue[]) => void;
  selectIssue: (issue: ValidationIssue | null) => void;
  setLastDeploymentKey: (key: string | null) => void;
  clear: () => void;
}

export const useDeploymentUiStore = create<DeploymentUiState>()((set) => ({
  issues: [],
  selectedIssue: null,
  lastDeploymentKey: null,
  setIssues: (issues) => set({ issues: sortIssues(issues), selectedIssue: null }),
  selectIssue: (issue) => set({ selectedIssue: issue }),
  setLastDeploymentKey: (key) => set({ lastDeploymentKey: key }),
  clear: () => set({ issues: [], selectedIssue: null }),
}));
