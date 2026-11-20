package service

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"telesrv/cmd/deltament/internal/domain"
	"telesrv/cmd/deltament/internal/store"
)

type sellTx struct {
	store.Tx
	item    domain.Item
	lot     domain.Lot
	credits []deliverCredit
	status  domain.ItemStatus
	updated bool
}

func (f *sellTx) LockItem(context.Context, int64, int64) (domain.Item, error) { return f.item, nil }
func (f *sellTx) SetItemStatus(_ context.Context, _ int64, status domain.ItemStatus) error {
	f.status = status
	return nil
}
func (f *sellTx) LockLot(context.Context, int64) (domain.Lot, error) { return f.lot, nil }
func (f *sellTx) UpdateLot(_ context.Context, l domain.Lot) error {
	f.lot = l
	f.updated = true
	return nil
}
func (f *sellTx) Credit(_ context.Context, userID int64, amount domain.Cents, kind domain.HistoryKind, _ string) error {
	f.credits = append(f.credits, deliverCredit{UserID: userID, Amount: amount, Kind: kind})
	return nil
}

type sellStore struct {
	walletStore
	tx *sellTx
}

func (f *sellStore) InTx(_ context.Context, fn func(store.Tx) error) error { return fn(f.tx) }

func TestSellRelistsAsConsignment(t *testing.T) {
	tx := &sellTx{
		item: domain.Item{ID: 9, UserID: 4, LotID: 12, Kind: domain.ItemUsername, Name: "rare", Title: "@rare", Value: 2500, Status: domain.ItemOwned},
		lot: domain.Lot{
			ID: 12, ServerID: 7, Kind: domain.LotUsername, Name: "rare", Slug: "rare",
			Price: 2500, Status: domain.LotSold, SellerUserID: 3, ChannelID: 99,
			UsernameAssetID: 5, AssetKind: domain.AssetChannel, Escrowed: true,
		},
	}
	db := &sellStore{tx: tx}
	w := NewWallet(db, deliverNets{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	got, err := w.Sell(context.Background(), domain.User{ID: 4, ServerID: 7}, 9)
	if err != nil {
		t.Fatalf("sell: %v", err)
	}
	if got != 2500 {
		t.Fatalf("listing price = %d, want 2500", got)
	}
	if tx.status != domain.ItemSold {
		t.Fatalf("item status = %q, want sold", tx.status)
	}
	if !tx.updated || tx.lot.Status != domain.LotSale || tx.lot.Price != 2500 {
		t.Fatalf("lot = %+v", tx.lot)
	}
	if tx.lot.SellerUserID != 4 || !tx.lot.Escrowed {
		t.Fatalf("resale lot must keep the consignor and stay escrowed: %+v", tx.lot)
	}
	// The old implementation credited the seller here, which let two accounts
	// mint unlimited GRAM. A resale may only pay out when a real buyer settles.
	if len(tx.credits) != 0 {
		t.Fatalf("sell must not credit before a buyer pays: %+v", tx.credits)
	}
}
