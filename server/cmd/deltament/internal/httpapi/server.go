package httpapi

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"time"

	"telesrv/cmd/deltament/internal/domain"
	"telesrv/cmd/deltament/internal/service"
)

// хендлеры зависят от этих интерфейсов, а не от конкретных сервисов

type Auth interface {
	RequestCode(ctx context.Context, serverSlug, phone string) error
	VerifyCode(ctx context.Context, serverSlug, phone, code, device, place string) (service.LoginResult, error)
	Authenticate(ctx context.Context, token string) (domain.Session, domain.User, error)
	Sessions(ctx context.Context, userID int64) ([]domain.Session, error)
	Logout(ctx context.Context, userID int64, sessionID string) error
	Kick(ctx context.Context, userID int64, current, target string) error
	KickOthers(ctx context.Context, userID int64, current string) error
}

type Market interface {
	List(ctx context.Context, serverID int64, kind domain.LotKind) ([]service.LotView, error)
	Get(ctx context.Context, serverID int64, kind domain.LotKind, slug string) (service.LotView, error)
	GetByID(ctx context.Context, serverID, id int64) (service.LotView, error)
	Buy(ctx context.Context, u domain.User, lotID int64) (domain.Item, error)
	Bid(ctx context.Context, u domain.User, lotID int64, amount domain.Cents) error
	ToggleWatch(ctx context.Context, u domain.User, lotID int64) (bool, error)
	Watching(ctx context.Context, userID int64) ([]int64, error)
	MyBids(ctx context.Context, userID int64) ([]domain.Bid, error)
	CreateLot(ctx context.Context, serverID int64, in service.NewLot) (domain.Lot, error)
	DeleteLot(ctx context.Context, serverID, lotID int64) error
	ServerLots(ctx context.Context, serverID int64) ([]domain.Lot, error)
	Inventory(ctx context.Context, u domain.User) (service.Inventory, error)
	CreateUserLot(ctx context.Context, u domain.User, in service.NewUserLot) (domain.Lot, error)
	MyListings(ctx context.Context, userID int64) ([]domain.Lot, error)
	CancelUserLot(ctx context.Context, u domain.User, lotID int64) error
	OwnedGifts(ctx context.Context, u domain.User) ([]domain.GiftAsset, error)
	ListGift(ctx context.Context, u domain.User, in service.NewGiftLot) (domain.Lot, error)
	ClaimGift(ctx context.Context, u domain.User, itemID int64) error
}

type Wallet interface {
	History(ctx context.Context, userID int64) ([]domain.HistoryEntry, error)
	Items(ctx context.Context, userID int64) ([]domain.Item, error)
	TopUp(ctx context.Context, u domain.User, gram int64) (domain.Cents, error)
	Sell(ctx context.Context, u domain.User, itemID int64) (domain.Cents, error)
	Withdraw(ctx context.Context, u domain.User, itemID int64) error
	WithdrawStars(ctx context.Context, u domain.User, amount domain.Cents) (int64, error)
	// TON payout rail.
	TONCard(ctx context.Context, u domain.User) (domain.TONCard, error)
	TONHistory(ctx context.Context, u domain.User) ([]domain.TONEntry, error)
	WithdrawTON(ctx context.Context, u domain.User, amount domain.Cents) (int64, error)
	RequestTONPayout(ctx context.Context, u domain.User, amountNanoton int64, address, note string) (domain.TONPayout, error)
	SetTONAddress(ctx context.Context, u domain.User, address string) error
	TONPayouts(ctx context.Context, status string) ([]domain.TONPayout, error)
	SetTONPayoutStatus(ctx context.Context, id int64, status string) error
	TONOrders(ctx context.Context) ([]domain.TONOrder, error)
	SetTONOrderStatus(ctx context.Context, id int64, status string) (domain.TONOrder, error)
	GrantTON(ctx context.Context, mainUserID int64, amountNanoton int64, actor string) (domain.TONCard, error)
}

