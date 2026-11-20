package service

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"telesrv/cmd/deltament/internal/domain"
	"telesrv/cmd/deltament/internal/network"
	"telesrv/cmd/deltament/internal/store"
)

type deliverTx struct {
	store.Tx
	items      []domain.Item
	credits    []deliverCredit
	tonCredits []deliverTONCredit
	nextItem   int64
	updated    []domain.Lot
}

type deliverTONCredit struct {
	UserID int64
	Amount int64
}

type deliverCredit struct {
	UserID int64
	Amount domain.Cents
	Kind   domain.HistoryKind
}

func (f *deliverTx) GetUser(_ context.Context, id int64) (domain.User, error) {
	return domain.User{ID: id, ServerID: 7, ServerUserID: id * 10}, nil
}

func (f *deliverTx) UpdateLot(_ context.Context, l domain.Lot) error {
	f.updated = append(f.updated, l)
	return nil
}

func (f *deliverTx) AddItem(_ context.Context, it domain.Item) (domain.Item, error) {
	f.nextItem++
	it.ID = f.nextItem
	f.items = append(f.items, it)
	return it, nil
}

func (f *deliverTx) Credit(_ context.Context, userID int64, amount domain.Cents, kind domain.HistoryKind, _ string) error {
	f.credits = append(f.credits, deliverCredit{UserID: userID, Amount: amount, Kind: kind})
	return nil
}

func (f *deliverTx) CreditTON(_ context.Context, userID int64, amount int64, _ string, _ string) error {
	f.tonCredits = append(f.tonCredits, deliverTONCredit{UserID: userID, Amount: amount})
	return nil
}

type deliverNet struct {
	network.Network
	channels    []network.ChannelAsset
	usernames   []network.UsernameAsset
	deleted     []int64
	transferred []string
	minted      []string
	escrowed    []string
	revoked     []string
	lastOp      network.Op
}

func (f *deliverNet) OwnedChannels(context.Context, int64) ([]network.ChannelAsset, error) {
	return f.channels, nil
}

func (f *deliverNet) OwnedUsernames(context.Context, int64) ([]network.UsernameAsset, error) {
	return f.usernames, nil
}

func (f *deliverNet) DeleteChannel(_ context.Context, op network.Op, channelID int64) error {
	f.lastOp = op
	f.deleted = append(f.deleted, channelID)
	return nil
}

func (f *deliverNet) TransferUsername(_ context.Context, op network.Op, username string, _ int64) error {
	f.lastOp = op
	f.transferred = append(f.transferred, username)
	return nil
}

func (f *deliverNet) GrantUsername(_ context.Context, op network.Op, _ int64, username string) error {
	f.lastOp = op
	f.minted = append(f.minted, username)
	return nil
}

func (f *deliverNet) MintUsernameToVault(_ context.Context, op network.Op, username string) error {
	f.lastOp = op
	f.escrowed = append(f.escrowed, username)
	return nil
}

func (f *deliverNet) RevokeUsername(_ context.Context, op network.Op, username string) error {
	f.lastOp = op
	f.revoked = append(f.revoked, username)
	return nil
}

type deliverNets struct{ net network.Network }

func (f deliverNets) Get(context.Context, int64) (network.Network, error) { return f.net, nil }

