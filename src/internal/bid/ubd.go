// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

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

// UBDWellFormed128 reports whether x is a well-formed UBD pattern, which is
// to say a canonical one, as the conversion from BID produces:
//
//   - a finite value: qExp in [MinExp128, MaxExp128] and a coefficient at
//     most MaxCoeff128;
//   - an infinity: UBDInfHi and the sign, and no other bit;
//   - a NaN: UBDNaNHi or UBDSNaNHi and the sign, and a payload below 10^33
//     in its 110 bits, with the six qExp bits between the tag and the
//     payload clear.
//
// Every other pattern is malformed. The 14-bit qExp field holds -8192 to
// 8191, of which decimal128 uses -6176 to 6111 and the special tag 7168 to
// 8191; the 3072 values between have no meaning. BID and DPD have
// non-canonical encodings (a coefficient above MaxCoeff128, which denotes
// zero; a NaN payload of 10^33 or more, which denotes payload 0; unused bits
// in an infinity), but the conversion to UBD applies IEEE 754's reading of
// them, so UBD has none. Values reach
// UBD only through this package's conversions and operations, which produce
// well-formed patterns, so a malformed one is a bug (or unsafe code). The
// inline paths and the kernel need not agree on one (see "The UBD kernel
// contract"), and math/decimal.Native128frombits stores one as a quiet NaN.
func UBDWellFormed128(x Uint128) bool {
	hi := x.Hi
	if hi>>60 != UBDSpecialTag {
		e := int64(hi) >> UBDExpShift
		return MinExp128 <= e && e <= MaxExp128 && !u128GtMaxCoeff(Uint128{Hi: hi & UBDCoeffMaskHi, Lo: x.Lo})
	}
	switch hi >> 58 {
	case UBDInfTag << 1: // an infinity, with the signaling bit clear
		return hi&^UBDSignHi == UBDInfHi && x.Lo == 0
	case UBDNaNTag << 1, UBDSNaNTag:
		// The tag, the unused qExp bits 57..52 clear, the payload's top
		// two bits in qExp, the 0x1F prefix, the payload's next 44 bits.
		if hi>>52&0x3F != 0 || hi&UBDSpecialCoeffHi != UBDSpecialCoeffHi {
			return false
		}
		ph := hi>>UBDExpShift&3<<44 | hi&(1<<44-1)
		return ph < MaxPayload128Hi || ph == MaxPayload128Hi && x.Lo <= MaxPayload128Lo
	}
	return false
}

// UBDFromBID128 returns the UBD encoding of the value the BID encoding x
// denotes: a non-canonical coefficient becomes zero and a non-canonical NaN
// payload zero, as IEEE 754-2019 reads them.
func UBDFromBID128(x Uint128) Uint128 {
	// A finite value of the first form with a coefficient below
	// MaxCoeff128Hi × 2^64 moves its exponent and sign and keeps its
	// coefficient: no unpacking. Every other encoding takes ubdFromBIDAny.
	if t := x.Hi; t&Form2Mask128Hi != Form2Mask128Hi && t&CoeffMask128Hi < MaxCoeff128Hi {
		e := int64(t>>ExpShift128&ExpMask128) - Bias128
		return Uint128{Hi: uint64(e)<<UBDExpShift | t>>63<<49 | t&CoeffMask128Hi, Lo: x.Lo}
	}
	return ubdFromBIDAny(x)
}

