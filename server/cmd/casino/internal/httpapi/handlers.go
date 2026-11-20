package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"telesrv/cmd/casino/internal/network"
	"telesrv/cmd/casino/internal/store/postgres"
)

// context key for the authenticated DeltaGram user id.
type ctxKey int

const userKey ctxKey = 0

func withUser(r *http.Request, id int64) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userKey, id))
}

func userIDFrom(ctx context.Context) int64 {
	id, _ := ctx.Value(userKey).(int64)
	return id
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r.Context())
	balance, err := s.wallet.Balance(r.Context(), userID)
	if err != nil {
		writeErr(w, err)
		return
	}
	nfts, err := s.wallet.EscrowedNFTs(r.Context(), userID)
	if err != nil {
		writeErr(w, err)
		return
	}
	// Star balance is advisory: a failed read must not break the session, the
	// player just sees a deposit screen without the "available" hint.
	stars, _ := s.wallet.StarBalance(r.Context(), userID)
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":        userID,
		"balance":        balance,
		"stars":          s.wallet.StarsForChips(balance),
		"star_balance":   stars,
		"chips_per_star": s.wallet.ChipsPerStar(),
		"payout_percent": s.rounds.PayoutPercent(r.Context()),
		"nfts":           nfts,
	})
}

func (s *Server) handleDepositStars(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Stars int64  `json:"stars"`
		Key   string `json:"key"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Stars <= 0 || in.Key == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "нужны звёзды и ключ операции")
		return
	}
	balance, err := s.wallet.DepositStars(r.Context(), userIDFrom(r.Context()), in.Stars, in.Key)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "balance": balance})
}

func (s *Server) handleWithdrawStars(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Stars int64  `json:"stars"`
		Key   string `json:"key"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Stars <= 0 || in.Key == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "нужны звёзды и ключ операции")
		return
	}
	balance, err := s.wallet.WithdrawStars(r.Context(), userIDFrom(r.Context()), in.Stars, in.Key)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "balance": balance})
}

func (s *Server) handleDepositNFT(w http.ResponseWriter, r *http.Request) {
	var in struct {
		GiftRowID int64 `json:"gift_row_id"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.GiftRowID <= 0 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "нужен идентификатор подарка")
		return
	}
	if err := s.wallet.DepositNFT(r.Context(), userIDFrom(r.Context()), in.GiftRowID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleWithdrawNFT(w http.ResponseWriter, r *http.Request) {
	var in struct {
		GiftRowID int64 `json:"gift_row_id"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.GiftRowID <= 0 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "нужен идентификатор подарка")
		return
	}
	if err := s.wallet.WithdrawNFT(r.Context(), userIDFrom(r.Context()), in.GiftRowID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleNftArtwork returns the real rendering of a deposited/prize NFT: the
// Lottie model and pattern DeltaGram itself plays plus the backdrop palette.
func (s *Server) handleNftArtwork(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "нужен идентификатор подарка")
		return
	}
	art, err := s.wallet.NftArtwork(r.Context(), userIDFrom(r.Context()), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	writeJSON(w, http.StatusOK, art)
}

// writeErr maps the store/network sentinels onto stable HTTP responses.
// writeErr maps an internal error onto a stable machine code and a Russian
// message. The Mini App shows the message verbatim, so it must read like player
// copy, never like a Go error.
func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, network.ErrInsufficient):
		writeError(w, http.StatusBadRequest, "INSUFFICIENT_STARS", "На балансе DeltaGram недостаточно звёзд.")
	case errors.Is(err, postgres.ErrNoFunds):
		writeError(w, http.StatusBadRequest, "INSUFFICIENT", "Недостаточно фишек в казино.")
	case errors.Is(err, postgres.ErrNotFound):
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Не найдено.")
	case errors.Is(err, postgres.ErrDuplicate):
		writeError(w, http.StatusConflict, "CONFLICT", "Операция уже выполнена.")
	case errors.Is(err, network.ErrStarsDisabled):
		writeError(w, http.StatusServiceUnavailable, "STARS_DISABLED", "Перевод звёзд временно недоступен.")
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL", "Что-то пошло не так. Попробуйте ещё раз.")
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(out); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "некорректный запрос")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": code, "message": msg})
}
