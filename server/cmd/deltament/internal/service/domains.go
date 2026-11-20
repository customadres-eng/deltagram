package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"telesrv/cmd/deltament/internal/domain"
	"telesrv/cmd/deltament/internal/store"
)

// DomainSaleFeePercent is the registry commission on resales. The seller is
// credited the rest immediately, in the same transaction as the buyer's debit.
const DomainSaleFeePercent = 10

type domainStore interface {
	store.TONDomains
	store.TON
	store.TxRunner
}

// Domains implements the DeltaGram .ton registry. It is deliberately
// off-chain: names and balances live in the same ledger as the TON cards, and
// the owner settles real TON DNS names by hand, like every other TON payout.
type Domains struct {
	db  domainStore
	log *slog.Logger
}

func NewDomains(db domainStore, log *slog.Logger) *Domains {
	return &Domains{db: db, log: log}
}

func (d *Domains) Check(ctx context.Context, userID int64, raw string) (domain.TONDomainCheck, error) {
	name := domain.NormalizeTONDomain(raw)
	if name == "" {
		return domain.TONDomainCheck{}, fmt.Errorf("%w: invalid domain name", domain.ErrInvalid)
	}
	base := domain.TONDomainBasePrice(name)
	out := domain.TONDomainCheck{Name: name, BasePrice: base}
	dom, err := d.db.GetTONDomain(ctx, name)
	if errors.Is(err, domain.ErrNotFound) {
		out.State = domain.TONDomainAvailable
		out.Price = base
		return out, nil
	}
	if err != nil {
		return domain.TONDomainCheck{}, err
	}
	out.OwnerID = dom.OwnerID
	out.OwnerName = dom.OwnerName
	out.OwnerUser = dom.OwnerUser
	switch {
	case dom.OwnerID == userID:
		out.State = domain.TONDomainYours
		out.Price = 0
	case dom.Listed:
		out.State = domain.TONDomainForSale
		out.Price = dom.Ask
	default:
		out.State = domain.TONDomainTaken
		out.Price = 0
	}
	return out, nil
}

func (d *Domains) My(ctx context.Context, userID int64) ([]domain.TONDomain, error) {
	return d.db.ListTONDomainsByOwner(ctx, userID)
}

func (d *Domains) Market(ctx context.Context, query, sort string, page, limit int) ([]domain.TONDomain, int, error) {
	if page < 0 {
		page = 0
	}
	return d.db.ListTONDomainMarket(ctx, query, sort, page*limit, limit)
}

func (d *Domains) Buy(ctx context.Context, userID int64, raw string) (domain.TONDomain, error) {
	name := domain.NormalizeTONDomain(raw)
	if name == "" || userID <= 0 {
		return domain.TONDomain{}, fmt.Errorf("%w: invalid domain request", domain.ErrInvalid)
	}
	var out domain.TONDomain
	err := d.db.InTx(ctx, func(tx store.Tx) error {
		dom, err := tx.GetTONDomain(ctx, name)
		if err == nil {
			if dom.OwnerID == userID {
				return fmt.Errorf("%w: domain already belongs to you", domain.ErrConflict)
			}
			if !dom.Listed {
				return fmt.Errorf("%w: domain is not for sale", domain.ErrConflict)
			}
			price := dom.Ask
			if price <= 0 {
				price = domain.TONDomainBasePrice(name)
			}
			if err := tx.DebitTON(ctx, userID, price, "domain_buy", "Покупка домена "+name+".ton"); err != nil {
				return err
			}
			fee := price * DomainSaleFeePercent / 100
			if err := tx.CreditTON(ctx, dom.OwnerID, price-fee, "domain_sale", "Продажа домена "+name+".ton"); err != nil {
				return err
			}
			out, err = tx.SetTONDomainOwner(ctx, name, userID, price)
			return err
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		price := domain.TONDomainBasePrice(name)
		if err := tx.DebitTON(ctx, userID, price, "domain_buy", "Покупка домена "+name+".ton"); err != nil {
			return err
		}
		out, err = tx.CreateTONDomain(ctx, name, userID, price)
		return err
	})
	if err != nil {
		return domain.TONDomain{}, err
	}
	d.log.Info("domain bought", "user", userID, "name", name)
	return out, nil
}

func (d *Domains) List(ctx context.Context, userID int64, raw string, ask int64) (domain.TONDomain, error) {
	name := domain.NormalizeTONDomain(raw)
	if name == "" || userID <= 0 {
		return domain.TONDomain{}, fmt.Errorf("%w: invalid domain request", domain.ErrInvalid)
	}
	min := domain.TONDomainBasePrice(name)
	if min < domain.NanotonPerTON {
		min = domain.NanotonPerTON
	}
	if ask < min || ask > 1_000_000*domain.NanotonPerTON {
		return domain.TONDomain{}, fmt.Errorf("%w: price must be between %d and 1000000 TON", domain.ErrInvalid, min/domain.NanotonPerTON)
	}
	dom, err := d.db.GetTONDomain(ctx, name)
	if err != nil {
		return domain.TONDomain{}, err
	}
	if dom.OwnerID != userID {
		return domain.TONDomain{}, domain.ErrForbidden
	}
	return d.db.SetTONDomainListing(ctx, name, userID, ask)
}

func (d *Domains) Unlist(ctx context.Context, userID int64, raw string) (domain.TONDomain, error) {
	name := domain.NormalizeTONDomain(raw)
	if name == "" || userID <= 0 {
		return domain.TONDomain{}, fmt.Errorf("%w: invalid domain request", domain.ErrInvalid)
	}
	dom, err := d.db.GetTONDomain(ctx, name)
	if err != nil {
		return domain.TONDomain{}, err
	}
	if dom.OwnerID != userID {
		return domain.TONDomain{}, domain.ErrForbidden
	}
	if err := d.db.ClearTONDomainListing(ctx, name, userID); err != nil {
		return domain.TONDomain{}, err
	}
	return d.db.GetTONDomain(ctx, name)
}
