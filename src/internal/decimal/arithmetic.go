// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package decimal provides functions for the decimal floating-point types
// decimal64 and decimal128, the counterparts of package math's functions for
// float64, and the operations IEEE 754 defines for decimal arithmetic that
// the operators don't provide: quantizing, reducing, rounding with an
// explicit mode, the quantum of a value, and reading and writing the payload
// of a NaN.
//
// Each function is generic over [Decimal] and computes in the format of its
// type argument, so a decimal64 result is rounded once, as a decimal64, not
// computed in decimal128 and narrowed. The arithmetic is internal/bid's,
// which also does the arithmetic of the operators.
package decimal

import (
	"internal/bid"
	"unsafe"
)

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

// The functions of this file are generic over the two widths, and each is
// a test of the width and a call to one of two functions of concrete type,
// named for the width: abs64 and abs128, fma64 and fma128. The concrete
// functions do the work, the conversion to and from the kernel's encoding
// included.
//
// The shape matters for speed. A generic function is compiled where it is
// instantiated, in the caller's package, and there the compiler does not
// inline the calls it makes to functions of this package and of
// internal/bid; and its inlining cost counts both widths, though only one
// is ever live. Written with the conversions and the kernel calls in the
// generic body, most of these functions were too large to inline and ran
// as a call to the instantiation, a call for each operand's conversion, an
// indirect call to the kernel and a call for the result's. With the two
// concrete calls charged as cheap ones (cmd/compile/internal/inline, by
// the names' endings, as it charges internal/bid's general paths) every
// generic function here inlines into its caller, and what is left there is
// the concrete function's body, if that inlines too, or one call to it.

func b128(x decimal128) bid.Uint128 {
	hi, lo := BID128bits(x)
	return bid.Uint128{Hi: hi, Lo: lo}
}

func d128(b bid.Uint128) decimal128 { return BID128frombits(b.Hi, b.Lo) }

// NaN returns an IEEE 754 "not-a-number" value of type D.
func NaN[D Decimal]() D {
	var x D
	if is64(x) {
		return D(nan64())
	}
	return D(nan128())
}

func nan64() decimal64   { return BID64frombits(bid.NaN64) }
func nan128() decimal128 { return BID128frombits(bid.NaN128Hi, 0) }

// Inf returns positive infinity if sign >= 0, negative infinity if sign < 0.
func Inf[D Decimal](sign int) D {
	var x D
	if is64(x) {
		return D(inf64(sign))
	}
	return D(inf128(sign))
}

func inf64(sign int) decimal64 {
	inf := bid.Inf64
	if sign < 0 {
		inf = bid.Neg64(inf)
	}
	return BID64frombits(inf)
}

func inf128(sign int) decimal128 {
	inf := bid.Uint128{Hi: bid.Inf128Hi}
	if sign < 0 {
		inf = bid.Neg128(inf)
	}
	return d128(inf)
}

// IsNaN reports whether x is an IEEE 754 "not-a-number" value.
func IsNaN[D Decimal](x D) bool {
	if is64(x) {
		return isNaN64(decimal64(x))
	}
	return isNaN128(decimal128(x))
}

func isNaN64(x decimal64) bool   { return bid.IsNaN64(BID64bits(x)) }
func isNaN128(x decimal128) bool { return bid.IsNaN128(b128(x)) }

// IsInf reports whether x is an infinity, according to sign.
// If sign > 0, IsInf reports whether x is positive infinity.
// If sign < 0, IsInf reports whether x is negative infinity.
// If sign == 0, IsInf reports whether x is either infinity.
func IsInf[D Decimal](x D, sign int) bool {
	if is64(x) {
		return isInf64(decimal64(x), sign)
	}
	return isInf128(decimal128(x), sign)
}

func isInf64(x decimal64, sign int) bool {
	b := BID64bits(x)
	return bid.IsInf64(b) && (sign == 0 || (sign > 0) != bid.Signbit64(b))
}

func isInf128(x decimal128, sign int) bool {
	b := b128(x)
	return bid.IsInf128(b) && (sign == 0 || (sign > 0) != bid.Signbit128(b))
}

