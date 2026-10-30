// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zfma64.go and zfma128.go: the fused
// multiply-add. See arith_tmpl.go for what a template is and "Width
// parameterisation" in the package documentation for its rules.
package bid

// fmaDigitsW is the number of digits fmaW scales a value up to. Three facts
// fix it.
//
// Fit. The aligned values and their sum must fit the wide type: this is 35
// digits for decimal64 and 71 for decimal128, and 2×10^35 < 2^128, 2×10^71 < 2^256
// (10^38 and 10^77 are the largest powers below those).
//
// Reach. No value may start above it, or the scale count would be negative:
// the product has at most 2×PrecisionW digits and the addend PrecisionW.
//
// The tail. A value is shifted, and leaves a fraction, only when the other has
// been scaled to exactly this many digits. What is shifted has at most
// 2×PrecisionW digits and loses at least one, so with the borrow of a
// difference it is at most 10^(2×PrecisionW-1). What remains therefore has at
// least 2×PrecisionW+2 digits, more than the PrecisionW that roundW requires
// of a value with a discarded tail. 2×PrecisionW+1 is the least that keeps a
// non-zero difference; the two digits above it are the margin addW also keeps.
const fmaDigitsW = 2*PrecisionW + 3

// fmaW returns x × y + z rounded once in mode.
//
// It is addW over the wide type. The product is computed exactly and is one
// operand; the other is z. Whichever has the larger exponent, the product or
// z, is scaled up towards the exponent of the other, as far as fmaDigitsW
// allows. If that reaches it the sum is exact and its exponent is the
// preferred one, min(Q(x)+Q(y), Q(z)). If it does not, the gap's remainder
// shifts the other operand right, leaving a fraction f of one unit whose
// class is known exactly, and the sum still reaches roundW as an exact value
// and that class, so nothing is rounded before roundW rounds.
//
// A value of more than PrecisionW digits at an exponent above the preferred
// one is not a loss: if it turns out exact, it has the digits it needs, and
// roundW takes the member of the cohort nearest the preferred exponent, which
// is the one with PrecisionW digits.
//
// A difference with a fraction is hi - (lo + f), and it reaches roundW as the
// integer hi - (lo + 1) and the fraction 1 - f, for the reason given at addW.
//
// A zero product or a zero z has nothing to scale, so the sum is the other
// operand at the exponent of the smaller, and two zeros are a zero at the
// preferred exponent; the signs follow "Sign of a result".
func fmaW(x, y, z coefW, mode Mode) coefW {
	xk, xneg, xc, xe := UnpackW(x)
	yk, yneg, yc, ye := UnpackW(y)
	zk, zneg, zc, ze := UnpackW(z)
	pneg := xneg != yneg
	if xk != KindFinite || yk != KindFinite || zk != KindFinite {
		switch {
		case xk == KindNaN || yk == KindNaN || zk == KindNaN:
			return cNaNW()
		case xk == KindFinite && yk == KindFinite:
			return cInfW(zneg) // a finite product and an infinite z
		case xk == KindFinite && cIsZeroW(xc) || yk == KindFinite && cIsZeroW(yc):
			return cNaNW() // 0 × ∞, of any exponent
		case zk == KindInf && zneg != pneg:
			return cNaNW() // the infinities cancel
		}
		return cInfW(pneg)
	}

	// a is the operand with the larger exponent, and b the other.
	a, aneg, ae := cMulFullW(xc, yc), pneg, xe+ye
	b, bneg, be := wideW{Lo: zc}, zneg, ze
	if ae < be {
		a, aneg, ae, b, bneg, be = b, bneg, be, a, aneg, ae
	}
	exp, class := be, classZero
	if gap := ae - be; gap > 0 && a != (wideW{}) {
		k := min(gap, fmaDigitsW-cLenFullW(a))
		a, _ = cScaleFullW(a, k)
		if k < gap {
			exp = ae - k
			b, class = cShiftFullW(b, gap-k)
		}
	}

	neg := aneg
	var sum wideW
	if aneg == bneg {
		sum, _ = cAddFullW(a, b)
	} else {
		if class != classZero {
			b, class = cIncFullW(b), class.complement()
		}
		var borrow bool
		sum, borrow = cSubFullW(a, b)
		switch {
		case borrow:
			// Only an exact sum can borrow: a shifted operand is the smaller
			// by at least two places.
			sum, _ = cSubFullW(wideW{}, sum)
			neg = !aneg
		case sum == (wideW{}):
			neg = mode == TowardNegative // an exact cancellation
		}
	}
	r, _ := roundW(neg, sum, exp, class, mode, MinExpW)
	return r
}
