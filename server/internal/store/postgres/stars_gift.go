package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"telesrv/internal/domain"
)

// GrantStarsGift credits a user's Stars balance and delivers an official
// messageActionGiftStars service message from the system account in one
// transaction. This is the "Stars received" card Telegram renders with its
// built-in star-box animation (the same one used for fiat Stars gifting), not a
// collectible Star Gift: no catalog sticker is attached and no saved gift is
// created. The value lives on the balance, so the card must never mint a second
// copy through the gift conversion path.
func (s *MessageStore) GrantStarsGift(ctx context.Context, req domain.StarsGiftGrant) (domain.StarsGiftGrantResult, error) {
	if s == nil || s.db == nil {
		return domain.StarsGiftGrantResult{}, domain.ErrStarGiftUnavailable
	}
	if req.Recipient.Type != domain.PeerTypeUser || req.Recipient.ID <= 0 ||
		req.Stars <= 0 || strings.TrimSpace(req.CommandKey) == "" || len(req.CommandKey) > 256 {
		return domain.StarsGiftGrantResult{}, domain.ErrStarGiftInvalid
	}
	if req.Date <= 0 {
		req.Date = int(time.Now().Unix())
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Stars gift"
	}
	action := &domain.MessageGiftStarsAction{
		Currency: "XTR",
		Amount:   req.Stars,
		Stars:    req.Stars,
	}
	messageReq := domain.SendPrivateTextRequest{
		SenderUserID:     domain.OfficialSystemUserID,
		RecipientUserID:  req.Recipient.ID,
		RandomID:         lifecycleCommandRandomID("stars-gift", req.Recipient.ID, req.CommandKey),
		Date:             req.Date,
		OriginUserID:     domain.OfficialSystemUserID,
		RecipientBlocked: req.RecipientBlocked,
		Media: &domain.MessageMedia{
			Kind:          domain.MessageMediaKindService,
			ServiceAction: &domain.MessageServiceAction{Kind: domain.MessageServiceActionGiftStars, GiftStars: action},
		},
	}

	var result domain.StarsGiftGrantResult
	credited := false
	hooks := privateSendTxHooks{
		before: func(ctx context.Context, tx pgx.Tx, _ *domain.SendPrivateTextRequest) error {
			var err error
			result.Balance, err = creditStarsTx(ctx, tx, req.Recipient.ID, req.Stars, req.Date, title, req.Reason)
			if err != nil {
				return err
			}
			credited = true
			// The recipient-only BalanceAfter lets online push and offline
			// difference emit updateStarsBalance without reading mutable state.
			action.BalanceAfter = result.Balance.Balance
			return nil
		},
		after: func(ctx context.Context, tx pgx.Tx, sent domain.SendPrivateTextResult) error {
			msgID := sent.RecipientMessage.ID
			if msgID <= 0 {
				msgID = sent.SenderMessage.ID
			}
			if msgID <= 0 {
				return fmt.Errorf("stars gift missing message id")
			}
			result.MsgID = msgID
			return nil
		},
	}
	sent, err := s.sendPrivateTextWithHooks(ctx, messageReq, hooks)
	if err != nil {
		return domain.StarsGiftGrantResult{}, err
	}
	if !credited || sent.Duplicate {
		// A replay returns the committed message without running the hooks, so
		// recover the balance and the existing message id.
		msgID := sent.RecipientMessage.ID
		if msgID <= 0 {
			msgID = sent.SenderMessage.ID
		}
		result.MsgID = msgID
		if err := s.db.QueryRow(ctx, `SELECT balance, granted FROM stars_balances WHERE user_id=$1`, req.Recipient.ID).
			Scan(&result.Balance.Balance, &result.Balance.Granted); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return domain.StarsGiftGrantResult{}, fmt.Errorf("load stars gift balance: %w", err)
		}
		result.Balance.UserID = req.Recipient.ID
	}
	return result, nil
}

// creditStarsTx is the transaction-scoped half of StarsStore.Credit: it bumps
// the balance and appends the ledger row without opening a nested transaction.
func creditStarsTx(ctx context.Context, tx pgx.Tx, userID, amount int64, date int, title, desc string) (domain.StarsBalance, error) {
	if userID <= 0 || amount <= 0 {
		return domain.StarsBalance{}, domain.ErrStarsInvalidAmount
	}
	out := domain.StarsBalance{UserID: userID}
	if err := tx.QueryRow(ctx, `
INSERT INTO stars_balances (user_id, balance, updated_at) VALUES ($1, $2, now())
ON CONFLICT (user_id) DO UPDATE SET balance = stars_balances.balance + EXCLUDED.balance, updated_at = now()
RETURNING balance, granted`, userID, amount).Scan(&out.Balance, &out.Granted); err != nil {
		return domain.StarsBalance{}, fmt.Errorf("credit stars balance: %w", err)
	}
	if err := insertStarsTxn(ctx, tx, userID, amount, domain.StarsReasonAdjust, domain.Peer{}, date, title, desc); err != nil {
		return domain.StarsBalance{}, err
	}
	return out, nil
}
