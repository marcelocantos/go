// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// The rest of math/decimal's decimal128 operations on UBD: the payload
// operations, Quantum, SameQuantum, Reduce, QuoQuantize, Rem and Mod, and
// FMA. Each is this package's BID function of the same name without the
// UBD prefix, with UBD's decoding and packing and the arithmetic unchanged;
// none converts to BID. internal/decimal calls them where decimal128 is
// stored as UBD (NativeUBD128).

// ubdQNaN is UBD's canonical quiet NaN, the NaN of an invalid operation.
var ubdQNaN = UBD128{Hi: UBDNaNHi}

// ubdQuiet returns the NaN x made quiet: its signaling bit cleared.
func ubdQuiet(x UBD128) UBD128 { return UBD128{Lo: x.Lo, Hi: x.Hi &^ (1 << 58)} }

// ubdNaN3 is nan128 on UBD: the NaN result of an operation on x, y and z, at
// least one a NaN: the first signaling NaN, else the first NaN, made quiet.
func ubdNaN3(x, y, z UBD128) UBD128 {
	sig := func(v UBD128) bool { return v.Hi>>58 == UBDSNaNTag }
	nan := func(v UBD128) bool { return v.Hi>>59 == UBDNaNTag }
	switch {
	case sig(x), !sig(y) && !sig(z) && nan(x):
		return ubdQuiet(x)
	case sig(y), !sig(z) && nan(y):
		return ubdQuiet(y)
	}
	return ubdQuiet(z)
}

// ubdKindExp is kindExp128 on UBD: the class of x and, for a finite x, its
// exponent.
func ubdKindExp(x UBD128) (Kind, int) {
	if x.Hi>>60 == UBDSpecialTag {
		if x.Hi>>59 == UBDNaNTag {
			return KindNaN, 0
		}
		return KindInf, 0
	}
	return KindFinite, int(int64(x.Hi) >> UBDExpShift)
}

// ubdFinite packs a finite result known to be inside the format: a
// coefficient of at most Precision128 digits at an exponent in range.
func ubdFinite(neg bool, c coef128, exp int) UBD128 {
	h := uint64(int64(exp))<<UBDExpShift | c.Hi
	if neg {
		h |= UBDSignHi
	}
	return UBD128{Lo: c.Lo, Hi: h}
}

// UBDPayload128 is Payload128 on UBD: the payload of a NaN x as a finite
// value with exponent 0, or -1 if x is not a NaN. A NaN's 110-bit payload is
// its low 108 coefficient bits with the low two qExp bits above them; a
// well-formed NaN's is below 10^33.
func UBDPayload128(x UBD128) UBD128 {
	if x.Hi>>59 != UBDNaNTag {
		return ubdFinite(true, coef128{Lo: 1}, 0)
	}
	return ubdFinite(false, coef128{Hi: x.Hi&(1<<44-1) | x.Hi>>UBDExpShift&3<<44, Lo: x.Lo}, 0)
}

// UBDNaNWithPayload128 is NaNWithPayload128 (SignalingNaNWithPayload128 if
// signaling is set) on UBD: the positive NaN whose payload is the value of
// p, an integer in [0, 10^33) at any exponent, or +0 for any other p.
func UBDNaNWithPayload128(p UBD128, signaling bool) UBD128 {
	kind, neg, c, exp := UBDUnpack128(p)
	var plusZero UBD128
	if kind != KindFinite {
		return plusZero
	}
	if !cIsZero128(c) {
		if neg {
			return plusZero
		}
		switch {
		case exp > 0:
			if cDigits128(c)+exp > Precision128-1 {
				return plusZero
			}
			c = cMulPow10128(c, exp)
		case exp < 0:
			if -exp >= Precision128 {
				return plusZero
			}
			q, r := cDivPow10128(c, -exp)
			if !cIsZero128(r) {
				return plusZero
			}
			c = q
		}
		if !cLt128(c, cPow10128(Precision128-1)) {
			return plusZero
		}
	}
	hi := UBDNaNHi
	if signaling {
		hi = UBDSNaNHi
	}
	// The payload's low 108 bits in the coefficient, the two above them in
	// the low qExp bits.
	return UBD128{Lo: c.Lo, Hi: hi | c.Hi&(1<<44-1) | c.Hi>>44&3<<UBDExpShift}
}

