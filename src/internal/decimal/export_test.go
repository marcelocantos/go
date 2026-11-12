// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package decimal

// The multiple-precision arithmetic of mp.go and the integer helpers of
// elementary.go, for the tests.

type Mpf = mpf

// The tests take their words from the heap: an arena with no store.

func MakeMpf(m []uint64, exp int, neg bool) Mpf { return mpNorm(new(arena), len(m), m, exp, neg) }

func (x mpf) Parts() (m []uint64, exp int, neg bool) { return x.m, x.exp, x.neg }

func MpMul(n int, x, y Mpf) Mpf                      { return mpMul(new(arena), n, x, y) }
func MpAdd(n int, x, y Mpf) Mpf                      { return mpAdd(new(arena), n, x, y) }
func MpSub(n int, x, y Mpf) Mpf                      { return mpSub(new(arena), n, x, y) }
func MpDiv(n int, x, y Mpf) Mpf                      { return mpDiv(new(arena), n, x, y) }
func MpMulWord(n int, x Mpf, w uint64) Mpf           { return mpMulWord(new(arena), n, x, w) }
func MpDivWord(n int, x Mpf, w uint64) Mpf           { return mpDivWord(new(arena), n, x, w) }
func MpSqrt(n int, x Mpf) Mpf                        { return mpSqrt(new(arena), n, x) }
func MpPow10(n int, e int) Mpf                       { return mpPow10(new(arena), n, e) }
func MpLn2(n int) Mpf                                { return mpLn2(new(arena), n) }
func MpLn10(n int) Mpf                               { return mpLn10(new(arena), n) }
func MpLn2Series(n int) Mpf                          { return mpLn2Series(new(arena), n) }
func MpLn10Series(n int) Mpf                         { return mpLn10Series(new(arena), n) }
func MpAtanh2(n int, z Mpf) Mpf                      { return mpAtanh2(new(arena), n, z) }
func MpLn(n int, c u128, e int) Mpf                  { return mpLn(new(arena), n, c, e) }
func MpDec(n int, neg bool, c u128, e int) Mpf       { return mpDec(new(arena), n, neg, c, e) }
func MpScale10(n int, v Mpf, q int) Mpf              { return mpScale10(new(arena), n, v, q) }
func MpDigits(n int, v Mpf, k int) (u128, int, bool) { return mpDigits(new(arena), n, v, k) }

var (
	MpCmpAbs     = mpCmpAbs
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
	v, ev := mpExp(new(arena), n, t)
	return v, ev == evValue
}

// MpPowInt returns b^k and whether the evaluation gave a value.
func MpPowInt(n int, b Mpf, k int64) (Mpf, bool) {
	v, ev := mpPowInt(new(arena), n, b, k)
	return v, ev == evValue
}
