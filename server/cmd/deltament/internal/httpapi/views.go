package httpapi

import (
	"context"
	"strconv"
	"strings"
	"time"

	"telesrv/cmd/deltament/internal/domain"
	"telesrv/cmd/deltament/internal/service"
)

// json в том виде, который ждёт фронт

func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func sid(id int64) string { return strconv.FormatInt(id, 10) }

type serverView struct {
	Slug  string  `json:"slug"`
	Name  string  `json:"name"`
	TopUp bool    `json:"topup"`
	Rate  float64 `json:"starsPerGram"`
}

func toServer(s domain.Server) serverView {
	return serverView{Slug: s.Slug, Name: s.Name, TopUp: s.StarsPerGram > 0, Rate: s.StarsPerGram}
}

type adminServerView struct {
	ID        int64   `json:"id"`
	Slug      string  `json:"slug"`
	Name      string  `json:"name"`
	Kind      string  `json:"kind"`
	APIURL    string  `json:"apiUrl"`
	Contact   string  `json:"contact"`
	Rate      float64 `json:"starsPerGram"`
	Status    string  `json:"status"`
	CreatedAt int64   `json:"createdAt"`
}

func toAdminServer(s domain.Server) adminServerView {
	return adminServerView{s.ID, s.Slug, s.Name, s.Kind, s.APIURL, s.Contact, s.StarsPerGram, string(s.Status), ms(s.CreatedAt)}
}

type userView struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Phone    string `json:"phone"`
	Joined   int64  `json:"joined"`
	// Banner is a ready-to-use URL when the user uploaded a profile banner.
	Banner string `json:"banner"`
}

func toUser(u domain.User) userView {
	name := u.Name
	if name == "" {
		name = "Deltament User"
	}
	// фронт рисует аватар по первой букве, пустым быть не может
	username := u.Username
	if username == "" {
		username = "user" + sid(u.ServerUserID%10000)
	}
	return userView{ID: u.ServerUserID, Name: name, Username: username, Phone: u.Phone, Joined: ms(u.CreatedAt)}
}

type sessionView struct {
	ID        string `json:"id"`
	Device    string `json:"device"`
	Place     string `json:"place"`
	CreatedAt int64  `json:"createdAt"`
	LastSeen  int64  `json:"lastSeen"`
}

func toSession(s domain.Session) sessionView {
	return sessionView{s.ID, s.Device, s.Place, ms(s.CreatedAt), ms(s.LastSeen)}
}

type itemView struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	Title    string  `json:"title"`
	Value    float64 `json:"value"`
	Ts       int64   `json:"ts"`
	Source   string  `json:"source"`
	Slug     string  `json:"slug,omitempty"`
	Num      int     `json:"num,omitempty"`
	Number   string  `json:"number,omitempty"`
	Username string  `json:"username,omitempty"`
	Amount   int64   `json:"amount,omitempty"`
	// GiftRowID is set when the item is an escrowed real gift that can be
	// moved to the buyer's profile ("Добавить в профиль").
	GiftRowID string         `json:"giftRowId,omitempty"`
	Art       domain.GiftArt `json:"art,omitempty"`
}

func toItem(it domain.Item) itemView {
	v := itemView{ID: sid(it.ID), Kind: string(it.Kind), Title: it.Title, Value: it.Value.Float(), Ts: ms(it.CreatedAt), Source: it.Source}
	switch it.Kind {
	case domain.ItemGift:
		v.Slug, v.Num = it.Slug, it.Num
		if it.GiftRowID > 0 {
			v.GiftRowID = sid(it.GiftRowID)
		}
		v.Art = it.Art
		if v.Art.GiftID == 0 {
			v.Art.GiftID = it.GiftID
		}
		if v.Art.ModelID > 0 {
			v.Art.Unique = true
		}
	case domain.ItemNumber:
		v.Number = it.Name
	case domain.ItemUsername:
		v.Username = it.Name
	case domain.ItemStars:
		v.Amount = it.Amount
	}
	return v
}

type historyView struct {
	ID     string  `json:"id"`
	Kind   string  `json:"kind"`
	Title  string  `json:"title"`
	Amount float64 `json:"amount"`
	Ts     int64   `json:"ts"`
}

func toHistory(h domain.HistoryEntry) historyView {
	return historyView{sid(h.ID), string(h.Kind), h.Title, h.Amount.Float(), ms(h.CreatedAt)}
}

type bidView struct {
	User   string  `json:"user"`
	Amount float64 `json:"amount"`
	Ts     int64   `json:"ts"`
}

type lotView struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	Price      float64   `json:"price"`
	StartPrice float64   `json:"startPrice"`
	MinBid     float64   `json:"minBid"`
	Status     string    `json:"status"`
	EndsAt     int64     `json:"endsAt"`
	Bids       []bidView `json:"bids"`
	Seller     string    `json:"seller,omitempty"`
	Mine       bool      `json:"mine,omitempty"`
}

type channelAssetView struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Username string `json:"username"`
}

type usernameAssetView struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type giftAssetView struct {
	RowID    string         `json:"rowId"`
	GiftID   string         `json:"giftId"`
	Title    string         `json:"title"`
	Slug     string         `json:"slug"`
	Num      int            `json:"num"`
	Date     int64          `json:"date"`
	Unique   bool           `json:"unique"`
	Model    domain.Trait   `json:"model"`
	Backdrop domain.Trait   `json:"backdrop"`
	Symbol   domain.Trait   `json:"symbol"`
	Art      domain.GiftArt `json:"art"`
}

