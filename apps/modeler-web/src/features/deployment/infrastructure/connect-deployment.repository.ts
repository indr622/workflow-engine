import { create } from '@bufbuild/protobuf';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import {
  ConnectError,
  DeploymentRejectionSchema,
  Profile,
  ResourceSchema,
  Severity,
  type DeploymentClient,
  type ProcessDefinition as ProtoDefinition,
  type ValidationIssue as ProtoIssue,
} from '@workflow/api-client';
import {
  DeploymentRejectedError,
  type DefinitionsPage,
  type DeploymentRepository,
  type DeployOutcome,
  type ListDefinitionsParams,
  type ProcessDefinition,
  type ResourceInput,
  type ValidateOutcome,
  type ValidationIssue,
} from '../domain/deployment';

const encoder = new TextEncoder();

export function toDomainIssue(i: ProtoIssue): ValidationIssue {
  return {
    code: i.code,
    severity: i.severity === Severity.WARNING ? 'warning' : 'error',
    message: i.message,
    resourceName: i.resourceName,
    processId: i.bpmnProcessId,
    elementId: i.elementId,
    line: i.line,
    column: i.column,
  };
}

export function toDomainDefinition(d: ProtoDefinition): ProcessDefinition {
  return {
    key: d.processDefinitionKey.toString(),
    bpmnProcessId: d.bpmnProcessId,
    version: d.version,
    versionTag: d.versionTag,
    name: d.name,
    resourceName: d.resourceName,
    deploymentKey: d.deploymentKey.toString(),
    createdAt: d.createdAt ? timestampDate(d.createdAt) : new Date(0),
    duplicate: d.duplicate,
  };
}

/** rejectionFrom membaca detail DeploymentRejection dari ConnectError, bila ada. */
export function rejectionFrom(error: unknown): DeploymentRejectedError | null {
  if (!(error instanceof ConnectError)) return null;
  const details = error.findDetails(DeploymentRejectionSchema);
  if (details.length === 0) return null;
  return new DeploymentRejectedError(details.flatMap((d) => d.issues.map(toDomainIssue)));
}

function toProtoResources(resources: ResourceInput[]) {
  return resources.map((r) => create(ResourceSchema, { name: r.name, content: encoder.encode(r.content) }));
}

/** ConnectDeploymentRepository mengimplementasikan port DeploymentRepository lewat Connect RPC. */
export class ConnectDeploymentRepository implements DeploymentRepository {
  constructor(private readonly client: DeploymentClient) {}

  async deploy(resources: ResourceInput[]): Promise<DeployOutcome> {
    try {
      const res = await this.client.deployResource({ resources: toProtoResources(resources) });
      return {
        deploymentKey: res.deploymentKey.toString(),
        definitions: res.processDefinitions.map(toDomainDefinition),
        warnings: res.warnings.map(toDomainIssue),
      };
    } catch (error) {
      throw rejectionFrom(error) ?? error;
    }
  }

  async validate(resources: ResourceInput[]): Promise<ValidateOutcome> {
    const res = await this.client.validateResource({
      resources: toProtoResources(resources),
      profile: Profile.EXECUTABLE,
    });
    return { valid: res.valid, issues: res.issues.map(toDomainIssue) };
  }

  async list(params: ListDefinitionsParams): Promise<DefinitionsPage> {
    const res = await this.client.listProcessDefinitions({
      bpmnProcessId: params.bpmnProcessId ?? '',
      latestOnly: params.latestOnly ?? false,
      pageSize: params.pageSize ?? 50,
      pageToken: params.pageToken ?? '',
    });
    return { items: res.processDefinitions.map(toDomainDefinition), nextPageToken: res.nextPageToken };
  }

  async get(key: string) {
    const res = await this.client.getProcessDefinition({ processDefinitionKey: key });
    if (!res.processDefinition) throw new Error(`process definition ${key} has no metadata`);
    return { definition: toDomainDefinition(res.processDefinition), bpmnXml: res.bpmnXml };
  }
}
