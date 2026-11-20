package main

import (
	"bytes"
	"context"
	"crypto/rsa"
	"embed"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The marketplace UI is a compiled static bundle. Keeping it embedded makes
// the container self-contained and avoids a second web server in production.
//
//go:embed static/index.html static/favicon.png static/assets
var staticFiles embed.FS

type app struct {
	db          *pgxpool.Pool
	loginKey    *rsa.PrivateKey
	loginIssuer string
	sessionKey  []byte
	auth        authBridge
}

func (a *app) isDeltamentAdmin(ctx context.Context, uid int64) bool {
	if uid <= 0 {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("DELTAMENT_ADMIN_USER_IDS")), "*") {
		return true
	}
	for _, raw := range strings.Split(os.Getenv("DELTAMENT_ADMIN_USER_IDS"), ",") {
		if n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && n == uid {
			return true
		}
	}
	var support bool
	return a.db.QueryRow(ctx, `SELECT COALESCE(support,false) FROM users WHERE id=$1 AND deleted_at IS NULL`, uid).Scan(&support) == nil && support
}

type lot struct {
	ID             int64     `json:"id"`
	Kind           string    `json:"kind"`
	Username       string    `json:"username"`
	Status         string    `json:"status"`
	Currency       string    `json:"currency"`
	Amount         int64     `json:"amount"`
	CryptoCurrency string    `json:"crypto_currency"`
	CryptoAmount   int64     `json:"crypto_amount"`
	OwnerUsername  string    `json:"owner_username"`
	URL            string    `json:"url"`
	EndsAt         time.Time `json:"-"`
	BidCount       int
	Bids           []map[string]any
}

