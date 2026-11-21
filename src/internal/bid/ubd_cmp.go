// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// This package's compare on UBD operands. ubdQuietCmp128 and
// ubdQuietCmpRest128 are quietCmpSlow128 and quietCmpRest128 with the BID
// decoding (unpackWord128, unpackFirst128, unpackRest128) replaced by UBD's
// and the specials found by their tag. compareWords and cCmpFinite128, which
// do the comparing, take the sign, coefficient and exponent apart and are
// used as they are: they only subtract the exponents, so it does not matter
// that UBD's are unbiased. Nothing converts.

// ubdQuietCmp128 is quietCmpSlow128 on UBD: the order of x and y, and false
// when they are unordered (a NaN among them).
func ubdQuietCmp128(x, y UBD128) (c int, ok bool) {
	if xn, xc, xe, ok := ubdUnpackWord(x); ok {
		if yn, yc, ye, ok := ubdUnpackWord(y); ok {
			return compareWords(xn, xc, xe, yn, yc, ye), true
		}
	}
	// Two finite values that are not word operands: one call that makes none
	// decides them (see quietCmpSlow128).
	if x.Hi>>60 != UBDSpecialTag && y.Hi>>60 != UBDSpecialTag {
		return cCmpFinite128(x.Hi&UBDSignHi != 0, coef128{Hi: x.Hi & UBDCoeffMaskHi, Lo: x.Lo}, int(int64(x.Hi)>>UBDExpShift),
			y.Hi&UBDSignHi != 0, coef128{Hi: y.Hi & UBDCoeffMaskHi, Lo: y.Lo}, int(int64(y.Hi)>>UBDExpShift)), true
	}
	return ubdQuietCmpRest128(x, y)
}

// ubdQuietCmpRest128 is quietCmpRest128 on UBD, for a pair with a special
// among it: a NaN is unordered; two infinities are equal if of one sign,
// else the negative is the lesser; an infinity is beyond every finite value.
func ubdQuietCmpRest128(x, y UBD128) (c int, ok bool) {
	xs, ys := x.Hi>>60 == UBDSpecialTag, y.Hi>>60 == UBDSpecialTag
	xneg, yneg := x.Hi&UBDSignHi != 0, y.Hi&UBDSignHi != 0
	switch {
	case x.Hi>>59 == UBDNaNTag || y.Hi>>59 == UBDNaNTag:
		return 0, false
	case xs && ys:
		if xneg == yneg {
			return 0, true
		}
		if xneg {
			return -1, true
		}
		return 1, true
	case xs:
		if xneg {
			return -1, true
		}
		return 1, true
	}
	if yneg {
		return 1, true
	}
	return -1, true
}
