// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zadd64.go and zadd128.go: addition and, by
// negating the second operand, subtraction. See arith_tmpl.go for what a
// template is and "Width parameterisation" in the package documentation for
// its rules.
package bid

import "math/bits"

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
//
// Two finite operands whose coefficients fit a word and whose exponents are
// at most maxWordGap apart take a fast path first. It computes their exact
// sum in two words at the smaller exponent, the preferred one, and
// packWordW encodes it if it fits the format; a sum that does not fit, which
// needs rounding, falls through to the general path. That is the exact
// packing exception roundW's comment allows, and the general path gives the
// same encoding. It scales the operand with the larger exponent by
// k = min(gap, addDigitsW - digits) places and shifts the other only if
// k < gap; then the scaled operand has at least addDigitsW = PrecisionW+3
// digits and the other is below 10^PrecisionW, so the sum or difference has
// more than PrecisionW digits and does not fit. When k = gap the general sum
// is this one, exact, at the same exponent, and roundW packs it unchanged.
// The sign follows the general path too: that of the operands when they
// agree, that of the larger magnitude when they differ, and for an exact
// cancellation the zero of "Sign of a result", negative only in
// TowardNegative.
func addW(x, y coefW, mode Mode) coefW {
	if xneg, xc, xe, ok := unpackWordW(x); ok {
		if yneg, yc, ye, ok := unpackWordW(y); ok {
			if xe < ye {
				xneg, xc, xe, yneg, yc, ye = yneg, yc, ye, xneg, xc, xe
			}
			if gap := uint(xe - ye); gap <= maxWordGap {
				hi, lo := bits.Mul64(xc, pow10tab64[gap])
				neg := xneg
				if xneg == yneg {
					var carry uint64
					lo, carry = bits.Add64(lo, yc, 0)
					hi += carry
				} else if hi == 0 && lo < yc {
					neg, lo = yneg, yc-lo
				} else {
					var borrow uint64
					lo, borrow = bits.Sub64(lo, yc, 0)
					hi -= borrow
					if hi|lo == 0 {
						neg = mode == TowardNegative
					}
				}
				if r, ok := packWordW(neg, hi, lo, ye-BiasW); ok {
					return r
				}
			}
		}
	}
	xneg, xc, xe, xok := unpackFirstW(x)
	yneg, yc, ye, yok := unpackFirstW(y)
	if !xok || !yok {
		var xk, yk Kind
		xk, xneg, xc, xe = unpackRestW(x)
		yk, yneg, yc, ye = unpackRestW(y)
		if xk != KindFinite || yk != KindFinite {
			switch {
			case xk == KindNaN || yk == KindNaN:
				return nanW(x, y, y)
			case xk != KindInf:
				return cInfW(yneg)
			case yk == KindInf && xneg != yneg:
				return cNaNW() // the infinities cancel
			}
			return cInfW(xneg)
		}
	}

	if xe < ye {
		xneg, xc, xe, yneg, yc, ye = yneg, yc, ye, xneg, xc, xe
	}
	exp, class := ye, classZero
	// A zero x has nothing to scale: the sum is y exactly, at the preferred
	// exponent ye, and shifting y would only lose digits of it.
	if gap := xe - ye; gap > 0 && !cIsZeroW(xc) {
		// A swamped y. Scaled by m places, x has exactly PrecisionW digits
		// X; if the gap exceeds m by more than the digits of a non-zero y,
		// y is a fraction f of one unit of X with 0 < f < 1/10, and the sum
		// is X + f or (X - 1) + (1 - f). Their classes are then known, below
		// a half and above it, so roundW may take them at face value with a
		// coefficient of PrecisionW digits (see its comment): it drops no
		// digit and divides nothing. X - 1 keeps PrecisionW digits unless X
		// is 10^(PrecisionW-1), whose difference would need padding, which a
		// face-value class does not survive. That X is taken one place
		// further instead, to 10^PrecisionW, if the gap leaves f below 1/10
		// there too, and the difference is 10^PrecisionW - 1 and 1 - 10f;
		// otherwise it takes the general path.
		if !cIsZeroW(yc) {
			m, dy := PrecisionW-cDigitsW(xc), cDigitsW(yc)
			if gap-m > dy {
				p, class := cMulPow10W(xc, m), classBelowHalf
				if xneg != yneg {
					class = classAboveHalf
					if cCmpW(p, cPow10W(PrecisionW-1)) == 0 {
						m++
						p = cPow10W(PrecisionW)
					}
					p, _ = cSubW(p, cPow10W(0))
				}
				if gap-m > dy {
					r, _ := roundW(xneg, wideW{Lo: p}, xe-m, class, mode, MinExpW)
					return r
				}
			}
		}
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
