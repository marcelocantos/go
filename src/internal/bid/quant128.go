// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// quantizeSlow64 is the quantize of decimal64 operands that quantize64
// did not decide: quantizeLong64 is all of it.
func quantizeSlow64(x, y uint64, mode Mode) uint64 {
	return quantizeLong64(x, y, mode)
}

// quantizeSlow128 returns x with the exponent of y, rounded in mode. A
// finite x whose coefficient has two words, with a finite y, is decided
// here, and every other operand goes to quantizeLong128, which has the
// word paths and decides the rest from the start. That is the rounding of
// a computed value to its unit: what a division or a long product left,
// brought to two places.
//
// With the exponent of x below that of y by d places, the coefficient
// loses its d lowest digits. As in division it is biased by half of 10^d
// before it is divided, so that the quotient is the coefficient rounded
// half up and a zero remainder is the tie, which takes the even neighbour
// by clearing the low bit. More than 34 places leave nothing: half of
// 10^35 exceeds every coefficient. That is to nearest; a directed mode is
// quantizeLong128's.
//
// With the exponent of x above that of y the coefficient is scaled up, and
// fits only if it is below 10^(34-gap): a coefficient of two words is
// above 10^19, so most such requests are refused, by one comparison, with
// the NaN that quantizeLong128 returns for them.
//
// The compiler calls this function directly, so its parameters and result
// are a contract with it: see the end of "Inline fast paths" in the package
// documentation.
func quantizeSlow128(x, y Uint128, mode Mode) Uint128 {
	xhi, _, xe, ye, ok := addPair128(x, y)
	xc := Uint128{Hi: xhi, Lo: x.Lo}
	// A coefficient that is not canonical is a zero, and a word has its
	// own paths.
	if !ok || xhi == 0 || u128GtMaxCoeff(xc) {
		return quantizeLong128(x, y, mode)
	}
	neg := x.Hi>>63 != 0
	switch gap := xe - ye; {
	case gap == 0:
		return x
	case gap > 0:
		if gap >= Precision128 || !u128Lt(xc, pow10tab128[Precision128-gap]) {
			return cNaN128()
		}
		xc = u128MulLow(xc, pow10tab128[gap])
	case mode > ToNearestAway:
		return quantizeLong128(x, y, mode)
	case gap < -Precision128:
		xc = Uint128{}
	default:
		d := -gap
		t, _ := u128Add(xc, u128Shr(pow10tab128[d], 1))
		q, r := divPow10Recip128(t, d)
		if r == (Uint128{}) && mode == ToNearestEven {
			q.Lo &^= 1 // a tie: the even neighbour is the one below
		}
		xc = q
	}
	if r, ok := packWord128(neg, xc.Hi, xc.Lo, ye-Bias128); ok {
		return r
	}
	return quantizeLong128(x, y, mode)
}
