package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"telesrv/cmd/deltament/internal/domain"
	"telesrv/cmd/deltament/internal/service"
)

// сервера

func (a *API) listServers(w http.ResponseWriter, r *http.Request) {
	// маркет всегда работает только с DeltaGram
	srv, err := a.Servers.BySlug(r.Context(), a.PrimaryServer)
	if errors.Is(err, domain.ErrNotFound) || (err == nil && srv.Status != domain.ServerActive) {
		writeJSON(w, 200, []serverView{})
		return
	}
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, []serverView{toServer(srv)})
}

func (a *API) registerServer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Slug         string  `json:"slug"`
		Name         string  `json:"name"`
		Kind         string  `json:"kind"`
		APIURL       string  `json:"api_url"`
		Secret       string  `json:"secret"`
		Contact      string  `json:"contact"`
		StarsPerGram float64 `json:"stars_per_gram"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	srv, key, err := a.Servers.Register(r.Context(), service.RegisterInput{
		Slug: in.Slug, Name: in.Name, Kind: in.Kind, APIURL: in.APIURL, Secret: in.Secret, Contact: in.Contact, StarsPerGram: in.StarsPerGram,
	})
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"server":       toServer(srv),
		"status":       srv.Status,
		"operator_key": key,
	})
}

// вход

func (a *API) requestCode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Server string `json:"server"`
		Phone  string `json:"phone"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	// сервер из запроса игнорируем: вход всегда идёт через DeltaGram
	if err := a.Auth.RequestCode(r.Context(), a.PrimaryServer, in.Phone); err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"sent": true})
}

