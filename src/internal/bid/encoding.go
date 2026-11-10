// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// notImplemented is the panic value of every operation that has been
// declared but not yet implemented. It is temporary: when the package is
// complete no function panics on any input.
const notImplemented = "internal/bid: not implemented"

// Uint128 is a 128-bit unsigned integer, most significant word first.
//
// It carries decimal128 bit patterns, as uint64 carries decimal64 bit
// patterns, and it carries 128-bit coefficients, as uint64 carries 64-bit
// coefficients. Two Uint128 values are equal as bit patterns exactly when
// they are equal with ==. The type is a value, not a memory image; it says
// nothing about the byte order of a decimal128 in memory.
type Uint128 struct {
	Hi, Lo uint64
}

// Format parameters of the two interchange formats (IEEE 754-2019 clause
// 3.3). A finite value is (-1)^s × c × 10^q with the coefficient
// c an integer of at most Precision digits and the quantum exponent q in
// [MinExp, MaxExp]. Emax and Emin bound the adjusted exponent of a normal
// value written d.ddd × 10^e; MinExp is the exponent of the smallest
// subnormal (the standard's etiny).
const (
	Precision64 = 16
	Emax64      = 384
	Emin64      = 1 - Emax64
	MaxExp64    = Emax64 - (Precision64 - 1) // 369
	MinExp64    = Emin64 - (Precision64 - 1) // -398
	Bias64      = -MinExp64                  // 398

	Precision128 = 34
	Emax128      = 6144
	Emin128      = 1 - Emax128
	MaxExp128    = Emax128 - (Precision128 - 1) // 6111
	MinExp128    = Emin128 - (Precision128 - 1) // -6176
	Bias128      = -MinExp128                   // 6176
)

// MaxCoeff64 is the largest decimal64 coefficient, 10^16 - 1. A finite
// encoding whose coefficient field exceeds it is non-canonical and reads
// as zero.
const MaxCoeff64 uint64 = 9999999999999999

// MaxCoeff128Hi and MaxCoeff128Lo are the words of the largest decimal128
// coefficient, 10^34 - 1. It is below 2^113, so every canonical decimal128
// coefficient fits the 113-bit coefficient field of the first encoding
// form and every second-form decimal128 encoding is non-canonical. (A
// Uint128 cannot be a constant; the package has no variables.)
const (
	MaxCoeff128Hi uint64 = 0x0001ED09BEAD87C0
	MaxCoeff128Lo uint64 = 0x378D8E63FFFFFFFF
)

// Field layout of the BID encodings (IEEE 754-2019 clause 3.5.2). The
// first form holds a coefficient below 2^CoeffBits with the exponent field
// starting at bit CoeffBits. The second form, marked by the two bits below
// the sign being 11, holds a coefficient with an implicit leading 100 in
// binary: its low Form2CoeffBits bits are stored and its exponent field
// starts at bit Form2CoeffBits. Infinity and NaN set the top four bits
// below the sign (SpecialMask); bit 58 (or bit 122) distinguishes NaN
// from infinity, and the bit below that marks a signaling NaN. For
// decimal128 the fields named here are those of the high word, Hi; the
// low word holds the low 64 coefficient bits.
const (
	SignMask64       uint64 = 1 << 63
	ExpBits64               = 10
	CoeffBits64             = 53 // first form: 53 explicit coefficient bits
	Form2CoeffBits64        = 51 // second form: 51 explicit bits below an implicit 100
	ExpShift64              = CoeffBits64
	Form2ExpShift64         = Form2CoeffBits64
	ExpMask64        uint64 = 1<<ExpBits64 - 1
	Form2Mask64      uint64 = 3 << 61
	SpecialMask64    uint64 = 0xF << 59 // infinity or NaN when all four bits set
	NaNBit64         uint64 = 1 << 58
	SignalingBit64   uint64 = 1 << 57

	SignMask128Hi       uint64 = 1 << 63
	ExpBits128                 = 14
	CoeffBits128               = 113 // 49 bits of Hi and all 64 of Lo
	Form2CoeffBits128          = 111
	ExpShift128                = CoeffBits128 - 64 // 49, within Hi
	Form2ExpShift128           = Form2CoeffBits128 - 64
	ExpMask128          uint64 = 1<<ExpBits128 - 1
	Form2Mask128Hi      uint64 = 3 << 61
	SpecialMask128Hi    uint64 = 0xF << 59
	NaNBit128Hi         uint64 = 1 << 58
	SignalingBit128Hi   uint64 = 1 << 57
	CoeffMask128Hi      uint64 = 1<<ExpShift128 - 1
	Form2CoeffMask128Hi uint64 = 1<<Form2ExpShift128 - 1
)

