// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zfma64.go and zfma128.go: the fused
// multiply-add. See arith_tmpl.go for what a template is and "Width
// parameterisation" in the package documentation for its rules.
package bid

import "math/bits"

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
// Three finite operands whose coefficients fit a word, and whose product
// fits a word too, take a fast path first, and if it refuses them the
// general path starts from the operands as the fast path decoded them, with
// the exact product it already holds. The path is the word fast path of
// addSlowW over the product, at the sum of the exponents, and z: the operand
// with the larger exponent is scaled across a gap of at most maxWordGap
// places, the exact sum or difference is formed in two words at the smaller
// exponent, which is the preferred one min(Q(x)+Q(y), Q(z)), and packWordW
// encodes it if it fits the format. That is the exact packing exception
// roundW's comment allows, and the general path gives the same encoding: with
// the product below 2^64 and the gap at most 19 the scaled operand has at
// most 39 digits, within fmaDigitsW for decimal128, so the general path
// scales it by the whole gap, shifts nothing and rounds the same exact sum at
// the same exponent, which roundW packs unchanged when it fits; for decimal64
// a scaled operand of more than fmaDigitsW digits is above 10^35 and no sum
// or difference with a word is below 10^16, so packWordW refuses every pair
// on which the general path shifts. The sign is the general path's: that of
// the operands when they agree, that of the larger magnitude when they
// differ, and for an exact cancellation the zero of "Sign of a result",
// negative only in TowardNegative. A sum whose exponent lies outside the
// format is refused too, and the general path clamps or folds it.
// TestFMAWordPath drives the bounds against the reference.
//
// A product of two words that the path above does not finish, because z
// is more than maxWordGap places away or the sum does not fit, is still
// exact whenever it fits the format, and then the result is the sum of
// the product and z by addSlowW: every rule of the sum of two values,
// the preferred exponent min(Q(x)+Q(y), Q(z)) and the signs of zeros
// included, is the rule for x × y + z with an exact product. A product
// that does not fit the format takes the general path.
//
// The general path is addW over the wide type. The product is computed
// exactly and is one operand; the other is z. Whichever has the larger
// exponent, the product or z, is scaled up towards the exponent of the
// other, as far as fmaDigitsW allows. If that reaches it the sum is exact
// and its exponent is the preferred one. If it does not, the gap's remainder
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
	var yneg, zneg, yok, zok bool
	var yw, zw uint64
	var ye, ze int
	xneg, xw, xe, xok := unpackWordCanonW(x)
	if xok {
		yneg, yw, ye, yok = unpackWordCanonW(y)
	}
	if yok {
		zneg, zw, ze, zok = unpackWordCanonW(z)
	}
	// a is the operand with the larger exponent, and b the other.
	var a wideW
	var aneg, bneg bool
	var ae, be int
	var zc coefW
	if zok {
		phi, plo := bits.Mul64(xw, yw)
		pneg, pe := xneg != yneg, xe+ye-BiasW
		if phi == 0 {
			// h is the operand with the larger exponent, and l the other.
			hneg, hw, he, lneg, lw, le := pneg, plo, pe, zneg, zw, ze
			if he < le {
				hneg, hw, he, lneg, lw, le = lneg, lw, le, hneg, hw, he
			}
			if gap := uint(he - le); gap <= maxWordGap {
				hi, lo := bits.Mul64(hw, pow10tab64[gap])
				neg := hneg
				if hneg == lneg {
					var carry uint64
					lo, carry = bits.Add64(lo, lw, 0)
					hi += carry
				} else if hi == 0 && lo < lw {
					neg, lo = lneg, lw-lo
				} else {
					var borrow uint64
					lo, borrow = bits.Sub64(lo, lw, 0)
					hi -= borrow
					if hi|lo == 0 {
						neg = mode == TowardNegative
					}
				}
				if r, ok := packWordW(neg, hi, lo, le-BiasW); ok {
					return r
				}
			}
		}
		// A product that is itself a value of the format, a coefficient of
		// at most PrecisionW digits at an exponent in range, is exact,
		// and then x × y + z rounded once is the sum of that value and z:
		// addSlowW, with its one shift and its face-value rounding, where
		// the general path below scales the product to fmaDigitsW digits
		// and divides a double-width sum back. The words of the product
		// encode by the exact packing exception, as the sum above does.
		if p, ok := packWordW(pneg, phi, plo, pe-BiasW); ok {
			return addSlowW(p, z, mode)
		}
		a, aneg, ae = cFromU128W(Uint128{Hi: phi, Lo: plo}), pneg, pe-BiasW
		zc, bneg, be = cFromU64W(zw), zneg, ze-BiasW
	} else {
		var xc, yc coefW
		xneg, xc, xe, xok = unpackFirstW(x)
		xk := KindFinite
		if !xok {
			xk, xneg, xc, xe = unpackRestW(x)
		}
		yneg, yc, ye, yok = unpackFirstW(y)
		yk := KindFinite
		if !yok {
			yk, yneg, yc, ye = unpackRestW(y)
		}
		zneg, zc, ze, zok = unpackFirstW(z)
		zk := KindFinite
		if !zok {
			zk, zneg, zc, ze = unpackRestW(z)
		}
		pneg := xneg != yneg
		if xk != KindFinite || yk != KindFinite || zk != KindFinite {
			switch {
			case xk == KindNaN || yk == KindNaN || zk == KindNaN:
				return nanW(x, y, z)
			case xk == KindFinite && yk == KindFinite:
				return cInfW(zneg) // a finite product and an infinite z
			case xk == KindFinite && cIsZeroW(xc) || yk == KindFinite && cIsZeroW(yc):
				return cNaNW() // 0 × ∞, of any exponent
			case zk == KindInf && zneg != pneg:
				return cNaNW() // the infinities cancel
			}
			return cInfW(pneg)
		}
		a, aneg, ae = cMulFullW(xc, yc), pneg, xe+ye
		bneg, be = zneg, ze
	}
	b := wideW{Lo: zc}
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
