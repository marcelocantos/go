// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package math

import "internal/bid"

// Decimal64Mod returns the floating-point remainder of x/y.
// The magnitude of the result is less than y and its sign agrees with that of x.
// The result is exact whatever the magnitude of the quotient.
//
// Special cases are:
//
//	Decimal64Mod(±Inf, y) = NaN
//	Decimal64Mod(NaN, y) = NaN
//	Decimal64Mod(x, 0) = NaN
//	Decimal64Mod(x, ±Inf) = x
//	Decimal64Mod(x, NaN) = NaN
func Decimal64Mod(x, y decimal64) decimal64 {
	return Decimal64frombits(bid.Mod64(Decimal64bits(x), Decimal64bits(y)))
}

// Decimal64Remainder returns the IEEE 754 floating-point remainder of x/y.
// The result is exact whatever the magnitude of the quotient.
//
// Special cases are:
//
//	Decimal64Remainder(±Inf, y) = NaN
//	Decimal64Remainder(NaN, y) = NaN
//	Decimal64Remainder(x, 0) = NaN
//	Decimal64Remainder(x, ±Inf) = x
//	Decimal64Remainder(x, NaN) = NaN
func Decimal64Remainder(x, y decimal64) decimal64 {
	return Decimal64frombits(bid.Rem64(Decimal64bits(x), Decimal64bits(y)))
}

// Decimal64FMA returns x * y + z, computed with only one rounding.
// (That is, Decimal64FMA returns the fused multiply-add of x, y, and z.)
func Decimal64FMA(x, y, z decimal64) decimal64 {
	return Decimal64frombits(bid.FMA64(Decimal64bits(x), Decimal64bits(y), Decimal64bits(z), bid.ToNearestEven))
}

// Decimal64Quantize returns x rounded to the quantum (exponent) of y.
// The result has the same exponent as y and the value is the closest
// to x with that exponent, using round-to-nearest-even for ties.
//
// Special cases are:
//
//	Decimal64Quantize(NaN, y) = NaN
//	Decimal64Quantize(x, NaN) = NaN
//	Decimal64Quantize(±Inf, ±Inf) = ±Inf (preserving x's sign)
//	Decimal64Quantize(±Inf, y) = NaN (if y is finite)
//	Decimal64Quantize(x, ±Inf) = NaN (if x is finite)
//	Decimal64Quantize(x, y) = NaN (if the result does not fit in the format)
func Decimal64Quantize(x, y decimal64) decimal64 {
	return Decimal64frombits(bid.Quantize64(Decimal64bits(x), Decimal64bits(y), bid.ToNearestEven))
}

// Decimal64SameQuantum reports whether x and y have the same quantum
// (exponent). Both NaN values are considered to have the same quantum.
// Both infinities are considered to have the same quantum.
func Decimal64SameQuantum(x, y decimal64) bool {
	return bid.SameQuantum64(Decimal64bits(x), Decimal64bits(y))
}

// --- decimal128 versions ---

// Decimal128Mod returns the floating-point remainder of x/y.
// The magnitude of the result is less than y and its sign agrees with that of x.
// The result is exact whatever the magnitude of the quotient.
//
// Special cases are:
//
//	Decimal128Mod(±Inf, y) = NaN
//	Decimal128Mod(NaN, y) = NaN
//	Decimal128Mod(x, 0) = NaN
//	Decimal128Mod(x, ±Inf) = x
//	Decimal128Mod(x, NaN) = NaN
func Decimal128Mod(x, y decimal128) decimal128 {
	return fromD128(bid.Mod128(d128(x), d128(y)))
}

// Decimal128Remainder returns the IEEE 754 floating-point remainder of x/y.
// The result is exact whatever the magnitude of the quotient.
//
// Special cases are:
//
//	Decimal128Remainder(±Inf, y) = NaN
//	Decimal128Remainder(NaN, y) = NaN
//	Decimal128Remainder(x, 0) = NaN
//	Decimal128Remainder(x, ±Inf) = x
//	Decimal128Remainder(x, NaN) = NaN
func Decimal128Remainder(x, y decimal128) decimal128 {
	return fromD128(bid.Rem128(d128(x), d128(y)))
}

// Decimal128FMA returns x * y + z, computed with only one rounding.
// (That is, Decimal128FMA returns the fused multiply-add of x, y, and z.)
func Decimal128FMA(x, y, z decimal128) decimal128 {
	return fromD128(bid.FMA128(d128(x), d128(y), d128(z), bid.ToNearestEven))
}

// Decimal128Quantize returns x rounded to the quantum (exponent) of y.
// The result has the same exponent as y and the value is the closest
// to x with that exponent, using round-to-nearest-even for ties.
//
// Special cases are:
//
//	Decimal128Quantize(NaN, y) = NaN
//	Decimal128Quantize(x, NaN) = NaN
//	Decimal128Quantize(±Inf, ±Inf) = ±Inf (preserving x's sign)
//	Decimal128Quantize(±Inf, y) = NaN (if y is finite)
//	Decimal128Quantize(x, ±Inf) = NaN (if x is finite)
//	Decimal128Quantize(x, y) = NaN (if the result does not fit in the format)
func Decimal128Quantize(x, y decimal128) decimal128 {
	return fromD128(bid.Quantize128(d128(x), d128(y), bid.ToNearestEven))
}

// Decimal128SameQuantum reports whether x and y have the same quantum
// (exponent). Both NaN values are considered to have the same quantum.
// Both infinities are considered to have the same quantum.
func Decimal128SameQuantum(x, y decimal128) bool {
	return bid.SameQuantum128(d128(x), d128(y))
}
