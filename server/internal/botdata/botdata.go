// Package botdata exposes the small read-only slice of account data that the
// standalone DeltaGram bots need: star balances, profile gift counters and
// message totals. The bots are external MTProto clients and cannot read another
// user's payments state through MTProto (payments.getStarsStatus is self-only),
// so they read the authoritative rows directly.
//
// Everything here is read-only and deliberately narrow. Writes that must stay
// transactional (for example collectible gift upgrades) go through the server
// process instead, never through this package.
package botdata

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrUserNotFound is returned when the requested account does not exist (or
// was deleted).
var ErrUserNotFound = errors.New("botdata: user not found")

// Reader is a small read-only PostgreSQL pool.
type Reader struct {
	pool *pgxpool.Pool
}

// Open builds a Reader. An empty DSN is a configuration error, not a runtime
// outage: callers decide whether to start the bot without stats.
func Open(ctx context.Context, dsn string) (*Reader, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, errors.New("botdata: postgres dsn is empty")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("botdata: parse postgres dsn: %w", err)
	}
	// These bots answer a handful of commands per minute; a tiny pool keeps the
	// footprint bounded without ever becoming a bottleneck.
	if cfg.MaxConns < 2 || cfg.MaxConns > 4 {
		cfg.MaxConns = 4
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("botdata: open postgres: %w", err)
	}
	return &Reader{pool: pool}, nil
}

// Close releases the pool.
func (r *Reader) Close() {
	if r != nil && r.pool != nil {
		r.pool.Close()
	}
}

// GiftCounts is the profile gift breakdown used by stats and mog battles.
type GiftCounts struct {
	// Total counts gifts currently displayed in the profile (unsaved gifts are
	// hidden from the profile and excluded).
	Total int64
	// Collectible counts upgraded (unique) gifts.
	Collectible int64
	// BlackBackdrop counts upgraded gifts whose backdrop is one of the black
	// palette entries ("Black", "Onyx Black", ...).
	BlackBackdrop int64
}

// MogStats is the comparison input for a mog battle.
type MogStats struct {
	UserID int64
	Stars  int64
	Gifts  GiftCounts
}

// UserStats is everything the Iris /статистика reply shows.
type UserStats struct {
	UserID       int64
	Username     string
	FirstName    string
	LastName     string
	CreatedAt    time.Time
	PremiumUntil time.Time
	Stars        int64
	Gifts        GiftCounts
	// PrivateMessages counts messages the user sent in private chats.
	PrivateMessages int64
	// ChannelMessages counts messages the user sent in groups and channels.
	ChannelMessages int64
}

// Messages is the total number of messages the user sent.
func (s UserStats) Messages() int64 { return s.PrivateMessages + s.ChannelMessages }

// Stats returns the personal statistics for one account.
func (r *Reader) Stats(ctx context.Context, userID int64) (UserStats, error) {
	out := UserStats{UserID: userID}
	if r == nil || r.pool == nil {
		return out, errors.New("botdata: reader is not configured")
	}
	if userID <= 0 {
		return out, ErrUserNotFound
	}
	var premium *time.Time
	err := r.pool.QueryRow(ctx, `
SELECT u.username, u.first_name, u.last_name, u.created_at, u.premium_expires_at,
	COALESCE(sb.balance, 0)::bigint
FROM users u
LEFT JOIN stars_balances sb ON sb.user_id = u.id
WHERE u.id = $1 AND u.deleted_at IS NULL`, userID).Scan(
		&out.Username, &out.FirstName, &out.LastName, &out.CreatedAt, &premium, &out.Stars)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrUserNotFound
	}
	if err != nil {
		return out, fmt.Errorf("botdata: load user: %w", err)
	}
	if premium != nil {
		out.PremiumUntil = *premium
	}
	if out.Gifts, err = r.giftCounts(ctx, userID); err != nil {
		return out, err
	}
	if err := r.pool.QueryRow(ctx, `
SELECT
	(SELECT count(*) FROM private_messages WHERE sender_user_id = $1),
	(SELECT count(*) FROM channel_messages WHERE sender_user_id = $1 AND NOT deleted)`,
		userID).Scan(&out.PrivateMessages, &out.ChannelMessages); err != nil {
		return out, fmt.Errorf("botdata: count messages: %w", err)
	}
	return out, nil
}

// Mog compares the two battle inputs. Both accounts must exist.
func (r *Reader) Mog(ctx context.Context, userID int64) (MogStats, error) {
	out := MogStats{UserID: userID}
	if r == nil || r.pool == nil {
		return out, errors.New("botdata: reader is not configured")
	}
	if userID <= 0 {
		return out, ErrUserNotFound
	}
	err := r.pool.QueryRow(ctx, `
SELECT COALESCE(sb.balance, 0)::bigint
FROM users u
LEFT JOIN stars_balances sb ON sb.user_id = u.id
WHERE u.id = $1 AND u.deleted_at IS NULL`, userID).Scan(&out.Stars)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrUserNotFound
	}
	if err != nil {
		return out, fmt.Errorf("botdata: load stars: %w", err)
	}
	out.Gifts, err = r.giftCounts(ctx, userID)
	if err != nil {
		return out, err
	}
	return out, nil
}

// giftCounts aggregates the user's visible profile gifts exactly once. The
// backdrop join only matches upgraded gifts: regular gifts have no collectible
// attribute row, so they never count as "black background".
func (r *Reader) giftCounts(ctx context.Context, userID int64) (GiftCounts, error) {
	var out GiftCounts
	err := r.pool.QueryRow(ctx, `
SELECT
	count(*)::bigint,
	count(*) FILTER (WHERE g.unique_gift_id IS NOT NULL)::bigint,
	count(*) FILTER (WHERE b.name ILIKE '%black%')::bigint
FROM peer_star_gifts g
LEFT JOIN unique_star_gifts ug ON ug.id = g.unique_gift_id
LEFT JOIN star_gift_collectible_backdrops b ON b.id = ug.backdrop_attribute_id
WHERE g.owner_peer_type = 'user' AND g.owner_peer_id = $1
	AND g.lifecycle_status = 'active' AND NOT g.unsaved`, userID).Scan(
		&out.Total, &out.Collectible, &out.BlackBackdrop)
	if err != nil {
		return GiftCounts{}, fmt.Errorf("botdata: count gifts: %w", err)
	}
	return out, nil
}