func main() {
	ctx := context.Background()
	dsn := os.Getenv("DELTAMENT_POSTGRES_DSN")
	if dsn == "" {
		dsn = os.Getenv("TELESRV_POSTGRES_DSN")
	}
	if dsn == "" {
		log.Fatal("DELTAMENT_POSTGRES_DSN or TELESRV_POSTGRES_DSN is required")
	}
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS deltament_user_settings (
		user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		language TEXT NOT NULL DEFAULT 'ru' CHECK (language IN ('ru', 'en')),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS deltament_balances (
		user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		stars_balance BIGINT NOT NULL DEFAULT 0 CHECK (stars_balance >= 0),
		ton_balance_nanoton BIGINT NOT NULL DEFAULT 0 CHECK (ton_balance_nanoton >= 0),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS deltament_topup_transfers (
		idempotency_key TEXT PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
		currency TEXT NOT NULL CHECK (currency IN ('XTR', 'TON')),
		amount BIGINT NOT NULL CHECK (amount > 0),
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS deltament_balance_transactions (
		id BIGSERIAL PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
		currency TEXT NOT NULL CHECK (currency IN ('XTR', 'TON')),
		amount BIGINT NOT NULL CHECK (amount <> 0),
		reason TEXT NOT NULL,
		asset TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS deltament_wallet_grants (
		user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS deltament_withdrawals (
		idempotency_key TEXT PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
		ton_amount_nanoton BIGINT NOT NULL CHECK (ton_amount_nanoton > 0),
		stars_amount BIGINT NOT NULL CHECK (stars_amount > 0),
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS deltament_auction_bids (
		id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		auction_id BIGINT NOT NULL REFERENCES deltament_auctions(id) ON DELETE CASCADE,
		bidder_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
		amount BIGINT NOT NULL CHECK (amount > 0),
		active BOOLEAN NOT NULL DEFAULT true,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		UNIQUE (auction_id, bidder_user_id)
	);
	CREATE INDEX IF NOT EXISTS deltament_auction_bids_top_idx ON deltament_auction_bids(auction_id, active, amount DESC, created_at ASC)`); err != nil {
		log.Fatal(err)
	}
	loginKey, err := loadLoginKey(os.Getenv("DELTAMENT_LOGIN_KEY"))
	if err != nil {
		log.Fatal(err)
	}
	sessionKey, err := loadSessionKey(os.Getenv("DELTAMENT_SESSION_KEY"))
	if err != nil {
		log.Fatal(err)
	}
	authSecret := strings.TrimSpace(os.Getenv("DELTAMENT_AUTH_SECRET"))
	if len(authSecret) < 24 {
		log.Fatal("DELTAMENT_AUTH_SECRET must contain at least 24 characters")
	}
	a := &app{db: db, loginKey: loginKey, loginIssuer: envOr("DELTAMENT_LOGIN_ISSUER", "https://dltagram.duckdns.org"), sessionKey: sessionKey, auth: authBridge{URL: envOr("DELTAMENT_AUTH_URL", "http://127.0.0.1:2801"), Secret: authSecret}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.healthz)
	mux.HandleFunc("GET /api/config", a.apiConfig)
	mux.HandleFunc("GET /api/lots", a.apiLots)
	mux.HandleFunc("POST /api/lots", a.apiCreateLot)
	mux.HandleFunc("GET /api/lots/lookup/{kind}/{key}", a.apiLotLookup)
	mux.HandleFunc("GET /api/lots/{id}", a.apiLot)
	mux.HandleFunc("GET /api/me", a.apiMe)
	mux.HandleFunc("GET /api/me/balance", a.apiMeBalance)
	mux.HandleFunc("POST /api/me/topup/stars", a.apiTopupStars)
	mux.HandleFunc("POST /api/me/topup/direct", a.apiTopupDirect)
	mux.HandleFunc("POST /api/me/withdraw", a.apiWithdraw)
	mux.HandleFunc("POST /api/me/topup/ton", a.apiTopupTON)
	mux.HandleFunc("POST /api/internal/topup/settle", a.apiSettleTopup)
	mux.HandleFunc("GET /api/me/inventory", a.apiMeInventory)
	mux.HandleFunc("POST /api/me/language", a.apiMeLanguage)
	mux.HandleFunc("GET /api/me/channels", a.apiMyChannels)
	mux.HandleFunc("POST /api/auth/oidc", a.apiOIDC)
	mux.HandleFunc("POST /api/auth/request", a.apiAuthRequest)
	mux.HandleFunc("POST /api/auth/verify", a.apiAuthVerify)
	mux.HandleFunc("POST /api/auth/logout", a.apiLogout)
	mux.HandleFunc("POST /api/lots/{id}/bid", a.apiBid)
	mux.HandleFunc("POST /api/lots/{id}/settle", a.apiSettleLot)
	mux.HandleFunc("GET /api/admin/overview", a.apiAdminOverview)
	mux.HandleFunc("GET /api/admin/lots", a.apiAdminLots)
	mux.HandleFunc("GET /api/admin/sales", a.apiAdminSales)
	mux.HandleFunc("POST /api/admin/retry", a.apiAdminRetry)
	mux.HandleFunc("POST /api/admin/lots", a.apiAdminCreateLot)
	mux.HandleFunc("POST /api/admin/lots/{id}/settle", a.apiAdminSettleLot)
	mux.HandleFunc("POST /api/admin/lots/{id}/cancel", a.apiAdminCancelLot)
	mux.HandleFunc("POST /api/lots/{id}/watch", unsupportedAuth)
	mux.HandleFunc("POST /api/lots/{id}/cancel", unsupportedAuth)
	mux.HandleFunc("GET /assets/", a.asset)
	mux.HandleFunc("GET /logo-mark.png", a.logoMark)
	mux.HandleFunc("GET /favicon.png", a.favicon)
	mux.HandleFunc("GET /", a.home)
	addr := os.Getenv("DELTAMENT_ADDR")
	if addr == "" {
		addr = "127.0.0.1:2700"
	}
	s := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("deltament listening on %s", addr)
	log.Fatal(s.ListenAndServe())
}

func (a *app) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (a *app) apiConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"ok": true, "name": "Deltament", "layer": 232,
		"rate": 1.5, "bot": "deltagram_robot", "otpLength": 5,
		"maxAttempts": 4, "minUsernameStart": 10, "minNumberStart": 50,
		"nftThreshold": 100, "auctionHours": 24,
		"currencies":      []string{"TON", "XTR"},
		"tonStarsRate":    100,
		"primaryCurrency": "TON",
	})
}

func (a *app) apiLots(w http.ResponseWriter, r *http.Request) {
	a.settleExpiredLots(r.Context())
	limit := 50
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n < 200 {
		limit = n
	}
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	if kind == "" {
		kind = "username"
	}
	if kind != "username" && kind != "number" {
		writeJSONStatus(w, 400, map[string]any{"error": "INVALID_KIND"})
		return
	}
	dbKind := kind
	if dbKind == "number" {
		dbKind = "phone"
	}
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "sold" && status != "settled" && status != "cancelled" {
		writeJSONStatus(w, 400, map[string]any{"error": "INVALID_STATUS"})
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	sort := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort")))
	order := "a.created_at DESC"
	switch sort {
	case "ending":
		order = "a.ends_at ASC"
	case "price_desc":
		order = "COALESCE((SELECT MAX(b.amount) FROM deltament_auction_bids b WHERE b.auction_id=a.id AND b.active),a.minimum_bid) DESC"
	case "price_asc":
		order = "COALESCE((SELECT MAX(b.amount) FROM deltament_auction_bids b WHERE b.auction_id=a.id AND b.active),a.minimum_bid) ASC"
	}
	where := "a.kind=$1 AND a.status=$2"
	args := []any{dbKind, status}
	if status == "active" {
		where += " AND a.ends_at > now()"
	}
	if q != "" {
		where += " AND a.asset_name ILIKE $3"
		args = append(args, "%"+q+"%")
	}
	args = append(args, limit)
	rows, err := a.db.Query(r.Context(), fmt.Sprintf(`
		SELECT a.id, a.kind, CASE WHEN a.kind='phone' THEN COALESCE(cp.phone,a.asset_name) ELSE a.asset_name END, a.status, a.currency, a.minimum_bid,
	       COALESCE((SELECT MAX(b.amount) FROM deltament_auction_bids b
	                 WHERE b.auction_id = a.id AND b.active), a.minimum_bid),
	       COALESCE(u.username, ''), COALESCE(c.username, ''), a.ends_at
	FROM deltament_auctions a
	JOIN users u ON u.id = a.seller_user_id
	LEFT JOIN channels c ON c.id = a.channel_id
		LEFT JOIN collectible_phones cp ON cp.id = a.collectible_phone_id
WHERE %s
ORDER BY %s
LIMIT $%d`, where, order, len(args)), args...)
	if err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	defer rows.Close()
	out := make([]lot, 0, limit)
	for rows.Next() {
		var x lot
		if err := rows.Scan(&x.ID, &x.Kind, &x.Username, &x.Status, &x.Currency, &x.Amount,
			&x.CryptoAmount, &x.OwnerUsername, &x.URL, &x.EndsAt); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		if kind == "number" {
			x.Kind = "number"
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	items := make([]map[string]any, 0, len(out))
	for _, x := range out {
		_ = a.attachBidHistory(r.Context(), &x)
		items = append(items, a.marketLot(x))
	}
	writeJSON(w, map[string]any{"lots": items, "total": len(items)})
}

func (a *app) settleExpiredLots(ctx context.Context) {
	rows, err := a.db.Query(ctx, `SELECT id FROM deltament_auctions WHERE status='active' AND ends_at<=now() ORDER BY id LIMIT 20`)
	if err != nil {
		return
	}
	defer rows.Close()
	ids := make([]int64, 0, 20)
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		_, _ = a.settleLot(ctx, id, 0, false)
	}
}

func (a *app) requireDeltamentAdmin(w http.ResponseWriter, r *http.Request) (int64, bool) {
	uid, ok := a.sessionUser(r)
	if !ok {
		writeJSONStatus(w, 401, map[string]any{"error": "AUTH_REQUIRED"})
		return 0, false
	}
	if !a.isDeltamentAdmin(r.Context(), uid) {
		writeJSONStatus(w, 403, map[string]any{"error": "ADMIN_REQUIRED"})
		return 0, false
	}
	return uid, true
}

func (a *app) apiAdminOverview(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireDeltamentAdmin(w, r); !ok {
		return
	}
	var users, active, sold int64
	var volume int64
	_ = a.db.QueryRow(r.Context(), `SELECT count(*) FROM users WHERE deleted_at IS NULL`).Scan(&users)
	_ = a.db.QueryRow(r.Context(), `SELECT count(*) FROM deltament_auctions WHERE status='active' AND ends_at>now()`).Scan(&active)
	_ = a.db.QueryRow(r.Context(), `SELECT count(*) FROM deltament_auctions WHERE status IN ('sold','settled')`).Scan(&sold)
	_ = a.db.QueryRow(r.Context(), `SELECT COALESCE(sum(amount),0) FROM deltament_balance_transactions WHERE reason='auction sale'`).Scan(&volume)
	lots, _ := a.adminLots(r.Context(), r.URL.Query())
	if lots == nil {
		lots = []map[string]any{}
	}
	writeJSON(w, map[string]any{"stats": map[string]any{"users": users, "activeLots": active, "soldLots": sold, "soldVolume": volume}, "recentLots": lots})
}

func (a *app) adminLots(ctx context.Context, q url.Values) ([]map[string]any, error) {
	status := q.Get("status")
	if status == "" || status == "all" {
		status = ""
	}
	kind := q.Get("kind")
	if kind == "number" {
		kind = "phone"
	}
	if kind != "username" && kind != "phone" {
		kind = ""
	}
	where := "1=1"
	args := []any{}
	n := 1
	if status != "" {
		where += fmt.Sprintf(" AND a.status=$%d", n)
		args = append(args, status)
		n++
	}
	if kind != "" {
		where += fmt.Sprintf(" AND a.kind=$%d", n)
		args = append(args, kind)
		n++
	}
	args = append(args, 50)
	rows, err := a.db.Query(ctx, fmt.Sprintf(`SELECT a.id,a.kind,a.asset_name,a.status,a.currency,a.minimum_bid,COALESCE((SELECT max(b.amount) FROM deltament_auction_bids b WHERE b.auction_id=a.id AND b.active),a.minimum_bid),COALESCE(u.username,''),COALESCE(c.username,''),a.ends_at FROM deltament_auctions a JOIN users u ON u.id=a.seller_user_id LEFT JOIN channels c ON c.id=a.channel_id WHERE %s ORDER BY a.created_at DESC LIMIT $%d`, where, n), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var x lot
		if rows.Scan(&x.ID, &x.Kind, &x.Username, &x.Status, &x.Currency, &x.Amount, &x.CryptoAmount, &x.OwnerUsername, &x.URL, &x.EndsAt) == nil {
			_ = a.attachBidHistory(ctx, &x)
			out = append(out, a.marketLot(x))
		}
	}
	return out, rows.Err()
}

func (a *app) apiAdminLots(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireDeltamentAdmin(w, r); !ok {
		return
	}
	lots, err := a.adminLots(r.Context(), r.URL.Query())
	if err != nil {
		writeJSONStatus(w, 503, map[string]any{"error": "DATABASE_UNAVAILABLE"})
		return
	}
	writeJSON(w, map[string]any{"lots": lots, "total": len(lots)})
}
func (a *app) apiAdminSales(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireDeltamentAdmin(w, r); !ok {
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT t.user_id,t.amount,t.created_at,t.asset,t.reason FROM deltament_balance_transactions t WHERE t.reason='auction sale' ORDER BY t.id DESC LIMIT 100`)
	if err != nil {
		writeJSONStatus(w, 503, map[string]any{"error": "DATABASE_UNAVAILABLE"})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var uid, amount int64
		var ts time.Time
		var asset, reason string
		if rows.Scan(&uid, &amount, &ts, &asset, &reason) == nil {
			out = append(out, map[string]any{"user": uid, "amount": amount, "ts": ts, "note": asset, "status": "completed"})
		}
	}
	writeJSON(w, map[string]any{"sales": out})
}
func (a *app) apiAdminRetry(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireDeltamentAdmin(w, r); !ok {
		return
	}
	a.settleExpiredLots(r.Context())
	writeJSON(w, map[string]any{"ok": true})
}
func (a *app) apiAdminSettleLot(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireDeltamentAdmin(w, r); !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSONStatus(w, 400, map[string]any{"error": "LOT_NOT_FOUND"})
		return
	}
	v, err := a.settleLot(r.Context(), id, 0, true)
	if err != nil {
		writeJSONStatus(w, 409, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, v)
}
func (a *app) apiAdminCancelLot(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireDeltamentAdmin(w, r); !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSONStatus(w, 400, map[string]any{"error": "LOT_NOT_FOUND"})
		return
	}
	var n int64
	if err = a.db.QueryRow(r.Context(), `UPDATE deltament_auctions SET status='cancelled' WHERE id=$1 AND status='active' RETURNING id`, id).Scan(&n); err != nil {
		writeJSONStatus(w, 404, map[string]any{"error": "LOT_NOT_FOUND"})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "status": "cancelled", "id": n})
}
func (a *app) apiAdminCreateLot(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireDeltamentAdmin(w, r); !ok {
		return
	}
	var b struct {
		Kind        string          `json:"kind"`
		AssetKey    string          `json:"asset_key"`
		StartPrice  json.RawMessage `json:"start_price"`
		SellerPhone string          `json:"seller_phone"`
		Hours       int             `json:"hours"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&b) != nil {
		writeJSONStatus(w, 400, map[string]any{"error": "INVALID_REQUEST"})
		return
	}
	if b.Hours <= 0 || b.Hours > 168 {
		b.Hours = 24
	}
	b.Kind = strings.ToLower(strings.TrimSpace(b.Kind))
	b.AssetKey = strings.TrimPrefix(strings.TrimSpace(b.AssetKey), "@")
	b.SellerPhone = strings.TrimSpace(b.SellerPhone)
	if b.Kind != "username" && b.Kind != "number" {
		writeJSONStatus(w, 400, map[string]any{"error": "INVALID_KIND"})
		return
	}
	if b.AssetKey == "" || b.SellerPhone == "" {
		writeJSONStatus(w, 400, map[string]any{"error": "INVALID_LOT"})
		return
	}
	price, err := parseCurrencyAmount(b.StartPrice, "TON")
	if err != nil {
		writeJSONStatus(w, 400, map[string]any{"error": "INVALID_START_PRICE"})
		return
	}
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		writeJSONStatus(w, 503, map[string]any{"error": "DATABASE_UNAVAILABLE"})
		return
	}
	defer tx.Rollback(ctx)
	var seller int64
	if err = tx.QueryRow(ctx, `SELECT id FROM users WHERE phone=$1 AND deleted_at IS NULL`, b.SellerPhone).Scan(&seller); err != nil {
		writeJSONStatus(w, 404, map[string]any{"error": "SELLER_NOT_FOUND"})
		return
	}
	ends := time.Now().UTC().Add(time.Duration(b.Hours) * time.Hour)
	var id int64
	if b.Kind == "username" {
		var channel int64
		if err = tx.QueryRow(ctx, `SELECT id FROM channels WHERE creator_user_id=$1 AND deleted=false AND lower(username)=lower($2)`, seller, b.AssetKey).Scan(&channel); err != nil {
			writeJSONStatus(w, 404, map[string]any{"error": "USERNAME_NOT_FOUND"})
			return
		}
		if err = tx.QueryRow(ctx, `INSERT INTO deltament_auctions(kind,channel_id,seller_user_id,asset_name,currency,minimum_bid,ends_at) VALUES('username',$1,$2,$3,'TON',$4,$5) RETURNING id`, channel, seller, b.AssetKey, price, ends).Scan(&id); err != nil {
			writeJSONStatus(w, 409, map[string]any{"error": "AUCTION_CREATE_FAILED"})
			return
		}
	} else {
		var phoneID int64
		if err = tx.QueryRow(ctx, `SELECT id FROM collectible_phones WHERE phone=$1 AND status='owned' AND owner_user_id=$2`, b.AssetKey, seller).Scan(&phoneID); err != nil {
			writeJSONStatus(w, 404, map[string]any{"error": "NUMBER_NOT_FOUND"})
			return
		}
		if err = tx.QueryRow(ctx, `INSERT INTO deltament_auctions(kind,collectible_phone_id,seller_user_id,asset_name,currency,minimum_bid,ends_at) VALUES('phone',$1,$2,$3,'TON',$4,$5) RETURNING id`, phoneID, seller, b.AssetKey, price, ends).Scan(&id); err != nil {
			writeJSONStatus(w, 409, map[string]any{"error": "AUCTION_CREATE_FAILED"})
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		writeJSONStatus(w, 500, map[string]any{"error": "DATABASE_COMMIT_FAILED"})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "lot": map[string]any{"id": id, "kind": b.Kind, "assetKey": b.AssetKey, "name": b.AssetKey, "status": "active", "currentBidTon": float64(price) / float64(nanotonPerTON), "endsAt": ends.UnixMilli()}})
}

func (a *app) marketLot(x lot) map[string]any {
	ton := x.CryptoAmount
	stars := x.CryptoAmount
	if x.Currency == "TON" {
		ton = x.CryptoAmount
		stars = x.CryptoAmount / tonPerStar
	} else {
		stars = x.CryptoAmount
		ton = x.CryptoAmount * tonPerStar
	}
	currentTON := float64(ton) / float64(nanotonPerTON)
	minRaw := x.CryptoAmount + bidStep(x.Currency)
	minTON := float64(minRaw)
	if x.Currency == "XTR" {
		minTON = float64(minRaw*tonPerStar) / float64(nanotonPerTON)
	} else {
		minTON /= float64(nanotonPerTON)
	}
	return map[string]any{
		"id": x.ID, "kind": x.Kind, "name": x.Username,
		"assetKey": x.Username, "assetType": "collectible", "status": x.Status,
		"price": x.Amount, "startPrice": x.Amount, "currentBid": currentTON,
		"currentBidTon": currentTON, "currentBidStars": stars,
		"minNextBid": minTON, "minNextBidTon": minTON, "minNextBidStars": stars + 100,
		"endsAt": x.EndsAt.UnixMilli(), "bidCount": x.BidCount,
		"bids": x.Bids, "watching": false, "isSeller": false,
		"seller":   map[string]any{"display": "@" + x.OwnerUsername},
		"currency": x.Currency, "url": x.URL,
	}
}

const (
	nanotonPerTON = int64(1000000000)
	starsPerTON   = int64(100)
	tonPerStar    = nanotonPerTON / starsPerTON
)

func bidStep(currency string) int64 {
	if currency == "TON" {
		return nanotonPerTON
	}
	return 1
}

func maskBidder(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "user***"
	}
	prefix := ""
	if strings.HasPrefix(value, "@") {
		prefix, value = "@", strings.TrimPrefix(value, "@")
	}
	if len(value) <= 2 {
		return prefix + value[:1] + "***"
	}
	return prefix + value[:1] + "****" + value[len(value)-1:]
}

func (a *app) attachBidHistory(ctx context.Context, x *lot) error {
	rows, err := a.db.Query(ctx, `
		SELECT b.amount, b.created_at,
		       COALESCE(NULLIF(u.username,''), NULLIF(u.phone,''), 'user')
		FROM deltament_auction_bids b
		JOIN users u ON u.id=b.bidder_user_id
		WHERE b.auction_id=$1
		ORDER BY b.created_at DESC`, x.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	x.Bids = make([]map[string]any, 0)
	for rows.Next() {
		var amount int64
		var created time.Time
		var bidder string
		if err := rows.Scan(&amount, &created, &bidder); err != nil {
			return err
		}
		ton := amount
		stars := amount
		if x.Currency == "TON" {
			stars = amount / tonPerStar
		} else {
			ton = amount * tonPerStar
		}
		x.Bids = append(x.Bids, map[string]any{
			"amount":    float64(ton) / float64(nanotonPerTON),
			"ton":       float64(ton) / float64(nanotonPerTON),
			"stars":     stars,
			"currency":  "TON",
			"bidder":    maskBidder(bidder),
			"createdAt": created.UTC().Format(time.RFC3339),
			"active":    true,
		})
	}
	x.BidCount = len(x.Bids)
	return rows.Err()
}

func (a *app) apiLotLookup(w http.ResponseWriter, r *http.Request) {
	kind, key := r.PathValue("kind"), strings.TrimPrefix(r.PathValue("key"), "@")
	dbKind := kind
	if dbKind == "number" {
		dbKind = "phone"
	}
	if kind != "username" && kind != "number" {
		http.Error(w, `{"error":"LOT_NOT_FOUND"}`, http.StatusNotFound)
		return
	}
	var x lot
	err := a.db.QueryRow(r.Context(), `
	SELECT a.id, a.kind, CASE WHEN a.kind='phone' THEN COALESCE(cp.phone,a.asset_name) ELSE a.asset_name END, a.status, a.currency, a.minimum_bid,
	       COALESCE((SELECT MAX(b.amount) FROM deltament_auction_bids b
	                 WHERE b.auction_id = a.id AND b.active), a.minimum_bid),
	       COALESCE(u.username, ''), COALESCE(c.username, ''), a.ends_at
	FROM deltament_auctions a
	JOIN users u ON u.id = a.seller_user_id
	LEFT JOIN channels c ON c.id = a.channel_id
	LEFT JOIN collectible_phones cp ON cp.id = a.collectible_phone_id
WHERE lower(CASE WHEN a.kind='phone' THEN COALESCE(cp.phone,a.asset_name) ELSE a.asset_name END) = lower($1) AND a.kind = $2 AND a.status = 'active' AND a.ends_at > now()
ORDER BY a.id DESC LIMIT 1`, key, dbKind).Scan(&x.ID, &x.Kind, &x.Username, &x.Status, &x.Currency, &x.Amount,
		&x.CryptoAmount, &x.OwnerUsername, &x.URL, &x.EndsAt)
	if err != nil {
		http.Error(w, `{"error":"LOT_NOT_FOUND"}`, http.StatusNotFound)
		return
	}
	_ = a.attachBidHistory(r.Context(), &x)
	if kind == "number" {
		x.Kind = "number"
	}
	writeJSON(w, map[string]any{"lot": a.marketLot(x)})
}

func (a *app) apiLot(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, `{"error":"LOT_NOT_FOUND"}`, http.StatusNotFound)
		return
	}
	var x lot
	err = a.db.QueryRow(r.Context(), `
	SELECT a.id, a.kind, a.asset_name, a.status, a.currency, a.minimum_bid,
	       COALESCE((SELECT MAX(b.amount) FROM deltament_auction_bids b
	                 WHERE b.auction_id = a.id AND b.active), a.minimum_bid),
	       COALESCE(u.username, ''), COALESCE(c.username, ''), a.ends_at
	FROM deltament_auctions a
	JOIN users u ON u.id = a.seller_user_id
	LEFT JOIN channels c ON c.id = a.channel_id
WHERE a.id = $1 AND a.status = 'active' AND a.ends_at > now()`, id).Scan(&x.ID, &x.Kind, &x.Username, &x.Status, &x.Currency, &x.Amount,
		&x.CryptoAmount, &x.OwnerUsername, &x.URL, &x.EndsAt)
	if err != nil {
		http.Error(w, `{"error":"LOT_NOT_FOUND"}`, http.StatusNotFound)
		return
	}
	_ = a.attachBidHistory(r.Context(), &x)
	writeJSON(w, map[string]any{"lot": a.marketLot(x)})
}

// apiCreateLot moves a channel's editable username into the collectible
// registry and creates the auction in one database transaction. No payment is
// charged here; bidding/settlement must use a separate ledger transaction.
func (a *app) apiCreateLot(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.sessionUser(r)
	if !ok {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "AUTH_REQUIRED"})
		return
	}
	var body struct {
		Kind        string          `json:"kind"`
		AssetKey    string          `json:"asset_key"`
		ChannelID   int64           `json:"channel_id"`
		StartPrice  json.RawMessage `json:"start_price"`
		Currency    string          `json:"currency"`
		Hours       int             `json:"hours"`
		Description string          `json:"description"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body) != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "INVALID_REQUEST"})
		return
	}
	name := strings.TrimPrefix(strings.TrimSpace(body.AssetKey), "@")
	if body.Kind != "username" || name == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "INVALID_LOT"})
		return
	}
	body.Currency = strings.ToUpper(strings.TrimSpace(body.Currency))
	if body.Currency == "" {
		body.Currency = "XTR"
	}
	if body.Currency != "XTR" && body.Currency != "TON" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "INVALID_CURRENCY"})
		return
	}
	startPrice, err := parseCurrencyAmount(body.StartPrice, body.Currency)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "INVALID_START_PRICE"})
		return
	}
	if body.Hours <= 0 || body.Hours > 168 {
		body.Hours = 24
	}
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "DATABASE_UNAVAILABLE"})
		return
	}
	defer tx.Rollback(ctx)
	var channelID int64
	var title, currentName string
	if err = tx.QueryRow(ctx, `SELECT id,title,username FROM channels WHERE id=$1 AND creator_user_id=$2 AND deleted=false FOR UPDATE`, body.ChannelID, uid).Scan(&channelID, &title, &currentName); err != nil {
		// The compiled client historically sent only asset_key. Resolve it safely
		// against the authenticated user's own channels as a compatibility path.
		if err2 := tx.QueryRow(ctx, `SELECT id,title,username FROM channels WHERE creator_user_id=$1 AND deleted=false AND lower(username)=lower($2) FOR UPDATE`, uid, name).Scan(&channelID, &title, &currentName); err2 != nil {
			writeJSONStatus(w, http.StatusForbidden, map[string]any{"error": "CHANNEL_NOT_OWNED"})
			return
		}
	}
	if !strings.EqualFold(strings.TrimSpace(currentName), name) {
		writeJSONStatus(w, http.StatusConflict, map[string]any{"error": "USERNAME_NOT_ACTIVE"})
		return
	}
	endsAt := time.Now().UTC().Add(time.Duration(body.Hours) * time.Hour)
	var lotID int64
	if err = tx.QueryRow(ctx, `INSERT INTO deltament_auctions(kind,collectible_id,channel_id,seller_user_id,asset_name,currency,minimum_bid,ends_at) VALUES('username',NULL,$1,$2,$3,$4,$5,$6) RETURNING id`, channelID, uid, name, body.Currency, startPrice, endsAt).Scan(&lotID); err != nil {
		writeJSONStatus(w, 500, map[string]any{"error": "AUCTION_CREATE_FAILED"})
		return
	}
	if err = tx.Commit(ctx); err != nil {
		writeJSONStatus(w, 500, map[string]any{"error": "DATABASE_COMMIT_FAILED"})
		return
	}
	writeJSON(w, map[string]any{"lot": map[string]any{"id": lotID, "kind": "username", "name": name, "assetKey": name, "status": "active", "startPrice": startPrice, "currentBid": startPrice, "minNextBid": startPrice + 1, "endsAt": endsAt.UnixMilli(), "currency": body.Currency, "seller": map[string]any{"display": "@" + name}, "channelId": channelID, "title": title}})
}