func (a *API) verifyCode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Server string `json:"server"`
		Phone  string `json:"phone"`
		Code   string `json:"code"`
		Device string `json:"device"`
		Place  string `json:"place"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	res, err := a.Auth.VerifyCode(r.Context(), a.PrimaryServer, in.Phone, in.Code, in.Device, in.Place)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"token": res.Token, "bonus": res.Bonus.Float()})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	if err := a.Auth.Logout(r.Context(), u.ID, sessionFrom(r.Context()).ID); err != nil {
		writeErr(w, a.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) kickSession(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	if err := a.Auth.Kick(r.Context(), u.ID, sessionFrom(r.Context()).ID, r.PathValue("id")); err != nil {
		writeErr(w, a.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) kickOthers(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	if err := a.Auth.KickOthers(r.Context(), u.ID, sessionFrom(r.Context()).ID); err != nil {
		writeErr(w, a.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// всё про юзера одним запросом, фронт держит это в сторе
func (a *API) me(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, _ := userFrom(ctx)
	sess := sessionFrom(ctx)

	items, err := a.Wallet.Items(ctx, u.ID)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	history, err := a.Wallet.History(ctx, u.ID)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	sessions, err := a.Auth.Sessions(ctx, u.ID)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	bids, err := a.Market.MyBids(ctx, u.ID)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	watching, err := a.Market.Watching(ctx, u.ID)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}

	// The TON card is the payout destination for rewards and lives next to the
	// stars card on the Money screen. A failed read must not break the whole
	// profile: the card simply renders with zeros.
	card, cardErr := a.Wallet.TONCard(ctx, u)
	if cardErr != nil {
		a.Log.Warn("ton card unavailable", "user", u.ID, "err", cardErr)
	}

	out := struct {
		User             userView                  `json:"user"`
		Server           string                    `json:"server"`
		Balance          float64                   `json:"balance"`
		Ton              float64                   `json:"ton"`
		TonPending       float64                   `json:"tonPending"`
		Stars            int64                     `json:"stars"`
		Inventory        []itemView                `json:"inventory"`
		History          []historyView             `json:"history"`
		Bids             map[string]map[string]any `json:"bids"`
		Watching         []string                  `json:"watching"`
		Sessions         []sessionView             `json:"sessions"`
		CurrentSessionID string                    `json:"currentSessionId"`
	}{
		User:             toUser(u),
		Balance:          u.Balance.Float(),
		Ton:              tonToFloat(card.Balance),
		TonPending:       tonToFloat(card.Pending),
		Stars:            card.Stars,
		Inventory:        []itemView{},
		History:          []historyView{},
		Bids:             map[string]map[string]any{},
		Watching:         []string{},
		Sessions:         []sessionView{},
		CurrentSessionID: sess.ID,
	}
	if srv, err := a.serverByID(r, u.ServerID); err == nil {
		out.Server = srv.Slug
	}
	// Баннер отдаём сразу ссылкой: клиенту не нужен отдельный запрос, чтобы
	// нарисовать фон под аватаром.
	if token, err := a.Banners.BannerToken(ctx, u.ServerUserID); err == nil && token != "" {
		out.User.Banner = bannerURL(u.ServerUserID, token)
	}
	for _, it := range items {
		a.fillItemArt(ctx, &it)
		out.Inventory = append(out.Inventory, toItem(it))
	}
	for _, h := range history {
		out.History = append(out.History, toHistory(h))
	}
	for _, b := range bids {
		out.Bids[sid(b.LotID)] = map[string]any{"amount": b.Amount.Float(), "ts": ms(b.CreatedAt)}
	}
	for _, id := range watching {
		out.Watching = append(out.Watching, sid(id))
	}
	for _, s := range sessions {
		out.Sessions = append(out.Sessions, toSession(s))
	}
	writeJSON(w, 200, out)
}

func (a *API) serverByID(r *http.Request, id int64) (domain.Server, error) {
	list, err := a.Servers.Public(r.Context())
	if err != nil {
		return domain.Server{}, err
	}
	for _, s := range list {
		if s.ID == id {
			return s, nil
		}
	}
	return domain.Server{}, domain.ErrNotFound
}

// витрина

// залогиненный видит свою приватку, гость выбирает через ?server=
func (a *API) viewerServer(r *http.Request) (int64, int64, error) {
	if u, ok := userFrom(r.Context()); ok {
		return u.ServerID, u.ID, nil
	}
	// гость всегда смотрит витрину DeltaGram, параметр server игнорируем
	srv, err := a.Servers.BySlug(r.Context(), a.PrimaryServer)
	if err != nil || srv.Status != domain.ServerActive {
		return 0, 0, domain.ErrNotFound
	}
	return srv.ID, 0, nil
}

func lotKind(s string) (domain.LotKind, bool) {
	switch domain.LotKind(s) {
	case domain.LotUsername, domain.LotNumber, domain.LotGift:
		return domain.LotKind(s), true
	}
	return "", false
}

func (a *API) listLots(w http.ResponseWriter, r *http.Request) {
	kind, ok := lotKind(r.URL.Query().Get("kind"))
	if !ok || kind == domain.LotGift {
		writeErr(w, a.Log, errors.Join(domain.ErrInvalid, errors.New("kind must be username or number")))
		return
	}
	serverID, viewer, err := a.viewerServer(r)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	out := []lotView{}
	if serverID != 0 {
		list, err := a.Market.List(r.Context(), serverID, kind)
		if err != nil {
			writeErr(w, a.Log, err)
			return
		}
		for _, v := range list {
			out = append(out, toLot(v, viewer))
		}
	}
	writeJSON(w, 200, out)
}

func (a *API) getLot(w http.ResponseWriter, r *http.Request) {
	kind, ok := lotKind(r.PathValue("kind"))
	if !ok || kind == domain.LotGift {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	serverID, viewer, err := a.viewerServer(r)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	v, err := a.Market.Get(r.Context(), serverID, kind, r.PathValue("slug"))
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, toLot(v, viewer))
}

func (a *API) listGifts(w http.ResponseWriter, r *http.Request) {
	serverID, _, err := a.viewerServer(r)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	out := []giftView{}
	if serverID != 0 {
		list, err := a.Market.List(r.Context(), serverID, domain.LotGift)
		if err != nil {
			writeErr(w, a.Log, err)
			return
		}
		u, _ := userFrom(r.Context())
		for _, v := range list {
			out = append(out, toGift(v, u.ID))
		}
	}
	writeJSON(w, 200, out)
}

func (a *API) getGift(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	serverID, _, err := a.viewerServer(r)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	v, err := a.Market.GetByID(r.Context(), serverID, id)
	if err != nil || v.Lot.Kind != domain.LotGift {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	u, _ := userFrom(r.Context())
	writeJSON(w, 200, toGift(v, u.ID))
}

// действия юзера

func (a *API) buy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	u, _ := userFrom(r.Context())
	it, err := a.Market.Buy(r.Context(), u, id)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, toItem(it))
}

func (a *API) bid(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	var in struct {
		Amount float64 `json:"amount"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	u, _ := userFrom(r.Context())
	if err := a.Market.Bid(r.Context(), u, id, domain.CentsFromFloat(in.Amount)); err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *API) watch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	u, _ := userFrom(r.Context())
	on, err := a.Market.ToggleWatch(r.Context(), u, id)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"watching": on})
}

