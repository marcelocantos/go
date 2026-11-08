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

// Decimal64bits returns the exact stored representation of d, including the
// zero-value XOR. The representation is implementation-dependent; see
// math/decimal.NativeRepresentation and the explicit BID and DPD codecs in
// math/decimal. No encoding conversion or canonicalization is performed.
// Decimal64bits(Decimal64frombits(x)) == x for every x.
func Decimal64bits(d decimal64) uint64 { return *(*uint64)(unsafe.Pointer(&d)) }

// Decimal64frombits returns the decimal64 with the exact stored bits b.
// Every bit pattern is preserved, including non-canonical encodings.
// Decimal64frombits(Decimal64bits(x)) has exactly the same bits as x.
func Decimal64frombits(b uint64) decimal64 { return *(*decimal64)(unsafe.Pointer(&b)) }

// Decimal128bits returns the exact stored representation of d as its high
// and low uint64 words, including the zero-value XOR. See [Decimal64bits].
// The word order of the result is independent of the target's byte order.
func Decimal128bits(d decimal128) (hi, lo uint64) {
	p := (*[2]uint64)(unsafe.Pointer(&d)) // decimal128 is stored as [lo, hi]
	return p[1], p[0]
}

// Decimal128frombits returns the decimal128 with the exact stored words hi
// and lo. Every bit pattern is preserved without encoding conversion.
// Decimal128bits(Decimal128frombits(hi, lo)) returns (hi, lo) for every pair.
func Decimal128frombits(hi, lo uint64) decimal128 {
	var d decimal128
	p := (*[2]uint64)(unsafe.Pointer(&d))
	p[0], p[1] = lo, hi
	return d
}
