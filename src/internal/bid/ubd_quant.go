// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// This package's quantize on UBD operands. ubdQuantize128 is
// quantize128 with the word path ubdQuantizeWord (ubd.go) after its first
// test; ubdQuantizeSame128, ubdQuantizeSlow128 and ubdQuantizeLong128 are
// quantizeSame128, quantizeSlow128 and quantizeLong128 with the BID decoding
// (addPair128, unpackWordCanon128, kindExp128, unpackFirst128,
// unpackRest128) replaced by UBD's, the specials decided on UBD, and the
// packing and rounding by ubdPackWord and ubdRoundFinite128; the arithmetic
// is unchanged. Nothing converts.

// ubdQuantize128 is quantize128 on UBD, written out in the entries
// (ubdQuantizeTTE128, ubdQuantizeRnd128) so that a quantize is one call:
// ubdQuantizeSame128 inline; an x wider than a word, or special, straight to
// ubdQuantizeSlow128 as quantize128 sends it; a word x to ubdQuantizeWord,
// then ubdQuantizeSlow128.
func ubdQuantize128(x, y UBD128, mode Mode) UBD128 {
	if ubdQuantizeSame128(x, y) {
		return x
	}
	if x.Hi&UBDCoeffMaskHi == 0 {
		if r, ok := ubdQuantizeWord(x, y, mode); ok {
			return r
		}
	}
	return ubdQuantizeSlow128(x, y, mode)
}

// ubdQuantizeSame128 is quantizeSame128 on UBD: x already at y's exponent,
// with a coefficient of one word, so that x is the result. A special has
// the 0x1F prefix in its coefficient's high word, so the one-word test
// excludes it; the sign is not compared, as quantize keeps x's.
func ubdQuantizeSame128(x, y UBD128) bool {
	return (x.Hi^y.Hi)>>UBDExpShift|x.Hi&UBDCoeffMaskHi == 0
}

// ubdQuantizeSlow128 is quantizeSlow128 on UBD.
func ubdQuantizeSlow128(x, y UBD128, mode Mode) UBD128 {
	xhi := x.Hi & UBDCoeffMaskHi
	xe, ye := int(int64(x.Hi)>>UBDExpShift), int(int64(y.Hi)>>UBDExpShift)
	ok := x.Hi>>60 != UBDSpecialTag && y.Hi>>60 != UBDSpecialTag
	xc := Uint128{Hi: xhi, Lo: x.Lo}
	// A word has its own paths.
	if !ok || xhi == 0 || u128GtMaxCoeff(xc) {
		return ubdQuantizeLong128(x, y, mode)
	}
	neg := x.Hi&UBDSignHi != 0
	switch gap := xe - ye; {
	case gap == 0:
		return x
	case gap > 0:
		if gap >= Precision128 || !u128Lt(xc, pow10tab128[Precision128-gap]) {
			return UBD128{Hi: UBDNaNHi}
		}
		xc = u128MulLow(xc, pow10tab128[gap])
	case mode > ToNearestAway:
		return ubdQuantizeLong128(x, y, mode)
	case gap < -Precision128:
		xc = Uint128{}
	default:
		d := -gap
		t, _ := u128Add(xc, u128Shr(pow10tab128[d], 1))
		q, r := divPow10Recip128(t, d)
		if r == (Uint128{}) && mode == ToNearestEven {
			q.Lo &^= 1 // a tie: the even neighbour is the one below
		}
		xc = q
	}
	if r, ok := ubdPackWord(neg, xc.Hi, xc.Lo, ye); ok {
		return r
	}
	return ubdQuantizeLong128(x, y, mode)
}

// ubdQuantizeLong128 is quantizeLong128 on UBD: a word x quantized to a
// finite y's exponent at or below its own is scaled and packed at once;
// specials are decided on UBD (a NaN gives that NaN, two infinities the
// infinity of x's sign, one infinity the canonical NaN); every other x is
// rounded to y's exponent by ubdRoundFinite128.
func ubdQuantizeLong128(x, y UBD128, mode Mode) UBD128 {
	xneg, xw, xe, xok := ubdUnpackWord(x)
	ys := y.Hi>>60 == UBDSpecialTag
	ye := int(int64(y.Hi) >> UBDExpShift)
	var xc coef128
	if xok {
		if gap := xe - ye; gap >= 0 && gap < len(pow10tab128) && !ys {
			if uint(gap) <= maxWordGap {
				hi, lo := bits.Mul64(xw, pow10tab64[gap])
				if r, ok := ubdPackWord(xneg, hi, lo, ye); ok {
					return r
				}
			} else if s, overflow := cScaleTab128(cFromU64128(xw), gap); !overflow {
				hi, lo := cWords128(s)
				if r, ok := ubdPackWord(xneg, hi, lo, ye); ok {
					return r
				}
			}
			return UBD128{Hi: UBDNaNHi}
		}
		xc = cFromU64128(xw)
	} else {
		xneg = x.Hi&UBDSignHi != 0
		xc, xe = coef128{Hi: x.Hi & UBDCoeffMaskHi, Lo: x.Lo}, int(int64(x.Hi)>>UBDExpShift)
	}
	if xs := x.Hi>>60 == UBDSpecialTag; xs || ys {
		switch {
		case x.Hi>>59 == UBDNaNTag || y.Hi>>59 == UBDNaNTag:
			return ubdNaN2(x, y)
		case xs && ys:
			return ubdInf(xneg)
		}
		return UBD128{Hi: UBDNaNHi}
	}
	if gap := xe - ye; gap > 0 {
		// The scaled coefficient has Precision128 digits or fewer exactly
		// when the coefficient is below 10^(Precision128-gap) (see
		// quantizeLong128).
		if gap >= Precision128 {
			if !cIsZero128(xc) {
				return UBD128{Hi: UBDNaNHi}
			}
		} else {
			if !cLt128(xc, cPow10128(Precision128-gap)) {
				return UBD128{Hi: UBDNaNHi}
			}
			xc = cMulPow10128(xc, gap)
		}
		xe = ye
	}
	r, _ := ubdRoundFinite128(xneg, xc, xe, mode, ye)
	return r
}

// ubdRoundToIntegral128 is roundToIntegral128 on UBD: a NaN quieted (in UBD,
// qExp's signaling bit 58 cleared), an infinity as it is, and a finite x
// rounded to exponent zero by ubdRoundFinite128; an x at exponent zero or
// above is an integer already and comes back as it is.
func ubdRoundToIntegral128(x UBD128, mode Mode) UBD128 {
	if x.Hi>>60 == UBDSpecialTag {
		if x.Hi>>59 == UBDNaNTag {
			return UBD128{Lo: x.Lo, Hi: x.Hi &^ (1 << 58)}
		}
		return ubdInf(x.Hi&UBDSignHi != 0)
	}
	r, _ := ubdRoundFinite128(x.Hi&UBDSignHi != 0, coef128{Hi: x.Hi & UBDCoeffMaskHi, Lo: x.Lo}, int(int64(x.Hi)>>UBDExpShift), mode, 0)
	return r
}
