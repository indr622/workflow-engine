/**
 * AppError adalah bentuk error standar lintas fitur, terlepas dari transport (Connect, HTTP).
 */
export interface AppError {
  code: string;
  message: string;
}

/**
 * Result dipakai lapisan domain/application untuk mengembalikan sukses atau gagal
 * tanpa melempar exception.
 */
export type Result<T, E = AppError> = { ok: true; value: T } | { ok: false; error: E };

export const ok = <T>(value: T): Result<T, never> => ({ ok: true, value });

export const err = <E = AppError>(error: E): Result<never, E> => ({ ok: false, error });

/** toAppError mengubah nilai apa pun yang dilempar menjadi AppError. */
export function toAppError(cause: unknown, fallbackCode = 'unknown'): AppError {
  if (cause instanceof Error) {
    return { code: fallbackCode, message: cause.message };
  }
  return { code: fallbackCode, message: String(cause) };
}
