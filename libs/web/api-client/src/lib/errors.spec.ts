import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it } from 'vitest';
import { ErrorInfoSchema } from '../gen/workflow/common/v1/error_info_pb';
import { DeploymentRejectionSchema } from '../gen/workflow/deployment/v1/deployment_pb';
import { findDeploymentRejection, findErrorInfo, isReason } from './errors';

describe('findErrorInfo', () => {
  it('reads the ErrorInfo detail of a ConnectError', () => {
    const info = create(ErrorInfoSchema, {
      reason: 'JOB_NOT_ACTIVE',
      domain: 'workflow-engine',
      metadata: { state: 'FAILED' },
    });
    const err = new ConnectError('job is not active', Code.FailedPrecondition, undefined, [
      { desc: ErrorInfoSchema, value: info },
    ]);
    expect(findErrorInfo(err)?.metadata['state']).toBe('FAILED');
    expect(isReason(err, 'JOB_NOT_ACTIVE')).toBe(true);
    expect(isReason(err, 'JOB_NOT_FOUND')).toBe(false);
  });

  it('returns undefined for errors without details or non-Connect errors', () => {
    expect(findErrorInfo(new ConnectError('x', Code.Internal))).toBeUndefined();
    expect(findErrorInfo(new Error('plain'))).toBeUndefined();
    expect(findErrorInfo('nope')).toBeUndefined();
    expect(isReason(undefined, 'REQUEST_INVALID')).toBe(false);
  });

  it('reads DeploymentRejection next to ErrorInfo', () => {
    const rejection = create(DeploymentRejectionSchema, { issues: [{ code: 'UNSUPPORTED_ELEMENT', message: 'm' }] });
    const err = new ConnectError('rejected', Code.InvalidArgument, undefined, [
      { desc: ErrorInfoSchema, value: create(ErrorInfoSchema, { reason: 'DEPLOYMENT_REJECTED' }) },
      { desc: DeploymentRejectionSchema, value: rejection },
    ]);
    expect(isReason(err, 'DEPLOYMENT_REJECTED')).toBe(true);
    expect(findDeploymentRejection(err)?.issues[0]?.code).toBe('UNSUPPORTED_ELEMENT');
    expect(findDeploymentRejection(new Error('x'))).toBeUndefined();
  });
});
