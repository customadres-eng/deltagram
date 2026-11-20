package domain

import "testing"

func TestValidAdminUsername(t *testing.T) {
	cases := []struct {
		name   string
		want   bool
		reason string
	}{
		{name: "abc", want: true, reason: "three letters"},
		{name: "ab_", want: true, reason: "underscore is allowed after the first character"},
		{name: "a1c", want: true, reason: "digit is allowed after the first character"},
		{name: "ab", want: false, reason: "below the operator minimum"},
		{name: "1ab", want: false, reason: "must start with a letter"},
		{name: "_ab", want: false, reason: "must start with a letter"},
		{name: "ab-c", want: false, reason: "dash is not part of the grammar"},
		{name: "", want: false, reason: "empty"},
	}
	for _, tc := range cases {
		if got := ValidAdminUsername(tc.name); got != tc.want {
			t.Errorf("ValidAdminUsername(%q) = %v, want %v (%s)", tc.name, got, tc.want, tc.reason)
		}
	}
}

func TestValidAdminUsernameKeepsPublicCollectibleMinimum(t *testing.T) {
	if ValidCollectibleUsername("abc") {
		t.Fatal("public collectible minimum must stay at four characters")
	}
	if !ValidAdminUsername("abc") {
		t.Fatal("operator path must accept a three-character name")
	}
}
