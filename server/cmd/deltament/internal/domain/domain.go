package domain

import (
	"errors"
	"time"
)

// деньги храним в сотих GRAM, чтобы не ловить ошибки float
type Cents int64

func (c Cents) Float() float64 { return float64(c) / 100 }

func CentsFromFloat(v float64) Cents {
	if v < 0 {
		return Cents(v*100 - 0.5)
	}
	return Cents(v*100 + 0.5)
}

var (
	ErrNotFound          = errors.New("not found")
	ErrNoFunds           = errors.New("not enough balance")
	ErrConflict          = errors.New("conflict")
	ErrForbidden         = errors.New("forbidden")
	ErrInvalid           = errors.New("invalid input")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrServerUnavailable = errors.New("server unavailable")
)

type ServerStatus string

const (
	ServerPending  ServerStatus = "pending"
	ServerActive   ServerStatus = "active"
	ServerDisabled ServerStatus = "disabled"
)

// привязанная приватка
type Server struct {
	ID           int64
	Slug         string
	Name         string
	Kind         string
	APIURL       string
	Secret       string
	Contact      string
	StarsPerGram float64
	Status       ServerStatus
	OperatorHash []byte
	CreatedAt    time.Time
}

type User struct {
	ID           int64
	ServerID     int64
	ServerUserID int64
	Phone        string
	Name         string
	Username     string
	Balance      Cents
	CreatedAt    time.Time
}

type Session struct {
	ID        string
	UserID    int64
	Device    string
	Place     string
	CreatedAt time.Time
	LastSeen  time.Time
}

type ItemKind string

const (
	ItemGift     ItemKind = "gift"
	ItemNumber   ItemKind = "number"
	ItemUsername ItemKind = "username"
	ItemStars    ItemKind = "stars"
)

type ItemStatus string

const (
	ItemOwned     ItemStatus = "owned"
	ItemWithdrawn ItemStatus = "withdrawn"
	ItemSold      ItemStatus = "sold"
)

type Item struct {
	ID     int64
	UserID int64
	LotID  int64
	Kind   ItemKind
	Title  string
	Name   string
	Slug   string
	Num    int
	GiftID int64
	// GiftRowID is set for gifts that already exist on the messenger and are
	// held by the relayer account. Zero means the item is minted on delivery.
	GiftRowID int64
	Art       GiftArt
	Amount    int64
	Value     Cents
	Source    string
	Status    ItemStatus
	CreatedAt time.Time
}

type HistoryKind string

const (
	HistoryBuy      HistoryKind = "buy"
	HistorySell     HistoryKind = "sell"
	HistoryTopup    HistoryKind = "topup"
	HistoryBid      HistoryKind = "bid"
	HistoryRefund   HistoryKind = "refund"
	HistoryWin      HistoryKind = "win"
	HistoryWithdraw HistoryKind = "withdraw"
)

type HistoryEntry struct {
	ID        int64
	UserID    int64
	Kind      HistoryKind
	Title     string
	Amount    Cents
	CreatedAt time.Time
}

// NanotonPerTON is the on-chain TON precision. A whole TON is 1e9 nanotons and
// the wallet API in this project always speaks nanotons so cents-like rounding
// never touches a balance.
const NanotonPerTON int64 = 1_000_000_000

// NanotonPerGram is the reward exchange rate: one whole GRAM of earned balance
// is paid out as one whole TON on the card. The two names are used
// interchangeably by the marketplace, so the card is a 1:1 payout rail.
const NanotonPerGram int64 = NanotonPerTON

// TONCard is the per-account payout card rendered as the blue card in the
// Money screen. Balance and Pending are nanotons; Stars is the gold card's
// balance and Address is the destination the owner sends TON to.
type TONCard struct {
	Address string
	Balance int64
	Pending int64
	Stars   int64
}

// TONPayout is one "send TON" request. The amount is reserved (debited from the
// card) when the request is created and the owner marks it sent after the
// on-chain transfer.
type TONPayout struct {
	ID        int64
	UserID    int64
	Address   string
	Amount    int64
	Status    string
	Note      string
	CreatedAt time.Time
	SentAt    time.Time
}

// TONEntry is one line of the card's TON ledger.
type TONEntry struct {
	ID        int64
	Amount    int64
	Kind      string
	Title     string
	CreatedAt time.Time
}

// TONOrder is a TON purchase order created by the bot. The price is fixed at
// 100 TON = 30 RUB, so ton_amount and rub_amount always respect that rate.
type TONOrder struct {
	ID        int64
	UserID    int64
	Username  string
	TON       int64
	RUB       int64
	Status    string
	CreatedAt time.Time
}

type LotKind string

