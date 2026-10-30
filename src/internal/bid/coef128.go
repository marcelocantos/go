// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// The decimal128 instantiation of the coefficient vocabulary used by the
// shared core (see "Width parameterisation" in the package documentation,
// rule 4). The decimal64 instantiation, with identical signatures, is in
// coef64.go. These functions may call any of the unexported Uint128 and
// Uint256 arithmetic freely; unlike their decimal64 counterparts they are
// not required to inline.
//
// Preconditions are as in coef64.go: stated, and a cheap one is checked by
// a panic rather than answered wrongly.

type coef128 = Uint128
type wide128 = Uint256

// cIsZero128 reports whether c is zero.
func cIsZero128(c coef128) bool {
	return c == coef128{}
}

// cCmp128 returns -1, 0 or +1 as a is less than, equal to or greater than b.
func cCmp128(a, b coef128) int {
	return u128Cmp(a, b)
}

// cAdd128 returns a + b modulo 2^128 and whether the sum carried out of the
// type, in which case the true sum is 2^128 + sum.
func cAdd128(a, b coef128) (sum coef128, carry bool) {
	sum, c := u128Add(a, b)
	return sum, c != 0
}

// cSub128 returns a - b modulo 2^128 and whether b exceeded a, in which
// case the true difference is diff - 2^128.
func cSub128(a, b coef128) (diff coef128, borrow bool) {
	diff, b2 := u128Sub(a, b)
	return diff, b2 != 0
}

// cOdd128 reports whether c is odd.
func cOdd128(c coef128) bool {
	return c.Lo&1 != 0
}

// cDigits128 returns the number of decimal digits of c: 0 for zero. See
// cDigits64 for the method; here the table has 39 entries, 10^0 to 10^38,
// the powers of ten below 2^128.
func cDigits128(c coef128) int {
	t := u128Len(c) * 1233 >> 12
	if u128Lt(c, pow10tab128[t]) {
		return t
	}
	return t + 1
}

// cMulPow10128 returns c × 10^k. Precondition: 0 <= k <= 38 and the product
// fits 128 bits; a product that does not panics rather than wrap.
func cMulPow10128(c coef128, k int) coef128 {
	p := u128Mul(c, pow10tab128[k])
	if p.Hi != (Uint128{}) {
		panic("internal/bid: coefficient overflow")
	}
	return p.Lo
}

// cDivPow10128 returns the quotient and remainder of c / 10^k, so that
// c = q × 10^k + r with 0 <= r < 10^k. Precondition: 0 <= k <= 38.
func cDivPow10128(c coef128, k int) (q, r coef128) {
	return u128Div(c, pow10tab128[k])
}

// cHalf128 returns 5 × 10^(k-1), half of 10^k: the value of a discarded
// k-digit tail at which rounding is a tie. Precondition: 1 <= k <= 38.
func cHalf128(k int) coef128 {
	h, _ := u128Mul64(pow10tab128[k-1], 5)
	return h
}

// cGtMax128 reports whether c exceeds the largest decimal128 coefficient.
func cGtMax128(c coef128) bool {
	return u128GtMaxCoeff(c)
}

// cPow10128 returns 10^k. Precondition: 0 <= k <= 38.
func cPow10128(k int) coef128 {
	return pow10tab128[k]
}

// cMulFull128 returns the exact product a × b.
func cMulFull128(a, b coef128) wide128 {
	return u128Mul(a, b)
}

// cReduce128 returns ⌊p / 10^k⌋ and whether the division left a remainder
// (the sticky bit of the discarded digits). Precondition: k >= 0 and the
// quotient fits 128 bits; a quotient that does not panics rather than
// truncate. Both hold for the product of two decimal128 coefficients
// reduced to PrecisionW+1 digits.
//
// 10^k has no 128-bit representation above k = 38, so a larger k divides in
// steps of at most 10^38; the intermediate quotients may need more than 128
// bits, which is why each step is the full 256-bit division. Any k above 77
// exceeds every 256-bit p, which then divides to zero with a remainder
// unless it is zero.
func cReduce128(p wide128, k int) (q coef128, sticky bool) {
	const maxK = len(pow10tab256) - 1 // 10^77, the last power below 2^256
	if k > maxK {
		return coef128{}, p != (wide128{})
	}
	for k > 0 {
		k1 := min(k, len(pow10tab128)-1)
		var r Uint128
		p, r = u256DivMod128(p, pow10tab128[k1])
		sticky = sticky || r != (Uint128{})
		k -= k1
	}
	if p.Hi != (Uint128{}) {
		panic("internal/bid: quotient overflow")
	}
	return p.Lo, sticky
}

// cLenFull128 returns the number of decimal digits of p: 0 for zero. The
// method is that of cDigits64, with a table of the 78 powers of ten below
// 2^256.
func cLenFull128(p wide128) int {
	t := u256Len(p) * 1233 >> 12
	if u256Lt(p, pow10tab256[t]) {
		return t
	}
	return t + 1
}
