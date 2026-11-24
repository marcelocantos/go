// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// This package's multiply on UBD operands. The ties-to-even entry
// (ubdMulTTE128) is ubdMulShort128 and then a two-word path written into
// the entry, so that a multiply is one call. ubdMulLong128 is the
// template's mulLongW with UBD's decoding and packing (signs at bit 49,
// exponents unbiased) and the rounding by ubdRound128. Nothing converts.

// ubdMulShort128 returns x × y, and true, for coefficients of one word each
// whose product has under 113 bits, below 2^112 < 10^34 and so exact, at an
// exponent sum in [-4096, 4095], inside the format. UBD's high word holds a
// product's top 48 bits below the sign at once, so products of up to 112
// bits, of two 15-digit coefficients for instance, are decided here. The
// three tests are one, without a branch: a special has the 0x1F prefix in
// its coefficient's high word, so the one-word test excludes it, and the
// exponent sum is in that range exactly when adding 4096 leaves it below
// 2^13. It inlines.
func ubdMulShort128(x, y UBD128) (UBD128, bool) {
	hi, lo := bits.Mul64(x.Lo, y.Lo)
	e := int64(x.Hi)>>UBDExpShift + int64(y.Hi)>>UBDExpShift
	return UBD128{Lo: lo, Hi: uint64(e)<<UBDExpShift | (x.Hi^y.Hi)&UBDSignHi | hi},
		(x.Hi|y.Hi)&UBDCoeffMaskHi|hi>>48|uint64(e+4096)>>13 == 0
}

// ubdMulTTE128, the multiply entry the compiler calls, is ubdMulShort128
// and then, for ties to even, a two-word path written into the entry, so
// that a multiply past the short path is one call with one frame. Finite
// canonical operands are multiplied there, and every other operand, and a
// result the path does not encode, goes to ubdMulLong128.
//
// The product of the coefficients has up to 68 digits in four words. With
// 34 or fewer it is the result, at the sum of the exponents. With more, the
// j digits beyond 34 go as in mulSlow64: the product is biased by half of
// 10^j, so that the quotient is the product rounded half up, and a zero
// remainder is the tie, which takes the even neighbour by clearing the low
// bit. The division is by the reciprocal of 10^j, and it and the digit
// count are written out here, so the path calls nothing.
//
// Coefficients of one word each take the two-word path like any others:
// the ones that reach it, after ubdMulShort128, are products of 112 bits or
// more, most of which round, and the two-word path rounds them in one call
// where ubdMulLong128 was a second (14.3 ns against 11.6). ubdMulRnd128,
// which has no caller in compiled code, calls this for ToNearestEven and
// ubdMulLong128 for every other mode.
func ubdMulTTE128(x, y UBD128) UBD128 {
	if r, ok := ubdMulShort128(x, y); ok {
		return r
	}
	const mode = ToNearestEven
	// A special has the 0x1F prefix in its coefficient's high word, which
	// the tag test finds.
	if x.Hi>>60 == UBDSpecialTag || y.Hi>>60 == UBDSpecialTag || mode > ToNearestAway {
		return ubdMulLong128(x, y, mode)
	}
	xhi, yhi := x.Hi&UBDCoeffMaskHi, y.Hi&UBDCoeffMaskHi
	xe, ye := int(int64(x.Hi)>>UBDExpShift), int(int64(y.Hi)>>UBDExpShift)
	if u128GtMaxCoeff(Uint128{Hi: xhi, Lo: x.Lo}) || u128GtMaxCoeff(Uint128{Hi: yhi, Lo: y.Lo}) {
		return ubdMulLong128(x, y, mode)
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
	neg := (x.Hi^y.Hi)&UBDSignHi != 0
	exp := xe + ye
	s := Uint128{Hi: p1, Lo: p0}

	// The digits of the product: t from its bit length, and one more when
	// the product is at least 10^t.
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
		// Biased by half of 10^j and divided by it through its reciprocal,
		// the quotient then corrected by at most one.
		d := &pow10tab128[j]
		var c uint64
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
		ge := lt - 1
		ql, c = bits.Add64(ql, ge&1, 0)
		qh += c
		rl, c = bits.Sub64(rl, d.Lo&ge, 0)
		rh, _ = bits.Sub64(rh, d.Hi&ge, c)
		if rl|rh == 0 && mode == ToNearestEven {
			ql &^= 1 // a tie: the even neighbour is the one below
		}
		s, exp = Uint128{Hi: qh, Lo: ql}, exp+j
	}
	if r, ok := ubdPackWord(neg, s.Hi, s.Lo, exp); ok {
		return r
	}
	return ubdMulLong128(x, y, mode)
}

// ubdMulLong128 is mulLongW (mul_tmpl.go) on UBD: a word product that fits
// packs at once; specials are decided on UBD (a NaN gives that NaN, an infinity
// times a zero the canonical NaN, an infinity times anything else the
// infinity of the product's sign); every other product is rounded by
// ubdRound128.
func ubdMulLong128(x, y UBD128, mode Mode) UBD128 {
	var yneg, yok bool
	var yw uint64
	var ye int
	xneg, xw, xe, xok := ubdUnpackWord(x)
	if xok {
		yneg, yw, ye, yok = ubdUnpackWord(y)
	}
	var xc, yc coef128
	if yok {
		hi, lo := bits.Mul64(xw, yw)
		if r, ok := ubdPackWord(xneg != yneg, hi, lo, xe+ye); ok {
			return r
		}
		xc, yc = cFromU64128(xw), cFromU64128(yw)
	} else {
		xs, ys := x.Hi>>60 == UBDSpecialTag, y.Hi>>60 == UBDSpecialTag
		xneg, yneg = x.Hi&UBDSignHi != 0, y.Hi&UBDSignHi != 0
		xc, xe = coef128{Hi: x.Hi & UBDCoeffMaskHi, Lo: x.Lo}, int(int64(x.Hi)>>UBDExpShift)
		yc, ye = coef128{Hi: y.Hi & UBDCoeffMaskHi, Lo: y.Lo}, int(int64(y.Hi)>>UBDExpShift)
		if xs || ys {
			if x.Hi>>59 == UBDNaNTag || y.Hi>>59 == UBDNaNTag {
				return ubdNaN2(x, y)
			}
			if !xs && cIsZero128(xc) || !ys && cIsZero128(yc) {
				return UBD128{Hi: UBDNaNHi} // an infinity times a zero
			}
			return ubdInf(xneg != yneg)
		}
	}
	return ubdRound128(xneg != yneg, cMulFull128(xc, yc), xe+ye, classZero, mode)
}
