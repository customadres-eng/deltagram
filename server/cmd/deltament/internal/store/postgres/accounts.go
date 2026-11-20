package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"telesrv/cmd/deltament/internal/domain"
)

func affected(tag pgconn.CommandTag, err error) error {
	if err != nil {
		return wrap(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// коды входа

func (db *DB) SaveCode(ctx context.Context, c domain.LoginCode) error {
	_, err := db.q.Exec(ctx, `
		INSERT INTO deltament_market_login_codes (server_id, phone, server_user_id, hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)`,
		c.ServerID, c.Phone, c.ServerUserID, c.Hash, c.ExpiresAt)
	return wrap(err)
}

func (db *DB) LatestCode(ctx context.Context, serverID int64, phone string) (domain.LoginCode, error) {
	var c domain.LoginCode
	err := db.q.QueryRow(ctx, `
		SELECT id, server_id, phone, server_user_id, hash, attempts, expires_at
		FROM deltament_market_login_codes WHERE server_id = $1 AND phone = $2
		ORDER BY id DESC LIMIT 1`, serverID, phone).
		Scan(&c.ID, &c.ServerID, &c.Phone, &c.ServerUserID, &c.Hash, &c.Attempts, &c.ExpiresAt)
	return c, wrap(err)
}

func (db *DB) BumpCodeAttempts(ctx context.Context, id int64) error {
	return affected(db.q.Exec(ctx, `UPDATE deltament_market_login_codes SET attempts = attempts + 1 WHERE id = $1`, id))
}

func (db *DB) DeleteCodes(ctx context.Context, serverID int64, phone string) error {
	_, err := db.q.Exec(ctx, `DELETE FROM deltament_market_login_codes WHERE server_id = $1 AND phone = $2`, serverID, phone)
	return wrap(err)
}

func (db *DB) CountCodesSince(ctx context.Context, serverID int64, phone string, since time.Time) (int, error) {
	var n int
	err := db.q.QueryRow(ctx, `
		SELECT count(*) FROM deltament_market_login_codes WHERE server_id = $1 AND phone = $2 AND created_at > $3`,
		serverID, phone, since).Scan(&n)
	return n, wrap(err)
}

// юзеры

const userCols = `id, server_id, server_user_id, phone, name, username, balance, created_at`

func scanUser(row interface{ Scan(...any) error }) (domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.ServerID, &u.ServerUserID, &u.Phone, &u.Name, &u.Username, &u.Balance, &u.CreatedAt)
	return u, wrap(err)
}

// новый юзер создаётся, старый просто обновляет номер
func (db *DB) UpsertUser(ctx context.Context, u domain.User) (domain.User, bool, error) {
	var created bool
	var out domain.User
	err := db.q.QueryRow(ctx, `
		INSERT INTO deltament_market_users (server_id, server_user_id, phone, name, username)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (server_id, server_user_id) DO UPDATE SET phone = EXCLUDED.phone
		RETURNING `+userCols+`, (xmax = 0)`,
		u.ServerID, u.ServerUserID, u.Phone, u.Name, u.Username).
		Scan(&out.ID, &out.ServerID, &out.ServerUserID, &out.Phone, &out.Name, &out.Username, &out.Balance, &out.CreatedAt, &created)
	return out, created, wrap(err)
}

func (db *DB) GetUser(ctx context.Context, id int64) (domain.User, error) {
	return scanUser(db.q.QueryRow(ctx, `SELECT `+userCols+` FROM deltament_market_users WHERE id = $1`, id))
}

// сессии

const sessionCols = `id, user_id, device, place, created_at, last_seen`

func scanSession(row interface{ Scan(...any) error }) (domain.Session, error) {
	var s domain.Session
	err := row.Scan(&s.ID, &s.UserID, &s.Device, &s.Place, &s.CreatedAt, &s.LastSeen)
	return s, wrap(err)
}

func (db *DB) CreateSession(ctx context.Context, s domain.Session, tokenHash []byte) error {
	_, err := db.q.Exec(ctx, `
		INSERT INTO deltament_market_sessions (id, user_id, token_hash, device, place) VALUES ($1, $2, $3, $4, $5)`,
		s.ID, s.UserID, tokenHash, s.Device, s.Place)
	return wrap(err)
}

func (db *DB) SessionByToken(ctx context.Context, tokenHash []byte) (domain.Session, error) {
	return scanSession(db.q.QueryRow(ctx, `SELECT `+sessionCols+` FROM deltament_market_sessions WHERE token_hash = $1`, tokenHash))
}

func (db *DB) TouchSession(ctx context.Context, id string) error {
	_, err := db.q.Exec(ctx, `UPDATE deltament_market_sessions SET last_seen = now() WHERE id = $1`, id)
	return wrap(err)
}

func (db *DB) ListSessions(ctx context.Context, userID int64) ([]domain.Session, error) {
	rows, err := db.q.Query(ctx, `SELECT `+sessionCols+` FROM deltament_market_sessions WHERE user_id = $1 ORDER BY last_seen DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (db *DB) DeleteSession(ctx context.Context, userID int64, id string) error {
	return affected(db.q.Exec(ctx, `DELETE FROM deltament_market_sessions WHERE user_id = $1 AND id = $2`, userID, id))
}

func (db *DB) DeleteOtherSessions(ctx context.Context, userID int64, keepID string) error {
	_, err := db.q.Exec(ctx, `DELETE FROM deltament_market_sessions WHERE user_id = $1 AND id <> $2`, userID, keepID)
	return wrap(err)
}

// баланс и история

func (db *DB) Credit(ctx context.Context, userID int64, amount domain.Cents, kind domain.HistoryKind, title string) error {
	if err := affected(db.q.Exec(ctx, `UPDATE deltament_market_users SET balance = balance + $2 WHERE id = $1`, userID, amount)); err != nil {
		return err
	}
	return db.log(ctx, userID, kind, title, amount)
}

// чек в таблице не даст уйти в минус, ловим это как ErrNoFunds
func (db *DB) Debit(ctx context.Context, userID int64, amount domain.Cents, kind domain.HistoryKind, title string) error {
	if err := affected(db.q.Exec(ctx, `UPDATE deltament_market_users SET balance = balance - $2 WHERE id = $1`, userID, amount)); err != nil {
		return err
	}
	return db.log(ctx, userID, kind, title, -amount)
}

func (db *DB) log(ctx context.Context, userID int64, kind domain.HistoryKind, title string, amount domain.Cents) error {
	_, err := db.q.Exec(ctx, `INSERT INTO deltament_market_history (user_id, kind, title, amount) VALUES ($1, $2, $3, $4)`, userID, kind, title, amount)
	return wrap(err)
}

func (db *DB) History(ctx context.Context, userID int64, limit int) ([]domain.HistoryEntry, error) {
	rows, err := db.q.Query(ctx, `
		SELECT id, user_id, kind, title, amount, created_at FROM deltament_market_history
		WHERE user_id = $1 ORDER BY id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.HistoryEntry
	for rows.Next() {
		var h domain.HistoryEntry
		if err := rows.Scan(&h.ID, &h.UserID, &h.Kind, &h.Title, &h.Amount, &h.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// инвентарь

const itemCols = `id, user_id, lot_id, kind, title, name, slug, num, gift_id, gift_row_id, art, amount, value, source, status, created_at`

func scanItem(row interface{ Scan(...any) error }) (domain.Item, error) {
	var it domain.Item
	var art []byte
	err := row.Scan(&it.ID, &it.UserID, &it.LotID, &it.Kind, &it.Title, &it.Name, &it.Slug, &it.Num, &it.GiftID, &it.GiftRowID, &art, &it.Amount, &it.Value, &it.Source, &it.Status, &it.CreatedAt)
	if err != nil {
		return it, wrap(err)
	}
	if len(art) > 0 {
		if err := json.Unmarshal(art, &it.Art); err != nil {
			return it, err
		}
	}
	return it, nil
}

func (db *DB) AddItem(ctx context.Context, it domain.Item) (domain.Item, error) {
	art, err := json.Marshal(it.Art)
	if err != nil {
		return it, err
	}
	return scanItem(db.q.QueryRow(ctx, `
		INSERT INTO deltament_market_items (user_id, lot_id, kind, title, name, slug, num, gift_id, gift_row_id, art, amount, value, source)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING `+itemCols,
		it.UserID, it.LotID, it.Kind, it.Title, it.Name, it.Slug, it.Num, it.GiftID, it.GiftRowID, art, it.Amount, it.Value, it.Source))
}

func (db *DB) LockItem(ctx context.Context, userID, id int64) (domain.Item, error) {
	return scanItem(db.q.QueryRow(ctx, `SELECT `+itemCols+` FROM deltament_market_items WHERE id = $1 AND user_id = $2 FOR UPDATE`, id, userID))
}

func (db *DB) SetItemStatus(ctx context.Context, id int64, status domain.ItemStatus) error {
	return affected(db.q.Exec(ctx, `UPDATE deltament_market_items SET status = $2 WHERE id = $1`, id, status))
}

func (db *DB) HasItemForLot(ctx context.Context, lotID int64) (bool, error) {
	var ok bool
	if err := db.q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM deltament_market_items WHERE lot_id = $1)`, lotID).Scan(&ok); err != nil {
		return false, wrap(err)
	}
	return ok, nil
}

func (db *DB) ListItems(ctx context.Context, userID int64) ([]domain.Item, error) {
	rows, err := db.q.Query(ctx, `SELECT `+itemCols+` FROM deltament_market_items WHERE user_id = $1 AND status = 'owned' ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
