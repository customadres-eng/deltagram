-- A username auction may be active while the source channel still owns its
-- public username. The collectible is minted only after a winning settlement.
ALTER TABLE public.deltament_auctions
    DROP CONSTRAINT IF EXISTS deltament_auctions_check;

ALTER TABLE public.deltament_auctions
    DROP CONSTRAINT IF EXISTS deltament_auctions_kind_check;

ALTER TABLE public.deltament_auctions
    ADD CONSTRAINT deltament_auctions_kind_check
    CHECK (kind IN ('username', 'phone'));

ALTER TABLE public.deltament_auctions
    ADD CONSTRAINT deltament_auctions_asset_shape_check
    CHECK ((kind = 'username' AND channel_id IS NOT NULL AND collectible_phone_id IS NULL)
        OR (kind = 'phone' AND collectible_phone_id IS NOT NULL AND channel_id IS NULL AND collectible_id IS NULL));

DROP INDEX IF EXISTS public.deltament_one_active_username_auction;
CREATE UNIQUE INDEX deltament_one_active_username_channel_auction
    ON public.deltament_auctions(channel_id) WHERE status = 'active' AND kind = 'username';

ALTER TABLE public.deltament_auctions
    ADD COLUMN IF NOT EXISTS settled_at timestamptz,
    ADD COLUMN IF NOT EXISTS settlement_key text;

CREATE UNIQUE INDEX IF NOT EXISTS deltament_auctions_settlement_key_uidx
    ON public.deltament_auctions(settlement_key) WHERE settlement_key IS NOT NULL;
