// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"fmt"
	"internal/bid"
	"math/rand/v2"
	"testing"
)

// The UBD memory form of decimal128 and the kernel entries the compiler
// calls on it (see "The UBD kernel contract" in ubd.go). TestUBDConversion
// checks the two conversions against the BID decoding; TestUBDKernel checks
// every rule of the contract for every entry, over every pair of a pool of
// UBD operands, in every mode. The reference is the exported BID function on
// the operands' BID encodings, so the test holds whatever the entries are
// built on: today the BID kernel behind a conversion, later a kernel that
// works on UBD itself.

// ubd128 returns the UBD pattern with qExp e (its low 14 bits), the sign and
// the coefficient words (the high word's low 49 bits).
func ubd128(neg bool, e int, chi, clo uint64) bid.Uint128 {
	x := bid.Uint128{Hi: uint64(e)<<bid.UBDExpShift | chi&bid.UBDCoeffMaskHi, Lo: clo}
	if neg {
		x.Hi |= bid.UBDSignHi
	}
	return x
}

// ubdNaN128 returns the UBD NaN with the tagged high word tag (UBDNaNHi or
// UBDSNaNHi, with or without the sign) and the payload p, which may be out
// of range: its top two bits of 46 go in qExp, the rest in the coefficient.
func ubdNaN128(tag uint64, p bid.Uint128) bid.Uint128 {
	return bid.Uint128{Hi: tag | p.Hi>>44&3<<bid.UBDExpShift | p.Hi&(1<<44-1), Lo: p.Lo}
}

// ubdCanonical128 reports whether x is a canonical UBD encoding, one that the
// conversion of some BID encoding produces.
func ubdCanonical128(x bid.Uint128) bool {
	return bid.UBDFromBID128(bid.BIDFromUBD128(x)) == x
}

// ubdWellFormed128 is the test's own statement of a well-formed UBD pattern
// (bid.UBDWellFormed128), from the layout: a qExp in the format, or the
// special tag (qExp 7168 and up) with 0x1F in the top five coefficient bits.
func ubdWellFormed128(x bid.Uint128) bool {
	qExp := int(int64(x.Hi) >> 50)
	if qExp >= 7168 {
		return x.Hi>>44&0x1F == 0x1F
	}
	return -6176 <= qExp && qExp <= 6111
}

// mustWellFormed128 fails the test at once on a malformed UBD pattern, which
// can only come from a bug: no input of these tests is malformed, so no
// result of the code under test may be.
func mustWellFormed128(t *testing.T, what string, x bid.Uint128) {
	t.Helper()
	if !ubdWellFormed128(x) {
		t.Fatalf("%s: malformed UBD pattern %#x", what, x)
	}
}

