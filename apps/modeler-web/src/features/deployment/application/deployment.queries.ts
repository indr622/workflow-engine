import { queryOptions, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DeploymentRejectedError,
  type DeploymentRepository,
  type ListDefinitionsParams,
  type ResourceInput,
} from '../domain/deployment';
import { useDeploymentRepository } from './deployment-repository.context';
import { useDeploymentUiStore } from './deployment.store';

/** Query key factory fitur deployment. */
export const deploymentKeys = {
  all: ['deployment'] as const,
  definitions: () => [...deploymentKeys.all, 'definitions'] as const,
  definitionList: (params: ListDefinitionsParams) => [...deploymentKeys.definitions(), 'list', params] as const,
  definition: (key: string) => [...deploymentKeys.definitions(), 'detail', key] as const,
};

export const definitionsQuery = (repository: DeploymentRepository, params: ListDefinitionsParams) =>
  queryOptions({
    queryKey: deploymentKeys.definitionList(params),
    queryFn: () => repository.list(params),
  });

export const definitionQuery = (repository: DeploymentRepository, key: string) =>
  queryOptions({
    queryKey: deploymentKeys.definition(key),
    queryFn: () => repository.get(key),
  });

/** useProcessDefinitions: use case "lihat daftar definition". */
export function useProcessDefinitions(params: ListDefinitionsParams) {
  const repository = useDeploymentRepository();
  return useQuery(definitionsQuery(repository, params));
}

/** useProcessDefinition: use case "lihat satu definition beserta XML". */
export function useProcessDefinition(key: string) {
  const repository = useDeploymentRepository();
  return useQuery(definitionQuery(repository, key));
}

/**
 * useDeployResource: use case "deploy diagram". Issue (ditolak maupun warning) disimpan ke store
 * agar kanvas dan panel bisa menampilkannya; daftar definition diinvalidasi saat sukses.
 */
export function useDeployResource() {
  const repository = useDeploymentRepository();
  const queryClient = useQueryClient();
  const setIssues = useDeploymentUiStore((s) => s.setIssues);
  const setLastDeploymentKey = useDeploymentUiStore((s) => s.setLastDeploymentKey);

  return useMutation({
    mutationFn: (resources: ResourceInput[]) => repository.deploy(resources),
    onSuccess: async (outcome) => {
      setIssues(outcome.warnings);
      setLastDeploymentKey(outcome.deploymentKey);
      await queryClient.invalidateQueries({ queryKey: deploymentKeys.definitions() });
    },
    onError: (error) => {
      if (error instanceof DeploymentRejectedError) {
        setIssues(error.issues);
      }
    },
  });
}

/** useValidateResource: use case "periksa diagram tanpa menyimpan". */
export function useValidateResource() {
  const repository = useDeploymentRepository();
  const setIssues = useDeploymentUiStore((s) => s.setIssues);
  return useMutation({
    mutationFn: (resources: ResourceInput[]) => repository.validate(resources),
    onSuccess: (outcome) => setIssues(outcome.issues),
  });
}
