package domain

import (
	"math"
	"testing"
)

// deterministic RNG over many outcomes: a fixed seed, a client seed, and we
// keep calling into one RNG so the nonce advances naturally.
func testRNG(t *testing.T) *RNG {
	t.Helper()
	return NewRNG([32]byte{1}, "test")
}

func TestRNGSameSeedsSameSequence(t *testing.T) {
	a := NewRNG([32]byte{7}, "client")
	b := NewRNG([32]byte{7}, "client")
	for i := 0; i < 1000; i++ {
		if a.Uint64() != b.Uint64() {
			t.Fatalf("sequence diverged at %d", i)
		}
	}
}

func TestRNGDifferentSeedsDiffer(t *testing.T) {
	a := NewRNG([32]byte{7}, "client")
	b := NewRNG([32]byte{8}, "client")
	same := 0
	for i := 0; i < 100; i++ {
		if a.Uint64() == b.Uint64() {
			same++
		}
	}
	if same > 0 {
		t.Fatalf("different seeds produced %d identical words", same)
	}
}

func TestRNGUniformRange(t *testing.T) {
	r := testRNG(t)
	for i := 0; i < 100000; i++ {
		u := r.Uniform()
		if u < 0 || u >= 1 {
			t.Fatalf("uniform out of range: %v", u)
		}
	}
}

func TestRNGIntNBoundsAndCoverage(t *testing.T) {
	r := testRNG(t)
	for n := 1; n <= 64; n++ {
		seen := make([]bool, n)
		for i := 0; i < n*200; i++ {
			v := r.IntN(n)
			if v < 0 || v >= n {
				t.Fatalf("IntN(%d) out of range: %d", n, v)
			}
			seen[v] = true
		}
		for v, hit := range seen {
			if !hit {
				t.Fatalf("IntN(%d) never produced %d in %d draws", n, v, n*200)
			}
		}
	}
}

func TestRNGNonceAdvances(t *testing.T) {
	r := testRNG(t)
	start := r.Nonce()
	_, _ = r.Uniform(), r.Uniform()
	if r.Nonce() != start+2 {
		t.Fatalf("nonce = %d, want %d", r.Nonce(), start+2)
	}
}

func TestCrashNeverBelowOne(t *testing.T) {
	r := testRNG(t)
	for i := 0; i < 100000; i++ {
		if c := r.Crash(); c < 1 {
			t.Fatalf("crash below 1: %v", c)
		}
	}
}

func TestCrashInstantCrashFrequency(t *testing.T) {
	// crash < some small epsilon means an instant (1.00x) crash, which is the
	// 5% house edge. Count 1.00x outcomes over a large sample.
	const n = 200_000
	r := testRNG(t)
	instant := 0
	for i := 0; i < n; i++ {
		if r.Crash() == 1 {
			instant++
		}
	}
	rate := float64(instant) / n
	if rate < 0.04 || rate > 0.06 {
		t.Fatalf("instant-crash rate = %.4f, want ~0.05", rate)
	}
}

func TestCrashRTP(t *testing.T) {
	// A player cashing out at a fixed target m wins iff crash >= m. Empirically
	// that probability times m should be the RTP (0.95) for m in (1, ~19).
	const n = 200_000
	for _, target := range []float64{1.5, 2, 3, 5, 10, 15} {
		r := testRNG(t)
		wins := 0
		for i := 0; i < n; i++ {
			if r.Crash() >= target {
				wins++
			}
		}
		rtp := float64(wins) / n * target
		if math.Abs(rtp-0.95) > 0.015 {
			t.Fatalf("target %.1f RTP = %.4f, want ~0.95", target, rtp)
		}
	}
}

func TestMinesBoardHasExactMineCount(t *testing.T) {
	r := testRNG(t)
	for _, m := range []int{1, 2, 3, 5, 10, 24} {
		b := r.NewMinesBoard(5, m)
		mines := 0
		for _, cell := range b.Cells {
			if cell {
				mines++
			}
		}
		if mines != m {
			t.Fatalf("board has %d mines, want %d", mines, m)
		}
		if len(b.Cells) != 25 {
			t.Fatalf("board has %d cells, want 25", len(b.Cells))
		}
	}
}

func TestMinesMultiplierMonotonic(t *testing.T) {
	prev := 0.0
	for k := 0; k <= 20; k++ {
		m := MinesMultiplier(5, 4, k)
		if m <= prev && k > 0 {
			t.Fatalf("multiplier not increasing at k=%d: %.4f -> %.4f", k, prev, m)
		}
		prev = m
	}
}

func TestMinesRTP(t *testing.T) {
	// Revealing exactly k safe cells then cashing out must return 95% on
	// average, for any k. We simulate whole boards and a k-safe-reveal strategy.
	const n = 100_000
	for _, k := range []int{1, 2, 3, 5, 8} {
		r := testRNG(t)
		acc := 0.0
		for i := 0; i < n; i++ {
			b := r.NewMinesBoard(5, 4)
			// Reveal cells in a fixed order; stop (lose) on the first mine.
			safe := 0
			for c := 0; c < 25 && safe < k; c++ {
				if b.Cells[c] {
					safe = -1
					break
				}
				safe++
			}
			if safe == k {
				acc += MinesMultiplier(5, 4, k)
			}
		}
		rtp := acc / n
		if math.Abs(rtp-0.95) > 0.03 {
			t.Fatalf("mines k=%d RTP = %.4f, want ~0.95", k, rtp)
		}
	}
}

func TestUpgradeChanceTimesMultiplierEqualsRTP(t *testing.T) {
	for _, m := range []float64{1.05, 1.2, 1.5, 2, 3, 5, 10, 50} {
		c := UpgradeChance(m)
		if got := c * m; math.Abs(got-0.95) > 1e-9 {
			t.Fatalf("chance(%.2f)*multiplier = %.6f, want 0.95", m, got)
		}
	}
}

func TestUpgradeRollDeterminism(t *testing.T) {
	if UpgradeRoll(0.049, 0.05) == false {
		t.Fatal("u < chance must win")
	}
	if UpgradeRoll(0.051, 0.05) == true {
		t.Fatal("u >= chance must lose")
	}
}

func TestUpgradeRTP(t *testing.T) {
	const n = 300_000
	r := testRNG(t)
	chance := UpgradeChance(3) // ~0.3167
	wins := 0
	for i := 0; i < n; i++ {
		if r.Upgrade(chance) {
			wins++
		}
	}
	rtp := float64(wins) / n * 3
	if math.Abs(rtp-0.95) > 0.015 {
		t.Fatalf("upgrade RTP = %.4f, want ~0.95", rtp)
	}
}

func TestPayoutRounding(t *testing.T) {
	cases := []struct {
		stake int64
		mult  float64
		want  int64
	}{
		{100, 1.9, 190},
		{100, 0.5, 50},
		{100, 1, 100},
		{1, 1.5, 2}, // rounds half up
		{3, 1.5, 5}, // 4.5 -> 5
		{100, 0, 0},
		{100, -1, 0},
	}
	for _, c := range cases {
		if got := Payout(c.stake, c.mult); got != c.want {
			t.Fatalf("Payout(%d, %.1f) = %d, want %d", c.stake, c.mult, got, c.want)
		}
	}
}
