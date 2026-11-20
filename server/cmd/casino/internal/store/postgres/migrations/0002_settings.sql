-- Operator-tunable casino economics. A single row (id = 1) holds the payout
-- multiplier applied to every winning round: 100 keeps the designed RTP, 0
-- means wins pay nothing, 200 doubles every win. The value is read at
-- settlement time so a panel change takes effect on the next round.
CREATE TABLE IF NOT EXISTS casino_settings (
    id             smallint    PRIMARY KEY,
    payout_percent integer     NOT NULL DEFAULT 100,
    updated_at     timestamptz NOT NULL DEFAULT now(),
    updated_by     text        NOT NULL DEFAULT '',
    CONSTRAINT casino_settings_payout_range CHECK (payout_percent >= 0 AND payout_percent <= 500)
);

INSERT INTO casino_settings (id, payout_percent)
VALUES (1, 100)
ON CONFLICT (id) DO NOTHING;
