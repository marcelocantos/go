// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zfromfloat64.go and zfromfloat128.go, the
// conversion of a binary floating-point value to each decimal width. It is
// written in the vocabulary of "Width parameterisation" in the package
// documentation: every identifier that depends on the width ends in a
// capital W, and the generator (gen_arith.go) replaces that W with 64 or
// 128. Nothing else is substituted. This file is not compiled; the build
// ignores it, and only the generated files are compiled. Everything from
// the build constraint to the package clause is commentary on the template
// and is dropped from the generated files, which nobody edits.
//
// Nothing below the package clause, in code or in a comment, may end an
// identifier with that capital letter unless it is meant to be replaced.
package bid

import "math/bits"

// fromFloatW converts the binary floating-point value of kind kind and, when
// finite, (-1)^neg × m × 2^e, to the decimal format, rounded once in mode
// from that exact value with preferred exponent 0 ("Binary floating-point
// conversions" in the package documentation). The arithmetic and the
// argument that its sticky bit and tie decisions are exact are in
// fromfloat.go; this function chooses the digit count and hands the result
// to the rounding routine. A NaN of either kind is the canonical NaN and an
// infinity keeps its sign; m and e are ignored for them.
//
// Precondition: m < 2^53 and e is a float64 exponent, in [-1074, 1023]
// before the trailing zeros of m are moved into it; a float32 satisfies
// both. The bounds of floatNat and of log10Pow2 rest on these.
func fromFloatW(kind Kind, neg bool, m uint64, e int, mode Mode) coefW {
	switch kind {
	case KindNaN:
		return cNaNW()
	case KindInf:
		return cInfW(neg)
	}
	if m == 0 {
		// A zero keeps its sign and takes the preferred exponent.
		c, _ := roundW(neg, wideW{}, 0, classZero, mode, MinExpW)
		return c
	}
	// Make m odd, so that m × 5^s is odd for every s: the value m × 5^s / 2^r
	// is then an integer exactly when r <= 0, which is the sticky bit.
	tz := bits.TrailingZeros64(m)
	m >>= tz
	e += tz

	// With n the exponent of the leading bit of v, 10^t <= 2^n <= v < 2^(n+1)
	// < 2 × 10^(t+1), so the adjusted exponent of v is t or t+1. Choosing
	// s = PrecisionW - t makes q = ⌊v × 10^s⌋ a number of PrecisionW+1 or
	// PrecisionW+2 digits: more than the precision, as roundW requires of
	// a value whose sticky bit stands for an unknown fraction, and below
	// 10^(PrecisionW+2) < 2^120, so it fits a Uint128 at either width.
	t := log10Pow2(bits.Len64(m) + e - 1)
	s := PrecisionW - t
	var q Uint128
	exp := 0
	class := classZero
	switch {
	case e >= 0 && bits.Len64(m)+e <= 128:
		// v is an integer that fits a Uint128: the rounding routine takes
		// it whole and reduces it exactly, with the preferred exponent.
		q = u128Shl(Uint128{Lo: m}, uint(e))
	case e >= 0:
		// v is a larger integer, and q = ⌊v / 10^k⌋ = ⌊m × 2^(e-k) / 5^k⌋
		// with the sticky bit the exact remainder of that division (the two
		// quotients are of one rational). k < e holds: this branch needs
		// bits.Len64(m) + e > 128, so e >= 76, and k = t - PrecisionW with
		// t <= log10(2^(53+e)) < 0.302 × (53 + e), which is below e + 16
		// for every e >= 0 and so below e + PrecisionW.
		k := -s
		var x floatNat
		x.setShl(m, e-k)
		if x.divPow5(k) {
			class = classBelowHalf
		}
		q = x.lo128()
		exp = k
	case -e <= s:
		// v = m × 5^(-e) × 10^e exactly, and m × 5^(-e) <= v × 10^s fits a
		// Uint128. Its last digit is not zero (the product is odd), so the
		// exponent e is the one closest to the preferred exponent 0 among
		// the exact value's encodings, and roundW drops digits only when
		// there are more than PrecisionW of them.
		var x floatNat
		x.setMulPow5(m, -e)
		q = x.lo128()
		exp = e
	default:
		// v × 10^s = m × 5^s / 2^r with r > 0 and an odd numerator: not an
		// integer, so the sticky bit is set and q is the exact quotient.
		var x floatNat
		x.setMulPow5(m, s)
		q = x.shr128(-e - s)
		exp = -s
		class = classBelowHalf
	}
	if class != classZero && !u128GePow10(q, PrecisionW) {
		// The digit estimate is exact, so q has at least PrecisionW+1 digits
		// whenever it is inexact; fewer would break roundW's contract.
		panic(errFromFloatDigits)
	}
	c, _ := roundW(neg, cFromU128W(q), exp, class, mode, MinExpW)
	return c
}
