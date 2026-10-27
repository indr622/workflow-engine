import { Link, Outlet, createRootRouteWithContext } from '@tanstack/react-router';
import { TanStackRouterDevtools } from '@tanstack/react-router-devtools';
import { ReactQueryDevtools } from '@tanstack/react-query-devtools';
import type { RouterContext } from '../app/router';

export const Route = createRootRouteWithContext<RouterContext>()({
  component: RootLayout,
});

function RootLayout() {
  return (
    <div className="min-h-screen">
      <header className="border-b">
        <div className="mx-auto flex h-14 max-w-7xl items-center gap-6 px-4">
          <span className="font-semibold">Workflow Engine</span>
          <nav className="flex gap-4 text-sm text-muted-foreground">
            <Link to="/" className="[&.active]:font-medium [&.active]:text-foreground">
              Modeler
            </Link>
            <Link to="/definitions" className="[&.active]:font-medium [&.active]:text-foreground">
              Definitions
            </Link>
          </nav>
        </div>
      </header>
      <main className="mx-auto max-w-7xl p-4">
        <Outlet />
      </main>
      {import.meta.env.DEV && (
        <>
          <ReactQueryDevtools buttonPosition="bottom-left" />
          <TanStackRouterDevtools position="bottom-right" />
        </>
      )}
    </div>
  );
}
