package main

import (
	"strings"
	"testing"

	"github.com/iamxvbaba/td/tg"
)

func TestPrivateIntroMarkupAddsBotAsFullAdmin(t *testing.T) {
	markup, ok := privateIntroMarkup(&tg.User{Username: "irisgram_bot"}).(*tg.ReplyInlineMarkup)
	if !ok {
		t.Fatalf("markup type = %T, want *tg.ReplyInlineMarkup", privateIntroMarkup(&tg.User{Username: "irisgram_bot"}))
	}
	if len(markup.Rows) != 1 || len(markup.Rows[0].Buttons) != 1 {
		t.Fatalf("markup rows = %#v, want one button", markup.Rows)
	}
	button := markup.Rows[0].Buttons[0]
	if button.Text == "" {
		t.Fatal("button text is empty")
	}
	urlButton, ok := button.Type.(*tg.InlineButtonTypeURL)
	if !ok {
		t.Fatalf("button type = %T, want *tg.InlineButtonTypeURL", button.Type)
	}
	for _, want := range []string{
		"https://t.me/irisgram_bot?startgroup=&admin=",
		"restrict_members",
		"delete_messages",
		"promote_members",
		"change_info",
	} {
		if !strings.Contains(urlButton.URL, want) {
			t.Errorf("deep link %q missing %q", urlButton.URL, want)
		}
	}
	if strings.Contains(urlButton.URL, "@") {
		t.Errorf("deep link %q must not contain @", urlButton.URL)
	}
}

func TestPrivateIntroMarkupFallsBackToDefaultUsername(t *testing.T) {
	markup := privateIntroMarkup(nil).(*tg.ReplyInlineMarkup)
	urlButton := markup.Rows[0].Buttons[0].Type.(*tg.InlineButtonTypeURL)
	if !strings.Contains(urlButton.URL, "https://t.me/irisgram_bot?startgroup=&admin=") {
		t.Fatalf("fallback deep link = %q", urlButton.URL)
	}
}

func TestHelpDocumentsFreeTextReason(t *testing.T) {
	help := helpText()
	for _, want := range []string{
		"ПРИЧИНА НАКАЗАНИЯ",
		"/бан 1780243200 1д спам рекламой",
		"!мут (ответом) 30м нецензурная лексика",
		"/варн 1780243200 неуважение к участникам",
		"/айди",
		"Юзернеймы и имена не принимаются",
		"/статистика — ваша личная статистика",
		"/статистика чат",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("help text missing %q", want)
		}
	}
	for _, banned := range []string{"@user", "[@user|ID|имя]", "по username", "по имени"} {
		if strings.Contains(help, banned) {
			t.Errorf("help text still documents non-ID targeting %q", banned)
		}
	}
}

func TestParseUserIDTokenAcceptsOnlyDeltaGramIDs(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"1780243200", 1780243200, true},
		{" id1780243200 ", 1780243200, true},
		{"ID42", 42, true},
		{"@ivan", 0, false},
		{"ivan", 0, false},
		{"Иван", 0, false},
		{"id", 0, false},
		{"id@ivan", 0, false},
		{"0", 0, false},
		{"-5", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseUserIDToken(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parseUserIDToken(%q) = (%d, %v), want (%d, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestReasonTextSurvivesTargetAndDuration(t *testing.T) {
	// The command handlers strip the target first, then try to parse a leading
	// duration from the remaining words and keep everything else as the reason.
	cases := []struct {
		name       string
		rest       []string
		wantReason string
	}{
		{"duration then reason", []string{"1д", "спам", "рекламой"}, "спам рекламой"},
		{"combined duration", []string{"10м", "флуд", "в", "чате"}, "флуд в чате"},
		{"split duration", []string{"10", "минут", "оскорбления"}, "оскорбления"},
		{"reason only", []string{"нецензурная", "лексика"}, "нецензурная лексика"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, left, ok := parseDurationLoose(tc.rest)
			if tc.rest[0] == "10" && !ok {
				t.Fatalf("split duration was not parsed: %v", tc.rest)
			}
			reason := strings.Join(left, " ")
			if reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", reason, tc.wantReason)
			}
			if got := reasonSuffix(reason); !strings.Contains(got, tc.wantReason) {
				t.Fatalf("reasonSuffix = %q, want it to contain %q", got, tc.wantReason)
			}
		})
	}
}

func TestFormatCount(t *testing.T) {
	for _, tc := range []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1 000"},
		{1234567, "1 234 567"},
		{-2500, "-2 500"},
	} {
		if got := formatCount(tc.in); got != tc.want {
			t.Errorf("formatCount(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
