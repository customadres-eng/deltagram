-- DeltaMent rewards now live in the same TON ledger the native client reads
-- through payments.getStarsStatus (ton_balances). Move any balance that was
-- accrued in the marketplace-only table once, then stop writing it so the two
-- services cannot drift.
INSERT INTO ton_balances (user_id, balance_nanoton, granted)
SELECT user_id, ton_balance_nanoton, true
FROM deltament_balances
WHERE ton_balance_nanoton > 0
ON CONFLICT (user_id) DO UPDATE
SET balance_nanoton = ton_balances.balance_nanoton + EXCLUDED.balance_nanoton,
    updated_at = now();

UPDATE deltament_balances SET ton_balance_nanoton = 0 WHERE ton_balance_nanoton <> 0;
