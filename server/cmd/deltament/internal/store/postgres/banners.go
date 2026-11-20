package postgres

import (
	"context"
	"strconv"
	"time"

	"telesrv/cmd/deltament/internal/domain"
)

func (db *DB) BannerTokens(ctx context.Context) (map[int64]string, error) {
	rows, err := db.q.Query(ctx, `SELECT user_id, token FROM deltament_profile_banners`)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var token string
		if err := rows.Scan(&id, &token); err != nil {
			return nil, wrap(err)
		}
		out[id] = token
	}
	return out, wrap(rows.Err())
}

func (db *DB) UserBanner(ctx context.Context, userID int64) (domain.Banner, error) {
	var b domain.Banner
	err := db.q.QueryRow(ctx, `
		SELECT user_id, token, mime, bytes, updated_at
		FROM deltament_profile_banners WHERE user_id = $1`, userID).
		Scan(&b.UserID, &b.Token, &b.MIME, &b.Bytes, &b.UpdatedAt)
	return b, wrap(err)
}

func (db *DB) BannerToken(ctx context.Context, userID int64) (string, error) {
	var token string
	err := db.q.QueryRow(ctx, `SELECT token FROM deltament_profile_banners WHERE user_id = $1`, userID).Scan(&token)
	return token, wrap(err)
}

// SetUserBanner replaces the image and bumps the version token that clients
// use for cache busting. It also mirrors the token into users.avatar_banner so
// the registry can be built with one indexed scan.
func (db *DB) SetUserBanner(ctx context.Context, userID int64, mime string, data []byte) (string, error) {
	token := strconv.FormatInt(time.Now().UnixNano(), 36)
	// Mirroring the token into users proves the account exists before any
	// image bytes are written, so a stale marketplace session cannot create
	// banner rows for deleted accounts.
	if err := affected(db.q.Exec(ctx, `UPDATE public.users SET avatar_banner = $2, updated_at = now() WHERE id = $1 AND deleted_at IS NULL`, userID, token)); err != nil {
		return "", err
	}
	if _, err := db.q.Exec(ctx, `
		INSERT INTO deltament_profile_banners (user_id, token, mime, bytes, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (user_id) DO UPDATE
		SET token = EXCLUDED.token, mime = EXCLUDED.mime, bytes = EXCLUDED.bytes, updated_at = now()`,
		userID, token, mime, data); err != nil {
		return "", wrap(err)
	}
	return token, nil
}

func (db *DB) ClearUserBanner(ctx context.Context, userID int64) error {
	if _, err := db.q.Exec(ctx, `DELETE FROM deltament_profile_banners WHERE user_id = $1`, userID); err != nil {
		return wrap(err)
	}
	_, err := db.q.Exec(ctx, `UPDATE public.users SET avatar_banner = '', updated_at = now() WHERE id = $1`, userID)
	return wrap(err)
}
