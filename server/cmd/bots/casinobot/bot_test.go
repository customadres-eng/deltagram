package main

import (
	"testing"

	"github.com/iamxvbaba/td/tg"
)

func TestParseCommand(t *testing.T) {
	self := &tg.User{ID: 1780243404, Username: "deltagram_casino_bot"}
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"/start", "start", true},
		{"/start@deltagram_casino_bot", "start", true},
		{"/START", "start", true},
		{" /balance  ", "balance", true},
		{"!help", "help", true},
		{"/start@other_bot", "", false},
		{"привет", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, _, ok := parseCommand(c.in, self)
		if ok != c.ok || got != c.want {
			t.Fatalf("parseCommand(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestNormalizeCommand(t *testing.T) {
	cases := map[string]string{
		"start": "start", "Старт": "start", "почати": "start",
		"help": "help", "правила": "help", "допомога": "help",
		"balance": "balance", "Баланс": "balance",
		"unknown": "unknown",
	}
	for in, want := range cases {
		if got := normalizeCommand(in); got != want {
			t.Fatalf("normalizeCommand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatStars(t *testing.T) {
	cases := map[float64]string{
		0:      "0",
		100:    "100",
		99.2:   "99,2",
		1.05:   "1,05",
		1234.5: "1234,5",
	}
	for in, want := range cases {
		if got := formatStars(in); got != want {
			t.Fatalf("formatStars(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatInt(t *testing.T) {
	cases := map[int64]string{
		0: "0", 999: "999", 1000: "1 000", 9920: "9 920", 1234567: "1 234 567",
	}
	for in, want := range cases {
		if got := formatInt(in); got != want {
			t.Fatalf("formatInt(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestOpenMarkupIsWebView(t *testing.T) {
	b := newBot(nil, "https://dltacasino.duckdns.org/", 100)
	markup, ok := b.openMarkup().(*tg.ReplyInlineMarkup)
	if !ok || len(markup.Rows) != 1 || len(markup.Rows[0].Buttons) != 1 {
		t.Fatalf("openMarkup() = %T, want one inline row with one button", b.openMarkup())
	}
	btn := markup.Rows[0].Buttons[0]
	web, ok := btn.Type.(*tg.InlineButtonTypeWebView)
	if !ok {
		t.Fatalf("button type = %T, want *tg.InlineButtonTypeWebView", btn.Type)
	}
	if web.URL != "https://dltacasino.duckdns.org/" {
		t.Fatalf("webview url = %q", web.URL)
	}
}