// Payload returns the payload of the NaN x (the getPayload operation of
// IEEE 754-2019 clause 9.7): the non-negative integer it carries, as a value
// of type D with exponent 0. The sign of x and whether it is signaling do
// not matter. A NaN's payload survives the arithmetic that propagates the
// NaN, and widening or narrowing keeps its value when the narrower format
// can hold it; otherwise it becomes 0.
//
// Payload returns -1 if x is not a NaN, which no payload is.
func Payload[D Decimal](x D) D {
	if is64(x) {
		return D(payload64(decimal64(x)))
	}
	return D(payload128(decimal128(x)))
}

func payload64(x decimal64) decimal64    { return BID64frombits(bid.Payload64(BID64bits(x))) }
func payload128(x decimal128) decimal128 { return d128(bid.Payload128(b128(x))) }

// NaNWithPayload returns a quiet NaN of type D whose payload is payload (the
// setPayload operation of IEEE 754-2019 clause 9.7). The payload must be an
// integer from 0 to 10^15-1 for decimal64, or to 10^33-1 for decimal128; its
// exponent does not matter, so 42, 42.0 and 4.2E+1 all give payload 42.
// Payload(NaNWithPayload(p)) == p for every such p.
//
// NaNWithPayload returns +0 if payload is not such an integer: a fraction,
// a negative number, an integer out of range, an infinity or a NaN.
func NaNWithPayload[D Decimal](payload D) D {
	if is64(payload) {
		return D(naNWithPayload64(decimal64(payload)))
	}
	return D(naNWithPayload128(decimal128(payload)))
}

func naNWithPayload64(x decimal64) decimal64 {
	return BID64frombits(bid.NaNWithPayload64(BID64bits(x)))
}
func naNWithPayload128(x decimal128) decimal128 { return d128(bid.NaNWithPayload128(b128(x))) }

// SignalingNaNWithPayload is NaNWithPayload, but returns a signaling NaN (the
// setPayloadSignaling operation of IEEE 754-2019 clause 9.7). Arithmetic on
// a signaling NaN returns it made quiet, with the same payload.
func SignalingNaNWithPayload[D Decimal](payload D) D {
	if is64(payload) {
		return D(signalingNaNWithPayload64(decimal64(payload)))
	}
	return D(signalingNaNWithPayload128(decimal128(payload)))
}

func signalingNaNWithPayload64(x decimal64) decimal64 {
	return BID64frombits(bid.SignalingNaNWithPayload64(BID64bits(x)))
}
func signalingNaNWithPayload128(x decimal128) decimal128 {
	return d128(bid.SignalingNaNWithPayload128(b128(x)))
}

// Signbit reports whether x is negative or negative zero.
func Signbit[D Decimal](x D) bool {
	if is64(x) {
		return signbit64(decimal64(x))
	}
	return signbit128(decimal128(x))
}

func signbit64(x decimal64) bool   { return bid.Signbit64(BID64bits(x)) }
func signbit128(x decimal128) bool { return bid.Signbit128(b128(x)) }

// Abs returns the absolute value of x, keeping its quantum.
//
// Special cases are:
//
//	Abs(±Inf) = +Inf
//	Abs(NaN) = NaN
func Abs[D Decimal](x D) D {
	if is64(x) {
		return D(abs64(decimal64(x)))
	}
	return D(abs128(decimal128(x)))
}

func abs64(x decimal64) decimal64    { return BID64frombits(bid.Abs64(BID64bits(x))) }
func abs128(x decimal128) decimal128 { return d128(bid.Abs128(b128(x))) }

// Copysign returns a value with the magnitude and quantum of f and the sign
// of sign.
func Copysign[D Decimal](f, sign D) D {
	if is64(f) {
		return D(copysign64(decimal64(f), decimal64(sign)))
	}
	return D(copysign128(decimal128(f), decimal128(sign)))
}

func copysign64(x, y decimal64) decimal64 {
	return BID64frombits(bid.CopySign64(BID64bits(x), BID64bits(y)))
}

func copysign128(x, y decimal128) decimal128 { return d128(bid.CopySign128(b128(x), b128(y))) }

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
		return D(integral64(decimal64(x), mode))
	}
	return D(integral128(decimal128(x), mode))
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
func Mod[D Decimal](x, y D) D {
	if is64(x) {
		return D(mod64(decimal64(x), decimal64(y)))
	}
	return D(mod128(decimal128(x), decimal128(y)))
}

