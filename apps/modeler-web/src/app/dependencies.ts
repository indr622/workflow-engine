import { createDeploymentClient, createEngineTransport, createHealthClient } from '@workflow/api-client';
import type { DeploymentRepository } from '../features/deployment';
import { ConnectDeploymentRepository } from '../features/deployment/infrastructure/connect-deployment.repository';
import type { HealthRepository } from '../features/health';
import { ConnectHealthRepository } from '../features/health/infrastructure/connect-health.repository';

/** AppDependencies adalah semua implementasi port yang dirakit composition root. */
export interface AppDependencies {
  healthRepository: HealthRepository;
  deploymentRepository: DeploymentRepository;
}

/** createDependencies merakit adapter infrastructure. Satu-satunya tempat app menyentuh transport. */
export function createDependencies(baseUrl = '/api'): AppDependencies {
  const transport = createEngineTransport(baseUrl);
  return {
    healthRepository: new ConnectHealthRepository(createHealthClient(transport)),
    deploymentRepository: new ConnectDeploymentRepository(createDeploymentClient(transport)),
  };
}
