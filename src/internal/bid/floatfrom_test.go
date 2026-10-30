// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// Tests of Float64From64, Float64From128, Float32From64 and Float32From128
// beyond the differential test TestKernel: an oracle through math/big.Float,
// written independently of the reference model's rounding, expected values
// worked out by hand at the places where a conversion goes wrong, and a fuzz
// target per function.

import (
	"internal/bid"
	"math"
	"math/big"
	"math/rand/v2"
	"testing"
)

var floatFromModes = []bid.Mode{bid.ToNearestEven, bid.ToNearestAway, bid.TowardZero, bid.TowardPositive, bid.TowardNegative}

func floatFromBigMode(m bid.Mode) big.RoundingMode {
	switch m {
	case bid.ToNearestAway:
		return big.ToNearestAway
	case bid.TowardZero:
		return big.ToZero
	case bid.TowardPositive:
		return big.ToPositiveInf
	case bid.TowardNegative:
		return big.ToNegativeInf
	}
	return big.ToNearestEven
}

// floatFromOracle returns the bits of the finite value (-1)^neg × c × 10^q
// rounded to the binary format b in mode: the exact rational, rounded once by
// big.Float at the format's precision.
//
// big.Float has no exponent range, so the format's is applied here. The
// significand of a value below the smallest normal 2^emin has its last place
// at the same position as that of any value in [2^emin, 2^(emin+1)), so such
// a value is rounded by adding 2^emin, rounding at full precision and
// subtracting 2^emin again. Above the range, the overflow table applies.
func floatFromOracle(neg bool, c *big.Int, q int, m bid.Mode, b binary) uint64 {
	width := 1 + b.expBits + b.prec - 1
	sign := b2u(neg) << (width - 1)
	if c.Sign() == 0 {
		return sign
	}

	// Exponents far outside any binary format would only make the arithmetic
	// slow: every such value has the same result as the one clamped to.
	if q > 400 {
		q = 400
	}
	if adj := digits(c) + q; adj < -400 {
		c, q = bigOne, -400
	}

	v := new(big.Rat).SetInt(c)
	if q >= 0 {
		v.Mul(v, new(big.Rat).SetInt(pow10(q)))
	} else {
		v.Quo(v, new(big.Rat).SetInt(pow10(-q)))
	}
	smallest := new(big.Rat).SetFrac(bigOne, new(big.Int).Lsh(bigOne, uint(-b.emin)))
	if neg {
		v.Neg(v)
		smallest.Neg(smallest)
	}

	mode := floatFromBigMode(m)
	round := func(r *big.Rat) *big.Float {
		return new(big.Float).SetPrec(uint(b.prec)).SetMode(mode).SetRat(r)
	}

	if new(big.Rat).Abs(v).Cmp(new(big.Rat).Abs(smallest)) < 0 {
		z, _ := round(new(big.Rat).Add(v, smallest)).Rat(nil)
		z.Sub(z, smallest).Abs(z)
		// z is now an integer multiple of 2^(emin-prec+1); at 2^(prec-1) the
		// multiple is also the encoding of the smallest normal.
		z.Mul(z, new(big.Rat).SetInt(new(big.Int).Lsh(bigOne, uint(b.prec-1-b.emin))))
		return sign | z.Num().Uint64()
	}

	z := round(v)
	e := z.MantExp(nil) - 1 // the value is in [2^e, 2^(e+1))
	expMask := uint64(1)<<b.expBits - 1
	if e > b.emax {
		toInf := true
		switch m {
		case bid.TowardZero:
			toInf = false
		case bid.TowardPositive:
			toInf = !neg
		case bid.TowardNegative:
			toInf = neg
		}
		if toInf {
			return sign | expMask<<(b.prec-1)
		}
		return sign | (expMask-1)<<(b.prec-1) | (1<<(b.prec-1) - 1)
	}
	scaled := new(big.Float).SetMantExp(z, b.prec-1-e)
	mant, _ := scaled.Abs(scaled).Int(nil)
	biased := uint64(e - b.emin + 1)
	return sign | biased<<(b.prec-1) | mant.Uint64()&(1<<(b.prec-1)-1)
}

// floatFromWant is floatFromOracle for an encoding, with the special values.
func floatFromWant(f *format, x bits, m bid.Mode, b binary) uint64 {
	a := f.decode(x)
	expMask := uint64(1)<<b.expBits - 1
	switch a.kind {
	case notANumber:
		return expMask<<(b.prec-1) | 1<<(b.prec-2)
	case infinite:
		return b2u(a.neg)<<(b.expBits+b.prec-1) | expMask<<(b.prec-1)
	}
	return floatFromOracle(a.neg, a.coeff, a.exp, m, b)
}