// Canonical encodings of the special values and of +0 with exponent 0.
// The decimal128 constants give the high word; the low word is zero.
// The memory form of a value is its encoding XORed with Zero64 or Zero128Hi,
// so that all bits clear means 0E+0. Neither has the sign bit set.
// Negative infinity is Inf64 | SignMask64.
const (
	Inf64  uint64 = SpecialMask64
	NaN64  uint64 = SpecialMask64 | NaNBit64
	SNaN64 uint64 = SpecialMask64 | NaNBit64 | SignalingBit64
	Zero64 uint64 = Bias64 << ExpShift64

	Inf128Hi  uint64 = SpecialMask128Hi
	NaN128Hi  uint64 = SpecialMask128Hi | NaNBit128Hi
	SNaN128Hi uint64 = SpecialMask128Hi | NaNBit128Hi | SignalingBit128Hi
	Zero128Hi uint64 = Bias128 << ExpShift128
)

// Kind classifies a decimal value as finite, infinite or NaN. Zero is
// finite; a signaling NaN is a NaN.
type Kind uint8

const (
	KindFinite Kind = iota
	KindInf
	KindNaN
)

// Unpack64 decodes the fields of x (IEEE 754-2019 clause 3.5.2) into its
// kind, sign, coefficient and exponent. For a finite value
// x = (-1)^neg × coeff × 10^exp with coeff ≤ MaxCoeff64 and exp in
// [MinExp64, MaxExp64]; a non-canonical coefficient reads as zero. For an
// infinity or a NaN, coeff and exp are zero; neg is the sign bit, which
// for a NaN carries no meaning.
func Unpack64(x uint64) (kind Kind, neg bool, coeff uint64, exp int) {
	if neg, coeff, exp, ok := unpackFirst64(x); ok {
		return KindFinite, neg, coeff, exp
	}
	return unpackRest64(x)
}

// Unpack128 is Unpack64 for decimal128.
func Unpack128(x Uint128) (kind Kind, neg bool, coeff Uint128, exp int) {
	if neg, coeff, exp, ok := unpackFirst128(x); ok {
		return KindFinite, neg, coeff, exp
	}
	return unpackRest128(x)
}

// Unpack64 and Unpack128 cannot inline: a call returning four results
// costs nearly the whole inlining budget by itself. They are therefore
// split into unpackFirst64 and unpackFirst128, which decode the common
// case and inline, and unpackRest64 and unpackRest128, which decode the
// rest and are calls. An operation that unpacks on its hot path calls the
// two halves itself.
//
// unpackFirst64 and unpackFirst128 decode a finite first-form encoding whose
// coefficient is canonical by its form alone, which Unpack would return
// as it is, and report ok = false for every other encoding. A decimal64
// first-form coefficient is below 2^53, which is below MaxCoeff64; a
// decimal128 one is canonical whenever its high word is below that of
// MaxCoeff128, which leaves out only the coefficients from
// MaxCoeff128Hi × 2^64 up. An exponent field of the first form is below
// 3 × 2^(ExpBits-2) and therefore always in range.
func unpackFirst64(x uint64) (neg bool, coeff uint64, exp int, ok bool) {
	if x&Form2Mask64 == Form2Mask64 {
		return
	}
	return x&SignMask64 != 0, x & (1<<CoeffBits64 - 1), int(x>>ExpShift64&ExpMask64) - Bias64, true
}

func unpackFirst128(x Uint128) (neg bool, coeff Uint128, exp int, ok bool) {
	t := x.Hi
	if t&Form2Mask128Hi == Form2Mask128Hi || t&CoeffMask128Hi >= MaxCoeff128Hi {
		return
	}
	return t&SignMask128Hi != 0, Uint128{Hi: t & CoeffMask128Hi, Lo: x.Lo}, int(t>>ExpShift128&ExpMask128) - Bias128, true
}

