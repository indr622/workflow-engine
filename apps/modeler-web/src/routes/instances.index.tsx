import { createFileRoute } from '@tanstack/react-router';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@workflow/ui';
import { InstancesTable, processInstancesQuery } from '../features/process-instance';

interface InstancesSearch {
  bpmnProcessId?: string;
}

export const Route = createFileRoute('/instances/')({
  validateSearch: (search: Record<string, unknown>): InstancesSearch =>
    typeof search['bpmnProcessId'] === 'string' && search['bpmnProcessId'] !== ''
      ? { bpmnProcessId: search['bpmnProcessId'] }
      : {},
  loaderDeps: ({ search }) => ({ bpmnProcessId: search.bpmnProcessId ?? '' }),
  loader: ({ context, deps }) =>
    context.queryClient.prefetchQuery(
      processInstancesQuery(context.dependencies.processInstanceRepository, {
        bpmnProcessId: deps.bpmnProcessId,
        onlyWithIncident: false,
        pageSize: 20,
        pageToken: '',
      }),
    ),
  component: InstancesPage,
});

function InstancesPage() {
  const { bpmnProcessId = '' } = Route.useSearch();
  const navigate = Route.useNavigate();
  return (
    <Card>
      <CardHeader>
        <CardTitle>Process instances</CardTitle>
        <CardDescription>Instance yang dijalankan engine; buka satu untuk melihat token di diagram</CardDescription>
      </CardHeader>
      <CardContent>
        <InstancesTable
          bpmnProcessId={bpmnProcessId}
          onBpmnProcessIdChange={(value) =>
            void navigate({ search: value ? { bpmnProcessId: value } : {}, replace: true })
          }
        />
      </CardContent>
    </Card>
  );
}
