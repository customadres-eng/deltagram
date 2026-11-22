package postgres

import (
	"context"
	"strings"

	"telesrv/cmd/deltament/internal/domain"
)

// CardRegistry returns every account that has a balance or a custom card. It is
// public by design: the native client renders other users' profile cards from
// this list, without asking MTProto for a special update.
func (db *DB) CardRegistry(ctx context.Context) ([]domain.CardPublic, error) {
	rows, err := db.q.Query(ctx, `
		SELECT u.id,
		       COALESCE(NULLIF(TRIM(u.first_name || ' ' || u.last_name), ''), u.username, ''),
		       COALESCE(u.username, ''),
		       COALESCE(tb.balance_nanoton, 0),
		       COALESCE(sb.balance, 0),
		       COALESCE(c.address, ''),
		       COALESCE(p.color_id, -1),
		       COALESCE(p.nft_row_id, 0),
		       COALESCE(p.nft_slug, ''),
		       COALESCE(p.nft_num, 0),
		       COALESCE(p.nft_x, 0.5),
		       COALESCE(p.nft_y, 0.5),
		       COALESCE(p.nft_scale, 0.231),
		       COALESCE(p.nft_rotation, 0)
		FROM users u
		LEFT JOIN ton_balances tb ON tb.user_id = u.id
		LEFT JOIN stars_balances sb ON sb.user_id = u.id
		LEFT JOIN deltament_card_prefs p ON p.user_id = u.id
		LEFT JOIN deltament_ton_cards c ON c.user_id = u.id
		WHERE u.deleted_at IS NULL
		  AND (COALESCE(tb.balance_nanoton, 0) > 0 OR COALESCE(sb.balance, 0) > 0 OR p.user_id IS NOT NULL)
		ORDER BY u.id`)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()
	out := make([]domain.CardPublic, 0, 64)
	for rows.Next() {
		var c domain.CardPublic
		if err := rows.Scan(&c.UserID, &c.Name, &c.Username, &c.TON, &c.Stars, &c.TONAddress,
			&c.ColorID, &c.NFTRowID, &c.NFTSlug, &c.NFTNum,
			&c.NFTX, &c.NFTY, &c.NFTScale, &c.NFTRotation); err != nil {
			return nil, wrap(err)
		}
		out = append(out, c)
	}
	return out, wrap(rows.Err())
}

func (db *DB) CardPrefs(ctx context.Context, userID int64) (domain.CardPrefs, error) {
	var p domain.CardPrefs
	err := db.q.QueryRow(ctx, `
		SELECT user_id, color_id, nft_row_id, nft_slug, nft_num,
		       nft_x, nft_y, nft_scale, nft_rotation, updated_at
		FROM deltament_card_prefs WHERE user_id = $1`, userID).
		Scan(&p.UserID, &p.ColorID, &p.NFTRowID, &p.NFTSlug, &p.NFTNum,
			&p.NFTX, &p.NFTY, &p.NFTScale, &p.NFTRotation, &p.UpdatedAt)
	return p, wrap(err)
}

func (db *DB) SetCardPrefs(ctx context.Context, p domain.CardPrefs) error {
	_, err := db.q.Exec(ctx, `
		INSERT INTO deltament_card_prefs
			(user_id, color_id, nft_row_id, nft_slug, nft_num,
			 nft_x, nft_y, nft_scale, nft_rotation, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		ON CONFLICT (user_id) DO UPDATE SET
			color_id = EXCLUDED.color_id,
			nft_row_id = EXCLUDED.nft_row_id,
			nft_slug = EXCLUDED.nft_slug,
			nft_num = EXCLUDED.nft_num,
			nft_x = EXCLUDED.nft_x,
			nft_y = EXCLUDED.nft_y,
			nft_scale = EXCLUDED.nft_scale,
			nft_rotation = EXCLUDED.nft_rotation,
			updated_at = now()`,
		p.UserID, p.ColorID, p.NFTRowID, p.NFTSlug, p.NFTNum,
		p.NFTX, p.NFTY, p.NFTScale, p.NFTRotation)
	return wrap(err)
}

func (db *DB) SetTONCardAddress(ctx context.Context, userID int64, address string) error {
	if _, err := db.q.Exec(ctx, `
		INSERT INTO deltament_ton_cards (user_id, address) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET address = EXCLUDED.address, updated_at = now()`, userID, address); err != nil {
		return wrap(err)
	}
	return nil
}

func (db *DB) PrimaryServerID(ctx context.Context) (int64, error) {
	var id int64
	err := db.q.QueryRow(ctx, `SELECT id FROM deltament_market_servers ORDER BY id LIMIT 1`).Scan(&id)
	return id, wrap(err)
}

func (db *DB) ResolveUsername(ctx context.Context, username string) (int64, error) {
	username = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(username), "@"))
	var id int64
	err := db.q.QueryRow(ctx, `
		SELECT u.id FROM users u
		WHERE u.deleted_at IS NULL AND (
			(u.username <> '' AND lower(u.username) = lower($1))
			OR EXISTS (
				SELECT 1 FROM peer_usernames p
				WHERE p.username_lower = lower($1)
				  AND p.peer_type = 'user' AND p.peer_id = u.id AND p.active
			)
			OR EXISTS (
				SELECT 1 FROM collectible_usernames c
				WHERE c.username_lower = lower($1)
				  AND c.owner_peer_type = 'user' AND c.owner_peer_id = u.id
				  AND c.status <> 'burned'
			)
		)
		LIMIT 1`, username).Scan(&id)
	return id, wrap(err)
}
