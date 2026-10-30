// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime_test

import (
	"internal/bid"
	"math"
	"testing"
)

// The operators are runtime calls that pass bit patterns, the decimal128
// halves high first. These tests check, for each operator and conversion, that
// the language reaches the right kernel function with its operands in the
// right order, by comparing with a direct call on the same bits. The kernel's
// own tests decide whether the kernel is right.

var decimal64Operands = []uint64{
	0x31c0000000000001, // 1
	0x31c0000000000003, // 3
	0x31a0000000000014, // 2.0
	0xb1c0000000000007, // -7
	0x31c0000000000000, // 0
	0xb1c0000000000000, // -0
	0x31c000000000000a, // 10
	0x31c03faa2ec1e600, // 70000000000000
	0x7c00000000000000, // NaN
	0x7800000000000000, // +Inf
	0xf800000000000000, // -Inf
	0x77fb86f26fc0ffff, // 9.999999999999999E+384
	0x0000000000000001, // 1E-398
}

var decimal128Operands = []bid.Uint128{
	{Hi: 0x3040000000000000, Lo: 1},                  // 1
	{Hi: 0x3040000000000000, Lo: 3},                  // 3
	{Hi: 0x303e000000000000, Lo: 20},                 // 2.0
	{Hi: 0xb040000000000000, Lo: 7},                  // -7
	{Hi: 0x3040000000000000},                         // 0
	{Hi: 0xb040000000000000},                         // -0
	{Hi: 0x3040000000000005, Lo: 0x6bc75e2d63100001}, // 100000000000000000001
	{Hi: 0x3040000000000005, Lo: 0x6bc75e2d63100002}, // 100000000000000000002
	{Hi: 0x7c00000000000000},                         // NaN
	{Hi: 0x7800000000000000},                         // +Inf
	{Hi: 0xf800000000000000},                         // -Inf
	{Hi: 0x311fed09bead87c0, Lo: 0x378d8e63ffffffff}, // 9.99…E+6144
	{Lo: 1}, // 1E-6176
}

func TestDecimal64Operators(t *testing.T) {
	const mode = bid.ToNearestEven
	for _, xb := range decimal64Operands {
		x := math.Decimal64frombits(xb)
		for _, yb := range decimal64Operands {
			y := math.Decimal64frombits(yb)
			for _, c := range []struct {
				op        string
				got, want uint64
			}{
				{"+", math.Decimal64bits(x + y), bid.Add64(xb, yb, mode)},
				{"-", math.Decimal64bits(x - y), bid.Sub64(xb, yb, mode)},
				{"*", math.Decimal64bits(x * y), bid.Mul64(xb, yb, mode)},
				{"/", math.Decimal64bits(x / y), bid.Div64(xb, yb, mode)},
				{"min", math.Decimal64bits(min(x, y)), bid.Min64(xb, yb)},
				{"max", math.Decimal64bits(max(x, y)), bid.Max64(xb, yb)},
			} {
				if c.got != c.want {
					t.Errorf("%#x %s %#x = %#x, want %#x", xb, c.op, yb, c.got, c.want)
				}
			}
			for _, c := range []struct {
				op        string
				got, want bool
			}{
				{"==", x == y, bid.Equal64(xb, yb)},
				{"!=", x != y, !bid.Equal64(xb, yb)},
				{"<", x < y, bid.Less64(xb, yb)},
				{"<=", x <= y, bid.LessEqual64(xb, yb)},
				{">", x > y, bid.Less64(yb, xb)},
				{">=", x >= y, bid.LessEqual64(yb, xb)},
			} {
				if c.got != c.want {
					t.Errorf("%#x %s %#x = %v, want %v", xb, c.op, yb, c.got, c.want)
				}
			}
		}
	}
}

