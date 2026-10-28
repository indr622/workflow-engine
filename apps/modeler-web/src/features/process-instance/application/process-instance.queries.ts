import { queryOptions, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  isFinalInstance,
  type CreateInstanceInput,
  type ListInstancesParams,
  type ProcessInstanceDetail,
  type ProcessInstanceRepository,
} from '../domain/process-instance';
import { useProcessInstanceRepository } from './process-instance-repository.context';

/** Interval polling detail instance selama masih aktif (rencana 8.14). */
export const ACTIVE_INSTANCE_POLL_MS = 1000;

/** Query key factory fitur process-instance. */
export const processInstanceKeys = {
  all: ['process-instance'] as const,
  lists: () => [...processInstanceKeys.all, 'list'] as const,
  list: (params: ListInstancesParams) => [...processInstanceKeys.lists(), params] as const,
  detail: (key: string) => [...processInstanceKeys.all, 'detail', key] as const,
};

/** pollIntervalFor: polling 1 detik selama instance belum final; berhenti setelah final atau error. */
export function pollIntervalFor(detail: ProcessInstanceDetail | undefined): number | false {
  if (!detail) return false;
  return isFinalInstance(detail.instance) ? false : ACTIVE_INSTANCE_POLL_MS;
}

export const processInstancesQuery = (repository: ProcessInstanceRepository, params: ListInstancesParams) =>
  queryOptions({
    queryKey: processInstanceKeys.list(params),
    queryFn: () => repository.list(params),
  });

export const processInstanceQuery = (repository: ProcessInstanceRepository, key: string) =>
  queryOptions({
    queryKey: processInstanceKeys.detail(key),
    queryFn: () => repository.get(key),
    refetchInterval: (query) => pollIntervalFor(query.state.data),
  });

/** useProcessInstances: use case "lihat daftar instance". */
export function useProcessInstances(params: ListInstancesParams) {
  const repository = useProcessInstanceRepository();
  return useQuery(processInstancesQuery(repository, params));
}

/** useProcessInstance: use case "pantau satu instance" (token, job, variabel, incident). */
export function useProcessInstance(key: string) {
  const repository = useProcessInstanceRepository();
  return useQuery(processInstanceQuery(repository, key));
}

/** useCreateInstance: use case "jalankan instance baru dari definition". */
export function useCreateInstance() {
  const repository = useProcessInstanceRepository();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateInstanceInput) => repository.create(input),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: processInstanceKeys.lists() }),
  });
}

/** useCompleteJob: use case "selesaikan job manual" (pengganti worker selama Fase 2). */
export function useCompleteJob(instanceKey: string) {
  const repository = useProcessInstanceRepository();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ jobKey, variables }: { jobKey: string; variables: string }) =>
      repository.completeJob(jobKey, variables),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: processInstanceKeys.detail(instanceKey) }),
  });
}

/** useCancelInstance: use case "batalkan instance". */
export function useCancelInstance() {
  const repository = useProcessInstanceRepository();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (key: string) => repository.cancel(key),
    onSuccess: async (_data, key) => {
      await queryClient.invalidateQueries({ queryKey: processInstanceKeys.detail(key) });
      await queryClient.invalidateQueries({ queryKey: processInstanceKeys.lists() });
    },
  });
}
