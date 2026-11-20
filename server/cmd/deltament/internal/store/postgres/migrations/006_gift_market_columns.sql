-- Gift marketplace columns.
--
-- Gifts listed for sale are escrowed on the DeltaMent relayer account, so a
-- lot and the item minted for the buyer both have to remember which
-- peer_star_gifts row they represent. 004/005 created the banner storage and
-- the synthetic msg_id sequence; these two columns complete the gift flow.

ALTER TABLE deltament_market_lots
    ADD COLUMN IF NOT EXISTS gift_row_id bigint NOT NULL DEFAULT 0;

ALTER TABLE deltament_market_items
    ADD COLUMN IF NOT EXISTS gift_row_id bigint NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS deltament_market_lots_gift_row_idx
    ON deltament_market_lots (gift_row_id)
    WHERE gift_row_id > 0;

CREATE INDEX IF NOT EXISTS deltament_market_items_gift_row_idx
    ON deltament_market_items (gift_row_id)
    WHERE gift_row_id > 0;
