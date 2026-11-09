// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// Uint256 is a 256-bit unsigned integer, most significant half first. It is
// the exact product of two 128-bit coefficients and the dividend of the
// decimal128 division, and it is the wide type of the decimal128
// instantiation of the shared core (see "Width parameterisation" in the
// package documentation).
type Uint256 struct {
	Hi, Lo Uint128
}

// u256Add returns a + b modulo 2^256 and the carry out of bit 255 (0 or 1).
func u256Add(a, b Uint256) (sum Uint256, carry uint64) {
	sum.Lo.Lo, carry = bits.Add64(a.Lo.Lo, b.Lo.Lo, 0)
	sum.Lo.Hi, carry = bits.Add64(a.Lo.Hi, b.Lo.Hi, carry)
	sum.Hi.Lo, carry = bits.Add64(a.Hi.Lo, b.Hi.Lo, carry)
	sum.Hi.Hi, carry = bits.Add64(a.Hi.Hi, b.Hi.Hi, carry)
	return sum, carry
}

// u256Sub returns a - b modulo 2^256 and the borrow out of bit 255 (0 or 1;
// 1 exactly when a < b).
func u256Sub(a, b Uint256) (diff Uint256, borrow uint64) {
	diff.Lo.Lo, borrow = bits.Sub64(a.Lo.Lo, b.Lo.Lo, 0)
	diff.Lo.Hi, borrow = bits.Sub64(a.Lo.Hi, b.Lo.Hi, borrow)
	diff.Hi.Lo, borrow = bits.Sub64(a.Hi.Lo, b.Hi.Lo, borrow)
	diff.Hi.Hi, borrow = bits.Sub64(a.Hi.Hi, b.Hi.Hi, borrow)
	return diff, borrow
}

// u256Cmp returns -1, 0 or +1 as a is less than, equal to or greater than b.
func u256Cmp(a, b Uint256) int {
	if c := u128Cmp(a.Hi, b.Hi); c != 0 {
		return c
	}
	return u128Cmp(a.Lo, b.Lo)
}

// u256Lt reports whether a < b.
func u256Lt(a, b Uint256) bool {
	if a.Hi != b.Hi {
		return u128Lt(a.Hi, b.Hi)
	}
	return u128Lt(a.Lo, b.Lo)
}

// u256Len returns the length of a in bits: 0 for zero, else the position of
// the highest set bit plus one.
func u256Len(a Uint256) int {
	if a.Hi != (Uint128{}) {
		return 128 + u128Len(a.Hi)
	}
	return u128Len(a.Lo)
}

// u256Shl returns a << n. Bits shifted out are lost; a shift of 256 or more
// yields zero.
func u256Shl(a Uint256, n uint) Uint256 {
	if n >= 128 {
		return Uint256{Hi: u128Shl(a.Lo, n-128)}
	}
	return Uint256{
		Hi: u128Or(u128Shl(a.Hi, n), u128Shr(a.Lo, 128-n)),
		Lo: u128Shl(a.Lo, n),
	}
}

// u256Shr returns a >> n (a logical shift). A shift of 256 or more yields
// zero.
func u256Shr(a Uint256, n uint) Uint256 {
	if n >= 128 {
		return Uint256{Lo: u128Shr(a.Hi, n-128)}
	}
	return Uint256{
		Hi: u128Shr(a.Hi, n),
		Lo: u128Or(u128Shr(a.Lo, n), u128Shl(a.Hi, 128-n)),
	}
}

// The division by a two-word divisor uses the method of Möller and
// Granlund, "Improved division by invariant integers" (IEEE Transactions on
// Computers 60(2), 2011): each quotient word is estimated by multiplying by
// a reciprocal of the normalised divisor, computed once per divisor and
// shared by both quotient words of u256Div128, instead of by a 128-by-64
// division per word. The reciprocal itself takes one bits.Div64.
//
// Measured on arm64 (Apple M4 Max), where bits.Div64 is software: the
// two-word division is about 16% faster this way. A one-word divisor still
// uses two bits.Div64, which measured faster than a reciprocal and two
// Möller–Granlund 2/1 steps, and computing the reciprocal without any
// division (their Algorithm 3, Newton steps from a 9-bit approximation)
// measured slower than the one bits.Div64.

