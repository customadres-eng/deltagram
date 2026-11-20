-- Real artwork metadata for purchased gifts.
--
-- An inventory item for a gift must keep the same model/pattern/backdrop ids
-- as the original lot, otherwise the item card can only fall back to a
-- Fragment image that does not match DeltaGram-specific gifts.

ALTER TABLE deltament_market_items
    ADD COLUMN IF NOT EXISTS art jsonb NOT NULL DEFAULT '{}'::jsonb;
