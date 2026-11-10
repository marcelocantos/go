// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// div64 is Div64. A decimal64 division is word arithmetic throughout, and
// divLong64 is all of it.
func div64(x, y uint64, mode Mode) uint64 {
	return divLong64(x, y, mode)
}

// div128 is Div128. Finite operands whose coefficients fit a word, neither
// of them zero, in a mode that rounds to nearest (the division of amounts,
// rates and counts as they are usually held) are divided here, and every
// other operand, and a result outside the range that packWord128 encodes,
// goes to divLong128, which decides it from the start.
//
// The method is divLong128's (see divLongW) in the arithmetic that such
// operands need, and nothing is decoded twice, called or kept in memory on
// the way. The digit counts are those of a word. The leading ratio compares
// the coefficients, each scaled to the length of the longer, as two
// products of two words. The dividend x × 10^k is a word times a power of
// ten of two words, three words in all, because the quotient is below
// 10^34 and the divisor below 2^64; beyond the last 128-bit power the
// coefficient takes the excess first and is still a word.
//
// Three words are divided by one in the way the processor makes cheapest.
// With an instruction that divides two words by one, that is two of them.
// Without it, a divisor below 2^32 divides half a word at a time by the
// instruction that divides a word by a word, four times: a chain of
// divisions, but a tenth of the work of the reciprocal, and the divisors of
// money are that small. A larger divisor takes its reciprocal
// (reciprocalNewton) and two steps by it (div2by1).
//
// As in divLongW the dividend is biased by half the divisor, so that the
// quotient is rounded half up and the remainder says the rest: equal to
// the half exactly when the division is exact, when the quotient goes to
// strip128 for its preferred exponent, and zero under an even divisor
// exactly on a tie. The rounded quotient cannot reach 10^34: it would need
// b × 10^34 - a × 10^k, a positive multiple of 10^min(k, 34), to be at most
// b / 2, and k is at least 13 more than the digits of b. packWord128 checks
// the coefficient all the same, and the exponent, which is the only thing
// that can send a quotient of 34 digits elsewhere: at an exponent in range
// it is normal, and rounding it once is the whole result.
//
// The compiler calls this function directly, so its parameters and result
// are a contract with it: see the end of "Inline fast paths" in the package
// documentation.
func div128(x, y Uint128, mode Mode) Uint128 {
	exp, neg, ok := wordPair128(x, y)
	a, b := x.Lo, y.Lo
	if !ok || a == 0 || b == 0 || mode > ToNearestAway {
		return divLong128(x, y, mode)
	}
	db := wordDigits(b)
	if b == pow10word[(db-1)&31] {
		// A power of ten divides by moving the point.
		q, k := Uint128{Lo: a}, db-1
		if k > 0 {
			q, k = strip128(q, k, 1)
		}
		if r, ok := packWord128(neg, q.Hi, q.Lo, exp-k); ok {
			return r
		}
		return divLong128(x, y, mode)
	}

	// The reciprocal, when it is needed, is asked for first: it takes the
	// divisor alone, it is the longest chain here, and nothing else is
	// yet in a register for the call to displace.
	var s uint
	var d, v uint64
	small := b>>32 == 0
	if !div64Native && !small {
		s = uint(bits.LeadingZeros64(b)) & 63
		d = b << s
		v = reciprocalNewton(d)
	}
	da := wordDigits(a)

	// lt is 1 when the leading ratio is below one. Which coefficient is
	// the shorter depends on the operands as much as their digits do, so
	// both are scaled, one of them by 10^0, and nothing branches.
	e := db - da
	sign := e >> 63 // all ones when x is the longer
	ah, al := bits.Mul64(a, pow10word[(e&^sign)&31])
	bh, bl := bits.Mul64(b, pow10word[(-e&sign)&31])
	_, lt := bits.Sub64(al, bl, 0)
	_, lt = bits.Sub64(ah, bh, lt)
	k := Precision128 - 1 + e + int(lt)

	// The dividend a × 10^k, three words. Past the last 128-bit power, a
	// takes the excess: it has at most db - 4 digits then.
	m, p := a, k
	if k >= len(pow10tab128) {
		p = len(pow10tab128) - 1
		m = a * pow10word[(k-p)&31]
	}
	t := &pow10tab128[p]
	h0, n0 := bits.Mul64(m, t.Lo)
	n2, l1 := bits.Mul64(m, t.Hi)
	n1, c := bits.Add64(h0, l1, 0)
	n2 += c
	half := b >> 1
	n0, c = bits.Add64(n0, half, 0)
	n1, c = bits.Add64(n1, 0, c)
	n2 += c

	// The quotient is below 2^113, so n2 is below b >> 15: the
	// precondition of each form of the first step.
	var q1, q0, rem uint64
	if div64Native {
		q1, rem = bits.Div64(n2, n1, b)
		q0, rem = bits.Div64(rem, n0, b)
	} else if small {
		// The top 96 bits of the dividend are below 2^49, and each
		// remainder with the next half word is below 2^64.
		w := n2<<32 | n1>>32
		q3 := w / b
		w = (w-q3*b)<<32 | n1&(1<<32-1)
		q2 := w / b
		w = (w-q2*b)<<32 | n0>>32
		q1 = q3<<32 | q2
		q3 = w / b
		w = (w-q3*b)<<32 | n0&(1<<32-1)
		q2 = w / b
		q0 = q3<<32 | q2
		rem = w - q2*b
	} else {
		u2 := n2<<s | n1>>1>>((63-s)&63)
		u1 := n1<<s | n0>>1>>((63-s)&63)
		q1, rem = div2by1(u2, u1, d, v)
		q0, rem = div2by1(rem, n0<<s, d, v)
		rem >>= s
	}
	if rem == half {
		// Exact: the zeros that the scale put in go, as far as the
		// preferred exponent.
		first := 0
		if k < Precision128 {
			first = k
		}
		var q Uint128
		q, k = strip128(Uint128{Hi: q1, Lo: q0}, k, first)
		q1, q0 = q.Hi, q.Lo
	} else if rem == 0 && b&1 == 0 && mode != ToNearestAway {
		q0 &^= 1 // a tie: the even neighbour is the one below
	}
	if r, ok := packWord128(neg, q1, q0, exp-k); ok {
		return r
	}
	return divLong128(x, y, mode)
}

// pow10word is pow10tab64 in an array of 32, so that an index masked to
// five bits needs no bound test; TestPow10Word compares the two.
var pow10word = [32]uint64{
	1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9,
	1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18, 1e19,
}

// wordDigits returns the number of decimal digits of c, which is not zero.
// It is cDigits64 with the comparison as a borrow: whether c reaches the
// power of ten that its bit length suggests is in its leading digits, as
// good as random.
func wordDigits(c uint64) int {
	t := bits.Len64(c) * 1233 >> 12
	_, b := bits.Sub64(c, pow10word[t&31], 0)
	return t + 1 - int(b)
}
