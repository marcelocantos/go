// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// Tests of Widen and Narrow beyond the differential test TestKernel: the
// audit case that a lost sticky bit gets wrong, the round trip, allocation,
// and a fuzz target against the reference model.

import (
	"internal/bid"
	"math/big"
	"math/rand/v2"
	"testing"
)

var convertModes = []bid.Mode{bid.ToNearestEven, bid.ToNearestAway, bid.TowardZero, bid.TowardPositive, bid.TowardNegative}

// TestNarrowAudit is the case found by audit: the 31-digit coefficient
// 1769609755703054503235303742165 at exponent 167 keeps its first 16 digits,
// ...054, and drops 503235303742165, which is above half of the last place,
// so the result is ...055 at exponent 182. An implementation that loses the
// digits below the first dropped one (a missing sticky bit) sees a tie and
// rounds to the even ...054.
func TestNarrowAudit(t *testing.T) {
	c, _ := new(big.Int).SetString("1769609755703054503235303742165", 10)
	x := dec128.encode(value{kind: finite, coeff: c, exp: 167})
	want := dec64.encode(value{kind: finite, coeff: big.NewInt(1769609755703055), exp: 182})
	if got := bid.Narrow(x, bid.ToNearestEven); got != want.Lo {
		t.Errorf("Narrow(%s, ToNearestEven) = %s, want %s", display(dec128, x), display(dec64, lo(got)), display(dec64, want))
	}
	for _, m := range convertModes {
		for _, neg := range []bool{false, true} {
			x := dec128.encode(value{kind: finite, neg: neg, coeff: c, exp: 167})
			if got, want := bid.Narrow(x, m), narrow(x, m); got != want.Lo {
				t.Errorf("Narrow(%s, %s) = %s, want %s", display(dec128, x), modeName(m), display(dec64, lo(got)), display(dec64, want))
			}
		}
	}
	// Dropped digits 500000000000001 are above half by the last place
	// only; losing that digit makes the case a tie, which rounds to the
	// even ...054.
	c2, _ := new(big.Int).SetString("1769609755703054500000000000001", 10)
	x = dec128.encode(value{kind: finite, coeff: c2, exp: 167})
	want = dec64.encode(value{kind: finite, coeff: big.NewInt(1769609755703055), exp: 182})
	if got := bid.Narrow(x, bid.ToNearestEven); got != want.Lo {
		t.Errorf("Narrow(%s) = %s, want %s: the sticky bit was lost", display(dec128, x), display(dec64, lo(got)), display(dec64, want))
	}
}

// TestWidenNarrowRoundTrip checks that Narrow(Widen(x)) is x bit for bit
// for every canonical decimal64, in every mode (NaNs become the canonical
// NaN), and that a non-canonical encoding comes back canonical. It runs
// every exponent against a set of coefficients, then random values and raw
// bit patterns.
func TestWidenNarrowRoundTrip(t *testing.T) {
	check := func(x uint64) {
		t.Helper()
		w := bid.Widen(x)
		want := dec64.encode(dec64.decode(lo(x))).Lo
		for _, m := range convertModes {
			if got := bid.Narrow(w, m); got != want {
				t.Fatalf("Narrow(Widen(%#016x), %s) = %#016x, want %#016x", x, modeName(m), got, want)
			}
		}
		if w != widen(lo(x)) {
			t.Fatalf("Widen(%#016x) = %#x, reference %#x", x, w, widen(lo(x)))
		}
	}

	coeffs := []int64{0, 1, 9, 10, 999999999999999, 1000000000000000, 1234567890123456, 9007199254740993, 9999999999999999}
	for _, c := range coeffs {
		for exp := dec64.minExp; exp <= dec64.maxExp; exp++ {
			for _, neg := range []bool{false, true} {
				check(dec64.encode(value{kind: finite, neg: neg, coeff: big.NewInt(c), exp: exp}).Lo)
			}
		}
	}
	for _, x := range []uint64{bid.Inf64, bid.Inf64 | bid.SignMask64, bid.NaN64, bid.SNaN64, bid.NaN64 | bid.SignMask64, bid.NaN64 | 0x3FFFFFFFFFFFF} {
		check(x)
	}

	n := numCases()
	g := &gen{f: dec64, r: rand.New(rand.NewPCG(*seedFlag, streamSeed("widennarrow")))}
	for i := range n {
		g.regime = regime(i % int(numRegimes))
		check(g.dec().Lo)
		check(g.r.Uint64())
	}
}

