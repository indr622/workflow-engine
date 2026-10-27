import nx from '@nx/eslint-plugin';
import baseConfig from '../../eslint.config.mjs';

/** Pola impor menuju lapisan tertentu di dalam fitur (relatif maupun absolut). */
const layer = (name) => [`**/${name}`, `**/${name}/**`];

const transport = {
  group: ['@workflow/api-client', '@connectrpc/*', '@bufbuild/*'],
  message: 'Transport hanya boleh dipakai lapisan infrastructure dan src/app.',
};

export default [
  ...nx.configs['flat/react'],
  ...baseConfig,
  // Clean Architecture per fitur: aturan impor tiap lapisan.
  {
    files: ['src/features/*/domain/**/*.{ts,tsx}'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: [...layer('application'), ...layer('infrastructure'), ...layer('presentation')],
              message: 'domain tidak boleh bergantung pada lapisan lain.',
            },
            {
              group: [
                'react',
                'react-dom',
                'react/*',
                '@tanstack/*',
                'zustand',
                '@workflow/ui',
                '@workflow/flow-canvas',
                '@workflow/bpmn-viewer',
                '@xyflow/*',
                'bpmn-js',
                'bpmn-js/*',
              ],
              message: 'domain harus TypeScript murni tanpa framework.',
            },
            transport,
          ],
        },
      ],
    },
  },
  {
    files: ['src/features/*/application/**/*.{ts,tsx}'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: [...layer('infrastructure'), ...layer('presentation')],
              message: 'application hanya boleh bergantung pada domain.',
            },
            transport,
          ],
        },
      ],
    },
  },
  {
    files: ['src/features/*/presentation/**/*.{ts,tsx}'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: layer('infrastructure'),
              message: 'presentation tidak boleh memanggil infrastructure; pakai hook dari application.',
            },
            transport,
          ],
        },
      ],
    },
  },
  {
    files: ['src/routes/**/*.{ts,tsx}'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: ['**/features/*/*', '**/features/*/*/**'],
              message: 'Route hanya boleh memakai API publik fitur (features/<fitur>/index.ts).',
            },
            transport,
          ],
        },
      ],
    },
  },
];
