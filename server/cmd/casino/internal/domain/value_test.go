package domain

import "testing"

func TestNftValueScalesBaseByFive(t *testing.T) {
	// A common common NFT is exactly 5x the market price.
	if got := NftValue(100, 50, 20, false); got != 500 {
		t.Fatalf("NftValue(100, common, common) = %d, want 500", got)
	}
}

func TestNftValueRarityMultipliers(t *testing.T) {
	cases := []struct {
		model    int
		backdrop int
		want     int64
	}{
		{50, 20, 1 * 1 * 5}, // common x common
		{18, 18, 2 * 2 * 5}, // uncommon x uncommon
		{12, 12, 4 * 4 * 5}, // rare x rare
		{8, 8, 8 * 8 * 5},   // epic x epic
		{1, 1, 16 * 16 * 5}, // legendary x legendary
	}
	for _, c := range cases {
		if got := NftValue(1, c.model, c.backdrop, false); got != c.want {
			t.Fatalf("NftValue(1, %d, %d) = %d, want %d", c.model, c.backdrop, got, c.want)
		}
	}
}

func TestNftValueBlackBackdropIsTopTier(t *testing.T) {
	// With the same legendary model, a black backdrop (x32) must beat a
	// legendary backdrop (x16), making black the single most expensive trait.
	legendaryBackdrop := NftValue(1, 1, 1, false) // 5 * 16 * 16 = 1280
	blackBackdrop := NftValue(1, 1, 1, true)      // 5 * 16 * 32 = 2560
	if blackBackdrop <= legendaryBackdrop {
		t.Fatalf("black backdrop %d should out-rank legendary backdrop %d", blackBackdrop, legendaryBackdrop)
	}
}
