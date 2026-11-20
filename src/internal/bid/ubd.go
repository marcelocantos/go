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

// UBDWellFormed128 reports whether x is a well-formed UBD pattern: a finite
// value with qExp in [MinExp128, MaxExp128], or a special value, which has
// both the special tag and the 0x1F coefficient prefix. A finite coefficient
// above MaxCoeff128 is well-formed (it reads as zero), as is any NaN payload.
//
// The 14-bit qExp field holds -8192 to 8191, of which decimal128 uses
// -6176 to 6111 and the special tag 7168 to 8191; the 3072 values between
// have no meaning, and neither has the tag without the prefix. Values reach
// UBD only through this package's conversions and operations, which produce
// well-formed patterns, so a malformed one is a bug (or unsafe code). The
// inline paths and the kernel need not agree on one (see "The UBD kernel
// contract"), and math/decimal.Native128frombits stores one as a quiet NaN.
func UBDWellFormed128(x Uint128) bool {
	if x.Hi>>60 == UBDSpecialTag {
		return x.Hi&UBDSpecialCoeffHi == UBDSpecialCoeffHi
	}
	e := int64(x.Hi) >> UBDExpShift
	return MinExp128 <= e && e <= MaxExp128
}

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

// # The UBD kernel contract
//
// On a target whose decimal128 memory form is UBD, the compiler's decimal128
// operators call the entries below, and no other entry of this package that
// takes an encoded decimal128, for every operand pair their inline fast
// paths do not decide (cmd/compile/internal/ssagen, initBIDKernelSyms). They
// receive and return the stored words unchanged, each decimal128 as a
// UBD128, low word first: the order in which a decimal128 is laid out in
// memory and passed in registers, so that a call site moves nothing into
// place and the result needs no reordering where it joins the inline
// paths. Their names and signatures are a contract with the
// compiler and stay fixed; what lies behind them may change. Today each
// converts its operands to BID, calls the BID entry of the same name without
// the ubd prefix and converts the result back. A kernel that works on UBD
// itself replaces those bodies and nothing else.
//
// Each entry meets these rules, which TestUBDKernel checks:
//
//   - Operands. Every well-formed UBD pattern (UBDWellFormed128) is
//     accepted and read as BIDFromUBD128 reads it: a coefficient above
//     MaxCoeff128 is a zero and a NaN payload of 10^33 or more is a zero
//     payload. A malformed pattern can only come from a bug or from unsafe
//     code; the result of any operation on one is unspecified, and the
//     inline paths and the kernel may give different results for it.
//   - Modes. Each rounding operation has two entries: TTE rounds to
//     nearest, ties to even, and takes no mode; Rnd takes the caller's.
//     The language's operators and the functions whose rounding is fixed
//     at ties to even call TTE. Rnd accepts every Mode; one outside the six
//     behaves as ToNearestEven.
//   - Values. The result is that of the exported BID function on the
//     operands' BID encodings, converted to UBD:
//
//     ubdAddTTE128, ubdAddRnd128                          Add128
//     ubdSubTTE128, ubdSubRnd128                          Sub128
//     ubdMulTTE128, ubdMulRnd128                          Mul128
//     ubdDivTTE128, ubdDivRnd128                          Div128
//     ubdQuantizeTTE128, ubdQuantizeRnd128                Quantize128
//     ubdRoundToIntegralTTE128, ubdRoundToIntegralRnd128  RoundToIntegral128
//     ubdCompare128                                       Compare128 (c and ok as they are)
//
//     with mode ToNearestEven for a TTE entry. The compiler makes one call
//     for every operand pair its inline paths do not decide, so an entry
//     tests for the operands its own fast paths take.
//   - Results. Every decimal128 result is canonical UBD: a fixed point of
//     UBDFromBID128(BIDFromUBD128(r)). A finite result has qExp in range and
//     a coefficient at most MaxCoeff128, an infinity has no bits but its
//     tag and sign, and a NaN's payload is below 10^33.

// UBD128 is a UBD decimal128 as the kernel entries below take and return
// it: its two words, low first, as decimal128 is laid out.
type UBD128 struct{ Lo, Hi uint64 }

func (x UBD128) bid() Uint128 { return BIDFromUBD128(Uint128{Hi: x.Hi, Lo: x.Lo}) }

func ubdFromBID(x Uint128) UBD128 {
	u := UBDFromBID128(x)
	return UBD128{Lo: u.Lo, Hi: u.Hi}
}

// The entries come in pairs: TTE rounds to nearest, ties to even, as the
// language's operators and the functions without a mode do, and takes no
// mode; Rnd takes the caller's.

func ubdAddTTE128(x, y UBD128) UBD128 { return ubdAddRnd128(x, y, ToNearestEven) }

func ubdAddRnd128(x, y UBD128, mode Mode) UBD128 {
	return ubdFromBID(addSlow128(x.bid(), y.bid(), mode))
}

func ubdSubTTE128(x, y UBD128) UBD128 { return ubdSubRnd128(x, y, ToNearestEven) }

func ubdSubRnd128(x, y UBD128, mode Mode) UBD128 {
	return ubdFromBID(subSlow128(x.bid(), y.bid(), mode))
}

func ubdMulTTE128(x, y UBD128) UBD128 { return ubdMulRnd128(x, y, ToNearestEven) }

func ubdMulRnd128(x, y UBD128, mode Mode) UBD128 {
	return ubdFromBID(mulSlow128(x.bid(), y.bid(), mode))
}

func ubdDivTTE128(x, y UBD128) UBD128 { return ubdDivRnd128(x, y, ToNearestEven) }

func ubdDivRnd128(x, y UBD128, mode Mode) UBD128 {
	return ubdFromBID(div128(x.bid(), y.bid(), mode))
}

func ubdQuantizeTTE128(x, y UBD128) UBD128 { return ubdQuantizeRnd128(x, y, ToNearestEven) }

func ubdQuantizeRnd128(x, y UBD128, mode Mode) UBD128 {
	return ubdFromBID(quantizeSlow128(x.bid(), y.bid(), mode))
}

func ubdRoundToIntegralTTE128(x UBD128) UBD128 { return ubdRoundToIntegralRnd128(x, ToNearestEven) }

func ubdRoundToIntegralRnd128(x UBD128, mode Mode) UBD128 {
	return ubdFromBID(roundToIntegral128(x.bid(), mode))
}

func ubdCompare128(x, y UBD128) (c int, ok bool) {
	return quietCmpSlow128(x.bid(), y.bid())
}