const (
	LotUsername LotKind = "username"
	LotNumber   LotKind = "number"
	LotGift     LotKind = "gift"
)

type LotStatus string

const (
	LotAuction LotStatus = "auction"
	LotSale    LotStatus = "sale"
	LotSold    LotStatus = "sold"
)

// атрибут гифта: модель, фон, символ
type Trait struct {
	Name   string  `json:"name"`
	Rarity float64 `json:"rarity"`
}

// GiftColors is the backdrop palette stored on
// star_gift_collectible_backdrops. The client uses it to paint the real
// gradient behind the model/pattern Lottie, so a DeltaGram gift looks the
// same on the site as it does in the app.
type GiftColors struct {
	Center  int32 `json:"center"`
	Edge    int32 `json:"edge"`
	Pattern int32 `json:"pattern"`
	Text    int32 `json:"text"`
}

// GiftArt identifies the animation assets of a gift. Model/Pattern are
// collectible attribute ids (only for upgraded/NFT gifts), GiftID points at
// the catalog revision animation used by regular gifts.
type GiftArt struct {
	GiftID     int64      `json:"giftId"`
	ModelID    int64      `json:"modelId"`
	BackdropID int64      `json:"backdropId"`
	PatternID  int64      `json:"patternId"`
	Unique     bool       `json:"unique"`
	Colors     GiftColors `json:"colors"`
}

type GiftMeta struct {
	Slug       string  `json:"slug"`
	Collection string  `json:"collection"`
	Num        int     `json:"num"`
	GiftID     int64   `json:"gift_id"`
	Model      Trait   `json:"model"`
	Backdrop   Trait   `json:"backdrop"`
	Symbol     Trait   `json:"symbol"`
	Art        GiftArt `json:"art"`
}

// GiftAsset is a real gift owned by a DeltaGram account, as shown in the
// "sell a gift" picker. RowID is peer_star_gifts.id.
type GiftAsset struct {
	RowID    int64   `json:"rowId"`
	GiftID   int64   `json:"giftId"`
	Num      int     `json:"num"`
	UniqueID int64   `json:"uniqueId"`
	Title    string  `json:"title"`
	Slug     string  `json:"slug"`
	Date     int64   `json:"date"`
	Unique   bool    `json:"unique"`
	Model    Trait   `json:"model"`
	Backdrop Trait   `json:"backdrop"`
	Symbol   Trait   `json:"symbol"`
	Art      GiftArt `json:"art"`
}

// Banner is a profile banner uploaded through Deltament.
type Banner struct {
	UserID    int64
	Token     string
	MIME      string
	Bytes     []byte
	UpdatedAt time.Time
}

type Lot struct {
	ID       int64
	ServerID int64
	Kind     LotKind
	Name     string
	Slug     string
	Price    Cents
	Status   LotStatus
	EndsAt   time.Time
	Gift     *GiftMeta
	// SellerUserID is zero for operator-created lots. Non-zero means the lot is
	// backed by an asset owned by a marketplace user and must be delivered by
	// deleting/transferring that asset on the DeltaGram server when it settles.
	SellerUserID    int64
	ChannelID       int64
	UsernameAssetID int64
	// GiftRowID points at peer_star_gifts.id for escrowed gift lots.
	GiftRowID int64
	AssetKind AssetKind
	// Escrowed is set once the seller's username/channel asset has actually
	// been moved into the operator vault. A resale lot (the buyer re-lists the
	// purchased item) keeps Escrowed=true: the asset is already in the vault,
	// so settling it must not try to revoke it a second time. It is also what
	// stops a relisted asset from being credited without a real buyer: the
	// seller is paid only when the next purchase is settled.
	Escrowed  bool
	CreatedAt time.Time
}

// AssetKind tells the settler what has to happen to the underlying asset.
type AssetKind string

const (
	// AssetChannel means the lot sells a channel's public username. Settlement
	// deletes the channel and mints the freed username to the winner.
	AssetChannel AssetKind = "channel"
	// AssetCollectible means the lot sells a collectible username asset that is
	// already owned by the seller; settlement transfers it to the winner.
	AssetCollectible AssetKind = "collectible"
	// AssetGift means the lot is backed by a peer_star_gifts row that is held by
	// the relayer account until the buyer claims it.
	AssetGift AssetKind = "gift"
)

type Bid struct {
	ID        int64
	LotID     int64
	UserID    int64
	Masked    string
	Amount    Cents
	Active    bool
	CreatedAt time.Time
}

type LoginCode struct {
	ID           int64
	ServerID     int64
	Phone        string
	ServerUserID int64
	Hash         []byte
	Attempts     int
	ExpiresAt    time.Time
}
