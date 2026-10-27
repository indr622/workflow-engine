import { queryOptions, useQuery } from '@tanstack/react-query';
import type { HealthRepository } from '../domain/health';
import { useHealthRepository } from './health-repository.context';
import { useHealthUiStore } from './health.store';

/** Query key factory fitur health. */
export const healthKeys = {
  all: ['health'] as const,
  report: () => [...healthKeys.all, 'report'] as const,
};

export const healthReportQuery = (repository: HealthRepository) =>
  queryOptions({
    queryKey: healthKeys.report(),
    queryFn: () => repository.check(),
    retry: 1,
  });

/** useHealthReport: use case "lihat kesehatan engine", dengan auto-refresh dari state klien. */
export function useHealthReport() {
  const repository = useHealthRepository();
  const autoRefresh = useHealthUiStore((s) => s.autoRefresh);
  return useQuery({ ...healthReportQuery(repository), refetchInterval: autoRefresh ? 5000 : false });
}
