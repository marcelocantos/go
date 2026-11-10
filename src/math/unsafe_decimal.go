// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package math

import (
	"internal/bid"
	"unsafe"
)

// In memory, a decimal value is its native encoding XORed with the native
// encoding of the integer 0 converted to its type, so that all bits clear is
// the zero value. The native encoding is BID on every target but
// linux/ppc64le, where it is DPD, the encoding of POWER's decimal
// floating-point unit. The functions below convert between the memory form
// and BID, which is what they exchange on every target.
//
// A decimal value always holds an encoding that its target's arithmetic
// produces or accepts, and where the native encoding is DPD it cannot hold
// a non-canonical BID encoding: Decimal64frombits stores the value its
// operand reads as under IEEE 754-2019 clause 3.5.2 (a coefficient above
// 10^16 - 1 reads as zero, a NaN payload of 10^15 or more as zero, and the
// reserved bits of an infinity or NaN are ignored), and Decimal64bits then
// returns the canonical encoding of that value. On every target, therefore,
// Decimal64bits(Decimal64frombits(b)) == b for every canonical b, and is
// the canonical encoding of b's value otherwise; whether a non-canonical b
// survives is the target's choice. The same holds for decimal128.

// Decimal64bits returns the IEEE 754-2019 BID (Binary Integer Decimal)
// representation of d as a uint64.
// Decimal64bits(Decimal64frombits(x)) == x for every canonical x.
func Decimal64bits(d decimal64) uint64 {
	return bid.FromNative64(*(*uint64)(unsafe.Pointer(&d)) ^ bid.NativeZero64)
}

// Decimal64frombits returns the decimal64 value corresponding to the
// IEEE 754-2019 BID encoding b.
// Decimal64frombits(Decimal64bits(x)) == x.
func Decimal64frombits(b uint64) decimal64 {
	b = bid.ToNative64(b) ^ bid.NativeZero64
	return *(*decimal64)(unsafe.Pointer(&b))
}

// Decimal128bits returns the IEEE 754-2019 BID (Binary Integer Decimal)
// representation of d as two uint64 values (hi, lo).
// Decimal128frombits(Decimal128bits(x)) == x.
func Decimal128bits(d decimal128) (hi, lo uint64) {
	p := (*[2]uint64)(unsafe.Pointer(&d))
	x := bid.FromNative128(bid.Uint128{Hi: p[1] ^ bid.NativeZero128Hi, Lo: p[0]}) // stored as [lo, hi] on every target
	return x.Hi, x.Lo
}

// Decimal128frombits returns the decimal128 value corresponding to the
// IEEE 754-2019 BID encoding given by hi and lo.
// Decimal128bits(Decimal128frombits(hi, lo)) == (hi, lo) for every
// canonical (hi, lo).
func Decimal128frombits(hi, lo uint64) decimal128 {
	x := bid.ToNative128(bid.Uint128{Hi: hi, Lo: lo})
	var d decimal128
	p := (*[2]uint64)(unsafe.Pointer(&d))
	p[0] = x.Lo
	p[1] = x.Hi ^ bid.NativeZero128Hi
	return d
}
