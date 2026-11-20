package postgres

import (
	"context"
	"encoding/json"
	"time"
)

// Round is a persisted bet. The raw server seed is stored so the round can be
// replayed for settlement and revealed to the player afterwards.
type Round struct {
	ID             int64
	UserID         int64
	Game           string
	Stake          int64
	CommandID      string
	ServerSeed     []byte
	ServerSeedHash []byte
	ClientSeed     string
	Params         map[string]any
	Status         string // active | won | lost
	Payout         int64
	Outcome        map[string]any
	CreatedAt      time.Time
	SettledAt      *time.Time
}

// Round statuses.
const (
	RoundActive = "active"
	RoundWon    = "won"
	RoundLost   = "lost"
)

// CreateRound inserts a new round and returns it with its assigned id.
func (db *DB) CreateRound(ctx context.Context, r *Round) error {
	params, err := json.Marshal(r.Params)
	if err != nil {
		return err
	}
	return wrap(db.q.QueryRow(ctx, `
		INSERT INTO casino_rounds
			(user_id, game, stake, command_id, server_seed, server_seed_hash, client_seed, params, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at`,
		r.UserID, r.Game, r.Stake, r.CommandID, r.ServerSeed, r.ServerSeedHash, r.ClientSeed, params, r.Status,
	).Scan(&r.ID, &r.CreatedAt))
}

// GetRoundByCommand loads one round by its idempotency key.
func (db *DB) GetRoundByCommand(ctx context.Context, userID int64, commandID string) (Round, error) {
	var id int64
	if err := db.q.QueryRow(ctx, `SELECT id FROM casino_rounds WHERE command_id = $1 AND user_id = $2`, commandID, userID).Scan(&id); err != nil {
		return Round{}, wrap(err)
	}
	return db.GetRound(ctx, userID, id)
}

// GetRound loads one round owned by the given user.
func (db *DB) GetRound(ctx context.Context, userID, roundID int64) (Round, error) {
	var r Round
	var params, outcome []byte
	var settledAt *time.Time
	// command_id must be part of the projection: settlement derives the win
	// ledger key from it (commandID+":win"). Leaving it out made every
	// crash/mines payout share the single idempotency key ":win", so after the
	// first credited win all later payouts were silently swallowed by the
	// ledger's ON CONFLICT DO NOTHING guard.
	err := db.q.QueryRow(ctx, `
		SELECT id, user_id, game, stake, command_id, server_seed, server_seed_hash, client_seed,
		       params, status, payout, outcome, created_at, settled_at
		FROM casino_rounds WHERE id = $2 AND user_id = $1`, userID, roundID).Scan(
		&r.ID, &r.UserID, &r.Game, &r.Stake, &r.CommandID, &r.ServerSeed, &r.ServerSeedHash, &r.ClientSeed,
		&params, &r.Status, &r.Payout, &outcome, &r.CreatedAt, &settledAt)
	if err != nil {
		return r, wrap(err)
	}
	r.Params = map[string]any{}
	r.Outcome = map[string]any{}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &r.Params)
	}
	if len(outcome) > 0 {
		_ = json.Unmarshal(outcome, &r.Outcome)
	}
	r.SettledAt = settledAt
	return r, nil
}

// SettleRound finalizes a round with its result. It is a single atomic update,
// so a double settlement or a settlement by another player cannot happen.
func (db *DB) SettleRound(ctx context.Context, userID, roundID int64, status string, payout int64, outcome map[string]any) error {
	out, err := json.Marshal(outcome)
	if err != nil {
		return err
	}
	tag, err := db.q.Exec(ctx, `
		UPDATE casino_rounds
		SET status = $3, payout = $4, outcome = $5, settled_at = now()
		WHERE id = $2 AND user_id = $1 AND status = 'active'`,
		userID, roundID, status, payout, out)
	if err != nil {
		return wrap(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListRounds returns the player's recent rounds, newest first.
func (db *DB) ListRounds(ctx context.Context, userID int64, limit int) ([]Round, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := db.q.Query(ctx, `
		SELECT id, user_id, game, stake, status, payout, outcome, created_at, settled_at
		FROM casino_rounds WHERE user_id = $1 ORDER BY id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()
	out := make([]Round, 0, limit)
	for rows.Next() {
		var r Round
		var outcome []byte
		if err := rows.Scan(&r.ID, &r.UserID, &r.Game, &r.Stake, &r.Status, &r.Payout, &outcome, &r.CreatedAt, &r.SettledAt); err != nil {
			return nil, wrap(err)
		}
		if len(outcome) > 0 {
			r.Outcome = map[string]any{}
			_ = json.Unmarshal(outcome, &r.Outcome)
		}
		out = append(out, r)
	}
	return out, wrap(rows.Err())
}

// UpdateOutcome stores the intermediate state of an active round (for example,
// the cells revealed so far in a mines game) without settling it.
func (db *DB) UpdateOutcome(ctx context.Context, userID, roundID int64, outcome map[string]any) error {
	out, err := json.Marshal(outcome)
	if err != nil {
		return err
	}
	tag, err := db.q.Exec(ctx, `
		UPDATE casino_rounds SET outcome = $3
		WHERE id = $2 AND user_id = $1 AND status = 'active'`, userID, roundID, out)
	if err != nil {
		return wrap(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
