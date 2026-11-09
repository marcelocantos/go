// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// Division of a 128-bit value by a power of ten, by multiplication.
//
// A divide by 10^k is the step that removes digits, and with a divisor
// chosen at run time neither the 128-by-64 bits.Div64 (software on ARM64)
// nor the 256-by-128 division is cheap. Both are replaced here by a
// multiply with a precomputed reciprocal and one correction.
//
// The reciprocal is m_k = ⌊(2^128 - 1) / 10^k⌋, for 0 <= k <= 38. For k >= 1
// that is ⌊2^128 / 10^k⌋, since 10^k does not divide 2^128; for k = 0 it is
// 2^128 - 1, so the same code serves k = 0 with no branch. In every case
// 2^128/10^k - 1 < m_k <= 2^128/10^k.
//
// Claim: for every 0 <= c < 2^128, the estimate q̂ = ⌊c·m_k / 2^128⌋ is the
// quotient q = ⌊c / 10^k⌋ or q - 1.
//
// Proof. From m_k <= 2^128/10^k, c·m_k/2^128 <= c/10^k, so q̂ <= q. From
// m_k > 2^128/10^k - 1, c·m_k/2^128 > c/10^k - c/2^128 > c/10^k - 1 >= q - 1,
// and the floor of a real above the integer q - 1 is at least q - 1. ∎
//
// The candidate remainder r̂ = c - q̂·10^k is then r or r + 10^k, which is
// below 2·10^k <= 2·10^38 < 2^128, so it is computed exactly in 128-bit
// arithmetic modulo 2^128 (the true value fits, so the wrap cannot occur).
// One comparison r̂ >= 10^k decides the correction q = q̂ + 1, r = r̂ - 10^k,
// after which c = q·10^k + r with 0 <= r < 10^k exactly: nothing is left
// estimated. TestRecip128Table recomputes the table (reciptab.go, written
// by gen_recip.go) with math/big, and TestRecip128 checks the
// quotient and remainder against math/big at every k on each side of every
// multiple boundary it tries.
//
// The high half of the 256-bit product c·m_k is formed from four word
// products. For k >= 20, m_k < 2^128/10^20 < 2^64, so m_k.Hi is zero and two
// of the products vanish; for k <= 19, 10^k fits a word, so the back
// multiplication is two word products. q̂ < 2^128 always.

// divPow10Recip128 returns ⌊c / 10^k⌋ and c mod 10^k. Precondition:
// 0 <= k <= 38; a k outside the table panics with an index error.
func divPow10Recip128(c Uint128, k int) (q, r Uint128) {
	m := recipTrunc10tab128[k]
	d := pow10tab128[k]
	if m.Hi == 0 {
		// k >= 20: m < 2^64 and q < 2^128/10^20 < 2^62, but d needs two
		// words. c·m = c.Hi·m·2^64 + c.Lo·m, and its high 128 bits are
		// (c.Hi·m + ⌊c.Lo·m / 2^64⌋) / 2^64 rounded down.
		h0, _ := bits.Mul64(c.Lo, m.Lo)
		h1, l1 := bits.Mul64(c.Hi, m.Lo)
		_, carry := bits.Add64(l1, h0, 0)
		qlo := h1 + carry
		ph, pl := bits.Mul64(qlo, d.Lo)
		ph += qlo * d.Hi
		var b uint64
		r.Lo, b = bits.Sub64(c.Lo, pl, 0)
		r.Hi, _ = bits.Sub64(c.Hi, ph, b)
		if !u128Lt(r, d) {
			qlo++
			r, _ = u128Sub(r, d)
		}
		return Uint128{Lo: qlo}, r
	}
	// k <= 19: d < 2^64.
	h00, _ := bits.Mul64(c.Lo, m.Lo)
	h01, l01 := bits.Mul64(c.Lo, m.Hi)
	h10, l10 := bits.Mul64(c.Hi, m.Lo)
	h11, l11 := bits.Mul64(c.Hi, m.Hi)
	// Sum the middle column (h00 + l01 + l10) only for its carries, then
	// the upper columns.
	t, c1 := bits.Add64(h00, l01, 0)
	_, c2 := bits.Add64(t, l10, 0)
	q.Lo, c1 = bits.Add64(l11, h01, c1)
	q.Hi = h11 + c1
	q.Lo, c2 = bits.Add64(q.Lo, h10, c2)
	q.Hi += c2
	// r̂ = c - q̂·d < 2·d < 2^65: the low word and one borrow word suffice.
	ph, pl := bits.Mul64(q.Lo, d.Lo)
	ph += q.Hi * d.Lo
	var b uint64
	rlo, b := bits.Sub64(c.Lo, pl, 0)
	rhi, _ := bits.Sub64(c.Hi, ph, b)
	if rhi != 0 || rlo >= d.Lo {
		q, _ = u128Add(q, Uint128{Lo: 1})
		rlo -= d.Lo // r̂ - d < d < 2^64: the low word is the whole value
	}
	return q, Uint128{Lo: rlo}
}

// Exact division by a power of ten. A coefficient divides by 10^k = 2^k × 5^k
// exactly when its low k bits are zero and n = c / 2^k divides by 5^k, and
// for an odd d the second test needs no division. Multiplication by the
// inverse I of d modulo 2^N is a bijection of [0, 2^N), and it maps each
// multiple j·d below 2^N to j, so it maps the multiples onto [0, L] with
// L = ⌊(2^N - 1) / d⌋ and, being a bijection, nothing else there. Hence d
// divides n exactly when n·I mod 2^N <= L, and then n·I mod 2^N is n / d.
// TestDivExactPow10 recomputes the tables and checks the division at every
// k against math/big.

// A divExact5x128 holds, for 5^k, its inverse modulo 2^128 and the limit L.
type divExact5x128 struct{ inv, lim Uint128 }

// divExactPow10Recip128 returns c / 10^k and true if 10^k divides c, and
// false otherwise. Any k >= 0 is allowed.
func divExactPow10Recip128(c Uint128, k int) (q Uint128, ok bool) {
	if k >= len(divExact5tab128) {
		return Uint128{}, c == Uint128{} // 10^k > 2^128 > c
	}
	n := u128Shr(c, uint(k))
	if u128Shl(n, uint(k)) != c {
		return Uint128{}, false
	}
	t := &divExact5tab128[k]
	hi, lo := bits.Mul64(n.Lo, t.inv.Lo)
	q = Uint128{Hi: hi + n.Lo*t.inv.Hi + n.Hi*t.inv.Lo, Lo: lo}
	return q, !u128Lt(t.lim, q)
}
