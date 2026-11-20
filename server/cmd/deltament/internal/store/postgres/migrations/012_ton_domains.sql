-- DeltaGram .ton domain registry.
--
-- Domains are an off-chain ledger asset, like the other DeltaMent items: the
-- balance lives in ton_balances and the owner settles real on-chain TON DNS
-- names manually, exactly like TON payouts. This keeps the in-app flow fast and
-- free of wallet addresses while still giving every user a real, persistent
-- name they can sell or keep.
CREATE TABLE IF NOT EXISTS ton_domains (
    name               TEXT PRIMARY KEY,
    owner_user_id      BIGINT NOT NULL,
    acquired_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_price_nanoton BIGINT NOT NULL DEFAULT 0,
    listed             BOOLEAN NOT NULL DEFAULT FALSE,
    ask_nanoton        BIGINT NOT NULL DEFAULT 0,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ton_domains_listed_idx
    ON ton_domains (ask_nanoton) WHERE listed;

CREATE INDEX IF NOT EXISTS ton_domains_owner_idx
    ON ton_domains (owner_user_id);
