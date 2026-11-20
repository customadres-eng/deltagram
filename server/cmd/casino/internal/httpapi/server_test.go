package httpapi

import (
	"net/url"
	"strings"
	"testing"
)

const testBotToken = "1780243404:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func signedInit(t *testing.T, token, userJSON string) string {
	t.Helper()
	values := url.Values{}
	values.Set("auth_date", "1794900000")
	values.Set("query_id", "AAHtest")
	values.Set("user", userJSON)
	values.Set("hash", initDataHash(values, token))
	return values.Encode()
}

// Clients differ in how they forward tgWebAppData: the decoded query string,
// the once-encoded value, or the whole fragment nested under tgWebAppData. All
// three carry the same signed fields and must authenticate.
func TestParseInitDataAcceptsForwardingShapes(t *testing.T) {
	const userJSON = `{"id":424242,"first_name":"Мама свинья"}`
	plain := signedInit(t, testBotToken, userJSON)

	shapes := map[string]string{
		"plain":    plain,
		"encoded":  url.QueryEscape(plain),
		"nested":   "tgWebAppData=" + url.QueryEscape(plain),
		"fragment": "#tgWebAppData=" + url.QueryEscape(plain),
	}
	for name, raw := range shapes {
		id, err := parseInitData(raw, testBotToken)
		if err != nil || id != 424242 {
			t.Fatalf("%s: parseInitData = (%d, %v), want (424242, nil)", name, id, err)
		}
	}
}

func TestParseInitDataRejectsForeignTokenAndTampering(t *testing.T) {
	const userJSON = `{"id":424242}`
	plain := signedInit(t, testBotToken, userJSON)

	const foreign = "1780243404:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	if _, err := parseInitData(plain, foreign); err == nil {
		t.Fatal("init data signed by another bot was accepted")
	}
	tampered := strings.Replace(plain, "auth_date=1794900000", "auth_date=1794900001", 1)
	if _, err := parseInitData(tampered, testBotToken); err == nil {
		t.Fatal("tampered payload was accepted")
	}
}