// floatFromGot returns the bits of the kernel's conversion of x to b.
func floatFromGot(f *format, x bits, m bid.Mode, b binary) uint64 {
	if b.prec == binary32.prec {
		return uint64(math.Float32bits(kernel{}.float32From(f, x, m)))
	}
	return math.Float64bits(kernel{}.float64From(f, x, m))
}

// floatFromCheck compares the kernel with the oracle.
func floatFromCheck(t *testing.T, f *format, x bits, m bid.Mode, b binary) {
	t.Helper()
	got, want := floatFromGot(f, x, m, b), floatFromWant(f, x, m, b)
	if got != want {
		t.Errorf("%d-bit binary from %s in mode %s: got %#x, want %#x", b.prec, display(f, x), modeName(m), got, want)
	}
}

// floatFromEncode returns the encoding of (-1)^neg × c × 10^q, which must fit.
func floatFromEncode(f *format, neg bool, c *big.Int, q int) bits {
	return f.encode(value{kind: finite, neg: neg, coeff: c, exp: q})
}

// floatFromFits reports whether c × 10^q is an encodable finite value of f.
func floatFromFits(f *format, c *big.Int, q int) bool {
	return digits(c) <= f.prec && q >= f.minExp && q <= f.maxExp
}

// floatFromExpect checks the kernel on x for both signs and all modes, given
// the magnitudes of the three possible results for a value that is not a tie:
// its nearest binary value and the neighbors toward and away from zero. The
// oracle is also checked here against the same expectations.
func floatFromExpect(t *testing.T, f *format, c *big.Int, q int, b binary, nearest, toward, away uint64) {
	t.Helper()
	signBit := uint64(1) << (b.expBits + b.prec - 1)
	for _, neg := range []bool{false, true} {
		x := floatFromEncode(f, neg, c, q)
		for _, m := range floatFromModes {
			want := nearest
			switch {
			case m == bid.TowardZero, m == bid.TowardPositive && neg, m == bid.TowardNegative && !neg:
				want = toward
			case m == bid.TowardPositive || m == bid.TowardNegative:
				want = away
			}
			if neg {
				want |= signBit
			}
			if got := floatFromGot(f, x, m, b); got != want {
				t.Errorf("%d-bit binary from %s in mode %s: got %#x, want %#x", b.prec, display(f, x), modeName(m), got, want)
			}
			if got := floatFromWant(f, x, m, b); got != want {
				t.Errorf("oracle: %d-bit binary from %s in mode %s: got %#x, want %#x", b.prec, display(f, x), modeName(m), got, want)
			}
		}
	}
}

// floatFromFormats lists the format and binary pairs under test.
var floatFromFormats = []struct {
	name string
	f    *format
	b    binary
}{
	{"Float64From64", dec64, binary64},
	{"Float64From128", dec128, binary64},
	{"Float32From64", dec64, binary32},
	{"Float32From128", dec128, binary32},
}

// TestFloatFromAudit checks the conversions the strawman got wrong, against
// the values the compiler gives the same literals.
func TestFloatFromAudit(t *testing.T) {
	tests := []struct {
		c    int64
		q    int
		want float64
	}{
		{1, 36, 1e36},
		{1, -211, 1e-211},
		{123456789, -20, 123456789e-20},
		{5, -42, 5e-42},
		{1, 23, 1e23},
		{1, 22, 1e22},
		{3, -1, 0.3},
	}
	for _, tt := range tests {
		for _, f := range []*format{dec64, dec128} {
			x := floatFromEncode(f, false, big.NewInt(tt.c), tt.q)
			if got := (kernel{}).float64From(f, x, bid.ToNearestEven); got != tt.want {
				t.Errorf("float64(%dE%d) from decimal%d = %v, want %v", tt.c, tt.q, f.width, got, tt.want)
			}
		}
	}
	// As a float32, a literal has its own correctly rounded value, which is
	// not a rounding of the float64 above in general.
	tests32 := []struct {
		c    int64
		q    int
		want float32
	}{
		{1, 36, 1e36},
		{123456789, -20, 123456789e-20},
		{5, -42, 5e-42},
	}
	for _, tt := range tests32 {
		for _, f := range []*format{dec64, dec128} {
			x := floatFromEncode(f, false, big.NewInt(tt.c), tt.q)
			if got := (kernel{}).float32From(f, x, bid.ToNearestEven); got != tt.want {
				t.Errorf("float32(%dE%d) from decimal%d = %v, want %v", tt.c, tt.q, f.width, got, tt.want)
			}
		}
	}
}

// floatFromLdexp returns the bits of the odd-or-even integer n × 2^e, which
// must be exactly representable in b.
func floatFromLdexp(n *big.Int, e int, b binary) uint64 {
	v := math.Ldexp(float64(n.Int64()), e)
	if b.prec == binary32.prec {
		return uint64(math.Float32bits(float32(v)))
	}
	return math.Float64bits(v)
}

