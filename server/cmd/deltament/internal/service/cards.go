package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"telesrv/cmd/deltament/internal/domain"
	"telesrv/cmd/deltament/internal/network"
	"telesrv/cmd/deltament/internal/secret"
	"telesrv/cmd/deltament/internal/store"
)

const (
	CardColorStock  = -1
	CardColorRed    = 0
	CardColorGold   = 1
	CardColorBlue   = 2
	CardColorGreen  = 3
	CardColorPurple = 4
	CardColorBlack  = 5
)

type cardStore interface {
	store.TON
	store.Gifts
	store.CardPrefsStore
	store.TONDomains
	store.TxRunner
	PrimaryServerID(ctx context.Context) (int64, error)
}

// Cards is the public DeltaGram card registry plus the P2P transfer rail.
type Cards struct {
	db   cardStore
	nets Networks
	log  *slog.Logger
}

func NewCards(db cardStore, nets Networks, log *slog.Logger) *Cards {
	return &Cards{db: db, nets: nets, log: log}
}

func (c *Cards) Registry(ctx context.Context) ([]domain.CardPublic, error) {
	return c.db.CardRegistry(ctx)
}

func (c *Cards) Prefs(ctx context.Context, userID int64) (domain.CardPrefs, error) {
	p, err := c.db.CardPrefs(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.CardPrefs{UserID: userID, ColorID: CardColorStock}, nil
	}
	return p, err
}

// SetPrefs validates the colour. Gift NFTs belong in the profile, not as card stickers.
func (c *Cards) SetPrefs(ctx context.Context, userID int64, colorID int, nftRowID int64, nftX, nftY, nftScale, nftRotation float64) (domain.CardPrefs, error) {
	if userID <= 0 {
		return domain.CardPrefs{}, fmt.Errorf("%w: user is required", domain.ErrInvalid)
	}
	if colorID < CardColorStock || colorID > CardColorBlack {
		return domain.CardPrefs{}, fmt.Errorf("%w: unknown card colour", domain.ErrInvalid)
	}
	if nftRowID != 0 {
		return domain.CardPrefs{}, fmt.Errorf("%w: NFT card stickers are disabled", domain.ErrInvalid)
	}
	if nftX < 0 || nftX > 1 || nftY < 0 || nftY > 1 || nftScale < 0.05 || nftScale > 1 || nftRotation < -180 || nftRotation > 180 {
		return domain.CardPrefs{}, fmt.Errorf("%w: invalid card sticker transform", domain.ErrInvalid)
	}
	p := domain.CardPrefs{UserID: userID, ColorID: colorID, NFTX: nftX, NFTY: nftY, NFTScale: nftScale, NFTRotation: nftRotation}
	if err := c.db.SetCardPrefs(ctx, p); err != nil {
		return domain.CardPrefs{}, err
	}
	return c.db.CardPrefs(ctx, userID)
}

func (c *Cards) SetAddress(ctx context.Context, userID int64, address string) (domain.CardPublic, error) {
	address = strings.TrimSpace(address)
	if userID <= 0 || address == "" || len(address) > 64 {
		return domain.CardPublic{}, fmt.Errorf("%w: address is required", domain.ErrInvalid)
	}
	if strings.HasSuffix(strings.ToLower(address), ".ton") {
		name := domain.NormalizeTONDomain(address)
		if name == "" {
			return domain.CardPublic{}, fmt.Errorf("%w: invalid .ton name", domain.ErrInvalid)
		}
		dom, err := c.db.GetTONDomain(ctx, name)
		if err != nil {
			return domain.CardPublic{}, err
		}
		if dom.OwnerID != userID {
			return domain.CardPublic{}, fmt.Errorf("%w: this .ton name belongs to another account", domain.ErrForbidden)
		}
	} else {
		ok := len(address) == 48 && (strings.HasPrefix(address, "UQ") || strings.HasPrefix(address, "EQ") || strings.HasPrefix(address, "kQ") || strings.HasPrefix(address, "0Q"))
		if !ok {
			return domain.CardPublic{}, fmt.Errorf("%w: enter a UQ/EQ TON address or your .ton name", domain.ErrInvalid)
		}
	}
	if err := c.db.SetTONCardAddress(ctx, userID, address); err != nil {
		return domain.CardPublic{}, err
	}
	return c.cardOf(ctx, userID)
}

