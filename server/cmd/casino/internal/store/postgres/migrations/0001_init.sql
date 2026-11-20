-- Casino service: schema + NFT relayer seed.
--
-- The casino stores its own balance ledger and round history here, and reuses
-- the shared telesrv `peer_star_gifts` / `unique_star_gifts` tables for NFT
-- escrow. A dedicated service account ("casino_relayer") holds deposited NFTs
-- so a gift cannot be hidden or converted while the casino has it, exactly the
-- way the Deltament relayer escrows marketplace gifts.

INSERT INTO public.users (
    id, access_hash, phone, first_name, last_name, username, country_code,
    created_at, updated_at, verified, support, about, last_seen_at,
    default_history_ttl_period, is_bot, bot_info_version, premium_expires_at,
    emoji_status_document_id, emoji_status_until, color_set, color,
    color_background_emoji_id, profile_color_set, profile_color,
    profile_color_background_emoji_id
) VALUES (
    1780243402, 1780243402001, '', 'Casino relayer', '', 'casino_relayer', '',
    now(), now(), false, false,
    'Служебный аккаунт казино: хранит NFT до вывода игроком.',
    0, 0, false, 0, NULL, 0, 0, false, 0, 0, false, 0, 0
)
ON CONFLICT (id) DO UPDATE SET
    access_hash = EXCLUDED.access_hash,
    first_name = EXCLUDED.first_name,
    last_name = EXCLUDED.last_name,
    username = EXCLUDED.username,
    about = EXCLUDED.about,
    updated_at = now();

-- Casino balance per player, denominated in integer "chips" (see config for the
-- chip<->star rate). Balance is authoritative and must never go negative.
CREATE TABLE IF NOT EXISTS casino_accounts (
    user_id    bigint PRIMARY KEY,
    balance    bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Append-only balance movements. command_id is the idempotency key: a retried
-- deposit/withdraw/bet can never double-apply.
CREATE TABLE IF NOT EXISTS casino_ledger (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint NOT NULL,
    amount     bigint NOT NULL,
    kind       text NOT NULL,
    command_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS casino_ledger_command_uidx ON casino_ledger (command_id);
CREATE INDEX IF NOT EXISTS casino_ledger_user_idx ON casino_ledger (user_id, id);

-- One row per bet. The server-seed hash is the pre-round commitment shown to the
-- player; the raw seed is revealed (and recorded) only after settlement so the
-- player can verify the outcome from the two seeds.
CREATE TABLE IF NOT EXISTS casino_rounds (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id          bigint NOT NULL,
    game             text NOT NULL,
    stake            bigint NOT NULL,
    command_id       text NOT NULL UNIQUE,
    server_seed      bytea NOT NULL,
    server_seed_hash bytea NOT NULL,
    client_seed      text NOT NULL,
    params           jsonb NOT NULL DEFAULT '{}'::jsonb,
    status           text NOT NULL DEFAULT 'active',
    payout           bigint NOT NULL DEFAULT 0,
    outcome          jsonb,
    created_at       timestamptz NOT NULL DEFAULT now(),
    settled_at       timestamptz
);
CREATE INDEX IF NOT EXISTS casino_rounds_user_idx ON casino_rounds (user_id, id);

-- NFT deposits: a gift row currently held by the relayer on the player's
-- behalf. `status` flips to 'released' once withdrawn.
CREATE TABLE IF NOT EXISTS casino_escrow (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     bigint NOT NULL,
    gift_row_id bigint NOT NULL UNIQUE,
    status      text NOT NULL DEFAULT 'escrowed',
    created_at  timestamptz NOT NULL DEFAULT now(),
    released_at timestamptz
);
CREATE INDEX IF NOT EXISTS casino_escrow_user_idx ON casino_escrow (user_id, id);

-- The casino "bank" account owns the NFT prize pool. A won NFT is a real gift
-- moved from this account to the player, so it renders with its actual model
-- and backdrop and can be withdrawn like any deposit.
INSERT INTO public.users (
    id, access_hash, phone, first_name, last_name, username, country_code,
    created_at, updated_at, verified, support, about, last_seen_at,
    default_history_ttl_period, is_bot, bot_info_version, premium_expires_at,
    emoji_status_document_id, emoji_status_until, color_set, color,
    color_background_emoji_id, profile_color_set, profile_color,
    profile_color_background_emoji_id
) VALUES (
    1780243403, 1780243403001, '', 'Casino bank', '', 'casino_bank', '',
    now(), now(), false, false,
    'Служебный аккаунт казино: пул NFT-призов, раздаваемых победителям.',
    0, 0, false, 0, NULL, 0, 0, false, 0, 0, false, 0, 0
)
ON CONFLICT (id) DO UPDATE SET
    access_hash = EXCLUDED.access_hash,
    first_name = EXCLUDED.first_name,
    last_name = EXCLUDED.last_name,
    username = EXCLUDED.username,
    about = EXCLUDED.about,
    updated_at = now();

-- NFT prize pool: one row per gift the bank holds, with its priced value.
CREATE TABLE IF NOT EXISTS casino_bank_nfts (
    gift_row_id bigint PRIMARY KEY,
    value       bigint NOT NULL,
    status      text NOT NULL DEFAULT 'available',
    awarded_to  bigint,
    created_at  timestamptz NOT NULL DEFAULT now(),
    awarded_at  timestamptz
);
CREATE INDEX IF NOT EXISTS casino_bank_nfts_value_idx ON casino_bank_nfts (value);
