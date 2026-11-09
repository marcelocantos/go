// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// A fast path for the two inexact cases of the binary-to-decimal conversion
// (fromfloat.go): the quotient q = ⌊m × 5^s / 2^r⌋ for a value below one
// unit of 2^-r, and q = ⌊m × 2^j / 5^k⌋ for a large integer. The exact
// multiword arithmetic of floatNat costs a pass over up to sixteen words per
// 27 powers of five, and the division by 5^27 is bits.Div64, which is
// software on ARM64. Here each is one 64-by-128-bit multiply by a table
// entry and a shift, and the result is either proved exact or refused. A
// refusal leaves the exact path to compute the answer, so the fast path can
// only ever change the speed of the conversion, never its result.
//
// # The interval argument
//
// Let V be the rational whose floor is wanted, and suppose integers X, d >= 1
// and sh >= 0 are known with V in [X/2^sh, (X + d)/2^sh) or in
// ((X - d)/2^sh, X/2^sh]. Write Q = ⌊X / 2^sh⌋.
//
//   - In the first case ⌊V⌋ >= Q, and since an integer n <= V < (X+d)/2^sh
//     has n·2^sh < X + d, so n·2^sh <= X + d - 1, ⌊V⌋ <= ⌊(X + d - 1)/2^sh⌋.
//     If that equals Q, ⌊V⌋ = Q.
//   - In the second case ⌊V⌋ <= Q, and ⌊V⌋ >= ⌊(X - d)/2^sh⌋. If that equals
//     Q, ⌊V⌋ = Q.
//
// Equality of the two floors is checked by shifting both bounds, so the
// result is exact whenever it is returned. The check fails only when the
// low sh bits of X are within d of a multiple of 2^sh. For decimal128,
// whose q has about 120 bits, sh is about 61 and d < 2^53, so roughly one
// conversion in 2^8 to 2^9 of a random 53-bit significand with a large
// exponent takes the exact path; for decimal64 sh is about 120 and the
// exact path is all but unreachable. TestFromFloatFast checks both
// outcomes against math/big and that the refusals do occur.
//
// # The tables
//
// pow5MulTab[s], 0 <= s <= 358, is T = ⌊5^s / 2^a⌋ with a = max(0,
// len(5^s) - 128), so T·2^a <= 5^s < (T + 1)·2^a, with equality on the left
// when a = 0. Then m × 5^s / 2^r lies in [X/2^sh, (X + m)/2^sh) for X = m·T
// and sh = r - a, and is exactly X/2^sh when a = 0. The largest s is that of
// 2^-1074 at decimal128: PrecisionW - log10Pow2(-1074) = 34 + 324.
//
// pow5DivTab[k], 1 <= k <= 292, is P = ⌈2^b / 5^k⌉ with b = len(5^k) + 127,
// so 2^127 < P < 2^128 and, as 5^k does not divide a power of two,
// P - 1 < 2^b / 5^k < P. Then m × 2^j / 5^k lies in ((X - m)/2^sh, X/2^sh)
// for X = m·P and sh = b - j. The largest k is that of the largest float64 at
// decimal64: its leading bit is 2^1023, so k = log10Pow2(1023) - 16 = 291,
// one below the table's end.
//
// X has at most 53 + 128 bits and fits three words; a shift whose quotient
// does not fit 128 bits, a negative shift, or an index outside a table is
// refused as well, so no precondition of the caller is load-bearing.

// pow5Len returns the bit length of 5^s, ⌊s × log2 5⌋ + 1, for 0 <= s <=
// 3528 (the bound of this fixed-point approximation, from Ryū);
// TestPow5Approx checks it over both tables.
func pow5Len(s int) int {
	return int(uint64(s)*1217359>>19) + 1
}

