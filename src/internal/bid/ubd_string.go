// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// The character form of a UBD value. Each function is the BID function of
// the same name without the UBD prefix, with UBD's decoding; the digit
// layout is shared (PutShort128 and PutLong128, which take the parts of a
// value, and writeDigits), and nothing converts to BID. strconv formats
// decimal128 through these where decimal128 is stored as UBD
// (NativeUBD128).

// UBDIsShort128 is IsShort128 on UBD. Adding 4 to the exponent field and
// rotating it to the bottom leaves k, the exponent plus 4, with the
// coefficient's high word above it: k is at most 4 only for a finite value
// with a one-word coefficient and an exponent from -4 to 0. A special's
// tag is an exponent field of 7168 or more.
func UBDIsShort128(x UBD128) bool {
	k := bits.RotateLeft64(x.Hi&^UBDSignHi+uint64(-shortMinExp)<<UBDExpShift, 64-UBDExpShift)
	return k <= -shortMinExp && x.Lo < shortLimit[k]
}

// UBDIsLong128 is IsLong128 on UBD: finite with a coefficient of at least
// 10^8. A well-formed finite UBD value is canonical.
func UBDIsLong128(x UBD128) bool {
	return x.Hi>>60 != UBDSpecialTag && (x.Hi&UBDCoeffMaskHi != 0 || x.Lo >= shortCoeff)
}

// UBDUnpack128 is Unpack128 on UBD: the class, the sign, the coefficient
// and the exponent, both zero for a special.
func UBDUnpack128(x UBD128) (kind Kind, neg bool, coeff Uint128, exp int) {
	neg = x.Hi&UBDSignHi != 0
	if x.Hi>>60 == UBDSpecialTag {
		if x.Hi>>59 == UBDNaNTag {
			return KindNaN, neg, Uint128{}, 0
		}
		return KindInf, neg, Uint128{}, 0
	}
	return KindFinite, neg, Uint128{Hi: x.Hi & UBDCoeffMaskHi, Lo: x.Lo}, int(int64(x.Hi) >> UBDExpShift)
}

// UBDUnpackDigits128 is UnpackDigits128 on UBD: the coefficient's digits
// right-aligned in buf, their count (zero for a zero coefficient and for a
// special), the class, the sign and the exponent.
func UBDUnpackDigits128(buf *[Precision128]byte, x UBD128) (kind Kind, neg bool, n, exp int) {
	neg = x.Hi&UBDSignHi != 0
	if x.Hi>>60 == UBDSpecialTag {
		if x.Hi>>59 == UBDNaNTag {
			return KindNaN, neg, 0, 0
		}
		return KindInf, neg, 0, 0
	}
	exp = int(int64(x.Hi) >> UBDExpShift)
	if hi := x.Hi & UBDCoeffMaskHi; hi != 0 || x.Lo >= pow10tab64[u64Digits] {
		h, l := cSplit128(coef128{Hi: hi, Lo: x.Lo})
		return KindFinite, neg, writeDigits(buf[:], h, l), exp
	}
	if x.Lo != 0 {
		n = writeWordDigits(buf[:], x.Lo)
	}
	return KindFinite, neg, n, exp
}

// UBDQuantizeMode128 is Quantize128 on UBD with the caller's rounding: x
// rounded to the exponent of y.
func UBDQuantizeMode128(x, y UBD128, mode Mode) UBD128 { return ubdQuantizeRnd128(x, y, mode) }
