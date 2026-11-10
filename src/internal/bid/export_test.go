// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// Export the unexported names the tests use, in the style of the runtime's
// export_test.go. All tests of this package live in package bid_test, because
// internal/bid is imported by math, strconv and runtime, and so by every
// package that testing and its helpers depend on: a test in package bid that
// imported any of them would be an import cycle. TestNoInternalTests enforces
// this. This file may import only packages that do not depend on internal/bid.

// Types.
type (
	DigitClass = digitClass
	FloatNat   = floatNat
)

// Constants.
const (
	ClassAboveHalf      = classAboveHalf
	ClassBelowHalf      = classBelowHalf
	ClassHalf           = classHalf
	ClassZero           = classZero
	ErrQuotientOverflow = errQuotientOverflow
	Pow5Step            = pow5Step
)

// Tables.
var (
	EightASCII      = eightASCII
	WriteDigits     = writeDigits
	WriteWordDigits = writeWordDigits
	Bin2DPD         = bin2dpd
	DPD2Bin         = dpd2bin
	Pow10tab128     = pow10tab128
	Pow10tab256     = pow10tab256
	Pow10tab64      = pow10tab64
)

// Functions.
var (
	CAdd128         = cAdd128
	CAdd64          = cAdd64
	CCmp128         = cCmp128
	CCmp64          = cCmp64
	CDigits128      = cDigits128
	CDigits64       = cDigits64
	CDivPow10128    = cDivPow10128
	CDivPow1064     = cDivPow1064
	CGtMax128       = cGtMax128
	CGtMax64        = cGtMax64
	CIsZero128      = cIsZero128
	CIsZero64       = cIsZero64
	CLenFull128     = cLenFull128
	CLenFull64      = cLenFull64
	CModFull128     = cModFull128
	CModFull64      = cModFull64
	CMulFull128     = cMulFull128
	CMulFull64      = cMulFull64
	CMulPow10128    = cMulPow10128
	CMulPow1064     = cMulPow1064
	COdd128         = cOdd128
	COdd64          = cOdd64
	CPow10128       = cPow10128
	CPow1064        = cPow1064
	CReduce128      = cReduce128
	CReduce64       = cReduce64
	CReduceClass128 = cReduceClass128
	CReduceClass64  = cReduceClass64
	CScale128       = cScale128
	CScale64        = cScale64
	CScaleFull128   = cScaleFull128
	CScaleFull64    = cScaleFull64
	CShift128       = cShift128
	CShift64        = cShift64
	CShiftFull128   = cShiftFull128
	CShiftFull64    = cShiftFull64
	CSub128         = cSub128
	CSub64          = cSub64
	Digits128       = digits128
	Log10Pow2       = log10Pow2
	Pow5            = pow5
	Reciprocal3by2  = reciprocal3by2
	DivPow10Recip64 = divPow10Recip64
	U128Add         = u128Add
	U128Cmp         = u128Cmp
	U128Div         = u128Div
	U128Div64       = u128Div64
	U128GePow10     = u128GePow10
	U128Len         = u128Len
	U128Lt          = u128Lt
	U128Mul         = u128Mul
	U128MulFits     = u128MulFits
	U128Mul64       = u128Mul64
	U128Shl         = u128Shl
	U128Shr         = u128Shr
	U128Sub         = u128Sub
	U256Add         = u256Add
	U256Cmp         = u256Cmp
	U256Div128      = u256Div128
	U256DivMod128   = u256DivMod128
	U256DivPow10    = u256DivPow10
	U256Len         = u256Len
	U256Lt          = u256Lt
	U256Shl         = u256Shl
	U256Shr         = u256Shr
	U256Sub         = u256Sub
)

