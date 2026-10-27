-- +goose Up
-- Migrasi awal: tabel metadata engine. Tabel bisnis tiap modul ditambahkan di fase berikutnya.
CREATE TABLE engine_metadata (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO engine_metadata (key, value) VALUES ('schema_owner', 'workflow-engine');

-- +goose Down
DROP TABLE engine_metadata;
