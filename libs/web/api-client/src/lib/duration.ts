import { create } from '@bufbuild/protobuf';
import { DurationSchema, type Duration } from '@bufbuild/protobuf/wkt';

/** durationFromMs mengubah milidetik menjadi google.protobuf.Duration. */
export function durationFromMs(ms: number): Duration {
  if (!Number.isFinite(ms) || ms < 0) {
    throw new RangeError(`duration must be a non-negative finite number of milliseconds, got ${ms}`);
  }
  const whole = Math.floor(ms);
  return create(DurationSchema, {
    seconds: BigInt(Math.floor(whole / 1000)),
    nanos: (whole % 1000) * 1_000_000,
  });
}

/** durationToMs mengubah google.protobuf.Duration menjadi milidetik (dibulatkan ke bawah). */
export function durationToMs(d: Duration | undefined): number {
  if (!d) {
    return 0;
  }
  return Number(d.seconds) * 1000 + Math.floor(d.nanos / 1_000_000);
}