func (a *API) topUp(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Amount int64 `json:"amount"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	u, _ := userFrom(r.Context())
	got, err := a.Wallet.TopUp(r.Context(), u, in.Amount)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]float64{"credited": got.Float()})
}

func (a *API) sell(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	u, _ := userFrom(r.Context())
	price, err := a.Wallet.Sell(r.Context(), u, id)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	// A resale is a consignment: the owner is paid when a real buyer settles,
	// not when the item is listed.
	writeJSON(w, 200, map[string]any{"listed": true, "price": price.Float()})
}

func (a *API) withdraw(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	u, _ := userFrom(r.Context())
	if err := a.Wallet.Withdraw(r.Context(), u, id); err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *API) withdrawStars(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Amount float64 `json:"amount"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	u, _ := userFrom(r.Context())
	stars, err := a.Wallet.WithdrawStars(r.Context(), u, domain.CentsFromFloat(in.Amount))
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"stars": stars, "debited": domain.CentsFromFloat(in.Amount).Float()})
}

func (a *API) inventory(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	inv, err := a.Market.Inventory(r.Context(), u)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	out := struct {
		Channels  []channelAssetView  `json:"channels"`
		Usernames []usernameAssetView `json:"usernames"`
		Listings  []lotView           `json:"listings"`
		Gifts     []giftAssetView     `json:"gifts"`
	}{
		Channels:  []channelAssetView{},
		Usernames: []usernameAssetView{},
		Listings:  []lotView{},
		Gifts:     []giftAssetView{},
	}
	for _, ch := range inv.Channels {
		out.Channels = append(out.Channels, channelAssetView{ID: sid(ch.ID), Title: ch.Title, Username: ch.Username})
	}
	for _, asset := range inv.Usernames {
		out.Usernames = append(out.Usernames, usernameAssetView{ID: sid(asset.ID), Username: asset.Username})
	}
	for _, g := range inv.Gifts {
		out.Gifts = append(out.Gifts, toGiftAsset(g))
	}
	for _, l := range inv.Listings {
		v, err := a.Market.GetByID(r.Context(), u.ServerID, l.ID)
		if err != nil {
			continue
		}
		lot := toLot(v, u.ID)
		lot.Mine = true
		out.Listings = append(out.Listings, lot)
	}
	writeJSON(w, 200, out)
}

