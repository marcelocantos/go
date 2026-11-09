// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math/big"
	"math/rand/v2"
	"testing"
)

// TestDPDSplitBoundaries checks the trailing significand field that
// DPDFromBID128 writes against declets computed from the coefficient's
// digits by math/big, and the round trip, for coefficients on each side of
// the multiples of 10^18 and 10^19 at which the split into digit groups
// changes, at the ends of the coefficient range and at random.
func TestDPDSplitBoundaries(t *testing.T) {
	rng := rand.New(rand.NewPCG(17, 19))
	limit := pow10(bid.Precision128)
	var cs []*big.Int
	around := func(b *big.Int) {
		for d := int64(-2); d <= 2; d++ {
			if c := new(big.Int).Add(b, big.NewInt(d)); c.Sign() >= 0 && c.Cmp(limit) < 0 {
				cs = append(cs, c)
			}
		}
	}
	for _, p := range []int{15, 18, 19, 33} {
		step := pow10(p)
		jmax := new(big.Int).Quo(limit, step)
		js := []*big.Int{big.NewInt(1), big.NewInt(9), big.NewInt(10), jmax, new(big.Int).Sub(jmax, bigOne)}
		for range 200 {
			js = append(js, bigBelow(rng, jmax))
		}
		for _, j := range js {
			around(new(big.Int).Mul(j, step))
		}
	}
	around(limit)
	around(new(big.Int).Lsh(bigOne, 64))
	for range floatToCount(100_000) {
		cs = append(cs, bigBelow(rng, limit))
	}
	thousand := big.NewInt(1000)
	for _, c := range cs {
		x, _ := bid.Pack128(false, toUint128(c), 0)
		d := bid.DPDFromBID128(x)
		var want bid.Uint128 // the 11 declets of the low 33 digits
		rest := new(big.Int).Set(c)
		digit := new(big.Int)
		for i := range 11 {
			rest.QuoRem(rest, thousand, digit)
			dec := uint64(bid.Bin2DPD[digit.Int64()])
			if sh := 10 * i; sh < 64 {
				want.Lo |= dec << sh
				want.Hi |= dec >> (64 - sh)
			} else {
				want.Hi |= dec << (sh - 64)
			}
		}
		got := bid.Uint128{Hi: d.Hi & (1<<46 - 1), Lo: d.Lo}
		if got != want {
			t.Fatalf("DPDFromBID128(%v) trailing field = %#x%016x, want %#x%016x", c, got.Hi, got.Lo, want.Hi, want.Lo)
		}
		if back := bid.BIDFromDPD128(d); back != x {
			t.Fatalf("BIDFromDPD128(DPDFromBID128(%v)) = %#x%016x, want %#x%016x", c, back.Hi, back.Lo, x.Hi, x.Lo)
		}
	}
}

// bigBelow returns a random integer in [0, n), n > 0, near enough uniform
// for a test: 64 more random bits than n has, reduced modulo n.
func bigBelow(rng *rand.Rand, n *big.Int) *big.Int {
	r := new(big.Int)
	for range n.BitLen()/64 + 2 {
		r.Lsh(r, 64)
		r.Or(r, new(big.Int).SetUint64(rng.Uint64()))
	}
	return r.Mod(r, n)
}

// TestDPDDecletPositions decodes every one of the 1024 declets at every
// position of the trailing significand field of both widths, alone in an
// otherwise zero finite encoding with exponent 0 and leading digit 0, and
// checks the coefficient against math/big.
func TestDPDDecletPositions(t *testing.T) {
	thousand := big.NewInt(1000)
	for i := range 11 {
		scale := new(big.Int).Exp(thousand, big.NewInt(int64(i)), nil)
		for d := range uint64(1024) {
			want := new(big.Int).Mul(big.NewInt(int64(bid.DPD2Bin[d])), scale)
			var x bid.Uint128
			if sh := 10 * i; sh < 64 {
				x = bid.Uint128{Hi: d >> (64 - sh), Lo: d << sh}
			} else {
				x = bid.Uint128{Hi: d << (sh - 64)}
			}
			x.Hi |= bid.DPDZero128Hi
			_, _, c, exp := bid.Unpack128(bid.BIDFromDPD128(x))
			if bigOf(c).Cmp(want) != 0 || exp != 0 {
				t.Fatalf("decimal128 declet %#x at position %d: coefficient %v exp %d, want %v exp 0", d, i, bigOf(c), exp, want)
			}
			if i < 5 {
				_, _, c64, exp := bid.Unpack64(bid.BIDFromDPD64(bid.DPDZero64 | d<<(10*i)))
				if c64 != want.Uint64() || exp != 0 {
					t.Fatalf("decimal64 declet %#x at position %d: coefficient %d exp %d, want %v exp 0", d, i, c64, exp, want)
				}
			}
		}
	}
}