// unpackRest64 is Unpack64 for every encoding, written for those that
// unpackFirst64 does not decode: the second form and the special values.
//
//go:noinline
func unpackRest64(x uint64) (kind Kind, neg bool, coeff uint64, exp int) {
	neg = x&SignMask64 != 0
	if x&SpecialMask64 == SpecialMask64 {
		if x&NaNBit64 == 0 {
			return KindInf, neg, 0, 0
		}
		return KindNaN, neg, 0, 0
	}
	var biased uint64
	if x&Form2Mask64 == Form2Mask64 {
		biased = x >> Form2ExpShift64 & ExpMask64
		coeff = 1<<53 | x&(1<<Form2CoeffBits64-1)
	} else {
		biased = x >> ExpShift64 & ExpMask64
		coeff = x & (1<<CoeffBits64 - 1)
	}
	if coeff > MaxCoeff64 {
		coeff = 0
	}
	return KindFinite, neg, coeff, int(biased) - Bias64
}

// unpackRest128 is Unpack128 for every encoding, written for those that
// unpackFirst128 does not decode: the largest first-form coefficients,
// canonical or not, the second form and the special values.
//
//go:noinline
func unpackRest128(x Uint128) (kind Kind, neg bool, coeff Uint128, exp int) {
	t := x.Hi
	neg = t>>63 != 0
	if t&Form2Mask128Hi != Form2Mask128Hi {
		coeff = Uint128{Hi: t & CoeffMask128Hi, Lo: x.Lo}
		if u128GtMaxCoeff(coeff) {
			coeff = Uint128{}
		}
		return KindFinite, neg, coeff, int(t>>ExpShift128&ExpMask128) - Bias128
	}
	if t&SpecialMask128Hi == SpecialMask128Hi {
		if t&NaNBit128Hi == 0 {
			return KindInf, neg, Uint128{}, 0
		}
		return KindNaN, neg, Uint128{}, 0
	}
	// Second-form decimal128 coefficients are always ≥ 2^113 > 10^34-1.
	return KindFinite, neg, Uint128{}, int(t>>Form2ExpShift128&ExpMask128) - Bias128
}

// unpackWord64 and unpackWord128 recognize first-form finite encodings whose
// coefficients fit uint64. Such coefficients are canonical: below 2^53 for
// decimal64 and below 2^64 for decimal128. The returned exponent is biased;
// callers compare exponents from the same width, so the bias cancels.
// Other encodings return ok = false and need the general decoder.
func unpackWord64(x uint64) (neg bool, coeff uint64, exp int, ok bool) {
	if x&Form2Mask64 == Form2Mask64 {
		return
	}
	return x&SignMask64 != 0, x & (1<<CoeffBits64 - 1), int(x >> ExpShift64 & ExpMask64), true
}

func unpackWord128(x Uint128) (neg bool, coeff uint64, exp int, ok bool) {
	if x.Hi&CoeffMask128Hi != 0 || x.Hi&Form2Mask128Hi == Form2Mask128Hi {
		return
	}
	return x.Hi&SignMask128Hi != 0, x.Lo, int(x.Hi >> ExpShift128 & ExpMask128), true
}

// wordPair128 decodes what a word path of two operands needs. ok reports
// that x and y are both finite encodings of the first form whose
// coefficients fit their low words, which are then the coefficients: no
// coefficient bit is set in either high word, and neither has both bits of
// the second form and the special values, which x & x<<1 brings together in
// the upper of the two. exp is the exponent of x less that of y, and neg
// whether their signs differ.
func wordPair128(x, y Uint128) (exp int, neg, ok bool) {
	ok = (x.Hi|y.Hi)&CoeffMask128Hi == 0 && (x.Hi&(x.Hi<<1)|y.Hi&(y.Hi<<1))&(Form2Mask128Hi&(Form2Mask128Hi<<1)) == 0
	exp = int(x.Hi>>ExpShift128&ExpMask128) - int(y.Hi>>ExpShift128&ExpMask128)
	return exp, (x.Hi^y.Hi)&SignMask128Hi != 0, ok
}

// shortExp128 returns the exponent of x less shortMinExp, a value from 0
// to -shortMinExp, if x is a finite first-form encoding whose coefficient
// fits its low word and whose exponent is from shortMinExp to 0; for every
// other encoding it returns a larger value. It is one comparison for the
// caller: without its sign, and less the high word of 0E-6, the high word
// is k << ExpShift128 with k from 0 to 6 exactly when the exponent is
// -6 + k and every coefficient bit of the word is clear, and rotated, k is
// the whole value. Every other finite encoding, and every special one,
// leaves a set bit above it.
func shortExp128(x Uint128) uint64 {
	return bits.RotateLeft64(x.Hi&^SignMask128Hi-(Bias128+shortMinExp)<<ExpShift128, 64-ExpShift128)
}