// UBDQuantum128 is Quantum128 on UBD: 1 × 10^Q(x) for a finite x, +∞ for an
// infinity, and a NaN made quiet.
func UBDQuantum128(x UBD128) UBD128 {
	switch kind, exp := ubdKindExp(x); kind {
	case KindNaN:
		return ubdQuiet(x)
	case KindInf:
		return ubdInf(false)
	default:
		return ubdFinite(false, coef128{Lo: 1}, exp)
	}
}

// UBDSameQuantum128 is SameQuantum128 on UBD: both finite with one
// exponent, both infinities, or both NaNs.
func UBDSameQuantum128(x, y UBD128) bool {
	// Two specials of one kind have one tag down to bit 59 (a NaN's
	// signaling bit is below it); two finite values one qExp field. A
	// special and a finite value differ in both.
	if x.Hi>>60 == UBDSpecialTag || y.Hi>>60 == UBDSpecialTag {
		return x.Hi>>59 == y.Hi>>59
	}
	return (x.Hi^y.Hi)>>UBDExpShift == 0
}

// UBDReduce128 is Reduce128 on UBD: x with the trailing zeros of its
// coefficient removed, as far as the exponent range allows, a zero at
// exponent 0, an infinity as it is and a NaN made quiet.
func UBDReduce128(x UBD128) UBD128 {
	kind, neg, c, exp := UBDUnpack128(x)
	switch kind {
	case KindNaN:
		return ubdQuiet(x)
	case KindInf:
		return ubdInf(neg)
	}
	if cIsZero128(c) {
		exp = 0
	} else if q, ok := cDivExactPow10128(c, 1); ok && exp < MaxExp128 {
		// reduce128's binary search on the number of trailing zeros,
		// never past MaxExp128.
		c, exp = q, exp+1
		step := 1
		for step*2 <= Precision128 {
			step *= 2
		}
		for ; step > 0; step /= 2 {
			if exp+step > MaxExp128 {
				continue
			}
			if q, ok := cDivExactPow10128(c, step); ok {
				c, exp = q, exp+step
			}
		}
	}
	return ubdFinite(neg, c, exp)
}

// UBDQuoQuantize128 is QuoQuantize128 on UBD: x / y rounded once in mode to
// the exponent of q, or a NaN when the result needs another exponent.
func UBDQuoQuantize128(x, y, q UBD128, mode Mode) UBD128 {
	xk, xneg, xc, xe := UBDUnpack128(x)
	yk, yneg, yc, ye := UBDUnpack128(y)
	qk, e := ubdKindExp(q)
	neg := xneg != yneg
	switch {
	case xk == KindNaN || yk == KindNaN || qk == KindNaN:
		return ubdNaN3(x, y, q)
	case xk == KindInf && yk == KindInf:
		return ubdQNaN
	case xk == KindInf:
		if qk == KindInf {
			return ubdInf(neg)
		}
		return ubdQNaN
	case yk == KindInf:
		if qk == KindInf {
			return ubdQNaN
		}
		r, _ := ubdRoundFloor128(neg, wide128{}, e, classZero, mode, e)
		return r
	case cIsZero128(yc):
		if !cIsZero128(xc) && qk == KindInf {
			return ubdInf(neg)
		}
		return ubdQNaN
	case qk == KindInf:
		return ubdQNaN
	case cIsZero128(xc):
		r, _ := ubdRoundFloor128(neg, wide128{}, e, classZero, mode, e)
		return r
	}
	dx, dy := cDigits128(xc), cDigits128(yc)
	var c, rem, d coef128
	class := classZero
	if s := xe - ye - e; s >= 0 {
		if dx-1+s-dy >= Precision128 {
			return ubdQNaN
		}
		c, rem = cDivFull128(cMulPow10Full128(xc, s), yc)
		d = yc
	} else if t := -s; t > dx-dy+1 {
		class = classBelowHalf
	} else {
		d = cMulPow10128(yc, t)
		c, rem = cDivFull128(wide128{Lo: xc}, d)
	}
	if !cIsZero128(rem) {
		other, _ := cSub128(d, rem)
		class = cClass128(rem, other)
	}
	r, st := ubdRoundFloor128(neg, wide128{Lo: c}, e, class, mode, e)
	if rk, re := ubdKindExp(r); st&Overflow != 0 || rk != KindFinite || re != e {
		return ubdQNaN
	}
	return r
}