// reciprocal3by2 returns ⌊(2^192 - 1) / (d1, d0)⌋ - 2^64, the reciprocal
// of the normalised two-word divisor (d1, d0) that div3by2 multiplies by,
// from the reciprocal ⌊(2^128 - 1) / d1⌋ - 2^64 of d1 alone, which is the
// quotient of (2^128 - 1) - d1 × 2^64 by d1 (Möller and Granlund,
// Algorithm 6).
func reciprocal3by2(d1, d0 uint64) uint64 {
	v, _ := bits.Div64(^d1, ^uint64(0), d1)
	p := d1*v + d0
	if p < d0 { // the addition carried
		v--
		if p >= d1 {
			v--
			p -= d1
		}
		p -= d1
	}
	t1, t0 := bits.Mul64(v, d0)
	p += t1
	if p < t1 { // the addition carried
		v--
		if p > d1 || p == d1 && t0 >= d0 {
			v--
		}
	}
	return v
}

// div3by2 divides the three-word number (u2, u1, u0) by the two-word number
// (d1, d0), whose reciprocal v is reciprocal3by2(d1, d0), and returns the
// one-word quotient q and the two-word remainder (r1, r0) (Möller and
// Granlund, Algorithm 5).
//
// Preconditions, not checked: d1 has its top bit set (the divisor is
// normalised), and (u2, u1) < (d1, d0), which is exactly the condition for
// the quotient to fit one word.
//
// The candidate quotient (q1, q0) = v u2 + (u2, u1) is the quotient or one
// more than it after the increment below; the remainder computed modulo
// 2^128 against that candidate says which, by comparison with the fraction
// word q0, and adding the divisor back corrects it. A last, rare step
// corrects a remainder that is still at or above the divisor.
func div3by2(u2, u1, u0, d1, d0, v uint64) (q, r1, r0 uint64) {
	q, q0 := bits.Mul64(v, u2)
	q0, c := bits.Add64(q0, u1, 0)
	q += u2 + c
	r1 = u1 - q*d1
	t1, t0 := bits.Mul64(d0, q)
	// (r1, r0) = (r1, u0) - (t1, t0) - (d1, d0), modulo 2^128.
	var b uint64
	r0, b = bits.Sub64(u0, t0, 0)
	r1, _ = bits.Sub64(r1, t1, b)
	r0, b = bits.Sub64(r0, d0, 0)
	r1, _ = bits.Sub64(r1, d1, b)
	q++
	if r1 >= q0 {
		q--
		r0, c = bits.Add64(r0, d0, 0)
		r1, _ = bits.Add64(r1, d1, c)
	}
	if r1 > d1 || r1 == d1 && r0 >= d0 {
		q++
		r0, b = bits.Sub64(r0, d0, 0)
		r1, _ = bits.Sub64(r1, d1, b)
	}
	return q, r1, r0
}

// div3by2Knuth is div3by2 without a reciprocal: one step of Knuth's
// Algorithm D (TAOCP vol. 2, 4.3.1) with a two-word divisor. Preconditions,
// not checked: d1 has its top bit set, and u2 < d1, which u256Div128's one
// caller meets because there u2 < 2^s <= 2^63 for its shift s <= 63. The
// quotient word is estimated from the top words alone, as
// q̂ = ⌊(u2·2^64 + u1) / d1⌋; because d1 is normalised the estimate is never
// too small and at most 2 too large. It is multiplied back and subtracted,
// and while that borrows the divisor is added back and q̂ decremented, at
// most twice.
func div3by2Knuth(u2, u1, u0, d1, d0 uint64) (q, r1, r0 uint64) {
	q, _ = bits.Div64(u2, u1, d1)

	// (r2, r1, r0) = (u2, u1, u0) - q × (d1, d0), modulo 2^192.
	h0, p0 := bits.Mul64(q, d0)
	h1, p1 := bits.Mul64(q, d1)
	var c, b uint64
	p1, c = bits.Add64(p1, h0, 0)
	p2 := h1 + c // q × d < 2^192, so this cannot overflow
	r0, b = bits.Sub64(u0, p0, 0)
	r1, b = bits.Sub64(u1, p1, b)
	r2, b := bits.Sub64(u2, p2, b)

	// A borrow means q̂ was too large and the difference wrapped negative.
	// Add the divisor back; the difference is non-negative again exactly
	// when the addition carries out of the top word.
	for b != 0 {
		q--
		r0, c = bits.Add64(r0, d0, 0)
		r1, c = bits.Add64(r1, d1, c)
		r2, c = bits.Add64(r2, 0, c)
		b ^= c
	}
	return q, r1, r0
}

