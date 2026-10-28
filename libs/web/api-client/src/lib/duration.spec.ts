import { describe, expect, it } from 'vitest';
import { durationFromMs, durationToMs } from './duration';

describe('duration', () => {
  it('round-trips milliseconds', () => {
    for (const ms of [0, 1, 999, 1000, 1500, 55_000, 86_400_000]) {
      expect(durationToMs(durationFromMs(ms))).toBe(ms);
    }
  });

  it('splits seconds and nanos', () => {
    const d = durationFromMs(2500);
    expect(d.seconds).toBe(2n);
    expect(d.nanos).toBe(500_000_000);
  });

  it('treats a missing duration as zero and rejects invalid input', () => {
    expect(durationToMs(undefined)).toBe(0);
    expect(() => durationFromMs(-1)).toThrow(RangeError);
    expect(() => durationFromMs(Number.NaN)).toThrow(RangeError);
  });
});
