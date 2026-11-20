package domain

import (
	"math"
	"testing"
)

// exactSlotRTP enumerates every 5-reel outcome (weighted) and returns the
// expected multiplier, i.e. the slot RTP. It is the reference the paytable must
// satisfy; TestSlotRTP pins it to 95%.
func exactSlotRTP() (total float64, byCount [6]float64) {
	weights := [8]float64{}
	var weightSum float64
	for s, w := range slotWeight {
		weights[s] = float64(w)
		weightSum += float64(w)
	}

	var reels [SlotReels]SlotSymbol
	var rec func(pos int, prob float64)
	rec = func(pos int, prob float64) {
		if pos == SlotReels {
			total += prob * slotMultiplier(reels)
			count, _ := slotLine(reels)
			if count < len(byCount) {
				byCount[count] += prob * slotMultiplier(reels)
			}
			return
		}
		for s := 0; s < 8; s++ {
			reels[pos] = SlotSymbol(s)
			rec(pos+1, prob*weights[s]/weightSum)
		}
	}
	rec(0, 1)
	return total, byCount
}

func TestSlotRTP(t *testing.T) {
	rtp, byCount := exactSlotRTP()
	t.Logf("slot RTP = %.6f", rtp)
	t.Logf("by match length: 2=%.4f 3=%.4f 4=%.4f 5=%.4f", byCount[2], byCount[3], byCount[4], byCount[5])
	if math.Abs(rtp-0.95) > 0.002 {
		t.Fatalf("slot RTP = %.6f, want 0.95 (+-0.002)", rtp)
	}
}

func TestSlotLineWinning(t *testing.T) {
	// Wild must substitute: three sevens followed by two wilds is five sevens.
	reels := [SlotReels]SlotSymbol{SlotSeven, SlotSeven, SlotSeven, SlotWild, SlotWild}
	count, sym := slotLine(reels)
	if count != 5 || sym != SlotSeven {
		t.Fatalf("line = (%d, %v), want (5, Seven)", count, sym)
	}
	if m := slotMultiplier(reels); m != slotFiveKind[SlotSeven] {
		t.Fatalf("multiplier = %v, want %v", m, slotFiveKind[SlotSeven])
	}
}

func TestSlotNoMatch(t *testing.T) {
	reels := [SlotReels]SlotSymbol{SlotCherry, SlotLemon, SlotGrape, SlotOrange, SlotBell}
	if m := slotMultiplier(reels); m != 0 {
		t.Fatalf("non-matching spin paid %v", m)
	}
}
