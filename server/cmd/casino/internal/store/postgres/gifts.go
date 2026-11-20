package postgres

import (
	"context"
	"encoding/json"
)

// Casino relayer: the fixed service account that escrows deposited NFTs. Seeded
// by migration 0001; the username is the source of truth with the id as a
// fallback for a renamed account.
const (
	relayerUsername = "casino_relayer"
	relayerUserID   = 1780243402
)

// RelayerUserID resolves the casino's NFT-escrow account id.
func (db *DB) RelayerUserID(ctx context.Context) (int64, error) {
	var id int64
	err := db.q.QueryRow(ctx, `SELECT id FROM public.users WHERE username = $1 AND deleted_at IS NULL LIMIT 1`, relayerUsername).Scan(&id)
	if err != nil {
		if wrap(err) == ErrNotFound {
			return relayerUserID, nil
		}
		return 0, wrap(err)
	}
	return id, nil
}

// GiftRow is the minimal identity of a deposited NFT.
type GiftRow struct {
	RowID    int64
	GiftID   int64
	UniqueID int64
	Num      int
	Title    string
	Slug     string
	OwnerID  int64
}

// OwnedNFT returns one unique (NFT) gift if it belongs to the given account.
// Regular catalog gifts are not deposit-able: they are untransferable.
func (db *DB) OwnedNFT(ctx context.Context, userID, rowID int64) (GiftRow, error) {
	var g GiftRow
	err := db.q.QueryRow(ctx, `
		SELECT g.id, g.gift_id, COALESCE(g.unique_gift_id, 0),
		       COALESCE(NULLIF(g.gift_num, 0), u.num, 0),
		       COALESCE(r.title, ''), COALESCE(u.slug, ''),
		       g.owner_peer_id
		FROM public.peer_star_gifts g
		JOIN public.star_gift_catalog_revisions r ON r.id = g.catalog_revision_id
		LEFT JOIN public.unique_star_gifts u ON u.id = g.unique_gift_id
		WHERE g.id = $2
		  AND g.owner_peer_type = 'user' AND g.owner_peer_id = $1
		  AND g.lifecycle_status = 'active' AND g.converted = false
		  AND g.unique_gift_id > 0`, userID, rowID).Scan(
		&g.RowID, &g.GiftID, &g.UniqueID, &g.Num, &g.Title, &g.Slug, &g.OwnerID)
	if err != nil {
		return g, wrap(err)
	}
	return g, nil
}

// moveGift transfers a gift row between two user accounts, keeping the unique
// gift aggregate owner in sync (a deferred trigger rejects owner mismatch).
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
		return ErrNotFound
	}
	if _, err := db.q.Exec(ctx, `
		UPDATE public.unique_star_gifts u
		SET owner_peer_type = 'user', owner_peer_id = $2, updated_at = now()
		WHERE u.id = (SELECT unique_gift_id FROM public.peer_star_gifts WHERE id = $1)`, rowID, toOwner); err != nil {
		return wrap(err)
	}
	return nil
}

// EscrowGift moves a deposited NFT to the relayer account.
func (db *DB) EscrowGift(ctx context.Context, rowID, ownerID, relayerID int64) error {
	return db.moveGift(ctx, rowID, ownerID, relayerID)
}

// ReleaseGift returns an NFT from the relayer to the withdrawing player.
func (db *DB) ReleaseGift(ctx context.Context, rowID, relayerID, ownerID int64) error {
	return db.moveGift(ctx, rowID, relayerID, ownerID)
}

// GiftOwner returns the current owner of a gift row (user owner id), or
// ErrNotFound if the row is not an active user-held gift.
func (db *DB) GiftOwner(ctx context.Context, rowID int64) (int64, error) {
	var owner int64
	err := db.q.QueryRow(ctx, `
		SELECT owner_peer_id FROM public.peer_star_gifts
		WHERE id = $1 AND owner_peer_type = 'user' AND lifecycle_status = 'active'`, rowID).Scan(&owner)
	if err != nil {
		return 0, wrap(err)
	}
	return owner, nil
}

// RecordEscrow inserts a deposit record for an escrowed NFT. The unique
// gift_row_id makes a duplicate deposit a no-op (ErrDuplicate).
func (db *DB) RecordEscrow(ctx context.Context, userID, giftRowID int64) error {
	_, err := db.q.Exec(ctx, `
		INSERT INTO casino_escrow (user_id, gift_row_id)
		VALUES ($1, $2)`, userID, giftRowID)
	return wrap(err)
}

