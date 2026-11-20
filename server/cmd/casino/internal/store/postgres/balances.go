package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Balance returns a player's casino balance in chips, creating a zero row on
// first read so callers can always distinguish "new player" from an error.
func (db *DB) Balance(ctx context.Context, userID int64) (int64, error) {
	var balance int64
	if _, err := db.q.Exec(ctx, `
		INSERT INTO casino_accounts (user_id, balance)
		VALUES ($1, 0)
		ON CONFLICT (user_id) DO NOTHING`, userID); err != nil {
		return 0, wrap(err)
	}
	if err := db.q.QueryRow(ctx, `SELECT balance FROM casino_accounts WHERE user_id = $1`, userID).Scan(&balance); err != nil {
		return 0, wrap(err)
	}
	return balance, nil
}

// StarBalance returns the player's star balance on the main DeltaGram server.
// Stars live in the shared schema, so the casino reads the authoritative number
// directly instead of round-tripping through the Admin API. A player who never
// received a star has no row, which reads as zero.
func (db *DB) StarBalance(ctx context.Context, userID int64) (int64, error) {
	var stars int64
	err := db.q.QueryRow(ctx, `SELECT balance FROM public.stars_balances WHERE user_id = $1`, userID).Scan(&stars)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, wrap(err)
	}
	return stars, nil
}

// Apply posts a signed amount to a player's balance and appends one ledger row.
// The ledger's unique command_id makes it idempotent: a retry with the same
// command_id is a no-op that returns the current balance without re-applying.
//
// Debits are guarded against driving the balance negative: if the player does
// not hold enough chips the update affects no rows and ErrNoFunds is returned.
//
// It runs inside the caller's transaction (see InTx) so a balance change and
// its matching external side-effect (star movement, NFT move) commit together.
func (db *DB) Apply(ctx context.Context, userID, amount int64, kind, commandID string) (int64, error) {
	if amount == 0 {
		return db.Balance(ctx, userID)
	}
	var applied bool
	if err := db.q.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO casino_ledger (user_id, amount, kind, command_id)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (command_id) DO NOTHING
			RETURNING id
		)
		SELECT EXISTS (SELECT 1 FROM ins)`, userID, amount, kind, commandID).Scan(&applied); err != nil {
		return 0, wrap(err)
	}
	if applied {
		var balance int64
		err := db.q.QueryRow(ctx, `
			UPDATE casino_accounts
			SET balance = balance + $2, updated_at = now()
			WHERE user_id = $1 AND ($2 > 0 OR balance + $2 >= 0)
			RETURNING balance`, userID, amount).Scan(&balance)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, ErrNoFunds
			}
			return 0, wrap(err)
		}
		return balance, nil
	}
	// Idempotent replay: the amount was already applied under this command id.
	return db.Balance(ctx, userID)
}