// ubdPool128 returns the UBD operands of TestUBDKernel: the conversions of
// inlinePool128, which straddles the BID kernel's bounds; finite patterns at
// the ends of the format's exponents and of the coefficient field; specials
// canonical and not; and random values. All are well-formed: a malformed
// pattern can only come from a bug, and the contract leaves its results
// unspecified (see "The UBD kernel contract" in ubd.go).
func ubdPool128(t *testing.T) []bid.Uint128 {
	var pool []bid.Uint128
	for _, x := range inlinePool128() {
		pool = append(pool, bid.UBDFromBID128(x))
	}

	type c struct{ hi, lo uint64 }
	coeffs := []c{{0, 0}, {0, 1}, {0, 1<<64 - 1}, {1, 0},
		{1<<47 - 1, 1<<64 - 1},                 // two of these sum, with the carry, to 2^112 - 1: the add fast path's bound
		{1 << 47, 0},                           // two of these sum to 2^112, just past it
		{1 << 48, 0},                           // 2^112: the top coefficient bit alone
		{0, 1 << 56},                           // squared, 2^112: just past the multiply fast path's bound
		{0, 1<<56 - 1},                         // squared, or times 1<<56, just below it
		{0, 1<<50 - 1},                         // times 1<<63, about 1.04e34: past 10^34 and below 2^113
		{bid.MaxCoeff128Hi, bid.MaxCoeff128Lo}, // the largest coefficient
		{bid.MaxCoeff128Hi, bid.MaxCoeff128Lo + 1}, // 10^34, which reads as zero
		{bid.UBDCoeffMaskHi, 1<<64 - 1}}            // the largest the field holds
	// qExp: the ends of the format and their neighbours inside it, and
	// zero and its neighbours.
	exps := []int{bid.MinExp128, bid.MinExp128 + 1, -1, 0, 1, bid.MaxExp128 - 1, bid.MaxExp128}
	for _, neg := range []bool{false, true} {
		for _, e := range exps {
			for _, c := range coeffs {
				pool = append(pool, ubd128(neg, e, c.hi, c.lo))
			}
		}
	}

	maxPayload := bid.Uint128{Hi: bid.MaxPayload128Hi, Lo: bid.MaxPayload128Lo}
	pool = append(pool,
		bid.Uint128{Hi: bid.UBDInfHi},
		bid.Uint128{Hi: bid.UBDInfHi | bid.UBDSignHi},
		bid.Uint128{Hi: bid.UBDInfHi | 1, Lo: 5}, // an infinity with coefficient bits
		bid.Uint128{Hi: bid.UBDInfHi | 0x3F<<52}, // an infinity with the unused qExp bits set
		bid.Uint128{Hi: bid.UBDNaNHi},
		bid.Uint128{Hi: bid.UBDNaNHi | bid.UBDSignHi, Lo: 7},
		bid.Uint128{Hi: bid.UBDSNaNHi, Lo: 42},
		bid.Uint128{Hi: bid.UBDSNaNHi | bid.UBDSignHi, Lo: 9},
		ubdNaN128(bid.UBDNaNHi, maxPayload),                                             // payload 10^33 - 1
		ubdNaN128(bid.UBDSNaNHi, bid.Uint128{Hi: maxPayload.Hi, Lo: maxPayload.Lo + 1}), // 10^33, a zero payload
		ubdNaN128(bid.UBDNaNHi|bid.UBDSignHi, bid.Uint128{Hi: 1<<46 - 1, Lo: 1<<64 - 1}),
	)

	r := rand.New(rand.NewPCG(1, 128))
	for range 32 {
		// A canonical finite value, half of them at exponents near zero,
		// where operands interact.
		hi, lo := r.Uint64N(bid.MaxCoeff128Hi+1), r.Uint64()
		if hi == bid.MaxCoeff128Hi && lo > bid.MaxCoeff128Lo {
			lo = bid.MaxCoeff128Lo
		}
		switch r.IntN(3) {
		case 0: // a word
			hi = 0
		case 1: // a few digits
			hi, lo = 0, r.Uint64N(1e6)
		}
		e := r.IntN(41) - 20
		if r.IntN(2) == 0 {
			e = bid.MinExp128 + r.IntN(bid.MaxExp128-bid.MinExp128+1)
		}
		pool = append(pool, ubd128(r.IntN(2) == 0, e, hi, lo))
	}
	for range 8 {
		// Random coefficient bits, often above MaxCoeff128, at a random
		// exponent of the format.
		e := bid.MinExp128 + r.IntN(bid.MaxExp128-bid.MinExp128+1)
		pool = append(pool, ubd128(r.IntN(2) == 0, e, r.Uint64(), r.Uint64()))
	}
	for _, x := range pool {
		mustWellFormed128(t, "ubdPool128", x)
	}
	return pool
}

// TestUBDWellFormed checks bid.UBDWellFormed128 against the test's own
// statement at every boundary of the qExp field and of the special prefix.
func TestUBDWellFormed(t *testing.T) {
	var exps []int
	for _, e := range []int{-8192, -6177, -6176, 6111, 6112, 7167, 7168, 7679, 7680, 8191} {
		exps = append(exps, e-1, e, e+1)
	}
	for _, e := range exps {
		for _, c := range []uint64{0, 1, 0x1E << 44, 0x1F << 44, 0x1F<<44 - 1, bid.UBDCoeffMaskHi} {
			for _, neg := range []bool{false, true} {
				x := ubd128(neg, e, c, 7)
				if got, want := bid.UBDWellFormed128(x), ubdWellFormed128(x); got != want {
					t.Errorf("UBDWellFormed128(%#x) (qExp %d) = %v, want %v", x, e, got, want)
				}
			}
		}
	}
}

