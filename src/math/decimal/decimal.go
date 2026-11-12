// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

// Package decimal provides functions for the decimal floating-point types
// decimal64 and decimal128, the counterparts of package math's functions for
// float64, and the operations IEEE 754 defines for decimal arithmetic that
// the operators don't provide: quantizing, reducing, rounding with an
// explicit mode, the quantum of a value, and reading and writing the payload
// of a NaN.
//
// It also provides the square root, the powers, the exponential and the
// logarithms. Those are correctly rounded: each returns the value of its
// type nearest to the exact result, with ties to even. They are slower
// than the arithmetic, hundreds of nanoseconds to a few microseconds a
// call, and they do not allocate, except for the rare result so close to
// a rounding boundary that it takes many times the usual precision to
// tell which side it is on.
//
// Each function is generic over [Decimal] and computes in the format of its
// type argument, so a decimal64 result is rounded once, as a decimal64, not
// computed in decimal128 and narrowed. The arithmetic is internal/bid's,
// which also does the arithmetic of the operators.
package decimal

import impl "internal/decimal"

// Decimal is the set of decimal floating-point types.
type Decimal interface {
	~decimal64 | ~decimal128
}

// A RoundingMode determines how a result that is not exactly representable
// is rounded. The operators always round to nearest, ties to even.
type RoundingMode byte

// The rounding modes of IEEE 754-2019.
const (
	ToNearestEven RoundingMode = iota // to nearest, ties to even
	ToNearestAway                     // to nearest, ties away from zero
	ToZero                            // toward zero
	ToPositiveInf                     // toward +Inf
	ToNegativeInf                     // toward -Inf
)

// NaN returns an IEEE 754 "not-a-number" value of type D.
func NaN[D Decimal]() D { return impl.NaN[D]() }

// Inf returns positive infinity if sign >= 0, negative infinity if sign < 0.
func Inf[D Decimal](sign int) D { return impl.Inf[D](sign) }

// IsNaN reports whether x is an IEEE 754 "not-a-number" value.
func IsNaN[D Decimal](x D) bool { return impl.IsNaN[D](x) }

// IsInf reports whether x is an infinity, according to sign.
// If sign > 0, IsInf reports whether x is positive infinity.
// If sign < 0, IsInf reports whether x is negative infinity.
// If sign == 0, IsInf reports whether x is either infinity.
func IsInf[D Decimal](x D, sign int) bool { return impl.IsInf[D](x, sign) }

// Payload returns the payload of the NaN x (the getPayload operation of
// IEEE 754-2019 clause 9.7): the non-negative integer it carries, as a value
// of type D with exponent 0. The sign of x and whether it is signaling do
// not matter. A NaN's payload survives the arithmetic that propagates the
// NaN, and widening or narrowing keeps its value when the narrower format
// can hold it; otherwise it becomes 0.
//
// Payload returns -1 if x is not a NaN, which no payload is.
func Payload[D Decimal](x D) D { return impl.Payload[D](x) }

// NaNWithPayload returns a quiet NaN of type D whose payload is payload (the
// setPayload operation of IEEE 754-2019 clause 9.7). The payload must be an
// integer from 0 to 10^15-1 for decimal64, or to 10^33-1 for decimal128; its
// exponent does not matter, so 42, 42.0 and 4.2E+1 all give payload 42.
// Payload(NaNWithPayload(p)) == p for every such p.
//
// NaNWithPayload returns +0 if payload is not such an integer: a fraction,
// a negative number, an integer out of range, an infinity or a NaN.
func NaNWithPayload[D Decimal](payload D) D { return impl.NaNWithPayload[D](payload) }

// SignalingNaNWithPayload is NaNWithPayload, but returns a signaling NaN (the
// setPayloadSignaling operation of IEEE 754-2019 clause 9.7). Arithmetic on
// a signaling NaN returns it made quiet, with the same payload.
func SignalingNaNWithPayload[D Decimal](payload D) D { return impl.SignalingNaNWithPayload[D](payload) }

// Signbit reports whether x is negative or negative zero.
func Signbit[D Decimal](x D) bool { return impl.Signbit[D](x) }

// Abs returns the absolute value of x, keeping its quantum.
//
// Special cases are:
//
//	Abs(±Inf) = +Inf
//	Abs(NaN) = NaN
func Abs[D Decimal](x D) D { return impl.Abs[D](x) }

// Copysign returns a value with the magnitude and quantum of f and the sign
// of sign.
func Copysign[D Decimal](f, sign D) D { return impl.Copysign[D](f, sign) }

// Dim returns the maximum of x-y or 0.
//
// Special cases are:
//
//	Dim(+Inf, +Inf) = NaN
//	Dim(-Inf, -Inf) = NaN
//	Dim(x, NaN) = Dim(NaN, x) = NaN
func Dim[D Decimal](x, y D) D { return impl.Dim[D](x, y) }

