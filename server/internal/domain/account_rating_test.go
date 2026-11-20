package domain

import "testing"

func TestAccountRatingSupportsLevel500(t *testing.T) {
	const wantThreshold int64 = 25_000_000

	if got := AccountRatingLevelThreshold(MaxAccountRatingLevel); got != wantThreshold {
		t.Fatalf("level %d threshold = %d, want %d", MaxAccountRatingLevel, got, wantThreshold)
	}

	level, current, next, hasNext := AccountRatingLevelForStars(wantThreshold)
	if level != 500 || current != wantThreshold || hasNext || next != 0 {
		t.Fatalf("level 500 projection = (%d, %d, %d, %t)", level, current, next, hasNext)
	}

	if got := AccountRatingLevelThreshold(501); got != wantThreshold {
		t.Fatalf("threshold above maximum = %d, want clamped %d", got, wantThreshold)
	}
}
