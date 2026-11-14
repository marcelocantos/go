// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// mulSlow64 returns x × y rounded in mode. Finite canonical operands of
// either encoding form, in a mode that rounds to nearest, are multiplied
// here, and every other operand, and a result that this path does not
// encode, goes to mulLong64, which decides it from the start.
//
// The product of the coefficients is exact in two words and below 10^32.
// With 16 or fewer digits it is the result, at the sum of the exponents,
// as in mulLong64's word path. With more, the j digits beyond 16 go, as
// in mulSlow128: the product is biased by half of 10^j, so that the
// quotient is the product rounded half up, and a zero remainder is the
// tie, which takes the even neighbour by clearing the low bit. The
// quotient has 16 digits, or is 10^16 after a carry, and its exponent is
// the sum of the exponents and j, where round64 puts it too when that is
// within the format.
//
// One entry of prodTab64, chosen by the leading zeros of the product,
// holds everything the rounding needs: the power of ten that decides
// between the two digit counts the product's bit length allows, and for
// each of them 10^j and its reciprocal at a shift that the bit length
// fixes. The power and both reciprocals are read at once and the borrow
// of the product less the power chooses between them, so that the
// division does not wait for the digit count to finish before its operands
// are read. The whole path calls nothing, and branches on the kind of its
// operands, on the rare tie and carry, and in packSign64 on the encoding
// form of the result, which is the second for one product of random
// operands in thirty.
//
// What packSign64 refuses is handed on: a quotient that the rounding took
// to 10^16, and an exponent outside the range, where the result is
// subnormal, clamped or an overflow.
//
// The compiler calls this function directly, so its parameters and result
// are a contract with it: see the end of "Inline fast paths" in the package
// documentation.
func mulSlow64(x, y uint64, mode Mode) uint64 {
	xc, xe := fields64(x)
	yc, ye := fields64(y)
	// Operands that are not finite and canonical, and the directed modes.
	// These are branches on the kind of the operands, and they measured
	// faster than one test of the bits of all five (5 percent).
	if xc > MaxCoeff64 || yc > MaxCoeff64 || xe >= specialExp64 || ye >= specialExp64 || mode > ToNearestAway {
		return mulLong64(x, y, mode)
	}
	sign := (x ^ y) & SignMask64
	exp := xe + ye - 2*Bias64
	hi, lo := bits.Mul64(xc, yc)
	if hi == 0 && lo <= MaxCoeff64 {
		if r, ok := packSign64(sign, lo, exp); ok {
			return r
		}
		return mulLong64(x, y, mode)
	}

	// The leading zeros of the product, from 21 to 74: those of the low
	// word count only when the high word is all zeros.
	z := uint(bits.LeadingZeros64(hi))
	z += uint(bits.LeadingZeros64(lo)) & -(z >> 6)
	e := &prodTab64[z]
	_, bw := bits.Sub64(lo, e.pow.Lo, 0)
	_, bw = bits.Sub64(hi, e.pow.Hi, bw)
	d, m, d0, m0 := e.d1, e.m1, e.d0, e.m0
	if bw != 0 {
		d, m = d0, m0 // the product has t digits, not t + 1
	}
	// The estimate (see prodRound64), and the remainder of the biased
	// product against it, which is below 2 × 10^j and exact in the low
	// word; one comparison corrects an estimate that is one short.
	q, _ := bits.Mul64(lo>>(e.s&63)|hi<<(e.u&63), m)
	r := lo + d>>1 - q*d
	if r >= d {
		q, r = q+1, r-d
	}
	if r == 0 && mode == ToNearestEven {
		q &^= 1 // a tie: the even neighbour is the one below
	}
	if r, ok := packSign64(sign, q, exp+int(e.j)-int(bw)); ok {
		return r
	}
	return mulLong64(x, y, mode)
}

// prodRound64 is the rounding of a decimal64 product of n bits, from 54 to
// 107, to 16 digits: prodTab64 holds one for each count of leading zeros,
// 128 - n. A product of n bits has t or t + 1 digits, t = ⌊n × 1233 /
// 4096⌋, as cLenFull64 counts them, and pow is 10^t, which it reaches
// exactly when it has t + 1. With t + 1 digits it loses j digits, d1 =
// 10^j, and with t it loses j - 1, d0 = 10^(j-1) (zero when j - 1 is not
// positive: such a product has 16 digits and does not round). m1 and m0
// are their reciprocals at the shift s = max(n - 62, 0), ⌊2^(64+s) / d⌋.
// With s = 0 the product has no high word, and the high word's shift u,
// 64 - s modulo 64, is zero too.
//
// The quotient wanted is q = ⌊P / d⌋ for the biased product P = p + d/2,
// the product rounded half up. Its estimate is q̂ = ⌊a × m / 2^64⌋ with a
// = ⌊p / 2^s⌋, the method of recip.go on the product without its bias,
// with a shift that the bit length fixes rather than the divisor, so that
// the product is shifted before the digit count is known. q̂ is q or q - 1.
// It is at most ⌊p / d⌋ ≤ q, because a ≤ p / 2^s and m ≤ 2^(64+s) / d.
// And p / d - a × m / 2^64 is below (a + m + 1) / 2^64, by recip.go's
// argument, which is below a half, because a < 2^62 and m < 2^62
// (TestProdTab64 checks both, and the estimate itself, for every entry):
// so a × m / 2^64 > p / d - 1/2 and q̂ ≥ ⌊p / d - 1/2⌋ = q - 1. The
// remainder P - q̂ × d is then below 2d < 2^55.
type prodRound64 struct {
	pow    Uint128
	d1, m1 uint64
	d0, m0 uint64
	j, s   uint8
	u      uint8     // (64 - s) mod 64, the shift of the high word
	_      [13]uint8 // to 64 bytes, so that an index is one shift
}

