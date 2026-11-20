package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"telesrv/cmd/deltament/internal/domain"
	"telesrv/cmd/deltament/internal/network"
	"telesrv/cmd/deltament/internal/store"
)

const (
	bidStepPercent = 5
	snipeWindow    = 5 * time.Minute
	bidsShown      = 20
	// maxLotCents caps every price and bid at 100000 GRAM/TON. The market
	// historically accepted arbitrary int64 prices, which produced lots like
	// 1.6e18 GRAM and made the UI unusable.
	maxLotCents = domain.Cents(100_000 * 100)
)

type marketStore interface {
	store.Lots
	store.Bids
	store.Watchlist
	store.Items
	store.Gifts
	store.TxRunner
}

// витрина, покупки и аукционы
type Market struct {
	db   marketStore
	nets Networks
	now  func() time.Time
	log  *slog.Logger
}

func NewMarket(db marketStore, nets Networks, log *slog.Logger) *Market {
	return &Market{db: db, nets: nets, now: time.Now, log: log}
}

type LotView struct {
	Lot  domain.Lot
	Top  domain.Cents
	Bids []domain.Bid
}

// текущая цена аукциона это высшая ставка, если она есть
func (v LotView) Highest() domain.Cents {
	if v.Top > v.Lot.Price {
		return v.Top
	}
	return v.Lot.Price
}

// шаг 5% от высшей, но не меньше 1 GRAM, всё в целых сотих как на фронте
func MinBid(highest domain.Cents) domain.Cents {
	whole := (int64(highest) + 50) / 100
	step := (whole*bidStepPercent + 50) / 100
	if step < 1 {
		step = 1
	}
	return domain.Cents((whole + step) * 100)
}

func (m *Market) view(ctx context.Context, l domain.Lot) (LotView, error) {
	v := LotView{Lot: l}
	if l.Kind == domain.LotGift && l.GiftRowID > 0 {
		// The lot snapshot may predate the artwork fields. Read the live gift
		// row and merge the real model/pattern/backdrop ids and colors so old
		// lots render exactly like the gift in the user's profile.
		if asset, err := m.db.GiftArtMeta(ctx, l.GiftRowID); err == nil {
			mergeGiftArt(l.Gift, asset)
			v.Lot = l
		} else if !errors.Is(err, domain.ErrNotFound) {
			m.log.Warn("gift art meta", "lot", l.ID, "row", l.GiftRowID, "err", err)
		}
	}
	// Gift auctions carry bids just like username lots; only fixed-price gift
	// listings can skip the bid query.
	if l.Kind == domain.LotGift && l.Status != domain.LotAuction {
		return v, nil
	}
	bids, err := m.db.ListBids(ctx, l.ID, bidsShown)
	if err != nil {
		return v, err
	}
	v.Bids = bids
	for _, b := range bids {
		if b.Active && b.Amount > v.Top {
			v.Top = b.Amount
		}
	}
	return v, nil
}

