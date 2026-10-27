import { create } from '@bufbuild/protobuf';
import { CheckResponseSchema, ServingStatus } from '@workflow/api-client';
import { toDomainReport } from './connect-health.repository';

describe('toDomainReport', () => {
  it('maps proto response to the domain model', () => {
    const res = create(CheckResponseSchema, {
      status: ServingStatus.NOT_SERVING,
      version: 'v1',
      dependencies: [{ name: 'postgres', status: ServingStatus.NOT_SERVING, message: 'refused' }],
    });

    expect(toDomainReport(res)).toEqual({
      status: 'not_serving',
      version: 'v1',
      dependencies: [{ name: 'postgres', status: 'not_serving', message: 'refused' }],
    });
  });
});