// mulSlow128 returns x × y rounded in mode. Finite operands of which one at
// least has a coefficient of two words, in a mode that rounds to nearest,
// are multiplied here, and every other operand, and a result that this
// path does not encode, goes to mulLong128, which decides it from the
// start. Operands of one word each are among those: the word path in front
// of mulLong128 is theirs.
//
// The product of the coefficients has up to 68 digits in four words. With
// 34 or fewer it is the result, at the sum of the exponents. With more,
// the digits beyond 34 go: as in division and addition the product is
// biased by half of the power of ten it is divided by, so that the
// quotient is the product rounded half up and a zero remainder is the tie,
// which takes the even neighbour by clearing the low bit. The division is
// u256DivPow10's, by the reciprocal of the power, and it and the digit
// count are written out here: this function calls nothing on its way, so
// nothing it holds goes through memory.
//
// What packWord128 refuses is handed on: a quotient that the rounding took
// to 10^34, and an exponent outside the range, where the result is
// subnormal, clamped or an overflow.
//
// The compiler calls this function directly, so its parameters and result
// are a contract with it: see the end of "Inline fast paths" in the package
// documentation.
func mulSlow128(x, y Uint128, mode Mode) Uint128 {
	// Coefficients of one word each go on at once: see addSlow128.
	if wordPair128(x, y) {
		return mulLong128(x, y, mode)
	}
	xhi, yhi, xe, ye, ok := addPair128(x, y)
	if !ok || mode > ToNearestAway {
		return mulLong128(x, y, mode)
	}
	// A coefficient that is not canonical is a zero. (A canonical zero
	// needs no test: its product has no digits, drops none, and is the
	// zero at the sum of the exponents, or packWord128 refuses it.)
	if u128GtMaxCoeff(Uint128{Hi: xhi, Lo: x.Lo}) || u128GtMaxCoeff(Uint128{Hi: yhi, Lo: y.Lo}) {
		return mulLong128(x, y, mode)
	}
	h00, p0 := bits.Mul64(x.Lo, y.Lo)
	h01, l01 := bits.Mul64(x.Lo, yhi)
	h10, l10 := bits.Mul64(xhi, y.Lo)
	h11, l11 := bits.Mul64(xhi, yhi)
	p1, c1 := bits.Add64(h00, l01, 0)
	p1, c2 := bits.Add64(p1, l10, 0)
	p2, c1 := bits.Add64(h01, h10, c1)
	p2, c2 = bits.Add64(p2, l11, c2)
	p3 := h11 + c1 + c2
	neg := (x.Hi^y.Hi)>>63 != 0
	exp := xe + ye - 2*Bias128
	s := Uint128{Hi: p1, Lo: p0}

	// The digits of the product, as cLenFull128 counts them: from its bit
	// length, corrected by the borrow of the product less the power of
	// ten that the length suggests. How many words the product has goes
	// with the operands' lengths and is a branch; the borrow is in its
	// digits.
	n := bits.Len64(p0)
	switch {
	case p3 != 0:
		n = 192 + bits.Len64(p3)
	case p2 != 0:
		n = 128 + bits.Len64(p2)
	case p1 != 0:
		n = 64 + bits.Len64(p1)
	}
	t := n * 1233 >> 12
	pt := &pow10tab256[t]
	_, bw := bits.Sub64(p0, pt.Lo.Lo, 0)
	_, bw = bits.Sub64(p1, pt.Lo.Hi, bw)
	_, bw = bits.Sub64(p2, pt.Hi.Lo, bw)
	_, bw = bits.Sub64(p3, pt.Hi.Hi, bw)
	if j := t + 1 - int(bw) - Precision128; j > 0 {
		// Biased by half of 10^j and divided by it: u256DivPow10's
		// method (see recip.go), written out here so that this
		// function calls nothing and keeps its operands in registers.
		// The quotient is at most 10^34, within that method's bound.
		d := &pow10tab128[j]
		var c uint64
		p0, c = bits.Add64(p0, d.Lo>>1|d.Hi<<63, 0)
		p1, c = bits.Add64(p1, d.Hi>>1, c)
		p2, c = bits.Add64(p2, 0, c)
		p3 += c
		// a is the biased product shifted down by the reciprocal's
		// shift, which is below 128: by a word if it has one, then by
		// its bits.
		e := &recip10tab128[j]
		u0, u1, u2 := p0, p1, p2
		if e.s >= 64 {
			u0, u1, u2 = p1, p2, p3
		}
		sh := uint(e.s) & 63
		al := u0>>sh | u1<<1<<(63-sh)
		ah := u1>>sh | u2<<1<<(63-sh)
		// The estimate: the high half of a times the reciprocal.
		h00, _ := bits.Mul64(al, e.m.Lo)
		h01, l01 := bits.Mul64(al, e.m.Hi)
		h10, l10 := bits.Mul64(ah, e.m.Lo)
		h11, l11 := bits.Mul64(ah, e.m.Hi)
		mid, c1 := bits.Add64(h00, l01, 0)
		_, c2 := bits.Add64(mid, l10, 0)
		ql, c1 := bits.Add64(h01, h10, c1)
		ql, c2 = bits.Add64(ql, l11, c2)
		qh := h11 + c1 + c2
		// The remainder against the estimate, exact in two words, and
		// the correction of an estimate one short, which the product's
		// low digits decide as often as not: a mask, not a branch.
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
	return mulLong128(x, y, mode)
}
