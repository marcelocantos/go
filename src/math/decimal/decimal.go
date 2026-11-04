// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package decimal provides functions for the decimal floating-point types
// decimal64 and decimal128, the counterparts of package math's functions for
// float64, and the operations IEEE 754 defines for decimal arithmetic that
// the operators don't provide: quantizing, reducing, and rounding with an
// explicit mode.
//
// Each function is generic over [Decimal] and computes in the format of its
// type argument, so a decimal64 result is rounded once, as a decimal64, not
// computed in decimal128 and narrowed. The arithmetic is internal/bid's,
// which also does the arithmetic of the operators.
package decimal

import (
	"internal/bid"
	"math"
	"unsafe"
)

// Decimal is the set of decimal floating-point types.
type Decimal interface {
	~decimal64 | ~decimal128
}

// A RoundingMode determines how a result that is not exactly representable
// is rounded. The operators always round to nearest, ties to even.
type RoundingMode byte

// The rounding modes of IEEE 754-2008.
const (
	ToNearestEven RoundingMode = iota // to nearest, ties to even
	ToNearestAway                     // to nearest, ties away from zero
	ToZero                            // toward zero
	ToPositiveInf                     // toward +Inf
	ToNegativeInf                     // toward -Inf
)

func (m RoundingMode) mode() bid.Mode {
	switch m {
	case ToNearestAway:
		return bid.ToNearestAway
	case ToZero:
		return bid.TowardZero
	case ToPositiveInf:
		return bid.TowardPositive
	case ToNegativeInf:
		return bid.TowardNegative
	}
	return bid.ToNearestEven
}

// is64 reports whether x is a decimal64.
func is64[D Decimal](x D) bool { return unsafe.Sizeof(x) == 8 }

func bits64[D Decimal](x D) uint64 { return math.Decimal64bits(decimal64(x)) }

func bits128[D Decimal](x D) bid.Uint128 {
	hi, lo := math.Decimal128bits(decimal128(x))
	return bid.Uint128{Hi: hi, Lo: lo}
}

func from64[D Decimal](b uint64) D { return D(math.Decimal64frombits(b)) }

func from128[D Decimal](b bid.Uint128) D { return D(math.Decimal128frombits(b.Hi, b.Lo)) }

func unary[D Decimal](x D, f64 func(uint64) uint64, f128 func(bid.Uint128) bid.Uint128) D {
	if is64(x) {
		return from64[D](f64(bits64(x)))
	}
	return from128[D](f128(bits128(x)))
}

func binary[D Decimal](x, y D, f64 func(a, b uint64) uint64, f128 func(a, b bid.Uint128) bid.Uint128) D {
	if is64(x) {
		return from64[D](f64(bits64(x), bits64(y)))
	}
	return from128[D](f128(bits128(x), bits128(y)))
}

// NaN returns an IEEE 754 "not-a-number" value of type D.
func NaN[D Decimal]() D {
	var x D
	if is64(x) {
		return from64[D](bid.NaN64)
	}
	return from128[D](bid.Uint128{Hi: bid.NaN128Hi})
}

// Inf returns positive infinity if sign >= 0, negative infinity if sign < 0.
func Inf[D Decimal](sign int) D {
	var x D
	if is64(x) {
		inf := bid.Inf64
		if sign < 0 {
			inf = bid.Neg64(inf)
		}
		return from64[D](inf)
	}
	inf := bid.Uint128{Hi: bid.Inf128Hi}
	if sign < 0 {
		inf = bid.Neg128(inf)
	}
	return from128[D](inf)
}

// IsNaN reports whether x is an IEEE 754 "not-a-number" value.
func IsNaN[D Decimal](x D) bool {
	if is64(x) {
		return bid.IsNaN64(bits64(x))
	}
	return bid.IsNaN128(bits128(x))
}

// IsInf reports whether x is an infinity, according to sign.
// If sign > 0, IsInf reports whether x is positive infinity.
// If sign < 0, IsInf reports whether x is negative infinity.
// If sign == 0, IsInf reports whether x is either infinity.
func IsInf[D Decimal](x D, sign int) bool {
	if is64(x) {
		b := bits64(x)
		return bid.IsInf64(b) && (sign == 0 || (sign > 0) != bid.Signbit64(b))
	}
	b := bits128(x)
	return bid.IsInf128(b) && (sign == 0 || (sign > 0) != bid.Signbit128(b))
}

// Signbit reports whether x is negative or negative zero.
func Signbit[D Decimal](x D) bool {
	if is64(x) {
		return bid.Signbit64(bits64(x))
	}
	return bid.Signbit128(bits128(x))
}

// Abs returns the absolute value of x, keeping its quantum.
//
// Special cases are:
//
//	Abs(±Inf) = +Inf
//	Abs(NaN) = NaN
func Abs[D Decimal](x D) D { return unary(x, bid.Abs64, bid.Abs128) }

