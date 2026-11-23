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

// UBDIsShortFixed128 is IsShortFixed128 on UBD: UBDIsShort128 without the
// bound on the magnitude.
func UBDIsShortFixed128(x UBD128) bool {
	k := bits.RotateLeft64(x.Hi&^UBDSignHi+uint64(-shortMinExp)<<UBDExpShift, 64-UBDExpShift)
	return k <= -shortMinExp && x.Lo < shortCoeff
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

// UBDParse128 is Parse128 on UBD: parse128, its short path written out
// here as there (see parseW for why it is not a call), the short numeral
// packed as UBD fields, since it is exact and inside the format, and every
// other numeral scanned by scanDecimal and rounded by ubdRoundStatus128.
// TestParseShort checks it against Parse128.
func UBDParse128(s string, mode Mode) (UBD128, Status) {
	if len(s) > 0 && len(s) <= 9 && s[0] <= '9' { // a sign, a point or a digit first
		i := 0
		if s[0] == '+' || s[0] == '-' {
			i = 1
		}
		if m := len(s) - i; m >= 1 && m <= 8 {
			// The bytes, first lowest, left-padded with '0' to eight: two
			// overlapping fixed-size loads (one, below four bytes).
			b := s[i:]
			var w uint64
			if m >= 4 {
				t := b[m-4:]
				w = uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24
				w |= (uint64(t[0]) | uint64(t[1])<<8 | uint64(t[2])<<16 | uint64(t[3])<<24) << (8 * (m - 4))
			} else {
				w = uint64(b[0]) | uint64(b[m-1])<<(8*(m-1))
				if m == 3 {
					w |= uint64(b[1]) << 8
				}
			}
			w = w<<(8*(8-m)) | asciiZeros>>(8*m)
			// Every byte must lie in '.'..'9' (0x2E..0x39), which two
			// subtractions test at once: a byte below the range borrows in
			// w - 0x2E…, one above it in 0x39… - w, and either sets a high
			// bit (the bytes are ASCII, so a borrow is the only way one is
			// set). An exponent form such as "-1.5E+300" leaves here; '/'
			// passes and fails the digit test below.
			if (w-0x2E2E_2E2E_2E2E_2E2E|0x3939_3939_3939_3939-w)&0x8080_8080_8080_8080 == 0 {
				// A single point, found by the zero-byte test on
				// w ^ "........", is removed by shifting the bytes below it
				// up one, another '0' entering at the bottom; the digits
				// after a point at byte k of the padded word are 7 - k,
				// whatever the padding. A point alone has no digit and is a
				// syntax error.
				exp := 0
				const points = 0x2E2E_2E2E_2E2E_2E2E
				p := w ^ points
				if z := (p - 0x0101_0101_0101_0101) &^ p & 0x8080_8080_8080_8080; z != 0 {
					if m == 1 {
						return UBD128{Hi: UBDNaNHi}, Syntax // a point alone
					}
					k := bits.TrailingZeros64(z) / 8
					below := uint64(1)<<(8*k) - 1
					w = (w&below)<<8 | w&^(uint64(1)<<(8*(k+1))-1) | '0'
					exp = -(7 - k)
				}
				// All eight bytes digits proves the form: a second point or
				// a '/' fails here.
				if eightDigits(w) {
					// Exact, and inside the format: the fields as they are.
					sign := uint64(0)
					if s[0] == '-' {
						sign = UBDSignHi
					}
					return UBD128{Lo: eightDigitsValue(w), Hi: uint64(int64(exp))<<UBDExpShift | sign}, Exact
				}
			}
		}
	}
	n := scanDecimal(s, Precision128+1)
	switch n.kind {
	case scanSyntax:
		return UBD128{Hi: UBDNaNHi}, Syntax
	case scanNaN:
		return UBD128{Hi: UBDNaNHi}, Exact
	case scanInf:
		return ubdInf(n.neg), Exact
	}
	class := classZero
	if n.sticky {
		class = classBelowHalf
	}
	return ubdRoundStatus128(n.neg, cWide128(n.mant), n.exp, class, mode)
}

// UBDAppend128 is Append128 on UBD: the character form of x appended to
// dst, the short and long layouts on the parts of x (PutShort128,
// PutLong128), and every other value written out from its digits as
// append128 writes it. It needs MaxLen128 bytes of spare capacity to avoid
// allocating. The runtime prints decimal128 with it, where decimal128 is
// stored as UBD.
func UBDAppend128(dst []byte, x UBD128) []byte {
	sign := uint(x.Hi >> UBDSignShift & 1)
	if i := len(dst); UBDIsShort128(x) && cap(dst)-i >= ShortLen128 {
		return dst[:i+PutShort128((*[ShortLen128]byte)(dst[i:i+ShortLen128]), sign, uint(-(int64(x.Hi)>>UBDExpShift)), x.Lo)]
	}
	if UBDIsLong128(x) {
		exp, c := int(int64(x.Hi)>>UBDExpShift), Uint128{Hi: x.Hi & UBDCoeffMaskHi, Lo: x.Lo}
		if i := len(dst); cap(dst)-i >= LongLen128 {
			return dst[:i+PutLong128((*[LongLen128]byte)(dst[i:i+LongLen128]), sign, exp, c)]
		}
		var buf [LongLen128]byte
		return append(dst, buf[:PutLong128(&buf, sign, exp, c)]...)
	}
	kind, neg, c, exp := UBDUnpack128(x)
	switch kind {
	case KindNaN:
		return append(dst, "NaN"...)
	case KindInf:
		if neg {
			return append(dst, "-Inf"...)
		}
		return append(dst, "+Inf"...)
	}
	var digs [digitBufLen]byte
	hi, lo := cSplit128(c)
	var n int
	if hi == 0 {
		n = writeWordDigits(digs[:], lo)
	} else {
		n = writeDigits(digs[:], hi, lo)
	}
	return appendDecimal(dst, neg, digs[len(digs)-n:], exp)
}