func (m *Market) List(ctx context.Context, serverID int64, kind domain.LotKind) ([]LotView, error) {
	lots, err := m.db.ListLots(ctx, serverID, kind)
	if err != nil {
		return nil, err
	}
	out := make([]LotView, 0, len(lots))
	for _, l := range lots {
		v, err := m.view(ctx, l)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (m *Market) Get(ctx context.Context, serverID int64, kind domain.LotKind, slug string) (LotView, error) {
	l, err := m.db.LotBySlug(ctx, serverID, kind, slug)
	if err != nil {
		return LotView{}, err
	}
	return m.view(ctx, l)
}

func (m *Market) GetByID(ctx context.Context, serverID, id int64) (LotView, error) {
	l, err := m.db.GetLot(ctx, id)
	if err != nil {
		return LotView{}, err
	}
	if l.ServerID != serverID {
		return LotView{}, domain.ErrNotFound
	}
	return m.view(ctx, l)
}

// покупка по фикс цене
func (m *Market) Buy(ctx context.Context, u domain.User, lotID int64) (domain.Item, error) {
	var item domain.Item
	err := m.db.InTx(ctx, func(tx store.Tx) error {
		l, err := tx.LockLot(ctx, lotID)
		if err != nil {
			return err
		}
		if l.ServerID != u.ServerID {
			return domain.ErrNotFound
		}
		if l.Status != domain.LotSale {
			return fmt.Errorf("%w: lot is not for sale", domain.ErrConflict)
		}
		if l.SellerUserID == u.ID {
			return fmt.Errorf("%w: you cannot buy your own lot", domain.ErrForbidden)
		}
		if err := tx.Debit(ctx, u.ID, l.Price, domain.HistoryBuy, "Покупка "+lotTitle(l)); err != nil {
			return err
		}
		l.Status, l.EndsAt = domain.LotSold, m.now()
		if err := tx.UpdateLot(ctx, l); err != nil {
			return err
		}
		item, err = m.deliverUserLot(ctx, tx, l, u.ID, "Маркет")
		return err
	})
	return item, err
}

// ставка держит GRAM, перебили — вернули прошлому лидеру
func (m *Market) Bid(ctx context.Context, u domain.User, lotID int64, amount domain.Cents) error {
	return m.db.InTx(ctx, func(tx store.Tx) error {
		l, err := tx.LockLot(ctx, lotID)
		if err != nil {
			return err
		}
		if l.ServerID != u.ServerID {
			return domain.ErrNotFound
		}
		now := m.now()
		if l.Status != domain.LotAuction || !now.Before(l.EndsAt) {
			return fmt.Errorf("%w: auction is closed", domain.ErrConflict)
		}
		if l.SellerUserID == u.ID {
			return fmt.Errorf("%w: you cannot bid on your own lot", domain.ErrForbidden)
		}
		top, err := tx.TopBid(ctx, l.ID)
		hasTop := err == nil
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		highest := l.Price
		if hasTop && top.Amount > highest {
			highest = top.Amount
		}
		if amount > maxLotCents {
			return fmt.Errorf("%w: bid must not exceed %.0f", domain.ErrInvalid, maxLotCents.Float())
		}
		if amount < MinBid(highest) {
			return fmt.Errorf("%w: bid must be at least %.0f", domain.ErrInvalid, MinBid(highest).Float())
		}

		title := "Ставка: " + lotTitle(l)
		charge := amount
		if hasTop && top.UserID == u.ID {
			charge = amount - top.Amount
		}
		if err := tx.Debit(ctx, u.ID, charge, domain.HistoryBid, title); err != nil {
			return err
		}
		if hasTop {
			if top.UserID != u.ID {
				if err := tx.Credit(ctx, top.UserID, top.Amount, domain.HistoryRefund, "Ставку перебили: "+lotTitle(l)); err != nil {
					return err
				}
			}
			if err := tx.DeactivateBid(ctx, top.ID); err != nil {
				return err
			}
		}
		if err := tx.AddBid(ctx, domain.Bid{LotID: l.ID, UserID: u.ID, Masked: maskUser(u), Amount: amount}); err != nil {
			return err
		}
		// ставка в последние минуты продлевает аукцион
		if l.EndsAt.Sub(now) < snipeWindow {
			l.EndsAt = now.Add(snipeWindow)
			return tx.UpdateLot(ctx, l)
		}
		return nil
	})
}

// закрываем аукцион и отдаём лот лидеру
func (m *Market) Settle(ctx context.Context, lotID int64) error {
	return m.db.InTx(ctx, func(tx store.Tx) error {
		l, err := tx.LockLot(ctx, lotID)
		if err != nil {
			return err
		}
		if l.Status != domain.LotAuction || m.now().Before(l.EndsAt) {
			return nil
		}
		top, err := tx.TopBid(ctx, l.ID)
		if errors.Is(err, domain.ErrNotFound) {
			// никто не поставил, переводим в обычную продажу
			l.Status, l.EndsAt = domain.LotSale, time.Time{}
			return tx.UpdateLot(ctx, l)
		}
		if err != nil {
			return err
		}
		if err := tx.DeactivateBid(ctx, top.ID); err != nil {
			return err
		}
		l.Status, l.Price = domain.LotSold, top.Amount
		if err := tx.UpdateLot(ctx, l); err != nil {
			return err
		}
		_, err = m.deliverUserLot(ctx, tx, l, top.UserID, "Аукцион")
		if errors.Is(err, domain.ErrConflict) {
			// The seller no longer owns the asset. Refund the held bid instead
			// of leaving the auction stuck past its end time forever.
			if cerr := tx.Credit(ctx, top.UserID, top.Amount, domain.HistoryRefund, "Лот отменён: "+lotTitle(l)); cerr != nil {
				return cerr
			}
			return nil
		}
		return err
	})
}

// фоновый цикл закрытия аукционов
func (m *Market) RunSettler(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		ids, err := m.db.DueAuctions(ctx, m.now())
		if err != nil {
			m.log.Error("due auctions", "err", err)
			continue
		}
		for _, id := range ids {
			if err := m.Settle(ctx, id); err != nil {
				m.log.Error("settle auction", "lot", id, "err", err)
			}
		}
	}
}

func (m *Market) ToggleWatch(ctx context.Context, u domain.User, lotID int64) (bool, error) {
	l, err := m.db.GetLot(ctx, lotID)
	if err != nil {
		return false, err
	}
	if l.ServerID != u.ServerID {
		return false, domain.ErrNotFound
	}
	return m.db.ToggleWatch(ctx, u.ID, lotID)
}

func (m *Market) Watching(ctx context.Context, userID int64) ([]int64, error) {
	return m.db.Watching(ctx, userID)
}

func (m *Market) MyBids(ctx context.Context, userID int64) ([]domain.Bid, error) {
	return m.db.UserBids(ctx, userID)
}

// deliverUserLot finishes a sale. Every lot produces an inventory item for the
// winner; binding that item is what actually grants the username to the
// winner's DeltaGram account. Seller assets are escrowed into the operator
// vault immediately, so the seller cannot keep using or re-list them, and the
// seller is paid at settlement rather than waiting for the winner to bind.
func (m *Market) deliverUserLot(ctx context.Context, tx store.Tx, l domain.Lot, winnerID int64, source string) (domain.Item, error) {
	// Move (and verify) the seller's asset before inserting the winner's item,
	// so a failed check cannot leave a committed item behind.
	if l.SellerUserID != 0 && l.GiftRowID == 0 && !l.Escrowed {
		if err := m.verifySellerAsset(ctx, tx, l); err != nil {
			return domain.Item{}, err
		}
		if err := m.escrowSellerUsername(ctx, l); err != nil {
			return domain.Item{}, err
		}
		l.Escrowed = true
		if err := tx.UpdateLot(ctx, l); err != nil {
			return domain.Item{}, err
		}
	}
	item, err := tx.AddItem(ctx, itemFromLot(l, winnerID, l.Price, source))
	if err != nil {
		return domain.Item{}, err
	}
	if l.SellerUserID == 0 {
		return item, nil
	}
	seller, err := tx.GetUser(ctx, l.SellerUserID)
	if err != nil {
		return domain.Item{}, err
	}
	// Rewards no longer turn into marketplace GRAM: the seller is paid in TON
	// straight onto the account's TON card. The zero-amount marketplace history
	// line keeps the sale visible in the profile ledger.
	nanoton := int64(l.Price) * (domain.NanotonPerTON / 100)
	paySeller := func() error {
		if err := tx.CreditTON(ctx, seller.ServerUserID, nanoton, "sale", "Продажа: "+lotTitle(l)); err != nil {
			return err
		}
		return tx.Credit(ctx, l.SellerUserID, 0, domain.HistorySell, "Продажа: "+lotTitle(l))
	}
	if l.GiftRowID > 0 {
		// The gift is already escrowed on the relayer account, so there is
		// nothing to move at sale time: it stays there until the buyer claims it.
		if err := paySeller(); err != nil {
			return domain.Item{}, err
		}
		if err := tx.Credit(ctx, winnerID, 0, domain.HistoryWin, "Победа: "+lotTitle(l)); err != nil {
			return domain.Item{}, err
		}
		return item, nil
	}
	// A consigned resale lot (Escrowed=true) has no asset on the seller's
	// account: it is either already in the vault or was never owned by the
	// seller (operator lot). Settling it must only pay the seller.
	if err := paySeller(); err != nil {
		return domain.Item{}, err
	}
	if err := tx.Credit(ctx, winnerID, 0, domain.HistoryWin, "Победа: "+lotTitle(l)); err != nil {
		return domain.Item{}, err
	}
	return item, nil
}

// verifySellerAsset makes sure the seller still owns exactly the asset named by
// the lot before settlement revokes it. Without this check a seller could list
// @name, transfer the name to a third party, and the settler would then revoke
// it from that innocent owner ("I bought a username and somebody else got it").
func (m *Market) verifySellerAsset(ctx context.Context, tx store.Tx, l domain.Lot) error {
	seller, err := tx.GetUser(ctx, l.SellerUserID)
	if err != nil {
		return err
	}
	net, err := m.nets.Get(ctx, l.ServerID)
	if err != nil {
		return err
	}
	inv, ok := net.(network.Inventory)
	if !ok {
		return fmt.Errorf("%w: this server cannot verify lot ownership", domain.ErrServerUnavailable)
	}
	switch l.AssetKind {
	case domain.AssetChannel:
		channels, err := inv.OwnedChannels(ctx, seller.ServerUserID)
		if err != nil {
			return err
		}
		for _, ch := range channels {
			if ch.ID == l.ChannelID && strings.EqualFold(ch.Username, l.Name) {
				return nil
			}
		}
		return fmt.Errorf("%w: seller no longer owns this channel username", domain.ErrConflict)
	case domain.AssetCollectible:
		assets, err := inv.OwnedUsernames(ctx, seller.ServerUserID)
		if err != nil {
			return err
		}
		for _, a := range assets {
			if strings.EqualFold(a.Username, l.Name) {
				return nil
			}
		}
		return fmt.Errorf("%w: seller no longer owns this username", domain.ErrConflict)
	default:
		return fmt.Errorf("%w: unknown lot asset kind %q", domain.ErrInvalid, l.AssetKind)
	}
}

// escrowSellerUsername moves the underlying channel/collectible name into the
// operator vault. The timestamp is derived from the lot, not from wall clock,
// so a retry after a rolled-back transaction sends the same purchase_date and
// reuses the same command id.
func (m *Market) escrowSellerUsername(ctx context.Context, l domain.Lot) error {
	net, err := m.nets.Get(ctx, l.ServerID)
	if err != nil {
		return err
	}
	escrow, ok := net.(network.UsernameEscrow)
	if !ok {
		return fmt.Errorf("%w: this server cannot escrow user-owned lots", domain.ErrServerUnavailable)
	}
	when := l.CreatedAt
	if !l.EndsAt.IsZero() {
		when = l.EndsAt
	}
	op := network.Op{
		Key:    "escrow:" + strconv.FormatInt(l.ID, 10),
		Reason: "Deltament escrow",
		At:     when,
		Value:  l.Price,
	}
	switch l.AssetKind {
	case domain.AssetChannel:
		if l.ChannelID <= 0 {
			return fmt.Errorf("%w: channel lot has no channel id", domain.ErrConflict)
		}
		inv, ok := net.(network.Inventory)
		if !ok {
			return fmt.Errorf("%w: this server cannot escrow user-owned lots", domain.ErrServerUnavailable)
		}
		if err := inv.DeleteChannel(ctx, op, l.ChannelID); err != nil {
			return fmt.Errorf("delete seller channel: %w", err)
		}
		if err := escrow.MintUsernameToVault(ctx, op, l.Name); err != nil {
			return fmt.Errorf("escrow channel username: %w", err)
		}
	case domain.AssetCollectible:
		if err := escrow.RevokeUsername(ctx, op, l.Name); err != nil {
			return fmt.Errorf("escrow collectible username: %w", err)
		}
	default:
		return fmt.Errorf("%w: unknown lot asset kind %q", domain.ErrInvalid, l.AssetKind)
	}
	return nil
}

// Inventory is what a user can put up for sale, plus their current listings.
type Inventory struct {
	Channels  []network.ChannelAsset
	Usernames []network.UsernameAsset
	Listings  []domain.Lot
	Gifts     []domain.GiftAsset
}

func (m *Market) Inventory(ctx context.Context, u domain.User) (Inventory, error) {
	var out Inventory
	net, err := m.nets.Get(ctx, u.ServerID)
	if err != nil {
		return out, err
	}
	inv, ok := net.(network.Inventory)
	if !ok {
		return out, fmt.Errorf("%w: this server does not expose an asset inventory", domain.ErrForbidden)
	}
	if out.Channels, err = inv.OwnedChannels(ctx, u.ServerUserID); err != nil {
		return out, err
	}
	if out.Usernames, err = inv.OwnedUsernames(ctx, u.ServerUserID); err != nil {
		return out, err
	}
	if out.Listings, err = m.db.ListLotsBySeller(ctx, u.ID); err != nil {
		return out, err
	}
	if out.Gifts, err = m.db.OwnedGifts(ctx, u.ServerUserID, 500); err != nil {
		return out, err
	}
	return out, nil
}

// OwnedGifts lists gifts that can still be listed: anything the marketplace
// already escrowed is no longer owned by the seller, so it is filtered out by
// the ownership query itself.
func (m *Market) OwnedGifts(ctx context.Context, u domain.User) ([]domain.GiftAsset, error) {
	gifts, err := m.db.OwnedGifts(ctx, u.ServerUserID, 500)
	if err != nil {
		return nil, err
	}
	if gifts == nil {
		gifts = []domain.GiftAsset{}
	}
	return gifts, nil
}

// NewGiftLot is a gift listing request coming from the marketplace UI.
type NewGiftLot struct {
	GiftRowID int64
	Price     domain.Cents
	Auction   bool
	Duration  time.Duration
}

// ListGift escrows a real gift on the DeltaMent relayer account and creates a
// market lot for it. The lot's GiftMeta is a snapshot: the underlying catalog
// can be revised later without changing a live auction.
func (m *Market) ListGift(ctx context.Context, u domain.User, in NewGiftLot) (domain.Lot, error) {
	var l domain.Lot
	if in.GiftRowID <= 0 {
		return l, fmt.Errorf("%w: gift id is required", domain.ErrInvalid)
	}
	if in.Price < 100 {
		return l, fmt.Errorf("%w: price must be at least 1 GRAM", domain.ErrInvalid)
	}
	if in.Price > maxLotCents {
		return l, fmt.Errorf("%w: price must not exceed %.0f GRAM", domain.ErrInvalid, maxLotCents.Float())
	}
	// Gifts are fixed-price only: gift auctions were removed from the
	// marketplace, so reject the request outright instead of silently
	// downgrading it.
	if in.Auction {
		return l, fmt.Errorf("%w: gifts are listed at a fixed price only", domain.ErrInvalid)
	}
	err := m.db.InTx(ctx, func(tx store.Tx) error {
		asset, err := tx.OwnedGift(ctx, u.ServerUserID, in.GiftRowID)
		if err != nil {
			return err
		}
		// Only upgraded (NFT) gifts may be traded. Regular catalog gifts are
		// untransferable collectibles in DeltaGram, so listing them would
		// create a lot the relayer could never legally deliver.
		if !asset.Unique {
			return fmt.Errorf("%w: only upgraded (NFT) gifts can be listed", domain.ErrForbidden)
		}
		relayerID, err := tx.RelayerUserID(ctx)
		if err != nil {
			return err
		}
		meta := &domain.GiftMeta{
			Slug:       giftSlug(asset.Title),
			Collection: asset.Title,
			Num:        asset.Num,
			GiftID:     asset.GiftID,
			Model:      asset.Model,
			Backdrop:   asset.Backdrop,
			Symbol:     asset.Symbol,
			Art:        asset.Art,
		}
		status, ends := domain.LotSale, time.Time{}
		l = domain.Lot{
			ServerID:     u.ServerID,
			Kind:         domain.LotGift,
			Name:         fmt.Sprintf("%s #%d", asset.Title, asset.Num),
			Slug:         fmt.Sprintf("gift-%d", asset.RowID),
			Price:        in.Price,
			Status:       status,
			EndsAt:       ends,
			Gift:         meta,
			SellerUserID: u.ID,
			GiftRowID:    asset.RowID,
			AssetKind:    domain.AssetGift,
		}
		if l, err = tx.CreateLot(ctx, l); err != nil {
			return err
		}
		return tx.EscrowGift(ctx, asset.RowID, u.ServerUserID, relayerID)
	})
	return l, err
}

// ClaimGift delivers a purchased gift from the relayer account to the buyer.
// Only gifts that were escrowed from the marketplace take this path; operator
// lots (GiftRowID == 0) are minted through the network instead.
func (m *Market) ClaimGift(ctx context.Context, u domain.User, itemID int64) error {
	var it domain.Item
	err := m.db.InTx(ctx, func(tx store.Tx) error {
		var err error
		if it, err = tx.LockItem(ctx, u.ID, itemID); err != nil {
			return err
		}
		if it.Kind != domain.ItemGift || it.GiftRowID <= 0 {
			return fmt.Errorf("%w: this item is not a transferable gift", domain.ErrInvalid)
		}
		if it.Status != domain.ItemOwned {
			return fmt.Errorf("%w: item is %s", domain.ErrConflict, it.Status)
		}
		relayerID, err := tx.RelayerUserID(ctx)
		if err != nil {
			return err
		}
		if err := tx.ReleaseGift(ctx, it.GiftRowID, relayerID, u.ServerUserID); err != nil {
			return err
		}
		if err := tx.SetItemStatus(ctx, it.ID, domain.ItemWithdrawn); err != nil {
			return err
		}
		return tx.Credit(ctx, u.ID, 0, domain.HistoryWithdraw, "Добавлен в профиль: "+itemTitle(it))
	})
	if err != nil {
		return err
	}
	return nil
}

type NewUserLot struct {
	AssetKind       domain.AssetKind
	ChannelID       int64
	UsernameAssetID int64
	Username        string
	Price           domain.Cents
	Auction         bool
	Duration        time.Duration
}

// CreateUserLot lists an owned channel username or collectible username for
// auction or at a fixed price. Ownership is checked against the DeltaGram Admin
// API, not the local database, so a stale marketplace record cannot sell
// someone else's asset.
func (m *Market) CreateUserLot(ctx context.Context, u domain.User, in NewUserLot) (domain.Lot, error) {
	var l domain.Lot
	if in.Price < 100 {
		return l, fmt.Errorf("%w: price must be at least 1 GRAM", domain.ErrInvalid)
	}
	if in.Price > maxLotCents {
		return l, fmt.Errorf("%w: price must not exceed %.0f GRAM", domain.ErrInvalid, maxLotCents.Float())
	}
	if in.Auction && (in.Duration < time.Hour || in.Duration > 30*24*time.Hour) {
		return l, fmt.Errorf("%w: auction must last 1h-30d", domain.ErrInvalid)
	}
	username := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(in.Username), "@"))
	if !usernameRe.MatchString(username) {
		return l, fmt.Errorf("%w: username must be 4-32 chars a-z0-9_", domain.ErrInvalid)
	}
	net, err := m.nets.Get(ctx, u.ServerID)
	if err != nil {
		return l, err
	}
	inv, ok := net.(network.Inventory)
	if !ok {
		return l, fmt.Errorf("%w: this server does not expose an asset inventory", domain.ErrForbidden)
	}
	switch in.AssetKind {
	case domain.AssetChannel:
		if in.ChannelID <= 0 {
			return l, fmt.Errorf("%w: channel_id is required", domain.ErrInvalid)
		}
		channels, err := inv.OwnedChannels(ctx, u.ServerUserID)
		if err != nil {
			return l, err
		}
		found := false
		for _, ch := range channels {
			if ch.ID == in.ChannelID && strings.EqualFold(ch.Username, username) {
				found = true
				break
			}
		}
		if !found {
			return l, fmt.Errorf("%w: channel not found or username changed", domain.ErrForbidden)
		}
	case domain.AssetCollectible:
		if in.UsernameAssetID <= 0 {
			return l, fmt.Errorf("%w: username_asset_id is required", domain.ErrInvalid)
		}
		assets, err := inv.OwnedUsernames(ctx, u.ServerUserID)
		if err != nil {
			return l, err
		}
		found := false
		for _, asset := range assets {
			if asset.ID == in.UsernameAssetID && strings.EqualFold(asset.Username, username) {
				found = true
				break
			}
		}
		if !found {
			return l, fmt.Errorf("%w: collectible username not found", domain.ErrForbidden)
		}
	default:
		return l, fmt.Errorf("%w: asset_kind must be channel or collectible", domain.ErrInvalid)
	}
	// A sold asset can be listed again by its new owner, but only one active
	// lot per username is allowed.
	if prev, err := m.db.LotBySlug(ctx, u.ServerID, domain.LotUsername, username); err == nil {
		if prev.Status == domain.LotAuction || prev.Status == domain.LotSale {
			return l, fmt.Errorf("%w: this username is already listed", domain.ErrConflict)
		}
		if prev.Status == domain.LotSold && prev.SellerUserID == u.ID {
			pending, err := m.db.HasItemForLot(ctx, prev.ID)
			if err != nil {
				return l, err
			}
			if pending {
				return l, fmt.Errorf("%w: the previous sale is still waiting to be bound", domain.ErrConflict)
			}
		}
	} else if !errors.Is(err, domain.ErrNotFound) {
		return l, err
	}
	l = domain.Lot{
		ServerID:        u.ServerID,
		Kind:            domain.LotUsername,
		Name:            username,
		Slug:            username,
		Price:           in.Price,
		Status:          domain.LotSale,
		SellerUserID:    u.ID,
		ChannelID:       in.ChannelID,
		UsernameAssetID: in.UsernameAssetID,
		AssetKind:       in.AssetKind,
	}
	if in.Auction {
		l.Status = domain.LotAuction
		l.EndsAt = m.now().Add(in.Duration)
	}
	return m.db.CreateLot(ctx, l)
}

