// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// This package's add on UBD operands. ubdAddSlow128,
// ubdAddLong128 and the rounding they end in (ubdRound128, ubdRoundFace128,
// ubdOverflow128, ubdPack128) are addSlow128, addLong128, round128,
// roundFace128, overflow128 and pack128 with the BID decoding and packing
// replaced by UBD's; the arithmetic is unchanged. Nothing converts.

// ubdSubSlow128 is subSlow128: y negated unless it is a NaN, then added.
func ubdSubSlow128(x, y UBD128, mode Mode) UBD128 {
	if y.Hi>>59 != UBDNaNTag {
		y.Hi ^= UBDSignHi
	}
	return ubdAddSlow128(x, y, mode)
}

// ubdAddLong128 is addLong128 on UBD operands: the operands are decoded on
// UBD (exponents unbiased, so the word path's subtractions of Bias128 go),
// specials are decided on UBD, an exact result that fits packs into UBD, and
// face-value rounding is ubdRoundFace128. The arithmetic is addLong128's,
// unchanged.
func ubdAddLong128(x, y UBD128, mode Mode) UBD128 {
	var yneg, yok bool
	var yw uint64
	var ye int
	xneg, xw, xe, xok := ubdUnpackWord(x)
	if xok {
		yneg, yw, ye, yok = ubdUnpackWord(y)
	}
	var xc, yc coef128
	if yok {
		if xe < ye {
			xneg, xw, xe, yneg, yw, ye = yneg, yw, ye, xneg, xw, xe
		}
		if gap := uint(xe - ye); gap <= maxWordGap {
			hi, lo := bits.Mul64(xw, pow10tab64[gap])
			neg := xneg
			if xneg == yneg {
				var carry uint64
				lo, carry = bits.Add64(lo, yw, 0)
				hi += carry
			} else if hi == 0 && lo < yw {
				neg, lo = yneg, yw-lo
			} else {
				var borrow uint64
				lo, borrow = bits.Sub64(lo, yw, 0)
				hi -= borrow
				if hi|lo == 0 {
					neg = mode == TowardNegative
				}
			}
			if r, ok := ubdPackWord(neg, hi, lo, ye); ok {
				return r
			}
		} else if xw != 0 {
			// A y so far below x that it only says which way the sum lies
			// from x scaled to its Precision128 digits (see addLong128).
			if m := Precision128 - wordDigits(xw); gap >= uint(m+farWordGap) {
				p := cMulPow10128(cFromU64128(xw), m)
				if mode <= ToNearestAway || yw == 0 {
					hi, lo := cWords128(p)
					if r, ok := ubdPackWord(xneg, hi, lo, xe-m); ok {
						return r
					}
				} else if xneg == yneg {
					return ubdRoundFace128(xneg, p, xe-m, classBelowHalf, mode)
				} else {
					if cEq128(p, cPow10128(Precision128-1)) {
						p, m = cPow10128(Precision128), m+1
					}
					p, _ = cSub128(p, cPow10128(0))
					return ubdRoundFace128(xneg, p, xe-m, classAboveHalf, mode)
				}
			}
		}
		xc, yc = cFromU64128(xw), cFromU64128(yw)
	} else {
		xs, ys := x.Hi>>60 == UBDSpecialTag, y.Hi>>60 == UBDSpecialTag
		xneg, yneg = x.Hi&UBDSignHi != 0, y.Hi&UBDSignHi != 0
		if xs || ys {
			switch {
			case x.Hi>>59 == UBDNaNTag || y.Hi>>59 == UBDNaNTag:
				return ubdNaN2(x, y)
			case !xs:
				return ubdInf(yneg)
			case ys && xneg != yneg:
				return UBD128{Hi: UBDNaNHi} // the infinities cancel
			}
			return ubdInf(xneg)
		}
		xc, xe = coef128{Hi: x.Hi & UBDCoeffMaskHi, Lo: x.Lo}, int(int64(x.Hi)>>UBDExpShift)
		yc, ye = coef128{Hi: y.Hi & UBDCoeffMaskHi, Lo: y.Lo}, int(int64(y.Hi)>>UBDExpShift)
		if xe < ye {
			xneg, xc, xe, yneg, yc, ye = yneg, yc, ye, xneg, xc, xe
		}
	}

	exp, class := ye, classZero
	if gap := xe - ye; gap > 0 && !cIsZero128(xc) {
		// A swamped y (see addLong128).
		dx := cDigits128(xc)
		m := Precision128 - dx
		if s := gap - m - 1; s > 0 && !cIsZero128(yc) && (s >= Precision128 || cLt128(yc, cPow10128(s))) {
			p, class := cMulPow10128(xc, m), classBelowHalf
			near := p
			if xneg != yneg {
				class = classAboveHalf
				if cEq128(p, cPow10128(Precision128-1)) {
					m, s = m+1, s-1
					p = cPow10128(Precision128)
					near = p
				}
				p, _ = cSub128(p, cPow10128(0))
			}
			if s > 0 && (s >= Precision128 || cLt128(yc, cPow10128(s))) {
				if mode <= ToNearestAway || mode > TowardPositive {
					return ubdRoundFace128(xneg, near, xe-m, classZero, mode)
				}
				return ubdRoundFace128(xneg, p, xe-m, class, mode)
			}
		}
		// An aligned y (see addLong128).
		if m := Precision128 - dx; gap > m {
			p := cMulPow10128(xc, m)
			q, c := cShift128(yc, gap-m)
			if xneg == yneg {
				p, _ = cAdd128(p, q)
				if c != classZero && !cGtMax128(p) {
					return ubdRoundFace128(xneg, p, xe-m, c, mode)
				}
				return ubdRound128(xneg, wide128{Lo: p}, xe-m, c, mode)
			}
			if c != classZero {
				q, c = cInc128(q), c.complement()
			}
			if p, _ = cSub128(p, q); !cLt128(p, cPow10128(Precision128-1)) {
				if c != classZero {
					return ubdRoundFace128(xneg, p, xe-m, c, mode)
				}
				return ubdRound128(xneg, wide128{Lo: p}, xe-m, c, mode)
			}
		}
		k := min(gap, addDigits128-dx)
		xc = cMulPow10128(xc, k)
		if k < gap {
			exp = xe - k
			yc, class = cShift128(yc, gap-k)
		}
	}

	neg := xneg
	var sum coef128
	if xneg == yneg {
		sum, _ = cAdd128(xc, yc)
	} else {
		if class != classZero {
			yc, class = cInc128(yc), class.complement()
		}
		var borrow bool
		sum, borrow = cSub128(xc, yc)
		switch {
		case borrow:
			var zero coef128
			sum, _ = cSub128(zero, sum)
			neg = !xneg
		case cIsZero128(sum):
			neg = mode == TowardNegative // an exact cancellation
		}
	}
	return ubdRound128(neg, wide128{Lo: sum}, exp, class, mode)
}

