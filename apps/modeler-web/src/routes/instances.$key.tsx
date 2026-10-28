import { useEffect } from 'react';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Badge, Button, Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@workflow/ui';
import { definitionQuery, useProcessDefinition } from '../features/deployment';
import {
  CancelInstanceButton,
  ElementInstancesPanel,
  IncidentsPanel,
  InstanceStateBadge,
  InstanceViewer,
  JobsPanel,
  VariablesPanel,
  isFinalInstance,
  processInstanceQuery,
  useInstanceUiStore,
  useProcessInstance,
  type ProcessInstanceDetail,
} from '../features/process-instance';

export const Route = createFileRoute('/instances/$key')({
  loader: async ({ context, params }) => {
    const { queryClient, dependencies } = context;
    const detail = await queryClient.ensureQueryData(
      processInstanceQuery(dependencies.processInstanceRepository, params.key),
    );
    // XML diagram diambil dari fitur deployment berdasarkan key definition instance.
    await queryClient.prefetchQuery(definitionQuery(dependencies.deploymentRepository, detail.instance.definitionKey));
  },
  component: InstancePage,
});

function InstancePage() {
  const { key } = Route.useParams();
  const { data, isPending, isError, error } = useProcessInstance(key);
  const clearSelection = useInstanceUiStore((s) => s.clear);

  useEffect(() => clearSelection, [key, clearSelection]);

  if (isPending) return <p className="text-sm text-muted-foreground">Loading…</p>;
  if (isError) return <p className="text-sm text-destructive">{error.message}</p>;
  return <InstanceDetail detail={data} />;
}

function InstanceDetail({ detail }: { detail: ProcessInstanceDetail }) {
  const { instance } = detail;
  const definition = useProcessDefinition(instance.definitionKey);

  return (
    <div className="grid gap-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            {instance.bpmnProcessId} <Badge variant="secondary">v{instance.version}</Badge>
            <InstanceStateBadge instance={instance} />
          </CardTitle>
          <CardDescription>
            instance <span className="font-mono">{instance.key}</span>
            {instance.startTime && <> · started {instance.startTime.toLocaleString()}</>}
            {instance.endTime && <> · ended {instance.endTime.toLocaleString()}</>}
          </CardDescription>
          <CardAction className="flex gap-2">
            <CancelInstanceButton instanceKey={instance.key} disabled={isFinalInstance(instance)} />
            <Button variant="outline" size="sm" asChild>
              <Link to="/definitions/$key" params={{ key: instance.definitionKey }}>
                Definition
              </Link>
            </Button>
            <Button variant="outline" size="sm" asChild>
              <Link to="/instances">Back</Link>
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent>
          {definition.isPending && <p className="text-sm text-muted-foreground">Loading diagram…</p>}
          {definition.isError && (
            <p className="text-sm text-destructive">Cannot load the diagram: {definition.error.message}</p>
          )}
          {definition.isSuccess && <InstanceViewer xml={definition.data.bpmnXml} detail={detail} />}
        </CardContent>
      </Card>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Open jobs</CardTitle>
            <CardDescription>Complete a job manually in place of a worker</CardDescription>
          </CardHeader>
          <CardContent>
            <JobsPanel instanceKey={instance.key} jobs={detail.jobs} />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Incidents</CardTitle>
          </CardHeader>
          <CardContent>
            <IncidentsPanel incidents={detail.incidents} />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Variables</CardTitle>
          </CardHeader>
          <CardContent>
            <VariablesPanel variables={detail.variables} rootScopeKey={instance.key} />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Element instances</CardTitle>
            <CardDescription>Click a row or a diagram element to highlight it</CardDescription>
          </CardHeader>
          <CardContent>
            <ElementInstancesPanel elementInstances={detail.elementInstances} />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
