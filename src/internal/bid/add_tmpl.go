// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zadd64.go and zadd128.go: addition and, by
// negating the second operand, subtraction. See arith_tmpl.go for what a
// template is and "Width parameterisation" in the package documentation for
// its rules.
package bid

// addDigitsW is the number of digits addW scales a coefficient up to. Two
// bounds fix it. The sum of two operands aligned this way must fit the
// coefficient type, so no addition needs the wide type: 10^19 + 10^16 and
// 10^37 + 10^34 are below 2^64 and 2^128. And an operand scaled this far must
// keep more than PrecisionW digits when the other, shifted, is taken from it,
// which is what roundW requires of a value with a discarded tail: PrecisionW
// plus two digits would do, because the shifted operand is at most
// 10^PrecisionW, a borrow included.
const addDigitsW = PrecisionW + 3

// addW returns x + y rounded in mode.
//
// The operand with the larger exponent is scaled up to the exponent of the
// other if that takes it to addDigitsW digits or fewer, and then the sum is
// exact and its exponent is the preferred one. Otherwise, however large the
// gap between the exponents, the larger operand is scaled to addDigitsW
// digits and the smaller is shifted right by the rest of the gap, leaving a
// fraction f of one unit whose class is known exactly. If the rest of the gap
// exceeds the digits of the smaller operand, the shift leaves nothing but f,
// and a sticky bit is all that remains of that operand.
//
// A difference with such a fraction is hi - (lo + f), and it reaches roundW
// as the integer hi - (lo + 1) and the fraction 1 - f. Dropping f instead
// would leave x unchanged where x - ε must land below x in a directed mode.
func addW(x, y coefW, mode Mode) coefW {
	xk, xneg, xc, xe := UnpackW(x)
	yk, yneg, yc, ye := UnpackW(y)
	if xk != KindFinite || yk != KindFinite {
		switch {
		case xk == KindNaN || yk == KindNaN:
			return cNaNW()
		case xk != KindInf:
			return cInfW(yneg)
		case yk == KindInf && xneg != yneg:
			return cNaNW() // the infinities cancel
		}
		return cInfW(xneg)
	}

	if xe < ye {
		xneg, xc, xe, yneg, yc, ye = yneg, yc, ye, xneg, xc, xe
	}
	exp, class := ye, classZero
	// A zero x has nothing to scale: the sum is y exactly, at the preferred
	// exponent ye, and shifting y would only lose digits of it.
	if gap := xe - ye; gap > 0 && !cIsZeroW(xc) {
		k := min(gap, addDigitsW-cDigitsW(xc))
		xc = cMulPow10W(xc, k)
		if k < gap {
			exp = xe - k
			yc, class = cShiftW(yc, gap-k)
		}
	}

	neg := xneg
	var sum coefW
	if xneg == yneg {
		sum, _ = cAddW(xc, yc)
	} else {
		if class != classZero {
			yc, class = cIncW(yc), class.complement()
		}
		var borrow bool
		sum, borrow = cSubW(xc, yc)
		switch {
		case borrow:
			// Only an exact sum can borrow: a shifted operand is the smaller
			// by two places at least.
			var zero coefW
			sum, _ = cSubW(zero, sum)
			neg = !xneg
		case cIsZeroW(sum):
			neg = mode == TowardNegative // an exact cancellation
		}
	}
	r, _ := roundW(neg, wideW{Lo: sum}, exp, class, mode, MinExpW)
	return r
}
