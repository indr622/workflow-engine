import { z } from 'zod';

/** Variabel instance/job harus berupa objek JSON (bukan array atau nilai primitif). */
const variablesSchema = z.record(z.string(), z.unknown());

export type VariablesParseResult = { ok: true; json: string } | { ok: false; error: string };

/**
 * parseVariablesInput memvalidasi teks dari editor JSON. Teks kosong berarti {}.
 * Hasil `json` sudah dinormalisasi (tanpa spasi) dan siap dikirim ke engine.
 */
export function parseVariablesInput(text: string): VariablesParseResult {
  const trimmed = text.trim();
  if (trimmed === '') return { ok: true, json: '{}' };
  let value: unknown;
  try {
    value = JSON.parse(trimmed);
  } catch (e) {
    return { ok: false, error: `Invalid JSON: ${e instanceof Error ? e.message : String(e)}` };
  }
  const parsed = variablesSchema.safeParse(value);
  if (!parsed.success || Array.isArray(value)) {
    return { ok: false, error: 'Variables must be a JSON object, e.g. {"amount": 100}' };
  }
  return { ok: true, json: JSON.stringify(parsed.data) };
}

/** formatJsonValue merapikan nilai variabel JSON untuk ditampilkan; teks tidak valid dikembalikan apa adanya. */
export function formatJsonValue(value: string): string {
  try {
    return JSON.stringify(JSON.parse(value));
  } catch {
    return value;
  }
}
