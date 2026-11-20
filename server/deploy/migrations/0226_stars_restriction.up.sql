-- Stars-only restriction: a narrower, reversible operator flag that blocks a
-- user from spending or moving stars (star gifts, paid reactions, star
-- transfers, Deltament deposits/withdrawals) WITHOUT freezing the account, so
-- messaging and the rest of the profile keep working normally.
ALTER TABLE account_restrictions ADD COLUMN stars_frozen boolean NOT NULL DEFAULT false;
