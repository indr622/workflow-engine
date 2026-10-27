import { useCallback, useState } from 'react';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Badge, Button, Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@workflow/ui';
import { BpmnCanvas } from '@workflow/bpmn-viewer';
import { definitionQuery, useProcessDefinition } from '../features/deployment';

export const Route = createFileRoute('/definitions/$key')({
  loader: ({ context, params }) =>
    context.queryClient.ensureQueryData(definitionQuery(context.dependencies.deploymentRepository, params.key)),
  component: DefinitionPage,
});

function DefinitionPage() {
  const { key } = Route.useParams();
  const { data, isPending, isError, error } = useProcessDefinition(key);
  const [hasLayout, setHasLayout] = useState(true);
  const onImported = useCallback((info: { hasLayout: boolean }) => setHasLayout(info.hasLayout), []);

  if (isPending) return <p className="text-sm text-muted-foreground">Loading…</p>;
  if (isError) return <p className="text-sm text-destructive">{error.message}</p>;

  const { definition, bpmnXml } = data;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {definition.bpmnProcessId} <Badge variant="secondary">v{definition.version}</Badge>
          {definition.versionTag && <Badge variant="outline">{definition.versionTag}</Badge>}
        </CardTitle>
        <CardDescription>
          {definition.resourceName} · key {definition.key} · deployed {definition.createdAt.toLocaleString()}
        </CardDescription>
        <CardAction>
          <Button variant="outline" size="sm" asChild>
            <Link to="/definitions">Back</Link>
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-2">
        {!hasLayout && (
          <p className="text-sm text-muted-foreground">
            This resource has no diagram layout (BPMNDI), so there is nothing to draw. The process is still executable.
          </p>
        )}
        <div className="h-[520px]">
          <BpmnCanvas mode="viewer" xml={bpmnXml} onImported={onImported} />
        </div>
      </CardContent>
    </Card>
  );
}
