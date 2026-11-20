package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
)

// betCommandID namespaces the client idempotency key by game so a reused key
// across two games can never collide in the rounds table.
func betCommandID(game, key string) string { return fmt.Sprintf("%s:%s", game, key) }

func (s *Server) handlePlaySlots(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Stake      int64  `json:"stake"`
		ClientSeed string `json:"client_seed"`
		Key        string `json:"key"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Stake <= 0 || in.Key == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "нужны ставка и ключ операции")
		return
	}
	res, err := s.rounds.PlaySlots(r.Context(), userIDFrom(r.Context()), in.Stake, in.ClientSeed, betCommandID("slots", in.Key))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handlePlayUpgrade(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Stake      int64   `json:"stake"`
		Target     float64 `json:"target"`
		ClientSeed string  `json:"client_seed"`
		Key        string  `json:"key"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Stake <= 0 || in.Key == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "нужны ставка и ключ операции")
		return
	}
	res, err := s.rounds.PlayUpgrade(r.Context(), userIDFrom(r.Context()), in.Stake, in.Target, in.ClientSeed, betCommandID("upgrade", in.Key))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handlePlayUpgradeNft(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Stake      int64   `json:"stake"`
		Target     float64 `json:"target"`
		ClientSeed string  `json:"client_seed"`
		Key        string  `json:"key"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Stake <= 0 || in.Key == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "нужны ставка и ключ операции")
		return
	}
	res, err := s.rounds.PlayUpgradeNft(r.Context(), userIDFrom(r.Context()), in.Stake, in.Target, in.ClientSeed, betCommandID("upgrade", in.Key))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleAdminAddBankNft prices a bank-held gift and adds it to the prize pool.
func (s *Server) handleAdminAddBankNft(w http.ResponseWriter, r *http.Request) {
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
	value, err := s.bank.AddNft(r.Context(), in.GiftRowID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "gift_row_id": in.GiftRowID, "value": value})
}

func (s *Server) handleStartCrash(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Stake      int64  `json:"stake"`
		ClientSeed string `json:"client_seed"`
		Key        string `json:"key"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Stake <= 0 || in.Key == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "нужны ставка и ключ операции")
		return
	}
	res, err := s.rounds.StartCrash(r.Context(), userIDFrom(r.Context()), in.Stake, in.ClientSeed, betCommandID("crash", in.Key))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleCrashState(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "нужен идентификатор раунда")
		return
	}
	res, err := s.rounds.CrashState(r.Context(), userIDFrom(r.Context()), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleCashoutCrash(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "нужен идентификатор раунда")
		return
	}
	var in struct {
		Multiplier float64 `json:"multiplier"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := s.rounds.CashoutCrash(r.Context(), userIDFrom(r.Context()), id, in.Multiplier)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleStartMines(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Stake      int64  `json:"stake"`
		Mines      int    `json:"mines"`
		ClientSeed string `json:"client_seed"`
		Key        string `json:"key"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Stake <= 0 || in.Key == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "нужны ставка и ключ операции")
		return
	}
	res, err := s.rounds.StartMines(r.Context(), userIDFrom(r.Context()), in.Stake, in.Mines, in.ClientSeed, betCommandID("mines", in.Key))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleRevealMines(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "нужен идентификатор раунда")
		return
	}
	var in struct {
		Cell int `json:"cell"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := s.rounds.RevealMines(r.Context(), userIDFrom(r.Context()), id, in.Cell)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleCashoutMines(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "нужен идентификатор раунда")
		return
	}
	res, err := s.rounds.CashoutMines(r.Context(), userIDFrom(r.Context()), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleListRounds(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	rounds, err := s.rounds.History(r.Context(), userIDFrom(r.Context()), limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rounds": rounds})
}

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
