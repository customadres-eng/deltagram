package rpc

import (
	"context"
	"testing"
	"unicode/utf16"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tlprofile"
	"telesrv/internal/domain"
)

// TestLocalizedVerificationDescription pins the server-side sentences: clients
// render bot_verification.description verbatim, so one stored organization name
// has to read naturally in every viewer's language.
func TestLocalizedVerificationDescription(t *testing.T) {
	for _, item := range []struct {
		lang string
		want string
	}{
		{"en", `The account is verified by the organization "Major".`},
		{"en-US", `The account is verified by the organization "Major".`},
		{"ru-RU", `Аккаунт верифицирован организацией «Major».`},
		{"uk-UA", `Акаунт верифіковано організацією «Major».`},
		{"de", `Das Konto wurde von der Organisation „Major“ verifiziert.`},
		{"zh-hans", `该账号已由组织“Major”验证。`},
		{"unknown", `The account is verified by the organization "Major".`},
	} {
		if got := domain.LocalizedVerificationDescription(item.lang, "Major"); got != item.want {
			t.Fatalf("%s sentence = %q, want %q", item.lang, got, item.want)
		}
	}
}

// TestCombinedBotVerificationCarriesInlineIcons covers the single-slot fold: the
// newest mark is the struct icon and description, and every additional mark
// contributes an inline custom-emoji entity so a profile can display several
// verifications with their own artwork.
func TestCombinedBotVerificationCarriesInlineIcons(t *testing.T) {
	ctx := WithClientInfo(context.Background(), ClientInfo{LangCode: "uk-UA"})
	marks := []domain.CustomVerification{
		{
			VerifierBotID:    1250000013,
			IconDocumentID:   5310000000000000101,
			OrganizationName: "Major",
		},
		{
			VerifierBotID:    1250000013,
			IconDocumentID:   5310000000000000106,
			OrganizationName: "Partner",
		},
	}
	projection, ok := combinedBotVerification(ctx, marks)
	if !ok {
		t.Fatal("combined projection rejected")
	}
	if projection.Icon != 5310000000000000101 || projection.BotID != 1250000013 {
		t.Fatalf("primary projection = %+v", projection)
	}
	first := `Акаунт верифіковано організацією «Major».`
	second := `Акаунт верифіковано організацією «Partner».`
	wantText := first + "\nx " + second
	if projection.Description.Text != wantText {
		t.Fatalf("description = %q, want %q", projection.Description.Text, wantText)
	}
	if len(projection.Description.Entities) != 1 {
		t.Fatalf("entities = %+v, want one inline badge", projection.Description.Entities)
	}
	entity, ok := projection.Description.Entities[0].(*tg.MessageEntityCustomEmoji)
	if !ok {
		t.Fatalf("entity type = %T, want custom emoji", projection.Description.Entities[0])
	}
	wantOffset := len(utf16.Encode([]rune(first + "\n")))
	if entity.Offset != wantOffset || entity.Length != 1 ||
		entity.DocumentID != 5310000000000000106 {
		t.Fatalf("entity = %+v, want offset %d len 1 doc 5310000000000000106", entity, wantOffset)
	}

	// A single mark keeps the plain sentence and no entities.
	single, ok := combinedBotVerification(ctx, marks[:1])
	if !ok || single.Description.Text != first || len(single.Description.Entities) != 0 {
		t.Fatalf("single projection = %+v ok=%v", single, ok)
	}

	// Layer 229 carries botVerification#f93cd45c, whose description is a plain
	// string: sending entities makes the wire encoder reject the whole response.
	// The sentences still stack, only the inline artwork is dropped.
	legacyCtx := WithLayer(ctx, 229)
	legacy, ok := combinedBotVerification(legacyCtx, marks)
	if !ok {
		t.Fatal("legacy combined projection rejected")
	}
	wantLegacy := first + "\n" + second
	if legacy.Description.Text != wantLegacy || len(legacy.Description.Entities) != 0 {
		t.Fatalf("legacy description = %+v, want plain %q", legacy.Description, wantLegacy)
	}
	// Layer 230 is the first profile with textWithEntities.
	modern, ok := combinedBotVerification(WithLayer(ctx, 230), marks)
	if !ok || len(modern.Description.Entities) != 1 {
		t.Fatalf("layer 230 projection = %+v ok=%v, want one inline badge", modern, ok)
	}
	// Prove both shapes cross the wire: the old profile must encode as the plain
	// string form instead of failing the whole response.
	var legacyWire, modernWire bin.Buffer
	if err := tlprofile.EncodeObject(tlprofile.Profile229, &legacy, &legacyWire); err != nil {
		t.Fatalf("encode layer 229 botVerification: %v", err)
	}
	if err := tlprofile.EncodeObject(tlprofile.Profile230, &modern, &modernWire); err != nil {
		t.Fatalf("encode layer 230 botVerification: %v", err)
	}
}