// shortParts128 returns the sign of a short x as 0 or 1 and the number of
// its digits after the point, the negation of its exponent: the fields
// PutShort128 lays out, with the coefficient in the low word.
func shortParts128(x Uint128) (sign, frac uint) {
	return uint(x.Hi >> 63), uint(Bias128 - int(x.Hi>>ExpShift128&ExpMask128))
}

// unpackWordCanon64 and unpackWordCanon128 recognize every finite canonical
// encoding whose coefficient fits uint64, for the word fast paths of the
// arithmetic: for decimal64 that is every finite canonical encoding, the
// second form included, whose coefficient is tested against MaxCoeff64 as
// Unpack64 tests it; for decimal128 it is what unpackWord128 recognizes,
// because a decimal128 second form is never canonical. The exponent is
// biased, as unpackWord's, and the other results are unspecified when ok is
// false. The comparisons keep unpackWord: there the second form measured
// faster on the general path.
func unpackWordCanon64(x uint64) (neg bool, coeff uint64, exp int, ok bool) {
	neg = x&SignMask64 != 0
	if x&Form2Mask64 != Form2Mask64 {
		return neg, x & (1<<CoeffBits64 - 1), int(x >> ExpShift64 & ExpMask64), true
	}
	coeff = 1<<53 | x&(1<<Form2CoeffBits64-1)
	return neg, coeff, int(x >> Form2ExpShift64 & ExpMask64), x&SpecialMask64 != SpecialMask64 && coeff <= MaxCoeff64
}

func unpackWordCanon128(x Uint128) (neg bool, coeff uint64, exp int, ok bool) {
	return unpackWord128(x)
}

// packWord64 and packWord128 encode the exact value (-1)^neg × (hi·2^64 +
// lo) × 10^exp, which a word fast path computed at its preferred exponent,
// if it fits the format, and report ok = false otherwise, when the caller
// takes its general path. They are the exact packing exception of the word
// fast paths (see roundW in arith_tmpl.go): the rounding routine would
// encode the value unchanged, and nothing is rounded here.
// TestRoundIsTheOnlyPacker lists their callers.
func packWord64(neg bool, hi, lo uint64, exp int) (uint64, bool) {
	if hi != 0 || lo > MaxCoeff64 || uint(exp-MinExp64) > MaxExp64-MinExp64 {
		return 0, false
	}
	x := uint64(exp+Bias64)<<ExpShift64 | lo
	if lo >= 1<<CoeffBits64 {
		x = Form2Mask64 | uint64(exp+Bias64)<<Form2ExpShift64 | lo&(1<<Form2CoeffBits64-1)
	}
	if neg {
		x |= SignMask64
	}
	return x, true
}

func packWord128(neg bool, hi, lo uint64, exp int) (Uint128, bool) {
	if (hi > MaxCoeff128Hi || hi == MaxCoeff128Hi && lo > MaxCoeff128Lo) || uint(exp-MinExp128) > MaxExp128-MinExp128 {
		return Uint128{}, false
	}
	hi |= uint64(exp+Bias128) << ExpShift128
	if neg {
		hi |= SignMask128Hi
	}
	return Uint128{Hi: hi, Lo: lo}, true
}

// The inline paths. The functions below decide, from the bits alone, the
// operations of money arithmetic whose result is exact and needs no
// decoding: operands of one quantum whose coefficients fit a word. The
// arithmetic templates call them in front of the general paths, and they are
// small enough that the front and its call to the general path inline into
// every caller (see "Inline fast paths" in the package documentation). Each
// reports ok = false for every operand it does not decide, and then the
// general path gives the result; where it decides, the general path gives the
// same encoding, as each says. TestRoundIsTheOnlyPacker lists their callers.
//
// They recognize a word operand by bit 62 clear, which is a first-form finite
// encoding with a biased exponent below 2^(ExpBits-1), together with a zero
// high coefficient field for decimal128. That leaves the largest exponents,
// above 2^(ExpBits-1) - Bias (113 for decimal64, 2015 for decimal128), to the
// general path, and saves a test of the second form.
const inlineWordBit = 1 << 62