// TestFloatFromBinaryTies checks values exactly halfway between two binary
// values, at 53 and 24 bits, and the values one unit in the last decimal place
// to each side. The decimal is (2^p + odd) × 2^j, written as an integer for
// j ≥ 0 and as that integer × 5^-j × 10^j for j < 0. The expected results
// follow from the two neighbors, which are computed by exact integer scaling.
func TestFloatFromBinaryTies(t *testing.T) {
	for _, ff := range floatFromFormats {
		t.Run(ff.name, func(t *testing.T) {
			f, b := ff.f, ff.b
			tested := 0
			for _, odd := range []int64{1, 3, 5, 7, 1<<(b.prec-1) - 1, 1<<(b.prec-1) + 1} {
				tie := new(big.Int).Lsh(bigOne, uint(b.prec))
				tie.Add(tie, big.NewInt(odd))
				below := new(big.Int).Sub(tie, bigOne)
				above := new(big.Int).Add(tie, bigOne)
				// The even significand is the one whose half is even.
				belowIsEven := new(big.Int).Rsh(below, 1).Bit(0) == 0

				for j := -60; j <= 80; j++ {
					c, q := new(big.Int).Set(tie), 0
					if j >= 0 {
						c.Lsh(c, uint(j))
					} else {
						c.Mul(c, new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(-j)), nil))
						q = j
					}
					if !floatFromFits(f, c, q) {
						continue
					}
					tested++
					lo := floatFromLdexp(below, j, b)
					hi := floatFromLdexp(above, j, b)
					even, away := lo, hi
					if !belowIsEven {
						even = hi
					}
					// The tie itself.
					for _, neg := range []bool{false, true} {
						x := floatFromEncode(f, neg, c, q)
						sign := b2u(neg) << (b.expBits + b.prec - 1)
						wants := map[bid.Mode]uint64{
							bid.ToNearestEven: even,
							bid.ToNearestAway: away,
							bid.TowardZero:    lo,
						}
						if neg {
							wants[bid.TowardPositive], wants[bid.TowardNegative] = lo, hi
						} else {
							wants[bid.TowardPositive], wants[bid.TowardNegative] = hi, lo
						}
						for m, want := range wants {
							if got := floatFromGot(f, x, m, b); got != sign|want {
								t.Errorf("tie %s in mode %s: got %#x, want %#x", display(f, x), modeName(m), got, sign|want)
							}
							floatFromCheck(t, f, x, m, b)
						}
					}
					// One unit in the last place to each side, at the longest
					// coefficient that holds the tie. For a tie that has to
					// stay an integer of 16 digits, that unit is the neighbor
					// itself and the value is not inexact.
					cw, qw := new(big.Int).Set(c), q
					for digits(cw) < f.prec && qw > f.minExp {
						cw.Mul(cw, big.NewInt(10))
						qw--
					}
					if j == 0 && qw == 0 {
						continue
					}
					floatFromExpect(t, f, new(big.Int).Sub(cw, bigOne), qw, b, lo, lo, hi)
					floatFromExpect(t, f, new(big.Int).Add(cw, bigOne), qw, b, hi, lo, hi)
				}
			}
			if tested == 0 {
				t.Errorf("no tie fits decimal%d", f.width)
			}
		})
	}
}

// TestFloatFromDoubleRounding checks values whose rounding to float64 lands
// on a float32 tie: through float64 they would tie, and the even neighbor
// would be the wrong one half of the time. The offset is far below half a
// float64 ulp, so these need a coefficient longer than 16 digits.
func TestFloatFromDoubleRounding(t *testing.T) {
	f := dec128
	const shift = 20 // 10^-20 is below half the float64 ulp of 2^-28 here
	scale := pow10(shift)
	tests := []struct {
		tie      int64 // a float32 tie: 2^24 + odd
		sign     int64 // the offset of the value from the tie, in 10^-20
		nearest  int64
		below    int64
		above    int64
		viaFloat int64 // what float32(float64(x)) gives, which is wrong
	}{
		{16777217, +1, 16777218, 16777216, 16777218, 16777216},
		{16777219, -1, 16777218, 16777218, 16777220, 16777220},
		{16777221, +1, 16777222, 16777220, 16777222, 16777220},
		{16777223, -1, 16777222, 16777222, 16777224, 16777224},
	}
	for _, tt := range tests {
		c := new(big.Int).Mul(big.NewInt(tt.tie), scale)
		c.Add(c, big.NewInt(tt.sign))
		x := floatFromEncode(f, false, c, -shift)

		got := bid.Float32From128(x, bid.ToNearestEven)
		if got != float32(tt.nearest) {
			t.Errorf("Float32From128(%s) = %v, want %v", display(f, x), got, tt.nearest)
		}
		// Make sure the case is a real trap, not a case any route gets right.
		via := float32(bid.Float64From128(x, bid.ToNearestEven))
		if via != float32(tt.viaFloat) {
			t.Errorf("float32(Float64From128(%s)) = %v, want the double-rounded %v", display(f, x), via, tt.viaFloat)
		}
		floatFromExpect(t, f, c, -shift, binary32,
			uint64(math.Float32bits(float32(tt.nearest))),
			uint64(math.Float32bits(float32(tt.below))),
			uint64(math.Float32bits(float32(tt.above))))
	}
}

