// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package math

import "unsafe"

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
