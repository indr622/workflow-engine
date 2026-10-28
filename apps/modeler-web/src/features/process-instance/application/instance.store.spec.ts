import { useInstanceUiStore } from './instance.store';
import { pollIntervalFor } from './process-instance.queries';
import type { ProcessInstanceDetail, ProcessInstanceState } from '../domain/process-instance';

describe('instance ui store', () => {
  beforeEach(() => useInstanceUiStore.getState().clear());

  it('selects, toggles and clears the selected element', () => {
    const store = useInstanceUiStore.getState();
    store.selectElement('task');
    expect(useInstanceUiStore.getState().selectedElementId).toBe('task');

    store.toggleElement('gw');
    expect(useInstanceUiStore.getState().selectedElementId).toBe('gw');
    store.toggleElement('gw');
    expect(useInstanceUiStore.getState().selectedElementId).toBeNull();

    store.selectElement('task');
    store.clear();
    expect(useInstanceUiStore.getState().selectedElementId).toBeNull();
  });
});

describe('instance polling', () => {
  const detail = (state: ProcessInstanceState): ProcessInstanceDetail => ({
    instance: {
      key: 'p',
      definitionKey: 'd',
      bpmnProcessId: 'order',
      version: 1,
      state,
      hasIncident: false,
      startTime: null,
      endTime: null,
    },
    elementInstances: [],
    variables: [],
    incidents: [],
    jobs: [],
  });

  it('polls every second while the instance is active only', () => {
    expect(pollIntervalFor(detail('active'))).toBe(1000);
    expect(pollIntervalFor(detail('completed'))).toBe(false);
    expect(pollIntervalFor(detail('terminated'))).toBe(false);
    expect(pollIntervalFor(undefined)).toBe(false);
  });
});
