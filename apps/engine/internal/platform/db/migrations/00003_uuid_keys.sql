-- +goose Up
-- Key semua entity menjadi UUIDv7 yang dibuat engine (ADR-0011), menggantikan sequence global
-- engine_key_seq dan BIGSERIAL deployment_resource.id. Data lama dipertahankan: key BIGINT
-- dikonversi deterministik ke UUID (32 digit heksadesimal berawalan nol), sehingga key lama tetap
-- unik, relasi antar tabel tetap cocok, dan semuanya terurut sebelum UUIDv7 baru.
ALTER TABLE process_definition
    DROP CONSTRAINT process_definition_resource_id_fkey,
    DROP CONSTRAINT process_definition_deployment_key_fkey;
ALTER TABLE deployment_resource
    DROP CONSTRAINT deployment_resource_deployment_key_fkey;

ALTER TABLE deployment
    ALTER COLUMN key TYPE UUID USING lpad(to_hex(key), 32, '0')::uuid;

ALTER TABLE deployment_resource
    ALTER COLUMN id DROP DEFAULT,
    ALTER COLUMN id TYPE UUID USING lpad(to_hex(id), 32, '0')::uuid,
    ALTER COLUMN deployment_key TYPE UUID USING lpad(to_hex(deployment_key), 32, '0')::uuid;
DROP SEQUENCE IF EXISTS deployment_resource_id_seq;

ALTER TABLE process_definition
    ALTER COLUMN key TYPE UUID USING lpad(to_hex(key), 32, '0')::uuid,
    ALTER COLUMN resource_id TYPE UUID USING lpad(to_hex(resource_id), 32, '0')::uuid,
    ALTER COLUMN deployment_key TYPE UUID USING lpad(to_hex(deployment_key), 32, '0')::uuid;

ALTER TABLE deployment_resource
    ADD CONSTRAINT deployment_resource_deployment_key_fkey
        FOREIGN KEY (deployment_key) REFERENCES deployment (key);
ALTER TABLE process_definition
    ADD CONSTRAINT process_definition_resource_id_fkey
        FOREIGN KEY (resource_id) REFERENCES deployment_resource (id),
    ADD CONSTRAINT process_definition_deployment_key_fkey
        FOREIGN KEY (deployment_key) REFERENCES deployment (key);

DROP SEQUENCE engine_key_seq;

-- +goose Down
-- UUIDv7 tidak bisa dikembalikan ke BIGINT tanpa kehilangan identitas; migrasi ini sengaja satu arah.
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'migration 00003_uuid_keys is irreversible';
END
$$;
-- +goose StatementEnd
