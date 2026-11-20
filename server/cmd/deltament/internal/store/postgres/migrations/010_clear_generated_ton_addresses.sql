-- Early TON cards were created with a deterministic placeholder address. That
-- address is not a wallet the user controls, so the owner must never send real
-- TON to it. Clear every card that was never edited by its owner; the account
-- now has to enter a real Tonkeeper / MyTonWallet address before a payout.
UPDATE deltament_ton_cards
SET address = '', updated_at = now()
WHERE address <> ''
  AND updated_at <= created_at + interval '1 second';
