// Package httpapi serves the casino Mini App HTTP surface: Telegram Mini App
// authentication plus the wallet and game endpoints.
package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"telesrv/cmd/casino/internal/config"
	"telesrv/cmd/casino/internal/service"
)

// Server is the HTTP handler for the casino API.
type Server struct {
	cfg    config.Config
	wallet *service.Wallet
	rounds *service.RoundService
	bank   *service.NftBank
	assets fs.FS
}

// New builds the API handler.
func New(cfg config.Config, wallet *service.Wallet, rounds *service.RoundService, bank *service.NftBank) *Server {
	return &Server{cfg: cfg, wallet: wallet, rounds: rounds, bank: bank}
}

// SetAssets registers the embedded mini app directory (index.html, app.css,
// app.js). The service serves these three files directly so the app has no
// external asset dependency.
func (s *Server) SetAssets(root fs.FS) {
	sub, err := fs.Sub(root, "web")
	if err == nil {
		s.assets = sub
	}
}

// Routes returns the casino HTTP mux.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /api/session", s.auth(s.handleSession))
	mux.HandleFunc("POST /api/deposit/stars", s.auth(s.handleDepositStars))
	mux.HandleFunc("POST /api/withdraw/stars", s.auth(s.handleWithdrawStars))
	mux.HandleFunc("POST /api/deposit/nft", s.auth(s.handleDepositNFT))
	mux.HandleFunc("POST /api/withdraw/nft", s.auth(s.handleWithdrawNFT))
	mux.HandleFunc("GET /api/nft/{id}/art", s.auth(s.handleNftArtwork))

	// Games.
	mux.HandleFunc("POST /api/play/slots", s.auth(s.handlePlaySlots))
	mux.HandleFunc("POST /api/play/upgrade", s.auth(s.handlePlayUpgrade))
	mux.HandleFunc("POST /api/play/upgrade-nft", s.auth(s.handlePlayUpgradeNft))
	mux.HandleFunc("POST /api/rounds/crash", s.auth(s.handleStartCrash))
	mux.HandleFunc("GET /api/rounds/{id}/crash-state", s.auth(s.handleCrashState))
	mux.HandleFunc("POST /api/rounds/{id}/cashout", s.auth(s.handleCashoutCrash))
	mux.HandleFunc("POST /api/rounds/mines", s.auth(s.handleStartMines))
	mux.HandleFunc("POST /api/rounds/{id}/reveal", s.auth(s.handleRevealMines))
	mux.HandleFunc("POST /api/rounds/{id}/mines-cashout", s.auth(s.handleCashoutMines))
	mux.HandleFunc("GET /api/rounds", s.auth(s.handleListRounds))

	// NFT prize pool administration (operator only, keyed by the admin token).
	mux.HandleFunc("POST /api/admin/bank/nft", s.adminAuth(s.handleAdminAddBankNft))

	// Mini app static assets and single-page HTML.
	mux.HandleFunc("GET /app.css", s.serveAsset("app.css", "text/css; charset=utf-8"))
	mux.HandleFunc("GET /app.js", s.serveAsset("app.js", "application/javascript; charset=utf-8"))
	mux.HandleFunc("GET /lottie.min.js", s.serveAsset("lottie.min.js", "application/javascript; charset=utf-8"))
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /{path...}", s.handleIndex)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	html, err := fs.ReadFile(s.assets, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// A fresh Mini App launch must never be served from a stale cache: the
	// launch params live in the URL and the JS bundle changes with the game.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(html)
}

func (s *Server) serveAsset(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(s.assets, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(data)
	}
}

// auth resolves the DeltaGram user id from the Telegram Mini App init_data,
// verifying its HMAC when a bot token is configured. In dev (no token) it
// trusts the X-User-ID header so the API stays testable without a bot.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := s.resolveUser(r)
		if err != nil {
			s.logAuthFailure(r, err)
			writeError(w, http.StatusUnauthorized, "INIT_DATA_INVALID", "Не удалось подтвердить вход. Откройте казино заново из чата с ботом.")
			return
		}
		next(w, withUser(r, userID))
	}
}

// logAuthFailure records why a Mini App launch was rejected without writing the
// signed payload itself to the log. The parsed (unverified) user id and short
// hash prefixes are enough to tell a wrong bot token from a double-encoded or
// stale launch parameter.
func (s *Server) logAuthFailure(r *http.Request, cause error) {
	raw := r.Header.Get("X-Telegram-Init-Data")
	via := "header"
	if raw == "" {
		raw = r.URL.Query().Get("init_data")
		via = "query"
	}
	if raw == "" {
		log.Printf("casino auth rejected: %v; init_data absent", cause)
		return
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		log.Printf("casino auth rejected: %v; init_data malformed (%d bytes via %s): %v", cause, len(raw), via, err)
		return
	}
	var u struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(values.Get("user")), &u)
	log.Printf("casino auth rejected: %v; via=%s bytes=%d user_id=%d auth_date=%s hash_got=%s hash_want=%s literal_amp=%t pct3d=%t pct26=%t",
		cause, via, len(raw), u.ID, values.Get("auth_date"),
		hashPrefix(values.Get("hash")), hashPrefix(initDataHash(values, s.cfg.BotToken)),
		strings.Contains(raw, "&"), strings.Contains(raw, "%3D"), strings.Contains(raw, "%26"))
}

