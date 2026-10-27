import { useDeploymentUiStore } from './deployment.store';
import type { ValidationIssue } from '../domain/deployment';

const issue = (code: string, severity: ValidationIssue['severity'], line: number): ValidationIssue => ({
  code,
  severity,
  message: '',
  resourceName: 'a.bpmn',
  processId: 'p',
  elementId: 'e',
  line,
  column: 1,
});

describe('deployment ui store', () => {
  beforeEach(() => useDeploymentUiStore.getState().clear());

  it('stores issues sorted and resets selection', () => {
    const store = useDeploymentUiStore.getState();
    store.selectIssue(issue('OLD', 'error', 1));
    store.setIssues([issue('W', 'warning', 1), issue('E', 'error', 5)]);

    const state = useDeploymentUiStore.getState();
    expect(state.issues.map((i) => i.code)).toEqual(['E', 'W']);
    expect(state.selectedIssue).toBeNull();
  });
});