func TestDecimal128Operators(t *testing.T) {
	const mode = bid.ToNearestEven
	bits := func(d decimal128) bid.Uint128 {
		hi, lo := math.Decimal128bits(d)
		return bid.Uint128{Hi: hi, Lo: lo}
	}
	for _, xb := range decimal128Operands {
		x := math.Decimal128frombits(xb.Hi, xb.Lo)
		for _, yb := range decimal128Operands {
			y := math.Decimal128frombits(yb.Hi, yb.Lo)
			for _, c := range []struct {
				op        string
				got, want bid.Uint128
			}{
				{"+", bits(x + y), bid.Add128(xb, yb, mode)},
				{"-", bits(x - y), bid.Sub128(xb, yb, mode)},
				{"*", bits(x * y), bid.Mul128(xb, yb, mode)},
				{"/", bits(x / y), bid.Div128(xb, yb, mode)},
				{"min", bits(min(x, y)), bid.Min128(xb, yb)},
				{"max", bits(max(x, y)), bid.Max128(xb, yb)},
			} {
				if c.got != c.want {
					t.Errorf("%#x %s %#x = %#x, want %#x", xb, c.op, yb, c.got, c.want)
				}
			}
			for _, c := range []struct {
				op        string
				got, want bool
			}{
				{"==", x == y, bid.Equal128(xb, yb)},
				{"!=", x != y, !bid.Equal128(xb, yb)},
				{"<", x < y, bid.Less128(xb, yb)},
				{"<=", x <= y, bid.LessEqual128(xb, yb)},
				{">", x > y, bid.Less128(yb, xb)},
				{">=", x >= y, bid.LessEqual128(yb, xb)},
			} {
				if c.got != c.want {
					t.Errorf("%#x %s %#x = %v, want %v", xb, c.op, yb, c.got, c.want)
				}
			}
		}
	}
}

func TestDecimalConversions(t *testing.T) {
	const mode = bid.ToNearestEven
	bits := func(d decimal128) bid.Uint128 {
		hi, lo := math.Decimal128bits(d)
		return bid.Uint128{Hi: hi, Lo: lo}
	}
	same64 := func(a, b float64) bool { return a == b || a != a && b != b }
	same32 := func(a, b float32) bool { return a == b || a != a && b != b }

	for _, xb := range decimal64Operands {
		x := math.Decimal64frombits(xb)
		wantI, _ := bid.Int64From64(xb, bid.TowardZero)
		if got := int64(x); got != wantI {
			t.Errorf("int64(%#x) = %d, want %d", xb, got, wantI)
		}
		wantU, _ := bid.Uint64From64(xb, bid.TowardZero)
		if got := uint64(x); got != wantU {
			t.Errorf("uint64(%#x) = %d, want %d", xb, got, wantU)
		}
		if got, want := int8(x), int8(wantI); got != want {
			t.Errorf("int8(%#x) = %d, want %d", xb, got, want)
		}
		if got, want := float64(x), bid.Float64From64(xb, mode); !same64(got, want) {
			t.Errorf("float64(%#x) = %v, want %v", xb, got, want)
		}
		if got, want := float32(x), bid.Float32From64(xb, mode); !same32(got, want) {
			t.Errorf("float32(%#x) = %v, want %v", xb, got, want)
		}
		if got, want := bits(decimal128(x)), bid.Widen(xb); got != want {
			t.Errorf("decimal128(%#x) = %#x, want %#x", xb, got, want)
		}
	}
	for _, xb := range decimal128Operands {
		x := math.Decimal128frombits(xb.Hi, xb.Lo)
		wantI, _ := bid.Int64From128(xb, bid.TowardZero)
		if got := int64(x); got != wantI {
			t.Errorf("int64(%#x) = %d, want %d", xb, got, wantI)
		}
		wantU, _ := bid.Uint64From128(xb, bid.TowardZero)
		if got := uint64(x); got != wantU {
			t.Errorf("uint64(%#x) = %d, want %d", xb, got, wantU)
		}
		if got, want := uint16(x), uint16(wantU); got != want {
			t.Errorf("uint16(%#x) = %d, want %d", xb, got, want)
		}
		if got, want := float64(x), bid.Float64From128(xb, mode); !same64(got, want) {
			t.Errorf("float64(%#x) = %v, want %v", xb, got, want)
		}
		if got, want := float32(x), bid.Float32From128(xb, mode); !same32(got, want) {
			t.Errorf("float32(%#x) = %v, want %v", xb, got, want)
		}
		if got, want := math.Decimal64bits(decimal64(x)), bid.Narrow(xb, mode); got != want {
			t.Errorf("decimal64(%#x) = %#x, want %#x", xb, got, want)
		}
	}
	for _, i := range []int64{0, 1, -1, 12345, -12345, math.MaxInt64, math.MinInt64} {
		if got, want := math.Decimal64bits(decimal64(i)), bid.Int64To64(i, mode); got != want {
			t.Errorf("decimal64(int64 %d) = %#x, want %#x", i, got, want)
		}
		if got, want := bits(decimal128(i)), bid.Int64To128(i); got != want {
			t.Errorf("decimal128(int64 %d) = %#x, want %#x", i, got, want)
		}
		if got, want := math.Decimal64bits(decimal64(int16(i))), bid.Int64To64(int64(int16(i)), mode); got != want {
			t.Errorf("decimal64(int16 %d) = %#x, want %#x", int16(i), got, want)
		}
		u := uint64(i)
		if got, want := math.Decimal64bits(decimal64(u)), bid.Uint64To64(u, mode); got != want {
			t.Errorf("decimal64(uint64 %d) = %#x, want %#x", u, got, want)
		}
		if got, want := bits(decimal128(u)), bid.Uint64To128(u); got != want {
			t.Errorf("decimal128(uint64 %d) = %#x, want %#x", u, got, want)
		}
		if got, want := math.Decimal64bits(decimal64(uint8(u))), bid.Uint64To64(uint64(uint8(u)), mode); got != want {
			t.Errorf("decimal64(uint8 %d) = %#x, want %#x", uint8(u), got, want)
		}
	}
	for _, f := range []float64{0, 0.5, 0.1, -2, 1e23, 1e-320, math.Inf(1), math.Inf(-1), math.NaN(), math.MaxFloat64} {
		if got, want := math.Decimal64bits(decimal64(f)), bid.Float64To64(f, mode); got != want {
			t.Errorf("decimal64(float64 %v) = %#x, want %#x", f, got, want)
		}
		if got, want := bits(decimal128(f)), bid.Float64To128(f, mode); got != want {
			t.Errorf("decimal128(float64 %v) = %#x, want %#x", f, got, want)
		}
		g := float32(f)
		if got, want := math.Decimal64bits(decimal64(g)), bid.Float32To64(g, mode); got != want {
			t.Errorf("decimal64(float32 %v) = %#x, want %#x", g, got, want)
		}
		if got, want := bits(decimal128(g)), bid.Float32To128(g, mode); got != want {
			t.Errorf("decimal128(float32 %v) = %#x, want %#x", g, got, want)
		}
	}
}

