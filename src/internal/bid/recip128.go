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
// estimated. TestRecip128 recomputes the table with math/big and checks the
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

// recipTrunc10tab128[k] is ⌊(2^128 - 1) / 10^k⌋; see divPow10Recip128.
// TestRecip128 recomputes it.
var recipTrunc10tab128 = [...]Uint128{
	{0xffffffffffffffff, 0xffffffffffffffff},
	{0x1999999999999999, 0x9999999999999999},
	{0x028f5c28f5c28f5c, 0x28f5c28f5c28f5c2},
	{0x004189374bc6a7ef, 0x9db22d0e56041893},
	{0x00068db8bac710cb, 0x295e9e1b089a0275},
	{0x0000a7c5ac471b47, 0x84230fcf80dc3372},
	{0x000010c6f7a0b5ed, 0x8d36b4c7f3493858},
	{0x000001ad7f29abca, 0xf485787a6520ec08},
	{0x0000002af31dc461, 0x1873bf3f70834acd},
	{0x000000044b82fa09, 0xb5a52cb98b405447},
	{0x000000006df37f67, 0x5ef6eadf5ab9a207},
	{0x000000000afebff0, 0xbcb24aafef78f69a},
	{0x0000000001197998, 0x12dea11197f27f0f},
	{0x00000000001c25c2, 0x68497681c2650cb4},
	{0x000000000002d093, 0x70d42573603d4e12},
	{0x000000000000480e, 0xbe7b9d58566c87ce},
	{0x0000000000000734, 0xaca5f6226f0ada61},
	{0x00000000000000b8, 0x77aa3236a4b44909},
	{0x0000000000000012, 0x725dd1d243aba0e7},
	{0x0000000000000001, 0xd83c94fb6d2ac34a},
	{0x0000000000000000, 0x2f394219248446ba},
	{0x0000000000000000, 0x04b8ed0283a6d3df},
	{0x0000000000000000, 0x0078e480405d7b96},
	{0x0000000000000000, 0x000c16d9a0095928},
	{0x0000000000000000, 0x0001357c299a88ea},
	{0x0000000000000000, 0x00001ef2d0f5da7d},
	{0x0000000000000000, 0x00000318481895d9},
	{0x0000000000000000, 0x0000004f3a68dbc8},
	{0x0000000000000000, 0x00000007ec3daf94},
	{0x0000000000000000, 0x00000000cad2f7f5},
	{0x0000000000000000, 0x0000000014484bfe},
	{0x0000000000000000, 0x0000000002073acc},
	{0x0000000000000000, 0x000000000033ec47},
	{0x0000000000000000, 0x000000000005313a},
	{0x0000000000000000, 0x00000000000084ec},
	{0x0000000000000000, 0x0000000000000d4a},
	{0x0000000000000000, 0x0000000000000154},
	{0x0000000000000000, 0x0000000000000022},
	{0x0000000000000000, 0x0000000000000003},
}
