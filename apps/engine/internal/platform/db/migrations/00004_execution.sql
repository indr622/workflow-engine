-- +goose Up
-- Skema modul execution (Fase 2). Semua key adalah UUIDv7 yang dibuat engine (ADR-0011).
-- Tabel state adalah proyeksi dari execution_record dan ditulis dalam transaksi yang sama.

CREATE TABLE process_instance (
    key                        UUID PRIMARY KEY, -- = key element instance PROCESS
    tenant_id                  TEXT NOT NULL DEFAULT '<default>',
    process_definition_key     UUID NOT NULL REFERENCES process_definition (key),
    bpmn_process_id            TEXT NOT NULL,
    process_definition_version INT NOT NULL,
    state                      TEXT NOT NULL CHECK (state IN ('ACTIVE', 'COMPLETED', 'TERMINATED')),
    has_incident               BOOLEAN NOT NULL DEFAULT false,
    version                    BIGINT NOT NULL DEFAULT 0, -- naik setiap command
    start_time                 TIMESTAMPTZ NOT NULL,
    end_time                   TIMESTAMPTZ
);
CREATE INDEX process_instance_list_idx ON process_instance (tenant_id, key DESC);
CREATE INDEX process_instance_by_process_idx ON process_instance (tenant_id, bpmn_process_id, key DESC);
CREATE INDEX process_instance_active_idx ON process_instance (key) WHERE state = 'ACTIVE';

CREATE TABLE element_instance (
    key                  UUID PRIMARY KEY,
    process_instance_key UUID NOT NULL REFERENCES process_instance (key),
    flow_scope_key       UUID, -- NULL untuk PROCESS
    element_id           TEXT NOT NULL,
    bpmn_element_type    TEXT NOT NULL,
    state                TEXT NOT NULL,
    incident_key         UUID,
    start_time           TIMESTAMPTZ NOT NULL,
    end_time             TIMESTAMPTZ
);
CREATE INDEX element_instance_by_pi_idx ON element_instance (process_instance_key, key);
CREATE INDEX element_instance_runtime_idx ON element_instance (process_instance_key)
    WHERE state NOT IN ('COMPLETED', 'TERMINATED');

CREATE TABLE taken_sequence_flow (
    process_instance_key UUID NOT NULL REFERENCES process_instance (key),
    flow_scope_key       UUID NOT NULL,
    gateway_id           TEXT NOT NULL,
    sequence_flow_id     TEXT NOT NULL,
    count                INT NOT NULL CHECK (count > 0), -- baris dihapus saat count = 0
    PRIMARY KEY (flow_scope_key, gateway_id, sequence_flow_id)
);
CREATE INDEX taken_sequence_flow_by_pi_idx ON taken_sequence_flow (process_instance_key);

CREATE TABLE variable (
    key                  UUID PRIMARY KEY,
    process_instance_key UUID NOT NULL REFERENCES process_instance (key),
    scope_key            UUID NOT NULL,
    name                 TEXT NOT NULL,
    value                TEXT NOT NULL, -- teks JSON ringkas apa adanya (bukan JSONB: presisi angka dan urutan key)
    updated_at           TIMESTAMPTZ NOT NULL,
    UNIQUE (scope_key, name)
);
CREATE INDEX variable_by_pi_idx ON variable (process_instance_key);

CREATE TABLE job (
    key                        UUID PRIMARY KEY,
    process_instance_key       UUID NOT NULL REFERENCES process_instance (key),
    element_instance_key       UUID NOT NULL,
    element_id                 TEXT NOT NULL,
    bpmn_process_id            TEXT NOT NULL,
    process_definition_key     UUID NOT NULL,
    process_definition_version INT NOT NULL,
    type                       TEXT NOT NULL,
    retries                    INT NOT NULL,
    custom_headers             JSONB NOT NULL DEFAULT '[]',
    state                      TEXT NOT NULL, -- CREATED, COMPLETED, CANCELED (Fase 4 menambah)
    created_at                 TIMESTAMPTZ NOT NULL,
    updated_at                 TIMESTAMPTZ NOT NULL
);
CREATE INDEX job_activatable_idx ON job (type, key) WHERE state = 'CREATED'; -- untuk SKIP LOCKED Fase 4
CREATE INDEX job_by_pi_idx ON job (process_instance_key, key);
CREATE INDEX job_list_idx ON job (key DESC);

CREATE TABLE incident (
    key                  UUID PRIMARY KEY,
    process_instance_key UUID NOT NULL REFERENCES process_instance (key),
    element_instance_key UUID NOT NULL,
    element_id           TEXT NOT NULL,
    error_type           TEXT NOT NULL,
    error_message        TEXT NOT NULL,
    state                TEXT NOT NULL, -- CREATED, RESOLVED
    created_at           TIMESTAMPTZ NOT NULL,
    resolved_at          TIMESTAMPTZ
);
CREATE INDEX incident_by_pi_idx ON incident (process_instance_key, key);
CREATE INDEX incident_open_idx ON incident (process_instance_key) WHERE state = 'CREATED';

-- Log append-only. Urutan per instance memakai sequence yang dihitung engine (bukan id
-- auto-increment); PRIMARY KEY (process_instance_key, sequence) menjadi jaring pengaman bila
-- row lock pernah terlewati: dua writer akan bertabrakan, bukan menulis riwayat bercabang.
CREATE TABLE execution_record (
    process_instance_key UUID NOT NULL,
    sequence             INT NOT NULL CHECK (sequence > 0),
    record_key           UUID NOT NULL,
    value_type           TEXT NOT NULL,
    intent               TEXT NOT NULL,
    element_id           TEXT,
    payload              JSONB NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (process_instance_key, sequence)
);
CREATE INDEX execution_record_created_idx ON execution_record (created_at);

-- +goose Down
DROP TABLE execution_record;
DROP TABLE incident;
DROP TABLE job;
DROP TABLE variable;
DROP TABLE taken_sequence_flow;
DROP TABLE element_instance;
DROP TABLE process_instance;