func (m *Market) MyListings(ctx context.Context, userID int64) ([]domain.Lot, error) {
	return m.db.ListLotsBySeller(ctx, userID)
}

func (m *Market) CancelUserLot(ctx context.Context, u domain.User, lotID int64) error {
	l, err := m.db.GetLot(ctx, lotID)
	if err != nil {
		return err
	}
	if l.SellerUserID != u.ID {
		return domain.ErrForbidden
	}
	if l.Status == domain.LotSold {
		return fmt.Errorf("%w: lot is already sold", domain.ErrConflict)
	}
	if _, err := m.db.TopBid(ctx, l.ID); err == nil {
		return fmt.Errorf("%w: lot already has bids", domain.ErrConflict)
	} else if !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	if l.GiftRowID > 0 {
		return m.db.InTx(ctx, func(tx store.Tx) error {
			relayerID, err := tx.RelayerUserID(ctx)
			if err != nil {
				return err
			}
			if err := tx.ReleaseGift(ctx, l.GiftRowID, relayerID, u.ServerUserID); err != nil {
				return err
			}
			return tx.DeleteLot(ctx, l.ServerID, l.ID)
		})
	}
	return m.DeleteLot(ctx, l.ServerID, l.ID)
}

// лоты выставляет оператор своей приватки

