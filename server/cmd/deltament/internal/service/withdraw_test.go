package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"telesrv/cmd/deltament/internal/domain"
	"telesrv/cmd/deltament/internal/network"
	"telesrv/cmd/deltament/internal/store"
)

type withdrawStore struct {
	walletStore
	server      domain.Server
	debits      []domain.Cents
	credits     []domain.Cents
	starCredits int64
}

func (f *withdrawStore) GetServer(context.Context, int64) (domain.Server, error) {
	return f.server, nil
}

func (f *withdrawStore) InTx(ctx context.Context, fn func(store.Tx) error) error {
	return fn(&withdrawTx{parent: f})
}

func (f *withdrawStore) Debit(_ context.Context, _ int64, amount domain.Cents, _ domain.HistoryKind, _ string) error {
	f.debits = append(f.debits, amount)
	return nil
}

func (f *withdrawStore) Credit(_ context.Context, _ int64, amount domain.Cents, _ domain.HistoryKind, _ string) error {
	f.credits = append(f.credits, amount)
	return nil
}

func (f *withdrawStore) CreditStars(_ context.Context, _ int64, amount int64, _ string, _ string) error {
	f.starCredits += amount
	return nil
}

func (f *withdrawStore) History(context.Context, int64, int) ([]domain.HistoryEntry, error) {
	return nil, nil
}

type withdrawTx struct {
	store.Tx
	parent *withdrawStore
}

func (f *withdrawTx) Debit(_ context.Context, _ int64, amount domain.Cents, _ domain.HistoryKind, _ string) error {
	f.parent.debits = append(f.parent.debits, amount)
	return nil
}

type starsNet struct {
	network.Network
	granted int64
	err     error
}

func (f *starsNet) GrantStars(_ context.Context, _ network.Op, _ int64, amount int64) error {
	if f.err != nil {
		return f.err
	}
	f.granted = amount
	return nil
}

func newWithdrawWallet(db *withdrawStore, net network.Network) *Wallet {
	return NewWallet(db, deliverNets{net: net}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestWithdrawStarsConvertsGramAtServerRate(t *testing.T) {
	db := &withdrawStore{server: domain.Server{ID: 7, StarsPerGram: 100}}
	net := &starsNet{}
	w := newWithdrawWallet(db, net)
	u := domain.User{ID: 4, ServerID: 7, ServerUserID: 40}

	stars, err := w.WithdrawStars(context.Background(), u, 100)
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if stars != 100 || net.granted != 100 {
		t.Fatalf("stars=%d granted=%d, want 100", stars, net.granted)
	}
	if len(db.debits) != 1 || db.debits[0] != 100 {
		t.Fatalf("debits = %v, want [100]", db.debits)
	}
	if len(db.credits) != 0 {
		t.Fatalf("unexpected refund: %v", db.credits)
	}
}

func TestWithdrawStarsFractionalGram(t *testing.T) {
	db := &withdrawStore{server: domain.Server{ID: 7, StarsPerGram: 100}}
	net := &starsNet{}
	w := newWithdrawWallet(db, net)

	stars, err := w.WithdrawStars(context.Background(), domain.User{ID: 4, ServerID: 7, ServerUserID: 40}, 250)
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if stars != 250 || net.granted != 250 {
		t.Fatalf("stars=%d granted=%d, want 250", stars, net.granted)
	}
}

func TestWithdrawStarsRejectsBelowOneGram(t *testing.T) {
	db := &withdrawStore{server: domain.Server{ID: 7, StarsPerGram: 100}}
	w := newWithdrawWallet(db, &starsNet{})

	if _, err := w.WithdrawStars(context.Background(), domain.User{ID: 4, ServerID: 7, ServerUserID: 40}, 99); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if len(db.debits) != 0 {
		t.Fatalf("debits = %v, want none", db.debits)
	}
}

func TestWithdrawStarsRefundsWhenGrantFails(t *testing.T) {
	db := &withdrawStore{server: domain.Server{ID: 7, StarsPerGram: 100}}
	net := &starsNet{err: errors.New("admin api down")}
	w := newWithdrawWallet(db, net)

	if _, err := w.WithdrawStars(context.Background(), domain.User{ID: 4, ServerID: 7, ServerUserID: 40}, 300); err == nil {
		t.Fatal("expected error")
	}
	if len(db.debits) != 1 || db.debits[0] != 300 {
		t.Fatalf("debits = %v, want [300]", db.debits)
	}
	if len(db.credits) != 1 || db.credits[0] != 300 {
		t.Fatalf("refunds = %v, want [300]", db.credits)
	}
}
