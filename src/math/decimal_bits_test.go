// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package math_test

import (
	"math"
	"math/decimal"
	"math/rand/v2"
	"runtime"
	"strconv"
	"testing"
)

// TestDecimalBits checks that the bits functions give the BID interchange
// encoding, as Float64bits gives the IEEE binary encoding, and agree with
// math/decimal's BID codecs.
func TestDecimalBits(t *testing.T) {
	for _, tt := range []struct {
		d    decimal64
		bits uint64
	}{
		{1, 0x31c0000000000001},
		{0, 0x31c0000000000000},
		{1.50, 0x3180000000000096},
		{-1, 0xb1c0000000000001},
	} {
		if got := math.Decimal64bits(tt.d); got != tt.bits {
			t.Errorf("Decimal64bits(%v) = %#x, want %#x", tt.d, got, tt.bits)
		}
		if got := math.Decimal64frombits(tt.bits); math.Decimal64bits(got) != tt.bits || got != tt.d {
			t.Errorf("Decimal64frombits(%#x) = %v", tt.bits, got)
		}
	}
	if hi, lo := math.Decimal128bits(1); hi != 0x3040000000000000 || lo != 1 {
		t.Errorf("Decimal128bits(1) = %#x, %#x, want 0x3040000000000000, 1", hi, lo)
	}

	// Random cohorts: the encoding round-trips, keeps the quantum, and is
	// math/decimal's.
	r := rand.New(rand.NewPCG(1, 2))
	for range 10000 {
		coeff := r.Int64N(1e16)
		e := r.IntN(700) - 398
		s := strconv.FormatInt(coeff, 10) + "e" + strconv.Itoa(e)
		if r.IntN(2) == 0 {
			s = "-" + s
		}
		d, err := strconv.ParseDecimal(s, 64)
		if err != nil {
			continue // beyond the range
		}
		d64 := decimal64(d)
		b := math.Decimal64bits(d64)
		if b != decimal.BID64bits(d64) {
			t.Fatalf("Decimal64bits(%s) = %#x, BID64bits gives %#x", s, b, decimal.BID64bits(d64))
		}
		back := math.Decimal64frombits(b)
		if math.Decimal64bits(back) != b || !decimal.SameQuantum(back, d64) || back != d64 {
			t.Fatalf("Decimal64frombits(Decimal64bits(%s)) = %v", s, back)
		}
		hi, lo := math.Decimal128bits(d)
		if h2, l2 := decimal.BID128bits(d); hi != h2 || lo != l2 {
			t.Fatalf("Decimal128bits(%s) = %#x %#x, BID128bits gives %#x %#x", s, hi, lo, h2, l2)
		}
		if q := math.Decimal128frombits(hi, lo); q != d || !decimal.SameQuantum(q, d) {
			t.Fatalf("Decimal128frombits(Decimal128bits(%s)) = %v", s, q)
		}
	}

	// On a target that stores BID every pattern survives, as for
	// Float64frombits: non-canonical encodings, NaN payloads and all.
	if runtime.GOARCH != "ppc64le" {
		for range 10000 {
			b, lo := r.Uint64(), r.Uint64()
			if got := math.Decimal64bits(math.Decimal64frombits(b)); got != b {
				t.Fatalf("decimal64 pattern %#x came back %#x", b, got)
			}
			if h2, l2 := math.Decimal128bits(math.Decimal128frombits(b, lo)); h2 != b || l2 != lo {
				t.Fatalf("decimal128 pattern (%#x, %#x) came back (%#x, %#x)", b, lo, h2, l2)
			}
		}
	}
}