func hashPrefix(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

func (s *Server) resolveUser(r *http.Request) (int64, error) {
	if s.cfg.BotToken == "" {
		id, err := strconv.ParseInt(strings.TrimSpace(r.Header.Get("X-User-ID")), 10, 64)
		if err != nil || id <= 0 {
			return 0, errBadInitData("dev X-User-ID header missing or invalid")
		}
		return id, nil
	}
	raw := r.Header.Get("X-Telegram-Init-Data")
	if raw == "" {
		raw = r.URL.Query().Get("init_data")
	}
	return parseInitData(raw, s.cfg.BotToken)
}

// parseInitData verifies the Telegram WebApp init_data HMAC and returns the
// authenticated user id.
func parseInitData(raw, botToken string) (int64, error) {
	values, err := normalizeInitData(raw)
	if err != nil {
		return 0, errBadInitData("malformed init_data")
	}
	hash := values.Get("hash")
	if hash == "" || !validInitDataHash(values, botToken) {
		return 0, errBadInitData("hash mismatch")
	}
	rawUser := values.Get("user")
	var user struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(rawUser), &user); err != nil || user.ID <= 0 {
		return 0, errBadInitData("user missing")
	}
	return user.ID, nil
}

// normalizeInitData turns whatever launch payload the client forwarded into the
// plain `key=value&...` form Telegram signs. Standard clients hand over the
// decoded query string, but some clients forward tgWebAppData exactly as it
// appeared in the URL (percent-encoded once, or nested under the tgWebAppData
// key). Decoding one extra layer is safe: the HMAC is still verified over the
// resulting field set, so a forged payload cannot pass.
func normalizeInitData(raw string) (url.Values, error) {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "#")
	values, err := url.ParseQuery(raw)
	if err != nil {
		return nil, err
	}
	if values.Get("hash") != "" {
		return values, nil
	}
	// Nested under tgWebAppData, possibly still encoded.
	if nested := values.Get("tgWebAppData"); nested != "" {
		if inner, err := url.ParseQuery(nested); err == nil && inner.Get("hash") != "" {
			return inner, nil
		}
		if decoded, err := url.QueryUnescape(nested); err == nil {
			if inner, err := url.ParseQuery(decoded); err == nil && inner.Get("hash") != "" {
				return inner, nil
			}
		}
	}
	// Whole payload percent-encoded once.
	if decoded, err := url.QueryUnescape(raw); err == nil && decoded != raw {
		if inner, err := url.ParseQuery(decoded); err == nil && inner.Get("hash") != "" {
			return inner, nil
		}
	}
	return values, nil
}

func validInitDataHash(values url.Values, botToken string) bool {
	got := values.Get("hash")
	if got == "" {
		return false
	}
	want := initDataHash(values, botToken)
	return hmac.Equal([]byte(got), []byte(want))
}

// initDataHash is the Telegram WebApp HMAC over the sorted init-data fields,
// keyed by the bot token. Kept separate from validInitDataHash so the failure
// log can compare short prefixes without ever storing the whole signature.
func initDataHash(values url.Values, botToken string) string {
	secretMAC := hmac.New(sha256.New, []byte("WebAppData"))
	_, _ = secretMAC.Write([]byte(botToken))
	secret := secretMAC.Sum(nil)
	dataMAC := hmac.New(sha256.New, secret)
	_, _ = dataMAC.Write([]byte(initDataCheckString(values)))
	return hex.EncodeToString(dataMAC.Sum(nil))
}

func initDataCheckString(values url.Values) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(values.Get(k))
	}
	return b.String()
}

func errBadInitData(msg string) error { return &apiError{msg: msg} }

type apiError struct{ msg string }

func (e *apiError) Error() string { return e.msg }

// adminAuth gates operator-only endpoints with the shared Admin API token. It
// is the same credential the casino uses to move stars, so an operator who can
// run the casino can also seed its NFT prize pool.
func (s *Server) adminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AdminAPIToken == "" {
			writeError(w, http.StatusServiceUnavailable, "ADMIN_DISABLED", "admin token not configured")
			return
		}
		got := r.Header.Get("Authorization")
		if got != "Bearer "+s.cfg.AdminAPIToken {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid admin token")
			return
		}
		next(w, r)
	}
}
