package store

import (
	"context"
	"time"

	"telesrv/cmd/deltament/internal/domain"
)

type Servers interface {
	CreateServer(ctx context.Context, s domain.Server) (domain.Server, error)
	GetServer(ctx context.Context, id int64) (domain.Server, error)
	ServerBySlug(ctx context.Context, slug string) (domain.Server, error)
	ServerByOperator(ctx context.Context, hash []byte) (domain.Server, error)
	ListServers(ctx context.Context, onlyActive bool) ([]domain.Server, error)
	SetServerStatus(ctx context.Context, id int64, status domain.ServerStatus) error
	SetServerSecret(ctx context.Context, id int64, apiURL, secret string) error
}

type Codes interface {
	SaveCode(ctx context.Context, c domain.LoginCode) error
	LatestCode(ctx context.Context, serverID int64, phone string) (domain.LoginCode, error)
	BumpCodeAttempts(ctx context.Context, id int64) error
	DeleteCodes(ctx context.Context, serverID int64, phone string) error
	CountCodesSince(ctx context.Context, serverID int64, phone string, since time.Time) (int, error)
}

type Users interface {
	UpsertUser(ctx context.Context, u domain.User) (domain.User, bool, error)
	GetUser(ctx context.Context, id int64) (domain.User, error)
}

type Sessions interface {
	CreateSession(ctx context.Context, s domain.Session, tokenHash []byte) error
	SessionByToken(ctx context.Context, tokenHash []byte) (domain.Session, error)
	TouchSession(ctx context.Context, id string) error
	ListSessions(ctx context.Context, userID int64) ([]domain.Session, error)
	DeleteSession(ctx context.Context, userID int64, id string) error
	DeleteOtherSessions(ctx context.Context, userID int64, keepID string) error
}

type Ledger interface {
	Credit(ctx context.Context, userID int64, amount domain.Cents, kind domain.HistoryKind, title string) error
	Debit(ctx context.Context, userID int64, amount domain.Cents, kind domain.HistoryKind, title string) error
	History(ctx context.Context, userID int64, limit int) ([]domain.HistoryEntry, error)
}

type Items interface {
	AddItem(ctx context.Context, it domain.Item) (domain.Item, error)
	LockItem(ctx context.Context, userID, id int64) (domain.Item, error)
	SetItemStatus(ctx context.Context, id int64, status domain.ItemStatus) error
	ListItems(ctx context.Context, userID int64) ([]domain.Item, error)
	// HasItemForLot reports whether a lot already produced an inventory item.
	// A sold asset that is still waiting to be bound must not be re-listed.
	HasItemForLot(ctx context.Context, lotID int64) (bool, error)
}

type Lots interface {
	CreateLot(ctx context.Context, l domain.Lot) (domain.Lot, error)
	GetLot(ctx context.Context, id int64) (domain.Lot, error)
	LotBySlug(ctx context.Context, serverID int64, kind domain.LotKind, slug string) (domain.Lot, error)
	LockLot(ctx context.Context, id int64) (domain.Lot, error)
	ListLots(ctx context.Context, serverID int64, kind domain.LotKind) ([]domain.Lot, error)
	ListLotsBySeller(ctx context.Context, sellerID int64) ([]domain.Lot, error)
	UpdateLot(ctx context.Context, l domain.Lot) error
	DeleteLot(ctx context.Context, serverID, id int64) error
	DueAuctions(ctx context.Context, now time.Time) ([]int64, error)
}

type Bids interface {
	TopBid(ctx context.Context, lotID int64) (domain.Bid, error)
	AddBid(ctx context.Context, b domain.Bid) error
	DeactivateBid(ctx context.Context, id int64) error
	ListBids(ctx context.Context, lotID int64, limit int) ([]domain.Bid, error)
	UserBids(ctx context.Context, userID int64) ([]domain.Bid, error)
}

type Watchlist interface {
	ToggleWatch(ctx context.Context, userID, lotID int64) (bool, error)
	Watching(ctx context.Context, userID int64) ([]int64, error)
}

// Gifts reads the user's real DeltaGram gifts and escrows them on the relayer
// account while they are listed on the marketplace.
type Gifts interface {
	RelayerUserID(ctx context.Context) (int64, error)
	OwnedGifts(ctx context.Context, userID int64, limit int) ([]domain.GiftAsset, error)
	OwnedGift(ctx context.Context, userID, rowID int64) (domain.GiftAsset, error)
	EscrowGift(ctx context.Context, rowID, ownerID, relayerID int64) error
	ReleaseGift(ctx context.Context, rowID, relayerID, buyerID int64) error
	// GiftArtMeta reads artwork metadata for a gift row regardless of owner.
	GiftArtMeta(ctx context.Context, rowID int64) (domain.GiftAsset, error)
	GiftCatalogAnimation(ctx context.Context, giftID int64) ([]byte, error)
	GiftModelAnimation(ctx context.Context, id int64) ([]byte, error)
	GiftPatternAnimation(ctx context.Context, id int64) ([]byte, error)
}

