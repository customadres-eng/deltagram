package bots

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"telesrv/internal/domain"
)

type fakeUpgrader struct {
	result domain.StarGiftUpgradeAllResult
	err    error
	calls  int
	userID int64
}

func (f *fakeUpgrader) UpgradeAllOwned(_ context.Context, userID int64, _ int) (domain.StarGiftUpgradeAllResult, error) {
	f.calls++
	f.userID = userID
	return f.result, f.err
}

func TestUpgradeCommandAliases(t *testing.T) {
	for _, body := range []string{
		"/upgrade", "upgrade", "!upgrade", "+upgrade",
		"/upgrade@deltagram_upgrade_bot", "/апгрейд", "/улучшить",
	} {
		if got := upgradeCommand(body); got != "upgrade" {
			t.Errorf("upgradeCommand(%q) = %q, want upgrade", body, got)
		}
	}
	for _, body := range []string{"/start", "/help", "старт", "помощь", "/start@deltagram_upgrade_bot"} {
		if got := upgradeCommand(body); got != "help" {
			t.Errorf("upgradeCommand(%q) = %q, want help", body, got)
		}
	}
	for _, body := range []string{"", "привет", "/stats", "/buy"} {
		if got := upgradeCommand(body); got != "" {
			t.Errorf("upgradeCommand(%q) = %q, want empty", body, got)
		}
	}
}

func TestUpgradeAllReplyReportsSpend(t *testing.T) {
	up := &fakeUpgrader{result: domain.StarGiftUpgradeAllResult{
		Upgraded: 3, SpentStars: 150, Balance: 850, BalanceKnown: true, Skipped: 1,
	}}
	s := &Service{giftUpgrader: up, now: func() time.Time { return time.Unix(1700000000, 0) }}
	reply := s.upgradeAllReply(context.Background(), 42)
	if up.calls != 1 || up.userID != 42 {
		t.Fatalf("upgrader called %d times for %d", up.calls, up.userID)
	}
	for _, want := range []string{"Улучшено подарков: 3", "Потрачено звёзд: 150", "Остаток звёзд: 850", "Пропущено недоступных: 1"} {
		if !strings.Contains(reply.Text, want) {
			t.Errorf("reply %q missing %q", reply.Text, want)
		}
	}
}

func TestUpgradeAllReplyExplainsInsufficientStars(t *testing.T) {
	up := &fakeUpgrader{result: domain.StarGiftUpgradeAllResult{StopReason: "insufficient_stars"}}
	s := &Service{giftUpgrader: up, now: time.Now}
	if reply := s.upgradeAllReply(context.Background(), 42); !strings.Contains(reply.Text, "Недостаточно звёзд") {
		t.Fatalf("reply = %q", reply.Text)
	}
}

func TestUpgradeAllReplyHandlesNothingToUpgrade(t *testing.T) {
	up := &fakeUpgrader{result: domain.StarGiftUpgradeAllResult{StopReason: "nothing_to_upgrade"}}
	s := &Service{giftUpgrader: up, now: time.Now}
	if reply := s.upgradeAllReply(context.Background(), 42); !strings.Contains(reply.Text, "нет подарков") {
		t.Fatalf("reply = %q", reply.Text)
	}
	up.result.Skipped = 2
	if reply := s.upgradeAllReply(context.Background(), 42); !strings.Contains(reply.Text, "уже улучшены") {
		t.Fatalf("reply = %q", reply.Text)
	}
}

func TestUpgradeAllReplyHandlesErrors(t *testing.T) {
	up := &fakeUpgrader{err: errors.New("boom")}
	s := &Service{giftUpgrader: up, now: time.Now}
	if reply := s.upgradeAllReply(context.Background(), 42); !strings.Contains(reply.Text, "Не удалось") {
		t.Fatalf("reply = %q", reply.Text)
	}
}
