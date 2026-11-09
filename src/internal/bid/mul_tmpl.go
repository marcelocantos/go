// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zmul64.go and zmul128.go, the
// multiplication family, written in the vocabulary of "Width
// parameterisation" in the package documentation (see arith_tmpl.go).
// Nothing below the package clause, in code or in a comment, may end an
// identifier with a capital W unless it is meant to be replaced.
package bid

import "math/bits"

// mulW returns x × y rounded in mode. Word operands with a short product
// are multiplied inline by mulShortW, and every other operand takes
// mulSlowW; see "Inline fast paths" in the package documentation.
func mulW(x, y coefW, mode Mode) coefW {
	if r, ok := mulShortW(x, y); ok {
		return r
	}
	return mulSlowW(x, y, mode)
}

// mulSlowW returns x × y rounded in mode. The special values follow "NaN and
// infinity operands" and "Sign of a result"; a finite product is computed
// exactly in the double-width type, whose digits roundW reduces with a sticky
// class of everything it drops, so the product is rounded once and nothing
// is lost to a narrower intermediate. The preferred exponent is the sum of
// the exponents, which can lie far outside the format (the extremes are
// 2×MinExpW and 2×MaxExpW); roundW is specified for any exponent, clamps it
// and folds, pads or overflows the result as the contract says.
//
// Two finite operands whose coefficients fit a word take a fast path first,
// and if it refuses them the general path starts from the operands as it
// decoded them, so that a refusal costs only the attempt:
// their product is exact in two words, its preferred exponent is the sum of
// the exponents, and packWordW encodes it if both fit the format. That is
// the exact packing exception roundW's comment allows, and roundW would
// encode the same value unchanged: a product of at most PrecisionW digits at
// an exponent within the format is exact there, and the sign of a zero
// product is the exclusive or of the signs either way. A product or
// exponent outside the format falls through.
func mulSlowW(x, y coefW, mode Mode) coefW {
	var yneg, yok bool
	var yw uint64
	var ye int
	xneg, xw, xe, xok := unpackWordCanonW(x)
	if xok {
		yneg, yw, ye, yok = unpackWordCanonW(y)
	}
	var xc, yc coefW
	if yok {
		hi, lo := bits.Mul64(xw, yw)
		if r, ok := packWordW(xneg != yneg, hi, lo, xe+ye-2*BiasW); ok {
			return r
		}
		xc, xe, yc, ye = cFromU64W(xw), xe-BiasW, cFromU64W(yw), ye-BiasW
	} else {
		xneg, xc, xe, xok = unpackFirstW(x)
		yneg, yc, ye, yok = unpackFirstW(y)
		if !xok || !yok {
			var xk, yk Kind
			xk, xneg, xc, xe = unpackRestW(x)
			yk, yneg, yc, ye = unpackRestW(y)
			if xk != KindFinite || yk != KindFinite {
				// A NaN operand gives that NaN, an infinity times a zero of
				// any exponent (a non-canonical operand reads as a zero) the
				// canonical NaN, and an infinity times anything else the
				// infinity of the product's sign.
				if xk == KindNaN || yk == KindNaN {
					return nanW(x, y, y)
				}
				if xk == KindFinite && cIsZeroW(xc) || yk == KindFinite && cIsZeroW(yc) {
					return cNaNW()
				}
				return cInfW(xneg != yneg)
			}
		}
	}
	neg := xneg != yneg
	r, _ := roundW(neg, cMulFullW(xc, yc), xe+ye, classZero, mode, MinExpW)
	return r
}
