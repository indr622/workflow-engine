import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import {
  CancelProcessInstanceResponseSchema,
  CompleteJobResponseSchema,
  CreateProcessInstanceResponseSchema,
  ElementInstanceSchema,
  ElementInstanceState,
  GetProcessInstanceResponseSchema,
  IncidentSchema,
  JobSchema,
  ListProcessInstancesResponseSchema,
  ProcessInstanceSchema,
  ProcessInstanceState,
  VariableSchema,
  type JobClient,
  type ProcessInstanceClient,
} from '@workflow/api-client';
import { ConnectProcessInstanceRepository, toDomainInstance } from './connect-process-instance.repository';

const instanceKey = '0190f5d2-6c1a-7b3e-8f00-0000000000a1';
const definitionKey = '0190f5d2-6c1a-7b3e-8f00-0000000000d1';

/** fakeClients membuat client palsu: hanya method yang dipakai test yang diisi. */
function fakeClients(instances: Partial<ProcessInstanceClient>, jobs: Partial<JobClient> = {}) {
  return new ConnectProcessInstanceRepository(instances as ProcessInstanceClient, jobs as JobClient);
}

describe('connect process instance repository', () => {
  it('maps a proto instance with uuid keys, enum state and timestamps', () => {
    const p = toDomainInstance(
      create(ProcessInstanceSchema, {
        processInstanceKey: instanceKey,
        processDefinitionKey: definitionKey,
        bpmnProcessId: 'order',
        version: 2,
        state: ProcessInstanceState.ACTIVE,
        hasIncident: true,
        startTime: timestampFromDate(new Date('2026-10-28T00:00:00Z')),
      }),
    );
    expect(p).toEqual({
      key: instanceKey,
      definitionKey,
      bpmnProcessId: 'order',
      version: 2,
      state: 'active',
      hasIncident: true,
      startTime: new Date('2026-10-28T00:00:00Z'),
      endTime: null,
    });
  });

  it('creates an instance by definition key using the oneof case', async () => {
    const createProcessInstance = vi.fn().mockResolvedValue(
      create(CreateProcessInstanceResponseSchema, {
        processInstanceKey: instanceKey,
        processDefinitionKey: definitionKey,
        bpmnProcessId: 'order',
        version: 2,
        state: ProcessInstanceState.ACTIVE,
      }),
    );
    const repo = fakeClients({ createProcessInstance });

    const created = await repo.create({ definition: { kind: 'key', definitionKey }, variables: '' });

    expect(createProcessInstance).toHaveBeenCalledWith({
      definition: { case: 'processDefinitionKey', value: definitionKey },
      variables: '{}',
    });
    expect(created).toEqual({ key: instanceKey, definitionKey, bpmnProcessId: 'order', version: 2, state: 'active' });
  });

  it('creates an instance by bpmn process id and version', async () => {
    const createProcessInstance = vi.fn().mockResolvedValue(create(CreateProcessInstanceResponseSchema, {}));
    const repo = fakeClients({ createProcessInstance });

    await repo.create({
      definition: { kind: 'bpmnProcess', bpmnProcessId: 'order', version: 0 },
      variables: '{"a":1}',
    });

    const req = createProcessInstance.mock.calls[0][0];
    expect(req.definition.case).toBe('bpmnProcess');
    expect(req.definition.value.bpmnProcessId).toBe('order');
    expect(req.definition.value.version).toBe(0);
    expect(req.variables).toBe('{"a":1}');
  });

  it('maps the full instance detail', async () => {
    const getProcessInstance = vi.fn().mockResolvedValue(
      create(GetProcessInstanceResponseSchema, {
        processInstance: create(ProcessInstanceSchema, {
          processInstanceKey: instanceKey,
          state: ProcessInstanceState.COMPLETED,
        }),
        elementInstances: [
          create(ElementInstanceSchema, {
            elementInstanceKey: 'e1',
            flowScopeKey: instanceKey,
            elementId: 'task',
            bpmnElementType: 'SERVICE_TASK',
            state: ElementInstanceState.ACTIVATED,
            incidentKey: 'i1',
          }),
        ],
        variables: [create(VariableSchema, { scopeKey: instanceKey, name: 'amount', value: '100' })],
        incidents: [create(IncidentSchema, { incidentKey: 'i1', elementId: 'task', errorType: 'JOB_NO_RETRIES' })],
        jobs: [create(JobSchema, { jobKey: 'j1', type: 'charge', elementId: 'task', retries: 3, state: 'CREATED' })],
      }),
    );
    const detail = await fakeClients({ getProcessInstance }).get(instanceKey);

    expect(getProcessInstance).toHaveBeenCalledWith({ processInstanceKey: instanceKey });
    expect(detail.instance.state).toBe('completed');
    expect(detail.elementInstances).toEqual([
      {
        key: 'e1',
        flowScopeKey: instanceKey,
        elementId: 'task',
        elementType: 'SERVICE_TASK',
        state: 'activated',
        incidentKey: 'i1',
        startTime: null,
        endTime: null,
      },
    ]);
    expect(detail.variables).toEqual([{ scopeKey: instanceKey, name: 'amount', value: '100' }]);
    expect(detail.incidents[0]).toMatchObject({ key: 'i1', elementId: 'task', resolved: false, createdAt: null });
    expect(detail.jobs).toEqual([
      { key: 'j1', type: 'charge', elementId: 'task', elementInstanceKey: '', retries: 3, state: 'CREATED' },
    ]);
  });

  it('rejects a detail response without instance metadata', async () => {
    const getProcessInstance = vi.fn().mockResolvedValue(create(GetProcessInstanceResponseSchema, {}));
    await expect(fakeClients({ getProcessInstance }).get(instanceKey)).rejects.toThrow(/no metadata/);
  });

  it('lists instances with filters and defaults', async () => {
    const listProcessInstances = vi.fn().mockResolvedValue(
      create(ListProcessInstancesResponseSchema, {
        processInstances: [create(ProcessInstanceSchema, { processInstanceKey: instanceKey })],
        nextPageToken: 'next',
      }),
    );
    const page = await fakeClients({ listProcessInstances }).list({ bpmnProcessId: 'order', state: 'active' });

    expect(listProcessInstances).toHaveBeenCalledWith({
      bpmnProcessId: 'order',
      processDefinitionKey: '',
      state: ProcessInstanceState.ACTIVE,
      onlyWithIncident: false,
      pageSize: 50,
      pageToken: '',
    });
    expect(page.items.map((i) => i.key)).toEqual([instanceKey]);
    expect(page.nextPageToken).toBe('next');
  });

  it('completes jobs through JobService and cancels instances', async () => {
    const completeJob = vi.fn().mockResolvedValue(create(CompleteJobResponseSchema, {}));
    const cancelProcessInstance = vi.fn().mockResolvedValue(create(CancelProcessInstanceResponseSchema, {}));
    const repo = fakeClients({ cancelProcessInstance }, { completeJob });

    await repo.completeJob('j1', ' ');
    await repo.cancel(instanceKey);

    expect(completeJob).toHaveBeenCalledWith({ jobKey: 'j1', variables: '{}' });
    expect(cancelProcessInstance).toHaveBeenCalledWith({ processInstanceKey: instanceKey });
  });
});
