package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"telesrv/cmd/deltament/internal/domain"
)

// ValidTONAddress accepts the two user-friendly forms the app sees (UQ../EQ..
// or a raw 48-character base64url string). It is intentionally permissive: the
// owner still confirms every payout manually.
func ValidTONAddress(addr string) bool {
	addr = strings.TrimSpace(addr)
	if strings.HasSuffix(strings.ToLower(addr), ".ton") && len(addr) <= 36 {
		name := strings.TrimSuffix(strings.ToLower(addr), ".ton")
		if name == "" {
			return false
		}
		for _, c := range name {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
				return false
			}
		}
		return true
	}
	if len(addr) != 48 {
		return false
	}
	if strings.HasPrefix(addr, "UQ") || strings.HasPrefix(addr, "EQ") || strings.HasPrefix(addr, "kQ") || strings.HasPrefix(addr, "0Q") {
		return true
	}
	_, err := base64.RawURLEncoding.DecodeString(addr)
	return err == nil
}

func (db *DB) ensureBalanceRow(ctx context.Context, userID int64) error {
	if _, err := db.q.Exec(ctx, `INSERT INTO deltament_balances (user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING`, userID); err != nil {
		return wrap(err)
	}
	// The client renders the TON earnings card from payments.getStarsStatus,
	// which reads ton_balances. DeltaMent credits that same ledger, so a reward
	// credited here is visible in the native wallet immediately.
	if _, err := db.q.Exec(ctx, `INSERT INTO ton_balances (user_id, balance_nanoton, granted) VALUES ($1, 0, false) ON CONFLICT (user_id) DO NOTHING`, userID); err != nil {
		return wrap(err)
	}
	return nil
}

// EnsureTONCard returns the account's card address, creating it on first use
// and making sure the balance row exists.
func (db *DB) EnsureTONCard(ctx context.Context, userID int64) (string, error) {
	if err := db.ensureBalanceRow(ctx, userID); err != nil {
		return "", err
	}
	var addr string
	err := db.q.QueryRow(ctx, `SELECT address FROM deltament_ton_cards WHERE user_id = $1`, userID).Scan(&addr)
	if err == nil {
		return addr, nil
	}
	if !errors.Is(wrap(err), domain.ErrNotFound) {
		return "", wrap(err)
	}
	// The address is entered by the owner. A placeholder is never generated:
	// real TON must only be sent to a wallet the user actually controls.
	if _, err := db.q.Exec(ctx, `
		INSERT INTO deltament_ton_cards (user_id, address) VALUES ($1, '')
		ON CONFLICT (user_id) DO NOTHING`, userID); err != nil {
		return "", wrap(err)
	}
	if err := db.q.QueryRow(ctx, `SELECT address FROM deltament_ton_cards WHERE user_id = $1`, userID).Scan(&addr); err != nil {
		return "", wrap(err)
	}
	return addr, nil
}

// SetTONAddress stores the destination the owner pays out to.
func (db *DB) SetTONAddress(ctx context.Context, userID int64, address string) error {
	address = strings.TrimSpace(address)
	if !ValidTONAddress(address) {
		return fmt.Errorf("%w: invalid TON address", domain.ErrInvalid)
	}
	if _, err := db.EnsureTONCard(ctx, userID); err != nil {
		return err
	}
	_, err := db.q.Exec(ctx, `UPDATE deltament_ton_cards SET address = $2, updated_at = now() WHERE user_id = $1`, userID, address)
	return wrap(err)
}

