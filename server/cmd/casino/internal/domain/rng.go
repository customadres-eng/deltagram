// Package domain holds the casino's pure game logic: a provably-fair random
// source plus the four game engines (crash, mines, upgrade, slots). Nothing in
// this package touches the network, the database or the wallet, so every
// outcome can be reproduced and audited from the two seeds alone.
package domain

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/bits"
)

// ServerSeed is the 256-bit secret the casino generates for one player round.
// Only its SHA-256 hash is published before the round begins; the raw value is
// revealed afterwards so the player can prove the casino never re-rolled.
type ServerSeed [32]byte

// NewServerSeed draws a fresh cryptographically-random server seed and returns
// both the seed and its SHA-256 hash (the value to publish up front).
func NewServerSeed() (ServerSeed, [32]byte, error) {
	var s ServerSeed
	if _, err := rand.Read(s[:]); err != nil {
		return s, [32]byte{}, err
	}
	return s, sha256.Sum256(s[:]), nil
}

// Hash returns the SHA-256 hash of the seed, i.e. the pre-round commitment.
func (s ServerSeed) Hash() [32]byte { return sha256.Sum256(s[:]) }

// RNG is a deterministic, provably-fair random source bound to a single round.
// Every outcome is derived from HMAC-SHA256(serverSeed, "clientSeed:nonce"), so
// the casino and the player both reproduce the exact sequence from the seeds.
type RNG struct {
	serverSeed [32]byte
	clientSeed string
	nonce      uint64
}

// NewRNG binds an RNG to a server seed and a client seed.
func NewRNG(serverSeed [32]byte, clientSeed string) *RNG {
	return &RNG{serverSeed: serverSeed, clientSeed: clientSeed}
}

// Nonce reports how many outcomes have been drawn so far.
func (r *RNG) Nonce() uint64 { return r.nonce }

// next derives one 32-byte digest for outcome number r.nonce, then advances.
func (r *RNG) next() [32]byte {
	msg := fmt.Sprintf("%s:%d", r.clientSeed, r.nonce)
	r.nonce++
	mac := hmac.New(sha256.New, r.serverSeed[:])
	_, _ = mac.Write([]byte(msg))
	var out [32]byte
	copy(out[:], mac.Sum(nil))
	return out
}

// Uint64 returns the next 64-bit word from the outcome stream.
func (r *RNG) Uint64() uint64 {
	h := r.next()
	return binary.BigEndian.Uint64(h[:8])
}

// Uniform returns the next float64 in [0,1). It uses the top 53 bits of the
// 64-bit word so every float64 value the range can represent is reachable.
func (r *RNG) Uniform() float64 {
	return float64(r.Uint64()>>11) / (1 << 53)
}

// IntN returns a uniform integer in [0, n). It uses Lemire's multiply-reject
// method (the same algorithm as math/rand/v2), so the result is exactly
// uniform for any n with no modulo bias.
func (r *RNG) IntN(n int) int {
	if n <= 0 {
		panic("domain: IntN requires n > 0")
	}
	if n == 1 {
		return 0
	}
	bn := uint64(n)
	if bn&(bn-1) == 0 { // power of two: a mask is exact
		return int(r.Uint64() & (bn - 1))
	}
	// hi = floor(bn*v / 2^64) is uniform in [0, bn). The quotient is biased only
	// when the low limb falls in the bottom 2^64%bn values, so reject those.
	hi, lo := bits.Mul64(bn, r.Uint64())
	if lo < bn {
		t := (-bn) % bn // == 2^64 % bn
		for lo < t {
			hi, lo = bits.Mul64(bn, r.Uint64())
		}
	}
	return int(hi)
}
