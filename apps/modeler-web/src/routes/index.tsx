import { createFileRoute } from '@tanstack/react-router';
import { HealthCard, healthReportQuery } from '../features/health';
import { BpmnPanel } from '../features/modeler';
import { ProcessMapPanel } from '../features/process-map';

export const Route = createFileRoute('/')({
  // Prefetch status engine; kegagalan tidak memblok halaman, ditampilkan oleh HealthCard.
  loader: ({ context }) =>
    context.queryClient.prefetchQuery(healthReportQuery(context.dependencies.healthRepository)),
  component: DashboardPage,
});

function DashboardPage() {
  return (
    <div className="grid gap-4">
      <HealthCard />
      <div className="grid gap-4 lg:grid-cols-2">
        <BpmnPanel />
        <ProcessMapPanel />
      </div>
    </div>
  );
}
