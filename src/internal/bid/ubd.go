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
	UBDSignShift   = 49
	UBDSignHi      = 1 << UBDSignShift
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
// non-canonical encodings, and IEEE 754-2019 clause 3.5.2 gives each its
// value: a significand above its format's maximum, 10^34 - 1 (10^33 - 1 as
// a NaN's payload), is non-canonical and its value is zero (item c.2 for
// the binary encoding of the significand), and the reserved bits of an
// infinity or a NaN are ignored. The conversion to UBD stores that value,
// so UBD has no non-canonical patterns. Values reach
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
// denotes. x is any 128 bits, off the wire or from a caller, and a
// non-canonical significand has the value IEEE 754-2019 clause 3.5.2 item
// c.2 gives it: one above 10^34 - 1, or above 10^33 - 1 as a NaN's payload,
// is zero. Every second-form significand, at 2^113 or more, is such a one.
func UBDFromBID128(x Uint128) Uint128 {
	// A first-form encoding whose coefficient's bit 112 is clear is
	// canonical with no comparison, its coefficient below 2^112 < 10^34,
	// and its exponent in range, as every first-form exponent is: it moves
	// its exponent and sign and keeps its coefficient. The form test also
	// sends the specials and the non-canonical second-form encodings,
	// whose significand is zero by clause 3.5.2 c.2, to ubdFromBIDAny,
	// with the coefficients from 2^112 up, which it compares with
	// 10^34 - 1.
	if t := x.Hi; t&Form2Mask128Hi != Form2Mask128Hi && t&(1<<(ExpShift128-1)) == 0 {
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

// BIDFromUBD128 returns the BID encoding of the well-formed UBD x
// (UBDWellFormed128), as every stored decimal128 is. A finite one is
// canonical: its exponent is in range and its coefficient below 10^34, so
// it has BID's first form, whose 113 coefficient bits hold every such
// coefficient (a second-form coefficient begins 100, at 2^113, above every
// canonical decimal128). Its sign and exponent move and its coefficient
// stays; the one test is for a special, which BIDFromUBDAny128 converts.
// A malformed pattern is a bug, and what this returns for one is
// unspecified; BIDFromUBDAny128 reads every pattern.
func BIDFromUBD128(x Uint128) Uint128 {
	if x.Hi>>60 != UBDSpecialTag {
		return Uint128{Hi: x.Hi>>UBDSignShift&1<<63 | uint64(int64(x.Hi)>>UBDExpShift+Bias128)<<ExpShift128 | x.Hi&UBDCoeffMaskHi, Lo: x.Lo}
	}
	return BIDFromUBDAny128(x)
}

// BIDFromUBDAny128 is BIDFromUBD128 for every pattern, malformed ones
// included, which a debugger may be shown. IEEE 754 does not define UBD, so
// the reading of a malformed pattern is this package's, modelled on clause
// 3.5.2 c.2's for BID: a coefficient above MaxCoeff128 reads as zero and a
// NaN payload of 10^33 or more as payload 0; an exponent out of range, which
// BID cannot encode, reads as a quiet NaN.
func BIDFromUBDAny128(x Uint128) Uint128 {
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

// Widening and narrowing between decimal64, stored as BID64, and
// decimal128 stored as UBD: conversions straight between the two
// encodings, with no BID128 between them.

// ubdWiden128 is Widen with a UBD result, the conversion of a decimal64,
// stored as BID64, to decimal128 stored as UBD: the same value and
// exponent, exact. A first-form encoding needs no test beyond its form:
// its coefficient is below 2^53 < 10^16, so canonical, and its biased
// exponent at most 767, so in range; the sign and the exponent move and
// the coefficient stays. The second form, a non-canonical coefficient (by
// IEEE 754-2019 clause 3.5.2, zero) and the specials take ubdWidenRest128.
func ubdWiden128(x uint64) UBD128 {
	if x&Form2Mask64 != Form2Mask64 {
		return UBD128{Lo: x & (1<<CoeffBits64 - 1), Hi: uint64(int64(x>>ExpShift64&ExpMask64)-Bias64)<<UBDExpShift | x>>63<<UBDSignShift}
	}
	return ubdWidenRest128(x)
}

// ubdWidenRest128 is ubdWiden128 for the second form and the specials, as
// Widen converts them: a NaN made quiet with its payload, which a
// decimal128 NaN holds, and an infinity with its sign.
//
//go:noinline
func ubdWidenRest128(x uint64) UBD128 {
	kind, neg, coeff, exp := Unpack64(x)
	switch kind {
	case KindNaN:
		hi := UBDNaNHi
		if x&SignMask64 != 0 {
			hi |= UBDSignHi
		}
		return UBD128{Lo: cQuiet64(x) & nanPayloadMask64, Hi: hi}
	case KindInf:
		return ubdInf(neg)
	}
	return ubdFinite(neg, coef128{Lo: coeff}, exp)
}

// ubdNarrow128 is Narrow on a UBD operand, the conversion of a decimal128
// stored as UBD to decimal64 stored as BID64, rounded in mode with
// preferred exponent Q(x). A value that decimal64 holds as it is, a
// one-word coefficient below 2^53 at an exponent in its range, packs
// BID64's first form directly; every other goes to ubdNarrowRest128.
func ubdNarrow128(x UBD128, mode Mode) uint64 {
	if q := int64(x.Hi) >> UBDExpShift; x.Hi&UBDCoeffMaskHi == 0 && x.Lo < 1<<CoeffBits64 && uint64(q-MinExp64) <= MaxExp64-MinExp64 {
		return x.Hi>>UBDSignShift&1<<63 | uint64(q+Bias64)<<ExpShift64 | x.Lo
	}
	return ubdNarrowRest128(x, mode)
}

// ubdNarrowRest128 is Narrow on UBD for every other operand: a NaN made
// quiet, its payload kept when decimal64 holds it canonically and zero
// otherwise; an infinity with its sign; and a finite value rounded by
// round64 from its whole coefficient.
//
//go:noinline
func ubdNarrowRest128(x UBD128, mode Mode) uint64 {
	kind, neg, coeff, exp := UBDUnpack128(x)
	switch kind {
	case KindNaN:
		var sign uint64
		if neg {
			sign = SignMask64
		}
		ph := x.Hi&(1<<44-1) | x.Hi>>UBDExpShift&3<<44
		if ph != 0 || x.Lo >= maxNaNPayload64 {
			return sign | NaN64
		}
		return sign | NaN64 | x.Lo
	case KindInf:
		return cInf64(neg)
	}
	z, _ := round64(neg, coeff, exp, classZero, mode, MinExp64)
	return z
}

// ubdNarrowTTE128 is ubdNarrow128 rounding to nearest, ties to even: the
// language's conversion of a decimal128 to decimal64, a kernel entry.
func ubdNarrowTTE128(x UBD128) uint64 { return ubdNarrow128(x, ToNearestEven) }

// UBDWiden128 and UBDNarrow128 are ubdWiden128 and ubdNarrow128 for the
// runtime and strconv, which convert between decimal64 and decimal128 with
// them where decimal128 is stored as UBD.
func UBDWiden128(x uint64) UBD128             { return ubdWiden128(x) }
func UBDNarrow128(x UBD128, mode Mode) uint64 { return ubdNarrow128(x, mode) }

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
// compiler and stay fixed; what lies behind them may change. Each works on
// UBD itself: it is this package's BID algorithm of the same name without the
// ubd prefix, with the BID decoding and packing replaced by UBD's (ubd_add.go,
// ubd_mul.go, ubd_div.go, ubd_quant.go, ubd_cmp.go, ubd_conv.go). None
// converts to BID. A conversion from int64 or uint64 has no entry: the
// compiler builds the coefficient at exponent 0 inline.
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
//     ubdMin128, ubdMax128                                Min128, Max128
//     ubdInt64From128, ubdUint64From128                   Int64From128, Uint64From128
//                                                         with TowardZero, the result alone
//     ubdWiden128 (a BID64 operand)                       Widen
//     ubdNarrowTTE128 (a BID64 result)                    Narrow with ToNearestEven
//
//     with mode ToNearestEven for a TTE entry. The compiler makes one call
//     for every operand pair its inline paths do not decide, so an entry
//     tests for the operands its own fast paths take.
//   - Results. Every decimal128 result is well-formed: a fixed point of
//     UBDFromBID128(BIDFromUBD128(r)).

// UBD128 is a UBD decimal128 as the kernel entries below take and return
// it: its two words, low first, as decimal128 is laid out.
type UBD128 struct{ Lo, Hi uint64 }

// The entries come in pairs: TTE rounds to nearest, ties to even, as the
// language's operators and the functions without a mode do, and takes no
// mode; Rnd takes the caller's.

// Add, subtract, multiply, divide and quantize are this package's
// arithmetic on UBD itself (ubd_add.go, ubd_mul.go, ubd_div.go,
// ubd_quant.go): its general paths with UBD's decoding and packing, nothing
// converted. Quantize and round-to-integral have word paths in front
// (ubdQuantizeWord, called by the quantize and round-to-integral entries).
// Compare is this package's on UBD itself (ubd_cmp.go).
//
// decimal128-go (github.com/migueldecimal128/decimal128-go), whose
// arithmetic works on UBD itself, was tried behind these entries and
// measured slower than this package's on money and wide operands alike
// (2026-11-21, Apple M3): an add of 34-digit operands 22.5 ns against 7.5,
// a multiply 29.1 against 10.6, a divide 23.6 against 11.7, a quantize to
// cents about 20 against 4.9, a compare 8.5 against 3.3. Its general paths
// take 128- and 256-bit routes and a final rounding step for word-sized
// operands too.

// ubdAddTTE128 is in ubd_add.go.

// ubdAddRnd128 has no caller in compiled code: ubdAddTTE128 for
// ToNearestEven, ubdAddLong128, which takes every mode, otherwise.
func ubdAddRnd128(x, y UBD128, mode Mode) UBD128 {
	if mode == ToNearestEven {
		return ubdAddTTE128(x, y)
	}
	return ubdAddLong128(x, y, mode)
}

func ubdSubTTE128(x, y UBD128) UBD128 {
	if (x.Hi|y.Hi)&UBDCoeffMaskHi == 0 {
		if r, ok := ubdAddWordTTE(x, UBD128{Lo: y.Lo, Hi: y.Hi ^ UBDSignHi}); ok {
			return r
		}
	}
	if y.Hi>>59 != UBDNaNTag {
		y.Hi ^= UBDSignHi // as in ubdSubSlow128
	}
	return ubdAddTTE128(x, y)
}

// ubdAddWordTTE adds two finite operands whose coefficients are one word
// each, so that their high words hold only qExp and the sign, when
// their exponents are at most maxWordGap apart: the exact sum at the smaller
// exponent, the preferred one, and false when it passes MaxCoeff128 and must
// round (ubdAddLong128's word path, which this is, then rounds it). It is the
// first thing the ties-to-even entries try, before any call.
//
// Three paths, in the order of how common they are in money arithmetic:
//
//   - One exponent and one sign, a sum of amounts: an add and its carry.
//     This is a branch, and it must be: a running total waits for each sum,
//     and with the sum chosen by masks (the third path) it took 4.82 ns a
//     step against 2.39 (and BID's 3.72); operands of mixed sign, which
//     mispredict it, take 2.67 against 2.21 that way.
//   - One exponent and opposite signs, a difference: both differences, and
//     the one that did not borrow chosen by a select, not a branch, since
//     x - y of random positive amounts goes either way half the time (a
//     branch there took 5.89 ns). A running balance that stays positive
//     takes 2.39 ns a step.
//   - Exponents up to maxWordGap apart: the operand with the larger exponent
//     scaled, and the other added or subtracted, each chosen by a branch, as
//     in ubdAddLong128, and a difference that goes negative negated by a
//     select, its magnitude then below 2^64. These are most of what reaches
//     the kernel at -d=decimalinline=2, whose inline path takes one exponent and
//     one sign. With the operand and the operation chosen by masks instead,
//     tax_preparation's FED program at level 2 was 3.5% slower than before
//     this word path existed; with branches, 4.4% faster. (An amount and a
//     rate, 2.52 ns against 3.52; a running total at either of two
//     exponents at random, 3.49 against 4.50 a step.)
//
// (Apple M3 Pro, BenchmarkUBDKernel, rows money, cents10neg, gap2, mixexp,
// Sub and the AddChain and SubChain rows.) An exact zero is the operands' sign when they
// agree and +0 when they differ, as rounding to nearest gives (IEEE 754-2019
// clause 6.3). The same one-exponent branch in ubdCmpWord was set aside: on
// operands of one exponent or two apart at random a compare took 50%
// longer.
func ubdAddWordTTE(x, y UBD128) (UBD128, bool) {
	xh, xl, yh, yl := x.Hi, x.Lo, y.Hi, y.Lo
	if (xh^yh)>>UBDSignShift == 0 {
		// One exponent and one sign: the usual sum, an add and its carry.
		lo, c := bits.Add64(xl, yl, 0)
		return UBD128{Lo: lo, Hi: xh | c}, true
	}
	if (xh^yh)>>UBDExpShift == 0 {
		// One exponent, opposite signs: both differences, and the one
		// that did not borrow, chosen by a select.
		d, b := bits.Sub64(xl, yl, 0)
		if b != 0 {
			d = yl - xl
		}
		sign := (xh>>UBDSignShift ^ b) & b2u(d != 0)
		return UBD128{Lo: d, Hi: xh&^UBDSignHi | sign<<UBDSignShift}, true
	}
	// a is the operand with the larger exponent, b the other, whose
	// exponent is the result's. Which it is goes with the data's kind
	// (an amount and a rate), so it is a branch.
	ah, al, bh, bl := xh, xl, yh, yl
	if int64(xh) < int64(yh) { // qExp is the high word's top bits
		ah, al, bh, bl = yh, yl, xh, xl
	}
	gap := uint64(int64(ah)>>UBDExpShift - int64(bh)>>UBDExpShift)
	if gap > uint64(maxWordGap) {
		return UBD128{}, false
	}
	hi, lo := bits.Mul64(al, pow10tab64[gap])
	sign := ah >> UBDSignShift & 1
	if (ah^bh)&UBDSignHi == 0 {
		var c uint64
		lo, c = bits.Add64(lo, bl, 0)
		hi += c
	} else {
		// a × 10^gap - b, negative only when a × 10^gap is below 2^64:
		// both differences, and the one that did not borrow selected.
		d, b := bits.Sub64(lo, bl, 0)
		var bw uint64
		hi, bw = bits.Sub64(hi, 0, b)
		lo = d
		if bw != 0 {
			lo, hi, sign = -d, 0, sign^1 // b's magnitude, below 2^64, the larger
		}
		sign &= b2u(hi|lo != 0) // an exact zero is +0
	}
	_, bw := bits.Sub64(MaxCoeff128Lo, lo, 0)
	_, bw = bits.Sub64(MaxCoeff128Hi, hi, bw)
	if bw != 0 {
		return UBD128{}, false
	}
	return UBD128{Lo: lo, Hi: bh&^(UBDSignHi|UBDCoeffMaskHi) | sign<<UBDSignShift | hi}, true
}

func ubdSubRnd128(x, y UBD128, mode Mode) UBD128 { return ubdSubSlow128(x, y, mode) }

// ubdMulTTE128 is in ubd_mul.go.

func ubdMulRnd128(x, y UBD128, mode Mode) UBD128 {
	if mode == ToNearestEven {
		return ubdMulTTE128(x, y)
	}
	if r, ok := ubdMulShort128(x, y); ok {
		return r
	}
	return ubdMulLong128(x, y, mode)
}

// ubdDivTTE128 is in ubd_div.go.

func ubdDivRnd128(x, y UBD128, mode Mode) UBD128 {
	if mode == ToNearestEven {
		return ubdDivTTE128(x, y)
	}
	return ubdDivLong128(x, y, mode)
}

func ubdQuantizeTTE128(x, y UBD128) UBD128 { // ubdQuantize128, written out
	if ubdQuantizeSame128(x, y) {
		return x
	}
	if x.Hi&UBDCoeffMaskHi == 0 {
		if r, ok := ubdQuantizeWord(x, y, ToNearestEven); ok {
			return r
		}
	} else if r, ok := ubdQuantizeWideTTE(x, y); ok {
		return r
	}
	return ubdQuantizeSlow128(x, y, ToNearestEven)
}

// ubdQuantizeWideTTE is ubdQuantizeSlow128 for ties to even, called by the
// entry itself for an x wider than a word, so that a quantize of a
// quotient to cents makes one call where it made two, each with a frame (a
// 30- to 34-digit quotient to cents: 5.11 ns against 4.25, BID's
// quantizeSlow128 4.52). It reports false for a special operand and for a
// coefficient that is not canonical, which ubdQuantizeSlow128 sends on to
// ubdQuantizeLong128.
func ubdQuantizeWideTTE(x, y UBD128) (UBD128, bool) {
	xc := Uint128{Hi: x.Hi & UBDCoeffMaskHi, Lo: x.Lo}
	if x.Hi>>60 == UBDSpecialTag || y.Hi>>60 == UBDSpecialTag || u128GtMaxCoeff(xc) {
		return UBD128{}, false
	}
	switch gap := int(int64(x.Hi)>>UBDExpShift) - int(int64(y.Hi)>>UBDExpShift); {
	case gap == 0:
		return x, true
	case gap > 0:
		if gap >= Precision128 || !u128Lt(xc, pow10tab128[Precision128-gap]) {
			return UBD128{Hi: UBDNaNHi}, true // invalid: the coefficient would not fit
		}
		xc = u128MulLow(xc, pow10tab128[gap])
	case gap < -Precision128:
		xc = Uint128{}
	default:
		d := -gap
		t, _ := u128Add(xc, u128Shr(pow10tab128[d], 1))
		var r Uint128
		xc, r = divPow10Recip128(t, d)
		if r == (Uint128{}) {
			xc.Lo &^= 1 // a tie: the even neighbour is the one below
		}
	}
	// xc is below 10^34 and y's exponent is in the format.
	return UBD128{Lo: xc.Lo, Hi: y.Hi&^(UBDSignHi|UBDCoeffMaskHi) | x.Hi&UBDSignHi | xc.Hi}, true
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
// money values, and reports false for every other pair, which goes to
// ubdQuantizeSlow128 (also on UBD itself). A finer quantum multiplies by 10^gap (gap up to 19) unless
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
		// The index unsigned and bounded and the divisor tested make the
		// function a leaf with no frame: a bounds check and a division
		// check each gave it one, which cost a quantize to cents 0.4 ns.
		p := pow10tab64[min(uint(-gap), maxWordGap)]
		if p == 0 {
			return UBD128{}, false // never
		}
		q := c / p
		rem := c - q*p
		// The rounding is computed, not branched on: whether a remainder
		// passes half is close to a coin flip for money amounts, and as a
		// branch it mispredicted about half the time. On telco's quantizes
		// (Apple M3) this took the word path from 6.5 to 4.3 ns; a
		// reciprocal multiply in place of the divide was 3% slower. Only
		// the mode is branched on.
		half := p >> 1
		neg := b2u(x.Hi&UBDSignHi != 0)
		nz := b2u(rem != 0)
		var up uint64
		switch mode {
		case ToNearestAway:
			up = b2u(rem >= half)
		case TowardZero:
		case AwayFromZero:
			up = nz
		case TowardNegative:
			up = nz & neg
		case TowardPositive:
			up = nz &^ neg
		default:
			up = b2u(rem > half) | b2u(rem == half)&q
		}
		q += up & 1
		return UBD128{Lo: q, Hi: head}, true
	}
}

// The round-to-integral entries round x on UBD itself when it is finite
// with a coefficient of one word: an x with an exponent of zero or more is
// an integer already and is returned as it is; one with a fraction of 1 to
// 19 digits is x quantized to exponent zero (ubdQuantizeWord against 1E0):
// one division, rounding by the mode, the sign kept also on a zero. Every
// other x is ubdRoundToIntegral128's. The word path is written into the
// entries, so that ubdQuantizeWord, a leaf with no frame, is their one call
// (through a helper of its own it cost 3.71 ns against 3.45).

func ubdRoundToIntegralTTE128(x UBD128) UBD128 {
	if x.Hi&UBDCoeffMaskHi == 0 { // also every special: its prefix is in those bits
		if int64(x.Hi) >= 0 {
			return x // qExp >= 0: qExp's sign is the word's top bit
		}
		if r, ok := ubdQuantizeWord(x, UBD128{Lo: 1}, ToNearestEven); ok {
			return r
		}
	} else if x.Hi>>60 != UBDSpecialTag {
		// Wider than a word, finite: the same, by ubdQuantizeWideTTE, one
		// call where ubdRoundToIntegral128 made two (a 30- to 34-digit
		// quotient: 6.51 ns against 4.51, BID's roundToIntegral128 6.18).
		if int64(x.Hi) >= 0 {
			return x
		}
		if r, ok := ubdQuantizeWideTTE(x, UBD128{Lo: 1}); ok {
			return r
		}
	}
	return ubdRoundToIntegral128(x, ToNearestEven)
}

func ubdRoundToIntegralRnd128(x UBD128, mode Mode) UBD128 {
	if x.Hi&UBDCoeffMaskHi == 0 {
		if int64(x.Hi) >= 0 {
			return x
		}
		if r, ok := ubdQuantizeWord(x, UBD128{Lo: 1}, mode); ok {
			return r
		}
	}
	return ubdRoundToIntegral128(x, mode)
}

// ubdCompare128 orders x and y: this package's compare on UBD itself
// (ubdQuietCmp128, ubd_cmp.go). decimal128-go's compare was measured slower:
// 8.5 ns a compare against 3.3 on UBD.
func ubdCompare128(x, y UBD128) (c int, ok bool) {
	return ubdQuietCmp128(x, y)
}

// The predicate entries: each returns the operator's result itself, so
// that a call site tests one bool instead of decoding c and ok. Unordered
// operands (a NaN among them) satisfy none of them; x != y is
// !ubdEqual128(x, y). x > y and x >= y have entries of their own so that a
// call site passes its operands where they are, not swapped.

func ubdEqual128(x, y UBD128) bool {
	// Equal high words with no coefficient bits in them (the shift leaves
	// only those) are one sign and one exponent with one-word
	// coefficients, which no special has: with equal low words too, the
	// same value, as two map keys of one encoding are.
	if (x.Hi^y.Hi)|x.Hi<<(64-UBDSignShift)|(x.Lo^y.Lo) == 0 {
		return true
	}
	if (x.Hi|y.Hi)&UBDCoeffMaskHi == 0 {
		return ubdCmpWord(x, y) == 0
	}
	c, ok := ubdQuietCmp128(x, y)
	return ok && c == 0
}

func ubdLess128(x, y UBD128) bool {
	if (x.Hi|y.Hi)&UBDCoeffMaskHi == 0 {
		return ubdCmpWord(x, y) < 0
	}
	c, ok := ubdQuietCmp128(x, y)
	return ok && c < 0
}

func ubdLessEqual128(x, y UBD128) bool {
	if (x.Hi|y.Hi)&UBDCoeffMaskHi == 0 {
		return ubdCmpWord(x, y) <= 0
	}
	c, ok := ubdQuietCmp128(x, y)
	return ok && c <= 0
}

func ubdGreater128(x, y UBD128) bool {
	if (x.Hi|y.Hi)&UBDCoeffMaskHi == 0 {
		return ubdCmpWord(x, y) > 0
	}
	c, ok := ubdQuietCmp128(x, y)
	return ok && c > 0
}

func ubdGreaterEqual128(x, y UBD128) bool {
	if (x.Hi|y.Hi)&UBDCoeffMaskHi == 0 {
		return ubdCmpWord(x, y) >= 0
	}
	c, ok := ubdQuietCmp128(x, y)
	return ok && c >= 0
}
