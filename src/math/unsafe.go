// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package math

import "unsafe"

// Despite being an exported symbol,
// Float32bits is linknamed by widely used packages.
// Notable members of the hall of shame include:
//   - gitee.com/quant1x/num
//
// Do not remove or change the type signature.
// See go.dev/issue/67401.
//
// Note that this comment is not part of the doc comment.
//
//go:linkname Float32bits

// Float32bits returns the IEEE 754 binary representation of f,
// with the sign bit of f and the result in the same bit position.
// Float32bits(Float32frombits(x)) == x.
func Float32bits(f float32) uint32 { return *(*uint32)(unsafe.Pointer(&f)) }

// Float32frombits returns the floating-point number corresponding
// to the IEEE 754 binary representation b, with the sign bit of b
// and the result in the same bit position.
// Float32frombits(Float32bits(x)) == x.
func Float32frombits(b uint32) float32 { return *(*float32)(unsafe.Pointer(&b)) }

// Float64bits returns the IEEE 754 binary representation of f,
// with the sign bit of f and the result in the same bit position,
// and Float64bits(Float64frombits(x)) == x.
func Float64bits(f float64) uint64 { return *(*uint64)(unsafe.Pointer(&f)) }

// Float64frombits returns the floating-point number corresponding
// to the IEEE 754 binary representation b, with the sign bit of b
// and the result in the same bit position.
// Float64frombits(Float64bits(x)) == x.
func Float64frombits(b uint64) float64 { return *(*float64)(unsafe.Pointer(&b)) }

// Decimal64bits returns the IEEE 754-2008 BID (Binary Integer Decimal)
// representation of d as a uint64.
// Decimal64bits(Decimal64frombits(x)) == x.
func Decimal64bits(d decimal64) uint64 { return *(*uint64)(unsafe.Pointer(&d)) }

// Decimal64frombits returns the decimal64 value corresponding to the
// IEEE 754-2008 BID encoding b.
// Decimal64frombits(Decimal64bits(x)) == x.
func Decimal64frombits(b uint64) decimal64 { return *(*decimal64)(unsafe.Pointer(&b)) }

// Decimal128bits returns the IEEE 754-2008 BID (Binary Integer Decimal)
// representation of d as two uint64 values (hi, lo).
// Decimal128frombits(Decimal128bits(x)) == x.
func Decimal128bits(d decimal128) (hi, lo uint64) {
	p := (*[2]uint64)(unsafe.Pointer(&d))
	return p[1], p[0] // stored as [lo, hi] in little-endian memory
}

// Decimal128frombits returns the decimal128 value corresponding to the
// IEEE 754-2008 BID encoding given by hi and lo.
// Decimal128bits(Decimal128frombits(hi, lo)) == (hi, lo).
func Decimal128frombits(hi, lo uint64) decimal128 {
	var d decimal128
	p := (*[2]uint64)(unsafe.Pointer(&d))
	p[0] = lo
	p[1] = hi
	return d
}
