-- +goose Up
-- Sequence key global milik platform (internal/platform/keygen); dipakai semua modul.
CREATE SEQUENCE engine_key_seq;

CREATE TABLE deployment (
    key        BIGINT PRIMARY KEY,
    tenant_id  TEXT NOT NULL DEFAULT '<default>',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE deployment_resource (
    id             BIGSERIAL PRIMARY KEY,
    deployment_key BIGINT NOT NULL REFERENCES deployment (key),
    resource_name  TEXT NOT NULL,
    content        BYTEA NOT NULL,
    checksum       BYTEA NOT NULL,
    UNIQUE (deployment_key, resource_name)
);

CREATE TABLE process_definition (
    key             BIGINT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT '<default>',
    bpmn_process_id TEXT NOT NULL,
    version         INT NOT NULL CHECK (version > 0),
    version_tag     TEXT NOT NULL DEFAULT '',
    name            TEXT NOT NULL DEFAULT '',
    resource_id     BIGINT NOT NULL REFERENCES deployment_resource (id),
    checksum        BYTEA NOT NULL,
    deployment_key  BIGINT NOT NULL REFERENCES deployment (key),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, bpmn_process_id, version)
);

CREATE INDEX process_definition_latest_idx
    ON process_definition (tenant_id, bpmn_process_id, version DESC);

-- +goose Down
DROP TABLE process_definition;
DROP TABLE deployment_resource;
DROP TABLE deployment;
DROP SEQUENCE engine_key_seq;