// NftItem is a deposited NFT as shown in the player's casino inventory.
type NftItem struct {
	RowID int64  `json:"row_id"`
	Title string `json:"title"`
	Slug  string `json:"slug,omitempty"`
	Num   int    `json:"num,omitempty"`
}

// EscrowedNFTs returns the NFTs a player currently has deposited, newest first.
func (db *DB) EscrowedNFTs(ctx context.Context, userID int64) ([]NftItem, error) {
	rows, err := db.q.Query(ctx, `
		SELECT e.gift_row_id,
		       COALESCE(r.title, 'NFT'),
		       COALESCE(u.slug, ''),
		       COALESCE(NULLIF(g.gift_num, 0), u.num, 0)
		FROM casino_escrow e
		LEFT JOIN public.peer_star_gifts g ON g.id = e.gift_row_id
		LEFT JOIN public.star_gift_catalog_revisions r ON r.id = g.catalog_revision_id
		LEFT JOIN public.unique_star_gifts u ON u.id = g.unique_gift_id
		WHERE e.user_id = $1 AND e.status = 'escrowed'
		ORDER BY e.id`, userID)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()
	out := make([]NftItem, 0, 4)
	for rows.Next() {
		var item NftItem
		if err := rows.Scan(&item.RowID, &item.Title, &item.Slug, &item.Num); err != nil {
			return nil, wrap(err)
		}
		out = append(out, item)
	}
	return out, wrap(rows.Err())
}

// NftArtwork is the real rendering data of a deposited collectible gift: the
// Lottie model and pattern that the DeltaGram client itself plays, plus the
// backdrop palette. The casino renders the exact artwork instead of a generic
// placeholder.
type NftArtwork struct {
	Title      string          `json:"title"`
	Num        int             `json:"num,omitempty"`
	Model      json.RawMessage `json:"model"`
	Pattern    json.RawMessage `json:"pattern,omitempty"`
	Center     int64           `json:"center"`
	Edge       int64           `json:"edge"`
	PatternCol int64           `json:"pattern_color"`
	Text       int64           `json:"text"`
}

// NftArtworkOf returns the rendering data for one gift the player has
// deposited or won into the casino. It is scoped to the player so one account
// cannot enumerate another's inventory.
func (db *DB) NftArtworkOf(ctx context.Context, userID, giftRowID int64) (NftArtwork, error) {
	row := db.q.QueryRow(ctx, `
		SELECT COALESCE(r.title, 'NFT'),
		       COALESCE(NULLIF(g.gift_num, 0), u.num, 0),
		       COALESCE(m.animation_json, '{}'::jsonb),
		       COALESCE(p.animation_json, '{}'::jsonb),
		       COALESCE(b.center_color, 0),
		       COALESCE(b.edge_color, 0),
		       COALESCE(b.pattern_color, 0),
		       COALESCE(b.text_color, 0)
		FROM casino_escrow e
		JOIN public.peer_star_gifts g ON g.id = e.gift_row_id
		LEFT JOIN public.star_gift_catalog_revisions r ON r.id = g.catalog_revision_id
		LEFT JOIN public.unique_star_gifts u ON u.id = g.unique_gift_id
		LEFT JOIN public.star_gift_collectible_models m ON m.id = u.model_attribute_id
		LEFT JOIN public.star_gift_collectible_patterns p ON p.id = u.pattern_attribute_id
		LEFT JOIN public.star_gift_collectible_backdrops b ON b.id = u.backdrop_attribute_id
		WHERE e.user_id = $1 AND e.gift_row_id = $2 AND e.status = 'escrowed'`, userID, giftRowID)
	var art NftArtwork
	var model, pattern []byte
	if err := row.Scan(&art.Title, &art.Num, &model, &pattern, &art.Center, &art.Edge, &art.PatternCol, &art.Text); err != nil {
		return art, wrap(err)
	}
	art.Model = json.RawMessage(model)
	art.Pattern = json.RawMessage(pattern)
	return art, nil
}

// ReleaseEscrow marks a deposited NFT as withdrawn. It verifies the caller is
// the depositor so one player cannot release another's NFT.
func (db *DB) ReleaseEscrow(ctx context.Context, userID, giftRowID int64) error {
	tag, err := db.q.Exec(ctx, `
		UPDATE casino_escrow
		SET status = 'released', released_at = now()
		WHERE user_id = $1 AND gift_row_id = $2 AND status = 'escrowed'`, userID, giftRowID)
	if err != nil {
		return wrap(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
