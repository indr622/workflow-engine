import {
  isActiveElementState,
  isFinalInstance,
  isOpenJob,
  type ElementInstance,
  type Incident,
} from './process-instance';
import { activeTokenBadges, elementIdsWithMarker, tokenStates } from './token-markers';
import { parseVariablesInput } from './variables-input';

const ei = (overrides: Partial<ElementInstance>): ElementInstance => ({
  key: 'k',
  flowScopeKey: 'root',
  elementId: 'task',
  elementType: 'SERVICE_TASK',
  state: 'completed',
  incidentKey: '',
  startTime: null,
  endTime: null,
  ...overrides,
});

const incident = (overrides: Partial<Incident>): Incident => ({
  key: 'inc-1',
  elementInstanceKey: 'k',
  elementId: 'gw',
  errorType: 'CONDITION_ERROR',
  errorMessage: 'no outgoing flow',
  resolved: false,
  createdAt: null,
  ...overrides,
});

describe('token marker rules', () => {
  it('marks completed, active and counts parallel tokens per element', () => {
    const states = tokenStates(
      [
        ei({ key: '1', elementId: 'start', elementType: 'START_EVENT' }),
        ei({ key: '2', elementId: 'fork', elementType: 'PARALLEL_GATEWAY' }),
        ei({ key: '3', elementId: 'task', state: 'activated' }),
        ei({ key: '4', elementId: 'task', state: 'activated' }),
        ei({ key: '5', elementId: 'task', state: 'completed' }),
      ],
      [],
    );
    expect(states).toEqual({
      start: { marker: 'token-completed', activeCount: 0 },
      fork: { marker: 'token-completed', activeCount: 0 },
      task: { marker: 'token-active', activeCount: 2 },
    });
    expect(elementIdsWithMarker(states, 'token-completed')).toEqual(['fork', 'start']);
    expect(activeTokenBadges(states)).toEqual({ task: 2 });
  });

  it('gives incidents priority and ignores resolved ones', () => {
    const states = tokenStates(
      [
        ei({ key: '1', elementId: 'gw', elementType: 'EXCLUSIVE_GATEWAY', state: 'activated', incidentKey: 'inc-1' }),
        ei({ key: '2', elementId: 'old', state: 'completed' }),
      ],
      [incident({}), incident({ key: 'inc-0', elementId: 'old', resolved: true })],
    );
    expect(states['gw']).toEqual({ marker: 'token-incident', activeCount: 1 });
    expect(states['old']?.marker).toBe('token-completed');
  });

  it('marks an incident referenced only by an active element instance', () => {
    const states = tokenStates([ei({ elementId: 'task', state: 'activated', incidentKey: 'unknown' })], []);
    expect(states['task']?.marker).toBe('token-incident');
  });

  it('marks incident elements without element instances', () => {
    expect(tokenStates([], [incident({ elementId: 'gw' })])).toEqual({
      gw: { marker: 'token-incident', activeCount: 0 },
    });
  });

  it('skips the process root scope', () => {
    const states = tokenStates([ei({ elementId: 'order', elementType: 'PROCESS', state: 'activated' })], []);
    expect(states).toEqual({});
  });
});

describe('process instance predicates', () => {
  it('classifies element, job and instance states', () => {
    expect(isActiveElementState('activated')).toBe(true);
    expect(isActiveElementState('terminated')).toBe(false);
    expect(
      isOpenJob({ key: 'j', type: 't', elementId: 'e', elementInstanceKey: 'k', retries: 3, state: 'CREATED' }),
    ).toBe(true);
    const base = {
      key: 'p',
      definitionKey: 'd',
      bpmnProcessId: 'order',
      version: 1,
      hasIncident: false,
      startTime: null,
      endTime: null,
    };
    expect(isFinalInstance({ ...base, state: 'active' })).toBe(false);
    expect(isFinalInstance({ ...base, state: 'terminated' })).toBe(true);
  });
});

describe('variables input', () => {
  it('accepts empty text and JSON objects', () => {
    expect(parseVariablesInput('  ')).toEqual({ ok: true, json: '{}' });
    expect(parseVariablesInput('{ "amount": 100, "items": [1] }')).toEqual({
      ok: true,
      json: '{"amount":100,"items":[1]}',
    });
  });

  it('rejects invalid JSON, arrays and primitives', () => {
    expect(parseVariablesInput('{amount: 1}').ok).toBe(false);
    expect(parseVariablesInput('[1, 2]').ok).toBe(false);
    expect(parseVariablesInput('42').ok).toBe(false);
    expect(parseVariablesInput('null').ok).toBe(false);
  });
});
