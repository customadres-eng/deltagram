-- A username may be sold more than once over time: the historical lot stays
-- for history, while only one auction/fixed-price lot may be active at a time.
ALTER TABLE deltament_market_lots
    DROP CONSTRAINT IF EXISTS deltament_market_lots_server_id_kind_slug_key;

CREATE UNIQUE INDEX IF NOT EXISTS deltament_market_lots_active_slug
    ON deltament_market_lots (server_id, kind, slug)
    WHERE status IN ('auction', 'sale');