// floatFromRat returns the exact value of v, which may also be a float32
// widened to float64, as a rational.
func floatFromRat(v float64) *big.Rat {
	return new(big.Rat).SetFloat64(v)
}

// floatFromBracket returns coefficient and exponent pairs of the largest
// decimal of f's precision below r and the smallest above it. r must not have
// an exact representation at that precision.
func floatFromBracket(t *testing.T, f *format, r *big.Rat) (below, above *big.Int, q int) {
	t.Helper()
	// r × 10^-q has f.prec integer digits for one q near this estimate.
	q = digits(r.Num()) - digits(r.Denom()) - f.prec
	var scaled *big.Rat
	for {
		scaled = new(big.Rat).Set(r)
		if q >= 0 {
			scaled.Quo(scaled, new(big.Rat).SetInt(pow10(q)))
		} else {
			scaled.Mul(scaled, new(big.Rat).SetInt(pow10(-q)))
		}
		below = new(big.Int).Quo(scaled.Num(), scaled.Denom())
		n := digits(below)
		if n == f.prec && below.Sign() != 0 {
			break
		}
		if n > f.prec {
			q++
		} else {
			q--
		}
	}
	above = new(big.Int).Add(below, bigOne)
	if !scaled.IsInt() && digits(above) <= f.prec {
		return below, above, q
	}
	t.Fatalf("no bracket of %v at %d digits", r, f.prec)
	return nil, nil, 0
}

// TestFloatFromRangeEdges checks the decimals just below and above each binary
// value and each midpoint where the format's range ends: the largest finite
// value, the overflow threshold, the smallest normal, and the subnormals at
// the bottom of the range. Each is out of a 16-digit decimal's reach for
// float64, so decimal64 runs against float32 only here; TestFloatFromOracle
// covers it for float64.
func TestFloatFromRangeEdges(t *testing.T) {
	bits64 := func(v float64) uint64 { return math.Float64bits(v) }
	bits32 := func(v float64) uint64 { return uint64(math.Float32bits(float32(v))) }
	half := big.NewRat(1, 2)
	two := func(a, b float64) *big.Rat {
		return new(big.Rat).Mul(half, new(big.Rat).Add(floatFromRat(a), floatFromRat(b)))
	}
	type edge struct {
		name      string
		r         *big.Rat
		belowNear uint64
		belowT    uint64
		belowA    uint64
		aboveNear uint64
		aboveT    uint64
		aboveA    uint64
	}
	binaryEdges := func(bitsOf func(float64) uint64, max, prevMax, minNormal, maxSub, minSub, twoSub float64, inf uint64) []edge {
		m := bitsOf(max)
		p := bitsOf(prevMax)
		n := bitsOf(minNormal)
		ls := bitsOf(maxSub)
		s := bitsOf(minSub)
		ts := bitsOf(twoSub)
		over := new(big.Rat).Add(floatFromRat(max), new(big.Rat).Mul(half, new(big.Rat).Sub(floatFromRat(max), floatFromRat(prevMax))))
		return []edge{
			{"largest", floatFromRat(max), m, p, m, m, m, inf},
			{"overflow threshold", over, m, m, inf, inf, m, inf},
			{"smallest normal", floatFromRat(minNormal), n, ls, n, n, n, n + 1},
			{"subnormal top midpoint", two(maxSub, minNormal), ls, ls, n, n, ls, n},
			{"smallest subnormal", floatFromRat(minSub), s, 0, s, s, s, ts},
			{"half smallest subnormal", new(big.Rat).Mul(half, floatFromRat(minSub)), 0, 0, s, s, 0, s},
			{"midpoint of two lowest", two(minSub, twoSub), s, s, ts, ts, s, ts},
		}
	}
	float64Edges := binaryEdges(bits64, math.MaxFloat64, math.Nextafter(math.MaxFloat64, 0),
		0x1p-1022, math.Nextafter(0x1p-1022, 0), math.SmallestNonzeroFloat64, 2*math.SmallestNonzeroFloat64, 0x7FF0000000000000)
	float32Edges := binaryEdges(bits32, math.MaxFloat32, float64(math.Nextafter32(math.MaxFloat32, 0)),
		0x1p-126, float64(math.Nextafter32(0x1p-126, 0)), math.SmallestNonzeroFloat32, 2*math.SmallestNonzeroFloat32, 0x7F800000)

	run := func(t *testing.T, f *format, b binary, edges []edge) {
		for _, e := range edges {
			below, above, q := floatFromBracket(t, f, e.r)
			t.Run(e.name, func(t *testing.T) {
				floatFromExpect(t, f, below, q, b, e.belowNear, e.belowT, e.belowA)
				floatFromExpect(t, f, above, q, b, e.aboveNear, e.aboveT, e.aboveA)
			})
		}
	}
	t.Run("Float64From128", func(t *testing.T) { run(t, dec128, binary64, float64Edges) })
	t.Run("Float32From128", func(t *testing.T) { run(t, dec128, binary32, float32Edges) })
	t.Run("Float32From64", func(t *testing.T) { run(t, dec64, binary32, float32Edges) })

	// Values decided by the size of the coefficient alone, and their
	// neighbors across that decision: the same result as the exact path.
	for _, ff := range floatFromFormats {
		t.Run(ff.name+"/far", func(t *testing.T) {
			f, b := ff.f, ff.b
			maxCoeff := new(big.Int).Sub(pow10(f.prec), bigOne)
			for q := -500; q <= 400; q++ {
				if q < f.minExp || q > f.maxExp {
					continue
				}
				for _, c := range []*big.Int{bigOne, big.NewInt(9), maxCoeff} {
					for _, m := range floatFromModes {
						floatFromCheck(t, f, floatFromEncode(f, false, c, q), m, b)
						floatFromCheck(t, f, floatFromEncode(f, true, c, q), m, b)
					}
				}
			}
			for _, q := range []int{f.minExp, f.maxExp} {
				for _, m := range floatFromModes {
					floatFromCheck(t, f, floatFromEncode(f, false, bigOne, q), m, b)
					floatFromCheck(t, f, floatFromEncode(f, true, maxCoeff, q), m, b)
				}
			}
		})
	}
}

