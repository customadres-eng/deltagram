package postgres

import (
	"context"

	"telesrv/cmd/deltament/internal/domain"
)

// DeltaMent relayer: the fixed service account that escrows marketplace gifts.
// It is seeded by migration 0225.
const relayerUsername = "deltament_relayer"

const relayerUserID = 1780243401

func (db *DB) RelayerUserID(ctx context.Context) (int64, error) {
	var id int64
	err := db.q.QueryRow(ctx, `SELECT id FROM public.users WHERE username = $1 AND deleted_at IS NULL LIMIT 1`, relayerUsername).Scan(&id)
	if err != nil {
		// The seeded account is authoritative, but a renamed username should not
		// break the marketplace silently.
		if wrapped := wrap(err); wrapped == domain.ErrNotFound {
			return relayerUserID, nil
		}
		return 0, wrap(err)
	}
	return id, nil
}

const giftAssetCols = `
SELECT g.id, g.gift_id, COALESCE(NULLIF(g.gift_num, 0), u.num, 0), COALESCE(g.unique_gift_id, 0),
       COALESCE(r.title, ''), COALESCE(u.slug, ''), g.gift_date,
       COALESCE(m.name, ''), COALESCE(b.name, ''), COALESCE(p.name, ''),
       COALESCE(u.model_attribute_id, 0), COALESCE(u.backdrop_attribute_id, 0), COALESCE(u.pattern_attribute_id, 0),
       COALESCE(b.center_color, 0), COALESCE(b.edge_color, 0), COALESCE(b.pattern_color, 0), COALESCE(b.text_color, 0)
FROM public.peer_star_gifts g
JOIN public.star_gift_catalog_revisions r ON r.id = g.catalog_revision_id
LEFT JOIN public.unique_star_gifts u ON u.id = g.unique_gift_id
LEFT JOIN public.star_gift_collectible_models m ON m.id = u.model_attribute_id
LEFT JOIN public.star_gift_collectible_backdrops b ON b.id = u.backdrop_attribute_id
LEFT JOIN public.star_gift_collectible_patterns p ON p.id = u.pattern_attribute_id`

const giftAssetQuery = giftAssetCols + `
WHERE g.owner_peer_type = 'user' AND g.owner_peer_id = $1
  AND g.lifecycle_status = 'active' AND g.converted = false`

func scanGiftAsset(row interface{ Scan(...any) error }) (domain.GiftAsset, error) {
	var g domain.GiftAsset
	if err := row.Scan(
		&g.RowID, &g.GiftID, &g.Num, &g.UniqueID, &g.Title, &g.Slug, &g.Date,
		&g.Model.Name, &g.Backdrop.Name, &g.Symbol.Name,
		&g.Art.ModelID, &g.Art.BackdropID, &g.Art.PatternID,
		&g.Art.Colors.Center, &g.Art.Colors.Edge, &g.Art.Colors.Pattern, &g.Art.Colors.Text,
	); err != nil {
		return g, wrap(err)
	}
	g.Unique = g.UniqueID > 0
	g.Art.Unique = g.Unique
	g.Art.GiftID = g.GiftID
	return g, nil
}

func (db *DB) OwnedGifts(ctx context.Context, userID int64, limit int) ([]domain.GiftAsset, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	rows, err := db.q.Query(ctx, giftAssetQuery+` ORDER BY g.gift_date DESC, g.id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()
	out := make([]domain.GiftAsset, 0, 16)
	for rows.Next() {
		g, err := scanGiftAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, wrap(rows.Err())
}

// OwnedGift returns one gift if it still belongs to the given account.
func (db *DB) OwnedGift(ctx context.Context, userID, rowID int64) (domain.GiftAsset, error) {
	return scanGiftAsset(db.q.QueryRow(ctx, giftAssetQuery+` AND g.id = $2`, userID, rowID))
}

// moveGift transfers a real gift row between two user accounts. It keeps the
// unique-gift aggregate in sync (a deferred trigger rejects owner mismatch)
// and stamps a synthetic positive msg_id, because peer_star_gifts requires
// msg_id > 0 for user-owned rows and the marketplace has no real message.
func (db *DB) moveGift(ctx context.Context, rowID, fromOwner, toOwner int64) error {
	tag, err := db.q.Exec(ctx, `
		UPDATE public.peer_star_gifts g
		SET owner_peer_type = 'user', owner_peer_id = $3,
		    msg_id = nextval('deltament_gift_message_seq'),
		    saved_id = 0, upgrade_msg_id = 0, pinned_order = 0,
		    unsaved = false, name_hidden = false
		WHERE g.id = $1 AND g.owner_peer_type = 'user' AND g.owner_peer_id = $2
		  AND g.lifecycle_status = 'active'`, rowID, fromOwner, toOwner)
	if err != nil {
		return wrap(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	if _, err := db.q.Exec(ctx, `
		UPDATE public.unique_star_gifts u
		SET owner_peer_type = 'user', owner_peer_id = $2, updated_at = now()
		WHERE u.id = (SELECT unique_gift_id FROM public.peer_star_gifts WHERE id = $1)`, rowID, toOwner); err != nil {
		return wrap(err)
	}
	return nil
}

// EscrowGift moves a gift to the relayer account.
func (db *DB) EscrowGift(ctx context.Context, rowID, ownerID, relayerID int64) error {
	return db.moveGift(ctx, rowID, ownerID, relayerID)
}

// ReleaseGift is the same move in the opposite direction: relayer -> buyer.
func (db *DB) ReleaseGift(ctx context.Context, rowID, relayerID, buyerID int64) error {
	return db.moveGift(ctx, rowID, relayerID, buyerID)
}

// GiftArtMeta returns the current artwork metadata of a gift row regardless
// of who owns it. Old marketplace lots only stored names in their JSON
// snapshot; the view layer merges this back in so a lot created before the
// artwork upgrade still renders the real model, pattern and backdrop.
func (db *DB) GiftArtMeta(ctx context.Context, rowID int64) (domain.GiftAsset, error) {
	return scanGiftAsset(db.q.QueryRow(ctx, giftAssetCols+` WHERE g.id = $1`, rowID))
}

func (db *DB) giftAnimation(ctx context.Context, query string, id int64) ([]byte, error) {
	var data []byte
	if err := db.q.QueryRow(ctx, query, id).Scan(&data); err != nil {
		return nil, wrap(err)
	}
	if len(data) == 0 {
		return nil, domain.ErrNotFound
	}
	return data, nil
}

// GiftCatalogAnimation is the Lottie animation of a regular (not upgraded)
// catalog gift. It is the real artwork DeltaGram itself renders for gifts
// that have no Fragment NFT image.
func (db *DB) GiftCatalogAnimation(ctx context.Context, giftID int64) ([]byte, error) {
	return db.giftAnimation(ctx, `
		SELECT animation_json FROM public.star_gift_catalog_revisions
		WHERE gift_id = $1 ORDER BY revision DESC LIMIT 1`, giftID)
}

// GiftModelAnimation is the Lottie of one collectible model attribute.
func (db *DB) GiftModelAnimation(ctx context.Context, id int64) ([]byte, error) {
	return db.giftAnimation(ctx, `SELECT animation_json FROM public.star_gift_collectible_models WHERE id = $1`, id)
}

// GiftPatternAnimation is the Lottie of one collectible pattern attribute.
func (db *DB) GiftPatternAnimation(ctx context.Context, id int64) ([]byte, error) {
	return db.giftAnimation(ctx, `SELECT animation_json FROM public.star_gift_collectible_patterns WHERE id = $1`, id)
}
