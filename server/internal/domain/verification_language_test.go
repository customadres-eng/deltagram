package domain

import "testing"

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