// addSameQuantum64 and addSameQuantum128 return x + y for word operands of
// one sign and one exponent: the sum, if its coefficient fits the first form
// (always, for decimal128: two words sum below 2^65), with that sign and
// exponent, the preferred one. The general path computes the same exact sum
// at the same exponent in its word fast path and packWord encodes it the same
// way, a first-form coefficient below 2^53 for decimal64.
func addSameQuantum64(x, y uint64) (uint64, bool) {
	s := x + y&(1<<CoeffBits64-1)
	return s, ((x^y)|(s^x))>>ExpShift64|x&inlineWordBit == 0
}

func addSameQuantum128(x, y Uint128) (Uint128, bool) {
	lo, carry := bits.Add64(x.Lo, y.Lo, 0)
	return Uint128{Hi: x.Hi + carry, Lo: lo}, (x.Hi^y.Hi)|x.Hi&(CoeffMask128Hi|inlineWordBit) == 0
}

// subSameQuantum64 and subSameQuantum128 return x - y for word operands of
// one sign and one exponent whose coefficients differ: the difference of the
// coefficients, with the sign of x if its coefficient is the larger and the
// other sign if not, at that exponent, the preferred one. The general path
// adds x to y negated, operands of opposite signs, and its word fast path
// gives the same coefficient, sign and exponent, which packWord encodes the
// same way. Equal coefficients cancel to a zero whose sign depends on the
// mode, and are left to the general path.
func subSameQuantum64(x, y uint64) (uint64, bool) {
	const m = 1<<CoeffBits64 - 1
	d, borrow := bits.Sub64(x&m, y&m, 0)
	return x&^m ^ borrow<<63 | (d ^ -borrow + borrow), ((x^y)|x&inlineWordBit)>>ExpShift64|(d-1)&^d>>63 == 0
}

func subSameQuantum128(x, y Uint128) (Uint128, bool) {
	d, borrow := bits.Sub64(x.Lo, y.Lo, 0)
	return Uint128{Hi: x.Hi ^ borrow<<63, Lo: d ^ -borrow + borrow}, (x.Hi^y.Hi)|x.Hi&(CoeffMask128Hi|inlineWordBit)|(d-1)&^d>>63 == 0
}

// mulShort64 and mulShort128 return x × y for word operands whose product is
// below 2^53 (decimal64) or 2^96 (decimal128) at a biased exponent below
// 2^(ExpBits-1), which is then exact and in the first form: the general
// path's word fast path computes the same product at the same preferred
// exponent, the sum of the exponents, and packWord encodes it the same way.
// The sign is the exclusive or of the signs, a zero product's included. The
// biased exponent of the product is xe + ye - Bias, which the unsigned
// comparison also refuses when it is negative.
func mulShort64(x, y uint64) (uint64, bool) {
	const m = 1<<CoeffBits64 - 1
	hi, lo := bits.Mul64(x&m, y&m)
	t := x&^m + y&^m - Bias64<<ExpShift64
	return t | lo, (x|y|t&^SignMask64)&inlineWordBit|hi|lo>>CoeffBits64 == 0
}

func mulShort128(x, y Uint128) (Uint128, bool) {
	hi, lo := bits.Mul64(x.Lo, y.Lo)
	t := x.Hi + y.Hi - Bias128<<ExpShift128
	return Uint128{Hi: t | hi, Lo: lo}, (x.Hi|y.Hi)&(CoeffMask128Hi|inlineWordBit)|(t&^SignMask128Hi)&inlineWordBit|hi>>32 == 0
}

// quantizeSame64 and quantizeSame128 report whether x is a word operand with
// the exponent of y, whose bits then agree with those of x from bit 62 down
// to the exponent field, so that y is finite too. Quantizing x to y gives x
// itself: the general path's word fast path scales the coefficient by 10^0
// and packWord encodes the same bits.
func quantizeSame64(x, y uint64) bool {
	return ((x^y)&^SignMask64|x&inlineWordBit)>>ExpShift64 == 0
}

func quantizeSame128(x, y Uint128) bool {
	return (x.Hi^y.Hi)&^SignMask128Hi>>ExpShift128|x.Hi&(CoeffMask128Hi|inlineWordBit) == 0
}