// Trunc returns the integer value of x, rounding toward zero.
func Trunc[D Decimal](x D) D { return impl.Trunc[D](x) }

// Floor returns the greatest integer value less than or equal to x.
func Floor[D Decimal](x D) D { return impl.Floor[D](x) }

// Ceil returns the least integer value greater than or equal to x.
func Ceil[D Decimal](x D) D { return impl.Ceil[D](x) }

// Round returns the nearest integer, rounding half away from zero.
func Round[D Decimal](x D) D { return impl.Round[D](x) }

// RoundToEven returns the nearest integer, rounding ties to even.
func RoundToEven[D Decimal](x D) D { return impl.RoundToEven[D](x) }

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
func Mod[D Decimal](x, y D) D { return impl.Mod[D](x, y) }

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
func Remainder[D Decimal](x, y D) D { return impl.Remainder[D](x, y) }

// FMA returns x * y + z, computed with only one rounding.
func FMA[D Decimal](x, y, z D) D { return impl.FMA[D](x, y, z) }

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
func Quantize[D Decimal](x, y D) D { return impl.Quantize[D](x, y) }

// QuantizeMode is Quantize with the rounding mode given by mode: for example,
// QuantizeMode(x, 0.01, ToNearestAway) rounds half a cent up in magnitude.
func QuantizeMode[D Decimal](x, y D, mode RoundingMode) D {
	return impl.QuantizeMode[D](x, y, impl.RoundingMode(mode))
}

// SameQuantum reports whether x and y have the same quantum, the exponent of
// their last digit. Two NaNs have the same quantum, and so do two infinities.
func SameQuantum[D Decimal](x, y D) bool { return impl.SameQuantum[D](x, y) }

// Quantum returns the quantum of x, the value of one unit in its last digit
// (the quantum operation of IEEE 754-2019 clause 5.3.2): 1 × 10^e, where e is
// the exponent of x, as a positive value of type D with exponent e.
// Quantum(1.50) is 0.01, Quantum(-5E+3) is 1E+3 and Quantum(0.00) is 0.01.
// Quantize(x, Quantum(y)) is Quantize(x, y) for every finite y.
//
// Special cases are:
//
//	Quantum(±Inf) = +Inf
//	Quantum(NaN) = NaN, with the payload of the NaN operand
func Quantum[D Decimal](x D) D { return impl.Quantum[D](x) }

// Reduce returns x with its trailing zeros removed: the value of x with the
// largest exponent at which it is representable. Reduce(1.50) is 1.5, and
// Reduce of any zero is a zero with exponent 0 and the sign of x.
func Reduce[D Decimal](x D) D { return impl.Reduce[D](x) }

// Sqrt returns the square root of x, correctly rounded. An exact root
// has the exponent nearest to half that of x: Sqrt(2.25) is 1.5 and
// Sqrt(4.00) is 2.0.
//
// Special cases are:
//
//	Sqrt(+Inf) = +Inf
//	Sqrt(±0) = ±0
//	Sqrt(x < 0) = NaN
//	Sqrt(NaN) = NaN
func Sqrt[D Decimal](x D) D { return impl.Sqrt[D](x) }

// Exp returns e**x, the base-e exponential of x, correctly rounded.
//
// Special cases are:
//
//	Exp(+Inf) = +Inf
//	Exp(-Inf) = 0
//	Exp(±0) = 1
//	Exp(NaN) = NaN
//
// Very large values overflow to +Inf and very small values underflow to
// 0.
func Exp[D Decimal](x D) D { return impl.Exp[D](x) }

// Log returns the natural logarithm of x, correctly rounded.
//
// Special cases are:
//
//	Log(+Inf) = +Inf
//	Log(1) = 0
//	Log(±0) = -Inf
//	Log(x < 0) = NaN
//	Log(NaN) = NaN
func Log[D Decimal](x D) D { return impl.Log[D](x) }

// Log10 returns the decimal logarithm of x, correctly rounded. The
// logarithm of a power of ten is exact: Log10(1000) is 3 and Log10(0.01)
// is -2. The special cases are the same as for [Log].
func Log10[D Decimal](x D) D { return impl.Log10[D](x) }

