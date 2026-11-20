package bots

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"telesrv/internal/domain"
	"telesrv/internal/store/memory"
)

type fakeGiftBroadcaster struct {
	gifts []domain.StarGift
	mu    sync.Mutex
	sent  []fakeGiftSend
}

type fakeGiftSend struct {
	userID int64
	giftID int64
	key    string
}

func (f *fakeGiftBroadcaster) Catalog(context.Context) ([]domain.StarGift, error) {
	return append([]domain.StarGift(nil), f.gifts...), nil
}

func (f *fakeGiftBroadcaster) SendGift(_ context.Context, userID, giftID int64, commandKey string) error {
	f.mu.Lock()
	f.sent = append(f.sent, fakeGiftSend{userID: userID, giftID: giftID, key: commandKey})
	f.mu.Unlock()
	return nil
}

func (f *fakeGiftBroadcaster) sends() []fakeGiftSend {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeGiftSend(nil), f.sent...)
}

type fakeGiftRecipients struct {
	ids []int64
}

func (f fakeGiftRecipients) RealUserIDs(context.Context) ([]int64, error) {
	return append([]int64(nil), f.ids...), nil
}

type giftBotFixture struct {
	svc      *Service
	users    *memory.UserStore
	messages *memory.MessageStore
	owner    domain.User
	other    domain.User
}

func newGiftBotFixture(t *testing.T, broadcaster *fakeGiftBroadcaster, recipients []int64) giftBotFixture {
	t.Helper()
	users := memory.NewUserStore()
	botStore := memory.NewBotStore(users)
	dialogs := memory.NewDialogStore()
	messages := memory.NewMessageStore(dialogs)
	owner, err := users.Create(context.Background(), domain.User{AccessHash: 1, Phone: "+7700", FirstName: "Operator"})
	if err != nil {
		t.Fatalf("create operator: %v", err)
	}
	other, err := users.Create(context.Background(), domain.User{AccessHash: 2, Phone: "+7701", FirstName: "Other"})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	svc := NewService(users, botStore, messages, WithGiftBotAdmins([]int64{owner.ID}))
	svc.SetGiftBot(broadcaster, fakeGiftRecipients{ids: recipients})
	return giftBotFixture{svc: svc, users: users, messages: messages, owner: owner, other: other}
}

func giftBotReplies(t *testing.T, messages *memory.MessageStore, userID int64) []domain.Message {
	t.Helper()
	list, err := messages.ListByUser(context.Background(), userID, domain.MessageFilter{
		HasPeer: true,
		Peer:    domain.Peer{Type: domain.PeerTypeUser, ID: domain.GiftBotUserID},
		Limit:   100,
	})
	if err != nil {
		t.Fatalf("list gift bot history: %v", err)
	}
	out := make([]domain.Message, 0, len(list.Messages))
	for _, msg := range list.Messages {
		if msg.From.ID == domain.GiftBotUserID {
			out = append(out, msg)
		}
	}
	return out
}

func latestGiftBotReply(t *testing.T, messages *memory.MessageStore, userID int64) domain.Message {
	t.Helper()
	replies := giftBotReplies(t, messages, userID)
	if len(replies) == 0 {
		t.Fatal("no gift bot reply")
	}
	latest := replies[0]
	for _, msg := range replies[1:] {
		if msg.ID > latest.ID {
			latest = msg
		}
	}
	return latest
}

func TestGiftBotCatalogOnlyForAdmins(t *testing.T) {
	broadcaster := &fakeGiftBroadcaster{gifts: []domain.StarGift{
		{ID: 1, Title: "Rose", Stars: 25},
		{ID: 2, Title: "Bear", Stars: 15},
	}}
	f := newGiftBotFixture(t, broadcaster, []int64{11, 12})

	f.svc.respondAsGiftBot(f.owner.ID, "/gifts")
	reply := latestGiftBotReply(t, f.messages, f.owner.ID)
	if reply.ReplyMarkup == nil || len(reply.ReplyMarkup.Inline) != 2 {
		t.Fatalf("catalog markup = %+v", reply.ReplyMarkup)
	}
	if got := string(reply.ReplyMarkup.Inline[0][0].Data); got != "gb:g:1" {
		t.Fatalf("first gift callback = %q", got)
	}

	before := len(giftBotReplies(t, f.messages, f.other.ID))
	f.svc.respondAsGiftBot(f.other.ID, "/gifts")
	if after := len(giftBotReplies(t, f.messages, f.other.ID)); after != before {
		t.Fatalf("non-admin received a catalog reply")
	}
}

func TestGiftBotCustomQuantityBroadcast(t *testing.T) {
	broadcaster := &fakeGiftBroadcaster{gifts: []domain.StarGift{{ID: 7, Title: "Rose", Stars: 25}}}
	f := newGiftBotFixture(t, broadcaster, []int64{11, 12})

	// Select the gift, ask for a custom quantity, then enter it as plain text.
	if _, handled, err := f.svc.onGiftBotCallback(context.Background(), domain.BotCallbackQuery{
		BotUserID: domain.GiftBotUserID, UserID: f.owner.ID, Data: []byte("gb:q:7:c"),
	}); err != nil || !handled {
		t.Fatalf("custom prompt callback handled=%v err=%v", handled, err)
	}
	if !strings.Contains(latestGiftBotReply(t, f.messages, f.owner.ID).Body, "от 1 до") {
		t.Fatalf("missing quantity prompt")
	}
	f.svc.respondAsGiftBot(f.owner.ID, "3")
	confirm := latestGiftBotReply(t, f.messages, f.owner.ID)
	if !strings.Contains(confirm.Body, "Получателей: 2") || !strings.Contains(confirm.Body, "Всего подарков: 6") {
		t.Fatalf("confirmation = %q", confirm.Body)
	}

	if _, handled, err := f.svc.onGiftBotCallback(context.Background(), domain.BotCallbackQuery{
		BotUserID: domain.GiftBotUserID, UserID: f.owner.ID, Data: []byte("gb:c:7:3"),
	}); err != nil || !handled {
		t.Fatalf("confirm callback handled=%v err=%v", handled, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for len(broadcaster.sends()) < 6 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	sent := broadcaster.sends()
	if len(sent) != 6 {
		t.Fatalf("sent %d gifts, want 6", len(sent))
	}
	seen := make(map[string]struct{}, len(sent))
	for _, grant := range sent {
		if grant.giftID != 7 {
			t.Fatalf("gift id = %d", grant.giftID)
		}
		if _, ok := seen[grant.key]; ok {
			t.Fatalf("duplicate command key %q", grant.key)
		}
		seen[grant.key] = struct{}{}
	}
}
