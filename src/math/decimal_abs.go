// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package math

import "internal/bid"

// Decimal64Abs returns the absolute value of d.
//
// Special cases are:
//
//	Decimal64Abs(±Inf) = +Inf
//	Decimal64Abs(NaN) = NaN
func Decimal64Abs(d decimal64) decimal64 {
	return Decimal64frombits(bid.Abs64(Decimal64bits(d)))
}

// Decimal64Copysign returns a value with the magnitude of f
// and the sign of sign.
func Decimal64Copysign(f, sign decimal64) decimal64 {
	return Decimal64frombits(bid.CopySign64(Decimal64bits(f), Decimal64bits(sign)))
}

// Decimal64Signbit reports whether d is negative or negative zero.
func Decimal64Signbit(d decimal64) bool {
	return bid.Signbit64(Decimal64bits(d))
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
	v := bid.Sub64(Decimal64bits(x), Decimal64bits(y), bid.ToNearestEven)
	if bid.LessEqual64(v, bid.Zero64) {
		// v is negative or 0
		return 0
	}
	// v is positive or NaN
	return Decimal64frombits(v)
}

// Decimal128Abs returns the absolute value of d.
//
// Special cases are:
//
//	Decimal128Abs(±Inf) = +Inf
//	Decimal128Abs(NaN) = NaN
func Decimal128Abs(d decimal128) decimal128 {
	return fromD128(bid.Abs128(d128(d)))
}

// Decimal128Copysign returns a value with the magnitude of f
// and the sign of sign.
func Decimal128Copysign(f, sign decimal128) decimal128 {
	return fromD128(bid.CopySign128(d128(f), d128(sign)))
}

// Decimal128Signbit reports whether d is negative or negative zero.
func Decimal128Signbit(d decimal128) bool {
	return bid.Signbit128(d128(d))
}

// Decimal128Dim returns the maximum of x-y or 0.
//
// Special cases are:
//
//	Decimal128Dim(+Inf, +Inf) = NaN
//	Decimal128Dim(-Inf, -Inf) = NaN
//	Decimal128Dim(x, NaN) = Decimal128Dim(NaN, x) = NaN
func Decimal128Dim(x, y decimal128) decimal128 {
	v := bid.Sub128(d128(x), d128(y), bid.ToNearestEven)
	if bid.LessEqual128(v, bid.Uint128{Hi: bid.Zero128Hi}) {
		return 0
	}
	return fromD128(v)
}