// mul128x64 returns the exact 192-bit product t × m as three words, least
// significant first.
func mul128x64(t Uint128, m uint64) (w0, w1, w2 uint64) {
	h0, l0 := bits.Mul64(t.Lo, m)
	h1, l1 := bits.Mul64(t.Hi, m)
	var c uint64
	w1, c = bits.Add64(l1, h0, 0)
	return l0, w1, h1 + c
}

// shr192 returns ⌊w / 2^sh⌋ for the 192-bit w = (w2, w1, w0) and whether it
// fits 128 bits. Precondition: 0 <= sh < 192. Go defines a shift by 64 or
// more as zero, which the word arithmetic relies on.
func shr192(w0, w1, w2 uint64, sh uint) (q Uint128, ok bool) {
	switch {
	case sh < 64:
		q.Lo = w0>>sh | w1<<(64-sh)
		q.Hi = w1>>sh | w2<<(64-sh)
		return q, w2>>sh == 0
	case sh < 128:
		sh -= 64
		q.Lo = w1>>sh | w2<<(64-sh)
		q.Hi = w2 >> sh
		return q, true
	}
	q.Lo = w2 >> (sh - 128)
	return q, true
}

// mulPow5Fast returns ⌊m × 5^s / 2^r⌋, and false when the interval argument
// cannot prove it or the inputs fall outside the tables. Precondition for a
// true result: none beyond m != 0.
func mulPow5Fast(m uint64, s, r int) (q Uint128, ok bool) {
	if s < 0 || s >= len(pow5MulTab) {
		return Uint128{}, false
	}
	a := max(0, pow5Len(s)-128)
	sh := r - a
	if sh < 0 || sh >= 192 {
		return Uint128{}, false
	}
	w0, w1, w2 := mul128x64(pow5MulTab[s], m)
	q, ok = shr192(w0, w1, w2, uint(sh))
	if !ok || a == 0 {
		return q, ok // a = 0: the table entry is 5^s and X/2^sh is exact
	}
	// The upper bound X + m - 1.
	var c uint64
	w0, c = bits.Add64(w0, m-1, 0)
	w1, c = bits.Add64(w1, 0, c)
	w2, c = bits.Add64(w2, 0, c)
	if c != 0 {
		return Uint128{}, false
	}
	if q2, ok2 := shr192(w0, w1, w2, uint(sh)); !ok2 || q2 != q {
		return Uint128{}, false
	}
	return q, true
}

// divPow5Fast returns ⌊m × 2^j / 5^k⌋ and whether the division is inexact,
// and false when the interval argument cannot prove the quotient or the
// inputs fall outside the table. Precondition for a true result: m != 0.
//
// The remainder is non-zero exactly when 5^k does not divide m, since 5^k
// and 2^j are coprime; m < 2^53 < 5^23, so that is certain for k >= 23 and a
// word remainder otherwise.
func divPow5Fast(m uint64, j, k int) (q Uint128, sticky, ok bool) {
	if k < 1 || k >= len(pow5DivTab) || j < 0 {
		return Uint128{}, false, false
	}
	sh := pow5Len(k) + 127 - j
	if sh < 0 || sh >= 192 {
		return Uint128{}, false, false
	}
	w0, w1, w2 := mul128x64(pow5DivTab[k], m)
	q, ok = shr192(w0, w1, w2, uint(sh))
	if !ok {
		return Uint128{}, false, false
	}
	// The lower bound X - m; X >= 2^127 > m, so it does not borrow out.
	var b uint64
	w0, b = bits.Sub64(w0, m, 0)
	w1, b = bits.Sub64(w1, 0, b)
	w2, _ = bits.Sub64(w2, 0, b)
	if q2, _ := shr192(w0, w1, w2, uint(sh)); q2 != q {
		return Uint128{}, false, false
	}
	sticky = k > pow5Exact || m%pow5(k) != 0
	return q, sticky, true
}

// pow5Exact is the largest k for which 5^k can divide a float64 significand:
// 5^22 < 2^53 < 5^23.
const pow5Exact = 22