func (a *app) apiBid(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.sessionUser(r)
	if !ok {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "AUTH_REQUIRED"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "LOT_NOT_FOUND"})
		return
	}
	var body struct {
		Amount json.RawMessage `json:"amount"`
		Bid    json.RawMessage `json:"bid"`
		Value  json.RawMessage `json:"value"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body) != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "INVALID_BID"})
		return
	}
	if len(bytes.TrimSpace(body.Amount)) == 0 {
		if len(bytes.TrimSpace(body.Bid)) > 0 {
			body.Amount = body.Bid
		} else {
			body.Amount = body.Value
		}
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeJSONStatus(w, 503, map[string]any{"error": "DATABASE_UNAVAILABLE"})
		return
	}
	defer tx.Rollback(r.Context())
	var status, currency, asset string
	var seller, minimum int64
	var ends time.Time
	if err = tx.QueryRow(r.Context(), `SELECT status,currency,asset_name,seller_user_id,minimum_bid,ends_at FROM deltament_auctions WHERE id=$1 FOR UPDATE`, id).Scan(&status, &currency, &asset, &seller, &minimum, &ends); err != nil || status != "active" || time.Now().After(ends) {
		writeJSONStatus(w, http.StatusConflict, map[string]any{"error": "AUCTION_CLOSED"})
		return
	}
	if (currency != "XTR" && currency != "TON") || uid == seller {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "BID_NOT_ALLOWED"})
		return
	}
	bidAmount, err := parseCurrencyAmount(body.Amount, currency)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "INVALID_BID", "message": "Сумма ставки должна быть положительным числом в валюте лота.", "currency": currency})
		return
	}
	var topBidder, topAmount int64
	_ = tx.QueryRow(r.Context(), `SELECT bidder_user_id,amount FROM deltament_auction_bids WHERE auction_id=$1 AND active ORDER BY amount DESC,created_at ASC LIMIT 1 FOR UPDATE`, id).Scan(&topBidder, &topAmount)
	if bidAmount < minimum || bidAmount <= topAmount {
		writeJSONStatus(w, http.StatusConflict, map[string]any{"error": "BID_TOO_LOW", "minimum": maxInt64(minimum, topAmount+1)})
		return
	}
	var ownAmount int64
	_ = tx.QueryRow(r.Context(), `SELECT amount FROM deltament_auction_bids WHERE auction_id=$1 AND bidder_user_id=$2 AND active FOR UPDATE`, id, uid).Scan(&ownAmount)
	delta := bidAmount - ownAmount
	if delta <= 0 {
		writeJSONStatus(w, http.StatusConflict, map[string]any{"error": "BID_TOO_LOW"})
		return
	}
	var debitErr error
	if currency == "TON" {
		debitErr = debitTONTx(r.Context(), tx, uid, delta, "auction bid", asset)
	} else {
		debitErr = debitDeltamentStarsTx(r.Context(), tx, uid, delta, "auction bid", asset)
	}
	if debitErr != nil {
		writeJSONStatus(w, http.StatusPaymentRequired, map[string]any{"error": "BALANCE_INSUFFICIENT", "currency": currency})
		return
	}
	if topBidder > 0 && topBidder != uid {
		if _, err = tx.Exec(r.Context(), `UPDATE deltament_auction_bids SET active=false WHERE auction_id=$1 AND bidder_user_id=$2`, id, topBidder); err != nil {
			writeJSONStatus(w, 500, map[string]any{"error": "BID_UPDATE_FAILED"})
			return
		}
		if currency == "TON" {
			err = creditTONTx(r.Context(), tx, topBidder, topAmount, "auction outbid refund", asset)
		} else {
			err = creditDeltamentStarsTx(r.Context(), tx, topBidder, topAmount, "auction outbid refund", asset)
		}
		if err != nil {
			writeJSONStatus(w, 500, map[string]any{"error": "REFUND_FAILED"})
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO deltament_auction_bids(auction_id,bidder_user_id,amount,active) VALUES($1,$2,$3,true) ON CONFLICT(auction_id,bidder_user_id) DO UPDATE SET amount=EXCLUDED.amount,active=true,created_at=now()`, id, uid, bidAmount); err != nil {
		writeJSONStatus(w, 500, map[string]any{"error": "BID_SAVE_FAILED"})
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeJSONStatus(w, 500, map[string]any{"error": "DATABASE_COMMIT_FAILED"})
		return
	}
	var balance int64
	_ = a.db.QueryRow(r.Context(), `SELECT COALESCE(stars_balance, 0) FROM deltament_balances WHERE user_id=$1`, uid).Scan(&balance)
	var tonBalance int64
	_ = a.db.QueryRow(r.Context(), `SELECT COALESCE(ton_balance_nanoton, 0) FROM deltament_balances WHERE user_id=$1`, uid).Scan(&tonBalance)
	if currency == "TON" {
		balance = tonBalance
	}
	currentTON := float64(bidAmount*tonPerStar) / float64(nanotonPerTON)
	currentStars := bidAmount
	if currency == "TON" {
		currentTON = float64(bidAmount) / float64(nanotonPerTON)
		currentStars = bidAmount / tonPerStar
	}
	writeJSON(w, map[string]any{
		"ok": true, "amount": currentTON, "amountRaw": bidAmount, "currentBid": currentTON,
		"currentBidTon": currentTON, "currentBidStars": currentStars,
		"minNextBid": currentTON + 1, "currency": "TON",
		"balance": float64(tonBalance) / float64(nanotonPerTON), "stars": balance, "ton": float64(tonBalance) / float64(nanotonPerTON), "tonNanoton": tonBalance, "tonUnit": "TON",
		"lot": map[string]any{"id": id, "currentBid": currentTON, "currentBidTon": currentTON, "currentBidStars": currentStars, "minNextBid": currentTON + 1,
			"myBid": map[string]any{"amount": bidAmount}},
	})
}