// ubdUnpackWord is unpackWord128 on UBD: the sign, the coefficient and the
// (unbiased) exponent of a finite x whose coefficient is one word. A special
// has the 0x1F prefix in its coefficient's high word, so it is never one.
func ubdUnpackWord(x UBD128) (neg bool, coeff uint64, exp int, ok bool) {
	if x.Hi&UBDCoeffMaskHi != 0 {
		return
	}
	return x.Hi&UBDSignHi != 0, x.Lo, int(int64(x.Hi) >> UBDExpShift), true
}

// ubdRoundFace128 is roundFace128 on UBD: the increment of the class at
// face value, the carry out of the largest coefficient, and the result
// packed into UBD.
func ubdRoundFace128(neg bool, q coef128, exp int, class digitClass, mode Mode) UBD128 {
	q = cAddU64128(q, roundInc(class, mode, neg, cOdd128(q)))
	if cGtMax128(q) {
		q, exp = cPow10128(Precision128-1), exp+1
		if exp > MaxExp128 {
			return ubdOverflow128(neg, mode)
		}
	}
	return ubdPack128(neg, q, exp)
}

// ubdPack128 is pack128 on UBD: the encoding of a coefficient and exponent
// that the rounding has put inside the format, and a panic otherwise, as
// pack128's.
func ubdPack128(neg bool, c coef128, exp int) UBD128 {
	if r, ok := ubdPackWord(neg, c.Hi, c.Lo, exp); ok {
		return r
	}
	panic(errRoundRange)
}

// ubdOverflow128 is overflow128 on UBD: an infinity, or the largest finite
// value in a mode that rounds the overflow toward zero.
func ubdOverflow128(neg bool, mode Mode) UBD128 {
	if overflowsToInf(mode, neg) {
		return ubdInf(neg)
	}
	return ubdPack128(neg, cMax128(), MaxExp128)
}

// ubdRoundFinite128 is roundFinite128 on UBD.
func ubdRoundFinite128(neg bool, c coef128, exp int, mode Mode, floor int) (UBD128, digitClass) {
	class := classZero
	if exp < floor {
		shift := floor - max(exp, floor-maxWideDigits)
		c, class = cShift128(c, shift)
		inc := roundInc(class, mode, neg, cOdd128(c))
		if class == classZero {
			inc = 0
		}
		c = cAddU64128(c, inc)
		exp = floor
	}
	return ubdPack128(neg, c, exp), class
}

