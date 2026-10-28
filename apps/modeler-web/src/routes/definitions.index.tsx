import { createFileRoute } from '@tanstack/react-router';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@workflow/ui';
import { DefinitionsTable, definitionsQuery } from '../features/deployment';
import { StartInstanceDialog } from '../features/process-instance';

export const Route = createFileRoute('/definitions/')({
  loader: ({ context }) =>
    context.queryClient.prefetchQuery(
      definitionsQuery(context.dependencies.deploymentRepository, {
        bpmnProcessId: '',
        latestOnly: true,
        pageSize: 20,
        pageToken: '',
      }),
    ),
  component: DefinitionsPage,
});

function DefinitionsPage() {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Process definitions</CardTitle>
        <CardDescription>Versi proses yang sudah di-deploy ke engine</CardDescription>
      </CardHeader>
      <CardContent>
        <DefinitionsTable
          rowActions={(d) => (
            <StartInstanceDialog definitionKey={d.key} bpmnProcessId={d.bpmnProcessId} version={d.version} />
          )}
        />
      </CardContent>
    </Card>
  );
}
