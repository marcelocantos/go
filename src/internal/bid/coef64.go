// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// The decimal64 instantiation of the coefficient vocabulary used by the
// shared core (see "Width parameterisation" in the package documentation,
// rule 4). The decimal128 instantiation, with identical signatures, is in
// coef128.go. Every function here must inline: the decimal64 operations
// stay within noise of hand-written code only if they do. After changing
// one, check that
//
//	go build -gcflags=-m=2 internal/bid 2>&1 | grep 'cannot inline c.*64'
//
// prints nothing. The level matters: plain -m does not print the
// "cannot inline" lines at all, so the same command with -m is empty
// whatever the helpers do.
//
// The template names are the width-independent names with a final W
// replaced by the width, so cMulPow10W becomes cMulPow1064 and cPow10W
// becomes cPow1064. The names read badly; the rule that makes them is worth
// more.
//
// Where a function has a precondition it says so. A precondition that is
// cheap to test is tested and violated by a panic, never by a silently
// wrong answer; the callers in the template establish each one from the
// format's precision.

type coef64 = uint64
type wide64 = Uint128

// cIsZero64 reports whether c is zero.
func cIsZero64(c coef64) bool {
	return c == 0
}

// cCmp64 returns -1, 0 or +1 as a is less than, equal to or greater than b.
func cCmp64(a, b coef64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// cAdd64 returns a + b modulo 2^64 and whether the sum carried out of the
// type, in which case the true sum is 2^64 + sum.
func cAdd64(a, b coef64) (sum coef64, carry bool) {
	sum, c := bits.Add64(a, b, 0)
	return sum, c != 0
}

// cSub64 returns a - b modulo 2^64 and whether b exceeded a, in which case
// the true difference is sum - 2^64.
func cSub64(a, b coef64) (diff coef64, borrow bool) {
	diff, b2 := bits.Sub64(a, b, 0)
	return diff, b2 != 0
}

// cOdd64 reports whether c is odd.
func cOdd64(c coef64) bool {
	return c&1 != 0
}

// cDigits64 returns the number of decimal digits of c: 0 for zero.
//
// The bit length b of c gives t = ⌊b × 1233 / 4096⌋ ≈ ⌊b × log10 2⌋, and
// c has t digits if c < 10^t and t+1 otherwise. (1233/4096 is just below
// log10 2, which is why the comparison is needed and why t is never too
// large.)
func cDigits64(c coef64) int {
	t := bits.Len64(c) * 1233 >> 12
	if c < pow10tab64[t] {
		return t
	}
	return t + 1
}

// cMulPow1064 returns c × 10^k. Precondition: 0 <= k <= 19 and the product
// fits 64 bits; a product that does not panics rather than wrap.
func cMulPow1064(c coef64, k int) coef64 {
	hi, lo := bits.Mul64(c, pow10tab64[k])
	if hi != 0 {
		panic("internal/bid: coefficient overflow")
	}
	return lo
}

// cDivPow1064 returns the quotient and remainder of c / 10^k, so that
// c = q × 10^k + r with 0 <= r < 10^k. Precondition: 0 <= k <= 19.
func cDivPow1064(c coef64, k int) (q, r coef64) {
	p := pow10tab64[k]
	return c / p, c % p
}

// cHalf64 returns 5 × 10^(k-1), half of 10^k: the value of a discarded
// k-digit tail at which rounding is a tie. Precondition: 1 <= k <= 19.
func cHalf64(k int) coef64 {
	return 5 * pow10tab64[k-1]
}

// cGtMax64 reports whether c exceeds the largest decimal64 coefficient.
func cGtMax64(c coef64) bool {
	return c > MaxCoeff64
}

// cPow1064 returns 10^k. Precondition: 0 <= k <= 19.
func cPow1064(k int) coef64 {
	return pow10tab64[k]
}

// cMulFull64 returns the exact product a × b.
func cMulFull64(a, b coef64) wide64 {
	hi, lo := bits.Mul64(a, b)
	return wide64{Hi: hi, Lo: lo}
}

// cReduce64 returns ⌊p / 10^k⌋ and whether the division left a remainder
// (the sticky bit of the discarded digits). Precondition: 0 <= k <= 19 and
// the quotient fits 64 bits; a quotient that does not panics rather than
// truncate. Both hold for the product of two decimal64 coefficients
// reduced to PrecisionW+1 digits.
func cReduce64(p wide64, k int) (q coef64, sticky bool) {
	q, r := bits.Div64(p.Hi, p.Lo, pow10tab64[k])
	return q, r != 0
}

// cLenFull64 returns the number of decimal digits of p: 0 for zero.
func cLenFull64(p wide64) int {
	return cDigits128(p)
}

// cAddFull64 returns a + b modulo 2^128 and whether the sum carried out of
// the type, in which case the true sum is 2^128 + sum.
func cAddFull64(a, b wide64) (sum wide64, carry bool) {
	sum, c := u128Add(a, b)
	return sum, c != 0
}

// cSubFull64 returns a - b modulo 2^128 and whether b exceeded a, in which
// case the true difference is diff - 2^128.
func cSubFull64(a, b wide64) (diff wide64, borrow bool) {
	diff, b2 := u128Sub(a, b)
	return diff, b2 != 0
}

// cIncFull64 returns p + 1. Precondition: p is below the largest value of the
// type, which holds for a value of at most 38 digits.
func cIncFull64(p wide64) wide64 {
	s, _ := u128Add(p, wide64{Lo: 1})
	return s
}

// cShift64 returns ⌊c / 10^k⌋ and the class of the discarded part. Any
// k >= 0 is allowed; see "Decimal-digit primitives" in digits.go for what
// the caller may assume.
func cShift64(c coef64, k int) (q coef64, class digitClass) {
	if k >= len(pow10tab64) {
		if c == 0 {
			return 0, classZero
		}
		return 0, classBelowHalf
	}
	d := pow10tab64[k]
	q = c / d
	return q, class64(c-q*d, d>>1)
}

// cShiftFull64 is cShift64 for a double-width value, which may keep a
// quotient too large for a coefficient.
func cShiftFull64(p wide64, k int) (q wide64, class digitClass) {
	return cShift128(p, k)
}

// cReduceClass64 is cShiftFull64 for a quotient known to fit a coefficient.
// A quotient that does not fit panics rather than truncates. It holds
// whenever the caller chose k to leave at most 19 digits of p.
func cReduceClass64(p wide64, k int) (q coef64, class digitClass) {
	return reduceClass64(p, k)
}

// cScale64 returns c × 10^k, or zero and true if that does not fit a
// coefficient. Any k >= 0 is allowed; a negative k panics.
func cScale64(c coef64, k int) (r coef64, overflow bool) {
	if k >= len(pow10tab64) {
		return 0, c != 0
	}
	hi, lo := bits.Mul64(c, pow10tab64[k])
	if hi != 0 {
		return 0, true
	}
	return lo, false
}

// cScaleFull64 is cScale64 for a double-width value.
func cScaleFull64(p wide64, k int) (r wide64, overflow bool) {
	return cScale128(p, k)
}

// cMulPow10Full64 returns c × 10^k as a double-width value. Precondition:
// 0 <= k <= 38 and the product fits 128 bits, which holds for a coefficient
// scaled by at most 2×Precision64 places (33 digits); a product that does
// not panics rather than wrap. The power comes from the 128-bit table so
// that one 128-by-64 multiply, whose third word is the overflow check,
// serves every k; a branch on k and a second path cost more than the
// inlining budget allows.
func cMulPow10Full64(c coef64, k int) wide64 {
	p, hi := u128Mul64(pow10tab128[k], c)
	if hi != 0 {
		panic("internal/bid: coefficient overflow")
	}
	return p
}

// cDivFull64 returns the quotient and remainder of n / d, so that
// n = q × d + r with 0 <= r < d. Precondition: d is non-zero and the
// quotient fits 64 bits, that is, n.Hi < d; bits.Div64 panics on either
// violation rather than truncate. The quotient of a decimal64 division
// scaled to Precision64+2 digits at most always fits.
func cDivFull64(n wide64, d coef64) (q, r coef64) {
	return bits.Div64(n.Hi, n.Lo, d)
}

// cNaN64 returns the canonical quiet NaN.
func cNaN64() coef64 {
	return NaN64
}

// cModFull64 returns p mod m. Precondition: m is not zero; a zero m panics
// with the math/bits divide error.
func cModFull64(p wide64, m coef64) coef64 {
	return bits.Rem64(p.Hi, p.Lo, m)
}

// cLo64 returns the coefficient held in the low word of p. Precondition:
// the high word is zero, that is, p fits a coefficient; a p that does not
// panics rather than truncates.
func cLo64(p wide64) coef64 {
	if p.Hi != 0 {
		panic(errQuotientOverflow)
	}
	return p.Lo
}

// cInc64 returns c + 1. Precondition: c is below the largest value of the
// type, which holds for every coefficient of at most Precision64 digits.
func cInc64(c coef64) coef64 {
	return c + 1
}

// cMax64 returns the largest decimal64 coefficient, 10^Precision64 - 1.
func cMax64() coef64 {
	return MaxCoeff64
}

// cInf64 returns the canonical encoding of the infinity of sign neg.
func cInf64(neg bool) coef64 {
	if neg {
		return Inf64 | SignMask64
	}
	return Inf64
}

// cFromU6464 returns the coefficient with the value m.
func cFromU6464(m uint64) coef64 {
	return m
}

// cToU6464 returns the value of c as a uint64, and whether it fits one,
// which every decimal64 coefficient does.
func cToU6464(c coef64) (m uint64, ok bool) {
	return c, true
}

// cFromU12864 returns a 128-bit value as a wide64, which is the value
// itself: the binary-to-decimal conversion generates its digits into a
// Uint128 at both widths and hands them to the rounding routine through
// this function.
func cFromU12864(q Uint128) wide64 {
	return q
}

// cSplit64 returns c split into the words hi and lo with
// c = hi × 10^19 + lo and lo < 10^19, the form writeDigits renders.
// Precondition: c is at most MaxCoeff64, which is below 10^19, so hi is
// zero.
func cSplit64(c coef64) (hi, lo uint64) {
	return 0, c
}

// cWide64 returns the 128-bit value m as a wide64, which it is. The
// precondition of the other width, that m fits it, is vacuous here.
func cWide64(m Uint128) wide64 {
	return m
}