// TestFloatFromSpecials checks the special values, signed zeros in every
// cohort, non-canonical encodings and a Mode outside the five.
func TestFloatFromSpecials(t *testing.T) {
	const (
		qNaN64 = 0x7FF8000000000000
		inf64  = 0x7FF0000000000000
		qNaN32 = 0x7FC00000
		inf32  = 0x7F800000
	)
	check := func(name string, got, want uint64) {
		t.Helper()
		if got != want {
			t.Errorf("%s: got %#x, want %#x", name, got, want)
		}
	}
	allModes := append([]bid.Mode{7}, floatFromModes...)
	for _, m := range allModes {
		// decimal64.
		for _, sign := range []uint64{0, bid.SignMask64} {
			f64 := func(x uint64) uint64 { return math.Float64bits(bid.Float64From64(x, m)) }
			f32 := func(x uint64) uint64 { return uint64(math.Float32bits(bid.Float32From64(x, m))) }
			neg := sign != 0
			check("Float64From64(NaN)", f64(bid.NaN64|sign), qNaN64)
			check("Float32From64(NaN)", f32(bid.NaN64|sign), qNaN32)
			check("Float64From64(sNaN)", f64(bid.SNaN64|sign), qNaN64)
			check("Float32From64(sNaN)", f32(bid.SNaN64|sign), qNaN32)
			check("Float64From64(NaN with payload)", f64(bid.NaN64|sign|12345), qNaN64)
			check("Float64From64(Inf)", f64(bid.Inf64|sign), b2u(neg)<<63|inf64)
			check("Float32From64(Inf)", f32(bid.Inf64|sign), b2u(neg)<<31|inf32)
			check("Float64From64(Inf with non-zero trailing bits)", f64(bid.Inf64|sign|1), b2u(neg)<<63|inf64)
			for _, q := range []int{bid.MinExp64, -1, 0, 1, bid.MaxExp64} {
				x, ok := bid.Pack64(neg, 0, q)
				if !ok {
					t.Fatalf("Pack64(0, %d) failed", q)
				}
				check("Float64From64(zero)", f64(x), b2u(neg)<<63)
				check("Float32From64(zero)", f32(x), b2u(neg)<<31)
			}
			// A second-form encoding with a coefficient above MaxCoeff64
			// reads as a zero of its sign. Below 2^53 no first-form
			// coefficient can exceed MaxCoeff64.
			nc := sign | bid.Form2Mask64 | uint64(bid.Bias64)<<bid.Form2ExpShift64 | (1<<bid.Form2CoeffBits64 - 1)
			check("Float64From64(non-canonical)", f64(nc), b2u(neg)<<63)
			check("Float32From64(non-canonical)", f32(nc), b2u(neg)<<31)
		}
		// decimal128.
		for _, sign := range []uint64{0, bid.SignMask128Hi} {
			f64 := func(x bid.Uint128) uint64 { return math.Float64bits(bid.Float64From128(x, m)) }
			f32 := func(x bid.Uint128) uint64 { return uint64(math.Float32bits(bid.Float32From128(x, m))) }
			neg := sign != 0
			check("Float64From128(NaN)", f64(bid.Uint128{Hi: bid.NaN128Hi | sign}), qNaN64)
			check("Float32From128(NaN)", f32(bid.Uint128{Hi: bid.NaN128Hi | sign}), qNaN32)
			check("Float64From128(sNaN)", f64(bid.Uint128{Hi: bid.SNaN128Hi | sign}), qNaN64)
			check("Float32From128(sNaN)", f32(bid.Uint128{Hi: bid.SNaN128Hi | sign}), qNaN32)
			check("Float64From128(NaN with payload)", f64(bid.Uint128{Hi: bid.NaN128Hi | sign, Lo: 12345}), qNaN64)
			check("Float64From128(Inf)", f64(bid.Uint128{Hi: bid.Inf128Hi | sign}), b2u(neg)<<63|inf64)
			check("Float32From128(Inf)", f32(bid.Uint128{Hi: bid.Inf128Hi | sign}), b2u(neg)<<31|inf32)
			for _, q := range []int{bid.MinExp128, -1, 0, 1, bid.MaxExp128} {
				x, ok := bid.Pack128(neg, bid.Uint128{}, q)
				if !ok {
					t.Fatalf("Pack128(0, %d) failed", q)
				}
				check("Float64From128(zero)", f64(x), b2u(neg)<<63)
				check("Float32From128(zero)", f32(x), b2u(neg)<<31)
			}
			// Every second-form decimal128 is non-canonical.
			nc := bid.Uint128{Hi: sign | bid.Form2Mask128Hi | 1000<<bid.Form2ExpShift128 | 1, Lo: 1}
			check("Float64From128(non-canonical)", f64(nc), b2u(neg)<<63)
			check("Float32From128(non-canonical)", f32(nc), b2u(neg)<<31)
			// A first-form field above MaxCoeff128.
			nc = bid.Uint128{Hi: sign | uint64(bid.Bias128)<<bid.ExpShift128 | bid.MaxCoeff128Hi, Lo: bid.MaxCoeff128Lo + 1}
			check("Float64From128(non-canonical, first form)", f64(nc), b2u(neg)<<63)
			check("Float32From128(non-canonical, first form)", f32(nc), b2u(neg)<<31)
		}
	}
	// A Mode outside the five behaves as ToNearestEven.
	tie, _ := bid.Pack64(false, 9007199254740993, 0)
	if got, want := bid.Float64From64(tie, 7), bid.Float64From64(tie, bid.ToNearestEven); got != want {
		t.Errorf("mode 7 = %v, want %v", got, want)
	}
}