type Cards interface {
	Registry(ctx context.Context) ([]domain.CardPublic, error)
	Prefs(ctx context.Context, userID int64) (domain.CardPrefs, error)
	SetPrefs(ctx context.Context, userID int64, colorID int, nftRowID int64) (domain.CardPrefs, error)
	Nfts(ctx context.Context, userID int64) ([]domain.GiftAsset, error)
	SetAddress(ctx context.Context, userID int64, address string) (domain.CardPublic, error)
	Send(ctx context.Context, fromUserID int64, toUsername, kind string, amount int64) (domain.CardPublic, error)
}

type Domains interface {
	Check(ctx context.Context, userID int64, name string) (domain.TONDomainCheck, error)
	My(ctx context.Context, userID int64) ([]domain.TONDomain, error)
	Market(ctx context.Context, query, sort string, page, limit int) ([]domain.TONDomain, int, error)
	Buy(ctx context.Context, userID int64, name string) (domain.TONDomain, error)
	List(ctx context.Context, userID int64, name string, ask int64) (domain.TONDomain, error)
	Unlist(ctx context.Context, userID int64, name string) (domain.TONDomain, error)
}

type Servers interface {
	Register(ctx context.Context, in service.RegisterInput) (domain.Server, string, error)
	Public(ctx context.Context) ([]domain.Server, error)
	All(ctx context.Context) ([]domain.Server, error)
	BySlug(ctx context.Context, slug string) (domain.Server, error)
	SetStatus(ctx context.Context, id int64, status domain.ServerStatus) error
	Operator(ctx context.Context, key string) (domain.Server, error)
	RotateSecret(ctx context.Context, srv domain.Server, apiURL, secret string) error
}

type Deps struct {
	Auth          Auth
	Market        Market
	Wallet        Wallet
	Servers       Servers
	Frames        Frames
	Banners       Banners
	Domains       Domains
	Native        NativeAuthVerifier
	Cards         Cards
	GiftArt       GiftArt
	PrimaryServer string
	AdminToken    string
	CORSOrigin    string
	Log           *slog.Logger
}

// Frames serves the DeltaGram desktop avatar-frame registry. It is public on
// purpose: the native client fetches it anonymously and it only contains
// cosmetic user ids plus frame ids.
type Frames interface {
	AvatarFrames(ctx context.Context) (map[int64]string, error)
}

// Banners serves profile banners: the registry is public, uploads are not.
// GiftArt serves the real Lottie animations that DeltaGram renders for gifts.
// Regular gifts use the catalog animation, upgraded gifts compose a model and
// a pattern over the backdrop colors.
type GiftArt interface {
	GiftCatalogAnimation(ctx context.Context, giftID int64) ([]byte, error)
	GiftModelAnimation(ctx context.Context, id int64) ([]byte, error)
	GiftPatternAnimation(ctx context.Context, id int64) ([]byte, error)
	// GiftArtMeta reads the live artwork of a gift row. Inventory items bought
	// before the artwork upgrade have an empty art snapshot; the viewer fills
	// it in here so their cards render the real model instead of a placeholder.
	GiftArtMeta(ctx context.Context, rowID int64) (domain.GiftAsset, error)
}

type Banners interface {
	BannerTokens(ctx context.Context) (map[int64]string, error)
	UserBanner(ctx context.Context, userID int64) (domain.Banner, error)
	BannerToken(ctx context.Context, userID int64) (string, error)
	SetUserBanner(ctx context.Context, userID int64, mime string, data []byte) (string, error)
	ClearUserBanner(ctx context.Context, userID int64) error
}

type API struct{ Deps }