func (a *app) apiSettleLot(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.sessionUser(r)
	if !ok {
		writeJSONStatus(w, 401, map[string]any{"error": "AUTH_REQUIRED"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSONStatus(w, 400, map[string]any{"error": "LOT_NOT_FOUND"})
		return
	}
	result, err := a.settleLot(r.Context(), id, uid, false)
	if err != nil {
		writeJSONStatus(w, http.StatusConflict, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, result)
}

func (a *app) settleLot(ctx context.Context, id, actor int64, force bool) (map[string]any, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var status, asset, currency string
	var seller, channel int64
	var collectible *int64
	var ends time.Time
	if err = tx.QueryRow(ctx, `SELECT status,asset_name,currency,seller_user_id,collectible_id,channel_id,ends_at FROM deltament_auctions WHERE id=$1 FOR UPDATE`, id).Scan(&status, &asset, &currency, &seller, &collectible, &channel, &ends); err != nil {
		return nil, fmt.Errorf("LOT_NOT_FOUND")
	}
	if status == "sold" || status == "settled" || status == "cancelled" {
		return map[string]any{"ok": true, "status": status}, nil
	}
	if !force && time.Now().Before(ends) {
		return nil, fmt.Errorf("AUCTION_NOT_ENDED")
	}
	var buyer, amount int64
	err = tx.QueryRow(ctx, `SELECT bidder_user_id,amount FROM deltament_auction_bids WHERE auction_id=$1 AND active ORDER BY amount DESC,created_at ASC LIMIT 1 FOR UPDATE`, id).Scan(&buyer, &amount)
	if err != nil {
		if _, err = tx.Exec(ctx, `UPDATE deltament_auctions SET status='cancelled' WHERE id=$1`, id); err != nil {
			return nil, err
		}
		if err = tx.Commit(ctx); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "status": "cancelled"}, nil
	}
	if actor != 0 && actor != seller && actor != buyer {
		return nil, fmt.Errorf("NOT_AUTHORIZED")
	}
	if channel == 0 {
		return nil, fmt.Errorf("CHANNEL_NOT_FOUND")
	}
	if _, err = tx.Exec(ctx, `UPDATE channels SET username=NULL, updated_at=now() WHERE id=$1 AND creator_user_id=$2 AND deleted=false`, channel, seller); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM peer_usernames WHERE peer_type='channel' AND peer_id=$1 AND lower(username)=lower($2)`, channel, asset); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var collectibleID int64
	if err = tx.QueryRow(ctx, `INSERT INTO collectible_usernames (username,username_lower,status,owner_peer_type,owner_peer_id,purchase_date,currency,amount,crypto_currency,crypto_amount,url,original_owner_peer_type,original_owner_peer_id,created_at,updated_at) VALUES($1,lower($1),'owned','user',$2,$3,$4,$5,'',0,$6,'user',$7,$3,$3) RETURNING id`, asset, buyer, now, currency, amount, "", seller).Scan(&collectibleID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO peer_usernames(username_lower,peer_type,peer_id,username,active,editable,sort_order,collectible_id,updated_at) VALUES(lower($1),'user',$2,$1,true,false,0,$3,now())`, asset, buyer, collectibleID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO collectible_username_transfers(collectible_id,kind,from_peer_type,from_peer_id,to_peer_type,to_peer_id,currency,amount,actor,reason,created_at) VALUES($1,'mint','channel',$2,'user',$3,$4,$5,'deltament','auction settlement',now())`, collectibleID, seller, buyer, currency, amount); err != nil {
		return nil, err
	}
	commission := amount / 20
	sellerAmount := amount - commission
	if currency == "TON" {
		err = creditTONTx(ctx, tx, seller, sellerAmount, "auction sale", asset)
	} else {
		err = creditDeltamentStarsTx(ctx, tx, seller, sellerAmount, "auction sale", asset)
	}
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE deltament_auction_bids SET active=false WHERE auction_id=$1`, id); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE deltament_auctions SET status='settled', collectible_id=$2, settled_at=now(), settlement_key=$3 WHERE id=$1`, id, collectibleID, fmt.Sprintf("auction:%d", id)); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "status": "sold", "buyerId": buyer, "amount": amount, "sellerAmount": sellerAmount, "assetKey": asset}, nil
}

func debitDeltamentStarsTx(ctx context.Context, tx pgx.Tx, userID, amount int64, title, asset string) error {
	if amount <= 0 {
		return fmt.Errorf("invalid amount")
	}
	var balance int64
	if err := tx.QueryRow(ctx, `SELECT stars_balance FROM deltament_balances WHERE user_id=$1 FOR UPDATE`, userID).Scan(&balance); err != nil || balance < amount {
		return fmt.Errorf("insufficient stars")
	}
	if _, err := tx.Exec(ctx, `UPDATE deltament_balances SET stars_balance=stars_balance-$2,updated_at=now() WHERE user_id=$1`, userID, amount); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO deltament_balance_transactions(user_id,currency,amount,reason,asset) VALUES($1,'XTR',$2,'auction bid',$3)`, userID, -amount, asset)
	return err
}

