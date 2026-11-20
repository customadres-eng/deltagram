-- Deltament auctions are separate from collectible inventory. A collectible
-- username becomes visible in the marketplace only after its owner explicitly
-- creates one of these rows.
CREATE TABLE IF NOT EXISTS public.deltament_auctions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('username', 'phone')),
    collectible_id bigint REFERENCES public.collectible_usernames(id) ON DELETE RESTRICT,
    collectible_phone_id bigint REFERENCES public.collectible_phones(id) ON DELETE RESTRICT,
    channel_id bigint REFERENCES public.channels(id) ON DELETE RESTRICT,
    seller_user_id bigint NOT NULL REFERENCES public.users(id) ON DELETE RESTRICT,
    asset_name text NOT NULL,
    currency text NOT NULL CHECK (currency IN ('XTR', 'TON')),
    minimum_bid bigint NOT NULL CHECK (minimum_bid > 0),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'cancelled', 'sold', 'settled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    ends_at timestamptz NOT NULL,
    CHECK ((kind = 'username' AND collectible_id IS NOT NULL AND channel_id IS NOT NULL AND collectible_phone_id IS NULL)
        OR (kind = 'phone' AND collectible_phone_id IS NOT NULL AND channel_id IS NULL AND collectible_id IS NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS deltament_one_active_username_auction
    ON public.deltament_auctions(collectible_id) WHERE status = 'active' AND kind = 'username';
CREATE UNIQUE INDEX IF NOT EXISTS deltament_one_active_phone_auction
    ON public.deltament_auctions(collectible_phone_id) WHERE status = 'active' AND kind = 'phone';
CREATE INDEX IF NOT EXISTS deltament_auctions_catalog_idx
    ON public.deltament_auctions(status, currency, minimum_bid, created_at DESC);