// TestWidenNarrowAllocs checks that neither conversion allocates, on
// finite values, special values and every mode.
func TestWidenNarrowAllocs(t *testing.T) {
	c, _ := new(big.Int).SetString("1769609755703054503235303742165", 10)
	in128 := []bits{
		dec128.encode(value{kind: finite, coeff: c, exp: 167}),
		dec128.encode(value{kind: finite, neg: true, coeff: c, exp: -6000}),
		dec128.encode(value{kind: finite, coeff: c, exp: 6000}),
		dec128.encode(value{kind: finite, coeff: new(big.Int), exp: 1000}),
		{Hi: bid.NaN128Hi},
		{Hi: bid.Inf128Hi},
	}
	in64 := []uint64{
		dec64.encode(value{kind: finite, coeff: big.NewInt(1234567890123456), exp: 369}).Lo,
		dec64.encode(value{kind: finite, coeff: big.NewInt(1), exp: -398}).Lo,
		bid.NaN64,
		bid.Inf64,
	}
	var sink64 uint64
	var sink128 bits
	if n := testing.AllocsPerRun(100, func() {
		for _, x := range in64 {
			sink128.Lo += bid.Widen(x).Lo
		}
	}); n != 0 {
		t.Errorf("Widen: %v allocations", n)
	}
	for _, m := range convertModes {
		if n := testing.AllocsPerRun(100, func() {
			for _, x := range in128 {
				sink64 += bid.Narrow(x, m)
			}
		}); n != 0 {
			t.Errorf("Narrow in mode %s: %v allocations", modeName(m), n)
		}
	}
	_, _ = sink64, sink128
}

// convertFuzz checks Widen on a decimal64 encoding and Narrow on a
// decimal128 encoding against the reference model. The raw bit patterns
// reach non-canonical encodings and payload NaNs; the canonical value
// built from the input reaches the exponents and digit counts around the
// decimal64 range, which raw patterns rarely do.
func convertFuzz(t *testing.T, hi, lo64 uint64, e int16, sel uint8) {
	m := convertModes[int(sel&0x7f)%len(convertModes)]
	neg := sel&0x80 != 0

	if got, want := bid.Widen(lo64), widen(lo(lo64)); got != want {
		t.Fatalf("Widen(%#016x) = %s (%#x), want %s (%#x)", lo64, display(dec128, got), got, display(dec128, want), want)
	}
	check := func(x bits) {
		t.Helper()
		if got, want := bid.Narrow(x, m), narrow(x, m); got != want.Lo {
			t.Fatalf("Narrow(%#x, %s) = %s (%#016x), want %s (%#016x)", x, modeName(m), display(dec64, lo(got)), got, display(dec64, want), want.Lo)
		}
	}
	check(bits{Hi: hi, Lo: lo64})

	c := new(big.Int).Lsh(new(big.Int).SetUint64(hi), 64)
	c.Or(c, new(big.Int).SetUint64(lo64))
	c.Mod(c, pow10(dec128.prec))
	// Exponents within the decimal64 range, widened by the digits that
	// decimal128 can add beyond it, so that a coefficient with more than
	// 16 digits straddles both ends.
	span := dec64.maxExp - dec64.minExp + 2*dec128.prec
	q := dec64.minExp - dec128.prec + (int(e)%span+span)%span
	check(dec128.encode(value{kind: finite, neg: neg, coeff: c, exp: q}))

	// A decimal64 tie and its neighbors: a 16-digit coefficient, a 5, and a
	// tail that is zero, or one in the last place above or below.
	t16 := new(big.Int).Mod(c, pow10(dec64.prec))
	tie := new(big.Int).Mul(t16, big.NewInt(10))
	tie.Add(tie, big.NewInt(5))
	tie.Mul(tie, pow10(dec128.prec-dec64.prec-2))
	tie.Add(tie, big.NewInt(int64(hi%3)-1))
	if tie.Sign() >= 0 && digits(tie) <= dec128.prec {
		check(dec128.encode(value{kind: finite, neg: neg, coeff: tie, exp: q}))
	}
}

func FuzzConvertFormat(f *testing.F) {
	f.Add(uint64(0), uint64(0), int16(0), uint8(0))
	f.Add(uint64(0), uint64(1), int16(36), uint8(1))
	f.Add(uint64(0x1ED09BEAD87C0), uint64(0x378D8E63FFFFFFFF), int16(-360), uint8(2))
	f.Add(uint64(0x00000005B1E1F2C7), uint64(0x0123456789ABCDEF), int16(580), uint8(0x83))
	f.Add(uint64(0x7C00000000000000), uint64(0x7C00000000000001), int16(1), uint8(4))
	f.Add(uint64(0x7E00000000000000), uint64(0xFE00000000000005), int16(-1), uint8(0x80))
	f.Add(uint64(0xF800000000000000), uint64(0x7800000000000000), int16(0), uint8(3))
	f.Fuzz(convertFuzz)
}
