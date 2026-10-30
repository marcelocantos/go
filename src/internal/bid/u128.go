// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// 128-bit unsigned arithmetic on Uint128, built on math/bits so that the
// compiler turns each carry and each wide multiply or divide into a single
// instruction. None of these functions allocates. Every result is exact:
// an operation whose result can exceed its result type returns the carry,
// the borrow, the high word or the wider type that holds the rest, and
// never wraps without saying so. The 256-bit counterparts are in u256.go.
//
// Functions that panic do so only for a zero divisor, as the / operator
// does, and say so.

// errDivideByZero is the panic value of the division functions in this
// package for a zero divisor. The 128-by-64 division inherits the
// math/bits panic (a run-time divide error) instead.
const errDivideByZero = "internal/bid: division by zero"

// u128Add returns a + b modulo 2^128 and the carry out of bit 127 (0 or 1).
func u128Add(a, b Uint128) (sum Uint128, carry uint64) {
	sum.Lo, carry = bits.Add64(a.Lo, b.Lo, 0)
	sum.Hi, carry = bits.Add64(a.Hi, b.Hi, carry)
	return sum, carry
}

// u128Sub returns a - b modulo 2^128 and the borrow out of bit 127 (0 or 1;
// 1 exactly when a < b).
func u128Sub(a, b Uint128) (diff Uint128, borrow uint64) {
	diff.Lo, borrow = bits.Sub64(a.Lo, b.Lo, 0)
	diff.Hi, borrow = bits.Sub64(a.Hi, b.Hi, borrow)
	return diff, borrow
}

// u128Cmp returns -1, 0 or +1 as a is less than, equal to or greater than b.
func u128Cmp(a, b Uint128) int {
	if a.Hi != b.Hi {
		if a.Hi < b.Hi {
			return -1
		}
		return 1
	}
	if a.Lo != b.Lo {
		if a.Lo < b.Lo {
			return -1
		}
		return 1
	}
	return 0
}

// u128Lt reports whether a < b.
func u128Lt(a, b Uint128) bool {
	if a.Hi != b.Hi {
		return a.Hi < b.Hi
	}
	return a.Lo < b.Lo
}

// u128Or returns a | b.
func u128Or(a, b Uint128) Uint128 {
	return Uint128{Hi: a.Hi | b.Hi, Lo: a.Lo | b.Lo}
}

// u128Len returns the length of a in bits: 0 for zero, else the position of
// the highest set bit plus one.
func u128Len(a Uint128) int {
	if a.Hi != 0 {
		return 64 + bits.Len64(a.Hi)
	}
	return bits.Len64(a.Lo)
}

// u128Shl returns a << n. Bits shifted out are lost, and a shift of 128 or
// more yields zero, as for the built-in shift operators. The expressions
// below rely on the same rule: a shift count of 64 or more produces zero
// for its term, so no case analysis on n is needed.
func u128Shl(a Uint128, n uint) Uint128 {
	return Uint128{
		Hi: a.Hi<<n | a.Lo>>(64-n) | a.Lo<<(n-64),
		Lo: a.Lo << n,
	}
}

// u128Shr returns a >> n (a logical shift). A shift of 128 or more yields
// zero.
func u128Shr(a Uint128, n uint) Uint128 {
	return Uint128{
		Hi: a.Hi >> n,
		Lo: a.Lo>>n | a.Hi<<(64-n) | a.Hi>>(n-64),
	}
}

// u128Mul64 returns the exact product a × b as the 192-bit value
// hi·2^128 + p. Nothing is discarded: hi is the word that a wrapping
// 128-bit multiply would lose.
func u128Mul64(a Uint128, b uint64) (p Uint128, hi uint64) {
	h0, l0 := bits.Mul64(a.Lo, b)
	h1, l1 := bits.Mul64(a.Hi, b)
	var c uint64
	p.Hi, c = bits.Add64(l1, h0, 0)
	p.Lo = l0
	return p, h1 + c // cannot overflow: the product is below 2^192
}

// u128Mul returns the exact 256-bit product a × b.
func u128Mul(a, b Uint128) Uint256 {
	// a × b = a × b.Lo + (a × b.Hi) << 64, each a 192-bit product.
	p, p2 := u128Mul64(a, b.Lo)
	q, q2 := u128Mul64(a, b.Hi)
	var r Uint256
	var c uint64
	r.Lo.Lo = p.Lo
	r.Lo.Hi, c = bits.Add64(p.Hi, q.Lo, 0)
	r.Hi.Lo, c = bits.Add64(p2, q.Hi, c)
	r.Hi.Hi = q2 + c // cannot overflow: the product is below 2^256
	return r
}

// u128Div64 returns the quotient and remainder of a / d. The quotient is a
// full 128 bits: a / d < 2^128 always, so there is no precondition beyond
// d != 0. It panics with the math/bits divide error if d is zero.
func u128Div64(a Uint128, d uint64) (q Uint128, r uint64) {
	q.Hi, r = bits.Div64(0, a.Hi, d)
	q.Lo, r = bits.Div64(r, a.Lo, d)
	return q, r
}

// u128Div returns the quotient and remainder of a / d, with
// 0 <= r < d. It panics if d is zero.
func u128Div(a, d Uint128) (q, r Uint128) {
	if d.Hi == 0 {
		var r64 uint64
		q, r64 = u128Div64(a, d.Lo)
		return q, Uint128{Lo: r64}
	}
	// d != 0 gives Hi = 0 < d, so the 256-by-128 division always succeeds.
	q, r, _ = u256Div128(Uint256{Lo: a}, d)
	return q, r
}

// u128GtMaxCoeff reports whether c exceeds the largest decimal128
// coefficient, 10^34 - 1.
func u128GtMaxCoeff(c Uint128) bool {
	if c.Hi > MaxCoeff128Hi {
		return true
	}
	if c.Hi < MaxCoeff128Hi {
		return false
	}
	return c.Lo > MaxCoeff128Lo
}

// u128GePow10 reports whether c >= 10^n. A negative n is treated as 0. For
// every n for which 10^n does not fit a Uint128 (n > 38) the answer is
// false.
func u128GePow10(c Uint128, n int) bool {
	n = max(n, 0)
	if n >= len(pow10tab128) {
		return false
	}
	return !u128Lt(c, pow10tab128[n])
}
