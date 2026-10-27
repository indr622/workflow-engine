/// <reference types='vitest' />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { tanstackRouter } from '@tanstack/router-plugin/vite';
import { nxReadySignal } from '@nx/vite/plugins/nx-ready-signal.plugin';

// Alamat engine untuk proxy dev; prefix /api dibuang sebelum diteruskan.
const engineUrl = process.env['ENGINE_URL'] ?? 'http://localhost:8080';

export default defineConfig(() => ({
  root: import.meta.dirname,
  cacheDir: '../../node_modules/.vite/apps/modeler-web',
  server: {
    port: 4200,
    host: 'localhost',
    proxy: {
      '/api': {
        target: engineUrl,
        changeOrigin: true,
        rewrite: (path: string) => path.replace(/^\/api/, ''),
      },
    },
  },
  preview: {
    port: 4300,
    host: 'localhost',
  },
  resolve: {
    tsconfigPaths: true,
  },
  plugins: [
    // Plugin router harus sebelum plugin React.
    tanstackRouter({
      target: 'react',
      autoCodeSplitting: true,
      routesDirectory: './src/routes',
      generatedRouteTree: './src/routeTree.gen.ts',
    }),
    react(),
    tailwindcss(),
    nxReadySignal(),
  ],
  build: {
    outDir: '../../dist/apps/modeler-web',
    emptyOutDir: true,
    reportCompressedSize: true,
    chunkSizeWarningLimit: 1500,
    commonjsOptions: {
      transformMixedEsModules: true,
    },
  },
  test: {
    name: 'modeler-web',
    watch: false,
    passWithNoTests: true,
    globals: true,
    environment: 'jsdom',
    include: ['{src,tests}/**/*.{test,spec}.{js,mjs,cjs,ts,mts,cts,jsx,tsx}'],
    reporters: ['default'],
    coverage: {
      reportsDirectory: '../../coverage/apps/modeler-web',
      provider: 'v8' as const,
    },
  },
}));
