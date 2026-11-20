// Package service wires the casino's game engine, wallet and store into the
// HTTP-facing operations.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"telesrv/cmd/casino/internal/network"
	"telesrv/cmd/casino/internal/store/postgres"
)

// Wallet moves value across the casino boundary. Stars cross through the main
// server's Admin API; NFTs move directly between the player and the relayer
// account. Every star movement is idempotent: the same command id can be
// replayed and will never double-apply, which is what makes a retry after a
// transient network failure safe.
type Wallet struct {
	db           *postgres.DB
	net          *network.Client
	chipsPerStar int64
	actor        string
}

// NewWallet builds a wallet against the store and the star gateway.
func NewWallet(db *postgres.DB, net *network.Client, chipsPerStar int64, actor string) *Wallet {
	return &Wallet{db: db, net: net, chipsPerStar: chipsPerStar, actor: actor}
}

// commandID derives a stable idempotency key from an action and the caller's
// op key. The same (action, key) always maps to the same id, so a retry is a
// no-op on both the casino ledger and the main server's command log.
func commandID(actor, action, key string) string {
	sum := sha256.Sum256([]byte(action + ":" + key))
	return actor + "-" + hex.EncodeToString(sum[:])[:40]
}

// DepositStars debits the player's stars on the main server and credits the
// equivalent casino chips. Returns the resulting chip balance.
//
// Order matters: stars are debited first, chips credited second. If the chip
// credit fails after the star debit, the player has temporarily "lost" stars
// and a retry (same key) credits the chips without re-debiting. The reverse
// order would mint free chips on a failure, which is the dangerous direction.
func (w *Wallet) DepositStars(ctx context.Context, userID, stars int64, key string) (int64, error) {
	if stars <= 0 {
		return 0, errors.New("stars must be positive")
	}
	if !w.net.Enabled() {
		return 0, network.ErrStarsDisabled
	}
	id := commandID(w.actor, "deposit", key)
	if err := w.net.DebitStars(ctx, id, "Casino deposit", userID, stars); err != nil {
		return 0, err
	}
	return w.db.Apply(ctx, userID, stars*w.chipsPerStar, "deposit", id)
}

// WithdrawStars debits the player's casino chips and credits whole stars on the
// main server. Returns the resulting chip balance.
func (w *Wallet) WithdrawStars(ctx context.Context, userID, stars int64, key string) (int64, error) {
	if stars <= 0 {
		return 0, errors.New("stars must be positive")
	}
	if !w.net.Enabled() {
		return 0, network.ErrStarsDisabled
	}
	id := commandID(w.actor, "withdraw", key)
	balance, err := w.db.Apply(ctx, userID, -(stars * w.chipsPerStar), "withdraw", id)
	if err != nil {
		return 0, err
	}
	if err := w.net.GrantStars(ctx, id, "Casino withdrawal", userID, stars); err != nil {
		return 0, err
	}
	return balance, nil
}

// DepositNFT escrows one of the player's NFTs to the relayer account so it can
// be used as casino collateral. Only unique (NFT) gifts can be deposited.
func (w *Wallet) DepositNFT(ctx context.Context, userID, giftRowID int64) error {
	return w.db.InTx(ctx, func(tx *postgres.DB) error {
		if _, err := tx.OwnedNFT(ctx, userID, giftRowID); err != nil {
			return err
		}
		relayerID, err := tx.RelayerUserID(ctx)
		if err != nil {
			return err
		}
		if err := tx.EscrowGift(ctx, giftRowID, userID, relayerID); err != nil {
			return err
		}
		return tx.RecordEscrow(ctx, userID, giftRowID)
	})
}

// WithdrawNFT returns an NFT to the player. A deposited NFT is held by the
// relayer and must be moved back; a won NFT is already owned by the player (it
// was transferred straight from the bank on award), so it only needs its escrow
// record released.
func (w *Wallet) WithdrawNFT(ctx context.Context, userID, giftRowID int64) error {
	return w.db.InTx(ctx, func(tx *postgres.DB) error {
		relayerID, err := tx.RelayerUserID(ctx)
		if err != nil {
			return err
		}
		ownerID, err := tx.GiftOwner(ctx, giftRowID)
		if err != nil {
			return err
		}
		switch ownerID {
		case userID:
			// already owned — won in the casino, just release the escrow.
		case relayerID:
			if err := tx.ReleaseGift(ctx, giftRowID, relayerID, userID); err != nil {
				return err
			}
		default:
			return postgres.ErrNotFound
		}
		return tx.ReleaseEscrow(ctx, userID, giftRowID)
	})
}

// EscrowedNFTs lists the NFTs a player currently has deposited.
func (w *Wallet) EscrowedNFTs(ctx context.Context, userID int64) ([]postgres.NftItem, error) {
	return w.db.EscrowedNFTs(ctx, userID)
}

// NftArtwork returns the real Lottie artwork of a deposited NFT so the Mini App
// can draw the gift exactly as DeltaGram does instead of a placeholder.
func (w *Wallet) NftArtwork(ctx context.Context, userID, giftRowID int64) (postgres.NftArtwork, error) {
	return w.db.NftArtworkOf(ctx, userID, giftRowID)
}

// DebitChips removes chips from a player's balance (a bet), returning the new
// balance or ErrNoFunds. Idempotent via commandID.
func (w *Wallet) DebitChips(ctx context.Context, userID, chips int64, kind, commandID string) (int64, error) {
	if chips <= 0 {
		return 0, errors.New("chips must be positive")
	}
	return w.db.Apply(ctx, userID, -chips, kind, commandID)
}

// CreditChips adds chips to a player's balance (a win). Idempotent via commandID.
func (w *Wallet) CreditChips(ctx context.Context, userID, chips int64, kind, commandID string) (int64, error) {
	if chips <= 0 {
		return 0, errors.New("chips must be positive")
	}
	return w.db.Apply(ctx, userID, chips, kind, commandID)
}

// Balance returns a player's casino balance in chips.
func (w *Wallet) Balance(ctx context.Context, userID int64) (int64, error) {
	return w.db.Balance(ctx, userID)
}

// StarBalance returns the player's star balance on the main server, which is
// what a deposit spends.
func (w *Wallet) StarBalance(ctx context.Context, userID int64) (int64, error) {
	return w.db.StarBalance(ctx, userID)
}

// ChipsPerStar exposes the chip/star rate for amount conversions.
func (w *Wallet) ChipsPerStar() int64 { return w.chipsPerStar }

// StarsForChips floors a chip amount to whole stars.
func (w *Wallet) StarsForChips(chips int64) int64 {
	if chips < 0 {
		return 0
	}
	return chips / w.chipsPerStar
}

// ChipsForStars converts whole stars to chips.
func (w *Wallet) ChipsForStars(stars int64) int64 {
	if stars <= 0 {
		return 0
	}
	if stars > (1<<62)/w.chipsPerStar {
		panic(fmt.Sprintf("stars overflow: %d", stars))
	}
	return stars * w.chipsPerStar
}
