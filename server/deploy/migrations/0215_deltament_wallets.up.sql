CREATE TABLE IF NOT EXISTS public.deltament_balances (
    user_id bigint PRIMARY KEY REFERENCES public.users(id) ON DELETE CASCADE,
    stars_balance bigint NOT NULL DEFAULT 0 CHECK (stars_balance >= 0),
    ton_balance_nanoton bigint NOT NULL DEFAULT 0 CHECK (ton_balance_nanoton >= 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS public.deltament_topup_transfers (
    idempotency_key text PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES public.users(id) ON DELETE RESTRICT,
    currency text NOT NULL CHECK (currency IN ('XTR', 'TON')),
    amount bigint NOT NULL CHECK (amount > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS public.deltament_balance_transactions (
    id bigserial PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES public.users(id) ON DELETE RESTRICT,
    currency text NOT NULL CHECK (currency IN ('XTR', 'TON')),
    amount bigint NOT NULL CHECK (amount <> 0),
    reason text NOT NULL,
    asset text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS deltament_balance_transactions_user_idx
    ON public.deltament_balance_transactions(user_id, id DESC);
