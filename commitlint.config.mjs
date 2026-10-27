// Conventional Commits, seluruh pesan bahasa Inggris (lihat CLAUDE.md).
const engineModules = ['health', 'deployment', 'execution', 'job', 'timer', 'message', 'usertask', 'incident'];

export default {
  extends: ['@commitlint/config-conventional'],
  rules: {
    'header-max-length': [2, 'always', 72],
    'scope-enum': [
      2,
      'always',
      [
        'engine',
        ...engineModules.map((m) => `engine/${m}`),
        'worker-example',
        'modeler-web',
        'operate-web',
        'tasklist-web',
        'ui',
        'core',
        'api-client',
        'flow-canvas',
        'bpmn-viewer',
        'proto',
        'bpmn',
        'feel',
        'sdk',
        'infra',
        'docker',
        'ci',
        'nx',
        'deps',
        'docs',
        'plan',
        'release',
      ],
    ],
  },
};
