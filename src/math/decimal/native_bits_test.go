// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package decimal_test

import (
	"math/decimal"
	"math/rand/v2"
	"testing"
	"unsafe"
)

func TestNativeBits(t *testing.T) {
	if decimal.Native64bits(0) != 0 {
		t.Fatal("decimal64 zero has nonzero stored bits")
	}
	if hi, lo := decimal.Native128bits(0); hi != 0 || lo != 0 {
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
		d := decimal.Native64frombits(b)
		if got := *(*uint64)(unsafe.Pointer(&d)); got != b {
			t.Fatalf("decimal64 memory %#x, want %#x", got, b)
		}
		if got := decimal.Native64bits(d); got != b {
			t.Fatalf("decimal64 bits round trip %#x, want %#x", got, b)
		}
		lo := r.Uint64()
		q := decimal.Native128frombits(b, lo)
		words := *(*[2]uint64)(unsafe.Pointer(&q))
		if words != [2]uint64{lo, b} {
			t.Fatalf("decimal128 memory %#x, want [lo=%#x hi=%#x]", words, lo, b)
		}
		if hi2, lo2 := decimal.Native128bits(q); hi2 != b || lo2 != lo {
			t.Fatalf("decimal128 bits round trip (%#x,%#x), want (%#x,%#x)", hi2, lo2, b, lo)
		}
	}
}

// TestNativeBitsFixed pins the stored bits of a few values. They
// depend on the target's representation, BID or DPD, and on nothing else:
// a big-endian target gives the same words as a little-endian one.
func TestNativeBitsFixed(t *testing.T) {
	type want struct {
		b64    uint64
		hi, lo uint64
	}
	cases := []struct {
		d64  decimal64
		d128 decimal128
		bid  want // IEEE BID XORed with the BID encoding of 0E+0; decimal128 UBD (experiment)
		dpd  want // IEEE DPD XORed with the DPD encoding of 0E+0
	}{
		{1, 1, want{0x1, 0, 0x1}, want{0x1, 0, 0x1}},
		{-1.5, -1.5, want{0x806000000000000f, 0xfffe000000000000, 0xf}, want{0x800c000000000015, 0x800fc00000000000, 0x15}},
		{19.99, 19.99, want{0x00400000000007cf, 0xfff8000000000000, 0x7cf}, want{0x00080000000004ff, 0x000f800000000000, 0x4ff}},
	}
	for _, c := range cases {
		w := c.bid
		if decimal.NativeRepresentation == decimal.DPDXOR {
			w = c.dpd
		}
		if got := decimal.Native64bits(c.d64); got != w.b64 {
			t.Errorf("Native64bits(%v) = %#x, want %#x", c.d64, got, w.b64)
		}
		if hi, lo := decimal.Native128bits(c.d128); hi != w.hi || lo != w.lo {
			t.Errorf("Native128bits(%v) = %#x, %#x, want %#x, %#x", c.d128, hi, lo, w.hi, w.lo)
		}
	}
}
