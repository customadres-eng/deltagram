CREATE TABLE IF NOT EXISTS public.deltament_topup_intents (
    id uuid PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES public.users(id) ON DELETE RESTRICT,
    currency text NOT NULL,
    amount bigint NOT NULL CHECK (amount > 0),
    status text NOT NULL DEFAULT 'pending',
    provider_charge_id text,
    provider_message_id bigint,
    expires_at timestamptz NOT NULL,
    settled_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT deltament_topup_intents_currency_check CHECK (currency IN ('XTR', 'TON')),
    CONSTRAINT deltament_topup_intents_status_check CHECK (status IN ('pending', 'paid', 'expired', 'failed'))
);

CREATE UNIQUE INDEX IF NOT EXISTS deltament_topup_intents_provider_charge_uidx
    ON public.deltament_topup_intents(provider_charge_id)
    WHERE provider_charge_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS deltament_topup_intents_user_status_idx
    ON public.deltament_topup_intents(user_id, status, created_at DESC);