func (a *API) createUserLot(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AssetKind       string  `json:"asset_kind"`
		ChannelID       int64   `json:"channel_id"`
		UsernameAssetID int64   `json:"username_asset_id"`
		Username        string  `json:"username"`
		Price           float64 `json:"price"`
		Mode            string  `json:"mode"`
		AuctionHours    int     `json:"auction_hours"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	mode := in.Mode
	if mode == "" {
		mode = "auction"
	}
	if mode != "auction" && mode != "sale" {
		writeErr(w, a.Log, fmt.Errorf("%w: mode must be auction or sale", domain.ErrInvalid))
		return
	}
	u, _ := userFrom(r.Context())
	l, err := a.Market.CreateUserLot(r.Context(), u, service.NewUserLot{
		AssetKind:       domain.AssetKind(in.AssetKind),
		ChannelID:       in.ChannelID,
		UsernameAssetID: in.UsernameAssetID,
		Username:        in.Username,
		Price:           domain.CentsFromFloat(in.Price),
		Auction:         mode == "auction",
		Duration:        time.Duration(in.AuctionHours) * time.Hour,
	})
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": sid(l.ID), "slug": l.Slug, "status": l.Status})
}

func (a *API) cancelLot(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	u, _ := userFrom(r.Context())
	if err := a.Market.CancelUserLot(r.Context(), u, id); err != nil {
		writeErr(w, a.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// подарки пользователя и их выставление на маркет

func (a *API) myGifts(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	gifts, err := a.Market.OwnedGifts(r.Context(), u)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	out := make([]giftAssetView, 0, len(gifts))
	for _, g := range gifts {
		out = append(out, toGiftAsset(g))
	}
	writeJSON(w, 200, map[string]any{"gifts": out})
}

func (a *API) listGift(w http.ResponseWriter, r *http.Request) {
	var in struct {
		GiftID       int64   `json:"gift_id"`
		Price        float64 `json:"price"`
		Mode         string  `json:"mode"`
		AuctionHours int     `json:"auction_hours"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	mode := in.Mode
	if mode == "" {
		mode = "sale"
	}
	if mode != "auction" && mode != "sale" {
		writeErr(w, a.Log, fmt.Errorf("%w: mode must be auction or sale", domain.ErrInvalid))
		return
	}
	hours := in.AuctionHours
	if hours <= 0 {
		hours = 24
	}
	u, _ := userFrom(r.Context())
	l, err := a.Market.ListGift(r.Context(), u, service.NewGiftLot{
		GiftRowID: in.GiftID,
		Price:     domain.CentsFromFloat(in.Price),
		Auction:   mode == "auction",
		Duration:  time.Duration(hours) * time.Hour,
	})
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": sid(l.ID), "slug": l.Slug, "status": l.Status})
}

