-- +goose Up
CREATE TABLE accounts (
    id         text        PRIMARY KEY,
    balance    bigint      NOT NULL DEFAULT 0,
    frozen     boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    -- only the house account may go negative
    CONSTRAINT balance_non_negative
        CHECK (balance >= 0 OR id = 'house')
);

INSERT INTO accounts (id) VALUES ('house');

CREATE TABLE transfers (
    id         text        PRIMARY KEY,
    from_acct  text        NOT NULL REFERENCES accounts (id),
    to_acct    text        NOT NULL REFERENCES accounts (id),
    amount     bigint      NOT NULL CHECK (amount > 0),
    idem_key   text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX transfers_from ON transfers (from_acct, created_at);
CREATE INDEX transfers_to ON transfers (to_acct, created_at);

-- +goose Down
DROP TABLE transfers;
DROP TABLE accounts;