// Copysign returns a value with the magnitude and quantum of f and the sign
// of sign.
func Copysign[D Decimal](f, sign D) D { return binary(f, sign, bid.CopySign64, bid.CopySign128) }

// Dim returns the maximum of x-y or 0.
//
// Special cases are:
//
//	Dim(+Inf, +Inf) = NaN
//	Dim(-Inf, -Inf) = NaN
//	Dim(x, NaN) = Dim(NaN, x) = NaN
func Dim[D Decimal](x, y D) D {
	// The special cases result in NaN after the subtraction.
	v := x - y
	if v <= 0 {
		return 0
	}
	return v
}

// Trunc returns the integer value of x, rounding toward zero.
func Trunc[D Decimal](x D) D { return integral(x, bid.TowardZero) }

// Floor returns the greatest integer value less than or equal to x.
func Floor[D Decimal](x D) D { return integral(x, bid.TowardNegative) }

// Ceil returns the least integer value greater than or equal to x.
func Ceil[D Decimal](x D) D { return integral(x, bid.TowardPositive) }

// Round returns the nearest integer, rounding half away from zero.
func Round[D Decimal](x D) D { return integral(x, bid.ToNearestAway) }

// RoundToEven returns the nearest integer, rounding ties to even.
func RoundToEven[D Decimal](x D) D { return integral(x, bid.ToNearestEven) }

func integral[D Decimal](x D, mode bid.Mode) D {
	if is64(x) {
		return from64[D](bid.RoundToIntegral64(bits64(x), mode))
	}
	return from128[D](bid.RoundToIntegral128(bits128(x), mode))
}

// Mod returns the floating-point remainder of x/y. The magnitude of the
// result is less than y and its sign agrees with that of x. The result is
// exact whatever the magnitude of the quotient.
//
// Special cases are:
//
//	Mod(±Inf, y) = NaN
//	Mod(NaN, y) = NaN
//	Mod(x, 0) = NaN
//	Mod(x, ±Inf) = x
//	Mod(x, NaN) = NaN
func Mod[D Decimal](x, y D) D { return binary(x, y, bid.Mod64, bid.Mod128) }

// Remainder returns the IEEE 754 floating-point remainder of x/y. The result
// is exact whatever the magnitude of the quotient.
//
// Special cases are:
//
//	Remainder(±Inf, y) = NaN
//	Remainder(NaN, y) = NaN
//	Remainder(x, 0) = NaN
//	Remainder(x, ±Inf) = x
//	Remainder(x, NaN) = NaN
func Remainder[D Decimal](x, y D) D { return binary(x, y, bid.Rem64, bid.Rem128) }

// FMA returns x * y + z, computed with only one rounding.
func FMA[D Decimal](x, y, z D) D {
	if is64(x) {
		return from64[D](bid.FMA64(bits64(x), bits64(y), bits64(z), bid.ToNearestEven))
	}
	return from128[D](bid.FMA128(bits128(x), bits128(y), bits128(z), bid.ToNearestEven))
}

// Quantize returns x rounded to the quantum of y, to nearest with ties to
// even: the result has the exponent of y and the value closest to x with that
// exponent. Quantize(x, 0.01) rounds x to the cent.
//
// Special cases are:
//
//	Quantize(NaN, y) = NaN
//	Quantize(x, NaN) = NaN
//	Quantize(±Inf, ±Inf) = ±Inf, with the sign of x
//	Quantize(±Inf, y) = NaN, for finite y
//	Quantize(x, ±Inf) = NaN, for finite x
//	Quantize(x, y) = NaN, if the result does not fit the format
func Quantize[D Decimal](x, y D) D { return QuantizeMode(x, y, ToNearestEven) }

// QuantizeMode is Quantize with the rounding mode given by mode: for example,
// QuantizeMode(x, 0.01, ToNearestAway) rounds half a cent up in magnitude.
func QuantizeMode[D Decimal](x, y D, mode RoundingMode) D {
	if is64(x) {
		return from64[D](bid.Quantize64(bits64(x), bits64(y), mode.mode()))
	}
	return from128[D](bid.Quantize128(bits128(x), bits128(y), mode.mode()))
}

// SameQuantum reports whether x and y have the same quantum, the exponent of
// their last digit. Two NaNs have the same quantum, and so do two infinities.
func SameQuantum[D Decimal](x, y D) bool {
	if is64(x) {
		return bid.SameQuantum64(bits64(x), bits64(y))
	}
	return bid.SameQuantum128(bits128(x), bits128(y))
}

// Reduce returns x with its trailing zeros removed: the value of x with the
// largest exponent at which it is representable. Reduce(1.50) is 1.5, and
// Reduce of any zero is a zero with exponent 0 and the sign of x.
func Reduce[D Decimal](x D) D { return unary(x, bid.Reduce64, bid.Reduce128) }