// sameQuantumKeys64 and sameQuantumKeys128 report whether x and y are word
// operands of one sign and one exponent, and return keys for them, words
// whose unsigned order is the order of the values: the coefficients, with
// every bit inverted for negative operands, which reverses their order. Zeros
// of one sign have equal keys, and are equal on the general path too.
func sameQuantumKeys64(x, y uint64) (kx, ky uint64, ok bool) {
	m := -(x >> 63)
	return x ^ m, y ^ m, ((x^y)|x&inlineWordBit)>>ExpShift64 == 0
}

func sameQuantumKeys128(x, y Uint128) (kx, ky uint64, ok bool) {
	m := -(x.Hi >> 63)
	return x.Lo ^ m, y.Lo ^ m, (x.Hi^y.Hi)|x.Hi&(CoeffMask128Hi|inlineWordBit) == 0
}

// kindExp64 and kindExp128 decode only the fields that quantum operations
// use. A coefficient, canonical or otherwise, cannot change these fields.
// Keeping that work separate lets these small decoders inline into callers
// that do not need Unpack's sign and coefficient results.
func kindExp64(x uint64) (Kind, int) {
	if x&Form2Mask64 != Form2Mask64 {
		return KindFinite, int(x>>ExpShift64&ExpMask64) - Bias64
	}
	if x&SpecialMask64 == SpecialMask64 {
		if x&NaNBit64 != 0 {
			return KindNaN, 0
		}
		return KindInf, 0
	}
	return KindFinite, int(x>>Form2ExpShift64&ExpMask64) - Bias64
}

func kindExp128(x Uint128) (Kind, int) {
	t := x.Hi
	if t&Form2Mask128Hi != Form2Mask128Hi {
		return KindFinite, int(t>>ExpShift128&ExpMask128) - Bias128
	}
	if t&SpecialMask128Hi == SpecialMask128Hi {
		if t&NaNBit128Hi != 0 {
			return KindNaN, 0
		}
		return KindInf, 0
	}
	return KindFinite, int(t>>Form2ExpShift128&ExpMask128) - Bias128
}

// Pack64 encodes the finite value (-1)^neg × coeff × 10^exp exactly in
// the canonical first or second form (IEEE 754-2019 clause 3.5.2). It
// reports ok = false and returns NaN64 when coeff exceeds MaxCoeff64 or exp
// lies outside [MinExp64, MaxExp64]; it never rounds. To encode a value
// that may need rounding or exponent adjustment, use Round64. Pack64 is
// the inverse of Unpack64 on finite values.
func Pack64(neg bool, coeff uint64, exp int) (x uint64, ok bool) {
	if coeff > MaxCoeff64 || exp < MinExp64 || exp > MaxExp64 {
		return NaN64, false
	}
	sign := uint64(0)
	if neg {
		sign = SignMask64
	}
	biased := uint64(exp + Bias64)
	if coeff < 1<<CoeffBits64 {
		return sign | biased<<ExpShift64 | coeff, true
	}
	return sign | Form2Mask64 | biased<<Form2ExpShift64 | coeff&(1<<Form2CoeffBits64-1), true
}

// Pack128 is Pack64 for decimal128.
func Pack128(neg bool, coeff Uint128, exp int) (x Uint128, ok bool) {
	if u128GtMaxCoeff(coeff) || exp < MinExp128 || exp > MaxExp128 {
		return Uint128{Hi: NaN128Hi}, false
	}
	// The coefficient bound above also proves that its high word fits
	// CoeffMask128Hi, so no second mask is needed.
	hi := uint64(exp+Bias128)<<ExpShift128 | coeff.Hi
	if neg {
		hi |= SignMask128Hi
	}
	return Uint128{
		Hi: hi,
		Lo: coeff.Lo,
	}, true
}

// IsNaN64 reports whether x is a NaN, quiet or signaling (IEEE 754-2019
// clause 5.7.2, isNaN).
func IsNaN64(x uint64) bool {
	return x&SpecialMask64 == SpecialMask64 && x&NaNBit64 != 0
}

// IsNaN128 is IsNaN64 for decimal128.
func IsNaN128(x Uint128) bool {
	return IsNaN64(x.Hi)
}

// IsSignaling64 reports whether x is a signaling NaN (IEEE 754-2019 clause
// 5.7.2, isSignaling).
func IsSignaling64(x uint64) bool {
	return IsNaN64(x) && x&SignalingBit64 != 0
}

