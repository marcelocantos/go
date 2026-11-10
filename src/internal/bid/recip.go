// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// Division by a power of ten with a precomputed reciprocal.
//
// Every rounding removes digits, which is a division by 10^k. Long division
// normalises the divisor and estimates each quotient word with bits.Div64,
// which is a single instruction only on amd64 and a software loop
// elsewhere. A division by a constant is instead a multiplication by its
// reciprocal and a shift, and there are only 19 and 38 constants. The
// tables recip10tab64 and recip10tab128 in zpow10.go hold them (the 128-bit
// entries also hold the plainer reciprocal of recip128.go); gen_arith.go
// computes them with math/big, and TestRecipTables recomputes them.
//
// The method, for an n-bit word (n = 64 or 128) and d = 10^k, k >= 1. Let
// ℓ = bitlen(d) - 1, so 2^ℓ <= d < 2^(ℓ+1), let s = max(ℓ - 6, 0) and
// m = ⌊2^(n+s) / d⌋. For a dividend p < d × 2^(n-8), the quotient estimate
// is q̂ = ⌊a × m / 2^n⌋ with a = ⌊p / 2^s⌋. Then q̂ is q = ⌊p / d⌋ or q - 1:
//
//   - a < 2^(n-1). If s = ℓ - 6, a < d × 2^(n-8-s) < 2^(ℓ+1+n-8-ℓ+6); if
//     s = 0, d <= 100 < 2^7 and a = p < d × 2^(n-8) < 2^(n-1).
//   - m < 2^(n-3). If s = ℓ - 6, m <= 2^(n+ℓ-6) / 2^ℓ; if s = 0, m <= 2^n / 10.
//   - q̂ <= q, because a <= p / 2^s and m <= 2^(n+s) / d, so a × m / 2^n is
//     at most p / d.
//   - q < q̂ + 2. Write p / 2^s = a + α and 2^(n+s) / d = m + μ with α and μ
//     in [0, 1). Then p / d - a × m / 2^n = (a μ + α m + α μ) / 2^n, which
//     is below (a + m + 1) / 2^n < (2^(n-1) + 2^(n-3)) / 2^n < 1. So
//     q <= p / d < a × m / 2^n + 1 < q̂ + 2.
//
// The remainder r̂ = p - q̂ × d is therefore in [0, 2d), and 2d < 2^n for
// every d in the 128-bit table (2 × 10^38 < 2^128), so it is exact modulo
// 2^n: the low half of p less the low half of q̂ × d. In the 64-bit table
// 2 × 10^19 exceeds 2^64, so there r̂ keeps a second word. One comparison
// with d and one correction (q̂ + 1, r̂ - d) give q and r. The remainder is
// exact, so the digit class follows from it by class64 or class128, the
// same as after a long division: nothing about the rounding changes.
//
// The bound p < d × 2^(n-8) says that the quotient has at most n - 8 bits.
// Every quotient that the rounding routine keeps has at most Precision
// digits (10^16 < 2^56 and 10^34 < 2^120), so the rounding of a product
// never misses it; a caller with a larger quotient falls back to long
// division, which ok reports.

// recip64 and recip128 are the reciprocal m of 10^k and its shift s, as
// the method above defines them, for 64- and 128-bit words.
type recip64 struct {
	m uint64
	s uint8
}

type recip128 struct {
	m Uint128
	t Uint128 // ⌊(2^128 - 1) / 10^k⌋, for divPow10Recip128 (recip128.go)
	s uint8
}

// divPow10Recip64 returns q = ⌊p / 10^k⌋ and r = p - q × 10^k for
// 0 <= k <= 19 by the method above, and ok = true, when p < 10^k × 2^56.
// Otherwise it returns ok = false and zeros, and the caller divides.
func divPow10Recip64(p Uint128, k int) (q, r uint64, ok bool) {
	d := pow10tab64[k]
	if lim := d >> 8; p.Hi > lim || p.Hi == lim && p.Lo >= d<<56 {
		return 0, 0, false
	}
	if k == 0 {
		return p.Lo, 0, true // p < 2^56
	}
	e := recip10tab64[k]
	s := uint(e.s)
	a := p.Lo>>s | p.Hi<<(64-s) // a shift by 64 is zero, so s = 0 needs no case
	q, _ = bits.Mul64(a, e.m)
	hi, lo := bits.Mul64(q, d)
	r, b := bits.Sub64(p.Lo, lo, 0)
	rhi := p.Hi - hi - b
	if rhi != 0 || r >= d {
		q++
		r -= d // the second word of r̂ is at most 1, and r̂ - d < d
	}
	return q, r, true
}

// u256DivPow10 returns q = ⌊p / 10^k⌋ and r = p - q × 10^k for
// 0 <= k <= 38 by the method above, and ok = true, when p < 10^k × 2^120.
// Otherwise it returns ok = false and zeros, and the caller divides.
func u256DivPow10(p Uint256, k int) (q, r Uint128, ok bool) {
	d := pow10tab128[k]
	// p < d × 2^120, compared as 256-bit values: d × 2^120 is d >> 8 in
	// the high half and d << 120 in the low half.
	if lim := u128Shr(d, 8); p.Hi != lim {
		if !u128Lt(p.Hi, lim) {
			return Uint128{}, Uint128{}, false
		}
	} else if !u128Lt(p.Lo, u128Shl(d, 120)) {
		return Uint128{}, Uint128{}, false
	}
	if k == 0 {
		return p.Lo, Uint128{}, true // p < 2^120
	}
	e := recip10tab128[k]
	s := uint(e.s)
	a := u128Or(u128Shr(p.Lo, s), u128Shl(p.Hi, 128-s)) // s = 0 shifts p.Hi out
	q = u128Mul(a, e.m).Hi

	// r̂ = p - q̂ × d modulo 2^128, which is exact because r̂ < 2d < 2^128.
	hi, lo := bits.Mul64(q.Lo, d.Lo)
	hi += q.Lo*d.Hi + q.Hi*d.Lo
	var b uint64
	r.Lo, b = bits.Sub64(p.Lo.Lo, lo, 0)
	r.Hi, _ = bits.Sub64(p.Lo.Hi, hi, b)
	if !u128Lt(r, d) {
		q, _ = u128Add(q, Uint128{Lo: 1})
		r, _ = u128Sub(r, d)
	}
	return q, r, true
}
