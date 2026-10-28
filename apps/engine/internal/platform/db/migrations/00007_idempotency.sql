-- +goose Up
-- Klaim idempotensi RPC mutasi (ADR-0017). Primary key adalah (scope, key) dari klien; tanpa kolom
-- auto-increment.
CREATE TABLE idempotency_key (
    scope        TEXT NOT NULL,          -- tenant + nama RPC lengkap
    key          TEXT NOT NULL,          -- request_id dari klien
    request_hash BYTEA NOT NULL,         -- SHA-256 request proto deterministik, request_id dikosongkan
    response     BYTEA,                  -- response tersimpan; NULL hanya selama transaksi pengklaim berjalan
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (scope, key)
);
CREATE INDEX idempotency_key_expires_idx ON idempotency_key (expires_at);

-- +goose Down
DROP TABLE idempotency_key;
