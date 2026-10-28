-- +goose Up
-- Outbox transaksional milik platform (ADR-0015). Event ditulis dalam transaksi command yang sama
-- dengan state, lalu dikirim relay ke consumer durable. Tanpa kolom auto-increment: event_id adalah
-- UUIDv7 dari generator engine (ADR-0011), tx_id adalah transaksi penulis.
CREATE TABLE outbox_event (
    event_id       UUID PRIMARY KEY,
    tx_id          XID8 NOT NULL DEFAULT pg_current_xact_id(), -- dasar pembacaan bebas celah
    topic          TEXT NOT NULL,                              -- "execution.job_created"
    aggregate_type TEXT NOT NULL,                              -- "process_instance", "process_definition"
    aggregate_key  UUID NOT NULL,
    aggregate_seq  INT NOT NULL DEFAULT 0,                     -- sequence record pemicu; 0 bila tidak relevan
    payload        JSONB NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX outbox_event_order_idx ON outbox_event (tx_id, event_id);
CREATE INDEX outbox_event_created_idx ON outbox_event (created_at);

-- Kursor per consumer durable: posisi (tx_id, event_id) event terakhir yang sudah diproses.
CREATE TABLE outbox_cursor (
    consumer   TEXT PRIMARY KEY,
    tx_id      XID8 NOT NULL,
    event_id   UUID NOT NULL,
    attempts   INT NOT NULL DEFAULT 0, -- percobaan gagal berturut-turut pada event di depan kursor
    last_error TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE outbox_dead_letter (
    consumer      TEXT NOT NULL,
    event_id      UUID NOT NULL,
    topic         TEXT NOT NULL,
    aggregate_key UUID NOT NULL,
    payload       JSONB NOT NULL,
    error         TEXT NOT NULL,
    attempts      INT NOT NULL,
    failed_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);

-- +goose Down
DROP TABLE outbox_dead_letter;
DROP TABLE outbox_cursor;
DROP TABLE outbox_event;
