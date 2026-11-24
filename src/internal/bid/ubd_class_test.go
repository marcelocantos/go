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

// TestUBDClass checks the UBD predicates of IEEE 754-2019 clause 5.7.2
// against the BID ones on the BID encoding each UBD pattern comes from:
// every value at the edge of the subnormal range, at every exponent from
// MinExp128 to just above Emin128, and random values of every length and
// exponent, specials and non-canonical encodings. The class is checked
// against the one the BID predicates give.
func TestUBDClass(t *testing.T) {
	var cases []bid.Uint128
	add := func(neg bool, c *big.Int, exp int) {
		lo := new(big.Int).And(c, new(big.Int).SetUint64(^uint64(0))).Uint64()
		hi := new(big.Int).Rsh(c, 64).Uint64()
		x, ok := bid.Pack128(neg, bid.Uint128{Hi: hi, Lo: lo}, exp)
		if !ok {
			t.Fatalf("Pack128(%v, %v, %d) failed", neg, c, exp)
		}
		cases = append(cases, x)
	}
	ten := big.NewInt(10)
	one := big.NewInt(1)
	maxCoeff := new(big.Int).Sub(new(big.Int).Exp(ten, big.NewInt(bid.Precision128), nil), one)
	for e := bid.MinExp128; e <= bid.Emin128+1; e++ {
		for _, neg := range []bool{false, true} {
			add(neg, big.NewInt(0), e)
			add(neg, one, e)
			add(neg, maxCoeff, e)
			if k := bid.Emin128 - e; k >= 1 {
				p := new(big.Int).Exp(ten, big.NewInt(int64(k)), nil)
				add(neg, p, e)                        // 10^Emin128: normal
				add(neg, new(big.Int).Sub(p, one), e) // just below: subnormal
			}
		}
	}
	r := rand.New(rand.NewPCG(41, 5072))
	for range 200000 {
		var e int
		if r.IntN(2) == 0 {
			e = bid.MinExp128 + r.IntN(bid.Precision128+2) // the subnormal edge
		} else {
			e = bid.MinExp128 + r.IntN(bid.MaxExp128-bid.MinExp128+1)
		}
		add(r.IntN(2) == 0, randDigits(r, 1+r.IntN(bid.Precision128)), e)
	}
	for _, s := range []bid.Uint128{
		{Hi: bid.Inf128Hi},
		{Hi: bid.NaN128Hi},
		{Hi: bid.NaN128Hi, Lo: 42},
		{Hi: bid.NaN128Hi | bid.SignalingBit64},
		{Hi: bid.NaN128Hi | bid.SignalingBit64, Lo: 42},
		{Hi: 0x6000000000000000, Lo: 7},                  // second form: non-canonical, zero
		{Hi: 0x0001ed09bead87c0, Lo: 0x378d8e6400000000}, // coefficient 10^34: non-canonical, zero
	} {
		cases = append(cases, s, bid.Neg128(s))
	}

	for _, b := range cases {
		w := bid.UBDFromBID128(b)
		u := bid.UBD128{Lo: w.Lo, Hi: w.Hi}
		finite, zero, sub := bid.IsFinite128(b), bid.IsZero128(b), bid.IsSubnormal128(b)
		if got := bid.UBDIsFinite128(u); got != finite {
			t.Errorf("UBDIsFinite128(%#x) = %v, want %v (BID %#x)", w, got, finite, b)
		}
		if got := bid.UBDIsZero128(u); got != zero {
			t.Errorf("UBDIsZero128(%#x) = %v, want %v (BID %#x)", w, got, zero, b)
		}
		if got := bid.UBDIsSubnormal128(u); got != sub {
			t.Errorf("UBDIsSubnormal128(%#x) = %v, want %v (BID %#x)", w, got, sub, b)
		}
		if got, want := bid.UBDIsNormal128(u), finite && !zero && !sub; got != want {
			t.Errorf("UBDIsNormal128(%#x) = %v, want %v (BID %#x)", w, got, want, b)
		}
		if !bid.UBDIsCanonical128(u) {
			t.Errorf("UBDIsCanonical128(%#x) = false", w)
		}
		if got, want := bid.UBDClass128(u), bidClass128(b); got != want {
			t.Errorf("UBDClass128(%#x) = %d, want %d (BID %#x)", w, got, want, b)
		}
	}
}

// bidClass128 is the class of the BID encoding x from the BID predicates.
func bidClass128(x bid.Uint128) int {
	neg := x.Hi&bid.SignMask64 != 0
	pick := func(pos, negClass int) int {
		if neg {
			return negClass
		}
		return pos
	}
	switch {
	case bid.IsSignaling128(x):
		return bid.ClassSignalingNaN
	case bid.IsNaN128(x):
		return bid.ClassQuietNaN
	case bid.IsInf128(x):
		return pick(bid.ClassPositiveInfinity, bid.ClassNegativeInfinity)
	case bid.IsZero128(x):
		return pick(bid.ClassPositiveZero, bid.ClassNegativeZero)
	case bid.IsSubnormal128(x):
		return pick(bid.ClassPositiveSubnormal, bid.ClassNegativeSubnormal)
	}
	return pick(bid.ClassPositiveNormal, bid.ClassNegativeNormal)
}
