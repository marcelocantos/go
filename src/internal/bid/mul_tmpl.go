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

// mulW returns x × y rounded in mode. The special values follow "NaN and
// infinity operands" and "Sign of a result"; a finite product is computed
// exactly in the double-width type, whose digits roundW reduces with a sticky
// class of everything it drops, so the product is rounded once and nothing
// is lost to a narrower intermediate. The preferred exponent is the sum of
// the exponents, which can lie far outside the format (the extremes are
// 2×MinExpW and 2×MaxExpW); roundW is specified for any exponent, clamps it
// and folds, pads or overflows the result as the contract says.
func mulW(x, y coefW, mode Mode) coefW {
	xk, xneg, xc, xe := UnpackW(x)
	yk, yneg, yc, ye := UnpackW(y)
	neg := xneg != yneg
	if xk != KindFinite || yk != KindFinite {
		// A NaN operand gives NaN, and so does an infinity times a zero of
		// any exponent (a non-canonical operand reads as a zero); an
		// infinity times anything else is the infinity of the product's sign.
		if xk == KindNaN || yk == KindNaN ||
			xk == KindFinite && cIsZeroW(xc) || yk == KindFinite && cIsZeroW(yc) {
			return cNaNW()
		}
		return cInfW(neg)
	}
	r, _ := roundW(neg, cMulFullW(xc, yc), xe+ye, classZero, mode, MinExpW)
	return r
}
