package postgres

import (
	"context"
	"fmt"
	"strings"

	"telesrv/cmd/deltament/internal/domain"
)

const tonDomainCols = `d.name, d.owner_user_id, COALESCE(u.name, ''), COALESCE(u.username, ''),
	d.acquired_at, d.last_price_nanoton, d.listed, d.ask_nanoton`

func scanTONDomain(row interface{ Scan(...any) error }) (domain.TONDomain, error) {
	var d domain.TONDomain
	err := row.Scan(&d.Name, &d.OwnerID, &d.OwnerName, &d.OwnerUser, &d.AcquiredAt, &d.LastPrice, &d.Listed, &d.Ask)
	return d, wrap(err)
}

const tonDomainFrom = `FROM ton_domains d
	LEFT JOIN deltament_market_users u ON u.server_user_id = d.owner_user_id`

func (db *DB) GetTONDomain(ctx context.Context, name string) (domain.TONDomain, error) {
	return scanTONDomain(db.q.QueryRow(ctx, `SELECT `+tonDomainCols+` `+tonDomainFrom+` WHERE d.name = $1`, name))
}

func (db *DB) ListTONDomainsByOwner(ctx context.Context, ownerID int64) ([]domain.TONDomain, error) {
	rows, err := db.q.Query(ctx, `SELECT `+tonDomainCols+` `+tonDomainFrom+`
		WHERE d.owner_user_id = $1 ORDER BY d.acquired_at DESC`, ownerID)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()
	out := make([]domain.TONDomain, 0)
	for rows.Next() {
		d, err := scanTONDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, wrap(rows.Err())
}

func (db *DB) ListTONDomainMarket(ctx context.Context, query, sort string, offset, limit int) ([]domain.TONDomain, int, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	order := "d.updated_at DESC"
	switch sort {
	case "price_asc":
		order = "d.ask_nanoton ASC, d.updated_at DESC"
	case "price_desc":
		order = "d.ask_nanoton DESC, d.updated_at DESC"
	case "name":
		order = "d.name ASC"
	case "date", "":
		order = "d.updated_at DESC"
	}
	args := make([]any, 0, 3)
	where := "WHERE d.listed = TRUE"
	if q := strings.ToLower(strings.TrimSpace(query)); q != "" {
		q = strings.TrimSuffix(strings.TrimPrefix(q, "@"), ".ton")
		args = append(args, "%"+q+"%")
		where += fmt.Sprintf(" AND d.name ILIKE $%d", len(args))
	}
	var total int
	if err := db.q.QueryRow(ctx, `SELECT COUNT(*) `+tonDomainFrom+` `+where, args...).Scan(&total); err != nil {
		return nil, 0, wrap(err)
	}
	args = append(args, limit, offset)
	rows, err := db.q.Query(ctx, `SELECT `+tonDomainCols+` `+tonDomainFrom+` `+where+
		fmt.Sprintf(" ORDER BY %s LIMIT $%d OFFSET $%d", order, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, wrap(err)
	}
	defer rows.Close()
	out := make([]domain.TONDomain, 0, limit)
	for rows.Next() {
		d, err := scanTONDomain(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, d)
	}
	return out, total, wrap(rows.Err())
}

func (db *DB) CreateTONDomain(ctx context.Context, name string, ownerID, price int64) (domain.TONDomain, error) {
	if _, err := db.q.Exec(ctx, `
		INSERT INTO ton_domains (name, owner_user_id, last_price_nanoton)
		VALUES ($1, $2, $3)`, name, ownerID, price); err != nil {
		return domain.TONDomain{}, wrap(err)
	}
	return db.GetTONDomain(ctx, name)
}

func (db *DB) SetTONDomainOwner(ctx context.Context, name string, ownerID, price int64) (domain.TONDomain, error) {
	tag, err := db.q.Exec(ctx, `
		UPDATE ton_domains
		SET owner_user_id = $2, last_price_nanoton = $3, listed = FALSE, ask_nanoton = 0,
		    acquired_at = now(), updated_at = now()
		WHERE name = $1`, name, ownerID, price)
	if err != nil {
		return domain.TONDomain{}, wrap(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.TONDomain{}, domain.ErrNotFound
	}
	return db.GetTONDomain(ctx, name)
}

func (db *DB) SetTONDomainListing(ctx context.Context, name string, ownerID, ask int64) (domain.TONDomain, error) {
	tag, err := db.q.Exec(ctx, `
		UPDATE ton_domains
		SET listed = TRUE, ask_nanoton = $3, updated_at = now()
		WHERE name = $1 AND owner_user_id = $2`, name, ownerID, ask)
	if err != nil {
		return domain.TONDomain{}, wrap(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.TONDomain{}, domain.ErrNotFound
	}
	return db.GetTONDomain(ctx, name)
}

func (db *DB) ClearTONDomainListing(ctx context.Context, name string, ownerID int64) error {
	tag, err := db.q.Exec(ctx, `
		UPDATE ton_domains
		SET listed = FALSE, ask_nanoton = 0, updated_at = now()
		WHERE name = $1 AND owner_user_id = $2`, name, ownerID)
	if err != nil {
		return wrap(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