// IsSignaling128 is IsSignaling64 for decimal128.
func IsSignaling128(x Uint128) bool {
	return IsSignaling64(x.Hi)
}

// IsInf64 reports whether x is an infinity of either sign (IEEE 754-2019
// clause 5.7.2, isInfinite).
func IsInf64(x uint64) bool {
	return x&SpecialMask64 == SpecialMask64 && x&NaNBit64 == 0
}

// IsInf128 is IsInf64 for decimal128.
func IsInf128(x Uint128) bool {
	return IsInf64(x.Hi)
}

// IsFinite64 reports whether x is neither an infinity nor a NaN
// (IEEE 754-2019 clause 5.7.2, isFinite).
func IsFinite64(x uint64) bool {
	return x&SpecialMask64 != SpecialMask64
}

// IsFinite128 is IsFinite64 for decimal128.
func IsFinite128(x Uint128) bool {
	return IsFinite64(x.Hi)
}

// IsZero64 reports whether x is a zero of either sign, including a
// non-canonical encoding that reads as zero (IEEE 754-2019 clause 5.7.2,
// isZero).
func IsZero64(x uint64) bool {
	k, _, c, _ := Unpack64(x)
	return k == KindFinite && c == 0
}

// IsZero128 is IsZero64 for decimal128.
func IsZero128(x Uint128) bool {
	k, _, c, _ := Unpack128(x)
	return k == KindFinite && c.Hi == 0 && c.Lo == 0
}

// IsSubnormal64 reports whether x is finite, non-zero and below the
// smallest normal magnitude 10^Emin64 (IEEE 754-2019 clause 5.7.2,
// isSubnormal).
func IsSubnormal64(x uint64) bool {
	k, _, c, exp := Unpack64(x)
	if k != KindFinite || c == 0 {
		return false
	}
	return digits64(c)-1+exp < Emin64
}

// IsSubnormal128 is IsSubnormal64 for decimal128.
func IsSubnormal128(x Uint128) bool {
	k, _, c, exp := Unpack128(x)
	if k != KindFinite || (c.Hi|c.Lo) == 0 {
		return false
	}
	return cDigits128(c)-1+exp < Emin128
}

// IsCanonical64 reports whether x is a canonical encoding (IEEE 754-2019
// clause 5.7.2, isCanonical): a finite value whose coefficient field is at
// most MaxCoeff64, an infinity with zero reserved and coefficient bits, or
// a NaN with zero reserved bits and a payload below 10^(Precision64-1).
// Every result of this package is canonical except those of the sign
// operations Neg64, Abs64 and CopySign64, which copy the bits of their
// operand (see "Sign operations" in the package documentation).
func IsCanonical64(x uint64) bool {
	if x>>59&0xF == 0xF {
		if x>>58&1 == 0 {
			return x&(1<<58-1) == 0
		}
		if x>>50&(1<<7-1) != 0 {
			return false
		}
		return x&nanPayloadMask64 < maxNaNPayload64
	}
	if x>>61&3 == 3 {
		return 1<<53|x&(1<<Form2CoeffBits64-1) <= MaxCoeff64
	}
	return x&(1<<CoeffBits64-1) <= MaxCoeff64
}

// IsCanonical128 is IsCanonical64 for decimal128.
func IsCanonical128(x Uint128) bool {
	t := x.Hi
	if t>>59&0xF == 0xF {
		if t>>58&1 == 0 {
			return t&(1<<58-1) == 0 && x.Lo == 0
		}
		if t>>46&(1<<11-1) != 0 {
			return false
		}
		payload := Uint128{Hi: t & nanPayloadMask128Hi, Lo: x.Lo}
		return u128Lt(payload, maxNaNPayload128)
	}
	if t>>61&3 == 3 {
		return false
	}
	c := Uint128{Hi: t & CoeffMask128Hi, Lo: x.Lo}
	return !u128GtMaxCoeff(c)
}

// Signbit64 reports whether the sign bit of x is set (IEEE 754-2019 clause
// 5.7.2, isSignMinus). It is true for -0 and for a NaN whose sign bit is
// set.
func Signbit64(x uint64) bool {
	return x&SignMask64 != 0
}

// Signbit128 is Signbit64 for decimal128.
func Signbit128(x Uint128) bool {
	return Signbit64(x.Hi)
}

