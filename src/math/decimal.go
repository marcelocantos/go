// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package math

import "internal/bid"

// The decimal64 and decimal128 helpers of this package hold no BID logic.
// Each converts its operands to bit patterns, makes one call into
// internal/bid, which defines the result on every input, and converts the
// result back. Where a helper is described below by a special case, the
// rule lives in the documentation of internal/bid.

// d128 returns the bit pattern of x in the form internal/bid operates on.
func d128(x decimal128) bid.Uint128 {
	hi, lo := Decimal128bits(x)
	return bid.Uint128{Hi: hi, Lo: lo}
}

// fromD128 returns the decimal128 with bit pattern u.
func fromD128(u bid.Uint128) decimal128 { return Decimal128frombits(u.Hi, u.Lo) }

// Decimal64NaN returns an IEEE 754 decimal64 "not-a-number" value.
func Decimal64NaN() decimal64 { return Decimal64frombits(bid.NaN64) }

// Decimal64Inf returns a decimal64 positive infinity if sign >= 0,
// negative infinity if sign < 0.
func Decimal64Inf(sign int) decimal64 {
	if sign >= 0 {
		return Decimal64frombits(bid.Inf64)
	}
	return Decimal64frombits(bid.Neg64(bid.Inf64))
}

// IsDecimal64NaN reports whether d is a decimal64 IEEE 754
// "not-a-number" value.
func IsDecimal64NaN(d decimal64) bool { return bid.IsNaN64(Decimal64bits(d)) }

// IsDecimal64Inf reports whether d is a decimal64 infinity, according to sign.
// If sign > 0, IsDecimal64Inf reports whether d is positive infinity.
// If sign < 0, IsDecimal64Inf reports whether d is negative infinity.
// If sign == 0, IsDecimal64Inf reports whether d is either infinity.
func IsDecimal64Inf(d decimal64, sign int) bool {
	x := Decimal64bits(d)
	return bid.IsInf64(x) && (sign == 0 || (sign > 0) != bid.Signbit64(x))
}

// Decimal128NaN returns an IEEE 754 decimal128 "not-a-number" value.
func Decimal128NaN() decimal128 { return Decimal128frombits(bid.NaN128Hi, 0) }

// Decimal128Inf returns a decimal128 positive infinity if sign >= 0,
// negative infinity if sign < 0.
func Decimal128Inf(sign int) decimal128 {
	inf := bid.Uint128{Hi: bid.Inf128Hi}
	if sign < 0 {
		inf = bid.Neg128(inf)
	}
	return fromD128(inf)
}

// IsDecimal128NaN reports whether d is a decimal128 IEEE 754
// "not-a-number" value.
func IsDecimal128NaN(d decimal128) bool { return bid.IsNaN128(d128(d)) }

// IsDecimal128Inf reports whether d is a decimal128 infinity, according to sign.
// If sign > 0, IsDecimal128Inf reports whether d is positive infinity.
// If sign < 0, IsDecimal128Inf reports whether d is negative infinity.
// If sign == 0, IsDecimal128Inf reports whether d is either infinity.
func IsDecimal128Inf(d decimal128, sign int) bool {
	x := d128(d)
	return bid.IsInf128(x) && (sign == 0 || (sign > 0) != bid.Signbit128(x))
}
