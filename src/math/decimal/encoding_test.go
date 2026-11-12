// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package decimal_test

import (
	"math"
	"math/decimal"
	"math/rand/v2"
	"runtime"
	"testing"
)

func TestDecimalEncoding(t *testing.T) {
	want := decimal.BIDXOR
	if runtime.GOARCH == "ppc64le" {
		want = decimal.DPDXOR
	}
	if decimal.NativeRepresentation != want {
		t.Fatalf("native representation %v, want %v", decimal.NativeRepresentation, want)
	}
	// Encodings written independently of the conversion routines. Integer 750
	// is declet 0x3d0 in IEEE's DPD table; the exponent here is zero.
	for _, c := range []struct {
		x        decimal64
		bid, dpd uint64
	}{
		{0, 0x31c0000000000000, 0x2238000000000000},
		{1, 0x31c0000000000001, 0x2238000000000001},
		{-1, 0xb1c0000000000001, 0xa238000000000001},
		{750, 0x31c00000000002ee, 0x22380000000003d0},
	} {
		if got := decimal.BID64bits(c.x); got != c.bid {
			t.Errorf("BID64bits(%v) = %#x, want %#x", c.x, got, c.bid)
		}
		if got := decimal.DPD64bits(c.x); got != c.dpd {
			t.Errorf("DPD64bits(%v) = %#x, want %#x", c.x, got, c.dpd)
		}
		if a, b := decimal.BID64frombits(c.bid), decimal.DPD64frombits(c.dpd); math.Decimal64bits(a) != math.Decimal64bits(c.x) || math.Decimal64bits(b) != math.Decimal64bits(c.x) {
			t.Errorf("decimal64 codecs disagree for %v", c.x)
		}
		q := decimal128(c.x)
		// These coefficients fit in the low word, so the high words encode
		// only the exponent and sign at either width.
		bidHi := uint64(0x3040000000000000) | c.bid&(1<<63)
		dpdHi := uint64(0x2208000000000000) | c.dpd&(1<<63)
		if hi, lo := decimal.BID128bits(q); hi != bidHi || lo != c.bid&0x3ff {
			t.Errorf("BID128bits(%v) = (%#x,%#x)", q, hi, lo)
		}
		if hi, lo := decimal.DPD128bits(q); hi != dpdHi || lo != c.dpd&0x3ff {
			t.Errorf("DPD128bits(%v) = (%#x,%#x)", q, hi, lo)
		}
		if a, b := decimal.BID128frombits(bidHi, c.bid&0x3ff), decimal.DPD128frombits(dpdHi, c.dpd&0x3ff); a != q || b != q {
			t.Errorf("decimal128 codecs disagree for %v", q)
		}
	}
	// Quantum and payload survive crossing encodings, not just numeric value.
	for _, x := range []decimal128{0.00, 1.50, -1234567890.123456789012345678901234, 1e-6176, 1e6144,
		decimal.Inf[decimal128](-1), decimal.NaNWithPayload[decimal128](123), decimal.SignalingNaNWithPayload[decimal128](999)} {
		hi, lo := decimal.BID128bits(x)
		dh, dl := decimal.DPD128bits(x)
		got := decimal.DPD128frombits(dh, dl)
		if gh, gl := decimal.BID128bits(got); gh != hi || gl != lo {
			t.Errorf("BID/DPD crossing = (%#x,%#x), want (%#x,%#x)", gh, gl, hi, lo)
		}
	}
}

func TestNativeEncodingPreservesEveryBit(t *testing.T) {
	// Matching the native encoding must not silently canonicalize a bit
	// pattern; crossing encodings is allowed to canonicalize its IEEE value.
	r := rand.New(rand.NewPCG(3, 4))
	for range 10000 {
		hi, lo := r.Uint64(), r.Uint64()
		if decimal.NativeRepresentation == decimal.BIDXOR {
			if b := decimal.BID64bits(decimal.BID64frombits(lo)); b != lo {
				t.Fatal("native BID64 encoding changed")
			}
			if h, l := decimal.BID128bits(decimal.BID128frombits(hi, lo)); h != hi || l != lo {
				t.Fatal("native BID128 encoding changed")
			}
		} else {
			if b := decimal.DPD64bits(decimal.DPD64frombits(lo)); b != lo {
				t.Fatal("native DPD64 encoding changed")
			}
			if h, l := decimal.DPD128bits(decimal.DPD128frombits(hi, lo)); h != hi || l != lo {
				t.Fatal("native DPD128 encoding changed")
			}
		}
	}
}