// TON stores the per-account TON card, its ledger and the payout/order queues.
// Every key is the messenger account id (users.id): the card belongs to the
// DeltaGram account, not to the marketplace row.
type TON interface {
	EnsureTONCard(ctx context.Context, userID int64) (string, error)
	SetTONAddress(ctx context.Context, userID int64, address string) error
	TONBalances(ctx context.Context, userID int64) (balance, pending, stars int64, err error)
	CreditTON(ctx context.Context, userID int64, amount int64, kind, title string) error
	DebitTON(ctx context.Context, userID int64, amount int64, kind, title string) error
	// CreditStars tops up the gold card when marketplace GRAM is converted back
	// into DeltaGram stars. The card tracks earned stars, not the messenger
	// account's live star wallet, so a star purchase never changes it.
	CreditStars(ctx context.Context, userID int64, amount int64, kind, title string) error
	TONLedger(ctx context.Context, userID int64, limit int) ([]domain.TONEntry, error)
	CreateTONPayout(ctx context.Context, userID int64, address string, amount int64, note string) (domain.TONPayout, error)
	ListTONPayouts(ctx context.Context, status string) ([]domain.TONPayout, error)
	SetTONPayoutStatus(ctx context.Context, id int64, status string) error
	// ResolveTONUser maps a messenger account id to its marketplace user so the
	// admin panel and the bot can grant TON by the id the owner sees in chat.
	ResolveTONUser(ctx context.Context, mainUserID int64) (domain.User, error)
	CreateTONOrder(ctx context.Context, userID int64, username string, ton, rub int64) (domain.TONOrder, error)
	TONOrderStatus(ctx context.Context, id int64) (domain.TONOrder, error)
	SetTONOrderStatus(ctx context.Context, id int64, status string) (domain.TONOrder, error)
	PendingTONOrders(ctx context.Context) ([]domain.TONOrder, error)
}

// CardPrefsStore backs the public DeltaGram card registry.
type CardPrefsStore interface {
	CardRegistry(ctx context.Context) ([]domain.CardPublic, error)
	CardHistory(ctx context.Context, userID int64) ([]domain.CardHistoryEntry, error)
	CardPrefs(ctx context.Context, userID int64) (domain.CardPrefs, error)
	SetCardPrefs(ctx context.Context, p domain.CardPrefs) error
	ResolveUsername(ctx context.Context, username string) (int64, error)
	PrimaryServerID(ctx context.Context) (int64, error)
	SetTONCardAddress(ctx context.Context, userID int64, address string) error
}

// TONDomains is the DeltaGram .ton registry.
type TONDomains interface {
	GetTONDomain(ctx context.Context, name string) (domain.TONDomain, error)
	ListTONDomainsByOwner(ctx context.Context, ownerID int64) ([]domain.TONDomain, error)
	ListTONDomainMarket(ctx context.Context, query, sort string, offset, limit int) ([]domain.TONDomain, int, error)
	CreateTONDomain(ctx context.Context, name string, ownerID, price int64) (domain.TONDomain, error)
	SetTONDomainOwner(ctx context.Context, name string, ownerID, price int64) (domain.TONDomain, error)
	SetTONDomainListing(ctx context.Context, name string, ownerID, ask int64) (domain.TONDomain, error)
	ClearTONDomainListing(ctx context.Context, name string, ownerID int64) error
}

// Banners stores profile banners and the public version registry.
type Banners interface {
	BannerTokens(ctx context.Context) (map[int64]string, error)
	UserBanner(ctx context.Context, userID int64) (domain.Banner, error)
	BannerToken(ctx context.Context, userID int64) (string, error)
	SetUserBanner(ctx context.Context, userID int64, mime string, data []byte) (string, error)
	ClearUserBanner(ctx context.Context, userID int64) error
}

// то что можно делать внутри одной транзакции
type Tx interface {
	Users
	Ledger
	Items
	Lots
	Bids
	Gifts
	TON
	TONDomains
}

type TxRunner interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

type Store interface {
	Servers
	Codes
	Users
	Sessions
	Ledger
	Items
	Lots
	Bids
	Watchlist
	Gifts
	TON
	TONDomains
	CardPrefsStore
	Banners
	TxRunner
}
