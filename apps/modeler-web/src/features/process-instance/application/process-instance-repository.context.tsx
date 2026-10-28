import { createContext, useContext, type ReactNode } from 'react';
import type { ProcessInstanceRepository } from '../domain/process-instance';

const ProcessInstanceRepositoryContext = createContext<ProcessInstanceRepository | null>(null);

/** ProcessInstanceRepositoryProvider menyuntikkan implementasi repository dari composition root. */
export function ProcessInstanceRepositoryProvider({
  repository,
  children,
}: {
  repository: ProcessInstanceRepository;
  children: ReactNode;
}) {
  return (
    <ProcessInstanceRepositoryContext.Provider value={repository}>{children}</ProcessInstanceRepositoryContext.Provider>
  );
}

export function useProcessInstanceRepository(): ProcessInstanceRepository {
  const repo = useContext(ProcessInstanceRepositoryContext);
  if (!repo) {
    throw new Error('ProcessInstanceRepositoryProvider is missing');
  }
  return repo;
}
