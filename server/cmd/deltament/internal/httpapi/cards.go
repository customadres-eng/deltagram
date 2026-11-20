package httpapi

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"telesrv/cmd/deltament/internal/domain"
)

func (a *API) cardRegistry(w http.ResponseWriter, r *http.Request) {
	list, err := a.Cards.Registry(r.Context())
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"cards": list})
}

func (a *API) cardPrefs(w http.ResponseWriter, r *http.Request) {
	userID := domainUser(r)
	if userID <= 0 {
		writeErr(w, a.Log, domain.ErrUnauthorized)
		return
	}
	p, err := a.Cards.Prefs(r.Context(), userID)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, cardPrefsView(p))
}

func (a *API) cardSetPrefs(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ColorID  int   `json:"color_id"`
		NFTRowID int64 `json:"nft_row_id"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	userID := domainUser(r)
	if userID <= 0 {
		writeErr(w, a.Log, domain.ErrUnauthorized)
		return
	}
	p, err := a.Cards.SetPrefs(r.Context(), userID, in.ColorID, in.NFTRowID)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "prefs": cardPrefsView(p)})
}

func (a *API) cardSetAddress(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Address string `json:"address"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	userID := domainUser(r)
	if userID <= 0 {
		writeErr(w, a.Log, domain.ErrUnauthorized)
		return
	}
	card, err := a.Cards.SetAddress(r.Context(), userID, in.Address)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "card": card})
}

func (a *API) cardNfts(w http.ResponseWriter, r *http.Request) {
	userID := domainUser(r)
	if userID <= 0 {
		writeErr(w, a.Log, domain.ErrUnauthorized)
		return
	}
	gifts, err := a.Cards.Nfts(r.Context(), userID)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	out := make([]map[string]any, 0, len(gifts))
	for _, g := range gifts {
		v := toGiftAsset(g)
		url := ""
		if v.Slug != "" && v.Num > 0 {
			url = "https://dltagram.duckdns.org/api/gift-art/" + v.Slug + "-" + strconv.Itoa(v.Num) + ".webp"
		}
		out = append(out, map[string]any{
			"row_id": g.RowID,
			"gift_id": g.GiftID,
			"title":   g.Title,
			"slug":    v.Slug,
			"num":     g.Num,
			"url":     url,
			"unique":  g.Unique,
		})
	}
	writeJSON(w, 200, map[string]any{"nfts": out})
}

func (a *API) cardSend(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ToUsername string `json:"to_username"`
		Kind       string `json:"kind"`
		Amount     int64  `json:"amount"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	userID := domainUser(r)
	if userID <= 0 {
		writeErr(w, a.Log, domain.ErrUnauthorized)
		return
	}
	card, err := a.Cards.Send(r.Context(), userID, in.ToUsername, in.Kind, in.Amount)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "card": card})
}

func (a *API) cardSetBanner(w http.ResponseWriter, r *http.Request) {
	userID := domainUser(r)
	if userID <= 0 {
		writeErr(w, a.Log, domain.ErrUnauthorized)
		return
	}
	mime := strings.TrimSpace(r.Header.Get("Content-Type"))
	if !strings.HasPrefix(mime, "image/") {
		writeErr(w, a.Log, domain.ErrInvalid)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 3<<20)
	data, err := io.ReadAll(r.Body)
	if err != nil || len(data) < 128 {
		writeErr(w, a.Log, domain.ErrInvalid)
		return
	}
	token, err := a.Banners.SetUserBanner(r.Context(), userID, mime, data)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "token": token})
}

func cardPrefsView(p domain.CardPrefs) map[string]any {
	return map[string]any{
		"user_id":    p.UserID,
		"color_id":   p.ColorID,
		"nft_row_id": p.NFTRowID,
		"nft_slug":   p.NFTSlug,
		"nft_num":    p.NFTNum,
	}
}
