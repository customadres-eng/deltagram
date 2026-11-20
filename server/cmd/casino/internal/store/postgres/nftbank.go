package postgres

import (
	"context"
)

// Casino bank: the fixed account that owns the NFT prize pool. Seeded by
// migration 0001.
const (
	bankUsername = "casino_bank"
	bankUserID   = 1780243403
)

// BankUserID resolves the casino bank account id.
func (db *DB) BankUserID(ctx context.Context) (int64, error) {
	var id int64
	err := db.q.QueryRow(ctx, `SELECT id FROM public.users WHERE username = $1 AND deleted_at IS NULL LIMIT 1`, bankUsername).Scan(&id)
	if err != nil {
		if wrap(err) == ErrNotFound {
			return bankUserID, nil
		}
		return 0, wrap(err)
	}
	return id, nil
}

// GiftValuation returns the inputs needed to price a gift: the base collection
// star price, the model rarity permille, the backdrop rarity permille, and
// whether the backdrop is the top-tier black one.
type GiftValuation struct {
	BaseStars        int64
	ModelPermille    int
	BackdropPermille int
	BlackBackdrop    bool
}

// ReadGiftValuation loads the pricing inputs of one gift row.
func (db *DB) ReadGiftValuation(ctx context.Context, giftRowID int64) (GiftValuation, error) {
	var v GiftValuation
	var backdropName string
	err := db.q.QueryRow(ctx, `
		SELECT COALESCE(r.stars, 0),
		       COALESCE(m.rarity_permille, 0),
		       COALESCE(b.rarity_permille, 0),
		       COALESCE(b.name, '')
		FROM public.peer_star_gifts g
		JOIN public.star_gift_catalog_revisions r ON r.id = g.catalog_revision_id
		LEFT JOIN public.unique_star_gifts u ON u.id = g.unique_gift_id
		LEFT JOIN public.star_gift_collectible_models m ON m.id = u.model_attribute_id
		LEFT JOIN public.star_gift_collectible_backdrops b ON b.id = u.backdrop_attribute_id
		WHERE g.id = $1`, giftRowID).Scan(&v.BaseStars, &v.ModelPermille, &v.BackdropPermille, &backdropName)
	if err != nil {
		return v, wrap(err)
	}
	v.BlackBackdrop = backdropName == "Black"
	return v, nil
}

// AddBankNFT registers a bank-held gift in the prize pool with its priced value.
func (db *DB) AddBankNFT(ctx context.Context, giftRowID, value int64) error {
	_, err := db.q.Exec(ctx, `
		INSERT INTO casino_bank_nfts (gift_row_id, value)
		VALUES ($1, $2)
		ON CONFLICT (gift_row_id) DO UPDATE SET value = EXCLUDED.value`, giftRowID, value)
	return wrap(err)
}

// PickBankNFT selects the most valuable available prize at or below maxValue,
// locking it against a concurrent award.
func (db *DB) PickBankNFT(ctx context.Context, maxValue int64) (giftRowID, value int64, err error) {
	err = db.q.QueryRow(ctx, `
		SELECT gift_row_id, value FROM casino_bank_nfts
		WHERE status = 'available' AND value <= $1
		ORDER BY value DESC
		LIMIT 1
		FOR UPDATE SKIP LOCKED`, maxValue).Scan(&giftRowID, &value)
	if err != nil {
		return 0, 0, wrap(err)
	}
	return giftRowID, value, nil
}

// AwardBankNFT marks a bank prize as awarded to a player.
func (db *DB) AwardBankNFT(ctx context.Context, giftRowID, userID int64) error {
	tag, err := db.q.Exec(ctx, `
		UPDATE casino_bank_nfts
		SET status = 'awarded', awarded_to = $2, awarded_at = now()
		WHERE gift_row_id = $1 AND status = 'available'`, giftRowID, userID)
	if err != nil {
		return wrap(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