// UBDRem128 and UBDMod128 are Rem128 and Mod128 on UBD.
func UBDRem128(x, y UBD128) UBD128 { return ubdRemainder128(x, y, true) }
func UBDMod128(x, y UBD128) UBD128 { return ubdRemainder128(x, y, false) }

// ubdRemainder128 is remainder128 on UBD: x - y × n exactly, n the quotient
// |x| / |y| truncated, or rounded to nearest with ties to even if nearest
// is set. See remainder128 for the method, which is unchanged.
func ubdRemainder128(x, y UBD128, nearest bool) UBD128 {
	if xneg, X, xe, ok := ubdUnpackWord(x); ok {
		if _, Y, ye, ok := ubdUnpackWord(y); ok && Y != 0 {
			var hi uint64
			e := ye
			if k := uint(xe - ye); k < uint(len(pow10tab64)) {
				hi, X = bits.Mul64(X, pow10tab64[k])
			} else if k = uint(ye - xe); k < uint(len(pow10tab64)) {
				hi, Y = bits.Mul64(Y, pow10tab64[k])
				e = xe
			} else {
				hi = 1
			}
			if hi == 0 {
				q := X / Y
				r := X - q*Y
				if d := Y - r; nearest && (r > d || r == d && q&1 != 0) {
					r, xneg = d, !xneg
				}
				if z, ok := ubdPackWord(xneg, 0, r, e); ok {
					return z
				}
			}
		}
	}
	xk, neg, xc, xe := UBDUnpack128(x)
	yk, _, yc, ye := UBDUnpack128(y)
	if xk == KindNaN || yk == KindNaN {
		return ubdNaN2(x, y)
	}
	if xk != KindFinite || (yk == KindFinite && cIsZero128(yc)) {
		return ubdQNaN
	}
	if yk == KindInf {
		return ubdRemExact128(neg, xc, xe)
	}

	k, Y := xe-ye, yc
	if k < 0 {
		scaled, overflow := cScale128(yc, -k)
		if overflow {
			return ubdRemExact128(neg, xc, xe)
		}
		k, Y = 0, scaled
	}

	r, odd := xc, false
	if k > 0 || !cLt128(xc, Y) {
		m, _ := cAdd128(Y, Y)
		r = remScaled128(xc, k, m)
		if !cLt128(r, Y) {
			r, _ = cSub128(r, Y)
			odd = true
		}
	}

	if nearest {
		twice, _ := cAdd128(r, r)
		if c := cCmp128(twice, Y); c > 0 || (c == 0 && odd) {
			r, _ = cSub128(Y, r)
			neg = !neg
		}
	}
	return ubdRemExact128(neg, r, min(xe, ye))
}

// ubdRemExact128 is remExact128 on UBD: an exact result, which
// ubdRemainder128 has shown fits.
func ubdRemExact128(neg bool, c coef128, exp int) UBD128 {
	z, _ := ubdRoundFloor128(neg, cMulFull128(c, cPow10128(0)), exp, classZero, ToNearestEven, MinExp128)
	return z
}

// UBDFMA128 is FMA128 on UBD: x × y + z rounded once in mode. Three
// one-word coefficients go to ubdFMALong128, whose word paths come first;
// the rest try ubdFMASlow128's path first.
func UBDFMA128(x, y, z UBD128, mode Mode) UBD128 {
	if (x.Hi|y.Hi|z.Hi)&UBDCoeffMaskHi == 0 {
		return ubdFMALong128(x, y, z, mode)
	}
	return ubdFMASlow128(x, y, z, mode)
}

