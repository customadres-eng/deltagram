package httpapi

import (
	"net/http"
	"strconv"

	"telesrv/cmd/deltament/internal/domain"
)

// domainUser returns the user resolved from the verified native request. It
// never reads headers or query parameters: the old X-DeltaGram-User path was
// an account takeover primitive.
func domainUser(r *http.Request) int64 {
	return nativeUser(r)
}

func domainView(d domain.TONDomain) map[string]any {
	return map[string]any{
		"name":              d.Name,
		"owner_id":          d.OwnerID,
		"owner_name":        d.OwnerName,
		"owner_username":    d.OwnerUser,
		"acquired_at":       d.AcquiredAt.UnixMilli(),
		"last_price_nanoton": d.LastPrice,
		"listed":            d.Listed,
		"ask_nanoton":       d.Ask,
	}
}

func (a *API) domainCheck(w http.ResponseWriter, r *http.Request) {
	userID := domainUser(r)
	if userID <= 0 {
		writeErr(w, a.Log, domain.ErrUnauthorized)
		return
	}
	check, err := a.Domains.Check(r.Context(), userID, r.URL.Query().Get("name"))
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"name":              check.Name,
		"state":             check.State,
		"price_nanoton":     check.Price,
		"base_price_nanoton": check.BasePrice,
		"owner_id":          check.OwnerID,
		"owner_name":        check.OwnerName,
		"owner_username":    check.OwnerUser,
	})
}

func (a *API) domainMy(w http.ResponseWriter, r *http.Request) {
	userID := domainUser(r)
	if userID <= 0 {
		writeErr(w, a.Log, domain.ErrUnauthorized)
		return
	}
	list, err := a.Domains.My(r.Context(), userID)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, d := range list {
		out = append(out, domainView(d))
	}
	writeJSON(w, 200, map[string]any{"domains": out})
}

func (a *API) domainMarket(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 30
	}
	list, total, err := a.Domains.Market(r.Context(), q.Get("query"), q.Get("sort"), page, limit)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, d := range list {
		out = append(out, domainView(d))
	}
	writeJSON(w, 200, map[string]any{"listings": out, "total": total, "page": page, "limit": limit})
}

func (a *API) domainBuy(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	userID := domainUser(r)
	if userID <= 0 {
		writeErr(w, a.Log, domain.ErrUnauthorized)
		return
	}
	dom, err := a.Domains.Buy(r.Context(), userID, in.Name)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "domain": domainView(dom)})
}

func (a *API) domainList(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name  string `json:"name"`
		Price int64  `json:"price_nanoton"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	userID := domainUser(r)
	if userID <= 0 {
		writeErr(w, a.Log, domain.ErrUnauthorized)
		return
	}
	dom, err := a.Domains.List(r.Context(), userID, in.Name, in.Price)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "domain": domainView(dom)})
}

func (a *API) domainUnlist(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	userID := domainUser(r)
	if userID <= 0 {
		writeErr(w, a.Log, domain.ErrUnauthorized)
		return
	}
	dom, err := a.Domains.Unlist(r.Context(), userID, in.Name)
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "domain": domainView(dom)})
}
