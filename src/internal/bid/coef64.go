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
