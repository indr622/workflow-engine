import { createContext, useContext, type ReactNode } from 'react';
import type { HealthRepository } from '../domain/health';

const HealthRepositoryContext = createContext<HealthRepository | null>(null);

/** HealthRepositoryProvider menyuntikkan implementasi repository dari composition root (src/app). */
export function HealthRepositoryProvider({
  repository,
  children,
}: {
  repository: HealthRepository;
  children: ReactNode;
}) {
  return <HealthRepositoryContext.Provider value={repository}>{children}</HealthRepositoryContext.Provider>;
}

export function useHealthRepository(): HealthRepository {
  const repo = useContext(HealthRepositoryContext);
  if (!repo) {
    throw new Error('HealthRepositoryProvider is missing');
  }
  return repo;
}
