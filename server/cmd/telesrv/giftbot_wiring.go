package main

// Wiring for the built-in gift-distribution bot (domain.GiftBotUserID).
//
// The bot lives in internal/app/bots, which deliberately does not import the
// admin or star-gift packages. This adapter is the only place the two meet: it
// exposes the enabled catalog and turns one broadcast grant into the same
// admin.Service.GiveGift call the operator panel uses, so every grant is
// recorded and replay-safe.

import (
	"context"

	adminapp "telesrv/internal/admin"
	"telesrv/internal/app/stargifts"
	"telesrv/internal/domain"
)

type giftBotBroadcaster struct {
	gifts *stargifts.Service
	admin *adminapp.Service
}

func (b giftBotBroadcaster) Catalog(ctx context.Context) ([]domain.StarGift, error) {
	return b.gifts.Catalog(ctx)
}

func (b giftBotBroadcaster) SendGift(ctx context.Context, userID, giftID int64, commandKey string) error {
	_, err := b.admin.GiveGift(ctx, adminapp.GiveGiftRequest{
		CommandMeta: adminapp.CommandMeta{
			CommandID: commandKey,
			Actor:     "giftbot",
			Reason:    "operator gift distribution",
		},
		UserID: userID,
		GiftID: giftID,
	})
	return err
}