// TestDecimalFloat32OneRounding checks that a conversion to float32 rounds
// once. This decimal128 is just above the midpoint of 1 and the next float32,
// so it rounds up. Through float64 it would land on the midpoint itself, which
// ties to even, downward.
func TestDecimalFloat32OneRounding(t *testing.T) {
	var one, tiny decimal128 = 1, 1e-30
	var half float64 = 1.0 / (1 << 24)
	x := one + decimal128(half) + tiny
	if got, want := float32(x), float32(1)+1.0/(1<<23); got != want {
		t.Errorf("float32(1 + 2^-24 + 1e-30) = %v, want %v", got, want)
	}
}

// TestDecimalReproducers holds results that the runtime's arithmetic got wrong
// before it used internal/bid.
func TestDecimalReproducers(t *testing.T) {
	var one, d decimal64 = 1, 70000000000000
	if got, want := math.Decimal64bits(one/d), uint64(0x2e25134734894925); got != want {
		t.Errorf("1 / 70000000000000 = %#x, want %#x (1.428571428571429E-14)", got, want)
	}
	big := math.Decimal128frombits(0x5ffe314dc6448d93, 0x38c15b0a00000000) // 1E+6144
	var ten decimal128 = 10
	if got := big * ten; !math.IsDecimal128Inf(got, 1) {
		t.Errorf("1e6144 * 10 is not +Inf")
	}
	// The exact sum has 41 digits; it rounds to 10^33 × 10^-13.
	var a, b decimal128 = 1e20, 1e-20
	hi, lo := math.Decimal128bits(a + b)
	if want := (bid.Uint128{Hi: 0x3026314dc6448d93, Lo: 0x38c15b0a00000000}); hi != want.Hi || lo != want.Lo {
		t.Errorf("1e20 + 1e-20 = %#x %#x, want %#x", hi, lo, want)
	}
}