// TONBalances reads the blue card's balance, the sum of pending payouts and the
// gold card's star balance.
func (db *DB) TONBalances(ctx context.Context, userID int64) (balance, pending, stars int64, err error) {
	if err := db.ensureBalanceRow(ctx, userID); err != nil {
		return 0, 0, 0, err
	}
	if err := db.q.QueryRow(ctx, `
		SELECT tb.balance_nanoton, COALESCE(sb.balance, 0)
		FROM deltament_balances db
		LEFT JOIN ton_balances tb ON tb.user_id = db.user_id
		LEFT JOIN stars_balances sb ON sb.user_id = db.user_id
		WHERE db.user_id = $1`, userID).
		Scan(&balance, &stars); err != nil {
		return 0, 0, 0, wrap(err)
	}
	if err := db.q.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_nanoton), 0) FROM deltament_ton_payouts
		WHERE user_id = $1 AND status = 'pending'`, userID).Scan(&pending); err != nil {
		return 0, 0, 0, wrap(err)
	}
	return balance, pending, stars, nil
}

func (db *DB) logTON(ctx context.Context, userID, amount int64, kind, title string) error {
	_, err := db.q.Exec(ctx, `
		INSERT INTO deltament_balance_transactions (user_id, currency, amount, reason, asset)
		VALUES ($1, 'TON', $2, $3, $4)`, userID, amount, kind, title)
	return wrap(err)
}

// CreditTON adds nanotons to the card and records the ledger line.
func (db *DB) CreditTON(ctx context.Context, userID int64, amount int64, kind, title string) error {
	if amount <= 0 {
		return fmt.Errorf("%w: amount must be positive", domain.ErrInvalid)
	}
	if err := db.ensureBalanceRow(ctx, userID); err != nil {
		return err
	}
	if _, err := db.q.Exec(ctx, `UPDATE ton_balances SET balance_nanoton = balance_nanoton + $2, updated_at = now() WHERE user_id = $1`, userID, amount); err != nil {
		return wrap(err)
	}
	return db.logTON(ctx, userID, amount, kind, title)
}

// DebitTON reserves nanotons on the card. The CHECK on the balance plus the
// explicit predicate turn an overspend into ErrNoFunds instead of negative
// money.
func (db *DB) DebitTON(ctx context.Context, userID int64, amount int64, kind, title string) error {
	if amount <= 0 {
		return fmt.Errorf("%w: amount must be positive", domain.ErrInvalid)
	}
	if err := db.ensureBalanceRow(ctx, userID); err != nil {
		return err
	}
	tag, err := db.q.Exec(ctx, `
		UPDATE ton_balances SET balance_nanoton = balance_nanoton - $2, updated_at = now()
		WHERE user_id = $1 AND balance_nanoton >= $2`, userID, amount)
	if err != nil {
		return wrap(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNoFunds
	}
	return db.logTON(ctx, userID, -amount, kind, title)
}

// CreditStars adds earned stars to the gold card.
func (db *DB) CreditStars(ctx context.Context, userID int64, amount int64, kind, title string) error {
	if amount <= 0 {
		return fmt.Errorf("%w: amount must be positive", domain.ErrInvalid)
	}
	if err := db.ensureBalanceRow(ctx, userID); err != nil {
		return err
	}
	if _, err := db.q.Exec(ctx, `UPDATE deltament_balances SET stars_balance = stars_balance + $2, updated_at = now() WHERE user_id = $1`, userID, amount); err != nil {
		return wrap(err)
	}
	_, err := db.q.Exec(ctx, `
		INSERT INTO deltament_balance_transactions (user_id, currency, amount, reason, asset)
		VALUES ($1, 'XTR', $2, $3, $4)`, userID, amount, kind, title)
	return wrap(err)
}

// TONLedger returns the card's TON movements, newest first.
func (db *DB) TONLedger(ctx context.Context, userID int64, limit int) ([]domain.TONEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.q.Query(ctx, `
		SELECT id, amount, reason, asset, created_at FROM deltament_balance_transactions
		WHERE user_id = $1 AND currency = 'TON' ORDER BY id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()
	out := make([]domain.TONEntry, 0, limit)
	for rows.Next() {
		var e domain.TONEntry
		if err := rows.Scan(&e.ID, &e.Amount, &e.Kind, &e.Title, &e.CreatedAt); err != nil {
			return nil, wrap(err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanTONPayout(row interface{ Scan(...any) error }) (domain.TONPayout, error) {
	var p domain.TONPayout
	var sentAt *time.Time
	err := row.Scan(&p.ID, &p.UserID, &p.Address, &p.Amount, &p.Status, &p.Note, &p.CreatedAt, &sentAt)
	if sentAt != nil {
		p.SentAt = *sentAt
	}
	return p, wrap(err)
}

// CreateTONPayout reserves the amount on the card and queues the send. It is
// one transaction so a failed insert cannot silently burn the balance.
func (db *DB) CreateTONPayout(ctx context.Context, userID int64, address string, amount int64, note string) (domain.TONPayout, error) {
	var p domain.TONPayout
	address = strings.TrimSpace(address)
	if address != "" && !ValidTONAddress(address) {
		return p, fmt.Errorf("%w: invalid TON address", domain.ErrInvalid)
	}
	if amount <= 0 {
		return p, fmt.Errorf("%w: amount must be positive", domain.ErrInvalid)
	}
	if err := db.ensureBalanceRow(ctx, userID); err != nil {
		return p, err
	}
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return p, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE ton_balances SET balance_nanoton = balance_nanoton - $2, updated_at = now()
		WHERE user_id = $1 AND balance_nanoton >= $2`, userID, amount)
	if err != nil {
		return p, wrap(err)
	}
	if tag.RowsAffected() == 0 {
		return p, domain.ErrNoFunds
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO deltament_balance_transactions (user_id, currency, amount, reason, asset)
		VALUES ($1, 'TON', $2, 'payout', $3)`, userID, -amount, "Отправка TON"); err != nil {
		return p, wrap(err)
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO deltament_ton_payouts (user_id, address, amount_nanoton, note)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, address, amount_nanoton, status, note, created_at, sent_at`,
		userID, address, amount, strings.TrimSpace(note))
	p, err = scanTONPayout(row)
	if err != nil {
		return p, err
	}
	if err := tx.Commit(ctx); err != nil {
		return p, err
	}
	return p, nil
}

// ListTONPayouts powers the owner's admin queue.
func (db *DB) ListTONPayouts(ctx context.Context, status string) ([]domain.TONPayout, error) {
	q := `SELECT id, user_id, address, amount_nanoton, status, note, created_at, sent_at FROM deltament_ton_payouts`
	args := []any{}
	if status != "" {
		q += ` WHERE status = $1`
		args = append(args, status)
	}
	q += ` ORDER BY id DESC LIMIT 200`
	rows, err := db.q.Query(ctx, q, args...)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()
	out := make([]domain.TONPayout, 0, 32)
	for rows.Next() {
		p, err := scanTONPayout(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetTONPayoutStatus moves a payout through the queue. A rejected payout is
// refunded to the card; a sent one is not touched again.
func (db *DB) SetTONPayoutStatus(ctx context.Context, id int64, status string) error {
	switch status {
	case "sent", "rejected", "pending":
	default:
		return fmt.Errorf("%w: unknown payout status", domain.ErrInvalid)
	}
	var userID, amount int64
	var current string
	if err := db.q.QueryRow(ctx, `SELECT user_id, amount_nanoton, status FROM deltament_ton_payouts WHERE id = $1`, id).Scan(&userID, &amount, &current); err != nil {
		return wrap(err)
	}
	if current == status {
		return nil
	}
	if current != "pending" {
		return fmt.Errorf("%w: payout is already %s", domain.ErrConflict, current)
	}
	if status == "rejected" {
		if err := db.CreditTON(ctx, userID, amount, "payout_refund", "Возврат отправки TON"); err != nil {
			return err
		}
	}
	tag, err := db.q.Exec(ctx, `
		UPDATE deltament_ton_payouts
		SET status = $2, updated_at = now(), sent_at = CASE WHEN $2 = 'sent' THEN now() ELSE sent_at END
		WHERE id = $1 AND status = 'pending'`, id, status)
	if err != nil {
		return wrap(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

// ResolveTONUser finds the marketplace account behind a messenger id.
func (db *DB) ResolveTONUser(ctx context.Context, mainUserID int64) (domain.User, error) {
	return scanUser(db.q.QueryRow(ctx, `
		SELECT `+userCols+` FROM deltament_market_users
		WHERE server_user_id = $1 ORDER BY id LIMIT 1`, mainUserID))
}

func scanTONOrder(row interface{ Scan(...any) error }) (domain.TONOrder, error) {
	var o domain.TONOrder
	err := row.Scan(&o.ID, &o.UserID, &o.Username, &o.TON, &o.RUB, &o.Status, &o.CreatedAt)
	return o, wrap(err)
}

// CreateTONOrder records a purchase request before the owner is contacted.
func (db *DB) CreateTONOrder(ctx context.Context, userID int64, username string, ton, rub int64) (domain.TONOrder, error) {
	if ton <= 0 || rub <= 0 {
		return domain.TONOrder{}, fmt.Errorf("%w: amount must be positive", domain.ErrInvalid)
	}
	return scanTONOrder(db.q.QueryRow(ctx, `
		INSERT INTO deltament_ton_orders (user_id, username, ton_amount, rub_amount)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, username, ton_amount, rub_amount, status, created_at`,
		userID, strings.TrimSpace(username), ton, rub))
}

func (db *DB) TONOrderStatus(ctx context.Context, id int64) (domain.TONOrder, error) {
	return scanTONOrder(db.q.QueryRow(ctx, `
		SELECT id, user_id, username, ton_amount, rub_amount, status, created_at
		FROM deltament_ton_orders WHERE id = $1`, id))
}

// SetTONOrderStatus advances the order. Marking an order paid credits the card
// once: the status transition is guarded, so a repeated callback cannot pay
// twice.
func (db *DB) SetTONOrderStatus(ctx context.Context, id int64, status string) (domain.TONOrder, error) {
	switch status {
	case "new", "contacted", "paid", "sent", "cancelled":
	default:
		return domain.TONOrder{}, fmt.Errorf("%w: unknown order status", domain.ErrInvalid)
	}
	var out domain.TONOrder
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	var current string
	if err := tx.QueryRow(ctx, `SELECT status FROM deltament_ton_orders WHERE id = $1 FOR UPDATE`, id).Scan(&current); err != nil {
		return out, wrap(err)
	}
	if current == status {
		out, err = scanTONOrder(tx.QueryRow(ctx, `
			SELECT id, user_id, username, ton_amount, rub_amount, status, created_at
			FROM deltament_ton_orders WHERE id = $1`, id))
		return out, err
	}
	if status == "paid" && current != "contacted" && current != "new" {
		return out, fmt.Errorf("%w: order is %s", domain.ErrConflict, current)
	}
	row := tx.QueryRow(ctx, `
		UPDATE deltament_ton_orders SET status = $2, updated_at = now() WHERE id = $1
		RETURNING id, user_id, username, ton_amount, rub_amount, status, created_at`, id, status)
	out, err = scanTONOrder(row)
	if err != nil {
		return out, err
	}
	if status == "paid" && current != "paid" {
		nanoton := out.TON * domain.NanotonPerTON
		if _, err := tx.Exec(ctx, `
			UPDATE ton_balances SET balance_nanoton = balance_nanoton + $2, updated_at = now()
			WHERE user_id = $1`, out.UserID, nanoton); err != nil {
			return out, wrap(err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO deltament_balance_transactions (user_id, currency, amount, reason, asset)
			VALUES ($1, 'TON', $2, 'topup', $3)`, out.UserID, nanoton, "Покупка TON"); err != nil {
			return out, wrap(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return out, err
	}
	return out, nil
}

// PendingTONOrders lists the queue the owner sees in the admin panel.
func (db *DB) PendingTONOrders(ctx context.Context) ([]domain.TONOrder, error) {
	rows, err := db.q.Query(ctx, `
		SELECT id, user_id, username, ton_amount, rub_amount, status, created_at
		FROM deltament_ton_orders WHERE status IN ('new', 'contacted', 'paid')
		ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return nil, wrap(err)
	}
	defer rows.Close()
	out := make([]domain.TONOrder, 0, 16)
	for rows.Next() {
		o, err := scanTONOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
