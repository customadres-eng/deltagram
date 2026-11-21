package domain

import (
	"strings"
	"testing"
)

func TestLocalizedVerificationDescriptionKeepsOrganizationName(t *testing.T) {
	cases := map[string]string{
		"ru": `Аккаунт верифицирован организацией «Major».`,
		"ru-RU": `Аккаунт верифицирован организацией «Major».`,
		"uk": `Акаунт верифіковано організацією «Major».`,
		"en": `The account is verified by the organization "Major".`,
		// Unknown languages fall back to English rather than the granting
		// operator's language.
		"xx": `The account is verified by the organization "Major".`,
	}
	for lang, want := range cases {
		if got := LocalizedVerificationDescription(lang, "Major"); got != want {
			t.Errorf("lang %q = %q, want %q", lang, got, want)
		}
	}
}

func TestChannelVerificationProjectionUsesChannelWording(t *testing.T) {
	mark := CustomVerification{
		VerifierBotID: 1, IconDocumentID: 2,
		Peer:             Peer{Type: PeerTypeChannel, ID: 42},
		OrganizationName: "DeltaGram",
	}
	for _, language := range []string{"ru", "uk", "en"} {
		got := mark.ProjectionForLanguage(language).Description
		if strings.Contains(strings.ToLower(got), "аккаунт") || strings.Contains(strings.ToLower(got), "account") {
			t.Fatalf("%s channel mark = %q, want channel wording", language, got)
		}
	}
	legacy := mark
	legacy.OrganizationName = ""
	legacy.Description = "Аккаунт верифицирован организацией «DeltaGram»."
	if got := legacy.Projection().Description; !strings.Contains(got, "Канал") {
		t.Fatalf("legacy channel mark = %q, want Канал", got)
	}
}