// ubdRound128 is round128 with floor MinExp128, on UBD: the same rounding,
// the result packed by ubdPack128 and an overflow by ubdOverflow128. It
// returns no status: no kernel entry reports one, so tiny, which only
// sets Underflow, is not computed.
func ubdRound128(neg bool, p wide128, exp int, class digitClass, mode Mode) UBD128 {
	const floor = MinExp128
	if class == classZero {
		var zero coef128
		if p.Hi == zero {
			if cIsZero128(p.Lo) {
				return ubdPack128(neg, zero, min(max(exp, floor), MaxExp128))
			}
			if exp <= MaxExp128 && !cGtMax128(p.Lo) {
				if exp >= floor {
					return ubdPack128(neg, p.Lo, exp)
				}
				r, _ := ubdRoundFinite128(neg, p.Lo, exp, mode, floor)
				return r
			}
		}
	}
	exp = min(max(exp, MinExp128-maxWideDigits-2), MaxExp128+Precision128+2)
	var d int
	narrow := cIsZero128(p.Hi)
	if narrow {
		d = cDigits128(p.Lo)
	} else {
		d = cLenFull128(p)
	}
	a := d - 1 + exp
	x := max(a-(Precision128-1), floor)
	s := exp - x
	var q coef128
	if s < 0 {
		var dropped digitClass
		if narrow {
			q, dropped = cShift128(p.Lo, -s)
		} else {
			q, dropped = cReduceClass128(p, -s)
		}
		class = dropped.withSticky(class != classZero)
	} else {
		q = cLo128(p)
	}
	if class == classZero {
		if s < 0 {
			exp = x
		}
		if exp > MaxExp128 {
			padded, overflow := cScale128(q, exp-MaxExp128)
			if overflow || cGtMax128(padded) {
				return ubdOverflow128(neg, mode)
			}
			q, exp = padded, MaxExp128
		}
		return ubdPack128(neg, q, exp)
	}
	if s > 0 {
		q = cMulPow10128(q, s)
	}
	q = cAddU64128(q, roundInc(class, mode, neg, cOdd128(q)))
	if cGtMax128(q) {
		q, x = cPow10128(Precision128-1), x+1
	}
	if x > MaxExp128 {
		return ubdOverflow128(neg, mode)
	}
	return ubdPack128(neg, q, x)
}

// ubdNaN2 is nan128(x, y, y) on UBD: a signaling NaN first, else x's NaN,
// else y's, quieted. In UBD a NaN is quieted by clearing qExp's signaling
// bit, bit 58.
func ubdNaN2(x, y UBD128) UBD128 {
	sig := func(v UBD128) bool { return v.Hi>>58 == UBDSNaNTag }
	quiet := func(v UBD128) UBD128 { return UBD128{Lo: v.Lo, Hi: v.Hi &^ (1 << 58)} }
	if sig(x) || !sig(y) && x.Hi>>59 == UBDNaNTag {
		return quiet(x)
	}
	return quiet(y)
}

// ubdInf is the infinity of the sign neg.
func ubdInf(neg bool) UBD128 {
	if neg {
		return UBD128{Hi: UBDInfHi | UBDSignHi}
	}
	return UBD128{Hi: UBDInfHi}
}

// ubdPackWord packs a finite result, as packWord128 does for BID: false when
// the coefficient passes MaxCoeff128 or the exponent leaves the format.
func ubdPackWord(neg bool, hi, lo uint64, exp int) (UBD128, bool) {
	if hi > MaxCoeff128Hi || hi == MaxCoeff128Hi && lo > MaxCoeff128Lo || exp < MinExp128 || exp > MaxExp128 {
		return UBD128{}, false
	}
	h := uint64(int64(exp))<<UBDExpShift | hi
	if neg {
		h |= UBDSignHi
	}
	return UBD128{Lo: lo, Hi: h}, true
}

