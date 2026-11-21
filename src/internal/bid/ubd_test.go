// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"fmt"
	"internal/bid"
	"math"
	"math/big"
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

// ubdWellFormed128 is the test's own statement of a well-formed (canonical)
// UBD pattern (bid.UBDWellFormed128), from the layout, with qExp e and the
// 113-bit coefficient c: a finite value has e in the format and c below
// 10^34; an infinity has e 7168 and c the 0x1F prefix alone (c = 0x1F<<108);
// a NaN has e 7680 + p (quiet) or 7936 + p (signaling) for p below 4, the
// payload's top two bits, c's top five bits the prefix, and a payload
// p<<108 + c mod 2^108 below 10^33.
func ubdWellFormed128(x bid.Uint128) bool {
	e := int(int64(x.Hi) >> 50)
	c := new(big.Int).Lsh(new(big.Int).SetUint64(x.Hi&(1<<49-1)), 64)
	c.Or(c, new(big.Int).SetUint64(x.Lo))
	prefix := new(big.Int).Lsh(big.NewInt(0x1F), 108)
	switch {
	case -6176 <= e && e <= 6111:
		return c.Cmp(pow10(34)) < 0
	case e == 7168:
		return c.Cmp(prefix) == 0
	case 7680 <= e && e < 7684, 7936 <= e && e < 7940:
		low := new(big.Int).Sub(c, prefix)
		if low.Sign() < 0 || low.BitLen() > 108 {
			return false
		}
		p := new(big.Int).Lsh(big.NewInt(int64(e&3)), 108)
		return p.Add(p, low).Cmp(pow10(33)) < 0
	}
	return false
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
// the ends of the format's exponents and of the coefficient field; specials,
// NaN payloads up to the largest; and random values. All are well-formed: a malformed
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
		{0, 1<<57 - 1},                         // squared, about 2^114: past 10^34 for a multiply bound one bit too wide
		{0, 25},                                // 2.5 at qExp -1 quantized to qExp 0: a tie, which ties to even rounds down
		{bid.MaxCoeff128Hi, bid.MaxCoeff128Lo}} // the largest coefficient
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
		bid.Uint128{Hi: bid.UBDNaNHi},
		bid.Uint128{Hi: bid.UBDNaNHi | bid.UBDSignHi, Lo: 7},
		bid.Uint128{Hi: bid.UBDSNaNHi, Lo: 42},
		bid.Uint128{Hi: bid.UBDSNaNHi | bid.UBDSignHi, Lo: 9},
		ubdNaN128(bid.UBDNaNHi, maxPayload), // payload 10^33 - 1, the largest
		ubdNaN128(bid.UBDSNaNHi|bid.UBDSignHi, maxPayload),
		ubdNaN128(bid.UBDNaNHi|bid.UBDSignHi, bid.Uint128{Hi: 1 << 44}), // payload 2^108: its top bits in qExp
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
	for _, x := range pool {
		mustWellFormed128(t, "ubdPool128", x)
	}
	return pool
}

// TestUBDWellFormed checks bid.UBDWellFormed128 against the test's own
// statement at every boundary of the qExp field, the coefficient, the
// special prefix and the NaN payload, and checks that well-formed is
// canonical: a pattern is well-formed exactly when the round trip through
// BID gives it back.
func TestUBDWellFormed(t *testing.T) {
	var exps []int
	for _, e := range []int{-8192, -6177, -6176, 6111, 6112, 7167, 7168, 7423, 7424, 7679, 7680, 7683, 7684, 7935, 7936, 7939, 7940, 8191} {
		exps = append(exps, e-1, e, e+1)
	}
	maxP := bid.MaxPayload128Hi & (1<<44 - 1) // the largest payload's coefficient bits, with e&3 = 3
	his := []uint64{0, 1, 0x1E << 44, 0x1F << 44, 0x1F<<44 - 1, 0x1F<<44 | 1, 0x1F<<44 | maxP, 0x1F<<44 | maxP + 1,
		0x1F<<44 | 1<<44 - 1, bid.MaxCoeff128Hi, bid.MaxCoeff128Hi + 1, bid.UBDCoeffMaskHi}
	los := []uint64{0, 7, bid.MaxCoeff128Lo, bid.MaxCoeff128Lo + 1, bid.MaxPayload128Lo, bid.MaxPayload128Lo + 1, 1<<64 - 1}
	check := func(x bid.Uint128) {
		t.Helper()
		got, want := bid.UBDWellFormed128(x), ubdWellFormed128(x)
		if got != want {
			t.Errorf("UBDWellFormed128(%#x) (qExp %d) = %v, want %v", x, int64(x.Hi)>>50, got, want)
		}
		if canon := ubdCanonical128(x); want != canon {
			t.Errorf("%#x (qExp %d): well-formed %v, but canonical %v", x, int64(x.Hi)>>50, want, canon)
		}
	}
	for _, e := range exps {
		for _, hi := range his {
			for _, lo := range los {
				for _, neg := range []bool{false, true} {
					check(ubd128(neg, e, hi, lo))
				}
			}
		}
	}
	// Random patterns, half of them under the special tag, with each of
	// the six qExp bits below the tag and the prefix often set alone.
	r := rand.New(rand.NewPCG(2, 128))
	for range 100000 {
		x := bid.Uint128{Hi: r.Uint64(), Lo: r.Uint64()}
		if r.IntN(2) == 0 {
			x.Hi = x.Hi&(1<<58-1) | uint64(28+r.IntN(4))<<58 | 0x1F<<44
			if r.IntN(2) == 0 {
				x.Hi &^= 0x3F << 52
			}
			if r.IntN(4) == 0 {
				x.Hi &^= 1<<44 - 1
				x.Lo = 0
			}
		}
		check(x)
	}
}

