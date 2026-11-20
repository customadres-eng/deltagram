package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"telesrv/internal/admin"
)

// deltament.go bridges the operator console to the DeltaMent market so an
// operator can put a +888 number up for auction without touching the Deltament
// UI. DeltaMent exposes the write under /api/admin/lots guarded by the same
// shared admin token the panel already holds; nothing here speaks as a
// marketplace user.

type deltamentLotAPIRequest struct {
	CommandID    string  `json:"command_id"`
	Reason       string  `json:"reason"`
	Confirm      bool    `json:"confirm"`
	Phone        string  `json:"phone"`
	StartBid     float64 `json:"start_bid"`
	AuctionHours int     `json:"auction_hours"`
}

type deltamentLotView struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Slug   string  `json:"slug"`
	Price  float64 `json:"price"`
	Status string  `json:"status"`
	EndsAt int64   `json:"endsAt"`
}

func (s *server) handleDeltamentLotAPI(w http.ResponseWriter, r *http.Request) {
	var b deltamentLotAPIRequest
	if !decodeAction(w, r, &b) {
		return
	}
	b.Phone = strings.TrimSpace(b.Phone)
	if b.Phone == "" {
		writeAPIError(w, http.StatusBadRequest, "phone is required")
		return
	}
	if b.StartBid < 1 || b.StartBid > 100_000 {
		writeAPIError(w, http.StatusBadRequest, "стартовая ставка должна быть от 1 до 100000 GRAM")
		return
	}
	if b.AuctionHours <= 0 {
		b.AuctionHours = 24
	}
	if b.AuctionHours < 1 || b.AuctionHours > 24*30 {
		writeAPIError(w, http.StatusBadRequest, "длительность аукциона должна быть от 1 часа до 30 дней")
		return
	}
	if s.cfg.DeltamentURL == "" || s.cfg.DeltamentToken == "" {
		writeAPIError(w, http.StatusServiceUnavailable, "DeltaMent bridge is not configured (TELESRV_DELTAMENT_URL/TOKEN)")
		return
	}

	if !b.Confirm {
		writeJSON(w, http.StatusOK, admin.CommandResult{
			CommandID: b.CommandID, Action: "deltament.lot.create", Status: "dry-run",
			DryRun:  true,
			Message: "Проверьте параметры лота",
			Details: map[string]any{
				"number":        b.Phone,
				"starting_bid":  b.StartBid,
				"auction_hours": b.AuctionHours,
			},
		})
		return
	}

	payload := map[string]any{
		"kind":          "number",
		"name":          b.Phone,
		"price":         b.StartBid,
		"auction_hours": b.AuctionHours,
	}
	var out deltamentLotView
	if err := s.callDeltament(r.Context(), http.MethodPost, "/api/admin/lots", payload, &out); err != nil {
		writeJSON(w, http.StatusBadGateway, admin.CommandResult{
			CommandID: b.CommandID, Action: "deltament.lot.create", Status: "failed",
			Error: err.Error(), Message: "Не удалось выставить лот в DeltaMent",
		})
		return
	}
	writeJSON(w, http.StatusOK, admin.CommandResult{
		CommandID: b.CommandID, Action: "deltament.lot.create", Status: "executed",
		Message: "Лот выставлен на аукцион DeltaMent",
		Details: map[string]any{
			"lot_id": out.ID, "name": out.Name, "slug": out.Slug,
			"starting_bid": out.Price, "status": out.Status, "ends_at": out.EndsAt,
		},
	})
}

func (s *server) handleDeltamentLotsAPI(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DeltamentURL == "" || s.cfg.DeltamentToken == "" {
		writeAPIError(w, http.StatusServiceUnavailable, "DeltaMent bridge is not configured")
		return
	}
	var out []deltamentLotView
	if err := s.callDeltament(r.Context(), http.MethodGet, "/api/admin/lots", nil, &out); err != nil {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lots": out})
}

func (s *server) callDeltament(ctx context.Context, method, apiPath string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(s.cfg.DeltamentURL, "/")+apiPath, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.DeltamentToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("deltament: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("deltament %s: status=%d body=%s", apiPath, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("deltament %s: %w", apiPath, err)
	}
	return nil
}

// grantTONAPIRequest is the operator-console grant. TON is credited to the
// account's TON card through DeltaMent, exactly like a star grant is credited to
// the star wallet through the main Admin API.
type grantTONAPIRequest struct {
	CommandID string  `json:"command_id"`
	Reason    string  `json:"reason"`
	Confirm   bool    `json:"confirm"`
	UserID    int64   `json:"user_id"`
	Amount    float64 `json:"amount"`
}

func (s *server) handleGrantTONAPI(w http.ResponseWriter, r *http.Request) {
	var b grantTONAPIRequest
	if !decodeAction(w, r, &b) {
		return
	}
	if b.UserID <= 0 {
		writeAPIError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	if b.Amount <= 0 || b.Amount > 1_000_000 {
		writeAPIError(w, http.StatusBadRequest, "сумма TON должна быть от 0.000000001 до 1000000")
		return
	}
	if s.cfg.DeltamentURL == "" || s.cfg.DeltamentToken == "" {
		writeAPIError(w, http.StatusServiceUnavailable, "DeltaMent bridge is not configured (TELESRV_DELTAMENT_URL/TOKEN)")
		return
	}
	if !b.Confirm {
		writeJSON(w, http.StatusOK, admin.CommandResult{
			CommandID: b.CommandID, Action: "deltament.ton.grant", Status: "dry-run",
			DryRun:  true,
			Message: "Проверьте начисление TON",
			Details: map[string]any{"user_id": b.UserID, "amount_ton": b.Amount},
		})
		return
	}
	payload := map[string]any{"user_id": b.UserID, "amount": b.Amount, "actor": "admin-panel"}
	var out struct {
		OK   bool `json:"ok"`
		Card struct {
			Balance float64 `json:"balance"`
			Address string  `json:"address"`
		} `json:"card"`
	}
	if err := s.callDeltament(r.Context(), http.MethodPost, "/api/admin/ton/grant", payload, &out); err != nil {
		writeJSON(w, http.StatusBadGateway, admin.CommandResult{
			CommandID: b.CommandID, Action: "deltament.ton.grant", Status: "failed",
			Error: err.Error(), Message: "Не удалось начислить TON",
		})
		return
	}
	writeJSON(w, http.StatusOK, admin.CommandResult{
		CommandID: b.CommandID, Action: "deltament.ton.grant", Status: "executed",
		Message: "TON начислен на карту аккаунта",
		Details: map[string]any{
			"user_id": b.UserID, "amount_ton": b.Amount,
			"card_balance": out.Card.Balance, "card_address": out.Card.Address,
		},
	})
}
