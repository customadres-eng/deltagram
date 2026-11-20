package domain

import (
	"crypto/rand"
	"testing"
)

// TestGatesRTPMonteCarlo asserts the scatter-pays slot returns ~95% over many
// spins. The exact expectation over all 30-cell outcomes is intractable, so the
// engine is validated statistically with a fixed sample count and a tight but
// achievable tolerance band.
func TestGatesRTPMonteCarlo(t *testing.T) {
	const n = 200000
	var total float64
	for i := 0; i < n; i++ {
		var seed ServerSeed
		if _, err := rand.Read(seed[:]); err != nil {
			t.Fatal(err)
		}
		rng := NewRNG(seed, "mc")
		total += rng.GatesSpin().Multiplier
	}
	rtp := total / float64(n)
	t.Logf("gates RTP over %d spins: %.4f", n, rtp)
	if rtp < 0.92 || rtp > 0.98 {
		t.Fatalf("gates RTP %.4f outside [0.92, 0.98]", rtp)
	}
}
