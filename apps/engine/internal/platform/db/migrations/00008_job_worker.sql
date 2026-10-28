-- +goose Up
-- Protokol job worker (ADR-0020). Lease (worker, deadline, activated_at, activation_count,
-- timeout_count) sengaja tidak tercatat di execution_record; kolom lain diturunkan dari record.
ALTER TABLE job
    ADD COLUMN activatable_at   TIMESTAMPTZ,          -- NULL = segera; diisi FailJob dengan backoff
    ADD COLUMN worker           TEXT,
    ADD COLUMN deadline         TIMESTAMPTZ,          -- lease; NULL bila tidak ACTIVATED
    ADD COLUMN activated_at     TIMESTAMPTZ,
    ADD COLUMN activation_count INT NOT NULL DEFAULT 0,
    ADD COLUMN timeout_count    INT NOT NULL DEFAULT 0,
    ADD COLUMN error_message    TEXT,                 -- FailJob/ThrowError terakhir
    ADD COLUMN error_code       TEXT;                 -- ThrowError

ALTER TABLE job ADD CONSTRAINT job_state_check
    CHECK (state IN ('CREATED', 'ACTIVATED', 'COMPLETED', 'FAILED', 'ERROR_THROWN', 'CANCELED'));
ALTER TABLE job ADD CONSTRAINT job_lease_check
    CHECK ((state = 'ACTIVATED') = (deadline IS NOT NULL));

-- Antrean aktivasi: FIFO per tipe menurut (activatable_at, key); NULL diurutkan lebih dulu.
DROP INDEX job_activatable_idx;
CREATE INDEX job_activatable_idx ON job (type, activatable_at NULLS FIRST, key) WHERE state = 'CREATED';
CREATE INDEX job_deadline_idx ON job (deadline) WHERE state = 'ACTIVATED';

-- Incident milik job (JOB_NO_RETRIES, UNHANDLED_ERROR_EVENT).
ALTER TABLE incident ADD COLUMN job_key UUID;
CREATE INDEX incident_list_idx ON incident (key DESC);

-- Buffer message (ADR-0022). Korelasi ke subscription menyusul di Fase 5.
CREATE TABLE message (
    key             UUID PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT '<default>',
    name            TEXT NOT NULL,
    correlation_key TEXT NOT NULL,
    message_id      TEXT,                     -- NULL = tanpa dedup
    variables       JSON NOT NULL DEFAULT '{}',
    published_at    TIMESTAMPTZ NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL      -- published_at + TTL; TTL 0 tidak disimpan
);
CREATE UNIQUE INDEX message_dedup_idx ON message (tenant_id, name, correlation_key, message_id)
    WHERE message_id IS NOT NULL;
CREATE INDEX message_correlate_idx ON message (tenant_id, name, correlation_key, published_at);
CREATE INDEX message_expires_idx ON message (expires_at);

-- +goose Down
DROP TABLE message;
DROP INDEX incident_list_idx;
ALTER TABLE incident DROP COLUMN job_key;
DROP INDEX job_deadline_idx;
DROP INDEX job_activatable_idx;
UPDATE job SET state = 'CREATED' WHERE state = 'ACTIVATED';
UPDATE job SET state = 'CANCELED' WHERE state IN ('FAILED', 'ERROR_THROWN');
CREATE INDEX job_activatable_idx ON job (type, key) WHERE state = 'CREATED';
ALTER TABLE job DROP CONSTRAINT job_lease_check, DROP CONSTRAINT job_state_check;
ALTER TABLE job DROP COLUMN activatable_at, DROP COLUMN worker, DROP COLUMN deadline,
    DROP COLUMN activated_at, DROP COLUMN activation_count, DROP COLUMN timeout_count,
    DROP COLUMN error_message, DROP COLUMN error_code;
