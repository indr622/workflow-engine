import { ConnectError } from '@connectrpc/connect';
import { ErrorInfoSchema, type ErrorInfo } from '../gen/workflow/common/v1/error_info_pb';
import { DeploymentRejectionSchema, type DeploymentRejection } from '../gen/workflow/deployment/v1/deployment_pb';

/**
 * Reason stabil di ErrorInfo.reason (ADR-0023). UI bercabang berdasarkan reason, bukan pesan.
 */
export type ErrorReason =
  | 'REQUEST_INVALID'
  | 'VARIABLES_INVALID'
  | 'PAGE_TOKEN_INVALID'
  | 'JOB_NOT_FOUND'
  | 'PROCESS_INSTANCE_NOT_FOUND'
  | 'INCIDENT_NOT_FOUND'
  | 'PROCESS_DEFINITION_NOT_FOUND'
  | 'SCOPE_NOT_FOUND'
  | 'JOB_NOT_ACTIVE'
  | 'JOB_NOT_ACTIVATED'
  | 'PROCESS_INSTANCE_NOT_ACTIVE'
  | 'INCIDENT_ALREADY_RESOLVED'
  | 'MESSAGE_DUPLICATE'
  | 'LOCAL_SCOPE_NOT_SUPPORTED'
  | 'RESULT_TIMEOUT'
  | 'DEPLOYMENT_REJECTED'
  | 'IDEMPOTENCY_KEY_REUSED'
  | 'PROCESS_HAS_NO_NONE_START_EVENT'
  | 'ENGINE_SHUTTING_DOWN';

/** findErrorInfo mengembalikan detail ErrorInfo dari error Connect, atau undefined. */
export function findErrorInfo(err: unknown): ErrorInfo | undefined {
  if (!(err instanceof ConnectError)) {
    return undefined;
  }
  return err.findDetails(ErrorInfoSchema)[0];
}

/** isReason mengembalikan true bila error membawa reason tertentu. */
export function isReason(err: unknown, reason: ErrorReason): boolean {
  return findErrorInfo(err)?.reason === reason;
}

/** findDeploymentRejection mengembalikan daftar issue penolakan deploy, atau undefined. */
export function findDeploymentRejection(err: unknown): DeploymentRejection | undefined {
  if (!(err instanceof ConnectError)) {
    return undefined;
  }
  return err.findDetails(DeploymentRejectionSchema)[0];
}
