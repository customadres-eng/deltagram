DROP INDEX IF EXISTS public.deltament_auctions_settlement_key_uidx;
ALTER TABLE public.deltament_auctions DROP COLUMN IF EXISTS settlement_key, DROP COLUMN IF EXISTS settled_at;
DROP INDEX IF EXISTS public.deltament_one_active_username_channel_auction;
CREATE UNIQUE INDEX deltament_one_active_username_auction
    ON public.deltament_auctions(collectible_id) WHERE status = 'active' AND kind = 'username';
ALTER TABLE public.deltament_auctions DROP CONSTRAINT IF EXISTS deltament_auctions_asset_shape_check;
ALTER TABLE public.deltament_auctions DROP CONSTRAINT IF EXISTS deltament_auctions_kind_check;
ALTER TABLE public.deltament_auctions ADD CONSTRAINT deltament_auctions_kind_check CHECK (kind IN ('username', 'phone'));
