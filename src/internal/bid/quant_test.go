// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// Tests of the operations that fix or inspect the exponent of a result:
// RoundToIntegral, Quantize, Reduce and SameQuantum (quant_tmpl.go). The
// differential tests and the conformance suite cover them; these are the
// fuzz targets, which drive the kernel and the reference model on arbitrary
// bit patterns (non-canonical encodings and signaling NaNs included) in
// every rounding mode, and the allocation check.

import (
	"internal/bid"
	"testing"
)

// quantEncode64 builds a decimal64 from a sign, coefficient and biased
// exponent without validating them, so that the fuzzer reaches every field
// value and every encoding form by construction rather than by luck.
func quantEncode64(neg bool, coeff uint64, biased uint16) uint64 {
	x := uint64(biased)&bid.ExpMask64<<bid.ExpShift64 | coeff&(1<<bid.CoeffBits64-1)
	if neg {
		x |= bid.SignMask64
	}
	return x
}

func quantEncode128(neg bool, hi, lo uint64, biased uint16) bid.Uint128 {
	x := bid.Uint128{Hi: uint64(biased)&bid.ExpMask128<<bid.ExpShift128 | hi&bid.CoeffMask128Hi, Lo: lo}
	if neg {
		x.Hi |= bid.SignMask128Hi
	}
	return x
}

func checkQuant64(t *testing.T, x, y uint64, m bid.Mode) {
	t.Helper()
	k, r := kernel{}, reference{}
	if got, want := k.roundToIntegral(dec64, lo(x), m), r.roundToIntegral(dec64, lo(x), m); got != want {
		t.Errorf("RoundToIntegral64(%#016x, %s) = %s, reference %s", x, modeName(m), display(dec64, got), display(dec64, want))
	}
	if got, want := k.quantize(dec64, lo(x), lo(y), m), r.quantize(dec64, lo(x), lo(y), m); got != want {
		t.Errorf("Quantize64(%#016x, %#016x, %s) = %s, reference %s", x, y, modeName(m), display(dec64, got), display(dec64, want))
	}
	if got, want := k.reduce(dec64, lo(x)), r.reduce(dec64, lo(x)); got != want {
		t.Errorf("Reduce64(%#016x) = %s, reference %s", x, display(dec64, got), display(dec64, want))
	}
	if got, want := k.sameQuantum(dec64, lo(x), lo(y)), r.sameQuantum(dec64, lo(x), lo(y)); got != want {
		t.Errorf("SameQuantum64(%#016x, %#016x) = %t, reference %t", x, y, got, want)
	}
}

func checkQuant128(t *testing.T, x, y bid.Uint128, m bid.Mode) {
	t.Helper()
	k, r := kernel{}, reference{}
	if got, want := k.roundToIntegral(dec128, x, m), r.roundToIntegral(dec128, x, m); got != want {
		t.Errorf("RoundToIntegral128(%v, %s) = %s, reference %s", x, modeName(m), display(dec128, got), display(dec128, want))
	}
	if got, want := k.quantize(dec128, x, y, m), r.quantize(dec128, x, y, m); got != want {
		t.Errorf("Quantize128(%v, %v, %s) = %s, reference %s", x, y, modeName(m), display(dec128, got), display(dec128, want))
	}
	if got, want := k.reduce(dec128, x), r.reduce(dec128, x); got != want {
		t.Errorf("Reduce128(%v) = %s, reference %s", x, display(dec128, got), display(dec128, want))
	}
	if got, want := k.sameQuantum(dec128, x, y), r.sameQuantum(dec128, x, y); got != want {
		t.Errorf("SameQuantum128(%v, %v) = %t, reference %t", x, y, got, want)
	}
}

// FuzzQuant64 compares the four operations with the reference model. The
// operands are given as fields so that the fuzzer controls the coefficient
// and the exponent of each independently; the raw-bits seeds add the
// special values and the non-canonical encodings.
func FuzzQuant64(f *testing.F) {
	f.Add(false, uint64(215), uint16(396), true, uint64(1), uint16(397), uint8(0)) // 2.15 quantized to 0.1
	f.Add(true, uint64(25), uint16(397), false, uint64(1), uint16(398), uint8(1))  // -2.5 to 0
	f.Add(false, uint64(1), uint16(0), false, uint64(1), uint16(767), uint8(2))    // 1E-398 to 1E+369
	f.Add(false, uint64(9999999999999999), uint16(767), false, uint64(1), uint16(766), uint8(3))
	f.Add(false, uint64(1000000000000000), uint16(767), false, uint64(1), uint16(0), uint8(4))
	f.Add(true, uint64(0), uint16(500), false, uint64(7), uint16(100), uint8(0))
	f.Add(false, uint64(1<<53-1), uint16(398), false, uint64(0), uint16(398), uint8(0)) // non-canonical coefficient
	f.Add(false, uint64(1200), uint16(767), false, uint64(1), uint16(398), uint8(0))    // reduce at MaxExp
	f.Fuzz(func(t *testing.T, xneg bool, xc uint64, xe uint16, yneg bool, yc uint64, ye uint16, mode uint8) {
		checkQuant64(t, quantEncode64(xneg, xc, xe), quantEncode64(yneg, yc, ye), bid.Mode(mode%5))
	})
}

