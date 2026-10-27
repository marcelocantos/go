// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// The arithmetic operations of IEEE 754-2008 clause 5.4.1 and the
// homogeneous operations of clause 5.3, for both widths. Each computes its
// result exactly and rounds it once in the given mode; each returns the
// clause 7 default result for every exception and signals nothing. The
// preferred exponent of each result and the sign of each exact zero are
// fixed by the per-operation contract that a later target adds to the
// package documentation.

// Add64 returns x + y rounded in mode (IEEE 754-2008 clause 5.4.1,
// addition).
func Add64(x, y uint64, mode Mode) uint64 {
	panic(notImplemented)
}

// Add128 is Add64 for decimal128.
func Add128(x, y Uint128, mode Mode) Uint128 {
	panic(notImplemented)
}

// Sub64 returns x - y rounded in mode (IEEE 754-2008 clause 5.4.1,
// subtraction). It is Add64(x, Neg64(y), mode) except that a NaN y is not
// negated; the distinction is invisible because NaN signs are not
// propagated.
func Sub64(x, y uint64, mode Mode) uint64 {
	panic(notImplemented)
}

// Sub128 is Sub64 for decimal128.
func Sub128(x, y Uint128, mode Mode) Uint128 {
	panic(notImplemented)
}

// Mul64 returns x × y rounded in mode (IEEE 754-2008 clause 5.4.1,
// multiplication).
func Mul64(x, y uint64, mode Mode) uint64 {
	panic(notImplemented)
}

// Mul128 is Mul64 for decimal128.
func Mul128(x, y Uint128, mode Mode) Uint128 {
	panic(notImplemented)
}

// Div64 returns x / y rounded in mode (IEEE 754-2008 clause 5.4.1,
// division). A finite non-zero x divided by a zero is the infinity with
// the exclusive-or of the signs (clause 7.3); 0 / 0 and ∞ / ∞ are NaN.
func Div64(x, y uint64, mode Mode) uint64 {
	panic(notImplemented)
}

// Div128 is Div64 for decimal128.
func Div128(x, y Uint128, mode Mode) Uint128 {
	panic(notImplemented)
}

// FMA64 returns x × y + z with a single rounding in mode (IEEE 754-2008
// clause 5.4.1, fusedMultiplyAdd). The product is exact; no rounding
// happens between it and the addition.
func FMA64(x, y, z uint64, mode Mode) uint64 {
	panic(notImplemented)
}

// FMA128 is FMA64 for decimal128.
func FMA128(x, y, z Uint128, mode Mode) Uint128 {
	panic(notImplemented)
}

// Rem64 returns the IEEE remainder x - y × n, where n is the integer
// nearest the exact quotient x / y, with n even when the quotient is
// halfway between two integers (IEEE 754-2008 clause 5.3.1, remainder).
// The result is always exact, so Rem64 takes no mode. If the result is
// zero its sign is that of x. Rem64(x, ±∞) is x for finite x; Rem64(±∞, y)
// and Rem64(x, 0) are NaN.
func Rem64(x, y uint64) uint64 {
	panic(notImplemented)
}

// Rem128 is Rem64 for decimal128.
func Rem128(x, y Uint128) Uint128 {
	panic(notImplemented)
}

// Mod64 returns the truncated modulo x - y × trunc(x / y), the decimal
// analogue of the Go % operator and of math.Mod. The result is exact and
// has the sign of x; its magnitude is below |y|. Mod64 is not an IEEE
// 754-2008 operation; it is the remainder operation of the General Decimal
// Arithmetic specification. Mod64(x, ±∞) is x for finite x; Mod64(±∞, y)
// and Mod64(x, 0) are NaN.
func Mod64(x, y uint64) uint64 {
	panic(notImplemented)
}

// Mod128 is Mod64 for decimal128.
func Mod128(x, y Uint128) Uint128 {
	panic(notImplemented)
}

// RoundToIntegral64 rounds x to an integral value in mode (IEEE 754-2008
// clause 5.3.1 and 5.9, roundToIntegral). The result has exponent
// max(exponent of x, 0), so the quantum of a value that is already
// integral is preserved. A zero keeps its sign and takes that exponent too,
// so 0.00 rounds to 0. Infinities are returned unchanged. The
// standard's roundToIntegralExact variant differs only by signaling
// inexact, which this package does not expose; a caller that needs to know
// compares the result with x.
func RoundToIntegral64(x uint64, mode Mode) uint64 {
	panic(notImplemented)
}

// RoundToIntegral128 is RoundToIntegral64 for decimal128.
func RoundToIntegral128(x Uint128, mode Mode) Uint128 {
	panic(notImplemented)
}

// Quantize64 returns x with the exponent of y, rounded in mode
// (IEEE 754-2008 clause 5.3.2, quantize). The result is NaN when either
// operand is NaN, when exactly one is an infinity, or when the coefficient
// of the result would exceed MaxCoeff64; two infinities quantize to an
// infinity with the sign of x. The exponent of the result is always that
// of y, so Quantize64 never adjusts an exponent out of range.
func Quantize64(x, y uint64, mode Mode) uint64 {
	panic(notImplemented)
}

// Quantize128 is Quantize64 for decimal128.
func Quantize128(x, y Uint128, mode Mode) Uint128 {
	panic(notImplemented)
}

// Reduce64 returns the member of x's cohort with the fewest coefficient
// digits: trailing zeros are removed from the coefficient and the exponent
// raised to match, as far as MaxExp64 allows (the reduce operation of the
// General Decimal Arithmetic specification, which IEEE 754-2008 does not
// define). The value is unchanged, so Reduce64 takes no mode. A zero
// reduces to a zero with exponent 0; infinities and NaN are canonicalized.
func Reduce64(x uint64) uint64 {
	panic(notImplemented)
}

// Reduce128 is Reduce64 for decimal128.
func Reduce128(x Uint128) Uint128 {
	panic(notImplemented)
}

// SameQuantum64 reports whether x and y have the same exponent, or are
// both infinities, or are both NaN (IEEE 754-2008 clause 5.7.3,
// sameQuantum).
func SameQuantum64(x, y uint64) bool {
	panic(notImplemented)
}

// SameQuantum128 is SameQuantum64 for decimal128.
func SameQuantum128(x, y Uint128) bool {
	panic(notImplemented)
}

// Min64 implements the Go min built-in for decimal64: if either operand is
// a NaN the result is NaN; otherwise it is the lesser operand, with -0
// less than +0, and x when the operands are equal. This is the language's
// semantics for floating-point min, not IEEE 754-2008 minNum, which
// ignores a NaN operand.
func Min64(x, y uint64) uint64 {
	panic(notImplemented)
}

// Min128 is Min64 for decimal128.
func Min128(x, y Uint128) Uint128 {
	panic(notImplemented)
}

// Max64 implements the Go max built-in for decimal64: if either operand is
// a NaN the result is NaN; otherwise it is the greater operand, with +0
// greater than -0, and x when the operands are equal.
func Max64(x, y uint64) uint64 {
	panic(notImplemented)
}

// Max128 is Max64 for decimal128.
func Max128(x, y Uint128) Uint128 {
	panic(notImplemented)
}
