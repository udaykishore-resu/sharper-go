-- +goose Up
-- One row per (client, key), stored as ledger.ScopedKey.
-- The middleware claims a key as 'inflight' (Technique 52);
-- store.Transfer flips it to 'done' inside the transfer's own
-- transaction (Technique 57).
CREATE TABLE idem (
    key          text        PRIMARY KEY,
    state        text        NOT NULL DEFAULT 'done'
                 CHECK (state IN ('inflight', 'done')),
    req_hash     bytea,
    status       integer     NOT NULL DEFAULT 201,
    response     bytea,
    locked_until timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

-- Technique 58: events commit with the change that caused them.
CREATE TABLE outbox (
    id         bigserial PRIMARY KEY,
    topic      text        NOT NULL,
    payload    jsonb       NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    sent_at    timestamptz
);
CREATE INDEX outbox_unsent ON outbox (id)
    WHERE sent_at IS NULL;

-- +goose Down
DROP TABLE outbox;
DROP TABLE idem;