// FuzzQuantBits64 is FuzzQuant64 on raw bit patterns, which reach the
// second encoding form, the infinities and the NaNs.
func FuzzQuantBits64(f *testing.F) {
	f.Add(bid.Inf64, bid.Inf64, uint8(0))
	f.Add(bid.NaN64, uint64(1), uint8(0))
	f.Add(bid.SNaN64|bid.SignMask64, bid.Inf64, uint8(1))
	f.Add(uint64(1), bid.Inf64|bid.SignMask64, uint8(2))
	f.Add(bid.Zero64, bid.Zero64, uint8(3))
	f.Fuzz(func(t *testing.T, x, y uint64, mode uint8) {
		checkQuant64(t, x, y, bid.Mode(mode%5))
	})
}

func FuzzQuant128(f *testing.F) {
	f.Add(false, uint64(0), uint64(215), uint16(6174), true, uint64(0), uint64(1), uint16(6175), uint8(0))
	f.Add(true, uint64(0), uint64(25), uint16(6175), false, uint64(0), uint64(1), uint16(6176), uint8(1))
	f.Add(false, uint64(0), uint64(1), uint16(0), false, uint64(0), uint64(1), uint16(12287), uint8(2))
	f.Add(false, bid.MaxCoeff128Hi, bid.MaxCoeff128Lo, uint16(12287), false, uint64(0), uint64(1), uint16(12286), uint8(3))
	f.Add(false, uint64(0x0000314DC6448D93), uint64(0x38C15B0A00000000), uint16(12287), false, uint64(0), uint64(1), uint16(0), uint8(4))
	f.Add(true, uint64(0), uint64(0), uint16(7000), false, uint64(0), uint64(7), uint16(100), uint8(0))
	f.Add(false, uint64(1<<49-1), uint64(1<<64-1), uint16(6176), false, uint64(0), uint64(0), uint16(6176), uint8(0)) // non-canonical
	f.Fuzz(func(t *testing.T, xneg bool, xhi, xlo uint64, xe uint16, yneg bool, yhi, ylo uint64, ye uint16, mode uint8) {
		checkQuant128(t, quantEncode128(xneg, xhi, xlo, xe), quantEncode128(yneg, yhi, ylo, ye), bid.Mode(mode%5))
	})
}

func FuzzQuantBits128(f *testing.F) {
	f.Add(bid.Inf128Hi, uint64(0), bid.Inf128Hi, uint64(0), uint8(0))
	f.Add(bid.NaN128Hi, uint64(0), uint64(0), uint64(1), uint8(0))
	f.Add(bid.SNaN128Hi|bid.SignMask128Hi, uint64(0), bid.Inf128Hi, uint64(0), uint8(1))
	f.Add(uint64(0), uint64(1), bid.Inf128Hi|bid.SignMask128Hi, uint64(0), uint8(2))
	f.Add(bid.Zero128Hi, uint64(0), bid.Zero128Hi, uint64(0), uint8(3))
	f.Add(uint64(3<<61), uint64(5), uint64(3<<61|1<<40), uint64(9), uint8(4)) // second form
	f.Fuzz(func(t *testing.T, xhi, xlo, yhi, ylo uint64, mode uint8) {
		checkQuant128(t, bid.Uint128{Hi: xhi, Lo: xlo}, bid.Uint128{Hi: yhi, Lo: ylo}, bid.Mode(mode%5))
	})
}

func TestQuantAllocs(t *testing.T) {
	var sink64 uint64
	var sink128 bid.Uint128
	var sinkOK bool
	x64, _ := bid.Pack64(false, 215, -2)
	y64, _ := bid.Pack64(false, 1, -1)
	x128, _ := bid.Pack128(true, bid.Uint128{Lo: 2500}, -3)
	y128, _ := bid.Pack128(false, bid.Uint128{Lo: 1}, 2)
	fn := func() {
		for m := bid.ToNearestEven; m <= bid.TowardNegative; m++ {
			sink64 ^= bid.RoundToIntegral64(x64, m)
			sink64 ^= bid.Quantize64(x64, y64, m)
			sink128.Lo ^= bid.RoundToIntegral128(x128, m).Lo
			sink128.Lo ^= bid.Quantize128(x128, y128, m).Lo
		}
		sink64 ^= bid.Reduce64(x64)
		sink128.Lo ^= bid.Reduce128(x128).Lo
		sinkOK = bid.SameQuantum64(x64, y64) != bid.SameQuantum128(x128, y128)
	}
	if n := testing.AllocsPerRun(100, fn); n != 0 {
		t.Errorf("RoundToIntegral, Quantize, Reduce and SameQuantum allocate: %v allocs per run", n)
	}
	_, _, _ = sink64, sink128, sinkOK
}