// ubdFMASlow128 is fmaSlow128 on UBD, with the paths of decimal128-go's
// d128_fma_tte in front of it and beside it: three finite operands, one
// of them at least of two words. A well-formed finite operand is
// canonical, so the coefficients need no bound test.
//
//   - A product of at most 112 bits at an exponent in range is exact and
//     a value of the format, and x × y + z is then the sum of it and z:
//     ubdAddTTE128, or ubdAddLong128 in another mode.
//   - z at the exponent of the product or up to 38 places above it, the
//     step of a sum of products: fmaSlow128's path, below.
//   - Every other gap, in ties to even: ubdFMAWide128, decimal128-go's exact
//     alignment and its path for an addend far above the product.
//
// What none of them finishes goes to ubdFMALong128.
func ubdFMASlow128(x, y, z UBD128, mode Mode) UBD128 {
	if x.Hi>>60 == UBDSpecialTag || y.Hi>>60 == UBDSpecialTag || z.Hi>>60 == UBDSpecialTag || mode > ToNearestAway {
		return ubdFMALong128(x, y, z, mode)
	}
	xhi, yhi, zhi := x.Hi&UBDCoeffMaskHi, y.Hi&UBDCoeffMaskHi, z.Hi&UBDCoeffMaskHi
	xe, ye, ze := int(int64(x.Hi)>>UBDExpShift), int(int64(y.Hi)>>UBDExpShift), int(int64(z.Hi)>>UBDExpShift)

	h00, p0 := bits.Mul64(x.Lo, y.Lo)
	h01, l01 := bits.Mul64(x.Lo, yhi)
	h10, l10 := bits.Mul64(xhi, y.Lo)
	h11, l11 := bits.Mul64(xhi, yhi)
	p1, c1 := bits.Add64(h00, l01, 0)
	p1, c2 := bits.Add64(p1, l10, 0)
	p2, c1 := bits.Add64(h01, h10, c1)
	p2, c2 = bits.Add64(p2, l11, c2)
	p3 := h11 + c1 + c2

	ep := xe + ye
	if p3|p2 == 0 && p1>>48 == 0 && uint(ep-MinExp128) <= MaxExp128-MinExp128 {
		// The product is exact and below 2^112 < 10^34: a value of the
		// format, with the sign of x × y even when it is zero.
		p := UBD128{Lo: p0, Hi: uint64(int64(ep))<<UBDExpShift | (x.Hi^y.Hi)&UBDSignHi | p1}
		if mode == ToNearestEven {
			return ubdAddTTE128(p, z)
		}
		return ubdAddLong128(p, z, mode)
	}
	g := ze - ep
	if uint(g) >= uint(len(pow10tab128)) {
		if mode != ToNearestEven || z.Lo|zhi == 0 {
			return ubdFMALong128(x, y, z, mode)
		}
		return ubdFMAWide128(x, y, z, p0, p1, p2, p3, ep, ze)
	}

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

	neg := (x.Hi^y.Hi)&UBDSignHi != 0
	var c uint64
	if (x.Hi^y.Hi^z.Hi)&UBDSignHi == 0 {
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
			p0, c = bits.Sub64(0, p0, 0)
			p1, c = bits.Sub64(0, p1, c)
			p2, c = bits.Sub64(0, p2, c)
			p3 = -p3 - c
			neg = !neg
		}
	}
	if p0|p1|p2|p3 == 0 {
		return ubdFMALong128(x, y, z, mode)
	}
	exp := xe + ye
	s := Uint128{Hi: p1, Lo: p0}

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
			return ubdFMALong128(x, y, z, mode)
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
	if r, ok := ubdPackWord(neg, s.Hi, s.Lo, exp); ok {
		return r
	}
	return ubdFMALong128(x, y, z, mode)
}