// giftArtSlug turns a collection slug/title into the `slug` fragment.com uses
// for its art files. Unique gifts store slug as "<collection>-<num>" (for
// example "hangingstar-110"); the number is appended separately by the client,
// so it has to be stripped or the URL becomes "hangingstar-110-110.webp".
func giftArtSlug(slug, title string) string {
	raw := strings.ToLower(strings.TrimSpace(slug))
	if raw == "" {
		raw = strings.ToLower(strings.TrimSpace(title))
	}
	if i := strings.LastIndex(raw, "-"); i > 0 {
		if _, err := strconv.Atoi(raw[i+1:]); err == nil {
			raw = raw[:i]
		}
	}
	out := strings.NewReplacer(" ", "", "'", "", "-", "", ".", "", "_", "").Replace(raw)
	if out == "" {
		return "gift"
	}
	return out
}

func toGiftAsset(g domain.GiftAsset) giftAssetView {
	slug := giftArtSlug(g.Slug, g.Title)
	return giftAssetView{
		RowID: sid(g.RowID), GiftID: sid(g.GiftID), Title: g.Title, Slug: slug,
		Num: g.Num, Date: g.Date * 1000, Unique: g.Unique,
		Model: g.Model, Backdrop: g.Backdrop, Symbol: g.Symbol, Art: g.Art,
	}
}

// своя ставка показывается как "Вы"
func toLot(v service.LotView, viewerID int64) lotView {
	out := lotView{
		ID:         sid(v.Lot.ID),
		Kind:       string(v.Lot.Kind),
		Name:       v.Lot.Name,
		Slug:       v.Lot.Slug,
		Price:      v.Highest().Float(),
		StartPrice: v.Lot.Price.Float(),
		MinBid:     service.MinBid(v.Highest()).Float(),
		Status:     string(v.Lot.Status),
		EndsAt:     ms(v.Lot.EndsAt),
		Bids:       []bidView{},
	}
	if v.Lot.Status == domain.LotSold {
		out.Price = v.Lot.Price.Float()
	}
	if v.Lot.SellerUserID > 0 {
		out.Seller = "Продавец"
	}
	out.Mine = viewerID != 0 && v.Lot.SellerUserID == viewerID
	for _, b := range v.Bids {
		user := b.Masked
		if viewerID != 0 && b.UserID == viewerID {
			user = "Вы"
		}
		out.Bids = append(out.Bids, bidView{user, b.Amount.Float(), ms(b.CreatedAt)})
	}
	return out
}

type giftView struct {
	ID         string         `json:"id"`
	Slug       string         `json:"slug"`
	Collection string         `json:"collection"`
	Num        int            `json:"num"`
	Price      float64        `json:"price"`
	StartPrice float64        `json:"startPrice"`
	MinBid     float64        `json:"minBid"`
	Status     string         `json:"status"`
	Date       int64          `json:"date"`
	EndsAt     int64          `json:"endsAt"`
	Mine       bool           `json:"mine,omitempty"`
	Unique     bool           `json:"unique"`
	Model      domain.Trait   `json:"model"`
	Backdrop   domain.Trait   `json:"backdrop"`
	Symbol     domain.Trait   `json:"symbol"`
	Art        domain.GiftArt `json:"art"`
	Bids       []bidView      `json:"bids"`
}

func toGift(v service.LotView, viewerID int64) giftView {
	l := v.Lot
	g := l.Gift
	if g == nil {
		g = &domain.GiftMeta{}
	}
	date := l.CreatedAt
	if l.Status == domain.LotSold && !l.EndsAt.IsZero() {
		date = l.EndsAt
	}
	out := giftView{
		ID:         sid(l.ID),
		Slug:       giftArtSlug(g.Slug, g.Collection),
		Collection: g.Collection,
		Num:        g.Num,
		Price:      v.Highest().Float(),
		StartPrice: l.Price.Float(),
		MinBid:     service.MinBid(v.Highest()).Float(),
		Status:     string(l.Status),
		Date:       ms(date),
		EndsAt:     ms(l.EndsAt),
		Mine:       viewerID != 0 && l.SellerUserID == viewerID,
		Unique:     g.Art.Unique || g.Art.ModelID > 0 || g.Model.Name != "",
		Model:      g.Model,
		Backdrop:   g.Backdrop,
		Symbol:     g.Symbol,
		Art:        g.Art,
		Bids:       []bidView{},
	}
	if l.Status == domain.LotSold {
		out.Price = l.Price.Float()
	}
	for _, b := range v.Bids {
		user := b.Masked
		if viewerID != 0 && b.UserID == viewerID {
			user = "Вы"
		}
		out.Bids = append(out.Bids, bidView{user, b.Amount.Float(), ms(b.CreatedAt)})
	}
	return out
}

// fillItemArt backfills the artwork of inventory items that were bought before
// lots started carrying GiftArt. The gift row still exists (the relayer keeps it
// until the buyer claims it), so its live model/pattern/backdrop is authoritative.
func (a *API) fillItemArt(ctx context.Context, it *domain.Item) {
	if it == nil || it.Kind != domain.ItemGift || it.GiftRowID <= 0 || a.GiftArt == nil {
		return
	}
	if it.Art.GiftID != 0 || it.Art.ModelID != 0 {
		return
	}
	meta, err := a.GiftArt.GiftArtMeta(ctx, it.GiftRowID)
	if err != nil {
		return
	}
	it.Art = meta.Art
	if it.Num == 0 {
		it.Num = meta.Num
	}
	if it.Slug == "" {
		it.Slug = giftArtSlug(meta.Slug, meta.Title)
	}
}