func newDeliverMarket(tx store.Tx, net network.Network) *Market {
	return NewMarket(nil, deliverNets{net: net}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestDeliverChannelLotEscrowsUsernameAndCreatesItem(t *testing.T) {
	tx := &deliverTx{}
	net := &deliverNet{channels: []network.ChannelAsset{{ID: 99, Username: "soldchannel"}}}
	m := newDeliverMarket(tx, net)
	lot := domain.Lot{
		ID: 11, ServerID: 7, Name: "soldchannel", Kind: domain.LotUsername, Slug: "soldchannel",
		Price: 5000, SellerUserID: 3, ChannelID: 99, AssetKind: domain.AssetChannel,
		CreatedAt: time.Unix(1700000000, 0),
	}

	item, err := m.deliverUserLot(context.Background(), tx, lot, 4, "Аукцион")
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if len(net.deleted) != 1 || net.deleted[0] != 99 {
		t.Fatalf("deleted = %v, want [99]", net.deleted)
	}
	if len(net.escrowed) != 1 || net.escrowed[0] != "soldchannel" {
		t.Fatalf("escrowed = %v, want [soldchannel]", net.escrowed)
	}
	if len(net.transferred) != 0 || len(net.minted) != 0 {
		t.Fatalf("unexpected direct delivery: transferred=%v minted=%v", net.transferred, net.minted)
	}
	if len(tx.items) != 1 || tx.items[0].UserID != 4 || tx.items[0].Kind != domain.ItemUsername {
		t.Fatalf("items = %+v, want one username item for the winner", tx.items)
	}
	if item.ID != tx.items[0].ID {
		t.Fatalf("returned item %+v, stored %+v", item, tx.items[0])
	}
	if len(tx.credits) != 2 {
		t.Fatalf("credits = %v, want seller and winner entries", tx.credits)
	}
	if tx.credits[0].UserID != 3 || tx.credits[0].Amount != 0 || tx.credits[0].Kind != domain.HistorySell {
		t.Fatalf("seller history entry = %+v", tx.credits[0])
	}
	if tx.credits[1].UserID != 4 || tx.credits[1].Amount != 0 || tx.credits[1].Kind != domain.HistoryWin {
		t.Fatalf("winner credit = %+v", tx.credits[1])
	}
	// The seller is paid in TON to the account card: 5000 cents = 50 GRAM =
	// 50 TON = 50e9 nanotons.
	if len(tx.tonCredits) != 1 || tx.tonCredits[0].UserID != 30 || tx.tonCredits[0].Amount != 50_000_000_000 {
		t.Fatalf("ton credits = %+v", tx.tonCredits)
	}
	if net.lastOp.At.Unix() != 1700000000 {
		t.Fatalf("escrow op.At = %v, want lot creation time", net.lastOp.At)
	}
	if net.lastOp.Value != 5000 {
		t.Fatalf("escrow op.Value = %d, want 5000", net.lastOp.Value)
	}
}

func TestDeliverCollectibleLotRevokesUsernameAndCreatesItem(t *testing.T) {
	tx := &deliverTx{}
	net := &deliverNet{usernames: []network.UsernameAsset{{ID: 5, Username: "rare"}}}
	m := newDeliverMarket(tx, net)
	lot := domain.Lot{
		ID: 12, ServerID: 7, Name: "rare", Kind: domain.LotUsername, Slug: "rare",
		Price: 2500, SellerUserID: 3, UsernameAssetID: 5, AssetKind: domain.AssetCollectible,
		CreatedAt: time.Unix(1700000100, 0),
	}

	if _, err := m.deliverUserLot(context.Background(), tx, lot, 4, "Маркет"); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if len(net.revoked) != 1 || net.revoked[0] != "rare" {
		t.Fatalf("revoked = %v, want [rare]", net.revoked)
	}
	if len(net.deleted) != 0 || len(net.escrowed) != 0 || len(net.transferred) != 0 {
		t.Fatalf("unexpected channel ops: deleted=%v escrowed=%v transferred=%v", net.deleted, net.escrowed, net.transferred)
	}
	if len(tx.items) != 1 || tx.items[0].UserID != 4 {
		t.Fatalf("items = %+v, want one item for the winner", tx.items)
	}
	if len(tx.credits) != 2 || tx.credits[0].Amount != 0 {
		t.Fatalf("credits = %+v", tx.credits)
	}
	if len(tx.tonCredits) != 1 || tx.tonCredits[0].Amount != 25_000_000_000 {
		t.Fatalf("ton credits = %+v", tx.tonCredits)
	}
}

func TestDeliverOperatorLotStaysWithdrawable(t *testing.T) {
	tx := &deliverTx{}
	net := &deliverNet{}
	m := newDeliverMarket(tx, net)
	lot := domain.Lot{ID: 13, ServerID: 7, Name: "operator", Kind: domain.LotUsername, Price: 700}

	if _, err := m.deliverUserLot(context.Background(), tx, lot, 4, "Аукцион"); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if len(tx.items) != 1 || tx.items[0].UserID != 4 {
		t.Fatalf("items = %+v, want one item for winner", tx.items)
	}
	if len(tx.credits) != 0 || len(net.deleted) != 0 || len(net.minted) != 0 || len(net.transferred) != 0 || len(net.escrowed) != 0 {
		t.Fatalf("operator lot must not touch ledger or assets: credits=%v net=%+v", tx.credits, net)
	}
}
