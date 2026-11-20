package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"telesrv/cmd/deltament/internal/domain"
	"telesrv/cmd/deltament/internal/network"
	"telesrv/cmd/deltament/internal/store"
)

func TestMinBid(t *testing.T) {
	cases := []struct {
		highest domain.Cents
		want    domain.Cents
	}{
		{100, 200},       // 1 -> шаг минимум 1
		{1000, 1100},     // 10 -> 10+1
		{10000, 10500},   // 100 -> 100+5
		{123400, 129600}, // 1234 -> 1234+62
	}
	for _, c := range cases {
		if got := MinBid(c.highest); got != c.want {
			t.Errorf("MinBid(%d) = %d, want %d", c.highest, got, c.want)
		}
	}
}

func TestFormatNumber(t *testing.T) {
	if got := formatNumber("88800000000"); got != "+888 0000 0000" {
		t.Errorf("got %q", got)
	}
	if got := formatNumber("79991234567"); got != "+79991234567" {
		t.Errorf("got %q", got)
	}
}

func TestCreateUserLotRejectsPriceAboveCap(t *testing.T) {
	db := &listingStore{}
	net := &deliverNet{usernames: []network.UsernameAsset{{ID: 3, Username: "money"}}}
	m := newListingMarket(db, net)

	_, err := m.CreateUserLot(context.Background(), listingUser(), NewUserLot{
		AssetKind:       domain.AssetCollectible,
		UsernameAssetID: 3,
		Username:        "money",
		Price:           maxLotCents + 1,
	})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if !strings.Contains(err.Error(), "100000") {
		t.Fatalf("err = %v, want the cap in the message", err)
	}
}

func TestCreateUserLotAllowsPriceAtCap(t *testing.T) {
	db := &listingStore{}
	net := &deliverNet{usernames: []network.UsernameAsset{{ID: 3, Username: "money"}}}
	m := newListingMarket(db, net)

	if _, err := m.CreateUserLot(context.Background(), listingUser(), NewUserLot{
		AssetKind:       domain.AssetCollectible,
		UsernameAssetID: 3,
		Username:        "money",
		Price:           maxLotCents,
	}); err != nil {
		t.Fatalf("create at cap: %v", err)
	}
}

func TestCreateLotRejectsPriceAboveCap(t *testing.T) {
	db := &listingStore{}
	m := newListingMarket(db, &deliverNet{})

	_, err := m.CreateLot(context.Background(), 7, NewLot{
		Kind:  domain.LotUsername,
		Name:  "money",
		Price: maxLotCents + 1,
	})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

type capBidTx struct {
	store.Tx
	lot     domain.Lot
	debited bool
}

func (f *capBidTx) LockLot(context.Context, int64) (domain.Lot, error) {
	return f.lot, nil
}

func (f *capBidTx) TopBid(context.Context, int64) (domain.Bid, error) {
	return domain.Bid{}, domain.ErrNotFound
}

func (f *capBidTx) Debit(context.Context, int64, domain.Cents, domain.HistoryKind, string) error {
	f.debited = true
	return nil
}

type capBidStore struct {
	marketStore
	lot domain.Lot
	tx  *capBidTx
}

func (f *capBidStore) InTx(_ context.Context, fn func(store.Tx) error) error {
	f.tx = &capBidTx{lot: f.lot}
	return fn(f.tx)
}

func TestBidRejectsAmountAboveCap(t *testing.T) {
	db := &capBidStore{lot: domain.Lot{
		ID: 5, ServerID: 7, Kind: domain.LotUsername, Status: domain.LotAuction,
		Price: 100, EndsAt: time.Now().Add(time.Hour),
	}}
	m := newListingMarket(db, &deliverNet{})

	err := m.Bid(context.Background(), listingUser(), 5, maxLotCents+1)
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if db.tx.debited {
		t.Fatal("an over-cap bid must not debit the bidder")
	}
	if !strings.Contains(err.Error(), "100000") {
		t.Fatalf("err = %v, want the cap in the message", err)
	}
}