func mod64(x, y decimal64) decimal64 {
	return BID64frombits(bid.Mod64(BID64bits(x), BID64bits(y)))
}

func mod128(x, y decimal128) decimal128 { return d128(bid.Mod128(b128(x), b128(y))) }

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
func Remainder[D Decimal](x, y D) D {
	if is64(x) {
		return D(remainder64(decimal64(x), decimal64(y)))
	}
	return D(remainder128(decimal128(x), decimal128(y)))
}

func remainder64(x, y decimal64) decimal64 {
	return BID64frombits(bid.Rem64(BID64bits(x), BID64bits(y)))
}

func remainder128(x, y decimal128) decimal128 { return d128(bid.Rem128(b128(x), b128(y))) }

// FMA returns x * y + z, computed with only one rounding.
func FMA[D Decimal](x, y, z D) D {
	if is64(x) {
		return D(fma64(decimal64(x), decimal64(y), decimal64(z)))
	}
	return D(fma128(decimal128(x), decimal128(y), decimal128(z)))
}

func fma64(x, y, z decimal64) decimal64 {
	return BID64frombits(bid.FMA64(BID64bits(x), BID64bits(y), BID64bits(z), bid.ToNearestEven))
}

func fma128(x, y, z decimal128) decimal128 {
	return d128(bid.FMA128(b128(x), b128(y), b128(z), bid.ToNearestEven))
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
		return D(quantize64(decimal64(x), decimal64(y), mode))
	}
	return D(quantize128(decimal128(x), decimal128(y), mode))
}

// SameQuantum reports whether x and y have the same quantum, the exponent of
// their last digit. Two NaNs have the same quantum, and so do two infinities.
func SameQuantum[D Decimal](x, y D) bool {
	if is64(x) {
		return sameQuantum64(decimal64(x), decimal64(y))
	}
	return sameQuantum128(decimal128(x), decimal128(y))
}

func sameQuantum64(x, y decimal64) bool   { return bid.SameQuantum64(BID64bits(x), BID64bits(y)) }
func sameQuantum128(x, y decimal128) bool { return bid.SameQuantum128(b128(x), b128(y)) }

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
func Quantum[D Decimal](x D) D {
	if is64(x) {
		return D(quantum64(decimal64(x)))
	}
	return D(quantum128(decimal128(x)))
}

func quantum64(x decimal64) decimal64    { return BID64frombits(bid.Quantum64(BID64bits(x))) }
func quantum128(x decimal128) decimal128 { return d128(bid.Quantum128(b128(x))) }

// Reduce returns x with its trailing zeros removed: the value of x with the
// largest exponent at which it is representable. Reduce(1.50) is 1.5, and
// Reduce of any zero is a zero with exponent 0 and the sign of x.
func Reduce[D Decimal](x D) D {
	if is64(x) {
		return D(reduce64(decimal64(x)))
	}
	return D(reduce128(decimal128(x)))
}

func reduce64(x decimal64) decimal64    { return BID64frombits(bid.Reduce64(BID64bits(x))) }
func reduce128(x decimal128) decimal128 { return d128(bid.Reduce128(b128(x))) }

// Concrete entry points keep the generic dispatch small enough to inline.
// ARM64 and AMD64 recognize these calls and lower bounded BID cases directly;
// these bodies are the portable fallback, including DPD conversion.
func integral64(x decimal64, mode bid.Mode) decimal64 {
	return BID64frombits(bid.RoundToIntegral64(BID64bits(x), mode))
}
func integral128(x decimal128, mode bid.Mode) decimal128 {
	return d128(bid.RoundToIntegral128(b128(x), mode))
}
func quantize64(x, y decimal64, mode RoundingMode) decimal64 {
	return BID64frombits(bid.Quantize64(BID64bits(x), BID64bits(y), mode.mode()))
}
func quantize128(x, y decimal128, mode RoundingMode) decimal128 {
	return d128(bid.Quantize128(b128(x), b128(y), mode.mode()))
}
