import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import {
  ConnectCode,
  ConnectError,
  DeploymentRejectionSchema,
  ProcessDefinitionSchema,
  Severity,
  ValidationIssueSchema,
} from '@workflow/api-client';
import { rejectionFrom, toDomainDefinition } from './connect-deployment.repository';

describe('connect deployment repository mappers', () => {
  it('maps a proto definition including int64 keys and timestamps', () => {
    const d = toDomainDefinition(
      create(ProcessDefinitionSchema, {
        processDefinitionKey: '0190f5d2-6c1a-7b3e-8f00-123456789abc',
        bpmnProcessId: 'order',
        version: 3,
        deploymentKey: '0190f5d2-6c1a-7b3e-8f00-000000000042',
        createdAt: timestampFromDate(new Date('2026-10-28T00:00:00Z')),
        duplicate: true,
      }),
    );
    expect(d.key).toBe('0190f5d2-6c1a-7b3e-8f00-123456789abc');
    expect(d.deploymentKey).toBe('0190f5d2-6c1a-7b3e-8f00-000000000042');
    expect(d.createdAt.toISOString()).toBe('2026-10-28T00:00:00.000Z');
    expect(d.duplicate).toBe(true);
  });

  it('extracts DeploymentRejection details from a ConnectError', () => {
    const detail = create(DeploymentRejectionSchema, {
      issues: [
        create(ValidationIssueSchema, {
          code: 'TASK_DEFINITION_MISSING',
          severity: Severity.ERROR,
          elementId: 'task',
          line: 4,
          column: 5,
        }),
        create(ValidationIssueSchema, { code: 'IMPLICIT_END', severity: Severity.WARNING, elementId: 'task' }),
      ],
    });
    const err = new ConnectError('rejected', ConnectCode.InvalidArgument, undefined, [
      { desc: DeploymentRejectionSchema, value: detail },
    ]);

    const rejection = rejectionFrom(err);
    expect(rejection?.issues.map((i) => [i.code, i.severity])).toEqual([
      ['TASK_DEFINITION_MISSING', 'error'],
      ['IMPLICIT_END', 'warning'],
    ]);
    expect(rejectionFrom(new Error('plain'))).toBeNull();
  });
});