func New(d Deps) http.Handler {
	if d.PrimaryServer == "" {
		d.PrimaryServer = "deltagram"
	}
	a := &API{d}
	mux := http.NewServeMux()

	// публичное
	health := func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]bool{"ok": true}) }
	mux.HandleFunc("GET /api/health", health)
	// совместимость с прежним Deltament и внешними health-проверками
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("GET /api/servers", a.listServers)
	mux.HandleFunc("POST /api/servers", a.registerServer)
	mux.HandleFunc("POST /api/auth/code", a.requestCode)
	mux.HandleFunc("POST /api/auth/verify", a.verifyCode)
	mux.HandleFunc("GET /api/lots", a.maybeUser(a.listLots))
	mux.HandleFunc("GET /api/lots/{kind}/{slug}", a.maybeUser(a.getLot))
	mux.HandleFunc("GET /api/gifts", a.maybeUser(a.listGifts))
	mux.HandleFunc("GET /api/gifts/{id}", a.maybeUser(a.getGift))
	mux.HandleFunc("GET /api/gift-art/{file}", a.getGiftArt)
	mux.HandleFunc("GET /api/gift-anim/catalog/{id}", a.getGiftCatalogAnimation)
	mux.HandleFunc("GET /api/gift-anim/model/{id}", a.getGiftModelAnimation)
	mux.HandleFunc("GET /api/gift-anim/pattern/{id}", a.getGiftPatternAnimation)
	mux.HandleFunc("GET /api/cards/registry", a.cardRegistry)
	mux.HandleFunc("GET /api/cards/prefs", a.nativeAuth(a.cardPrefs))
	mux.HandleFunc("POST /api/cards/prefs", a.nativeAuth(a.cardSetPrefs))
	mux.HandleFunc("GET /api/cards/nfts", a.nativeAuth(a.cardNfts))
	mux.HandleFunc("POST /api/cards/address", a.nativeAuth(a.cardSetAddress))
	mux.HandleFunc("POST /api/cards/send", a.nativeAuth(a.cardSend))
	mux.HandleFunc("POST /api/cards/banner", a.nativeAuth(a.cardSetBanner))
	mux.HandleFunc("GET /api/domains/check", a.nativeAuth(a.domainCheck))
	mux.HandleFunc("GET /api/domains/my", a.nativeAuth(a.domainMy))
	mux.HandleFunc("GET /api/domains/listings", a.domainMarket)
	mux.HandleFunc("POST /api/domains/buy", a.nativeAuth(a.domainBuy))
	mux.HandleFunc("POST /api/domains/list", a.nativeAuth(a.domainList))
	mux.HandleFunc("POST /api/domains/unlist", a.nativeAuth(a.domainUnlist))
	mux.HandleFunc("GET /api/frames/registry", a.getFramesRegistry)
	mux.HandleFunc("GET /api/banners/registry", a.getBannerRegistry)
	mux.HandleFunc("GET /api/banners/{id}", a.getBanner)

	// юзер
	mux.HandleFunc("GET /api/me", a.user(a.me))
	mux.HandleFunc("GET /api/profile/banner", a.user(a.myBanner))
	mux.HandleFunc("POST /api/profile/banner", a.user(a.uploadBanner))
	mux.HandleFunc("DELETE /api/profile/banner", a.user(a.deleteBanner))
	mux.HandleFunc("POST /api/auth/logout", a.user(a.logout))
	mux.HandleFunc("DELETE /api/sessions/{id}", a.user(a.kickSession))
	mux.HandleFunc("POST /api/sessions/kick-others", a.user(a.kickOthers))
	mux.HandleFunc("POST /api/lots/{id}/buy", a.user(a.buy))
	mux.HandleFunc("POST /api/lots/{id}/bid", a.user(a.bid))
	mux.HandleFunc("DELETE /api/lots/{id}", a.user(a.cancelLot))
	mux.HandleFunc("POST /api/lots", a.user(a.createUserLot))
	mux.HandleFunc("GET /api/inventory", a.user(a.inventory))
	mux.HandleFunc("POST /api/lots/{id}/watch", a.user(a.watch))
	mux.HandleFunc("POST /api/wallet/topup", a.user(a.topUp))
	mux.HandleFunc("POST /api/wallet/withdraw", a.user(a.withdrawStars))
	mux.HandleFunc("GET /api/wallet/ton", a.user(a.tonCard))
	mux.HandleFunc("POST /api/wallet/ton/withdraw", a.user(a.withdrawTON))
	mux.HandleFunc("GET /api/wallet/ton/history", a.user(a.tonHistory))
	mux.HandleFunc("POST /api/wallet/ton/payout", a.user(a.requestTONPayout))
	mux.HandleFunc("POST /api/wallet/ton/address", a.user(a.setTONAddress))
	mux.HandleFunc("POST /api/items/{id}/sell", a.user(a.sell))
	mux.HandleFunc("POST /api/items/{id}/withdraw", a.user(a.withdraw))
	mux.HandleFunc("POST /api/items/{id}/claim", a.user(a.claimGift))
	mux.HandleFunc("GET /api/my-gifts", a.user(a.myGifts))
	mux.HandleFunc("POST /api/gifts/list", a.user(a.listGift))

	// оператор приватки
	mux.HandleFunc("GET /api/operator/server", a.operator(a.operatorServer))
	mux.HandleFunc("GET /api/operator/lots", a.operator(a.operatorLots))
	mux.HandleFunc("POST /api/operator/lots", a.operator(a.createLot))
	mux.HandleFunc("DELETE /api/operator/lots/{id}", a.operator(a.deleteLot))
	mux.HandleFunc("POST /api/operator/secret", a.operator(a.rotateSecret))

	// владелец deltament
	mux.HandleFunc("GET /api/admin/servers", a.admin(a.adminServers))
	mux.HandleFunc("POST /api/admin/servers/{id}/status", a.admin(a.adminSetStatus))
	// Operator-console bridge: the main admin panel lists a +888 number on the
	// DeltaMent auction with the shared admin token instead of a private-server
	// operator secret.
	mux.HandleFunc("GET /api/admin/lots", a.admin(a.adminLots))
	mux.HandleFunc("POST /api/admin/lots", a.admin(a.adminCreateLot))
	// TON payouts and purchases. The owner grants TON exactly like stars and
	// confirms the send queue from the bot or the admin panel.
	mux.HandleFunc("POST /api/admin/ton/grant", a.admin(a.adminGrantTON))
	mux.HandleFunc("GET /api/admin/ton/payouts", a.admin(a.adminTONPayouts))
	mux.HandleFunc("POST /api/admin/ton/payouts/{id}/status", a.admin(a.adminTONPayoutStatus))
	mux.HandleFunc("GET /api/admin/ton/orders", a.admin(a.adminTONOrders))
	mux.HandleFunc("POST /api/admin/ton/orders/{id}/status", a.admin(a.adminTONOrderStatus))

	return a.cors(a.logRequests(mux))
}

