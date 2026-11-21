package domain

import (
	"testing"
	"time"
)

func TestStarGiftPurchaseWindow(t *testing.T) {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Fatal(err)
	}
	gift := StarGift{
		PurchaseWindowEnabled:         true,
		PurchaseWindowStartMinute:     19*60 + 40,
		PurchaseWindowDurationMinutes: 30,
		PurchaseWindowTimezone:        "Europe/Kyiv",
	}
	cases := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"before", time.Date(2026, 11, 21, 19, 39, 0, 0, kyiv), false},
		{"start", time.Date(2026, 11, 21, 19, 40, 0, 0, kyiv), true},
		{"inside", time.Date(2026, 11, 21, 20, 9, 0, 0, kyiv), true},
		{"end", time.Date(2026, 11, 21, 20, 10, 0, 0, kyiv), false},
		{"summer_dst", time.Date(2026, 7, 21, 19, 40, 0, 0, kyiv), true},
		{"summer_before", time.Date(2026, 7, 21, 19, 39, 0, 0, kyiv), false},
	}
	for _, tc := range cases {
		if got := gift.PurchaseWindowOpen(tc.at); got != tc.want {
			t.Errorf("%s: PurchaseWindowOpen(%s)=%v, want %v", tc.name, tc.at, got, tc.want)
		}
	}
	if !(StarGift{}).PurchaseWindowOpen(time.Now()) {
		t.Fatal("a gift without a window must always be open")
	}
	bad := gift
	bad.PurchaseWindowTimezone = "Mars/Olympus"
	if bad.PurchaseWindowOpen(time.Now()) {
		t.Fatal("an invalid timezone must fail closed")
	}
}

func TestStarGiftPurchaseWindowWrapsMidnight(t *testing.T) {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Fatal(err)
	}
	gift := StarGift{
		PurchaseWindowEnabled:         true,
		PurchaseWindowStartMinute:     23*60 + 50,
		PurchaseWindowDurationMinutes: 30,
		PurchaseWindowTimezone:        "Europe/Kyiv",
	}
	if !gift.PurchaseWindowOpen(time.Date(2026, 11, 22, 0, 10, 0, 0, kyiv)) {
		t.Fatal("a window that crosses midnight must stay open after 00:00")
	}
	if gift.PurchaseWindowOpen(time.Date(2026, 11, 22, 0, 20, 0, 0, kyiv)) {
		t.Fatal("a window that crosses midnight must close after its duration")
	}
}
