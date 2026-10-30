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
func cReduce128(p wide128, k int) (q coef128, sticky bool) {
	q, class := cReduceClass128(p, k)
	return q, class != classZero
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

// cShift128 returns ⌊c / 10^k⌋ and the class of the discarded part. Any
// k >= 0 is allowed; see "Decimal-digit primitives" in digits.go for what
// the caller may assume.
func cShift128(c coef128, k int) (q coef128, class digitClass) {
	if k >= len(pow10tab128) {
		if c == (coef128{}) {
			return coef128{}, classZero
		}
		return coef128{}, classBelowHalf
	}
	d := pow10tab128[k]
	q, r := u128Div(c, d)
	return q, class128(r, u128Shr(d, 1))
}

// cShiftFull128 is cShift128 for a 256-bit value.
//
// Only 10^k up to k = 38 has a 128-bit divisor, so a larger k first removes
// the low digits 10^38 at a time, at most twice since k < 78. Those steps
// are exact and can only contribute a sticky bit, because the half point of
// the whole shift lies in the digits that remain. The last step, whose k is
// at least 1 if an earlier step ran, classifies, and withSticky applies the
// earlier remainders to that class. An intermediate quotient may need more
// than 128 bits, which is why each step is the full 256-bit division.
func cShiftFull128(p wide128, k int) (q wide128, class digitClass) {
	if k >= len(pow10tab256) {
		if p == (wide128{}) {
			return wide128{}, classZero
		}
		return wide128{}, classBelowHalf
	}
	const maxStep = len(pow10tab128) - 1 // 10^38, the last power below 2^128
	sticky := false
	for k > maxStep {
		var r Uint128
		p, r = u256DivMod128(p, pow10tab128[maxStep])
		sticky = sticky || r != (Uint128{})
		k -= maxStep
	}
	d := pow10tab128[k]
	q, r := u256DivMod128(p, d)
	return q, class128(r, u128Shr(d, 1)).withSticky(sticky)
}

// cReduceClass128 is cShiftFull128 for a quotient known to fit a
// coefficient. A quotient that does not fit panics rather than truncates. It
// holds whenever the caller chose k to leave at most 38 digits of p.
func cReduceClass128(p wide128, k int) (q coef128, class digitClass) {
	wq, class := cShiftFull128(p, k)
	if wq.Hi != (Uint128{}) {
		panic(errQuotientOverflow)
	}
	return wq.Lo, class
}

// cScale128 returns c × 10^k, or zero and true if that does not fit a
// coefficient. Any k >= 0 is allowed; a negative k panics.
func cScale128(c coef128, k int) (r coef128, overflow bool) {
	if k >= len(pow10tab128) {
		return coef128{}, c != coef128{}
	}
	w := u128Mul(c, pow10tab128[k])
	if w.Hi != (Uint128{}) {
		return coef128{}, true
	}
	return w.Lo, false
}

// cScaleFull128 is cScale128 for a 256-bit value. If both factors exceed
// 128 bits the product is at least 2^256, so otherwise one of them fits 128
// bits and the product is one 256-by-128 multiply.
func cScaleFull128(p wide128, k int) (r wide128, overflow bool) {
	if k >= len(pow10tab256) {
		return wide128{}, p != wide128{}
	}
	m := pow10tab256[k]
	if m.Hi != (Uint128{}) {
		if p.Hi != (Uint128{}) {
			return wide128{}, true
		}
		p, m = m, p
	}
	lo := u128Mul(p.Lo, m.Lo)
	hi := u128Mul(p.Hi, m.Lo)
	r.Lo = lo.Lo
	var c uint64
	r.Hi, c = u128Add(lo.Hi, hi.Lo)
	if c != 0 || hi.Hi != (Uint128{}) {
		return wide128{}, true
	}
	return r, false
}

// cLo128 returns the coefficient held in the low half of p. Precondition:
// the high half is zero, that is, p fits a coefficient; a p that does not
// panics rather than truncates.
func cLo128(p wide128) coef128 {
	if p.Hi != (Uint128{}) {
		panic(errQuotientOverflow)
	}
	return p.Lo
}

// cInc128 returns c + 1. Precondition: c is below the largest value of the
// type, which holds for every coefficient of at most Precision128 digits.
func cInc128(c coef128) coef128 {
	s, _ := u128Add(c, Uint128{Lo: 1})
	return s
}

// cMax128 returns the largest decimal128 coefficient, 10^Precision128 - 1.
func cMax128() coef128 {
	return Uint128{Hi: MaxCoeff128Hi, Lo: MaxCoeff128Lo}
}

// cInf128 returns the canonical encoding of the infinity of sign neg.
func cInf128(neg bool) coef128 {
	if neg {
		return Uint128{Hi: Inf128Hi | SignMask128Hi}
	}
	return Uint128{Hi: Inf128Hi}
}

// cNaN128 returns the canonical quiet NaN.
func cNaN128() coef128 {
	return Uint128{Hi: NaN128Hi}
}