func creditDeltamentStarsTx(ctx context.Context, tx pgx.Tx, userID, amount int64, reason, asset string) error {
	if amount <= 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO deltament_balances(user_id,stars_balance,updated_at) VALUES($1,$2,now()) ON CONFLICT(user_id) DO UPDATE SET stars_balance=deltament_balances.stars_balance+EXCLUDED.stars_balance,updated_at=now()`, userID, amount); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO deltament_balance_transactions(user_id,currency,amount,reason,asset) VALUES($1,'XTR',$2,$3,$4)`, userID, amount, reason, asset)
	return err
}

func debitTONTx(ctx context.Context, tx pgx.Tx, userID, amount int64, reason, asset string) error {
	if amount <= 0 {
		return fmt.Errorf("invalid amount")
	}
	var balance int64
	if err := tx.QueryRow(ctx, `SELECT ton_balance_nanoton FROM deltament_balances WHERE user_id=$1 FOR UPDATE`, userID).Scan(&balance); err != nil || balance < amount {
		return fmt.Errorf("insufficient ton")
	}
	if _, err := tx.Exec(ctx, `UPDATE deltament_balances SET ton_balance_nanoton=ton_balance_nanoton-$2,updated_at=now() WHERE user_id=$1`, userID, amount); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO deltament_balance_transactions(user_id,currency,amount,reason,asset) VALUES($1,'TON',$2,$3,$4)`, userID, -amount, reason, asset)
	return err
}

func creditTONTx(ctx context.Context, tx pgx.Tx, userID, amount int64, reason, asset string) error {
	if amount <= 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO deltament_balances(user_id,ton_balance_nanoton,updated_at) VALUES($1,$2,now()) ON CONFLICT(user_id) DO UPDATE SET ton_balance_nanoton=deltament_balances.ton_balance_nanoton+EXCLUDED.ton_balance_nanoton,updated_at=now()`, userID, amount); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO deltament_balance_transactions(user_id,currency,amount,reason,asset) VALUES($1,'TON',$2,$3,$4)`, userID, amount, reason, asset)
	return err
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func parseCurrencyAmount(raw json.RawMessage, currency string) (int64, error) {
	s := strings.TrimSpace(string(raw))
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return 0, err
		}
		s = strings.TrimSpace(decoded)
	}
	if s == "" || strings.ContainsAny(s, "eE") {
		return 0, fmt.Errorf("invalid amount")
	}
	if currency == "XTR" {
		value, err := strconv.ParseInt(s, 10, 64)
		if err != nil || value <= 0 {
			return 0, fmt.Errorf("invalid stars amount")
		}
		return value, nil
	}
	// Human TON is the public API unit. Keep accepting legacy numeric nanotons
	// from older clients when the value is unambiguously large.
	if len(raw) > 0 && strings.TrimSpace(string(raw))[0] != '"' {
		value, err := strconv.ParseInt(s, 10, 64)
		if err != nil || value <= 0 {
			return 0, fmt.Errorf("invalid TON amount")
		}
		if value >= nanotonPerTON {
			return value, nil
		}
		s = strconv.FormatInt(value, 10)
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 || parts[0] == "" || strings.Trim(parts[0], "0123456789") != "" {
		return 0, fmt.Errorf("invalid TON amount")
	}
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
		if frac == "" || len(frac) > 9 || strings.Trim(frac, "0123456789") != "" {
			return 0, fmt.Errorf("invalid TON amount")
		}
	}
	for len(frac) < 9 {
		frac += "0"
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 || whole > (1<<63-1)/1000000000 {
		return 0, fmt.Errorf("invalid TON amount")
	}
	minor := whole*1000000000 + parseDigits(frac)
	if minor <= 0 {
		return 0, fmt.Errorf("invalid TON amount")
	}
	return minor, nil
}

func parseDigits(value string) int64 {
	var result int64
	for _, ch := range value {
		result = result*10 + int64(ch-'0')
	}
	return result
}

func (a *app) apiMe(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.sessionUser(r)
	if !ok {
		writeJSON(w, map[string]any{"user": nil})
		return
	}
	var id int64
	var phone, username, first, last string
	language := "ru"
	if err := a.db.QueryRow(r.Context(), `SELECT u.id, u.phone, u.username, u.first_name, u.last_name,
		COALESCE(s.language, 'ru')
		FROM users u LEFT JOIN deltament_user_settings s ON s.user_id = u.id
		WHERE u.id=$1 AND u.deleted_at IS NULL`, uid).Scan(&id, &phone, &username, &first, &last, &language); err != nil {
		a.clearSession(w)
		writeJSON(w, map[string]any{"user": nil})
		return
	}
	_ = a.ensureDeltamentWallet(r.Context(), id)
	var balance int64
	_ = a.db.QueryRow(r.Context(), `SELECT COALESCE(stars_balance, 0) FROM deltament_balances WHERE user_id=$1`, id).Scan(&balance)
	var tonBalance int64
	_ = a.db.QueryRow(r.Context(), `SELECT COALESCE(ton_balance_nanoton, 0) FROM deltament_balances WHERE user_id=$1`, id).Scan(&tonBalance)

	inventory := make([]map[string]any, 0)
	rows, err := a.db.Query(r.Context(), `
		SELECT c.id, c.title, c.username,
		       COALESCE((SELECT a.id FROM deltament_auctions a
		                 WHERE a.channel_id=c.id AND a.status='active'
		                 ORDER BY a.id DESC LIMIT 1), 0)
		FROM channels c
		WHERE c.creator_user_id=$1 AND c.deleted=false AND c.username IS NOT NULL AND c.username<>''
		ORDER BY c.id DESC`, id)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var channelID, lockedLotID int64
			var title, name string
			if rows.Scan(&channelID, &title, &name, &lockedLotID) == nil {
				item := map[string]any{"id": channelID, "kind": "username", "name": name,
					"username": name, "assetKey": name, "title": title, "source": "Ваш канал"}
				if lockedLotID > 0 {
					item["lockedLotId"] = lockedLotID
				}
				inventory = append(inventory, item)
			}
		}
	}
	writeJSON(w, map[string]any{
		"user": map[string]any{"id": id, "phone": phone, "username": username,
			"display": strings.TrimSpace(first + " " + last), "language": language, "isAdmin": a.isDeltamentAdmin(r.Context(), id)},
		"balance": balance, "stars": balance, "ton": tonBalance, "tonUnit": "nanoton",
		"inventory": inventory, "history": []any{}, "bids": map[string]any{},
		"watching": []any{}, "sessions": []any{}, "currentSessionId": "",
	})
}

func (a *app) apiMeLanguage(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.sessionUser(r)
	if !ok {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "Требуется вход в аккаунт."})
		return
	}
	var body struct {
		Language string `json:"language"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "Некорректный запрос."})
		return
	}
	language := strings.ToLower(strings.TrimSpace(body.Language))
	if language != "ru" && language != "en" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "Поддерживаются только русский и английский языки."})
		return
	}
	if _, err := a.db.Exec(r.Context(), `INSERT INTO deltament_user_settings(user_id, language, updated_at)
		VALUES($1, $2, now()) ON CONFLICT(user_id) DO UPDATE SET language=EXCLUDED.language, updated_at=now()`, uid, language); err != nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "Не удалось сохранить язык интерфейса."})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "language": language})
}

