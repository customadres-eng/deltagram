CREATE TABLE IF NOT EXISTS deltament_market_servers (
    id             BIGSERIAL PRIMARY KEY,
    slug           TEXT NOT NULL UNIQUE,
    name           TEXT NOT NULL,
    kind           TEXT NOT NULL,
    api_url        TEXT NOT NULL,
    secret         TEXT NOT NULL,
    contact        TEXT NOT NULL DEFAULT '',
    stars_per_gram DOUBLE PRECISION NOT NULL DEFAULT 0,
    status         TEXT NOT NULL DEFAULT 'pending',
    operator_hash  BYTEA NOT NULL UNIQUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS deltament_market_users (
    id             BIGSERIAL PRIMARY KEY,
    server_id      BIGINT NOT NULL REFERENCES deltament_market_servers(id),
    server_user_id BIGINT NOT NULL,
    phone          TEXT NOT NULL,
    name           TEXT NOT NULL DEFAULT '',
    username       TEXT NOT NULL DEFAULT '',
    balance        BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (server_id, server_user_id)
);

CREATE TABLE IF NOT EXISTS deltament_market_login_codes (
    id             BIGSERIAL PRIMARY KEY,
    server_id      BIGINT NOT NULL REFERENCES deltament_market_servers(id),
    phone          TEXT NOT NULL,
    server_user_id BIGINT NOT NULL,
    hash           BYTEA NOT NULL,
    attempts       INT NOT NULL DEFAULT 0,
    expires_at     TIMESTAMPTZ NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS deltament_market_login_codes_phone ON deltament_market_login_codes (server_id, phone, created_at DESC);

CREATE TABLE IF NOT EXISTS deltament_market_sessions (
    id         TEXT PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES deltament_market_users(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL UNIQUE,
    device     TEXT NOT NULL DEFAULT '',
    place      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS deltament_market_sessions_user ON deltament_market_sessions (user_id);

CREATE TABLE IF NOT EXISTS deltament_market_history (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES deltament_market_users(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,
    title      TEXT NOT NULL,
    amount     BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS deltament_market_history_user ON deltament_market_history (user_id, id DESC);

CREATE TABLE IF NOT EXISTS deltament_market_lots (
    id         BIGSERIAL PRIMARY KEY,
    server_id  BIGINT NOT NULL REFERENCES deltament_market_servers(id),
    kind       TEXT NOT NULL,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL,
    price      BIGINT NOT NULL CHECK (price > 0),
    status     TEXT NOT NULL,
    ends_at    TIMESTAMPTZ,
    gift       JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (server_id, kind, slug)
);
CREATE INDEX IF NOT EXISTS deltament_market_lots_auctions ON deltament_market_lots (ends_at) WHERE status = 'auction';

CREATE TABLE IF NOT EXISTS deltament_market_items (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES deltament_market_users(id) ON DELETE CASCADE,
    lot_id     BIGINT NOT NULL REFERENCES deltament_market_lots(id),
    kind       TEXT NOT NULL,
    title      TEXT NOT NULL,
    name       TEXT NOT NULL DEFAULT '',
    slug       TEXT NOT NULL DEFAULT '',
    num        INT NOT NULL DEFAULT 0,
    gift_id    BIGINT NOT NULL DEFAULT 0,
    amount     BIGINT NOT NULL DEFAULT 0,
    value      BIGINT NOT NULL,
    source     TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'owned',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS deltament_market_items_user ON deltament_market_items (user_id, id DESC);

CREATE TABLE IF NOT EXISTS deltament_market_bids (
    id         BIGSERIAL PRIMARY KEY,
    lot_id     BIGINT NOT NULL REFERENCES deltament_market_lots(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES deltament_market_users(id) ON DELETE CASCADE,
    masked     TEXT NOT NULL,
    amount     BIGINT NOT NULL,
    active     BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS deltament_market_bids_lot ON deltament_market_bids (lot_id, amount DESC);

CREATE TABLE IF NOT EXISTS deltament_market_watchlist (
    user_id BIGINT NOT NULL REFERENCES deltament_market_users(id) ON DELETE CASCADE,
    lot_id  BIGINT NOT NULL REFERENCES deltament_market_lots(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, lot_id)
);