func TestUBDConversion(t *testing.T) {
	// All bits clear is +0E+0.
	if got, want := bid.BIDFromUBD128(bid.Uint128{}), (bid.Uint128{Hi: bid.Zero128Hi}); got != want {
		t.Errorf("BIDFromUBD128(0) = %#x, want +0E+0 %#x", got, want)
	}
	for _, x := range inlinePool128() {
		u := bid.UBDFromBID128(x)
		mustWellFormed128(t, fmt.Sprintf("UBDFromBID128(%#x)", x), u)
		if !ubdCanonical128(u) {
			t.Errorf("UBDFromBID128(%#x) = %#x, not canonical", x, u)
		}
		if bid.IsCanonical128(x) {
			if got := bid.BIDFromUBD128(u); got != x {
				t.Errorf("BIDFromUBD128(UBDFromBID128(%#x)) = %#x, want it back", x, got)
			}
		}
		kind, neg, coeff, exp := bid.Unpack128(x)
		if kind != bid.KindFinite {
			continue
		}
		gotNeg := u.Hi&bid.UBDSignHi != 0
		gotExp := int(int64(u.Hi) >> bid.UBDExpShift)
		gotCoeff := bid.Uint128{Hi: u.Hi & bid.UBDCoeffMaskHi, Lo: u.Lo}
		if gotNeg != neg || gotExp != exp || gotCoeff != coeff {
			t.Errorf("UBDFromBID128(%#x) = %#x: sign %v, qExp %d, coefficient %#x; want %v, %d, %#x",
				x, u, gotNeg, gotExp, gotCoeff, neg, exp, coeff)
		}
	}
}

func TestUBDKernel(t *testing.T) {
	pool := ubdPool128(t)
	// The six modes and two outside them, which behave as ToNearestEven.
	modes := append(append([]bid.Mode{}, inlineModes...), 6, 255)
	if testing.Short() {
		modes = []bid.Mode{bid.ToNearestEven, bid.TowardNegative, 6}
	}

	// Each rounding operation has a TTE entry, checked at ToNearestEven,
	// and an Rnd entry, checked in every mode.
	type tte = func(x, y bid.Uint128) bid.Uint128
	type rnd = func(x, y bid.Uint128, mode bid.Mode) bid.Uint128
	ops := []struct {
		name     string
		tte      tte
		rnd, ref rnd // ref on BID encodings
	}{
		{"Add", bid.UBDAddTTE128, bid.UBDAddRnd128, bid.Add128},
		{"Sub", bid.UBDSubTTE128, bid.UBDSubRnd128, bid.Sub128},
		{"Mul", bid.UBDMulTTE128, bid.UBDMulRnd128, bid.Mul128},
		{"Div", bid.UBDDivTTE128, bid.UBDDivRnd128, bid.Div128},
		{"Quantize", bid.UBDQuantizeTTE128, bid.UBDQuantizeRnd128, bid.Quantize128},
	}

	errs := 0
	fail := func(format string, args ...any) {
		t.Helper()
		if errs++; errs <= 20 {
			t.Errorf(format, args...)
		} else if errs == 21 {
			t.Errorf("too many errors")
		}
	}
	check := func(name string, x, y bid.Uint128, mode bid.Mode, got, want bid.Uint128) {
		t.Helper()
		mustWellFormed128(t, name, got)
		if got != want {
			fail("%s(%#x, %#x, %d) = %#x, want %#x", name, x, y, mode, got, want)
		} else if !ubdCanonical128(got) {
			fail("%s(%#x, %#x, %d) = %#x, not canonical", name, x, y, mode, got)
		}
	}

	for _, x := range pool {
		bx := bid.BIDFromUBD128(x)
		check("ubdRoundToIntegralTTE128", x, bid.Uint128{}, bid.ToNearestEven,
			bid.UBDRoundToIntegralTTE128(x), bid.UBDFromBID128(bid.RoundToIntegral128(bx, bid.ToNearestEven)))
		for _, mode := range modes {
			check("ubdRoundToIntegralRnd128", x, bid.Uint128{}, mode,
				bid.UBDRoundToIntegralRnd128(x, mode), bid.UBDFromBID128(bid.RoundToIntegral128(bx, mode)))
		}
		for _, y := range pool {
			by := bid.BIDFromUBD128(y)
			c, ok := bid.UBDCompare128(x, y)
			if wc, wok := bid.Compare128(bx, by); c != wc || ok != wok {
				fail("ubdCompare128(%#x, %#x) = %d, %v; want %d, %v", x, y, c, ok, wc, wok)
			}
			for _, op := range ops {
				check("ubd"+op.name+"TTE128", x, y, bid.ToNearestEven, op.tte(x, y), bid.UBDFromBID128(op.ref(bx, by, bid.ToNearestEven)))
				for _, mode := range modes {
					check("ubd"+op.name+"Rnd128", x, y, mode, op.rnd(x, y, mode), bid.UBDFromBID128(op.ref(bx, by, mode)))
				}
			}
		}
	}
	if errs == 0 {
		t.Logf("%d operands, %d pairs, %d modes", len(pool), len(pool)*len(pool), len(modes))
	}
}
