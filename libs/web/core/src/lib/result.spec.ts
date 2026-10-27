import { err, ok, toAppError } from './result';

describe('result', () => {
  it('wraps success and failure values', () => {
    expect(ok(42)).toEqual({ ok: true, value: 42 });
    expect(err({ code: 'x', message: 'boom' })).toEqual({
      ok: false,
      error: { code: 'x', message: 'boom' },
    });
  });

  it('converts thrown values into AppError', () => {
    expect(toAppError(new Error('fail'), 'network')).toEqual({ code: 'network', message: 'fail' });
    expect(toAppError('oops')).toEqual({ code: 'unknown', message: 'oops' });
  });
});