func (a *app) apiMeBalance(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.sessionUser(r)
	if !ok {
		writeJSONStatus(w, 401, map[string]any{"error": "AUTH_REQUIRED"})
		return
	}
	_ = a.ensureDeltamentWallet(r.Context(), uid)
	var stars int64
	if err := a.db.QueryRow(r.Context(), `SELECT COALESCE(stars_balance,0) FROM deltament_balances WHERE user_id=$1`, uid).Scan(&stars); err != nil {
		stars = 0
	}
	var ton int64
	if err := a.db.QueryRow(r.Context(), `SELECT COALESCE(ton_balance_nanoton,0) FROM deltament_balances WHERE user_id=$1`, uid).Scan(&ton); err != nil {
		ton = 0
	}
	writeJSON(w, map[string]any{"stars": stars, "ton": float64(ton) / float64(nanotonPerTON), "tonNanoton": ton, "tonUnit": "TON", "currency": "TON", "rate": map[string]any{"ton": 1, "stars": 100}})
}

func (a *app) ensureDeltamentWallet(ctx context.Context, uid int64) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `INSERT INTO deltament_wallet_grants(user_id) VALUES($1) ON CONFLICT(user_id) DO NOTHING`, uid)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 1 {
		if _, err = tx.Exec(ctx, `INSERT INTO deltament_balances(user_id,ton_balance_nanoton,updated_at) VALUES($1,$2,now()) ON CONFLICT(user_id) DO UPDATE SET ton_balance_nanoton=deltament_balances.ton_balance_nanoton+$2,updated_at=now()`, uid, 10*nanotonPerTON); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO deltament_balance_transactions(user_id,currency,amount,reason,asset) VALUES($1,'TON',$2,'initial grant','DeltaGram')`, uid, 10*nanotonPerTON); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (a *app) apiTopupDirect(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.sessionUser(r)
	if !ok {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "AUTH_REQUIRED"})
		return
	}
	var body struct {
		Currency  string          `json:"currency"`
		Amount    json.RawMessage `json:"amount"`
		RequestID string          `json:"request_id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "INVALID_AMOUNT"})
		return
	}
	body.Currency = strings.ToUpper(strings.TrimSpace(body.Currency))
	if body.Currency == "" {
		body.Currency = "XTR"
	}
	if body.Currency != "XTR" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "INVALID_CURRENCY"})
		return
	}
	amount, err := parseCurrencyAmount(body.Amount, body.Currency)
	if err != nil || amount > 1000000 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "INVALID_AMOUNT"})
		return
	}
	requestID := strings.TrimSpace(body.RequestID)
	if requestID == "" {
		requestID = uuid.NewString()
	}
	if len(requestID) > 128 || strings.ContainsAny(requestID, "\r\n") {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "INVALID_REQUEST_ID"})
		return
	}
	key := fmt.Sprintf("deltament-topup:%d:%s:%s", uid, body.Currency, requestID)
	// The grammystore bridge performs the real Stars debit through the Admin API.
	// Its command id is derived from this stable key, so retrying after a local
	// database failure is safe and does not charge the account twice.
	result, status, err := a.auth.call("/v1/deltament/topup/direct", map[string]any{
		"server_user_id": uid, "amount": amount, "currency": "XTR", "idempotency_key": key,
	})
	if err != nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "TOPUP_BRIDGE_UNAVAILABLE"})
		return
	}
	if status >= 400 {
		writeJSONStatus(w, status, result)
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "DATABASE_UNAVAILABLE"})
		return
	}
	defer tx.Rollback(r.Context())
	inserted, err := tx.Exec(r.Context(), `INSERT INTO deltament_topup_transfers(idempotency_key,user_id,currency,amount) VALUES($1,$2,'XTR',$3) ON CONFLICT(idempotency_key) DO NOTHING`, key, uid, amount)
	if err != nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "TOPUP_LEDGER_FAILED"})
		return
	}
	if inserted.RowsAffected() == 0 {
		var existingCurrency string
		var existingAmount int64
		if err = tx.QueryRow(r.Context(), `SELECT currency,amount FROM deltament_topup_transfers WHERE idempotency_key=$1`, key).Scan(&existingCurrency, &existingAmount); err != nil || existingCurrency != "XTR" || existingAmount != amount {
			writeJSONStatus(w, http.StatusConflict, map[string]any{"error": "REQUEST_ID_REUSED"})
			return
		}
		_ = tx.Commit(r.Context())
		writeJSON(w, map[string]any{"ok": true, "duplicate": true, "currency": "XTR", "amount": amount})
		return
	}
	tonAmount := amount * tonPerStar
	if _, err = tx.Exec(r.Context(), `INSERT INTO deltament_balances(user_id,ton_balance_nanoton,updated_at) VALUES($1,$2,now()) ON CONFLICT(user_id) DO UPDATE SET ton_balance_nanoton=deltament_balances.ton_balance_nanoton+$2,updated_at=now()`, uid, tonAmount); err != nil {
		writeJSONStatus(w, 500, map[string]any{"error": "BALANCE_CREDIT_FAILED"})
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO deltament_balance_transactions(user_id,currency,amount,reason,asset) VALUES($1,'TON',$2,'topup','DeltaGram Stars')`, uid, tonAmount); err != nil {
		writeJSONStatus(w, 500, map[string]any{"error": "BALANCE_LEDGER_FAILED"})
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeJSONStatus(w, 500, map[string]any{"error": "DATABASE_COMMIT_FAILED"})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "currency": "TON", "inputCurrency": "XTR", "inputAmount": amount, "amount": tonAmount, "amountUnit": "nanoton", "ton": float64(amount) / float64(starsPerTON), "stars": amount})
}

func (a *app) apiTopupStars(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.sessionUser(r)
	if !ok {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "AUTH_REQUIRED"})
		return
	}
	var body struct {
		Amount int64 `json:"amount"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil || body.Amount < 1 || body.Amount > 1000000 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "INVALID_AMOUNT"})
		return
	}
	intentID := uuid.New()
	if _, err := a.db.Exec(r.Context(), `INSERT INTO deltament_topup_intents(id,user_id,currency,amount,expires_at) VALUES($1,$2,'XTR',$3,now()+interval '15 minutes')`, intentID, uid, body.Amount); err != nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "TOPUP_INTENT_CREATE_FAILED"})
		return
	}
	result, status, err := a.auth.call("/v1/deltament/topup/invoice", map[string]any{"intent_id": intentID.String(), "server_user_id": uid, "amount": body.Amount})
	if err != nil || status >= 400 {
		_, _ = a.db.Exec(r.Context(), `UPDATE deltament_topup_intents SET status='failed',updated_at=now() WHERE id=$1 AND status='pending'`, intentID)
		if err != nil {
			writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "TOPUP_BRIDGE_UNAVAILABLE"})
		} else {
			writeJSONStatus(w, status, result)
		}
		return
	}
	writeJSON(w, map[string]any{"ok": true, "intent_id": intentID.String(), "status": "pending", "message": "Invoice created. Confirm the official Telegram Stars payment."})
}

