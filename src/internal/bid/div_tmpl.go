// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zdiv64.go and zdiv128.go, written once in
// the vocabulary of "Width parameterisation" in the package documentation:
// every identifier that depends on the width ends in a capital W, and the
// generator (gen_arith.go) replaces that W with 64 or 128. Nothing else is
// substituted. This file is not compiled; the build ignores it, and only
// the generated files are compiled. Everything from the build constraint
// to the package clause is commentary on the template and is dropped from
// the generated files, which nobody edits.
//
// Nothing below the package clause, in code or in a comment, may end an
// identifier with that capital letter unless it is meant to be replaced.
package bid

// divW is DivW: x / y rounded once in mode, with the special cases of
// "NaN and infinity operands" and the zero and infinity results of
// "Exponent of a result" and "Sign of a result" in the package
// documentation.
//
// The finite case is long division in one step. With dx and dy the digit
// counts of the coefficients, the dividend is scaled by
//
//	k = PrecisionW + 1 - dx + dy
//
// places, so that the integer quotient q = ⌊xc × 10^k / yc⌋ has at least
// PrecisionW+1 digits: xc × 10^k is at least 10^(dx-1+k) and yc is below
// 10^dy, so q is at least 10^(dx-1+k-dy) = 10^PrecisionW; and xc × 10^k is
// below 10^(dx+k) while yc is at least 10^(dy-1), so q is below
// 10^(PrecisionW+2). The quotient thus has PrecisionW+1 or PrecisionW+2
// digits, and the remainder, which is exact, supplies the sticky bit for
// everything below them. That meets the contract of roundW: at least one
// digit of q is discarded, and the discarded digits and the remainder
// combine exactly into one class. The digit counts bound the widths: dx is
// at most PrecisionW and dy at least 1, so k is between 2 and 2×PrecisionW,
// the scaled dividend has at most 2×PrecisionW+1 digits (33 for decimal64,
// below 2^128; 69 for decimal128, below 10^77 and so below 2^256), and the
// quotient, with at most PrecisionW+2 digits (18 and 36), fits a
// coefficient. cDivFullW is the 128-by-64 or 256-by-128 division whose
// quotient is known to fit.
//
// The exponent of q × 10^-k is xe - ye - k, and for an inexact result that
// is what roundW rounds at: the result then takes the least exponent, as
// every inexact result does. An exact result (the remainder is zero) must
// instead take the exponent closest to the preferred one, xe - ye, among
// those at which it is representable, so the trailing zeros of q, which
// the scaling introduced, are stripped: at most k of them, so the exponent
// never passes xe - ye, and only when the remainder is zero, so that no
// non-zero fraction is lost. roundW then raises the exponent further only
// if the stripped q still has more than PrecisionW digits, which is the
// cohort rule. The strip is a binary search on the number of zeros, as in
// keyW: q has at most PrecisionW+2 digits, so at most PrecisionW+1 trailing
// zeros, and the steps start at the largest power of two not above the
// smaller of k and PrecisionW+1, which keeps every power of ten within the
// coefficient table.
func divW(x, y coefW, mode Mode) coefW {
	xk, xneg, xc, xe := UnpackW(x)
	yk, yneg, yc, ye := UnpackW(y)
	neg := xneg != yneg
	switch {
	case xk == KindNaN || yk == KindNaN:
		return cNaNW()
	case xk == KindInf:
		if yk == KindInf {
			// ∞ / ∞ is invalid (IEEE 754-2008 clause 7.2 item e).
			return cNaNW()
		}
		return cInfW(neg)
	case yk == KindInf:
		// x / ∞ is a zero at MinExpW ("Exponent of a result").
		r, _ := roundW(neg, wideW{}, MinExpW, classZero, mode, MinExpW)
		return r
	case cIsZeroW(yc):
		if cIsZeroW(xc) {
			// 0 / 0 is invalid (clause 7.2 item e).
			return cNaNW()
		}
		// Division by zero (clause 7.3).
		return cInfW(neg)
	case cIsZeroW(xc):
		// A zero quotient takes the preferred exponent, clamped by roundW.
		r, _ := roundW(neg, wideW{}, xe-ye, classZero, mode, MinExpW)
		return r
	}

	k := PrecisionW + 1 - cDigitsW(xc) + cDigitsW(yc)
	q, rem := cDivFullW(cMulPow10FullW(xc, k), yc)
	class := classBelowHalf
	if cIsZeroW(rem) {
		class = classZero
		step := 1
		for step*2 <= min(k, PrecisionW+1) {
			step *= 2
		}
		for ; step > 0; step /= 2 {
			if step > k {
				continue
			}
			if sq, sr := cDivPow10W(q, step); cIsZeroW(sr) {
				q, k = sq, k-step
			}
		}
	}
	r, _ := roundW(neg, wideW{Lo: q}, xe-ye-k, class, mode, MinExpW)
	return r
}