// floatFromDraw returns a random operand for the oracle: a coefficient of any
// length with an exponent near b's range, a tie between binary values, or
// a neighbor of a tie.
func floatFromDraw(g *gen, f *format, b binary) bits {
	r := g.r
	neg := r.IntN(2) == 0
	maxAdj := int(float64(b.emax)*0.30103) + 2
	minAdj := int(float64(b.emin-b.prec)*0.30103) - 2
	switch r.IntN(3) {
	case 0: // any coefficient, any scale in the range of b
		n := 1 + r.IntN(f.prec)
		c := g.below(pow10(n))
		adj := minAdj + r.IntN(maxAdj-minAdj+1)
		q := min(max(adj-digits(c), f.minExp), f.maxExp)
		return floatFromEncode(f, neg, c, q)
	case 1: // near the ends of the range
		c := g.below(pow10(f.prec))
		adj := [...]int{maxAdj - 3, minAdj + 3}[r.IntN(2)] + r.IntN(5) - 2
		q := min(max(adj-digits(c), f.minExp), f.maxExp)
		return floatFromEncode(f, neg, c, q)
	}
	// A tie (or within a unit in the last place of one) at a random scale.
	tie := g.below(new(big.Int).Lsh(bigOne, uint(b.prec)))
	tie.SetBit(tie, b.prec, 1).SetBit(tie, 0, 1)
	j := r.IntN(100) - 50
	c, q := tie, 0
	if j >= 0 {
		c = new(big.Int).Lsh(tie, uint(j))
	} else {
		c = new(big.Int).Mul(tie, new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(-j)), nil))
		q = j
	}
	for digits(c) < f.prec {
		c.Mul(c, big.NewInt(10))
		q--
	}
	if digits(c) > f.prec {
		return floatFromDraw(g, f, b)
	}
	c.Add(c, big.NewInt(int64(r.IntN(3)-1)))
	if digits(c) > f.prec {
		return floatFromDraw(g, f, b)
	}
	return floatFromEncode(f, neg, c, q)
}

