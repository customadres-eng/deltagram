-- TON cards are the payout destination of every DeltaMent account. Rewards no
-- longer turn into stars: they accumulate on the account's own TON card and are
-- sent out on request. One card per messenger account, keyed by users.id (the
-- same key deltament_balances uses).
CREATE TABLE IF NOT EXISTS deltament_ton_cards (
    user_id    BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    address    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Pending TON payouts. The user requests a send, the amount leaves the card
-- balance immediately, and the owner confirms it from the bot or the admin
-- panel once the on-chain transfer is done.
CREATE TABLE IF NOT EXISTS deltament_ton_payouts (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    address         TEXT NOT NULL,
    amount_nanoton  BIGINT NOT NULL CHECK (amount_nanoton > 0),
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'rejected')),
    note            TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at         TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS deltament_ton_payouts_status_idx
    ON deltament_ton_payouts(status, id DESC);
CREATE INDEX IF NOT EXISTS deltament_ton_payouts_user_idx
    ON deltament_ton_payouts(user_id, id DESC);

-- TON top-up orders created by the purchase bot (100 TON = 30 RUB). The owner
-- moves the order through contacted -> paid -> sent and the balance is credited
-- exactly once when it is marked paid.
CREATE TABLE IF NOT EXISTS deltament_ton_orders (
    id             BIGSERIAL PRIMARY KEY,
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    username       TEXT NOT NULL DEFAULT '',
    ton_amount     BIGINT NOT NULL CHECK (ton_amount > 0),
    rub_amount     BIGINT NOT NULL CHECK (rub_amount > 0),
    status         TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'contacted', 'paid', 'sent', 'cancelled')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS deltament_ton_orders_status_idx
    ON deltament_ton_orders(status, id DESC);
CREATE INDEX IF NOT EXISTS deltament_ton_orders_user_idx
    ON deltament_ton_orders(user_id, id DESC);
