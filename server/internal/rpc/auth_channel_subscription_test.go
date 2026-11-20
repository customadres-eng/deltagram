package rpc

import (
	"context"
	"testing"

	"github.com/iamxvbaba/td/clock"
	"go.uber.org/zap"

	appchannels "telesrv/internal/app/channels"
	"telesrv/internal/domain"
	"telesrv/internal/store/memory"
)

func TestAutoJoinDeltaGramChannel(t *testing.T) {
	ctx := context.Background()
	store := memory.NewChannelStore()
	channels := appchannels.NewService(store)
	const ownerID, userID = 1001, 1002

	created, err := channels.CreateChannel(ctx, ownerID, domain.CreateChannelRequest{
		Title:     "DeltaGram",
		Broadcast: true,
		Date:      10,
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if _, err := channels.UpdateUsername(ctx, ownerID, domain.UpdateChannelUsernameRequest{
		ChannelID: created.Channel.ID,
		Username:  "dltagram",
	}); err != nil {
		t.Fatalf("set channel username: %v", err)
	}

	router := New(Config{}, Deps{Channels: channels}, zap.NewNop(), clock.System)
	router.autoJoinDeltaGramChannel(ctx, userID)
	router.autoJoinDeltaGramChannel(ctx, userID)

	member, err := store.GetParticipant(ctx, userID, created.Channel.ID, userID)
	if err != nil {
		t.Fatalf("get joined member: %v", err)
	}
	if member.Status != domain.ChannelMemberActive {
		t.Fatalf("membership status = %q, want active", member.Status)
	}
}