// TestFloatFromOracle compares the kernel with the math/big.Float oracle, and
// the oracle with the reference model, on random operands in all modes.
func TestFloatFromOracle(t *testing.T) {
	n := 20000
	if testing.Short() {
		n = 2000
	}
	for _, ff := range floatFromFormats {
		t.Run(ff.name, func(t *testing.T) {
			f, b := ff.f, ff.b
			g := &gen{r: rand.New(rand.NewPCG(*seedFlag, streamSeed("floatfrom"+ff.name))), f: f}
			for range n {
				x := floatFromDraw(g, f, b)
				m := floatFromModes[g.r.IntN(len(floatFromModes))]
				floatFromCheck(t, f, x, m, b)
				if got, want := f.toFloat(x, m, b), floatFromWant(f, x, m, b); got != want {
					t.Errorf("reference and oracle differ on %s in mode %s: %#x, %#x", display(f, x), modeName(m), got, want)
				}
			}
		})
	}
}

// TestFloatFromAllocs checks that no path allocates: the fast path, the tie
// that needs the exact path, the subnormals, the range decisions, and the
// modes that always take the exact path.
func TestFloatFromAllocs(t *testing.T) {
	long := new(big.Int).Mul(big.NewInt(1<<53+1), new(big.Int).Exp(big.NewInt(5), big.NewInt(25), nil))
	maxCoeff := new(big.Int).Sub(pow10(34), bigOne)
	inputs128 := []bits{
		floatFromEncode(dec128, false, big.NewInt(1), 36),
		floatFromEncode(dec128, false, long, -25),
		floatFromEncode(dec128, true, new(big.Int).Add(long, bigOne), -25),
		floatFromEncode(dec128, false, maxCoeff, 0),
		floatFromEncode(dec128, false, maxCoeff, -360), // subnormal in float64
		floatFromEncode(dec128, false, maxCoeff, 274),  // near the largest
		floatFromEncode(dec128, true, maxCoeff, 6000),  // decided by size
		floatFromEncode(dec128, false, big.NewInt(0), -6000),
		{Hi: bid.NaN128Hi},
		{Hi: bid.Inf128Hi},
	}
	inputs64 := []uint64{
		floatFromEncode(dec64, false, big.NewInt(1), 36).Lo,
		floatFromEncode(dec64, false, big.NewInt(9007199254740993), 0).Lo,
		floatFromEncode(dec64, false, big.NewInt(9999999999999999), -320).Lo,
		floatFromEncode(dec64, true, big.NewInt(9999999999999999), 293).Lo,
		floatFromEncode(dec64, false, big.NewInt(0), 0).Lo,
		bid.NaN64,
		bid.Inf64,
	}
	var sink64 float64
	var sink32 float32
	for _, m := range floatFromModes {
		if n := testing.AllocsPerRun(100, func() {
			for _, x := range inputs64 {
				sink64 += bid.Float64From64(x, m)
			}
		}); n != 0 {
			t.Errorf("Float64From64 in mode %s: %v allocations", modeName(m), n)
		}
		if n := testing.AllocsPerRun(100, func() {
			for _, x := range inputs128 {
				sink64 += bid.Float64From128(x, m)
			}
		}); n != 0 {
			t.Errorf("Float64From128 in mode %s: %v allocations", modeName(m), n)
		}
		if n := testing.AllocsPerRun(100, func() {
			for _, x := range inputs64 {
				sink32 += bid.Float32From64(x, m)
			}
		}); n != 0 {
			t.Errorf("Float32From64 in mode %s: %v allocations", modeName(m), n)
		}
		if n := testing.AllocsPerRun(100, func() {
			for _, x := range inputs128 {
				sink32 += bid.Float32From128(x, m)
			}
		}); n != 0 {
			t.Errorf("Float32From128 in mode %s: %v allocations", modeName(m), n)
		}
	}
	_, _ = sink64, sink32
}