// ubdAddSlow128 is addSlow128 on UBD: the operands' signs are bit 49 and
// their exponents unbiased, and the result packs into UBD. A special has
// the 0x1F prefix in its coefficient's high word, so it is never a word
// pair; addPair128's test for one becomes the tag test. Where addSlow128
// calls addLong128 it calls ubdAddLong128.
func ubdAddSlow128(x, y UBD128, mode Mode) UBD128 {
	// Coefficients of one word each go on at once, before anything is
	// decoded, as in addSlow128: ubdAddLong128's word path decides them.
	if (x.Hi|y.Hi)&UBDCoeffMaskHi == 0 {
		return ubdAddLong128(x, y, mode)
	}
	if x.Hi>>60 == UBDSpecialTag || y.Hi>>60 == UBDSpecialTag || mode > ToNearestAway {
		return ubdAddLong128(x, y, mode)
	}
	xhi, yhi := x.Hi&UBDCoeffMaskHi, y.Hi&UBDCoeffMaskHi
	xe, ye := int(int64(x.Hi)>>UBDExpShift), int(int64(y.Hi)>>UBDExpShift)
	// a is the operand with the larger exponent, x if they are equal.
	sw := uint64(int64(xe-ye) >> 63) // all ones when y has the larger
	th, tl := (xhi^yhi)&sw, (x.Lo^y.Lo)&sw
	a := Uint128{Hi: xhi ^ th, Lo: x.Lo ^ tl}
	b := Uint128{Hi: yhi ^ th, Lo: y.Lo ^ tl}
	gap := int((uint64(xe-ye) ^ sw) - sw)
	exp := ye + int(uint64(xe-ye)&sw) // the smaller exponent
	sub := (x.Hi ^ y.Hi) >> 49 & 1    // 1 when the signs differ
	neg := (x.Hi^(x.Hi^y.Hi)&sw)>>49&1 != 0
	da := cDigits128(a)
	if uint(da-1) >= Precision128 || u128GtMaxCoeff(b) {
		return ubdAddLong128(x, y, mode)
	}
	m := Precision128 - da
	d := gap - m
	var s Uint128
	if d <= 0 {
		p := u128MulLow(a, pow10tab128[gap])
		if sub == 0 {
			s, _ = u128Add(p, b)
			if u128GtMaxCoeff(s) {
				s, exp = addCarry128(s, false, mode), exp+1
			}
		} else {
			var borrow uint64
			s, borrow = u128Sub(p, b)
			if borrow != 0 {
				s, _ = u128Sub(Uint128{}, s)
				neg = !neg
			}
			if s == (Uint128{}) {
				neg = false
			}
		}
	} else {
		p := u128MulLow(a, pow10tab128[m])
		exp += d
		var q, rb, half Uint128
		rb.Lo = 1
		if d <= Precision128 {
			pd := &pow10tab128[d]
			half = u128Shr(*pd, 1)
			t, _ := u128Add(b, half)
			rm := &recip10tab128[d].t
			h00, _ := bits.Mul64(t.Lo, rm.Lo)
			h01, l01 := bits.Mul64(t.Lo, rm.Hi)
			h10, l10 := bits.Mul64(t.Hi, rm.Lo)
			h11, l11 := bits.Mul64(t.Hi, rm.Hi)
			mid, c1 := bits.Add64(h00, l01, 0)
			_, c2 := bits.Add64(mid, l10, 0)
			q.Lo, c1 = bits.Add64(l11, h01, c1)
			q.Hi = h11 + c1
			q.Lo, c2 = bits.Add64(q.Lo, h10, c2)
			q.Hi += c2
			ph, pl := bits.Mul64(q.Lo, pd.Lo)
			ph += q.Lo*pd.Hi + q.Hi*pd.Lo
			var bw uint64
			rb.Lo, bw = bits.Sub64(t.Lo, pl, 0)
			rb.Hi, _ = bits.Sub64(t.Hi, ph, bw)
			if !u128Lt(rb, *pd) {
				q, _ = u128Add(q, Uint128{Lo: 1})
				rb, _ = u128Sub(rb, *pd)
			}
		}
		tie := uint64(0)
		if rb == (Uint128{}) {
			tie = 1
		}
		if sub == 0 {
			s, _ = u128Add(p, q)
			if u128GtMaxCoeff(s) {
				t := s
				if d <= Precision128 && u128Lt(rb, half) {
					t, _ = u128Sub(s, Uint128{Lo: 1})
				}
				s, exp = addCarry128(t, d > Precision128 || rb != half, mode), exp+1
			} else if mode == ToNearestEven {
				s.Lo &^= tie
			}
		} else {
			s, _ = u128Sub(p, q)
			up := tie
			if mode == ToNearestEven {
				up &= s.Lo
			}
			s, _ = u128Add(s, Uint128{Lo: up})
			if !u128Lt(pow10tab128[Precision128-1], s) {
				const spare = len(pow10tab128) - 1 - Precision128
				if d > spare {
					return ubdAddLong128(x, y, mode)
				}
				s, _ = u128Sub(u128MulLow(a, pow10tab128[gap]), b)
				exp -= d
				if j := cDigits128(s) - Precision128; j > 0 {
					t, _ := u128Add(s, u128Shr(pow10tab128[j], 1))
					var r Uint128
					s, r = divPow10Recip128(t, j)
					if r == (Uint128{}) && mode == ToNearestEven {
						s.Lo &^= 1
					}
					exp += j
				}
			}
		}
	}
	if r, ok := ubdPackWord(neg, s.Hi, s.Lo, exp); ok {
		return r
	}
	return ubdAddLong128(x, y, mode)
}