// Pow returns x**y, the base-x exponential of y, correctly rounded. A
// result that is exact has the exponent nearest to y times that of x, as
// repeated multiplication would give it: Pow(1.5, 2) is 2.25 and
// Pow(2.25, 0.5) is 1.5.
//
// Special cases are (in order):
//
//	Pow(x, ±0) = 1 for any x
//	Pow(1, y) = 1 for any y
//	Pow(x, NaN) = NaN
//	Pow(NaN, y) = NaN
//	Pow(±0, y) = ±Inf for y an odd integer < 0
//	Pow(±0, -Inf) = +Inf
//	Pow(±0, +Inf) = +0
//	Pow(±0, y) = +Inf for finite y < 0 and not an odd integer
//	Pow(±0, y) = ±0 for y an odd integer > 0
//	Pow(±0, y) = +0 for finite y > 0 and not an odd integer
//	Pow(-1, ±Inf) = 1
//	Pow(x, +Inf) = +Inf for |x| > 1
//	Pow(x, -Inf) = +0 for |x| > 1
//	Pow(x, +Inf) = +0 for |x| < 1
//	Pow(x, -Inf) = +Inf for |x| < 1
//	Pow(+Inf, y) = +Inf for y > 0
//	Pow(+Inf, y) = +0 for y < 0
//	Pow(-Inf, y) = Pow(-0, -y)
//	Pow(x, y) = NaN for finite x < 0 and finite non-integer y
func Pow[D Decimal](x, y D) D { return impl.Pow[D](x, y) }

// Pown returns x**n for an integer n, correctly rounded (the pown
// operation of IEEE 754-2019 clause 9.2). The exponent of an exact result
// is the nearest to n times that of x: Pown(1.5, 2) is 2.25 and
// Pown(2, -2) is 0.25.
//
// Special cases are:
//
//	Pown(x, 0) = 1 for any x, even a NaN
//	Pown(±0, n) = ±Inf for odd n < 0
//	Pown(±0, n) = +Inf for even n < 0
//	Pown(±0, n) = ±0 for odd n > 0
//	Pown(±0, n) = +0 for even n > 0
//	Pown(±Inf, n) = Pown(±0, -n)
//	Pown(NaN, n) = NaN
func Pown[D Decimal](x D, n int) D { return impl.Pown[D](x, n) }

// Compound returns (1+x)**n, correctly rounded (the compound operation
// of IEEE 754-2019 clause 9.2): the growth of one unit at the rate x
// over n periods. It is exact about the sum 1+x, which Pown(1+x, n) is
// not when x is a small rate whose digits the sum would lose.
//
// Special cases are:
//
//	Compound(x, 0) = 1 for x >= -1 or a NaN
//	Compound(-1, n) = +Inf for n < 0
//	Compound(-1, n) = +0 for n > 0
//	Compound(±0, n) = 1
//	Compound(+Inf, n) = +Inf for n > 0
//	Compound(+Inf, n) = +0 for n < 0
//	Compound(x, n) = NaN for x < -1
//	Compound(NaN, n) = NaN for n != 0
func Compound[D Decimal](x D, n int) D { return impl.Compound[D](x, n) }

// Rootn returns the nth root of x, correctly rounded (the rootn
// operation of IEEE 754-2019 clause 9.2): Rootn(x, 12) is the monthly
// growth factor of a yearly one. The root of a negative x is negative
// for an odd n.
//
// Special cases are:
//
//	Rootn(x, 0) = NaN
//	Rootn(±0, n) = ±Inf for odd n < 0
//	Rootn(±0, n) = +Inf for even n < 0
//	Rootn(±0, n) = ±0 for odd n > 0
//	Rootn(±0, n) = +0 for even n > 0
//	Rootn(+Inf, n) = +Inf for n > 0
//	Rootn(+Inf, n) = +0 for n < 0
//	Rootn(-Inf, n) = -Inf for odd n > 0
//	Rootn(-Inf, n) = -0 for odd n < 0
//	Rootn(x, n) = NaN for x < 0 and even n
//	Rootn(NaN, n) = NaN
func Rootn[D Decimal](x D, n int) D { return impl.Rootn[D](x, n) }

// ScaleB returns x × 10**n (the scaleB operation of IEEE 754-2019 clause
// 5.3.3). It moves the decimal point and changes no digit, unless the
// result is beyond the format's range: ScaleB(1.50, 2) is 150 and
// ScaleB(150, -2) is 1.50. An infinity or a NaN is returned as it is.
func ScaleB[D Decimal](x D, n int) D { return impl.ScaleB[D](x, n) }

// LogB returns the decimal exponent of x as an integer value of type D
// (the logB operation of IEEE 754-2019 clause 5.3.3): the exponent of x
// written with one digit before the point, so LogB(250) is 2 and
// LogB(0.025) is -2.
//
// Special cases are:
//
//	LogB(±Inf) = +Inf
//	LogB(±0) = -Inf
//	LogB(NaN) = NaN
func LogB[D Decimal](x D) D { return impl.LogB[D](x) }

// NextUp returns the least value of type D that is greater than x (the
// nextUp operation of IEEE 754-2019 clause 5.3.1).
//
// Special cases are:
//
//	NextUp(±0) = the least positive value
//	NextUp(+Inf) = +Inf
//	NextUp(-Inf) = the most negative finite value
//	NextUp(NaN) = NaN
func NextUp[D Decimal](x D) D { return impl.NextUp[D](x) }

// NextDown returns the greatest value of type D that is less than x: it
// is -NextUp(-x).
func NextDown[D Decimal](x D) D { return impl.NextDown[D](x) }