// u256Div128 divides n by d when the quotient fits 128 bits.
//
// Precondition, checked: d != 0 and n.Hi < d. That is exactly the condition
// for the quotient to be below 2^128, because n / d < 2^128 if and only if
// n < d × 2^128. ok reports whether it holds. When ok is false the
// quotient does not fit (or d is zero) and q and r are zero; nothing
// panics, and nothing is truncated. A caller whose quotient may need more
// than 128 bits uses u256DivMod128, which returns all of it.
//
// When ok is true, q = ⌊n / d⌋ and r = n - q × d, 0 <= r < d.
func u256Div128(n Uint256, d Uint128) (q, r Uint128, ok bool) {
	if !u128Lt(n.Hi, d) {
		return Uint128{}, Uint128{}, false
	}
	if d.Hi == 0 {
		// n.Hi < d.Lo < 2^64, so n.Hi.Hi is zero and two word divisions
		// suffice. bits.Div64 needs its high word below the divisor,
		// which the precondition gives at each step.
		var r64 uint64
		q.Hi, r64 = bits.Div64(n.Hi.Lo, n.Lo.Hi, d.Lo)
		q.Lo, r64 = bits.Div64(r64, n.Lo.Lo, d.Lo)
		return q, Uint128{Lo: r64}, true
	}

	// Normalise: shift d left until its top bit is set, and n by the same
	// amount. n << s cannot lose bits because n.Hi < d implies
	// n.Hi << s < d << s < 2^128. For s == 0 the shifts by 64-s are by 64,
	// which yields zero, so no case analysis is needed.
	s := uint(bits.LeadingZeros64(d.Hi))
	v1 := d.Hi<<s | d.Lo>>(64-s)
	v0 := d.Lo << s
	w3 := n.Hi.Hi<<s | n.Hi.Lo>>(64-s)
	w2 := n.Hi.Lo<<s | n.Lo.Hi>>(64-s)
	w1 := n.Lo.Hi<<s | n.Lo.Lo>>(64-s)
	w0 := n.Lo.Lo << s

	// Two quotient words, high first. The first step's precondition
	// (w3, w2) < (v1, v0) is n.Hi < d after the shift; the second's is that
	// the remainder of the first is below the divisor.
	// A dividend below 2^128 has a high quotient word of zero, because
	// d >= 2^64, and only the low word to find. One word does not repay
	// the reciprocal: Knuth's estimate from one bits.Div64 is cheaper.
	var r1, r0 uint64
	if n.Hi == (Uint128{}) {
		q.Lo, r1, r0 = div3by2Knuth(w2, w1, w0, v1, v0)
	} else {
		v := reciprocal3by2(v1, v0)
		q.Hi, r1, r0 = div3by2(w3, w2, w1, v1, v0, v)
		q.Lo, r1, r0 = div3by2(r1, r0, w0, v1, v0, v)
	}

	// Undo the normalisation of the remainder.
	r = Uint128{Hi: r1 >> s, Lo: r0>>s | r1<<(64-s)}
	return q, r, true
}

// u256DivMod128 returns the full quotient and the remainder of n / d, with
// 0 <= r < d. The quotient is up to 256 bits and nothing is discarded: this
// is the primitive for a caller that cannot bound the quotient by 2^128.
// Precondition: d != 0; a zero d panics with errDivideByZero.
//
// It is long division in base 2^128: the high half of n divides first
// (its quotient is the high half of the result, with a remainder below d),
// and that remainder joined to the low half of n divides second, which
// satisfies u256Div128's precondition by construction.
func u256DivMod128(n Uint256, d Uint128) (q Uint256, r Uint128) {
	if d == (Uint128{}) {
		panic(errDivideByZero)
	}
	var rh Uint128
	q.Hi, rh = u128Div(n.Hi, d)
	q.Lo, r, _ = u256Div128(Uint256{Hi: rh, Lo: n.Lo}, d)
	return q, r
}