// The inline fast paths and the general paths behind them.
var (
	AddSameQuantum128  = addSameQuantum128
	AddSameQuantum64   = addSameQuantum64
	AddSlow64          = addSlow64
	MulShort128        = mulShort128
	MulShort64         = mulShort64
	MulSlow64          = mulSlow64
	QuantizeSame128    = quantizeSame128
	QuantizeSame64     = quantizeSame64
	QuantizeSlow128    = quantizeSlow128
	QuantizeSlow64     = quantizeSlow64
	QuietCmpSlow128    = quietCmpSlow128
	QuietCmpSlow64     = quietCmpSlow64
	SameQuantumKeys128 = sameQuantumKeys128
	SameQuantumKeys64  = sameQuantumKeys64
	SubSameQuantum128  = subSameQuantum128
	SubSameQuantum64   = subSameQuantum64
	SubSlow64          = subSlow64
)

// Recip10tab64 and Recip10tab128 return entry k of the reciprocal tables.
func Recip10tab64(k int) (m uint64, s uint) {
	return recip10tab64[k].m, uint(recip10tab64[k].s)
}

func Recip10tab128(k int) (m Uint128, s uint) {
	return recip10tab128[k].m, uint(recip10tab128[k].s)
}

// RecipTrunc10tab128 returns ⌊(2^128 - 1) / 10^k⌋ from entry k of the
// 128-bit reciprocal table, the reciprocal of divPow10Recip128.
func RecipTrunc10tab128(k int) Uint128 { return recip10tab128[k].t }

// Methods.

func (c DigitClass) WithSticky(sticky bool) DigitClass { return c.withSticky(sticky) }
func (c DigitClass) Complement() DigitClass            { return c.complement() }

func (x *FloatNat) SetShl(m uint64, e int)      { x.setShl(m, e) }
func (x *FloatNat) SetMulPow5(m uint64, s int)  { x.setMulPow5(m, s) }
func (x *FloatNat) DivPow5(k int) (sticky bool) { return x.divPow5(k) }
func (x *FloatNat) Shr128(r int) Uint128        { return x.shr128(r) }
func (x *FloatNat) Lo128() Uint128              { return x.lo128() }

// Len returns the number of words of x in use.
func (x *FloatNat) Len() int { return x.n }

// Words returns all the words of x, little-endian, in use or not.
func (x *FloatNat) Words() []uint64 { return x.w[:] }

// The reciprocal division by powers of ten (recip128.go).
var DivPow10Recip128 = divPow10Recip128

// The binary-to-decimal fast path (fromfloat_fast.go).
var (
	DivPow5Fast = divPow5Fast
	MulPow5Fast = mulPow5Fast
	Pow5DivTab  = pow5DivTab
	Pow5Len     = pow5Len
	Pow5MulTab  = pow5MulTab
)

// Exact division by a power of ten (recip128.go).
var (
	CDivExactPow10128 = cDivExactPow10128
	CDivExactPow1064  = cDivExactPow1064
)

// ParseLong64 and ParseLong128 are the general path of parseW alone: the
// scanner and the rounding routine, as parseW's tail has them, without the
// short-numeral path. TestParseShort compares Parse64 and Parse128 with
// them. Kept in step with parseW's tail by hand; the comparison on words,
// syntax errors and long numerals would show a drift.
func ParseLong64(s string, mode Mode) (uint64, Status) {
	n := scanDecimal(s, Precision64+1)
	switch n.kind {
	case scanSyntax:
		return cNaN64(), Syntax
	case scanNaN:
		return cNaN64(), Exact
	case scanInf:
		return cInf64(n.neg), Exact
	}
	class := classZero
	if n.sticky {
		class = classBelowHalf
	}
	return round64(n.neg, cWide64(n.mant), n.exp, class, mode, MinExp64)
}

func ParseLong128(s string, mode Mode) (Uint128, Status) {
	n := scanDecimal(s, Precision128+1)
	switch n.kind {
	case scanSyntax:
		return cNaN128(), Syntax
	case scanNaN:
		return cNaN128(), Exact
	case scanInf:
		return cInf128(n.neg), Exact
	}
	class := classZero
	if n.sticky {
		class = classBelowHalf
	}
	return round128(n.neg, cWide128(n.mant), n.exp, class, mode, MinExp128)
}
