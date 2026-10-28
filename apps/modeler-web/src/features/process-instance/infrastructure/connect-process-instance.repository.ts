import { create } from '@bufbuild/protobuf';
import { timestampDate, type Timestamp } from '@bufbuild/protobuf/wkt';
import {
  ElementInstanceState as ProtoElementState,
  ProcessDefinitionRefSchema,
  ProcessInstanceState as ProtoInstanceState,
  type CreateProcessInstanceRequest,
  type ElementInstance as ProtoElementInstance,
  type Incident as ProtoIncident,
  type Job as ProtoJob,
  type JobClient,
  type ProcessInstance as ProtoInstance,
  type ProcessInstanceClient,
  type Variable as ProtoVariable,
} from '@workflow/api-client';
import type {
  CreatedInstance,
  CreateInstanceInput,
  DefinitionSelector,
  ElementInstance,
  ElementInstanceState,
  Incident,
  InstancesPage,
  Job,
  ListInstancesParams,
  ProcessInstance,
  ProcessInstanceDetail,
  ProcessInstanceRepository,
  ProcessInstanceState,
  Variable,
} from '../domain/process-instance';

const instanceStates: Record<ProtoInstanceState, ProcessInstanceState> = {
  [ProtoInstanceState.UNSPECIFIED]: 'unspecified',
  [ProtoInstanceState.ACTIVE]: 'active',
  [ProtoInstanceState.COMPLETED]: 'completed',
  [ProtoInstanceState.TERMINATED]: 'terminated',
};

const elementStates: Record<ProtoElementState, ElementInstanceState> = {
  [ProtoElementState.UNSPECIFIED]: 'unspecified',
  [ProtoElementState.ACTIVATING]: 'activating',
  [ProtoElementState.ACTIVATED]: 'activated',
  [ProtoElementState.COMPLETING]: 'completing',
  [ProtoElementState.COMPLETED]: 'completed',
  [ProtoElementState.TERMINATING]: 'terminating',
  [ProtoElementState.TERMINATED]: 'terminated',
};

function toDate(ts: Timestamp | undefined): Date | null {
  return ts ? timestampDate(ts) : null;
}

export function toDomainInstanceState(state: ProtoInstanceState): ProcessInstanceState {
  return instanceStates[state] ?? 'unspecified';
}

export function toProtoInstanceState(state: ProcessInstanceState | undefined): ProtoInstanceState {
  switch (state) {
    case 'active':
      return ProtoInstanceState.ACTIVE;
    case 'completed':
      return ProtoInstanceState.COMPLETED;
    case 'terminated':
      return ProtoInstanceState.TERMINATED;
    default:
      return ProtoInstanceState.UNSPECIFIED;
  }
}

export function toDomainInstance(p: ProtoInstance): ProcessInstance {
  return {
    key: p.processInstanceKey,
    definitionKey: p.processDefinitionKey,
    bpmnProcessId: p.bpmnProcessId,
    version: p.version,
    state: toDomainInstanceState(p.state),
    hasIncident: p.hasIncident,
    startTime: toDate(p.startTime),
    endTime: toDate(p.endTime),
  };
}

export function toDomainElementInstance(e: ProtoElementInstance): ElementInstance {
  return {
    key: e.elementInstanceKey,
    flowScopeKey: e.flowScopeKey,
    elementId: e.elementId,
    elementType: e.bpmnElementType,
    state: elementStates[e.state] ?? 'unspecified',
    incidentKey: e.incidentKey,
    startTime: toDate(e.startTime),
    endTime: toDate(e.endTime),
  };
}

export function toDomainVariable(v: ProtoVariable): Variable {
  return { scopeKey: v.scopeKey, name: v.name, value: v.value };
}

export function toDomainIncident(i: ProtoIncident): Incident {
  return {
    key: i.incidentKey,
    elementInstanceKey: i.elementInstanceKey,
    elementId: i.elementId,
    errorType: i.errorType,
    errorMessage: i.errorMessage,
    resolved: i.resolved,
    createdAt: toDate(i.createdAt),
  };
}

export function toDomainJob(j: ProtoJob): Job {
  return {
    key: j.jobKey,
    type: j.type,
    elementId: j.elementId,
    elementInstanceKey: j.elementInstanceKey,
    retries: j.retries,
    state: j.state,
  };
}

/** toProtoDefinition memetakan pemilih definisi domain ke oneof `definition` (protobuf-es v2). */
export function toProtoDefinition(selector: DefinitionSelector): CreateProcessInstanceRequest['definition'] {
  if (selector.kind === 'key') {
    return { case: 'processDefinitionKey', value: selector.definitionKey };
  }
  return {
    case: 'bpmnProcess',
    value: create(ProcessDefinitionRefSchema, { bpmnProcessId: selector.bpmnProcessId, version: selector.version }),
  };
}

/**
 * ConnectProcessInstanceRepository mengimplementasikan port ProcessInstanceRepository lewat Connect
 * RPC: ProcessInstanceService untuk instance dan JobService untuk penyelesaian job manual.
 */
export class ConnectProcessInstanceRepository implements ProcessInstanceRepository {
  constructor(
    private readonly instances: ProcessInstanceClient,
    private readonly jobs: JobClient,
  ) {}

  async create(input: CreateInstanceInput): Promise<CreatedInstance> {
    const res = await this.instances.createProcessInstance({
      definition: toProtoDefinition(input.definition),
      variables: input.variables.trim() || '{}',
    });
    return {
      key: res.processInstanceKey,
      definitionKey: res.processDefinitionKey,
      bpmnProcessId: res.bpmnProcessId,
      version: res.version,
      state: toDomainInstanceState(res.state),
    };
  }

  async get(key: string): Promise<ProcessInstanceDetail> {
    const res = await this.instances.getProcessInstance({ processInstanceKey: key });
    if (!res.processInstance) throw new Error(`process instance ${key} has no metadata`);
    return {
      instance: toDomainInstance(res.processInstance),
      elementInstances: res.elementInstances.map(toDomainElementInstance),
      variables: res.variables.map(toDomainVariable),
      incidents: res.incidents.map(toDomainIncident),
      jobs: res.jobs.map(toDomainJob),
    };
  }

  async list(params: ListInstancesParams): Promise<InstancesPage> {
    const res = await this.instances.listProcessInstances({
      bpmnProcessId: params.bpmnProcessId ?? '',
      processDefinitionKey: params.definitionKey ?? '',
      state: toProtoInstanceState(params.state),
      onlyWithIncident: params.onlyWithIncident ?? false,
      pageSize: params.pageSize ?? 50,
      pageToken: params.pageToken ?? '',
    });
    return { items: res.processInstances.map(toDomainInstance), nextPageToken: res.nextPageToken };
  }

  async cancel(key: string): Promise<void> {
    await this.instances.cancelProcessInstance({ processInstanceKey: key });
  }

  async completeJob(jobKey: string, variables: string): Promise<void> {
    await this.jobs.completeJob({ jobKey, variables: variables.trim() || '{}' });
  }
}
