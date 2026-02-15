// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package math

// Decimal64Abs returns the absolute value of d.
//
// Special cases are:
//
//	Decimal64Abs(±Inf) = +Inf
//	Decimal64Abs(NaN) = NaN
func Decimal64Abs(d decimal64) decimal64 {
	return Decimal64frombits(Decimal64bits(d) &^ (1 << 63))
}

// Decimal64Copysign returns a value with the magnitude of f
// and the sign of sign.
func Decimal64Copysign(f, sign decimal64) decimal64 {
	const signBit = 1 << 63
	return Decimal64frombits(Decimal64bits(f)&^signBit | Decimal64bits(sign)&signBit)
}

// Decimal64Signbit reports whether d is negative or negative zero.
func Decimal64Signbit(d decimal64) bool {
	return Decimal64bits(d)&(1<<63) != 0
}

// Decimal64Dim returns the maximum of x-y or 0.
//
// Special cases are:
//
//	Decimal64Dim(+Inf, +Inf) = NaN
//	Decimal64Dim(-Inf, -Inf) = NaN
//	Decimal64Dim(x, NaN) = Decimal64Dim(NaN, x) = NaN
func Decimal64Dim(x, y decimal64) decimal64 {
	// The special cases result in NaN after the subtraction:
	//      +Inf - +Inf = NaN
	//      -Inf - -Inf = NaN
	//       NaN - y    = NaN
	//         x - NaN  = NaN
	v := x - y
	if v <= 0 {
		// v is negative or 0
		return 0
	}
	// v is positive or NaN
	return v
}

// Decimal128Abs returns the absolute value of d.
//
// Special cases are:
//
//	Decimal128Abs(±Inf) = +Inf
//	Decimal128Abs(NaN) = NaN
func Decimal128Abs(d decimal128) decimal128 {
	hi, lo := Decimal128bits(d)
	return Decimal128frombits(hi&^(1<<63), lo)
}

// Decimal128Copysign returns a value with the magnitude of f
// and the sign of sign.
func Decimal128Copysign(f, sign decimal128) decimal128 {
	const signBit = 1 << 63
	fhi, flo := Decimal128bits(f)
	shi, _ := Decimal128bits(sign)
	return Decimal128frombits(fhi&^signBit|shi&signBit, flo)
}

// Decimal128Signbit reports whether d is negative or negative zero.
func Decimal128Signbit(d decimal128) bool {
	hi, _ := Decimal128bits(d)
	return hi&(1<<63) != 0
}

// Decimal128Dim returns the maximum of x-y or 0.
//
// Special cases are:
//
//	Decimal128Dim(+Inf, +Inf) = NaN
//	Decimal128Dim(-Inf, -Inf) = NaN
//	Decimal128Dim(x, NaN) = Decimal128Dim(NaN, x) = NaN
func Decimal128Dim(x, y decimal128) decimal128 {
	v := x - y
	if v <= 0 {
		return 0
	}
	return v
}
