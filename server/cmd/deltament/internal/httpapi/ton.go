package httpapi

import (
	"math"
	"net/http"
	"strconv"

	"telesrv/cmd/deltament/internal/domain"
)

// tonToFloat renders nanotons as a TON amount with full 9-decimal precision
// preserved as far as a float can carry it for display.
func tonToFloat(nanoton int64) float64 {
	return float64(nanoton) / float64(domain.NanotonPerTON)
}

func tonCardView(c domain.TONCard) map[string]any {
	return map[string]any{
		"balance":         tonToFloat(c.Balance),
		"balance_nanoton": strconv.FormatInt(c.Balance, 10),
		"pending":         tonToFloat(c.Pending),
		"stars":           c.Stars,
		"rate": map[string]any{
			// The bot sells TON at a fixed price; the card is a 1:1 payout rail.
			"ton_per_gram":    1,
			"rub_per_100":     30,
			"min_withdraw":    1,
			"nanoton_per_ton": domain.NanotonPerTON,
		},
	}
}

func (a *API) tonCard(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	c, err := a.Wallet.TONCard(r.Context(), u)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, tonCardView(c))
}

func (a *API) withdrawTON(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Amount float64 `json:"amount"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	u, _ := userFrom(r.Context())
	nanoton, err := a.Wallet.WithdrawTON(r.Context(), u, domain.CentsFromFloat(in.Amount))
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	c, err := a.Wallet.TONCard(r.Context(), u)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"credited":         tonToFloat(nanoton),
		"credited_nanoton": strconv.FormatInt(nanoton, 10),
		"card":             tonCardView(c),
	})
}

func (a *API) setTONAddress(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Address string `json:"address"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	u, _ := userFrom(r.Context())
	if err := a.Wallet.SetTONAddress(r.Context(), u, in.Address); err != nil {
		writeErr(w, a.Log, err)
		return
	}
	c, err := a.Wallet.TONCard(r.Context(), u)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "card": tonCardView(c)})
}

func (a *API) tonHistory(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	list, err := a.Wallet.TONHistory(r.Context(), u)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, e := range list {
		out = append(out, map[string]any{
			"id":      sid(e.ID),
			"amount":  tonToFloat(e.Amount),
			"kind":    e.Kind,
			"title":   e.Title,
			"created": e.CreatedAt.UnixMilli(),
		})
	}
	writeJSON(w, 200, map[string]any{"entries": out})
}

func (a *API) requestTONPayout(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Amount float64 `json:"amount"`
		Note   string  `json:"note"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	u, _ := userFrom(r.Context())
	nanoton := int64(math.Round(in.Amount * float64(domain.NanotonPerTON)))
	p, err := a.Wallet.RequestTONPayout(r.Context(), u, nanoton, "", in.Note)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	c, err := a.Wallet.TONCard(r.Context(), u)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"payout": map[string]any{
			"id":      sid(p.ID),
			"address": p.Address,
			"amount":  tonToFloat(p.Amount),
			"status":  p.Status,
			"created": p.CreatedAt.UnixMilli(),
		},
		"card": tonCardView(c),
	})
}

func (a *API) adminGrantTON(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UserID int64   `json:"user_id"`
		Amount float64 `json:"amount"`
		Actor  string  `json:"actor"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	nanoton := int64(math.Round(in.Amount * float64(domain.NanotonPerTON)))
	c, err := a.Wallet.GrantTON(r.Context(), in.UserID, nanoton, in.Actor)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "card": tonCardView(c)})
}

func (a *API) adminTONPayouts(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	list, err := a.Wallet.TONPayouts(r.Context(), status)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		var sent int64
		if !p.SentAt.IsZero() {
			sent = p.SentAt.UnixMilli()
		}
		out = append(out, map[string]any{
			"id":      sid(p.ID),
			"user_id": sid(p.UserID),
			"address": p.Address,
			"amount":  tonToFloat(p.Amount),
			"status":  p.Status,
			"note":    p.Note,
			"created": p.CreatedAt.UnixMilli(),
			"sent":    sent,
		})
	}
	writeJSON(w, 200, map[string]any{"payouts": out})
}

func (a *API) adminTONPayoutStatus(w http.ResponseWriter, r *http.Request) {
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
	if err := a.Wallet.SetTONPayoutStatus(r.Context(), id, in.Status); err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *API) adminTONOrders(w http.ResponseWriter, r *http.Request) {
	list, err := a.Wallet.TONOrders(r.Context())
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, o := range list {
		out = append(out, map[string]any{
			"id":       sid(o.ID),
			"user_id":  sid(o.UserID),
			"username": o.Username,
			"ton":      o.TON,
			"rub":      o.RUB,
			"status":   o.Status,
			"created":  o.CreatedAt.UnixMilli(),
		})
	}
	writeJSON(w, 200, map[string]any{"orders": out})
}

func (a *API) adminTONOrderStatus(w http.ResponseWriter, r *http.Request) {
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
	o, err := a.Wallet.SetTONOrderStatus(r.Context(), id, in.Status)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "status": o.Status})
}
