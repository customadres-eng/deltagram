-- Track whether a lot's underlying username/channel asset has already been
-- moved into the operator vault. Resale lots (a buyer re-lists the item they
-- just bought) are already escrowed; settling them must only pay the seller,
-- never mint GRAM and never revoke the name from a third party.
ALTER TABLE deltament_market_lots
    ADD COLUMN IF NOT EXISTS escrowed BOOLEAN NOT NULL DEFAULT false;

-- Existing lots that already changed hands once had their asset escrowed by
-- the old settlement path. Their rows may still say seller_user_id = 0, which
-- is exactly the "operator lot" shape, so they are safe to leave as-is.
