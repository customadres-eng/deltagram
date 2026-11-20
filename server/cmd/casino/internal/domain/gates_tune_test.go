package domain

import (
	"crypto/rand"
	"fmt"
	"testing"
)

func TestGatesTune(t *testing.T) {
	const n = 400000
	var base, orb, mult float64
	var orbSeen, spinsWithOrb, spinsWithWin float64
	var maxMult float64
	for i := 0; i < n; i++ {
		var seed ServerSeed
		_, _ = rand.Read(seed[:])
		r := NewRNG(seed, "t").GatesSpin()
		var b float64
		for _, tm := range r.Tumbles {
			b += tm.Win
		}
		base += b
		orb += float64(r.OrbTotal)
		mult += r.Multiplier
		if r.OrbTotal > 1 {
			spinsWithOrb++
			orbSeen += float64(r.OrbTotal)
		}
		if b > 0 {
			spinsWithWin++
		}
		if r.Multiplier > maxMult {
			maxMult = r.Multiplier
		}
	}
	fmt.Printf("base RTP      = %.4f\n", base/float64(n))
	fmt.Printf("orb E[orb]    = %.4f (avg over spins; cond on orb=%.2f, P(orb)=%.4f)\n", orb/float64(n), orbSeen/spinsWithOrb, spinsWithOrb/float64(n))
	fmt.Printf("total RTP     = %.4f\n", mult/float64(n))
	fmt.Printf("P(win)        = %.4f\n", spinsWithWin/float64(n))
	fmt.Printf("max mult      = %.2f\n", maxMult)
}
