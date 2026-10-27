import { createContext, useContext, type ReactNode } from 'react';
import type { DeploymentRepository } from '../domain/deployment';

const DeploymentRepositoryContext = createContext<DeploymentRepository | null>(null);

/** DeploymentRepositoryProvider menyuntikkan implementasi repository dari composition root. */
export function DeploymentRepositoryProvider({
  repository,
  children,
}: {
  repository: DeploymentRepository;
  children: ReactNode;
}) {
  return <DeploymentRepositoryContext.Provider value={repository}>{children}</DeploymentRepositoryContext.Provider>;
}

export function useDeploymentRepository(): DeploymentRepository {
  const repo = useContext(DeploymentRepositoryContext);
  if (!repo) {
    throw new Error('DeploymentRepositoryProvider is missing');
  }
  return repo;
}
