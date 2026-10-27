import { useState } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider } from '@tanstack/react-router';
import { DeploymentRepositoryProvider } from '../features/deployment';
import { HealthRepositoryProvider } from '../features/health';
import { createDependencies } from './dependencies';
import { createAppRouter } from './router';

/** App adalah composition root UI: QueryClient, dependensi, dan router. */
export function App() {
  const [queryClient] = useState(() => new QueryClient({ defaultOptions: { queries: { staleTime: 2000 } } }));
  const [dependencies] = useState(() => createDependencies());
  const [router] = useState(() => createAppRouter({ queryClient, dependencies }));

  return (
    <QueryClientProvider client={queryClient}>
      <HealthRepositoryProvider repository={dependencies.healthRepository}>
        <DeploymentRepositoryProvider repository={dependencies.deploymentRepository}>
          <RouterProvider router={router} />
        </DeploymentRepositoryProvider>
      </HealthRepositoryProvider>
    </QueryClientProvider>
  );
}
