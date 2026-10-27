import { ServingStatus, type CheckResponse, type HealthClient } from '@workflow/api-client';
import type { HealthReport, HealthRepository, HealthStatus } from '../domain/health';

/** toDomainStatus memetakan enum proto ke status domain. */
export function toDomainStatus(status: ServingStatus): HealthStatus {
  switch (status) {
    case ServingStatus.SERVING:
      return 'serving';
    case ServingStatus.NOT_SERVING:
      return 'not_serving';
    default:
      return 'unknown';
  }
}

/** toDomainReport memetakan DTO proto ke model domain. */
export function toDomainReport(res: CheckResponse): HealthReport {
  return {
    status: toDomainStatus(res.status),
    version: res.version,
    dependencies: res.dependencies.map((d) => ({
      name: d.name,
      status: toDomainStatus(d.status),
      message: d.message || undefined,
    })),
  };
}

/** ConnectHealthRepository mengimplementasikan port HealthRepository lewat Connect RPC. */
export class ConnectHealthRepository implements HealthRepository {
  constructor(private readonly client: HealthClient) {}

  async check(): Promise<HealthReport> {
    const res = await this.client.check({});
    return toDomainReport(res);
  }
}
