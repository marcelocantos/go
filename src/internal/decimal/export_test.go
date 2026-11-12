// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package decimal

// The multiple-precision arithmetic of mp.go and the integer helpers of
// elementary.go, for the tests.

type Mpf = mpf

func MakeMpf(m []uint64, exp int, neg bool) Mpf { return mpNorm(len(m), m, exp, neg) }

func (x mpf) Parts() (m []uint64, exp int, neg bool) { return x.m, x.exp, x.neg }

var (
	MpMul        = mpMul
	MpAdd        = mpAdd
	MpSub        = mpSub
	MpDiv        = mpDiv
	MpMulWord    = mpMulWord
	MpDivWord    = mpDivWord
	MpSqrt       = mpSqrt
	MpPow10      = mpPow10
	MpCmpAbs     = mpCmpAbs
	MpLn2        = mpLn2
	MpLn10       = mpLn10
	MpLn2Series  = mpLn2Series
	MpLn10Series = mpLn10Series
	MpAtanh2     = mpAtanh2
	MpLn         = mpLn
	MpDec        = mpDec
	MpDigits     = mpDigits
	LnTableWords = len(ln2Words)

	DigitsU   = digitsU
	StripU    = stripU
	PowU      = powU
	RootU     = rootU
	DivPow10U = divPow10U
	PowExact  = powExact
)

// MpExp returns e^t and whether the evaluation gave a value.
func MpExp(n int, t Mpf) (Mpf, bool) {
	v, ev := mpExp(n, t)
	return v, ev == evValue
}
