package domain

import (
	"math"
	"time"
)

// Crash is the "rocket" game: the multiplier climbs until it crashes at a
// hidden point, and the player must cash out before that point to win.
//
// RTP 95%: the crash point is crash = (1 - edge) / u for u uniform in [0,1),
// clamped to [1, max]. A player who always cashes out at a fixed target m wins
// exactly when crash >= m, i.e. when u <= (1-edge)/m, with probability
// (1-edge)/m; their expected payout is therefore m * (1-edge)/m = 1-edge = 95%
// of the stake for every m in (1, max). The 5% house edge materialises as the
// 5% of rounds that "crash" immediately at 1.00x.

const (
	// crashHouseEdge is the fraction of each stake the house keeps on average.
	crashHouseEdge = 0.05
	// defaultCrashMax caps the multiplier so a single round cannot pay out
	// arbitrarily. The casino may override it per round.
	defaultCrashMax = 100.0

	// CrashCurveK is the exponential growth rate of the live multiplier,
	// m(t) = exp(CrashCurveK * t) with t in seconds. The Mini App animates the
	// exact same curve, so the multiplier a player sees equals the server's.
	// ×2 in ~2.5s, ×10 in ~8.2s, ×100 in ~16.4s — a snappy, readable round.
	CrashCurveK = 0.28
)

// CrashMultiplierAt returns the live crash multiplier after elapsed time.
// It is the single source of truth shared by the server and the client.
func CrashMultiplierAt(elapsed time.Duration) float64 {
	if elapsed < 0 {
		elapsed = 0
	}
	m := math.Exp(CrashCurveK * elapsed.Seconds())
	if m > defaultCrashMax {
		m = defaultCrashMax
	}
	return m
}

// CrashPoint computes the crash multiplier from a uniform draw u in [0,1).
// It is deterministic, so the same u always yields the same point.
func CrashPoint(u float64) float64 {
	return crashPoint(u, defaultCrashMax)
}

// CrashPointMax is CrashPoint with an explicit multiplier ceiling.
func CrashPointMax(u, max float64) float64 {
	if max <= 1 {
		max = defaultCrashMax
	}
	return crashPoint(u, max)
}

func crashPoint(u, max float64) float64 {
	p := (1 - crashHouseEdge) / u
	if p < 1 {
		return 1
	}
	if p > max {
		return max
	}
	return p
}

// Crash draws the next crash point from the RNG.
func (r *RNG) Crash() float64 { return CrashPoint(r.Uniform()) }

// CrashMax draws the next crash point with an explicit ceiling.
func (r *RNG) CrashMax(max float64) float64 { return CrashPointMax(r.Uniform(), max) }

// Payout rounds a stake (in the casino's integer unit, "cents") up by a
// multiplier and returns the integer payout. It is the single place where a
// float multiplier turns into real money, so rounding is consistent everywhere.
func Payout(stake int64, multiplier float64) int64 {
	if multiplier <= 0 {
		return 0
	}
	p := float64(stake) * multiplier
	if p >= float64(1<<62) {
		return 1<<62 - 1
	}
	// Round half away from zero; payouts are always positive.
	if p-float64(int64(p)) >= 0.5 {
		return int64(p) + 1
	}
	return int64(p)
}