func (c *Cards) Nfts(ctx context.Context, userID int64) ([]domain.GiftAsset, error) {
	gifts, err := c.db.OwnedGifts(ctx, userID, 500)
	if err != nil {
		return nil, err
	}
	if gifts == nil {
		gifts = []domain.GiftAsset{}
	}
	return gifts, nil
}

// Send moves TON or stars between two DeltaGram accounts by username. TON is an
// internal ledger transfer; stars go through the main server's debit/grant API
// so both accounts see the movement in their normal star history.
func (c *Cards) Send(ctx context.Context, fromUserID int64, toUsername, kind string, amount int64) (domain.CardPublic, error) {
	if fromUserID <= 0 {
		return domain.CardPublic{}, domain.ErrUnauthorized
	}
	toUserID, err := c.db.ResolveUsername(ctx, toUsername)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.CardPublic{}, fmt.Errorf("%w: user @%s not found", domain.ErrNotFound, strings.TrimPrefix(toUsername, "@"))
		}
		return domain.CardPublic{}, err
	}
	if toUserID == fromUserID {
		return domain.CardPublic{}, fmt.Errorf("%w: cannot send to yourself", domain.ErrInvalid)
	}
	switch kind {
	case "ton":
		if amount < 10_000_000 {
			return domain.CardPublic{}, fmt.Errorf("%w: minimum is 0.01 TON", domain.ErrInvalid)
		}
		err = c.db.InTx(ctx, func(tx store.Tx) error {
			if err := tx.DebitTON(ctx, fromUserID, amount, "card_send", "Перевод TON пользователю @"+strings.TrimPrefix(toUsername, "@")); err != nil {
				return err
			}
			return tx.CreditTON(ctx, toUserID, amount, "card_receive", "Перевод TON от пользователя")
		})
	case "stars":
		if amount < 1 {
			return domain.CardPublic{}, fmt.Errorf("%w: amount must be positive", domain.ErrInvalid)
		}
		err = c.sendStars(ctx, fromUserID, toUserID, amount)
	default:
		return domain.CardPublic{}, fmt.Errorf("%w: kind must be ton or stars", domain.ErrInvalid)
	}
	if err != nil {
		return domain.CardPublic{}, err
	}
	c.log.Info("card transfer", "from", fromUserID, "to", toUserID, "kind", kind, "amount", amount)
	return c.cardOf(ctx, fromUserID)
}

func (c *Cards) sendStars(ctx context.Context, fromUserID, toUserID, amount int64) error {
	serverID := int64(0)
	if u, err := c.db.ResolveTONUser(ctx, fromUserID); err == nil {
		serverID = u.ServerID
	}
	if serverID == 0 {
		id, err := c.db.PrimaryServerID(ctx)
		if err != nil {
			return err
		}
		serverID = id
	}
	net, err := c.nets.Get(ctx, serverID)
	if err != nil {
		return err
	}
	op := networkOp("stars-send:"+fmt.Sprint(fromUserID)+":"+fmt.Sprint(toUserID), "Перевод звёзд")
	if err := net.DebitStars(ctx, op, fromUserID, amount); err != nil {
		return err
	}
	grant := networkOp("stars-receive:"+fmt.Sprint(fromUserID)+":"+fmt.Sprint(toUserID), "Получение звёзд")
	if err := net.GrantStars(ctx, grant, toUserID, amount); err != nil {
		// Refund the sender if the recipient credit failed; the sender's debit
		// already exists on the main server and must not vanish.
		refund := networkOp("stars-refund:"+fmt.Sprint(fromUserID)+":"+fmt.Sprint(toUserID), "Возврат перевода звёзд")
		if rerr := net.GrantStars(ctx, refund, fromUserID, amount); rerr != nil {
			c.log.Error("star refund failed", "from", fromUserID, "to", toUserID, "amount", amount, "err", rerr)
		}
		return err
	}
	return nil
}

func networkOp(prefix, reason string) network.Op {
	return network.Op{Key: prefix + ":" + secret.Hex(6), Reason: reason, At: time.Now()}
}

func (c *Cards) cardOf(ctx context.Context, userID int64) (domain.CardPublic, error) {
	list, err := c.db.CardRegistry(ctx)
	if err != nil {
		return domain.CardPublic{}, err
	}
	for _, row := range list {
		if row.UserID == userID {
			return row, nil
		}
	}
	return domain.CardPublic{UserID: userID}, nil
}
