// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// This package's compare on UBD operands. ubdQuietCmp128 and
// ubdQuietCmpRest128 are the template's quietCmpSlowW and quietCmpRestW
// (arith_tmpl.go) with UBD's decoding and the specials found by their
// tag.
// compareWords and cCmpFinite128, which do the comparing, take the sign,
// coefficient and exponent apart and are used as they are: they only
// subtract the exponents, so it does not matter that UBD's are unbiased.
// Nothing converts.

// ubdQuietCmp128 is quietCmpSlowW on UBD: the order of x and y, and false
// when they are unordered (a NaN among them).
func ubdQuietCmp128(x, y UBD128) (c int, ok bool) {
	if (x.Hi|y.Hi)&UBDCoeffMaskHi == 0 {
		return ubdCmpWord(x, y), true
	}
	// Two finite values that are not word operands: one call that makes none
	// decides them (see quietCmpSlowW).
	if x.Hi>>60 != UBDSpecialTag && y.Hi>>60 != UBDSpecialTag {
		return cCmpFinite128(x.Hi&UBDSignHi != 0, coef128{Hi: x.Hi & UBDCoeffMaskHi, Lo: x.Lo}, int(int64(x.Hi)>>UBDExpShift),
			y.Hi&UBDSignHi != 0, coef128{Hi: y.Hi & UBDCoeffMaskHi, Lo: y.Lo}, int(int64(y.Hi)>>UBDExpShift)), true
	}
	return ubdQuietCmpRest128(x, y)
}

// ubdQuietCmpRest128 is quietCmpRestW on UBD, for a pair with a special
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

// ubdCmpWord is compareWords on UBD: the order of two finite x and y whose
// coefficients are one word each, so that their high words hold only qExp
// and the sign. The predicate entries call it for such a pair before
// anything else; it is a leaf, where ubdQuietCmp128 has a frame and called
// compareWords, which does not inline, so that a compare of money amounts
// made three calls. The final choice between the magnitude order and the
// sign order is a select, not a branch: on operands of mixed sign the
// branch was taken whenever exactly one was negative.
func ubdCmpWord(x, y UBD128) int {
	xc, yc := x.Lo, y.Lo
	// The sign that counts: a zero is not negative.
	xs := int(x.Hi >> UBDSignShift & ((xc | -xc) >> 63) & 1)
	ys := int(y.Hi >> UBDSignShift & ((yc | -yc) >> 63) & 1)
	gap := int(int64(x.Hi)>>UBDExpShift - int64(y.Hi)>>UBDExpShift)
	m := gap >> intSign // all ones if x has the smaller exponent, and y is scaled
	a, b := xc^(xc^yc)&uint64(m), yc^(xc^yc)&uint64(m)
	g := uint((gap ^ m) - m)
	hi, lo := bits.Mul64(a, pow10tab64[min(g, maxWordGap)]) // no bounds check: g is unsigned
	// Signed and 64 bits wide: as a uint, maxWordGap - g would wrap at 32
	// bits on a 32-bit target and leave bit 63 clear.
	far := uint64(int64(maxWordGap)-int64(g)) >> 63 & ((a | -a) >> 63)
	_, lt := bits.Sub64(lo, b, 0)
	_, gt := bits.Sub64(b, lo, 0)
	h := (hi|-hi)>>63 | far
	c := int(gt|h) - int(lt&^h)
	c = (c ^ m) - m    // reversed if the operands were swapped
	c = (c ^ -xs) + xs // reversed for a negative x
	d := ys - xs       // the sign order, when the signs differ
	dm := -(d & 1)     // all ones when they do
	return c&^dm | d&dm
}
