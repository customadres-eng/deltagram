package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Settings is the operator-tunable casino economy. PayoutPercent scales every
// winning payout: 100 leaves the designed RTP untouched, 0 pays nothing and
// 200 doubles every win.
type Settings struct {
	PayoutPercent int
	UpdatedAt     string
	UpdatedBy     string
}

// DefaultPayoutPercent is used when the settings row is unreachable so a
// transient read error never turns into a "pay double" or "pay nothing" round.
const DefaultPayoutPercent = 100

// Settings returns the current casino economy row. A missing row or a read
// error falls back to the designed 100% so settlement always has a value.
func (db *DB) Settings(ctx context.Context) Settings {
	out := Settings{PayoutPercent: DefaultPayoutPercent}
	err := db.q.QueryRow(ctx, `
		SELECT payout_percent, to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), updated_by
		FROM casino_settings WHERE id = 1`).Scan(&out.PayoutPercent, &out.UpdatedAt, &out.UpdatedBy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			out.PayoutPercent = DefaultPayoutPercent
			out.UpdatedAt = ""
			out.UpdatedBy = ""
			return out
		}
		return Settings{PayoutPercent: DefaultPayoutPercent}
	}
	if out.PayoutPercent < 0 || out.PayoutPercent > 500 {
		out.PayoutPercent = DefaultPayoutPercent
	}
	return out
}

// PayoutPercent is the hot path used by round settlement.
func (db *DB) PayoutPercent(ctx context.Context) int {
	return db.Settings(ctx).PayoutPercent
}
