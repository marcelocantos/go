// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package math

import (
	"internal/bid"
	"unsafe"
)

// Decimal64bits returns the IEEE 754 binary integer decimal (BID)
// interchange encoding of d, as Float64bits returns the IEEE 754 binary
// encoding of a float64, the same on every target. It is the same function as
// math/decimal's BID64bits. On a target whose decimal64 is stored in BID
// every encoding bit is preserved, so
// Decimal64bits(Decimal64frombits(x)) == x for every x; on one that stores
// DPD (ppc64le) the result is the canonical BID encoding of d's sign,
// coefficient, exponent and NaN payload. How a value is stored in memory is
// internal to the implementation.
func Decimal64bits(d decimal64) uint64 {
	return bid.FromNative64(*(*uint64)(unsafe.Pointer(&d)) ^ bid.NativeZero64)
}

// Decimal64frombits returns the decimal64 whose BID interchange encoding is
// b, as Float64frombits returns the float64 of an IEEE 754 binary encoding.
// It is the same function as math/decimal's BID64frombits. On a target that
// stores BID every bit pattern is preserved, non-canonical encodings
// included, and Decimal64bits(Decimal64frombits(b)) == b; on one that stores
// DPD, b is read as the IEEE value it denotes.
func Decimal64frombits(b uint64) decimal64 {
	w := bid.ToNative64(b) ^ bid.NativeZero64
	return *(*decimal64)(unsafe.Pointer(&w))
}

// Decimal128bits returns the BID interchange encoding of d as its high and
// low words, independently of the target's byte order. See [Decimal64bits].
// It is the same function as math/decimal's BID128bits.
func Decimal128bits(d decimal128) (hi, lo uint64) {
	p := (*[2]uint64)(unsafe.Pointer(&d)) // decimal128 is stored as [lo, hi]
	x := bid.FromNative128(bid.Uint128{Hi: p[1] ^ bid.NativeZero128Hi, Lo: p[0]})
	return x.Hi, x.Lo
}

// Decimal128frombits returns the decimal128 whose BID interchange encoding
// has the words hi and lo. See [Decimal64frombits]. It is the same function
// as math/decimal's BID128frombits.
func Decimal128frombits(hi, lo uint64) decimal128 {
	x := bid.ToNative128(bid.Uint128{Hi: hi, Lo: lo})
	var d decimal128
	p := (*[2]uint64)(unsafe.Pointer(&d))
	p[0], p[1] = x.Lo, x.Hi^bid.NativeZero128Hi
	return d
}
