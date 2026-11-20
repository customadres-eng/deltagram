package domain

// Slots is a classic 5-reel, left-to-right slot with a wild symbol. The three
// visible rows on each reel show the same weighted symbol, so the three
// horizontal paylines are identical and the per-line RTP equals the game RTP.
//
// RTP is pinned to 95% by the paytable: the multipliers below are chosen so
// that the exact weighted expectation over all 5-reel outcomes equals 0.95.
// slots_test.go enumerates every outcome and asserts this, so any change to the
// weights or the paytable that drifts the RTP fails the build.

// SlotSymbol enumerates the reel symbols. The zero value is Cherry.
type SlotSymbol uint8

const (
	SlotCherry SlotSymbol = iota
	SlotLemon
	SlotGrape
	SlotOrange
	SlotBell
	SlotStar
	SlotSeven
	SlotWild
)

// slotWeight maps each symbol to its reel weight. Higher weight == more common.
var slotWeight = [8]uint64{
	SlotCherry: 6,
	SlotLemon:  4,
	SlotGrape:  4,
	SlotOrange: 3,
	SlotBell:   3,
	SlotStar:   2,
	SlotSeven:  1,
	SlotWild:   1,
}

// slotWeightTotal is the sum of slotWeight, precomputed for clarity.
var slotWeightTotal = uint64(24)

// SlotReels is the fixed reel count.
const SlotReels = 5

// SlotSpin is one resolved spin: the drawn symbols and the total payout
// multiplier (0 when nothing hits).
type SlotSpin struct {
	Reels      [SlotReels]SlotSymbol
	Multiplier float64
}

// Spin resolves one slot spin from the RNG.
func (r *RNG) Spin() SlotSpin {
	var s SlotSpin
	for i := 0; i < SlotReels; i++ {
		s.Reels[i] = r.drawSlotSymbol()
	}
	s.Multiplier = slotMultiplier(s.Reels)
	return s
}

func (r *RNG) drawSlotSymbol() SlotSymbol {
	roll := uint64(r.IntN(int(slotWeightTotal)))
	for sym, w := range slotWeight {
		if roll < w {
			return SlotSymbol(sym)
		}
		roll -= w
	}
	return SlotCherry // unreachable
}

// slotMultiplier returns the payout multiplier for one payline. Matching is
// left-to-right: a win of length k means reels 0..k-1 all show the same symbol
// (or a wild), with the winning symbol taken from the first non-wild reel.
func slotMultiplier(reels [SlotReels]SlotSymbol) float64 {
	count, sym := slotLine(reels)
	switch {
	case count >= 5:
		return slotFiveKind[sym]
	case count == 4:
		return slotFourKind
	case count == 3:
		return slotThreeKind
	case count == 2 && sym == SlotCherry:
		return slotCherryPair
	default:
		return 0
	}
}

// slotLine returns the matched length and winning symbol for one payline.
func slotLine(reels [SlotReels]SlotSymbol) (count int, sym SlotSymbol) {
	sym = SlotWild
	for _, s := range reels {
		if s != SlotWild {
			sym = s
			break
		}
	}
	// All-wild reels pay as the top symbol.
	if sym == SlotWild {
		sym = SlotSeven
	}
	for _, s := range reels {
		if s == sym || s == SlotWild {
			count++
			continue
		}
		break
	}
	return count, sym
}

// slotFiveKind is the payout for five-of-a-kind, per winning symbol.
var slotFiveKind = [8]float64{
	SlotCherry: 72,
	SlotLemon:  108,
	SlotGrape:  144,
	SlotOrange: 192,
	SlotBell:   240,
	SlotStar:   360,
	SlotSeven:  600,
	SlotWild:   0, // wild cannot be the matched symbol
}

const (
	slotFourKind   = 25
	slotThreeKind  = 8
	slotCherryPair = 1
)
