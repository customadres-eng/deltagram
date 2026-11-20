package main

import (
	"strings"
	"testing"

	"telesrv/internal/botdata"
)

func TestCompareMogStarsAndBlackOnly(t *testing.T) {
	// Total gifts are deliberately skewed the other way: they must not score.
	a := botdata.MogStats{Stars: 1000, Gifts: botdata.GiftCounts{Total: 3, BlackBackdrop: 2}}
	b := botdata.MogStats{Stars: 500, Gifts: botdata.GiftCounts{Total: 100, BlackBackdrop: 0}}
	v := compareMog(a, b)
	if v.StarsWinner != -1 || v.BlackWinner != -1 {
		t.Fatalf("category winners = %d/%d, want -1/-1", v.StarsWinner, v.BlackWinner)
	}
	if v.Winner != -1 || v.AWins != 2 || v.BWins != 0 {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestCompareMogDrawOnSplitCategories(t *testing.T) {
	// One category each: a wins black, b wins stars -> draw.
	a := botdata.MogStats{Stars: 100, Gifts: botdata.GiftCounts{Total: 3, BlackBackdrop: 4}}
	b := botdata.MogStats{Stars: 500, Gifts: botdata.GiftCounts{Total: 9, BlackBackdrop: 1}}
	v := compareMog(a, b)
	if v.StarsWinner != 1 || v.BlackWinner != -1 {
		t.Fatalf("category winners = %d/%d, want 1/-1", v.StarsWinner, v.BlackWinner)
	}
	if v.Winner != 0 || v.AWins != 1 || v.BWins != 1 {
		t.Fatalf("verdict = %+v, want draw 1:1", v)
	}
}

func TestCompareMogTotalGiftsDoNotBreakDraw(t *testing.T) {
	a := botdata.MogStats{Stars: 100, Gifts: botdata.GiftCounts{Total: 0, BlackBackdrop: 1}}
	b := botdata.MogStats{Stars: 100, Gifts: botdata.GiftCounts{Total: 100, BlackBackdrop: 1}}
	v := compareMog(a, b)
	if v.Winner != 0 || v.AWins != 0 || v.BWins != 0 {
		t.Fatalf("verdict = %+v, want 0:0 draw", v)
	}
}

func TestCompareMogAllEqual(t *testing.T) {
	a := botdata.MogStats{Stars: 7, Gifts: botdata.GiftCounts{Total: 2, BlackBackdrop: 1}}
	v := compareMog(a, a)
	if v.Winner != 0 || v.AWins != 0 || v.BWins != 0 {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestWhyTextNamesCategories(t *testing.T) {
	ch := &challenge{ChallengerTag: "@a", TargetTag: "@b"}
	a := botdata.MogStats{Stars: 1000, Gifts: botdata.GiftCounts{BlackBackdrop: 2}}
	b := botdata.MogStats{Stars: 500, Gifts: botdata.GiftCounts{BlackBackdrop: 0}}
	got := whyText(compareMog(a, b), ch)
	for _, part := range []string{"@a", "звёздам", "подаркам на чёрном фоне"} {
		if !strings.Contains(got, part) {
			t.Fatalf("whyText = %q, missing %q", got, part)
		}
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
	} {
		if got := formatCount(tc.in); got != tc.want {
			t.Errorf("formatCount(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