// ubdFromBIDAny is UBDFromBID128 for every encoding.
func ubdFromBIDAny(x Uint128) Uint128 {
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
// denotes. No operation produces a malformed pattern; of them, a
// coefficient above MaxCoeff128 reads as zero, a NaN payload of 10^33 or
// more as payload 0, and an exponent out of range as a quiet NaN.
func BIDFromUBD128(x Uint128) Uint128 {
	// A finite value with its exponent in range and a coefficient below
	// MaxCoeff128Hi × 2^64 moves its exponent and sign and keeps its
	// coefficient. Every other pattern takes bidFromUBDAny.
	if e := int64(x.Hi) >> UBDExpShift; uint64(e-MinExp128) <= MaxExp128-MinExp128 && x.Hi&UBDCoeffMaskHi < MaxCoeff128Hi {
		return Uint128{Hi: x.Hi>>49&1<<63 | uint64(e+Bias128)<<ExpShift128 | x.Hi&UBDCoeffMaskHi, Lo: x.Lo}
	}
	return bidFromUBDAny(x)
}

// bidFromUBDAny is BIDFromUBD128 for every pattern.
func bidFromUBDAny(x Uint128) Uint128 {
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
//   - Operands. Every well-formed (canonical) UBD pattern
//     (UBDWellFormed128) is accepted and read as BIDFromUBD128 reads it. A
//     finite coefficient is at most MaxCoeff128, so a finite value is zero
//     exactly when its coefficient is 0, which the inline paths rely on. A
//     malformed pattern can only come from a bug or from unsafe
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
//     ubdEqual128, ubdLess128, ubdLessEqual128,           Compare128: ok and c == 0, c < 0,
//     ubdGreater128, ubdGreaterEqual128                   c <= 0, c > 0, c >= 0
//
//     with mode ToNearestEven for a TTE entry. The compiler makes one call
//     for every operand pair its inline paths do not decide, so an entry
//     tests for the operands its own fast paths take.
//   - Results. Every decimal128 result is well-formed: a fixed point of
//     UBDFromBID128(BIDFromUBD128(r)).

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

// Add, subtract and multiply are this package's arithmetic on UBD itself
// (ubd_add.go, ubd_mul.go): addSlow128, addLong128, mulSlow128, mulLong128
// and their rounding with UBD's decoding and packing, nothing converted.
// Quantize, compare and round-to-integral handle word-sized coefficients on
// UBD (ubdQuantizeWord, ubdCompareWord, ubdRoundToIntegralWord) and send
// every other operand through BID to this package's general paths. Divide
// is this package's divide on UBD itself (ubd_div.go). decimal128-go's add, multiply, quantize and
// compare were measured slower than this package's (see internal/d128).

func ubdAddTTE128(x, y UBD128) UBD128 { return ubdAddSlow128(x, y, ToNearestEven) }

func ubdAddRnd128(x, y UBD128, mode Mode) UBD128 { return ubdAddSlow128(x, y, mode) }

func ubdSubTTE128(x, y UBD128) UBD128 { return ubdSubSlow128(x, y, ToNearestEven) }

func ubdSubRnd128(x, y UBD128, mode Mode) UBD128 { return ubdSubSlow128(x, y, mode) }

func ubdMulTTE128(x, y UBD128) UBD128 {
	if r, ok := ubdMulShort128(x, y); ok {
		return r
	}
	return ubdMulSlow128(x, y, ToNearestEven)
}

func ubdMulRnd128(x, y UBD128, mode Mode) UBD128 {
	if r, ok := ubdMulShort128(x, y); ok {
		return r
	}
	return ubdMulSlow128(x, y, mode)
}

func ubdDivTTE128(x, y UBD128) UBD128 { return ubdDiv128(x, y, ToNearestEven) }

func ubdDivRnd128(x, y UBD128, mode Mode) UBD128 { return ubdDiv128(x, y, mode) }

func ubdQuantizeTTE128(x, y UBD128) UBD128 { // ubdQuantize128, written out
	if ubdQuantizeSame128(x, y) {
		return x
	}
	if x.Hi&UBDCoeffMaskHi == 0 {
		if r, ok := ubdQuantizeWord(x, y, ToNearestEven); ok {
			return r
		}
	}
	return ubdQuantizeSlow128(x, y, ToNearestEven)
}

func ubdQuantizeRnd128(x, y UBD128, mode Mode) UBD128 { // ubdQuantize128, written out
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

// ubdQuantizeWord quantizes x to y's exponent on UBD itself when x is
// finite with a coefficient of one word and y is finite, which covers
// money values, and reports false for every other pair, which goes
// through BID. A finer quantum multiplies by 10^gap (gap up to 19) unless
// the product would exceed MaxCoeff128, which is invalid; a coarser one
// divides by 10^d (d up to 19) and rounds by the mode. The result keeps
// x's sign, also when it is zero. A mode outside the six rounds to
// nearest, ties to even.
func ubdQuantizeWord(x, y UBD128, mode Mode) (UBD128, bool) {
	// A special x has the 0x1F prefix in its coefficient's high word, so the
	// one-word test excludes it.
	if x.Hi&UBDCoeffMaskHi != 0 || y.Hi>>60 == UBDSpecialTag {
		return UBD128{}, false
	}
	xe, ye := int64(x.Hi)>>UBDExpShift, int64(y.Hi)>>UBDExpShift
	head := uint64(ye)<<UBDExpShift | x.Hi&UBDSignHi
	c := x.Lo
	switch gap := xe - ye; {
	case gap == 0:
		return x, true
	case gap > 0:
		if gap >= int64(len(pow10tab64)) {
			return UBD128{}, false
		}
		hi, lo := bits.Mul64(c, pow10tab64[gap])
		if hi > MaxCoeff128Hi || hi == MaxCoeff128Hi && lo > MaxCoeff128Lo {
			return UBD128{}, false // invalid: the BID path gives the NaN
		}
		return UBD128{Lo: lo, Hi: head | hi}, true
	case gap < -int64(len(pow10tab64)-1):
		return UBD128{}, false
	default:
		p := pow10tab64[-gap]
		q := c / p
		rem := c - q*p
		half := p >> 1
		var up bool
		switch mode {
		case ToNearestAway:
			up = rem >= half
		case TowardZero:
		case AwayFromZero:
			up = rem != 0
		case TowardNegative:
			up = rem != 0 && x.Hi&UBDSignHi != 0
		case TowardPositive:
			up = rem != 0 && x.Hi&UBDSignHi == 0
		default:
			up = rem > half || rem == half && q&1 != 0
		}
		if up {
			q++
		}
		return UBD128{Lo: q, Hi: head}, true
	}
}

func ubdRoundToIntegralTTE128(x UBD128) UBD128 {
	if r, ok := ubdRoundToIntegralWord(x, ToNearestEven); ok {
		return r
	}
	return ubdFromBID(roundToIntegral128(x.bid(), ToNearestEven))
}

func ubdRoundToIntegralRnd128(x UBD128, mode Mode) UBD128 {
	if r, ok := ubdRoundToIntegralWord(x, mode); ok {
		return r
	}
	return ubdFromBID(roundToIntegral128(x.bid(), mode))
}

// ubdRoundToIntegralWord rounds x to an integer on UBD itself when x is
// finite with a coefficient of one word, and reports false otherwise. An x
// with an exponent of zero or more is an integer already and is returned as
// it is; one with a fraction of 1 to 19 digits is x quantized to exponent
// zero (ubdQuantizeWord against 1E0): one division, rounding by the mode,
// the sign kept also on a zero.
func ubdRoundToIntegralWord(x UBD128, mode Mode) (UBD128, bool) {
	if x.Hi&UBDCoeffMaskHi != 0 {
		return UBD128{}, false // also every special: its prefix is in those bits
	}
	if int64(x.Hi) >= 0 {
		return x, true // qExp >= 0: qExp's sign is the word's top bit
	}
	return ubdQuantizeWord(x, UBD128{Lo: 1}, mode)
}

// ubdCompare128 orders x and y. Operands whose coefficients are one word
// each, which covers money values, are ordered on UBD itself
// (ubdCompareWord); every other pair goes through BID to quietCmpSlow128.
// decimal128-go's compare was measured slower than both (see
// internal/d128).
func ubdCompare128(x, y UBD128) (c int, ok bool) {
	if c, ok := ubdCompareWord(x, y); ok {
		return c, true
	}
	return quietCmpSlow128(x.bid(), y.bid())
}

// ubdCompareWord orders x and y when both are finite with a coefficient of
// one word, and reports false otherwise. Zeros, of either sign and any
// exponent, are equal and order against the other operand's sign, and
// operands of different signs order by sign. Otherwise the magnitudes
// compare with the larger exponent's coefficient scaled to the smaller
// exponent: one 64x64 product for a gap up to 19, and for a larger gap that
// side is the larger, since a nonzero coefficient times 10^20 exceeds any
// word.
func ubdCompareWord(x, y UBD128) (c int, ok bool) {
	// A special has the 0x1F prefix in its coefficient's high word, so the
	// one-word test excludes it.
	if x.Hi&UBDCoeffMaskHi != 0 || y.Hi&UBDCoeffMaskHi != 0 {
		return 0, false
	}
	xneg, yneg := x.Hi&UBDSignHi != 0, y.Hi&UBDSignHi != 0
	cx, cy := x.Lo, y.Lo
	switch {
	case cx == 0 && cy == 0:
		return 0, true
	case cx == 0:
		return signOrder(!yneg), true
	case cy == 0, xneg != yneg:
		return signOrder(xneg), true
	}
	ex, ey := int64(x.Hi)>>UBDExpShift, int64(y.Hi)>>UBDExpShift
	var m int
	switch {
	case ex == ey:
		m = wordOrder(0, cx, 0, cy)
	case ex-ey >= int64(len(pow10tab64)):
		m = 1
	case ey-ex >= int64(len(pow10tab64)):
		m = -1
	case ex > ey:
		hi, lo := bits.Mul64(cx, pow10tab64[ex-ey])
		m = wordOrder(hi, lo, 0, cy)
	default:
		hi, lo := bits.Mul64(cy, pow10tab64[ey-ex])
		m = wordOrder(0, cx, hi, lo)
	}
	if xneg {
		m = -m
	}
	return m, true
}

// signOrder is -1 for a negative operand against a zero or a positive one,
// +1 otherwise: the order of x when only its sign decides.
func signOrder(neg bool) int {
	if neg {
		return -1
	}
	return 1
}

// wordOrder orders the 128-bit (ah, al) and (bh, bl).
func wordOrder(ah, al, bh, bl uint64) int {
	switch {
	case ah != bh:
		if ah < bh {
			return -1
		}
		return 1
	case al < bl:
		return -1
	case al > bl:
		return 1
	}
	return 0
}

// The predicate entries: each returns the operator's result itself, so
// that a call site tests one bool instead of decoding c and ok. Unordered
// operands (a NaN among them) satisfy none of them; x != y is
// !ubdEqual128(x, y). x > y and x >= y have entries of their own so that a
// call site passes its operands where they are, not swapped.

func ubdEqual128(x, y UBD128) bool {
	c, ok := ubdCompare128(x, y)
	return ok && c == 0
}

func ubdLess128(x, y UBD128) bool {
	c, ok := ubdCompare128(x, y)
	return ok && c < 0
}

func ubdLessEqual128(x, y UBD128) bool {
	c, ok := ubdCompare128(x, y)
	return ok && c <= 0
}

func ubdGreater128(x, y UBD128) bool {
	c, ok := ubdCompare128(x, y)
	return ok && c > 0
}

func ubdGreaterEqual128(x, y UBD128) bool {
	c, ok := ubdCompare128(x, y)
	return ok && c >= 0
}
