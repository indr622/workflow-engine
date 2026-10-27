import { Activity, RefreshCw } from 'lucide-react';
import { Badge, Button, Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@workflow/ui';
import { useHealthReport } from '../application/health.queries';
import { useHealthUiStore } from '../application/health.store';
import { failingDependencies, isHealthy, type HealthStatus } from '../domain/health';

const tone: Record<HealthStatus, 'success' | 'destructive' | 'secondary'> = {
  serving: 'success',
  not_serving: 'destructive',
  unknown: 'secondary',
};

/** HealthCard menampilkan status engine. Tidak berisi logika bisnis maupun pemanggilan API. */
export function HealthCard() {
  const { data, isPending, isError, error, refetch, isFetching } = useHealthReport();
  const autoRefresh = useHealthUiStore((s) => s.autoRefresh);
  const toggleAutoRefresh = useHealthUiStore((s) => s.toggleAutoRefresh);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Activity className="size-4" /> Engine health
        </CardTitle>
        <CardDescription>{data ? `version ${data.version}` : 'Connect RPC HealthService.Check'}</CardDescription>
        <CardAction className="flex gap-2">
          <Button variant="outline" size="sm" onClick={toggleAutoRefresh}>
            Auto refresh: {autoRefresh ? 'on' : 'off'}
          </Button>
          <Button variant="outline" size="icon" onClick={() => refetch()} aria-label="Refresh">
            <RefreshCw className={isFetching ? 'animate-spin' : undefined} />
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        {isPending && <p className="text-muted-foreground">Checking engine…</p>}
        {isError && (
          <div className="space-y-1">
            <Badge variant="destructive">unreachable</Badge>
            <p className="text-muted-foreground">{error.message}</p>
          </div>
        )}
        {data && (
          <>
            <Badge variant={isHealthy(data) ? 'success' : 'destructive'}>{data.status}</Badge>
            <ul className="space-y-1">
              {data.dependencies.map((d) => (
                <li key={d.name} className="flex items-center justify-between">
                  <span>{d.name}</span>
                  <Badge variant={tone[d.status]}>{d.status}</Badge>
                </li>
              ))}
            </ul>
            {failingDependencies(data).map((d) => (
              <p key={d.name} className="text-destructive">
                {d.name}: {d.message}
              </p>
            ))}
          </>
        )}
      </CardContent>
    </Card>
  );
}
