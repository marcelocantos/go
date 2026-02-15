// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package math

// IEEE 754-2008 decimal64 constants and utility functions.
// The decimal64 type uses BID (Binary Integer Decimal) encoding.

const (
	// decimal64 BID encoding constants.
	uvdnan  uint64 = 0x7C00000000000000 // quiet NaN
	uvdinf  uint64 = 0x7800000000000000 // +Infinity
	uvdsign uint64 = 1 << 63            // sign bit

	// Mask covering bits [62:58] — used to distinguish NaN, Inf, and normal values.
	// NaN has these 5 bits as 11111, Inf has 11110.
	uvdspecial uint64 = 0x7C00000000000000
)

// Decimal64NaN returns an IEEE 754 decimal64 "not-a-number" value.
func Decimal64NaN() decimal64 { return Decimal64frombits(uvdnan) }

// Decimal64Inf returns a decimal64 positive infinity if sign >= 0,
// negative infinity if sign < 0.
func Decimal64Inf(sign int) decimal64 {
	var v uint64
	if sign >= 0 {
		v = uvdinf
	} else {
		v = uvdinf | uvdsign
	}
	return Decimal64frombits(v)
}

// IsDecimal64NaN reports whether d is a decimal64 IEEE 754
// "not-a-number" value.
func IsDecimal64NaN(d decimal64) bool {
	// NaN: bits[62:58] == 11111 (0x7C prefix)
	return Decimal64bits(d)&uvdspecial == uvdnan
}

// IsDecimal64Inf reports whether d is a decimal64 infinity, according to sign.
// If sign > 0, IsDecimal64Inf reports whether d is positive infinity.
// If sign < 0, IsDecimal64Inf reports whether d is negative infinity.
// If sign == 0, IsDecimal64Inf reports whether d is either infinity.
func IsDecimal64Inf(d decimal64, sign int) bool {
	x := Decimal64bits(d)
	// Infinity: bits[62:58] == 11110 (0x78 prefix, but not 0x7C which is NaN).
	isInf := x&uvdspecial == uvdinf
	if !isInf {
		return false
	}
	if sign == 0 {
		return true
	}
	isNeg := x&uvdsign != 0
	if sign > 0 {
		return !isNeg
	}
	return isNeg
}

// IEEE 754-2008 decimal128 constants and utility functions.
// The decimal128 type uses BID (Binary Integer Decimal) encoding.

const (
	// decimal128 BID encoding constants.
	// These operate on the high 64 bits of the 128-bit representation.
	uvd128nan  uint64 = 0x7C00000000000000 // quiet NaN (high word)
	uvd128inf  uint64 = 0x7800000000000000 // +Infinity (high word)
	uvd128sign uint64 = 1 << 63            // sign bit (high word)

	// Mask covering bits [62:58] of the high word.
	uvd128special uint64 = 0x7C00000000000000
)

// Decimal128NaN returns an IEEE 754 decimal128 "not-a-number" value.
func Decimal128NaN() decimal128 { return Decimal128frombits(uvd128nan, 0) }

// Decimal128Inf returns a decimal128 positive infinity if sign >= 0,
// negative infinity if sign < 0.
func Decimal128Inf(sign int) decimal128 {
	var v uint64
	if sign >= 0 {
		v = uvd128inf
	} else {
		v = uvd128inf | uvd128sign
	}
	return Decimal128frombits(v, 0)
}

// IsDecimal128NaN reports whether d is a decimal128 IEEE 754
// "not-a-number" value.
func IsDecimal128NaN(d decimal128) bool {
	hi, _ := Decimal128bits(d)
	// NaN: bits[62:58] == 11111 (0x7C prefix in high word)
	return hi&uvd128special == uvd128nan
}

// IsDecimal128Inf reports whether d is a decimal128 infinity, according to sign.
// If sign > 0, IsDecimal128Inf reports whether d is positive infinity.
// If sign < 0, IsDecimal128Inf reports whether d is negative infinity.
// If sign == 0, IsDecimal128Inf reports whether d is either infinity.
func IsDecimal128Inf(d decimal128, sign int) bool {
	hi, _ := Decimal128bits(d)
	// Infinity: bits[62:58] == 11110 (0x78 prefix, but not 0x7C which is NaN).
	isInf := hi&uvd128special == uvd128inf
	if !isInf {
		return false
	}
	if sign == 0 {
		return true
	}
	isNeg := hi&uvd128sign != 0
	if sign > 0 {
		return !isNeg
	}
	return isNeg
}
