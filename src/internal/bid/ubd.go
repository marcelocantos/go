// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// UBD (unpacked binary decimal) is an in-memory layout for decimal128:
//
//	bits 127..114  qExp, the exponent, two's complement, unbiased
//	bit  113       sign
//	bits 112..0    coefficient, a binary integer
//
// In the high word that is qExp in bits 63..50, the sign in bit 49 and the
// top 49 coefficient bits in bits 48..0, so that all bits clear is +0E+0
// and needs no storage mask. A finite value has qExp in
// [MinExp128, MaxExp128] and a coefficient at most MaxCoeff128.
//
// Every special value has 0b0111 in the top four bits of qExp (qExp 7168
// and up) and 0x1F in the top five coefficient bits. Below that tag, the
// next qExp bit tells a NaN from an infinity and the one after it a
// signaling NaN from a quiet one. A NaN's 110-bit payload is the low 108
// coefficient bits with the low two bits of qExp above them.
//
// See https://migueldecimal128.github.io/decimal128-www/whitepapers/ubd.html.
const (
	UBDExpShift    = 50
	UBDSignHi      = 1 << 49
	UBDCoeffMaskHi = 1<<49 - 1
	// UBDSpecialCoeffHi is the 0x1F prefix of every special value's
	// coefficient, in the high word.
	UBDSpecialCoeffHi uint64 = 0x1F << 44

	UBDInfHi  uint64 = 7168<<UBDExpShift | UBDSpecialCoeffHi
	UBDNaNHi  uint64 = 7680<<UBDExpShift | UBDSpecialCoeffHi
	UBDSNaNHi uint64 = 7936<<UBDExpShift | UBDSpecialCoeffHi

	// The tag tests: the top four, five and six bits of the high word.
	UBDSpecialTag = 7  // hi>>60: an infinity or a NaN
	UBDInfTag     = 14 // hi>>59
	UBDNaNTag     = 15 // hi>>59
	UBDSNaNTag    = 31 // hi>>58
)

// UBDIsSpecial128 reports whether the UBD encoding x is an infinity or a NaN.
func UBDIsSpecial128(x Uint128) bool { return x.Hi>>60 == UBDSpecialTag }

// UBDFromBID128 returns the UBD encoding of the value the BID encoding x
// denotes: a non-canonical coefficient becomes zero and a non-canonical NaN
// payload zero, as IEEE 754-2019 reads them.
func UBDFromBID128(x Uint128) Uint128 {
	kind, neg, coeff, exp := Unpack128(x)
	var sign uint64
	if neg {
		sign = UBDSignHi
	}
	switch kind {
	case KindInf:
		return Uint128{Hi: UBDInfHi | sign}
	case KindNaN:
		p := Uint128{Hi: x.Hi & (1<<46 - 1), Lo: x.Lo}
		if p.Hi > MaxPayload128Hi || p.Hi == MaxPayload128Hi && p.Lo > MaxPayload128Lo {
			p = Uint128{}
		}
		hi := UBDNaNHi
		if IsSignaling128(x) {
			hi = UBDSNaNHi
		}
		// p < 10^33 < 2^110: its high word has 46 bits, the top two of
		// which go in qExp.
		return Uint128{Hi: hi | sign | p.Hi>>44<<UBDExpShift | p.Hi&(1<<44-1), Lo: p.Lo}
	}
	return Uint128{Hi: uint64(exp)<<UBDExpShift | sign | coeff.Hi, Lo: coeff.Lo}
}

// BIDFromUBD128 returns the BID encoding of the value the UBD encoding x
// denotes. A coefficient above MaxCoeff128 reads as zero, a NaN payload of
// 10^33 or more as zero, and a pattern with neither the special tag nor an
// exponent in range, which no operation produces, as a quiet NaN.
func BIDFromUBD128(x Uint128) Uint128 {
	hi := x.Hi
	var sign uint64
	if hi&UBDSignHi != 0 {
		sign = SignMask128Hi
	}
	if hi>>60 == UBDSpecialTag {
		if hi>>59 == UBDInfTag {
			return Uint128{Hi: Inf128Hi | sign}
		}
		ph := hi>>UBDExpShift&3<<44 | hi&(1<<44-1)
		if ph > MaxPayload128Hi || ph == MaxPayload128Hi && x.Lo > MaxPayload128Lo {
			ph, x.Lo = 0, 0
		}
		nan := NaN128Hi
		if hi>>58 == UBDSNaNTag {
			nan = SNaN128Hi
		}
		return Uint128{Hi: nan | sign | ph, Lo: x.Lo}
	}
	exp := int(int64(hi) >> UBDExpShift)
	if exp < MinExp128 || exp > MaxExp128 {
		return Uint128{Hi: NaN128Hi}
	}
	c := Uint128{Hi: hi & UBDCoeffMaskHi, Lo: x.Lo}
	if u128GtMaxCoeff(c) {
		c = Uint128{}
	}
	x, _ = Pack128(sign != 0, c, exp)
	return x
}

// The largest canonical decimal128 NaN payload, 10^33 - 1.
const (
	MaxPayload128Hi uint64 = 0x0000314DC6448D93
	MaxPayload128Lo uint64 = 0x38C15B09FFFFFFFF
)

// The kernel's entries that the compiler calls directly on a target whose
// decimal128 memory form is UBD: each converts its operands to BID, calls
// the BID entry and converts the result back, so that the code at each call
// site passes and receives the stored words unchanged.

func ubdAddSlow128(x, y Uint128, mode Mode) Uint128 {
	return UBDFromBID128(addSlow128(BIDFromUBD128(x), BIDFromUBD128(y), mode))
}

func ubdAddLong128(x, y Uint128, mode Mode) Uint128 {
	return UBDFromBID128(addLong128(BIDFromUBD128(x), BIDFromUBD128(y), mode))
}

func ubdMulSlow128(x, y Uint128, mode Mode) Uint128 {
	return UBDFromBID128(mulSlow128(BIDFromUBD128(x), BIDFromUBD128(y), mode))
}

func ubdMulLong128(x, y Uint128, mode Mode) Uint128 {
	return UBDFromBID128(mulLong128(BIDFromUBD128(x), BIDFromUBD128(y), mode))
}

func ubdDiv128(x, y Uint128, mode Mode) Uint128 {
	return UBDFromBID128(div128(BIDFromUBD128(x), BIDFromUBD128(y), mode))
}

func ubdQuietCmpSlow128(x, y Uint128) (c int, ok bool) {
	return quietCmpSlow128(BIDFromUBD128(x), BIDFromUBD128(y))
}

func ubdQuantizeSlow128(x, y Uint128, mode Mode) Uint128 {
	return UBDFromBID128(quantizeSlow128(BIDFromUBD128(x), BIDFromUBD128(y), mode))
}

func ubdQuantizeLong128(x, y Uint128, mode Mode) Uint128 {
	return UBDFromBID128(quantizeLong128(BIDFromUBD128(x), BIDFromUBD128(y), mode))
}

func ubdRoundToIntegral128(x Uint128, mode Mode) Uint128 {
	return UBDFromBID128(roundToIntegral128(BIDFromUBD128(x), mode))
}
