import nx from '@nx/eslint-plugin';

export default [
  ...nx.configs['flat/base'],
  ...nx.configs['flat/typescript'],
  ...nx.configs['flat/javascript'],
  {
    ignores: [
      '**/dist',
      '**/out-tsc',
      '**/vite.config.*.timestamp*',
      '**/vitest.config.*.timestamp*',
      '**/routeTree.gen.ts',
      '**/src/gen/**',
    ],
  },
  {
    files: ['**/*.ts', '**/*.tsx', '**/*.js', '**/*.jsx'],
    rules: {
      // Batas antar proyek berdasarkan tag di project.json.
      '@nx/enforce-module-boundaries': [
        'error',
        {
          enforceBuildableLibDependency: true,
          allow: ['^.*/eslint(\\.base)?\\.config\\.[cm]?[jt]s$'],
          depConstraints: [
            { sourceTag: 'type:app', onlyDependOnLibsWithTags: ['type:ui', 'type:core', 'type:api-client'] },
            { sourceTag: 'type:ui', onlyDependOnLibsWithTags: ['type:ui', 'type:core'] },
            { sourceTag: 'type:api-client', onlyDependOnLibsWithTags: ['type:core'] },
            { sourceTag: 'type:core', onlyDependOnLibsWithTags: ['type:core'] },
          ],
        },
      ],
    },
  },
];
