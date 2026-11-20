package service

import (
	"context"
	"errors"

	"telesrv/cmd/casino/internal/domain"
	"telesrv/cmd/casino/internal/store/postgres"
)

// NftBank manages the casino's NFT prize pool. A won NFT is a real gift held on
// the casino bank account; awarding it moves the gift to the player and records
// it in the player's casino inventory so it can be withdrawn like any deposit.
type NftBank struct {
	db *postgres.DB
}

// NewNftBank builds the prize-pool manager.
func NewNftBank(db *postgres.DB) *NftBank { return &NftBank{db: db} }

// AddNft registers a gift already owned by the bank account as an available
// prize, pricing it from its base collection and model/backdrop rarity.
func (b *NftBank) AddNft(ctx context.Context, giftRowID int64) (int64, error) {
	v, err := b.db.ReadGiftValuation(ctx, giftRowID)
	if err != nil {
		return 0, err
	}
	value := domain.NftValue(v.BaseStars, v.ModelPermille, v.BackdropPermille, v.BlackBackdrop)
	if value <= 0 {
		return 0, errors.New("gift has no value")
	}
	if err := b.db.AddBankNFT(ctx, giftRowID, value); err != nil {
		return 0, err
	}
	return value, nil
}

// AwardNft picks the most valuable prize at or below targetValue and transfers
// it to the player's casino inventory. It returns the gift row id and its
// priced value, or an error if the pool has nothing affordable at that price.
func (b *NftBank) AwardNft(ctx context.Context, userID, targetValue int64) (int64, int64, error) {
	var giftRowID, value int64
	err := b.db.InTx(ctx, func(tx *postgres.DB) error {
		var err error
		giftRowID, value, err = b.AwardNftTx(ctx, tx, userID, targetValue)
		return err
	})
	if err != nil {
		return 0, 0, err
	}
	return giftRowID, value, nil
}

// AwardNftTx is AwardNft running inside the caller's transaction, so an award
// can be committed atomically with the round that produced it.
func (b *NftBank) AwardNftTx(ctx context.Context, tx *postgres.DB, userID, targetValue int64) (int64, int64, error) {
	bankID, err := tx.BankUserID(ctx)
	if err != nil {
		return 0, 0, err
	}
	giftRowID, value, err := tx.PickBankNFT(ctx, targetValue)
	if err != nil {
		return 0, 0, err
	}
	// Move the real gift from the bank account to the player, then record it in
	// the player's withdrawable casino inventory.
	if err := tx.ReleaseGift(ctx, giftRowID, bankID, userID); err != nil {
		return 0, 0, err
	}
	if err := tx.RecordEscrow(ctx, userID, giftRowID); err != nil {
		return 0, 0, err
	}
	if err := tx.AwardBankNFT(ctx, giftRowID, userID); err != nil {
		return 0, 0, err
	}
	return giftRowID, value, nil
}
