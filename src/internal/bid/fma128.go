// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// fmaWordSlow128 is fmaLong128 under a name that the compiler charges as a
// cheap call, so that FMA128, which calls it for operands of one word
// each, inlines into its callers; it inlines into them itself, and they
// call fmaLong128.
func fmaWordSlow128(x, y, z Uint128, mode Mode) Uint128 {
	return fmaLong128(x, y, z, mode)
}

// fmaSlow128 returns x × y + z rounded once in mode, for operands that
// FMA128 did not send to fmaLong128's word paths: one of them at least has
// a coefficient of two words. Finite operands with z at the exponent of
// the product or up to 38 places above it, in a mode that rounds to
// nearest, are computed here, and every other operand, and a result that
// this path does not encode, goes to fmaLong128, which decides it from the
// start.
// That is the step of a sum of products or of a polynomial at full
// precision: the product of two such values has its exponent far below
// that of the value it is added to.
//
// The method is fmaLong128's in the arithmetic these operands need, with
// no call, as mulSlow128 is mulLong128's. The product of the coefficients
// is exact in four words, below 10^68. z is scaled to the exponent of the
// product, by one multiplication of two words by two: below 10^34 × 10^38,
// which four words hold. Their sum or difference is the exact value of
// x × y + z at the exponent of the product, the preferred one, and is
// rounded once, as mulSlow128 rounds its product: biased by half of the
// power of ten that takes it to 34 digits and divided by that power, with
// a zero remainder the tie. A sum of 34 digits or fewer is the result as
// it stands.
//
// What is handed on: a sum that is zero, whose sign and exponent have
// rules of their own; one of more than 72 digits, beyond the table of
// powers that divide; and what packWord128 refuses, a quotient that the
// rounding took to 10^34 and an exponent outside the range.
func fmaSlow128(x, y, z Uint128, mode Mode) Uint128 {
	xhi, yhi, xe, ye, ok := addPair128(x, y)
	zhi, ze, zok := finite128(z)
	if !ok || !zok || mode > ToNearestAway {
		return fmaLong128(x, y, z, mode)
	}
	// A coefficient that is not canonical is a zero.
	if u128GtMaxCoeff(Uint128{Hi: xhi, Lo: x.Lo}) || u128GtMaxCoeff(Uint128{Hi: yhi, Lo: y.Lo}) || u128GtMaxCoeff(Uint128{Hi: zhi, Lo: z.Lo}) {
		return fmaLong128(x, y, z, mode)
	}
	// g is how far z lies above the product.
	g := ze - (xe + ye - Bias128)
	if uint(g) >= uint(len(pow10tab128)) {
		return fmaLong128(x, y, z, mode)
	}

	// The product, as in mulSlow128.
	h00, p0 := bits.Mul64(x.Lo, y.Lo)
	h01, l01 := bits.Mul64(x.Lo, yhi)
	h10, l10 := bits.Mul64(xhi, y.Lo)
	h11, l11 := bits.Mul64(xhi, yhi)
	p1, c1 := bits.Add64(h00, l01, 0)
	p1, c2 := bits.Add64(p1, l10, 0)
	p2, c1 := bits.Add64(h01, h10, c1)
	p2, c2 = bits.Add64(p2, l11, c2)
	p3 := h11 + c1 + c2

	// z × 10^g.
	t := &pow10tab128[g]
	k00, z0 := bits.Mul64(z.Lo, t.Lo)
	k01, m01 := bits.Mul64(z.Lo, t.Hi)
	k10, m10 := bits.Mul64(zhi, t.Lo)
	k11, m11 := bits.Mul64(zhi, t.Hi)
	z1, c1 := bits.Add64(k00, m01, 0)
	z1, c2 = bits.Add64(z1, m10, 0)
	z2, c1 := bits.Add64(k01, k10, c1)
	z2, c2 = bits.Add64(z2, m11, c2)
	z3 := k11 + c1 + c2

	// The sum, with the sign of the product unless z exceeds it. Which
	// of the two it is goes with the signs of the data, as the rest of
	// this path goes with its lengths.
	neg := (x.Hi^y.Hi)>>63 != 0
	var c uint64
	if (x.Hi^y.Hi^z.Hi)>>63 == 0 {
		p0, c = bits.Add64(p0, z0, 0)
		p1, c = bits.Add64(p1, z1, c)
		p2, c = bits.Add64(p2, z2, c)
		p3 += z3 + c
	} else {
		p0, c = bits.Sub64(p0, z0, 0)
		p1, c = bits.Sub64(p1, z1, c)
		p2, c = bits.Sub64(p2, z2, c)
		p3, c = bits.Sub64(p3, z3, c)
		if c != 0 {
			// z is the larger: the sum is its difference from the
			// product, with its sign.
			p0, c = bits.Sub64(0, p0, 0)
			p1, c = bits.Sub64(0, p1, c)
			p2, c = bits.Sub64(0, p2, c)
			p3 = -p3 - c
			neg = !neg
		}
	}
	if p0|p1|p2|p3 == 0 {
		return fmaLong128(x, y, z, mode)
	}
	exp := xe + ye - 2*Bias128
	s := Uint128{Hi: p1, Lo: p0}

	// The digits of the sum, and its rounding to 34 of them: mulSlow128's
	// (see there), with up to 73 digits where a product has 68.
	n := bits.Len64(p0)
	switch {
	case p3 != 0:
		n = 192 + bits.Len64(p3)
	case p2 != 0:
		n = 128 + bits.Len64(p2)
	case p1 != 0:
		n = 64 + bits.Len64(p1)
	}
	d10 := n * 1233 >> 12
	pt := &pow10tab256[d10]
	_, bw := bits.Sub64(p0, pt.Lo.Lo, 0)
	_, bw = bits.Sub64(p1, pt.Lo.Hi, bw)
	_, bw = bits.Sub64(p2, pt.Hi.Lo, bw)
	_, bw = bits.Sub64(p3, pt.Hi.Hi, bw)
	if j := d10 + 1 - int(bw) - Precision128; j > 0 {
		if j >= len(pow10tab128) {
			return fmaLong128(x, y, z, mode)
		}
		d := &pow10tab128[j]
		p0, c = bits.Add64(p0, d.Lo>>1|d.Hi<<63, 0)
		p1, c = bits.Add64(p1, d.Hi>>1, c)
		p2, c = bits.Add64(p2, 0, c)
		p3 += c
		e := &recip10tab128[j]
		u0, u1, u2 := p0, p1, p2
		if e.s >= 64 {
			u0, u1, u2 = p1, p2, p3
		}
		sh := uint(e.s) & 63
		al := u0>>sh | u1<<1<<(63-sh)
		ah := u1>>sh | u2<<1<<(63-sh)
		h00, _ := bits.Mul64(al, e.m.Lo)
		h01, l01 := bits.Mul64(al, e.m.Hi)
		h10, l10 := bits.Mul64(ah, e.m.Lo)
		h11, l11 := bits.Mul64(ah, e.m.Hi)
		mid, c1 := bits.Add64(h00, l01, 0)
		_, c2 := bits.Add64(mid, l10, 0)
		ql, c1 := bits.Add64(h01, h10, c1)
		ql, c2 = bits.Add64(ql, l11, c2)
		qh := h11 + c1 + c2
		rh, rl := bits.Mul64(ql, d.Lo)
		rh += ql*d.Hi + qh*d.Lo
		rl, c = bits.Sub64(p0, rl, 0)
		rh, _ = bits.Sub64(p1, rh, c)
		_, lt := bits.Sub64(rl, d.Lo, 0)
		_, lt = bits.Sub64(rh, d.Hi, lt)
		ge := lt - 1 // all ones when the remainder reaches 10^j
		ql, c = bits.Add64(ql, ge&1, 0)
		qh += c
		rl, c = bits.Sub64(rl, d.Lo&ge, 0)
		rh, _ = bits.Sub64(rh, d.Hi&ge, c)
		if rl|rh == 0 && mode == ToNearestEven {
			ql &^= 1 // a tie: the even neighbour is the one below
		}
		s, exp = Uint128{Hi: qh, Lo: ql}, exp+j
	}
	if r, ok := packWord128(neg, s.Hi, s.Lo, exp); ok {
		return r
	}
	return fmaLong128(x, y, z, mode)
}
