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