// floatFromFuzz is the body of the fuzz targets: the raw encoding of the
// input, and then a canonical value and a value at or beside a binary tie
// that the input selects within b's range, which raw encodings rarely reach.
func floatFromFuzz(t *testing.T, f *format, b binary, hi, lo uint64, e int16, sel uint8) {
	m := floatFromModes[int(sel&0x7f)%len(floatFromModes)]
	neg := sel&0x80 != 0

	raw := bits{Hi: hi, Lo: lo}
	if f == dec64 {
		raw.Hi = 0
	}
	floatFromCheck(t, f, raw, m, b)

	c := new(big.Int).Lsh(new(big.Int).SetUint64(hi), 64)
	c.Or(c, new(big.Int).SetUint64(lo))
	c.Mod(c, pow10(f.prec))
	minQ := int(float64(b.emin-b.prec)*0.30103) - f.prec - 3
	maxQ := int(float64(b.emax)*0.30103) + 3
	q := minQ + ((int(e)%(maxQ-minQ+1))+(maxQ-minQ+1))%(maxQ-minQ+1)
	if q >= f.minExp && q <= f.maxExp {
		floatFromCheck(t, f, floatFromEncode(f, neg, c, q), m, b)
	}

	// (2^prec + (lo mod 2^prec) | 1) × 2^j, plus an offset in the last digit.
	tie := new(big.Int).SetUint64(lo & (1<<b.prec - 1))
	tie.SetBit(tie, b.prec, 1).SetBit(tie, 0, 1)
	j := int(e) % 64
	q = 0
	if j >= 0 {
		tie.Lsh(tie, uint(j))
	} else {
		tie.Mul(tie, new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(-j)), nil))
		q = j
	}
	for digits(tie) < f.prec {
		tie.Mul(tie, big.NewInt(10))
		q--
	}
	tie.Add(tie, big.NewInt(int64(hi%3)-1))
	if digits(tie) <= f.prec && q >= f.minExp {
		floatFromCheck(t, f, floatFromEncode(f, neg, tie, q), m, b)
	}
}

func floatFromFuzzSeeds(f *testing.F) {
	f.Add(uint64(0), uint64(0), int16(0), uint8(0))
	f.Add(uint64(0), uint64(1), int16(36), uint8(0))
	f.Add(uint64(0), uint64(9007199254740993), int16(0), uint8(1))
	f.Add(uint64(0x0001ED09BEAD87C0), uint64(0x378D8E63FFFFFFFF), int16(-360), uint8(2))
	f.Add(uint64(0x0001ED09BEAD87C0), uint64(0x378D8E63FFFFFFFF), int16(274), uint8(0x83))
	f.Add(uint64(0x7C00000000000000), uint64(0), int16(1), uint8(4))
	f.Add(uint64(0x7E00000000000000), uint64(5), int16(-1), uint8(0x80))
}

func FuzzFloat64From64(f *testing.F) {
	floatFromFuzzSeeds(f)
	f.Fuzz(func(t *testing.T, hi, lo uint64, e int16, sel uint8) {
		floatFromFuzz(t, dec64, binary64, hi, lo, e, sel)
	})
}

func FuzzFloat64From128(f *testing.F) {
	floatFromFuzzSeeds(f)
	f.Fuzz(func(t *testing.T, hi, lo uint64, e int16, sel uint8) {
		floatFromFuzz(t, dec128, binary64, hi, lo, e, sel)
	})
}

func FuzzFloat32From64(f *testing.F) {
	floatFromFuzzSeeds(f)
	f.Fuzz(func(t *testing.T, hi, lo uint64, e int16, sel uint8) {
		floatFromFuzz(t, dec64, binary32, hi, lo, e, sel)
	})
}

func FuzzFloat32From128(f *testing.F) {
	floatFromFuzzSeeds(f)
	f.Fuzz(func(t *testing.T, hi, lo uint64, e int16, sel uint8) {
		floatFromFuzz(t, dec128, binary32, hi, lo, e, sel)
	})
}

// BenchmarkFloatFrom measures the paths: a short coefficient and a long one,
// which take the fast scaling path; a subnormal and a directed mode, which take the
// exact path; and an exponent decided by size.
func BenchmarkFloatFrom(b *testing.B) {
	long, _ := new(big.Int).SetString("1234567890123456789012345678901234", 10)
	cases := []struct {
		name string
		x    bits
		m    bid.Mode
	}{
		{"short", floatFromEncode(dec128, false, big.NewInt(12345), -2), bid.ToNearestEven},
		{"long", floatFromEncode(dec128, false, long, -30), bid.ToNearestEven},
		{"subnormal", floatFromEncode(dec128, false, long, -350), bid.ToNearestEven},
		{"directed", floatFromEncode(dec128, false, long, -30), bid.TowardZero},
		{"directed-extreme", floatFromEncode(dec128, false, long, 270), bid.TowardZero},
		{"far", floatFromEncode(dec128, false, long, 5000), bid.ToNearestEven},
	}
	var sink float64
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				sink += bid.Float64From128(c.x, c.m)
			}
		})
	}
	_ = sink
}
