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
// A divisor coefficient 10^k needs no long division: the exact quotient is
// xc at exponent xe-ye-k. Strip at most k coefficient zeros to approach the
// preferred exponent xe-ye, then let roundW handle exponent bounds. This
// includes a coefficient of one without discarding any of x's quantum.
//
// The general finite case is long division in one step. With dx and dy the
// digit counts of the coefficients, the dividend is scaled by
//
//	k = PrecisionW - dx + dy - lead
//
// places, where lead is 1 when the leading ratio xc × 10^(dy-dx) reaches
// yc and 0 otherwise, so that the integer quotient q = ⌊xc × 10^k / yc⌋
// has exactly PrecisionW digits: xc × 10^k / yc lies in
// [10^(dx+k-dy-1), 10^(dx+k-dy+1)) and reaches 10^(dx+k-dy) exactly when
// the leading ratio does. The remainder is exact, and so is the class of
// the discarded fraction rem / yc: below a half, a half or above it as rem
// compares with yc - rem. The digit counts bound the widths: dx is at most
// PrecisionW and dy at least 1, so k is between 0 and 2×PrecisionW-1, the
// scaled dividend has at most 2×PrecisionW digits (32 for decimal64, below
// 2^128; 68 for decimal128, below 10^77 and so below 2^256), and the
// quotient fits a coefficient. cDivFullW is the 128-by-64 or 256-by-128
// division whose quotient is known to fit; the leading comparison
// multiplies a coefficient up to the other's digit count, at most
// PrecisionW digits, which fits too.
//
// An inexact quotient inside the exponent range of roundFaceW's proof
// (MinExpW < exp <= MaxExpW, adjusted exponent at least EminW) is rounded
// at face value: the one row of the rounding table, with the carry of the
// largest coefficient one place up. This is the second use of the
// face-value exception, after the swamped addend of addSlowW ("Face-value
// rounding" in the package documentation): it saves the reduction of a
// longer quotient by a reciprocal division and its digit count, which were
// a third of a division. In the two nearest modes the row is not even
// looked up: the dividend is biased by half the divisor and the quotient
// comes out rounded, with the exact quotient and the tie told apart by the
// biased remainder (the code below, and the same section). A directed mode
// takes the class of the remainder, rem against yc - rem. Nearer the exponent limits the
// result may be subnormal, clamped or overflowing, which only roundW
// decides, so one more division step supplies a round digit and a sticky
// bit, and roundW rounds q × 10 + d at exponent xe - ye - k - 1 as it
// rounds every other inexact result with a discarded digit.
//
// The exponent of q × 10^-k is xe - ye - k, and for an inexact result that
// is what is rounded at: the result then takes the least exponent, as
// every inexact result does. An exact result (the remainder is zero) must
// instead take the exponent closest to the preferred one, xe - ye, among
// those at which it is representable, so the trailing zeros of q, which
// the scaling introduced, are stripped: at most k of them, so the exponent
// never passes xe - ye, and only when the remainder is zero, so that no
// non-zero fraction is lost. roundW then raises the exponent further only
// if the stripped q still has more than PrecisionW digits, which is the
// cohort rule. The strip first tries the likely count of zeros by one exact
// division through the modular inverse of a power of ten (cDivExactPow10W,
// which tests divisibility without a remainder): all k of them for the
// long division's quotient, which has them whenever yc divides xc (8.00 /
// 2), and one for a dividend over a power of ten, whose quotient is x's
// own coefficient and as a rule has none. Only when that attempt leaves
// the count open does the binary search on the number of zeros run, as in
// keyW, by the same exact divisions: q has at most PrecisionW+1 digits
// (PrecisionW, or one more after the round-digit step), so at most
// PrecisionW trailing zeros, and the attempt and the steps stay within the
// smaller of k and PrecisionW+1, which keeps every power of ten within the
// coefficient table.
func divW(x, y coefW, mode Mode) coefW {
	xneg, xc, xe, xok := unpackFirstW(x)
	xk := KindFinite
	if !xok {
		xk, xneg, xc, xe = unpackRestW(x)
	}
	yneg, yc, ye, yok := unpackFirstW(y)
	yk := KindFinite
	if !yok {
		yk, yneg, yc, ye = unpackRestW(y)
	}
	neg := xneg != yneg
	switch {
	case xk == KindNaN || yk == KindNaN:
		return nanW(x, y, y)
	case xk == KindInf:
		if yk == KindInf {
			// ∞ / ∞ is invalid (IEEE 754-2019 clause 7.2 item e).
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

	dy := cDigitsW(yc)
	k, q, class := dy-1, xc, classZero
	first := 1 // the likely count of zeros to strip from an exact quotient
	if !cEqW(yc, cPow10W(k)) {
		// Scale so that q has exactly PrecisionW digits: xc × 10^k / yc
		// lies in [10^(dx+k-dy-1), 10^(dx+k-dy+1)) and reaches 10^(dx+k-dy)
		// exactly when the leading ratio does, xc × 10^(dy-dx) >= yc, a
		// comparison of two values of dy or dx digits, which fit.
		//
		// Which coefficient is the longer is a property of the operands'
		// kind and stays a branch; whether the ratio reaches one is in
		// their leading digits, as good as random, and is a borrow
		// (cLtW), not a branch.
		dx := cDigitsW(xc)
		a, b := xc, yc
		if dx <= dy {
			a = cMulPow10W(xc, dy-dx)
		} else {
			b = cMulPow10W(yc, dx-dy)
		}
		lead := 1
		if cLtW(a, b) {
			lead = 0
		}
		k = PrecisionW - dx + dy - lead
		n := cMulPow10FullW(xc, k)
		exp := xe - ye - k
		face := MinExpW < exp && exp <= MaxExpW && exp+PrecisionW-1 >= EminW
		var rem coefW
		if face && mode != TowardZero && mode != TowardPositive && mode != TowardNegative {
			// To nearest, inside the exponent range of roundFaceW's proof:
			// the dividend is biased by half the divisor, and the quotient
			// of n + ⌊yc/2⌋ by yc is the quotient of n by yc rounded to
			// nearest with ties up, because the bias carries into the next
			// unit exactly when the fraction is a half or more. Its
			// remainder says the rest: it is ⌊yc/2⌋ exactly when yc divides
			// n, the exact quotient, which goes on to the preferred
			// exponent below; and it is zero with an even yc exactly on a
			// tie, where ties to even takes the even neighbour, the one
			// below. Nothing here waits for a comparison of the remainder
			// with the half or branches on the direction, which is as good
			// as random: the two tests are rare and the carry is packFaceW's.
			half := cHalfW(yc)
			nb, _ := cAddFullW(n, wideW{Lo: half})
			var rb coefW
			q, rb = cDivFullW(nb, yc)
			if !cEqW(rb, half) {
				if cIsZeroW(rb) && !cOddW(yc) && mode != ToNearestAway {
					q = cEvenW(q)
				}
				return packFaceW(neg, q, exp, mode)
			}
		} else {
			q, rem = cDivFullW(n, yc)
		}
		if !cIsZeroW(rem) {
			// An inexact quotient in a directed mode or near the exponent
			// limits: the remainder places the true value between q and
			// q + 1 at exponent xe - ye - k, and its class is exact: rem
			// against yc - rem. Within the exponent range that roundFaceW's
			// proof names, that is the whole rounding decision, taken at
			// face value; nearer the limits roundW takes the usual round
			// digit and sticky bit, found by one more division step.
			other, _ := cSubW(yc, rem)
			class = cClassW(rem, other)
			if face {
				return roundFaceW(neg, q, exp, class, mode)
			}
			d, rem2 := cDivFullW(wideW{Lo: cMulPow10W(rem, 1)}, yc)
			q, _ = cAddW(cMulPow10W(q, 1), d)
			k++
			class = classZero
			if !cIsZeroW(rem2) {
				class = classBelowHalf
			}
		} else if k < PrecisionW {
			// The long division's exact quotient has all k zeros whenever
			// yc divides xc, the usual exact division (8.00 / 2): try them
			// at once. q has PrecisionW digits, so k >= PrecisionW cannot
			// strip whole; the search takes those.
			first = k
		} else {
			first = 0
		}
	}
	if class == classZero && k > 0 {
		// The first attempt is the likely count of zeros, by one exact
		// division through the modular inverse of 10^first
		// (cDivExactPow10W, which tests divisibility without a remainder
		// and refuses by the low bits alone), and the binary search runs
		// only when the attempt leaves the count open. A dividend over a
		// power of ten keeps its own coefficient, which as a rule has no
		// trailing zero: one place is tried, and a failure ends the strip.
		// The long division's all-at-once attempt ends the strip on success.
		search := true
		if first > 0 {
			if sq, ok := cDivExactPow10W(q, first); ok {
				q, k = sq, k-first
				search = first == 1
			} else {
				search = first > 1
			}
		}
		if search && k > 0 {
			step := 1
			for step*2 <= min(k, PrecisionW+1) {
				step *= 2
			}
			for ; step > 0; step /= 2 {
				if step > k {
					continue
				}
				if sq, ok := cDivExactPow10W(q, step); ok {
					q, k = sq, k-step
				}
			}
		}
	}
	r, _ := roundW(neg, wideW{Lo: q}, xe-ye-k, class, mode, MinExpW)
	return r
}
