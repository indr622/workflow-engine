import { createClient, type Client, type Transport } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { HealthService } from '../gen/workflow/health/v1/health_pb';
import { DeploymentService } from '../gen/workflow/deployment/v1/deployment_pb';
import { ProcessInstanceService } from '../gen/workflow/process/v1/process_instance_pb';
import { JobService } from '../gen/workflow/job/v1/job_pb';
import { IncidentService } from '../gen/workflow/incident/v1/incident_pb';
import { MessageService } from '../gen/workflow/message/v1/message_pb';

/**
 * createEngineTransport membuat transport Connect ke engine.
 * baseUrl "/api" berarti same-origin lewat proxy Vite saat dev dan nginx saat produksi.
 */
export function createEngineTransport(baseUrl = '/api'): Transport {
  return createConnectTransport({ baseUrl, useBinaryFormat: false });
}

export type HealthClient = Client<typeof HealthService>;
export type DeploymentClient = Client<typeof DeploymentService>;
export type ProcessInstanceClient = Client<typeof ProcessInstanceService>;
export type JobClient = Client<typeof JobService>;
export type IncidentClient = Client<typeof IncidentService>;
export type MessageClient = Client<typeof MessageService>;

/** createHealthClient membuat client bertipe untuk HealthService. */
export function createHealthClient(transport: Transport): HealthClient {
  return createClient(HealthService, transport);
}

/** createDeploymentClient membuat client bertipe untuk DeploymentService. */
export function createDeploymentClient(transport: Transport): DeploymentClient {
  return createClient(DeploymentService, transport);
}

/** createProcessInstanceClient membuat client bertipe untuk ProcessInstanceService. */
export function createProcessInstanceClient(transport: Transport): ProcessInstanceClient {
  return createClient(ProcessInstanceService, transport);
}

/** createJobClient membuat client bertipe untuk JobService. */
export function createJobClient(transport: Transport): JobClient {
  return createClient(JobService, transport);
}

/** createIncidentClient membuat client bertipe untuk IncidentService. */
export function createIncidentClient(transport: Transport): IncidentClient {
  return createClient(IncidentService, transport);
}

/** createMessageClient membuat client bertipe untuk MessageService. */
export function createMessageClient(transport: Transport): MessageClient {
  return createClient(MessageService, transport);
}