// ubdFMAWide128 is the rest of decimal128-go's d128_fma_tte, for
// ubdFMASlow128's finite operands whose exponents are more than 38 apart
// with z the higher, or with z below the product: the exact product
// p of four words at exponent ep, a non-zero z at ez, in ties to even.
// m is the operand with the larger exponent and n the other, G places
// apart.
//
//   - Exact alignment, digits(m) + G <= 76: m × 10^G ± n is exact in four
//     words at n's exponent, the preferred one, and is rounded once to 34
//     digits: half of 10^j is added and one reciprocal division by 10^j
//     (u256DivRemPow10) gives the quotient; a zero remainder is a tie,
//     made even. An exact cancellation is +0.
//   - z far above the product, past exact alignment: z is scaled to 34
//     digits and p / 10^d, rounded half up the same way, is added or
//     subtracted; a sum of 35 digits is rounded once more from the
//     truncated sum and whether anything below it is non-zero.
//
// A product far above z, and a result that leaves the format or loses a
// digit, go to ubdFMALong128.
func ubdFMAWide128(x, y, z UBD128, p0, p1, p2, p3 uint64, ep, ez int) UBD128 {
	al, ah := z.Lo, z.Hi&UBDCoeffMaskHi
	pSign := (x.Hi ^ y.Hi) & UBDSignHi
	aSign := z.Hi & UBDSignHi
	dp := digits256(p0, p1, p2, p3)
	da := digits128(Uint128{Hi: ah, Lo: al})

	var s0, s1, s2, s3 uint64 // the exact sum, or z scaled to 34 digits
	var exp int
	var sign uint64
	var c uint64
	if ep >= ez && dp+ep-ez <= 76 || ez > ep && da+ez-ep <= 76 {
		// Exact alignment: m × 10^G at n's exponent, then ± n.
		var m0, m1, m2, m3, n0, n1, n2, n3 uint64
		var g int
		if ep >= ez {
			m0, m1, m2, m3, n0, n1, sign, g, exp = p0, p1, p2, p3, al, ah, pSign, ep-ez, ez
		} else {
			m0, m1, n0, n1, n2, n3, sign, g, exp = al, ah, p0, p1, p2, p3, aSign, ez-ep, ep
		}
		nSign := pSign ^ aSign ^ sign
		if g > 0 {
			if g < len(pow10tab128) {
				t := &pow10tab128[g]
				m3, m2, m1, m0 = mul256x128(m0, m1, m2, m3, t.Lo, t.Hi)
			} else {
				t := &pow10tab256[g] // m is below 10^38 here, two words
				m3, m2, m1, m0 = mul256x128(t.Lo.Lo, t.Lo.Hi, t.Hi.Lo, t.Hi.Hi, m0, m1)
			}
		}
		if pSign == aSign {
			s0, c = bits.Add64(m0, n0, 0)
			s1, c = bits.Add64(m1, n1, c)
			s2, c = bits.Add64(m2, n2, c)
			s3 = m3 + n3 + c
		} else {
			s0, c = bits.Sub64(m0, n0, 0)
			s1, c = bits.Sub64(m1, n1, c)
			s2, c = bits.Sub64(m2, n2, c)
			s3, c = bits.Sub64(m3, n3, c)
			if c != 0 {
				// n was the larger: its sign, and the difference negated.
				s0, c = bits.Sub64(0, s0, 0)
				s1, c = bits.Sub64(0, s1, c)
				s2, c = bits.Sub64(0, s2, c)
				s3, _ = bits.Sub64(0, s3, c)
				sign = nSign
			}
			if s0|s1|s2|s3 == 0 && exp >= MinExp128 && exp <= MaxExp128 {
				return UBD128{Hi: uint64(int64(exp)) << UBDExpShift} // an exact cancellation: +0
			}
		}
		// Rounded once to 34 digits.
		if j := digits256(s0, s1, s2, s3) - Precision128; j > 0 {
			if exp+j < MinExp128 {
				// The subnormal range removes more digits than the precision does.
				return ubdFMALong128(x, y, z, ToNearestEven)
			}
			h := &pow10tab256[j]
			h0, h1, h2, h3 := h.Lo.Lo>>1|h.Lo.Hi<<63, h.Lo.Hi>>1|h.Hi.Lo<<63, h.Hi.Lo>>1|h.Hi.Hi<<63, h.Hi.Hi>>1
			s0, c = bits.Add64(s0, h0, 0)
			s1, c = bits.Add64(s1, h1, c)
			s2, c = bits.Add64(s2, h2, c)
			s3 += h3 + c
			var r0, r1, r2, r3 uint64
			s0, s1, r0, r1, r2, r3 = u256DivRemPow10(s0, s1, s2, s3, j)
			s2, s3 = 0, 0 // the quotient is at most 10^34
			if r0|r1|r2|r3 == 0 {
				s0 &^= 1 // a tie: the even neighbour
			}
			exp += j
			if t := &pow10tab128[Precision128]; s1 == t.Hi && s0 == t.Lo {
				t = &pow10tab128[Precision128-1]
				s0, s1, exp = t.Lo, t.Hi, exp+1
			}
		}
	} else if ez > ep {
		// z far above the product: z scaled to 34 digits, p / 10^d rounded
		// half up beside it (d > 42 here, so the quotient is small; past
		// 76, p is below half a unit and only says the result is inexact).
		m := Precision128 - da
		t := &pow10tab128[m]
		s1, s0 = bits.Mul64(al, t.Lo)
		s1 += al*t.Hi + ah*t.Lo
		exp = ez - m
		d := exp - ep
		var q0, q1 uint64
		rbBelowHalf, rbHalf := false, false
		rbZero := false
		if d <= 76 {
			h := &pow10tab256[d]
			h0, h1, h2, h3 := h.Lo.Lo>>1|h.Lo.Hi<<63, h.Lo.Hi>>1|h.Hi.Lo<<63, h.Hi.Lo>>1|h.Hi.Hi<<63, h.Hi.Hi>>1
			t0, c := bits.Add64(p0, h0, 0)
			t1, c := bits.Add64(p1, h1, c)
			t2, c := bits.Add64(p2, h2, c)
			t3 := p3 + h3 + c
			var r0, r1, r2, r3 uint64
			q0, q1, r0, r1, r2, r3 = u256DivRemPow10(t0, t1, t2, t3, d)
			rbZero = r0|r1|r2|r3 == 0
			_, lt := bits.Sub64(r0, h0, 0)
			_, lt = bits.Sub64(r1, h1, lt)
			_, lt = bits.Sub64(r2, h2, lt)
			_, lt = bits.Sub64(r3, h3, lt)
			rbBelowHalf = lt != 0
			rbHalf = r0 == h0 && r1 == h1 && r2 == h2 && r3 == h3
		}
		sign = aSign // z scaled is the larger magnitude
		t34 := &pow10tab128[Precision128]
		if pSign == aSign {
			s0, c = bits.Add64(s0, q0, 0)
			s1 += q1 + c
			_, bw := bits.Sub64(s0, t34.Lo, 0)
			if _, bw = bits.Sub64(s1, t34.Hi, bw); bw == 0 {
				// 35 digits: rounded once more, at exp + 1, from the
				// truncated sum and whether anything below it is non-zero.
				if d <= 76 && rbBelowHalf {
					s0, c = bits.Sub64(s0, 1, 0)
					s1 -= c
				}
				sticky := d > 76 || !rbHalf
				var u uint64
				s0, s1, u, _, _, _ = u256DivRemPow10(s0, s1, 0, 0, 1)
				if u > 5 || u == 5 && (sticky || s0&1 != 0) {
					s0, c = bits.Add64(s0, 1, 0)
					s1 += c
				}
				exp++
			} else if rbZero {
				s0 &^= 1 // a tie: the even neighbour
			}
		} else {
			s0, c = bits.Sub64(s0, q0, 0)
			s1 -= q1 + c
			if rbZero {
				s0, c = bits.Add64(s0, s0&1, 0) // halfway: the even one
				s1 += c
			}
			t33 := &pow10tab128[Precision128-1]
			_, bw := bits.Sub64(t33.Lo, s0, 0)
			if _, bw = bits.Sub64(t33.Hi, s1, bw); bw == 0 {
				// At most 10^33: a digit was lost to the scale.
				return ubdFMALong128(x, y, z, ToNearestEven)
			}
		}
	} else {
		return ubdFMALong128(x, y, z, ToNearestEven)
	}
	if r, ok := ubdPackWord(sign != 0, s1, s0, exp); ok && s3|s2 == 0 {
		return r
	}
	return ubdFMALong128(x, y, z, ToNearestEven)
}

