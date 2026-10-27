import { failingDependencies, isHealthy, type HealthReport } from './health';

const report = (overrides: Partial<HealthReport> = {}): HealthReport => ({
  status: 'serving',
  version: 'dev',
  dependencies: [{ name: 'postgres', status: 'serving' }],
  ...overrides,
});

describe('health domain', () => {
  it('is healthy when everything is serving', () => {
    expect(isHealthy(report())).toBe(true);
  });

  it('is unhealthy when a dependency fails', () => {
    const r = report({ dependencies: [{ name: 'postgres', status: 'not_serving', message: 'refused' }] });
    expect(isHealthy(r)).toBe(false);
    expect(failingDependencies(r).map((d) => d.name)).toEqual(['postgres']);
  });
});
