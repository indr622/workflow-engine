-- +goose Up
-- Fase 3: penanda proyeksi untuk verifikasi (last_sequence, updated_at), snapshot state runtime,
-- dan index untuk verifikasi dalam serta retensi (ADR-0016, ADR-0019).
ALTER TABLE process_instance
    ADD COLUMN last_sequence INT NOT NULL DEFAULT 0,
    ADD COLUMN updated_at    TIMESTAMPTZ NOT NULL DEFAULT now();

-- Backfill dari record. Tabel masih kecil saat Fase 3 di-merge; di produksi dengan data besar,
-- backfill seperti ini dipecah per batch (docs/ops/migrations.md).
UPDATE process_instance pi
SET last_sequence = r.max_seq, version = r.max_seq, updated_at = r.last_at
FROM (SELECT process_instance_key, MAX(sequence) AS max_seq, MAX(created_at) AS last_at
      FROM execution_record GROUP BY process_instance_key) r
WHERE r.process_instance_key = pi.key;

CREATE INDEX process_instance_updated_idx ON process_instance (updated_at) WHERE state = 'ACTIVE';
CREATE INDEX process_instance_retention_idx ON process_instance (end_time) WHERE state <> 'ACTIVE';

-- Snapshot state runtime instance aktif berumur panjang; rebuild mulai dari sini.
CREATE TABLE execution_snapshot (
    process_instance_key UUID PRIMARY KEY REFERENCES process_instance (key) ON DELETE CASCADE,
    sequence             INT NOT NULL,  -- state setelah record ini di-Apply
    format_version       INT NOT NULL,  -- versi codec; versi tak dikenal → rebuild dari nol
    state                JSONB NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Payload record disimpan sebagai JSON (teks apa adanya), bukan JSONB: JSONB menormalkan isi
-- (spasi, urutan key object, key ganda) sehingga nilai variabel hasil replay berbeda dari input.
ALTER TABLE execution_record ALTER COLUMN payload TYPE JSON USING payload::json;

-- +goose Down
ALTER TABLE execution_record ALTER COLUMN payload TYPE JSONB USING payload::jsonb;
DROP TABLE execution_snapshot;
DROP INDEX process_instance_retention_idx;
DROP INDEX process_instance_updated_idx;
ALTER TABLE process_instance DROP COLUMN updated_at, DROP COLUMN last_sequence;
