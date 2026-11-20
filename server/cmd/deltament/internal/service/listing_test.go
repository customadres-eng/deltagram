package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"telesrv/cmd/deltament/internal/domain"
	"telesrv/cmd/deltament/internal/network"
)

type listingStore struct {
	marketStore
	created domain.Lot
	prev    *domain.Lot
}

func (f *listingStore) LotBySlug(context.Context, int64, domain.LotKind, string) (domain.Lot, error) {
	if f.prev != nil {
		return *f.prev, nil
	}
	return domain.Lot{}, domain.ErrNotFound
}

func (f *listingStore) CreateLot(_ context.Context, l domain.Lot) (domain.Lot, error) {
	l.ID = 77
	f.created = l
	return l, nil
}

func newListingMarket(db marketStore, net network.Network) *Market {
	return NewMarket(db, deliverNets{net: net}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func listingUser() domain.User {
	return domain.User{ID: 2, ServerID: 7, ServerUserID: 20}
}

func TestCreateUserLotFixedPrice(t *testing.T) {
	db := &listingStore{}
	net := &deliverNet{usernames: []network.UsernameAsset{{ID: 3, Username: "money"}}}
	m := newListingMarket(db, net)

	l, err := m.CreateUserLot(context.Background(), listingUser(), NewUserLot{
		AssetKind:       domain.AssetCollectible,
		UsernameAssetID: 3,
		Username:        "money",
		Price:           500,
		Auction:         false,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if l.Status != domain.LotSale {
		t.Fatalf("status = %q, want sale", l.Status)
	}
	if !l.EndsAt.IsZero() {
		t.Fatalf("ends_at = %v, want zero for fixed sale", l.EndsAt)
	}
	if db.created.SellerUserID != 2 || db.created.Price != 500 {
		t.Fatalf("created = %+v", db.created)
	}
}

func TestCreateUserLotAuctionNeedsDuration(t *testing.T) {
	db := &listingStore{}
	net := &deliverNet{usernames: []network.UsernameAsset{{ID: 3, Username: "money"}}}
	m := newListingMarket(db, net)

	_, err := m.CreateUserLot(context.Background(), listingUser(), NewUserLot{
		AssetKind:       domain.AssetCollectible,
		UsernameAssetID: 3,
		Username:        "money",
		Price:           500,
		Auction:         true,
	})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestCreateUserLotAuctionSetsEndsAt(t *testing.T) {
	db := &listingStore{}
	net := &deliverNet{usernames: []network.UsernameAsset{{ID: 3, Username: "money"}}}
	m := newListingMarket(db, net)
	start := time.Now()

	l, err := m.CreateUserLot(context.Background(), listingUser(), NewUserLot{
		AssetKind:       domain.AssetCollectible,
		UsernameAssetID: 3,
		Username:        "money",
		Price:           500,
		Auction:         true,
		Duration:        24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if l.Status != domain.LotAuction {
		t.Fatalf("status = %q, want auction", l.Status)
	}
	if l.EndsAt.Before(start.Add(23 * time.Hour)) {
		t.Fatalf("ends_at = %v, want about 24h ahead", l.EndsAt)
	}
}

func TestCreateUserLotRejectsSecondActiveListing(t *testing.T) {
	db := &listingStore{prev: &domain.Lot{Status: domain.LotSale}}
	net := &deliverNet{usernames: []network.UsernameAsset{{ID: 3, Username: "money"}}}
	m := newListingMarket(db, net)

	_, err := m.CreateUserLot(context.Background(), listingUser(), NewUserLot{
		AssetKind:       domain.AssetCollectible,
		UsernameAssetID: 3,
		Username:        "money",
		Price:           500,
		Auction:         false,
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}