type NewLot struct {
	Kind     domain.LotKind
	Name     string
	Price    domain.Cents
	Auction  bool
	Duration time.Duration
	Gift     *domain.GiftMeta
}

var (
	usernameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{3,31}$`)
	slugRe     = regexp.MustCompile(`^[a-z0-9]{2,40}$`)
)

func (m *Market) CreateLot(ctx context.Context, serverID int64, in NewLot) (domain.Lot, error) {
	l := domain.Lot{ServerID: serverID, Kind: in.Kind, Price: in.Price, Status: domain.LotSale}
	if in.Price < 100 {
		return l, fmt.Errorf("%w: price must be at least 1 GRAM", domain.ErrInvalid)
	}
	if in.Price > maxLotCents {
		return l, fmt.Errorf("%w: price must not exceed %.0f GRAM", domain.ErrInvalid, maxLotCents.Float())
	}
	switch in.Kind {
	case domain.LotUsername:
		name := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(in.Name), "@"))
		if !usernameRe.MatchString(name) {
			return l, fmt.Errorf("%w: username must be 4-32 chars a-z0-9_", domain.ErrInvalid)
		}
		l.Name, l.Slug = name, name
	case domain.LotNumber:
		d := digits(in.Name)
		if len(d) < 7 || len(d) > 15 {
			return l, fmt.Errorf("%w: bad number", domain.ErrInvalid)
		}
		l.Name, l.Slug = formatNumber(d), d
	case domain.LotGift:
		g := in.Gift
		if g == nil || !slugRe.MatchString(g.Slug) || g.Num <= 0 || g.GiftID <= 0 || g.Collection == "" {
			return l, fmt.Errorf("%w: gift needs slug, collection, num and gift_id", domain.ErrInvalid)
		}
		l.Name, l.Slug, l.Gift = fmt.Sprintf("%s #%d", g.Collection, g.Num), fmt.Sprintf("%s-%d", g.Slug, g.Num), g
	default:
		return l, fmt.Errorf("%w: kind must be username, number or gift", domain.ErrInvalid)
	}
	if in.Auction {
		if in.Kind == domain.LotGift {
			return l, fmt.Errorf("%w: gifts are fixed price only", domain.ErrInvalid)
		}
		if in.Duration < time.Hour || in.Duration > 30*24*time.Hour {
			return l, fmt.Errorf("%w: auction must last 1h-30d", domain.ErrInvalid)
		}
		l.Status, l.EndsAt = domain.LotAuction, m.now().Add(in.Duration)
	}
	return m.db.CreateLot(ctx, l)
}

func (m *Market) DeleteLot(ctx context.Context, serverID, lotID int64) error {
	err := m.db.DeleteLot(ctx, serverID, lotID)
	if errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("%w: lot not found, sold or has bids", domain.ErrConflict)
	}
	return err
}

func (m *Market) ServerLots(ctx context.Context, serverID int64) ([]domain.Lot, error) {
	var out []domain.Lot
	for _, k := range []domain.LotKind{domain.LotUsername, domain.LotNumber, domain.LotGift} {
		lots, err := m.db.ListLots(ctx, serverID, k)
		if err != nil {
			return nil, err
		}
		out = append(out, lots...)
	}
	return out, nil
}

// +888 0000 0000 как на фрагменте
func formatNumber(d string) string {
	if strings.HasPrefix(d, "888") && len(d) == 11 {
		return "+888 " + d[3:7] + " " + d[7:]
	}
	return "+" + d
}

// mergeGiftArt fills a lot's gift snapshot with the real artwork metadata of
// the underlying gift row. Names already stored in the snapshot win, because
// they describe the lot at listing time; ids/colors always come from the live
// row so the renderer can load the right animations.
func mergeGiftArt(meta *domain.GiftMeta, asset domain.GiftAsset) {
	if meta == nil {
		return
	}
	meta.Art = asset.Art
	if meta.GiftID == 0 {
		meta.GiftID = asset.GiftID
	}
	if meta.Num == 0 {
		meta.Num = asset.Num
	}
	if meta.Collection == "" {
		meta.Collection = asset.Title
	}
	if meta.Slug == "" {
		meta.Slug = giftSlug(asset.Title)
	}
	if meta.Model.Name == "" {
		meta.Model = asset.Model
	}
	if meta.Backdrop.Name == "" {
		meta.Backdrop = asset.Backdrop
	}
	if meta.Symbol.Name == "" {
		meta.Symbol = asset.Symbol
	}
}

func giftSlug(title string) string {
	slug := giftSlugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(title)), "")
	if slug == "" {
		slug = "gift"
	}
	if len(slug) > 40 {
		slug = slug[:40]
	}
	return slug
}

var giftSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

func maskUser(u domain.User) string {
	p := digits(u.Phone)
	if len(p) < 4 {
		return "•••"
	}
	return "+" + p[:1] + "•••" + p[len(p)-2:]
}
