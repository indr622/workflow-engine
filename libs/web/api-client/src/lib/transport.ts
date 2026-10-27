import { createClient, type Client, type Transport } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { HealthService } from '../gen/workflow/health/v1/health_pb';

/**
 * createEngineTransport membuat transport Connect ke engine.
 * baseUrl kosong berarti same-origin (lewat proxy Vite saat dev, nginx saat produksi).
 */
export function createEngineTransport(baseUrl = '/api'): Transport {
  return createConnectTransport({ baseUrl, useBinaryFormat: false });
}

export type HealthClient = Client<typeof HealthService>;

/** createHealthClient membuat client bertipe untuk HealthService. */
export function createHealthClient(transport: Transport): HealthClient {
  return createClient(HealthService, transport);
}