func (a *app) apiWithdraw(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.sessionUser(r)
	if !ok {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "AUTH_REQUIRED"})
		return
	}
	var body struct {
		Amount    json.RawMessage `json:"amount"`
		RequestID string          `json:"request_id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "INVALID_AMOUNT"})
		return
	}
	tonAmount, err := parseCurrencyAmount(body.Amount, "TON")
	if err != nil || tonAmount < nanotonPerTON/100 || tonAmount > 1000000*tonPerStar {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "INVALID_AMOUNT"})
		return
	}
	if tonAmount%tonPerStar != 0 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "AMOUNT_MUST_CONVERT_TO_WHOLE_STARS"})
		return
	}
	requestID := strings.TrimSpace(body.RequestID)
	if requestID == "" {
		requestID = uuid.NewString()
	}
	if len(requestID) > 128 || strings.ContainsAny(requestID, "\r\n") {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "INVALID_REQUEST_ID"})
		return
	}
	key := fmt.Sprintf("deltament-withdraw:%d:%s", uid, requestID)
	starsAmount := tonAmount / tonPerStar

	var current int64
	if err := a.db.QueryRow(r.Context(), `SELECT COALESCE(ton_balance_nanoton,0) FROM deltament_balances WHERE user_id=$1`, uid).Scan(&current); err != nil || current < tonAmount {
		writeJSONStatus(w, http.StatusPaymentRequired, map[string]any{"error": "BALANCE_INSUFFICIENT", "currency": "TON"})
		return
	}
	result, status, err := a.auth.call("/v1/deltament/withdraw/direct", map[string]any{
		"server_user_id": uid, "stars_amount": starsAmount, "idempotency_key": key,
	})
	if err != nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "WITHDRAW_BRIDGE_UNAVAILABLE"})
		return
	}
	if status >= 400 {
		writeJSONStatus(w, status, result)
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "DATABASE_UNAVAILABLE"})
		return
	}
	defer tx.Rollback(r.Context())
	inserted, err := tx.Exec(r.Context(), `INSERT INTO deltament_withdrawals(idempotency_key,user_id,ton_amount_nanoton,stars_amount) VALUES($1,$2,$3,$4) ON CONFLICT(idempotency_key) DO NOTHING`, key, uid, tonAmount, starsAmount)
	if err != nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "WITHDRAWAL_LEDGER_FAILED"})
		return
	}
	if inserted.RowsAffected() == 0 {
		_ = tx.Commit(r.Context())
		writeJSON(w, map[string]any{"ok": true, "duplicate": true, "currency": "XTR", "ton": float64(tonAmount) / float64(nanotonPerTON), "stars": starsAmount})
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE deltament_balances SET ton_balance_nanoton=ton_balance_nanoton-$2,updated_at=now() WHERE user_id=$1 AND ton_balance_nanoton >= $2`, uid, tonAmount); err != nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "BALANCE_DEBIT_FAILED"})
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO deltament_balance_transactions(user_id,currency,amount,reason,asset) VALUES($1,'TON',$2,'withdrawal','Telegram Stars')`, uid, -tonAmount); err != nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "BALANCE_LEDGER_FAILED"})
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "DATABASE_COMMIT_FAILED"})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "currency": "XTR", "ton": float64(tonAmount) / float64(nanotonPerTON), "stars": starsAmount})
}

// TON requires an external wallet/payment provider and an on-chain
// confirmation. This endpoint intentionally refuses to create a fake balance
// or pretend that a Telegram Stars invoice paid TON.
func (a *app) apiTopupTON(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.sessionUser(r); !ok {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "AUTH_REQUIRED"})
		return
	}
	writeJSONStatus(w, http.StatusNotImplemented, map[string]any{"error": "TON_PROVIDER_NOT_CONFIGURED", "message": "TON top-up requires a configured wallet provider and on-chain confirmation."})
}

func (a *app) apiSettleTopup(w http.ResponseWriter, r *http.Request) {
	if !a.auth.authorized(r) {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "UNAUTHORIZED"})
		return
	}
	var body struct {
		IntentID, ChargeID, Currency    string
		ServerUserID, Amount, MessageID int64
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body) != nil || body.IntentID == "" || body.ChargeID == "" || body.ServerUserID <= 0 || body.Amount <= 0 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "INVALID_TOPUP_SETTLEMENT"})
		return
	}
	intentID, err := uuid.Parse(body.IntentID)
	if err != nil {
		writeJSONStatus(w, 400, map[string]any{"error": "INVALID_INTENT"})
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		writeJSONStatus(w, 503, map[string]any{"error": "DATABASE_UNAVAILABLE"})
		return
	}
	defer tx.Rollback(r.Context())
	var userID, amount int64
	var status string
	err = tx.QueryRow(r.Context(), `SELECT user_id,amount,status FROM deltament_topup_intents WHERE id=$1 FOR UPDATE`, intentID).Scan(&userID, &amount, &status)
	if err != nil || userID != body.ServerUserID || amount != body.Amount {
		writeJSONStatus(w, 404, map[string]any{"error": "TOPUP_INTENT_NOT_FOUND"})
		return
	}
	if status == "paid" {
		_ = tx.Commit(r.Context())
		writeJSON(w, map[string]any{"ok": true, "duplicate": true, "status": "paid"})
		return
	}
	if status != "pending" {
		writeJSONStatus(w, 409, map[string]any{"error": "TOPUP_INTENT_CLOSED"})
		return
	}
	var intentCurrency string
	if err = tx.QueryRow(r.Context(), `SELECT currency FROM deltament_topup_intents WHERE id=$1`, intentID).Scan(&intentCurrency); err != nil || intentCurrency != body.Currency {
		writeJSONStatus(w, 409, map[string]any{"error": "TOPUP_CURRENCY_MISMATCH"})
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE deltament_topup_intents SET status='paid',provider_charge_id=$2,provider_message_id=$3,settled_at=now(),updated_at=now() WHERE id=$1`, intentID, body.ChargeID, body.MessageID); err != nil {
		writeJSONStatus(w, 500, map[string]any{"error": "TOPUP_SETTLEMENT_FAILED"})
		return
	}
	if body.Currency == "TON" {
		if _, err = tx.Exec(r.Context(), `INSERT INTO ton_balances(user_id,balance_nanoton,granted,updated_at) VALUES($1,$2,true,now()) ON CONFLICT(user_id) DO UPDATE SET balance_nanoton=ton_balances.balance_nanoton+EXCLUDED.balance_nanoton,updated_at=now()`, userID, amount); err != nil {
			writeJSONStatus(w, 500, map[string]any{"error": "TOPUP_CREDIT_FAILED"})
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO ton_transactions(user_id,amount_nanoton,reason,date) VALUES($1,$2,'official TON top-up',$3)`, userID, amount, time.Now().Unix()); err != nil {
			writeJSONStatus(w, 500, map[string]any{"error": "TOPUP_LEDGER_FAILED"})
			return
		}
	} else if err = creditDeltamentStarsTx(r.Context(), tx, userID, amount, "official Stars top-up", "Deltament"); err != nil {
		writeJSONStatus(w, 500, map[string]any{"error": "TOPUP_CREDIT_FAILED"})
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeJSONStatus(w, 500, map[string]any{"error": "DATABASE_COMMIT_FAILED"})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "status": "paid"})
}

func (a *app) apiMeInventory(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.sessionUser(r)
	if !ok {
		writeJSONStatus(w, 401, map[string]any{"error": "AUTH_REQUIRED"})
		return
	}
	channels := make([]map[string]any, 0)
	channelRows, channelErr := a.db.Query(r.Context(), `
		SELECT c.id,c.title,c.username,
		       EXISTS(SELECT 1 FROM deltament_auctions a WHERE a.channel_id=c.id AND a.status='active' AND a.ends_at>now())
		FROM channels c
		WHERE c.creator_user_id=$1 AND c.deleted=false AND c.username IS NOT NULL AND c.username<>''
		ORDER BY c.id DESC`, uid)
	if channelErr == nil {
		defer channelRows.Close()
		for channelRows.Next() {
			var id int64
			var title, name string
			var active bool
			if channelRows.Scan(&id, &title, &name, &active) == nil {
				channels = append(channels, map[string]any{"id": id, "title": title, "username": name, "auctionable": !active, "activeAuction": active})
			}
		}
	}
	items := make([]map[string]any, 0)
	rows, err := a.db.Query(r.Context(), `SELECT id,username,username_lower,status,amount,currency,url FROM collectible_usernames WHERE status='owned' AND owner_peer_type='user' AND owner_peer_id=$1 ORDER BY id DESC`, uid)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, amount int64
			var name, lower, status, currency, url string
			if rows.Scan(&id, &name, &lower, &status, &amount, &currency, &url) == nil {
				items = append(items, map[string]any{"id": id, "kind": "username", "username": name, "status": status, "amount": amount, "currency": currency, "url": url})
			}
		}
	}
	gifts := make([]map[string]any, 0)
	giftRows, err := a.db.Query(r.Context(), `SELECT p.id,p.gift_id,COALESCE(p.unique_gift_id,0),p.gift_date FROM peer_star_gifts p WHERE p.owner_peer_type='user' AND p.owner_peer_id=$1 ORDER BY p.id DESC LIMIT 200`, uid)
	if err == nil {
		defer giftRows.Close()
		for giftRows.Next() {
			var id, giftID, uniqueID int64
			var date int
			if giftRows.Scan(&id, &giftID, &uniqueID, &date) == nil {
				gifts = append(gifts, map[string]any{"id": id, "kind": "gift", "giftId": giftID, "uniqueGiftId": uniqueID, "date": date})
			}
		}
	}
	writeJSON(w, map[string]any{"channels": channels, "usernames": items, "gifts": gifts})
}

func (a *app) apiMyChannels(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.sessionUser(r)
	if !ok {
		http.Error(w, "{\"error\":\"AUTH_REQUIRED\"}", http.StatusUnauthorized)
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT c.id,c.title,c.username,
		       EXISTS(SELECT 1 FROM deltament_auctions a WHERE a.channel_id=c.id AND a.status='active' AND a.ends_at>now())
		FROM channels c
		WHERE c.creator_user_id=$1 AND c.deleted=false AND c.username IS NOT NULL AND c.username<>''
		ORDER BY c.id DESC`, uid)
	if err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var title, name string
		var activeAuction bool
		if rows.Scan(&id, &title, &name, &activeAuction) == nil {
			out = append(out, map[string]any{"id": id, "title": title, "username": name, "auctionable": !activeAuction, "activeAuction": activeAuction})
		}
	}
	writeJSON(w, map[string]any{"channels": out})
}