// ubdFMALong128 is fmaLong128 on UBD: the word paths for three one-word
// coefficients, the specials, and the exact sum of the product and z
// rounded once.
func ubdFMALong128(x, y, z UBD128, mode Mode) UBD128 {
	var yneg, zneg, yok, zok bool
	var yw, zw uint64
	var ye, ze int
	xneg, xw, xe, xok := ubdUnpackWord(x)
	if xok {
		yneg, yw, ye, yok = ubdUnpackWord(y)
	}
	if yok {
		zneg, zw, ze, zok = ubdUnpackWord(z)
	}
	var a wide128
	var aneg, bneg bool
	var ae, be int
	var zc coef128
	if zok {
		phi, plo := bits.Mul64(xw, yw)
		pneg, pe := xneg != yneg, xe+ye
		if phi == 0 {
			hneg, hw, he, lneg, lw, le := pneg, plo, pe, zneg, zw, ze
			if he < le {
				hneg, hw, he, lneg, lw, le = lneg, lw, le, hneg, hw, he
			}
			if gap := uint(he - le); gap <= maxWordGap {
				hi, lo := bits.Mul64(hw, pow10tab64[gap])
				neg := hneg
				if hneg == lneg {
					var carry uint64
					lo, carry = bits.Add64(lo, lw, 0)
					hi += carry
				} else if hi == 0 && lo < lw {
					neg, lo = lneg, lw-lo
				} else {
					var borrow uint64
					lo, borrow = bits.Sub64(lo, lw, 0)
					hi -= borrow
					if hi|lo == 0 {
						neg = mode == TowardNegative
					}
				}
				if r, ok := ubdPackWord(neg, hi, lo, le); ok {
					return r
				}
			}
		}
		if p, ok := ubdPackWord(pneg, phi, plo, pe); ok {
			return ubdAddLong128(p, z, mode)
		}
		a, aneg, ae = cFromU128128(Uint128{Hi: phi, Lo: plo}), pneg, pe
		zc, bneg, be = cFromU64128(zw), zneg, ze
	} else {
		xk, xneg, xc, xe := UBDUnpack128(x)
		yk, yneg, yc, ye := UBDUnpack128(y)
		zk, zneg, zc2, ze := UBDUnpack128(z)
		pneg := xneg != yneg
		if xk != KindFinite || yk != KindFinite || zk != KindFinite {
			switch {
			case xk == KindNaN || yk == KindNaN || zk == KindNaN:
				return ubdNaN3(x, y, z)
			case xk == KindFinite && yk == KindFinite:
				return ubdInf(zneg) // a finite product and an infinite z
			case xk == KindFinite && cIsZero128(xc) || yk == KindFinite && cIsZero128(yc):
				return ubdQNaN // 0 × ∞, of any exponent
			case zk == KindInf && zneg != pneg:
				return ubdQNaN // the infinities cancel
			}
			return ubdInf(pneg)
		}
		a, aneg, ae = cMulFull128(xc, yc), pneg, xe+ye
		zc, bneg, be = zc2, zneg, ze
	}
	b := wide128{Lo: zc}
	if ae < be {
		a, aneg, ae, b, bneg, be = b, bneg, be, a, aneg, ae
	}
	exp, class := be, classZero
	if gap := ae - be; gap > 0 && a != (wide128{}) {
		k := min(gap, fmaDigits128-cLenFull128(a))
		a, _ = cScaleFull128(a, k)
		if k < gap {
			exp = ae - k
			b, class = cShiftFull128(b, gap-k)
		}
	}

	neg := aneg
	var sum wide128
	if aneg == bneg {
		sum, _ = cAddFull128(a, b)
	} else {
		if class != classZero {
			b, class = cIncFull128(b), class.complement()
		}
		var borrow bool
		sum, borrow = cSubFull128(a, b)
		switch {
		case borrow:
			sum, _ = cSubFull128(wide128{}, sum)
			neg = !aneg
		case sum == (wide128{}):
			neg = mode == TowardNegative // an exact cancellation
		}
	}
	r, _ := ubdRoundFloor128(neg, sum, exp, class, mode, MinExp128)
	return r
}

// UBDRoundToIntegral128 is RoundToIntegral128 on UBD: the kernel entry
// ubdRoundToIntegralRnd128, for internal/decimal's integral rounding where
// the compiler does not lower it.
func UBDRoundToIntegral128(x UBD128, mode Mode) UBD128 { return ubdRoundToIntegralRnd128(x, mode) }