func TestUBDConversion(t *testing.T) {
	// All bits clear is +0E+0.
	if got, want := bid.BIDFromUBD128(bid.Uint128{}), (bid.Uint128{Hi: bid.Zero128Hi}); got != want {
		t.Errorf("BIDFromUBD128(0) = %#x, want +0E+0 %#x", got, want)
	}
	// The pool, and random encodings of the kinds UBDFromBID128's fast
	// path must tell apart: any bits; first-form coefficients with bit 112
	// set, below and above 10^34 - 1; and second-form encodings, the
	// specials among them and the rest non-canonical.
	xs := inlinePool128()
	r := rand.New(rand.NewPCG(121, 122))
	for range 300_000 {
		x := bid.Uint128{Hi: r.Uint64(), Lo: r.Uint64()}
		switch r.IntN(3) {
		case 0: // first form, coefficient bit 112 set
			x.Hi = x.Hi&^bid.Form2Mask128Hi | 1<<48
			if r.IntN(2) == 0 {
				x.Hi = x.Hi&^(1<<48-1) | r.Uint64N(bid.MaxCoeff128Hi&(1<<48-1)+2)
			}
		case 1: // second form
			x.Hi |= bid.Form2Mask128Hi
		}
		xs = append(xs, x)
	}
	for _, x := range xs {
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
		if got, want := bid.UBDKey128(bid.UBD128{Lo: x.Lo, Hi: x.Hi}), bid.Key128(bx); got != want {
			fail("UBDKey128(%#x) = %+v, want %+v", x, got, want)
		}
		if got, want := bid.UBDInt64From128(x), first(bid.Int64From128(bx, bid.TowardZero)); got != want {
			fail("ubdInt64From128(%#x) = %d, want %d", x, got, want)
		}
		if got, want := bid.UBDUint64From128(x), first(bid.Uint64From128(bx, bid.TowardZero)); got != want {
			fail("ubdUint64From128(%#x) = %d, want %d", x, got, want)
		}
		for _, y := range pool {
			by := bid.BIDFromUBD128(y)
			c, ok := bid.UBDCompare128(x, y)
			wc, wok := bid.Compare128(bx, by)
			if c != wc || ok != wok {
				fail("ubdCompare128(%#x, %#x) = %d, %v; want %d, %v", x, y, c, ok, wc, wok)
			}
			if got, want := bid.UBDPredicates128(x, y), [5]bool{wok && wc == 0, wok && wc < 0, wok && wc <= 0, wok && wc > 0, wok && wc >= 0}; got != want {
				fail("predicate entries (==, <, <=, >, >=) on %#x, %#x = %v, want %v", x, y, got, want)
			}
			check("ubdMin128", x, y, bid.ToNearestEven, bid.UBDMin128(x, y), bid.UBDFromBID128(bid.Min128(bx, by)))
			check("ubdMax128", x, y, bid.ToNearestEven, bid.UBDMax128(x, y), bid.UBDFromBID128(bid.Max128(bx, by)))
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

// first is the first of two results.
func first[A, B any](a A, _ B) A { return a }

// TestUBDMathDecimal checks math/decimal's remaining operations on UBD
// (ubd_misc.go) against this package's BID functions on the same values,
// as TestUBDKernel checks the kernel entries: every operand of the pool,
// every pair, and for the three-operand ones a third from a short list of
// quanta, addends and specials.
func TestUBDMathDecimal(t *testing.T) {
	pool := ubdPool128(t)
	u := func(x bid.Uint128) bid.UBD128 { return bid.UBD128{Lo: x.Lo, Hi: x.Hi} }
	w := func(r bid.UBD128) bid.Uint128 { return bid.Uint128{Hi: r.Hi, Lo: r.Lo} }
	modes := append(append([]bid.Mode{}, inlineModes...), 6)
	if testing.Short() {
		modes = []bid.Mode{bid.ToNearestEven, bid.TowardZero}
	}
	errs := 0
	check := func(name string, args []bid.Uint128, got bid.UBD128, want bid.Uint128) {
		t.Helper()
		mustWellFormed128(t, name, w(got))
		if w(got) != bid.UBDFromBID128(want) {
			if errs++; errs <= 20 {
				t.Errorf("%s%#x = %#x, want %#x", name, args, w(got), bid.UBDFromBID128(want))
			}
		}
	}
	var thirds []bid.Uint128
	for _, s := range []string{"0.01", "1", "1E+3", "1E-20", "0", "-7.5", "123456789012345678901234567890.1234", "Inf", "NaN"} {
		x, _ := bid.Parse128(s, bid.ToNearestEven)
		thirds = append(thirds, bid.UBDFromBID128(x))
	}
	// Parse reads "sNaN" as a quiet NaN: a signaling one is built.
	thirds = append(thirds, bid.Uint128{Hi: bid.UBDSNaNHi | bid.UBDSignHi, Lo: 5})
	// For the one-operand functions, coefficients with trailing zeros at
	// the top of the exponent range, where Reduce must stop.
	unary := append([]bid.Uint128{}, pool...)
	for _, e := range []int{bid.MaxExp128 - 20, bid.MaxExp128 - 3, bid.MaxExp128 - 1, bid.MaxExp128} {
		for _, c := range []uint64{10, 1000, 1e20 / 1e1, 1e19} {
			unary = append(unary, ubd128(false, e, 0, c), ubd128(true, e, 0, c))
		}
	}
	for _, x := range unary {
		bx := bid.BIDFromUBD128(x)
		args := []bid.Uint128{x}
		check("UBDPayload128", args, bid.UBDPayload128(u(x)), bid.Payload128(bx))
		check("UBDNaNWithPayload128", args, bid.UBDNaNWithPayload128(u(x), false), bid.NaNWithPayload128(bx))
		check("UBDSignalingNaNWithPayload128", args, bid.UBDNaNWithPayload128(u(x), true), bid.SignalingNaNWithPayload128(bx))
		check("UBDQuantum128", args, bid.UBDQuantum128(u(x)), bid.Quantum128(bx))
		check("UBDReduce128", args, bid.UBDReduce128(u(x)), bid.Reduce128(bx))
		for _, m := range modes {
			check("UBDRoundToIntegral128", args, bid.UBDRoundToIntegral128(u(x), m), bid.RoundToIntegral128(bx, m))
		}
		for _, y := range pool {
			by := bid.BIDFromUBD128(y)
			args := []bid.Uint128{x, y}
			if got, want := bid.UBDSameQuantum128(u(x), u(y)), bid.SameQuantum128(bx, by); got != want {
				if errs++; errs <= 20 {
					t.Errorf("UBDSameQuantum128(%#x, %#x) = %v, want %v", x, y, got, want)
				}
			}
			check("UBDRem128", args, bid.UBDRem128(u(x), u(y)), bid.Rem128(bx, by))
			check("UBDMod128", args, bid.UBDMod128(u(x), u(y)), bid.Mod128(bx, by))
			for _, z := range thirds {
				bz := bid.BIDFromUBD128(z)
				args := []bid.Uint128{x, y, z}
				for _, m := range modes {
					check("UBDFMA128", args, bid.UBDFMA128(u(x), u(y), u(z), m), bid.FMA128(bx, by, bz, m))
					check("UBDQuoQuantize128", args, bid.UBDQuoQuantize128(u(x), u(y), u(z), m), bid.QuoQuantize128(bx, by, bz, m))
				}
			}
		}
	}
	if errs == 0 {
		t.Logf("%d operands, %d pairs, %d thirds, %d modes", len(pool), len(pool)*len(pool), len(thirds), len(modes))
	}
}

// TestUBDFloat checks the conversions between decimal128 and the binary
// formats on UBD (ubd_float.go) against the BID ones: floats of every kind
// (random bit patterns, which include subnormals and NaN payloads, the
// specials and exact integers and fractions) to decimal128 in every mode,
// and the UBD operand pool and random decimals to both binary formats.
func TestUBDFloat(t *testing.T) {
	modes := append(append([]bid.Mode{}, inlineModes...), 6)
	n := 200_000
	if testing.Short() {
		n = 20_000
	}
	r := rand.New(rand.NewPCG(101, 102))
	errs := 0
	fail := func(format string, args ...any) {
		t.Helper()
		if errs++; errs <= 20 {
			t.Errorf(format, args...)
		}
	}
	u := func(x bid.UBD128) bid.Uint128 { return bid.Uint128{Hi: x.Hi, Lo: x.Lo} }
	f64s := []float64{0, math.Copysign(0, -1), 1, -1, 0.1, 0.5, 2, 1e20, 1e23, 1 << 53, math.MaxFloat64, math.SmallestNonzeroFloat64,
		math.Inf(1), math.Inf(-1), math.NaN(), math.Float64frombits(0xfff8000000000123)}
	for i := 0; i < n; i++ {
		f64s = append(f64s, math.Float64frombits(r.Uint64()))
	}
	for _, f := range f64s {
		for _, m := range modes {
			if got, want := u(bid.UBDFloat64To128(f, m)), bid.UBDFromBID128(bid.Float64To128(f, m)); got != want {
				fail("UBDFloat64To128(%#x, %d) = %#x, want %#x", math.Float64bits(f), m, got, want)
			}
			f32 := math.Float32frombits(uint32(math.Float64bits(f)))
			if got, want := u(bid.UBDFloat32To128(f32, m)), bid.UBDFromBID128(bid.Float32To128(f32, m)); got != want {
				fail("UBDFloat32To128(%#x, %d) = %#x, want %#x", math.Float32bits(f32), m, got, want)
			}
		}
	}
	decs := ubdPool128(t)
	for i := 0; i < n; i++ {
		var x bid.Uint128
		switch r.IntN(3) {
		case 0:
			x = ubd128(r.IntN(2) == 0, r.IntN(801)-400, 0, r.Uint64())
		case 1:
			x = ubd128(r.IntN(2) == 0, r.IntN(81)-60, r.Uint64N(bid.MaxCoeff128Hi), r.Uint64())
		default:
			x = ubd128(r.IntN(2) == 0, bid.MinExp128+r.IntN(bid.MaxExp128-bid.MinExp128+1), 0, r.Uint64N(1e12))
		}
		decs = append(decs, x)
	}
	for _, x := range decs {
		ux, bx := bid.UBD128{Lo: x.Lo, Hi: x.Hi}, bid.BIDFromUBD128(x)
		for _, m := range modes {
			if got, want := math.Float64bits(bid.UBDFloat64From128(ux, m)), math.Float64bits(bid.Float64From128(bx, m)); got != want {
				fail("UBDFloat64From128(%#x, %d) = %#x, want %#x", x, m, got, want)
			}
			if got, want := math.Float32bits(bid.UBDFloat32From128(ux, m)), math.Float32bits(bid.Float32From128(bx, m)); got != want {
				fail("UBDFloat32From128(%#x, %d) = %#x, want %#x", x, m, got, want)
			}
		}
	}
	if errs == 0 {
		t.Logf("%d floats, %d decimals, %d modes", len(f64s), len(decs), len(modes))
	}
}

// TestUBDAppend checks UBDAppend128 against Append128 on the same values:
// the UBD pool and random values of every layout, into a buffer with the
// spare capacity the contract asks and into one without.
func TestUBDAppend(t *testing.T) {
	xs := ubdPool128(t)
	r := rand.New(rand.NewPCG(131, 132))
	for range 200_000 {
		switch r.IntN(3) {
		case 0:
			xs = append(xs, ubd128(r.IntN(2) == 0, r.IntN(9)-6, 0, r.Uint64N(1e9)))
		case 1:
			xs = append(xs, ubd128(r.IntN(2) == 0, r.IntN(81)-60, r.Uint64N(bid.MaxCoeff128Hi), r.Uint64()))
		default:
			xs = append(xs, ubd128(r.IntN(2) == 0, bid.MinExp128+r.IntN(bid.MaxExp128-bid.MinExp128+1), 0, r.Uint64N(1e19)))
		}
	}
	errs := 0
	for _, x := range xs {
		want := string(bid.Append128(nil, bid.BIDFromUBD128(x)))
		u := bid.UBD128{Lo: x.Lo, Hi: x.Hi}
		roomy := string(bid.UBDAppend128(make([]byte, 0, bid.MaxLen128), u))
		tight := string(bid.UBDAppend128(nil, u))
		if roomy != want || tight != want {
			if errs++; errs <= 20 {
				t.Errorf("UBDAppend128(%#x) = %q (%q without room), want %q", x, roomy, tight, want)
			}
		}
	}
	if errs == 0 {
		t.Logf("%d values", len(xs))
	}
}