func (a *app) apiLogout(w http.ResponseWriter, r *http.Request) {
	a.clearSession(w)
	writeJSON(w, map[string]any{"ok": true})
}

func unsupportedAuth(w http.ResponseWriter, r *http.Request) {
	http.Error(w, `{"error":"OIDC_LOGIN_REQUIRED","message":"Use the Deltagram login flow"}`, http.StatusGone)
}

func unsupportedPayment(w http.ResponseWriter, r *http.Request) {
	http.Error(w, `{"error":"PAYMENT_BRIDGE_NOT_CONFIGURED","message":"No Stars or TON payment was charged"}`, http.StatusNotImplemented)
}

func (a *app) asset(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || name == "assets" || !strings.HasPrefix(name, "assets/") || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	b, err := staticFiles.ReadFile("static/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(name, ".js") {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	}
	if strings.HasSuffix(name, ".css") {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	}
	if strings.HasSuffix(name, ".mp3") {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	_, _ = w.Write(b)
}

func (a *app) logoMark(w http.ResponseWriter, r *http.Request) {
	b, err := staticFiles.ReadFile("static/favicon.png")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(b)
}

func (a *app) favicon(w http.ResponseWriter, r *http.Request) {
	b, err := staticFiles.ReadFile("static/favicon.png")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(b)
}

func (a *app) home(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	b, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "frontend unavailable", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(b)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

const indexHTML = `<!doctype html>
<html lang="uk">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Deltament — DeltaGram</title>
<style>
*{box-sizing:border-box}
body{font:15px/1.5 system-ui;background:#0e1116;color:#eef2f7;max-width:960px;margin:0 auto;padding:24px 16px}
header{display:flex;align-items:center;gap:14px;margin-bottom:32px}
header h1{font-size:2.2rem;margin:0}
header small{color:#9ca9ba}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(260px,1fr));gap:16px}
.card{background:#171c24;border:1px solid #293241;border-radius:16px;padding:18px;position:relative}
.card .name{font-size:1.1rem;font-weight:700}
.card .price{margin-top:8px;color:#77e0b5;font-weight:600}
.card .meta{margin-top:6px;font-size:0.82rem;color:#9ca9ba}
.status-owned{color:#6ea8fe} .status-vault{color:#9ca9ba}
footer{margin-top:48px;font-size:0.8rem;color:#4a5568;text-align:center}
</style></head>
<body>
<header>
  <div>
    <h1>Deltament</h1>
    <small>Колекційні @юзернейми та цифрові активи DeltaGram</small>
  </div>
</header>
<div id="lots" class="grid">Завантаження…</div>
<footer>DeltaGram &bull; layer 232</footer>
<script>
async function load(){
  const el=document.getElementById('lots');
  try{
    const xs=await fetch('/api/lots').then(r=>r.json());
    if(!xs.length){el.innerHTML='<p>Лотів поки немає.</p>';return}
    el.innerHTML=xs.map(x=>'<div class="card">'+
      '<div class="name">@'+x.username+'</div>'+
      '<div class="price">'+fmt(x.amount,x.currency)+'</div>'+
      '<div class="meta"><span class="status-'+x.status+'">'+x.status+'</span>'+
      (x.owner_username?' &bull; @'+x.owner_username:'')+'</div></div>').join('');
  }catch(e){el.textContent='Каталог тимчасово недоступний.'}
}
function fmt(n,cur){
  if(cur==='XTR')return n+' ⭐';
  if(cur==='TON')return (n/1e9).toFixed(2)+' TON';
  return n+' '+cur;
}
load();
</script>
</body></html>`
