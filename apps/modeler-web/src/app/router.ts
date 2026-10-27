import { createRouter } from '@tanstack/react-router';
import type { QueryClient } from '@tanstack/react-query';
import { routeTree } from '../routeTree.gen';
import type { AppDependencies } from './dependencies';

export interface RouterContext {
  queryClient: QueryClient;
  dependencies: AppDependencies;
}

export function createAppRouter(context: RouterContext) {
  return createRouter({ routeTree, context, defaultPreload: 'intent', scrollRestoration: true });
}

declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof createAppRouter>;
  }
}