type ctxKey int

const (
	keyUser ctxKey = iota
	keySession
	keyServer
)

func userFrom(ctx context.Context) (domain.User, bool) {
	u, ok := ctx.Value(keyUser).(domain.User)
	return u, ok
}

func sessionFrom(ctx context.Context) domain.Session {
	s, _ := ctx.Value(keySession).(domain.Session)
	return s
}

func serverFrom(ctx context.Context) domain.Server {
	s, _ := ctx.Value(keyServer).(domain.Server)
	return s
}

func (a *API) withUser(r *http.Request) (*http.Request, error) {
	tok := bearer(r)
	if tok == "" {
		return r, domain.ErrUnauthorized
	}
	sess, u, err := a.Auth.Authenticate(r.Context(), tok)
	if err != nil {
		return r, err
	}
	ctx := context.WithValue(r.Context(), keyUser, u)
	ctx = context.WithValue(ctx, keySession, sess)
	return r.WithContext(ctx), nil
}

func (a *API) user(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r, err := a.withUser(r)
		if err != nil {
			writeErr(w, a.Log, err)
			return
		}
		next(w, r)
	}
}

// витрину можно смотреть и без входа
func (a *API) maybeUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if bearer(r) != "" {
			if authed, err := a.withUser(r); err == nil {
				r = authed
			}
		}
		next(w, r)
	}
}

func (a *API) operator(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		srv, err := a.Servers.Operator(r.Context(), bearer(r))
		if err != nil {
			writeErr(w, a.Log, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), keyServer, srv)))
	}
}

func (a *API) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := bearer(r)
		if a.AdminToken == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(a.AdminToken)) != 1 {
			writeErr(w, a.Log, domain.ErrUnauthorized)
			return
		}
		next(w, r)
	}
}

func (a *API) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.CORSOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", a.CORSOrigin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (a *API) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		a.Log.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status, "took", time.Since(start).Round(time.Millisecond))
	})
}
