// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package math_test

import (
	"math"
	"math/rand/v2"
	"testing"
	"unsafe"
)

func TestDecimalStoredBits(t *testing.T) {
	if math.Decimal64bits(0) != 0 {
		t.Fatal("decimal64 zero has nonzero stored bits")
	}
	if hi, lo := math.Decimal128bits(0); hi != 0 || lo != 0 {
		t.Fatal("decimal128 zero has nonzero stored bits")
	}
	// All patterns must survive: finite values, non-canonical encodings,
	// reserved bits, infinities, and both kinds of NaN with any payload.
	r := rand.New(rand.NewPCG(1, 2))
	patterns := []uint64{0, ^uint64(0), 0x6c7386f26fc10000, 0x22380000000003ff, 0x7dffffffffffffff, 0x7e00000000000123}
	for range 10000 {
		patterns = append(patterns, r.Uint64())
	}
	for _, b := range patterns {
		d := math.Decimal64frombits(b)
		if got := *(*uint64)(unsafe.Pointer(&d)); got != b {
			t.Fatalf("decimal64 memory %#x, want %#x", got, b)
		}
		if got := math.Decimal64bits(d); got != b {
			t.Fatalf("decimal64 bits round trip %#x, want %#x", got, b)
		}
		lo := r.Uint64()
		q := math.Decimal128frombits(b, lo)
		words := *(*[2]uint64)(unsafe.Pointer(&q))
		if words != [2]uint64{lo, b} {
			t.Fatalf("decimal128 memory %#x, want [lo=%#x hi=%#x]", words, lo, b)
		}
		if hi2, lo2 := math.Decimal128bits(q); hi2 != b || lo2 != lo {
			t.Fatalf("decimal128 bits round trip (%#x,%#x), want (%#x,%#x)", hi2, lo2, b, lo)
		}
	}
}
