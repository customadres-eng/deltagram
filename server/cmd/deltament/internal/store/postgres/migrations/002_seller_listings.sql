-- User-owned listings: channels and collectible usernames.
-- A zero seller_user_id means an operator-created lot (existing behaviour).
ALTER TABLE deltament_market_lots
    ADD COLUMN IF NOT EXISTS seller_user_id BIGINT REFERENCES deltament_market_users(id),
    ADD COLUMN IF NOT EXISTS channel_id BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS username_asset_id BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS asset_kind TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS deltament_market_lots_seller
    ON deltament_market_lots (seller_user_id, id DESC)
    WHERE seller_user_id IS NOT NULL;