// Payload64 returns the payload of the NaN x as a decimal64 (IEEE 754-2019
// clause 9.7, getPayload): the integer in its trailing significand field,
// with exponent 0 and a positive sign, whatever the sign and signaling bit
// of x; a non-canonical payload, 10^15 or more, reads as 0. For an x that
// is not a NaN the result is -1 with exponent 0, which clause 9.7
// recommends and which no NaN gives (see "NaN results" in the package
// documentation).
func Payload64(x uint64) uint64 {
	return payload64(x)
}

// Payload128 is Payload64 for decimal128; a payload of 10^33 or more reads
// as 0.
func Payload128(x Uint128) Uint128 {
	return payload128(x)
}

// NaNWithPayload64 returns the positive quiet NaN whose payload is the value
// of p (IEEE 754-2019 clause 9.7, setPayload) when that value is an integer
// in [0, 10^15), at any exponent and either sign of zero: 42, 4.20E+1 and
// 42.000 all give payload 42. For any other p, including an infinity, a NaN,
// a fraction, a negative value and 10^15 or more, it returns +0 with
// exponent 0. Payload64(NaNWithPayload64(p)) is p reduced to exponent 0 for
// every canonical payload p.
func NaNWithPayload64(p uint64) uint64 {
	return nanWithPayload64(p, false)
}

// NaNWithPayload128 is NaNWithPayload64 for decimal128, whose canonical
// payloads are the integers in [0, 10^33).
func NaNWithPayload128(p Uint128) Uint128 {
	return nanWithPayload128(p, false)
}

// SignalingNaNWithPayload64 is NaNWithPayload64 with a signaling NaN as its
// result (IEEE 754-2019 clause 9.7, setPayloadSignaling). It is, with the
// sign operations, the only function of the package that returns a
// signaling NaN.
func SignalingNaNWithPayload64(p uint64) uint64 {
	return nanWithPayload64(p, true)
}

// SignalingNaNWithPayload128 is SignalingNaNWithPayload64 for decimal128.
func SignalingNaNWithPayload128(p Uint128) Uint128 {
	return nanWithPayload128(p, true)
}

// Neg64 returns x with its sign bit inverted (IEEE 754-2019 clause 5.5.1,
// negate). It is a bit operation, as are Abs64 and CopySign64: it does not
// canonicalize and it does not quiet a NaN (see "Sign operations" in the
// package documentation).
func Neg64(x uint64) uint64 {
	return x ^ SignMask64
}

// Neg128 is Neg64 for decimal128.
func Neg128(x Uint128) Uint128 {
	return Uint128{Hi: x.Hi ^ SignMask128Hi, Lo: x.Lo}
}

// Abs64 returns x with its sign bit cleared (IEEE 754-2019 clause 5.5.1,
// abs). It is a bit operation.
func Abs64(x uint64) uint64 {
	return x &^ SignMask64
}

// Abs128 is Abs64 for decimal128.
func Abs128(x Uint128) Uint128 {
	return Uint128{Hi: x.Hi &^ SignMask128Hi, Lo: x.Lo}
}

// CopySign64 returns x with the sign bit of sign (IEEE 754-2019 clause
// 5.5.1, copySign). It is a bit operation.
func CopySign64(x, sign uint64) uint64 {
	return x&^SignMask64 | sign&SignMask64
}

// CopySign128 is CopySign64 for decimal128.
func CopySign128(x, sign Uint128) Uint128 {
	return Uint128{Hi: x.Hi&^SignMask128Hi | sign.Hi&SignMask128Hi, Lo: x.Lo}
}

const maxNaNPayload64 = 1_000_000_000_000_000 // 10^15

// The trailing significand field of a NaN, which holds its payload: the
// low 50 bits of a decimal64, and the low 46 bits of the high word of a
// decimal128 with all of its low word (IEEE 754-2019 clause 3.5.2).
const (
	nanPayloadMask64    uint64 = 1<<50 - 1
	nanPayloadMask128Hi uint64 = 1<<46 - 1
)

var maxNaNPayload128 = Uint128{Hi: 0x0000314DC6448D93, Lo: 0x38C15B0A00000000} // 10^33

// digits64 returns the number of decimal digits of c, 0 for zero.
func digits64(c uint64) int {
	return cDigits64(c)
}

// digits128 returns the number of decimal digits of c, 0 for zero.
func digits128(c Uint128) int {
	return cDigits128(c)
}