func (a *API) claimGift(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	u, _ := userFrom(r.Context())
	if err := a.Market.ClaimGift(r.Context(), u, id); err != nil {
		writeErr(w, a.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// оператор

func (a *API) operatorServer(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, toAdminServer(serverFrom(r.Context())))
}

func (a *API) operatorLots(w http.ResponseWriter, r *http.Request) {
	lots, err := a.Market.ServerLots(r.Context(), serverFrom(r.Context()).ID)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	out := make([]map[string]any, 0, len(lots))
	for _, l := range lots {
		out = append(out, map[string]any{
			"id": sid(l.ID), "kind": l.Kind, "name": l.Name, "slug": l.Slug,
			"price": l.Price.Float(), "status": l.Status, "endsAt": ms(l.EndsAt), "gift": l.Gift,
		})
	}
	writeJSON(w, 200, out)
}

func (a *API) createLot(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind         string           `json:"kind"`
		Name         string           `json:"name"`
		Price        float64          `json:"price"`
		AuctionHours int              `json:"auction_hours"`
		Gift         *domain.GiftMeta `json:"gift"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	l, err := a.Market.CreateLot(r.Context(), serverFrom(r.Context()).ID, service.NewLot{
		Kind:     domain.LotKind(in.Kind),
		Name:     in.Name,
		Price:    domain.CentsFromFloat(in.Price),
		Auction:  in.AuctionHours > 0,
		Duration: time.Duration(in.AuctionHours) * time.Hour,
		Gift:     in.Gift,
	})
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": sid(l.ID), "slug": l.Slug, "status": l.Status})
}

func (a *API) deleteLot(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	if err := a.Market.DeleteLot(r.Context(), serverFrom(r.Context()).ID, id); err != nil {
		writeErr(w, a.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) rotateSecret(w http.ResponseWriter, r *http.Request) {
	var in struct {
		APIURL string `json:"api_url"`
		Secret string `json:"secret"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := a.Servers.RotateSecret(r.Context(), serverFrom(r.Context()), in.APIURL, in.Secret); err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// админка владельца

func (a *API) adminServers(w http.ResponseWriter, r *http.Request) {
	list, err := a.Servers.All(r.Context())
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	out := make([]adminServerView, 0, len(list))
	for _, s := range list {
		out = append(out, toAdminServer(s))
	}
	writeJSON(w, 200, out)
}

func (a *API) adminSetStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, a.Log, domain.ErrNotFound)
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := a.Servers.SetStatus(r.Context(), id, domain.ServerStatus(in.Status)); err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// adminLots lists the primary server's lots for the operator console, so the
// panel can show what is already on the DeltaMent auction after adding a number.
func (a *API) adminLots(w http.ResponseWriter, r *http.Request) {
	srv, err := a.Servers.BySlug(r.Context(), a.PrimaryServer)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	lots, err := a.Market.ServerLots(r.Context(), srv.ID)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	out := make([]map[string]any, 0, len(lots))
	for _, l := range lots {
		out = append(out, map[string]any{
			"id": sid(l.ID), "kind": l.Kind, "name": l.Name, "slug": l.Slug,
			"price": l.Price.Float(), "status": l.Status, "endsAt": ms(l.EndsAt),
		})
	}
	writeJSON(w, 200, out)
}

// adminCreateLot lets the main admin panel put a +888 number (or any lot) on
// the primary DeltaMent market. Price is in GRAM, matching the operator API.
func (a *API) adminCreateLot(w http.ResponseWriter, r *http.Request) {
	srv, err := a.Servers.BySlug(r.Context(), a.PrimaryServer)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	var in struct {
		Kind         string  `json:"kind"`
		Name         string  `json:"name"`
		Price        float64 `json:"price"`
		AuctionHours int     `json:"auction_hours"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Kind == "" {
		in.Kind = string(domain.LotNumber)
	}
	l, err := a.Market.CreateLot(r.Context(), srv.ID, service.NewLot{
		Kind:     domain.LotKind(in.Kind),
		Name:     in.Name,
		Price:    domain.CentsFromFloat(in.Price),
		Auction:  in.AuctionHours > 0,
		Duration: time.Duration(in.AuctionHours) * time.Hour,
	})
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": sid(l.ID), "name": l.Name, "slug": l.Slug,
		"price": l.Price.Float(), "status": l.Status, "endsAt": ms(l.EndsAt),
	})
}

// getFramesRegistry serves the public DeltaGram avatar-frame registry.
//
// The desktop client polls this anonymously and paints a cosmetic ring around
// matching avatars. The payload is intentionally tiny and cacheable so that a
// large user base costs one small response per client refresh interval.
func (a *API) getFramesRegistry(w http.ResponseWriter, r *http.Request) {
	if a.Frames == nil {
		writeJSON(w, 200, map[string]any{"version": 1, "frames": map[string]string{}})
		return
	}
	frames, err := a.Frames.AvatarFrames(r.Context())
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	out := make(map[string]string, len(frames))
	for id, frame := range frames {
		if frame == "" {
			continue
		}
		out[strconv.FormatInt(id, 10)] = frame
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, 200, map[string]any{
		"version": 1,
		"frames":  out,
	})
}

func (a *API) getVerificationRegistry(w http.ResponseWriter, r *http.Request) {
	if a.Frames == nil {
		writeJSON(w, 200, map[string]any{"icons": map[string]int64{}})
		return
	}
	icons, err := a.Frames.VerificationIcons(r.Context())
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	out := make(map[string]int64, len(icons))
	for userID, icon := range icons {
		out[strconv.FormatInt(userID, 10)] = icon
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, 200, map[string]any{"icons": out})
}
