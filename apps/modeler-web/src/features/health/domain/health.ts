/**
 * Domain kesehatan engine. Murni TypeScript: tidak boleh meng-import React,
 * client API, maupun lapisan lain.
 */

export type HealthStatus = 'serving' | 'not_serving' | 'unknown';

export interface DependencyHealth {
  name: string;
  status: HealthStatus;
  message?: string;
}

export interface HealthReport {
  status: HealthStatus;
  version: string;
  dependencies: DependencyHealth[];
}

/** Port keluar: cara mendapatkan laporan kesehatan, diimplementasikan di infrastructure. */
export interface HealthRepository {
  check(): Promise<HealthReport>;
}

/** failingDependencies mengembalikan dependensi yang tidak serving. */
export function failingDependencies(report: HealthReport): DependencyHealth[] {
  return report.dependencies.filter((d) => d.status !== 'serving');
}

/** isHealthy: engine sehat hanya jika status agregat serving dan tidak ada dependensi gagal. */
export function isHealthy(report: HealthReport): boolean {
  return report.status === 'serving' && failingDependencies(report).length === 0;
}
