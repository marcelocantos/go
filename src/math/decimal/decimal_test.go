// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package decimal_test

import (
	"fmt"
	. "math"
	"math/decimal"
	"testing"
	"unsafe"
)

func TestDecimal64BitsRoundTrip(t *testing.T) {
	for _, v := range []decimal64{0, 1, -1, 3.14, -3.14, 0.001, 9999999999999999} {
		bits := Decimal64bits(v)
		got := Decimal64frombits(bits)
		if got != v {
			t.Errorf("Decimal64frombits(Decimal64bits(%v)) = %v", v, got)
		}
	}
}

func TestDecimal128BitsRoundTrip(t *testing.T) {
	for _, v := range []decimal128{0, 1, -1, 3.14, -3.14, 0.001, 9999999999999999} {
		hi, lo := Decimal128bits(v)
		got := Decimal128frombits(hi, lo)
		if got != v {
			t.Errorf("Decimal128frombits(Decimal128bits(%v)) = %v", v, got)
		}
	}
}

func TestIsDecimal64NaN(t *testing.T) {
	if !decimal.IsNaN[decimal64](decimal.NaN[decimal64]()) {
		t.Error("decimal.IsNaN[decimal64](decimal.NaN[decimal64]()) = false")
	}
	if decimal.IsNaN[decimal64](decimal64(0)) {
		t.Error("decimal.IsNaN[decimal64](0) = true")
	}
	if decimal.IsNaN[decimal64](decimal64(1)) {
		t.Error("decimal.IsNaN[decimal64](1) = true")
	}
	if decimal.IsNaN[decimal64](decimal.Inf[decimal64](1)) {
		t.Error("decimal.IsNaN[decimal64](+Inf) = true")
	}
	if decimal.IsNaN[decimal64](decimal.Inf[decimal64](-1)) {
		t.Error("decimal.IsNaN[decimal64](-Inf) = true")
	}
}

func TestIsDecimal64Inf(t *testing.T) {
	pinf := decimal.Inf[decimal64](1)
	ninf := decimal.Inf[decimal64](-1)
	nan := decimal.NaN[decimal64]()
	zero := decimal64(0)
	one := decimal64(1)

	tests := []struct {
		d    decimal64
		sign int
		want bool
		name string
	}{
		{pinf, 0, true, "+Inf sign=0"},
		{pinf, 1, true, "+Inf sign=1"},
		{pinf, -1, false, "+Inf sign=-1"},
		{ninf, 0, true, "-Inf sign=0"},
		{ninf, 1, false, "-Inf sign=1"},
		{ninf, -1, true, "-Inf sign=-1"},
		{nan, 0, false, "NaN sign=0"},
		{zero, 0, false, "0 sign=0"},
		{one, 0, false, "1 sign=0"},
	}
	for _, tt := range tests {
		got := decimal.IsInf[decimal64](tt.d, tt.sign)
		if got != tt.want {
			t.Errorf("decimal.IsInf[decimal64](%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsDecimal128NaN(t *testing.T) {
	if !decimal.IsNaN[decimal128](decimal.NaN[decimal128]()) {
		t.Error("decimal.IsNaN[decimal128](decimal.NaN[decimal128]()) = false")
	}
	if decimal.IsNaN[decimal128](decimal128(0)) {
		t.Error("decimal.IsNaN[decimal128](0) = true")
	}
	if decimal.IsNaN[decimal128](decimal128(1)) {
		t.Error("decimal.IsNaN[decimal128](1) = true")
	}
	if decimal.IsNaN[decimal128](decimal.Inf[decimal128](1)) {
		t.Error("decimal.IsNaN[decimal128](+Inf) = true")
	}
	if decimal.IsNaN[decimal128](decimal.Inf[decimal128](-1)) {
		t.Error("decimal.IsNaN[decimal128](-Inf) = true")
	}
}

func TestIsDecimal128Inf(t *testing.T) {
	pinf := decimal.Inf[decimal128](1)
	ninf := decimal.Inf[decimal128](-1)
	nan := decimal.NaN[decimal128]()
	zero := decimal128(0)
	one := decimal128(1)

	tests := []struct {
		d    decimal128
		sign int
		want bool
		name string
	}{
		{pinf, 0, true, "+Inf sign=0"},
		{pinf, 1, true, "+Inf sign=1"},
		{pinf, -1, false, "+Inf sign=-1"},
		{ninf, 0, true, "-Inf sign=0"},
		{ninf, 1, false, "-Inf sign=1"},
		{ninf, -1, true, "-Inf sign=-1"},
		{nan, 0, false, "NaN sign=0"},
		{zero, 0, false, "0 sign=0"},
		{one, 0, false, "1 sign=0"},
	}
	for _, tt := range tests {
		got := decimal.IsInf[decimal128](tt.d, tt.sign)
		if got != tt.want {
			t.Errorf("decimal.IsInf[decimal128](%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// --- Abs, Copysign, Signbit, Dim ---

func TestDecimal64Abs(t *testing.T) {
	tests := []struct {
		in   decimal64
		want decimal64
	}{
		{0, 0},
		{1, 1},
		{-1, 1},
		{3.14, 3.14},
		{-3.14, 3.14},
		{decimal.Inf[decimal64](1), decimal.Inf[decimal64](1)},
		{decimal.Inf[decimal64](-1), decimal.Inf[decimal64](1)},
	}
	for _, tt := range tests {
		got := decimal.Abs[decimal64](tt.in)
		if got != tt.want {
			t.Errorf("decimal.Abs[decimal64](%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
	if !decimal.IsNaN[decimal64](decimal.Abs[decimal64](decimal.NaN[decimal64]())) {
		t.Error("decimal.Abs[decimal64](NaN) should be NaN")
	}
}

func TestDecimal128Abs(t *testing.T) {
	tests := []struct {
		in   decimal128
		want decimal128
	}{
		{0, 0},
		{1, 1},
		{-1, 1},
		{3.14, 3.14},
		{-3.14, 3.14},
	}
	for _, tt := range tests {
		got := decimal.Abs[decimal128](tt.in)
		if got != tt.want {
			t.Errorf("decimal.Abs[decimal128](%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestDecimal64Copysign(t *testing.T) {
	tests := []struct {
		f, sign decimal64
		want    decimal64
	}{
		{1, 1, 1},
		{1, -1, -1},
		{-1, 1, 1},
		{-1, -1, -1},
		{3.14, -1, -3.14},
		{-3.14, 1, 3.14},
	}
	for _, tt := range tests {
		got := decimal.Copysign[decimal64](tt.f, tt.sign)
		if got != tt.want {
			t.Errorf("decimal.Copysign[decimal64](%v, %v) = %v, want %v", tt.f, tt.sign, got, tt.want)
		}
	}
	// Copysign with negative zero created via Copysign itself
	negzero := decimal.Copysign[decimal64](0, -1)
	if !decimal.Signbit[decimal64](negzero) {
		t.Error("decimal.Copysign[decimal64](0, -1) should produce negative zero")
	}
}

func TestDecimal128Copysign(t *testing.T) {
	tests := []struct {
		f, sign decimal128
		want    decimal128
	}{
		{1, 1, 1},
		{1, -1, -1},
		{-1, 1, 1},
		{-1, -1, -1},
	}
	for _, tt := range tests {
		got := decimal.Copysign[decimal128](tt.f, tt.sign)
		if got != tt.want {
			t.Errorf("decimal.Copysign[decimal128](%v, %v) = %v, want %v", tt.f, tt.sign, got, tt.want)
		}
	}
}

func TestDecimal64Signbit(t *testing.T) {
	tests := []struct {
		in   decimal64
		want bool
	}{
		{0, false},
		{1, false},
		{-1, true},
		{decimal.Inf[decimal64](1), false},
		{decimal.Inf[decimal64](-1), true},
	}
	for _, tt := range tests {
		got := decimal.Signbit[decimal64](tt.in)
		if got != tt.want {
			t.Errorf("decimal.Signbit[decimal64](%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestDecimal128Signbit(t *testing.T) {
	tests := []struct {
		in   decimal128
		want bool
	}{
		{0, false},
		{1, false},
		{-1, true},
	}
	for _, tt := range tests {
		got := decimal.Signbit[decimal128](tt.in)
		if got != tt.want {
			t.Errorf("decimal.Signbit[decimal128](%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestDecimal64Dim(t *testing.T) {
	tests := []struct {
		x, y decimal64
		want decimal64
	}{
		{5, 3, 2},
		{3, 5, 0},
		{0, 0, 0},
		{-1, -3, 2},
		{-3, -1, 0},
	}
	for _, tt := range tests {
		got := decimal.Dim[decimal64](tt.x, tt.y)
		if got != tt.want {
			t.Errorf("decimal.Dim[decimal64](%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
	if !decimal.IsNaN[decimal64](decimal.Dim[decimal64](decimal.NaN[decimal64](), 1)) {
		t.Error("decimal.Dim[decimal64](NaN, 1) should be NaN")
	}
	if !decimal.IsNaN[decimal64](decimal.Dim[decimal64](1, decimal.NaN[decimal64]())) {
		t.Error("decimal.Dim[decimal64](1, NaN) should be NaN")
	}
	if !decimal.IsNaN[decimal64](decimal.Dim[decimal64](decimal.Inf[decimal64](1), decimal.Inf[decimal64](1))) {
		t.Error("decimal.Dim[decimal64](+Inf, +Inf) should be NaN")
	}
}

func TestDecimal128Dim(t *testing.T) {
	tests := []struct {
		x, y decimal128
		want decimal128
	}{
		{5, 3, 2},
		{3, 5, 0},
		{0, 0, 0},
	}
	for _, tt := range tests {
		got := decimal.Dim[decimal128](tt.x, tt.y)
		if got != tt.want {
			t.Errorf("decimal.Dim[decimal128](%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}

// --- Rounding functions ---

func TestDecimal64Trunc(t *testing.T) {
	tests := []struct {
		in   decimal64
		want decimal64
	}{
		{0, 0},
		{1, 1},
		{-1, -1},
		{1.5, 1},
		{-1.5, -1},
		{2.9, 2},
		{-2.9, -2},
		{0.1, 0},
		{-0.1, 0},
		{0.999, 0},
		{100, 100},
		{decimal.Inf[decimal64](1), decimal.Inf[decimal64](1)},
		{decimal.Inf[decimal64](-1), decimal.Inf[decimal64](-1)},
	}
	for _, tt := range tests {
		got := decimal.Trunc[decimal64](tt.in)
		if got != tt.want {
			t.Errorf("decimal.Trunc[decimal64](%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
	if !decimal.IsNaN[decimal64](decimal.Trunc[decimal64](decimal.NaN[decimal64]())) {
		t.Error("decimal.Trunc[decimal64](NaN) should be NaN")
	}
}

func TestDecimal64Floor(t *testing.T) {
	tests := []struct {
		in   decimal64
		want decimal64
	}{
		{0, 0},
		{1, 1},
		{-1, -1},
		{1.5, 1},
		{-1.5, -2},
		{2.9, 2},
		{-2.9, -3},
		{0.1, 0},
		{-0.1, -1},
		{100, 100},
		{decimal.Inf[decimal64](1), decimal.Inf[decimal64](1)},
		{decimal.Inf[decimal64](-1), decimal.Inf[decimal64](-1)},
	}
	for _, tt := range tests {
		got := decimal.Floor[decimal64](tt.in)
		if got != tt.want {
			t.Errorf("decimal.Floor[decimal64](%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
	if !decimal.IsNaN[decimal64](decimal.Floor[decimal64](decimal.NaN[decimal64]())) {
		t.Error("decimal.Floor[decimal64](NaN) should be NaN")
	}
}

func TestDecimal64Ceil(t *testing.T) {
	tests := []struct {
		in   decimal64
		want decimal64
	}{
		{0, 0},
		{1, 1},
		{-1, -1},
		{1.5, 2},
		{-1.5, -1},
		{2.9, 3},
		{-2.9, -2},
		{0.1, 1},
		{-0.1, 0},
		{100, 100},
		{decimal.Inf[decimal64](1), decimal.Inf[decimal64](1)},
		{decimal.Inf[decimal64](-1), decimal.Inf[decimal64](-1)},
	}
	for _, tt := range tests {
		got := decimal.Ceil[decimal64](tt.in)
		if got != tt.want {
			t.Errorf("decimal.Ceil[decimal64](%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
	if !decimal.IsNaN[decimal64](decimal.Ceil[decimal64](decimal.NaN[decimal64]())) {
		t.Error("decimal.Ceil[decimal64](NaN) should be NaN")
	}
}

func TestDecimal64Round(t *testing.T) {
	tests := []struct {
		in   decimal64
		want decimal64
	}{
		{0, 0},
		{1, 1},
		{-1, -1},
		{1.4, 1},
		{1.5, 2}, // ties away from zero
		{1.6, 2},
		{-1.4, -1},
		{-1.5, -2},
		{-1.6, -2},
		{2.5, 3},
		{-2.5, -3},
		{0.4, 0},
		{0.5, 1},
		{-0.5, -1},
		{100, 100},
	}
	for _, tt := range tests {
		got := decimal.Round[decimal64](tt.in)
		if got != tt.want {
			t.Errorf("decimal.Round[decimal64](%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
	if !decimal.IsNaN[decimal64](decimal.Round[decimal64](decimal.NaN[decimal64]())) {
		t.Error("decimal.Round[decimal64](NaN) should be NaN")
	}
}

func TestDecimal64RoundToEven(t *testing.T) {
	tests := []struct {
		in   decimal64
		want decimal64
	}{
		{0, 0},
		{1, 1},
		{-1, -1},
		{1.4, 1},
		{1.5, 2}, // ties to even (2)
		{1.6, 2},
		{2.5, 2}, // ties to even (2)
		{3.5, 4}, // ties to even (4)
		{-1.5, -2},
		{-2.5, -2},
		{-3.5, -4},
		{0.4, 0},
		{0.5, 0}, // ties to even (0)
		{-0.5, 0},
		{100, 100},
	}
	for _, tt := range tests {
		got := decimal.RoundToEven[decimal64](tt.in)
		if got != tt.want {
			t.Errorf("decimal.RoundToEven[decimal64](%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
	if !decimal.IsNaN[decimal64](decimal.RoundToEven[decimal64](decimal.NaN[decimal64]())) {
		t.Error("decimal.RoundToEven[decimal64](NaN) should be NaN")
	}
}

// --- decimal128 rounding ---

func TestDecimal128Trunc(t *testing.T) {
	tests := []struct {
		in   decimal128
		want decimal128
	}{
		{0, 0},
		{1, 1},
		{-1, -1},
		{1.5, 1},
		{-1.5, -1},
		{2.9, 2},
		{-2.9, -2},
		{0.1, 0},
		{-0.1, 0},
		{100, 100},
	}
	for _, tt := range tests {
		got := decimal.Trunc[decimal128](tt.in)
		if got != tt.want {
			t.Errorf("decimal.Trunc[decimal128](%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestDecimal128Floor(t *testing.T) {
	tests := []struct {
		in   decimal128
		want decimal128
	}{
		{0, 0},
		{1, 1},
		{-1, -1},
		{1.5, 1},
		{-1.5, -2},
		{2.9, 2},
		{-2.9, -3},
		{0.1, 0},
		{-0.1, -1},
		{100, 100},
	}
	for _, tt := range tests {
		got := decimal.Floor[decimal128](tt.in)
		if got != tt.want {
			t.Errorf("decimal.Floor[decimal128](%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestDecimal128Ceil(t *testing.T) {
	tests := []struct {
		in   decimal128
		want decimal128
	}{
		{0, 0},
		{1, 1},
		{-1, -1},
		{1.5, 2},
		{-1.5, -1},
		{0.1, 1},
		{-0.1, 0},
		{100, 100},
	}
	for _, tt := range tests {
		got := decimal.Ceil[decimal128](tt.in)
		if got != tt.want {
			t.Errorf("decimal.Ceil[decimal128](%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestDecimal128Round(t *testing.T) {
	tests := []struct {
		in   decimal128
		want decimal128
	}{
		{0, 0},
		{1.4, 1},
		{1.5, 2},
		{2.5, 3},
		{-1.5, -2},
		{-2.5, -3},
		{0.5, 1},
	}
	for _, tt := range tests {
		got := decimal.Round[decimal128](tt.in)
		if got != tt.want {
			t.Errorf("decimal.Round[decimal128](%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestDecimal128RoundToEven(t *testing.T) {
	tests := []struct {
		in   decimal128
		want decimal128
	}{
		{0, 0},
		{1.5, 2},
		{2.5, 2},
		{3.5, 4},
		{-1.5, -2},
		{-2.5, -2},
		{0.5, 0},
		{-0.5, 0},
	}
	for _, tt := range tests {
		got := decimal.RoundToEven[decimal128](tt.in)
		if got != tt.want {
			t.Errorf("decimal.RoundToEven[decimal128](%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// --- Mod and Remainder ---

func TestDecimal64Mod(t *testing.T) {
	tests := []struct {
		x, y decimal64
		want decimal64
	}{
		{5, 3, 2},
		{7, 4, 3},
		{10, 3, 1},
		{-5, 3, -2},
		{5, -3, 2},
		{-5, -3, -2},
		{1.5, 1, 0.5},
		{0, 1, 0},
	}
	for _, tt := range tests {
		got := decimal.Mod[decimal64](tt.x, tt.y)
		if got != tt.want {
			t.Errorf("decimal.Mod[decimal64](%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
	// Special cases
	if !decimal.IsNaN[decimal64](decimal.Mod[decimal64](decimal.Inf[decimal64](1), 1)) {
		t.Error("decimal.Mod[decimal64](+Inf, 1) should be NaN")
	}
	if !decimal.IsNaN[decimal64](decimal.Mod[decimal64](1, 0)) {
		t.Error("decimal.Mod[decimal64](1, 0) should be NaN")
	}
	if !decimal.IsNaN[decimal64](decimal.Mod[decimal64](decimal.NaN[decimal64](), 1)) {
		t.Error("decimal.Mod[decimal64](NaN, 1) should be NaN")
	}
	if !decimal.IsNaN[decimal64](decimal.Mod[decimal64](1, decimal.NaN[decimal64]())) {
		t.Error("decimal.Mod[decimal64](1, NaN) should be NaN")
	}
	// x mod ±Inf = x
	got := decimal.Mod[decimal64](decimal64(3), decimal.Inf[decimal64](1))
	if got != 3 {
		t.Errorf("decimal.Mod[decimal64](3, +Inf) = %v, want 3", got)
	}
}

func TestDecimal64Remainder(t *testing.T) {
	tests := []struct {
		x, y decimal64
		want decimal64
	}{
		{5, 3, -1},     // 5/3 ≈ 1.667, rounds to 2, 5 - 2*3 = -1
		{7, 4, -1},     // 7/4 = 1.75, rounds to 2, 7 - 2*4 = -1
		{10, 3, 1},     // 10/3 ≈ 3.33, rounds to 3, 10 - 3*3 = 1
		{1.5, 1, -0.5}, // 1.5/1 = 1.5, rounds to 2, 1.5 - 2*1 = -0.5
		{0, 1, 0},
	}
	for _, tt := range tests {
		got := decimal.Remainder[decimal64](tt.x, tt.y)
		if got != tt.want {
			t.Errorf("decimal.Remainder[decimal64](%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
	if !decimal.IsNaN[decimal64](decimal.Remainder[decimal64](decimal.Inf[decimal64](1), 1)) {
		t.Error("decimal.Remainder[decimal64](+Inf, 1) should be NaN")
	}
	if !decimal.IsNaN[decimal64](decimal.Remainder[decimal64](1, 0)) {
		t.Error("decimal.Remainder[decimal64](1, 0) should be NaN")
	}
}

func TestDecimal128Mod(t *testing.T) {
	tests := []struct {
		x, y decimal128
		want decimal128
	}{
		{5, 3, 2},
		{7, 4, 3},
		{10, 3, 1},
		{-5, 3, -2},
		{0, 1, 0},
	}
	for _, tt := range tests {
		got := decimal.Mod[decimal128](tt.x, tt.y)
		if got != tt.want {
			t.Errorf("decimal.Mod[decimal128](%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}

func TestDecimal128Remainder(t *testing.T) {
	tests := []struct {
		x, y decimal128
		want decimal128
	}{
		{5, 3, -1},
		{10, 3, 1},
		{0, 1, 0},
	}
	for _, tt := range tests {
		got := decimal.Remainder[decimal128](tt.x, tt.y)
		if got != tt.want {
			t.Errorf("decimal.Remainder[decimal128](%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}

// --- FMA ---

func TestDecimal64FMA(t *testing.T) {
	tests := []struct {
		x, y, z decimal64
		want    decimal64
	}{
		{2, 3, 4, 10},
		{0, 5, 3, 3},
		{5, 0, 3, 3},
		{1, 1, 0, 1},
		{-2, 3, 10, 4},
	}
	for _, tt := range tests {
		got := decimal.FMA[decimal64](tt.x, tt.y, tt.z)
		if got != tt.want {
			t.Errorf("decimal.FMA[decimal64](%v, %v, %v) = %v, want %v", tt.x, tt.y, tt.z, got, tt.want)
		}
	}
	// Special cases: Inf and NaN propagation
	if !decimal.IsNaN[decimal64](decimal.FMA[decimal64](decimal.Inf[decimal64](1), 0, 1)) {
		t.Error("decimal.FMA[decimal64](+Inf, 0, 1) should be NaN")
	}
	got := decimal.FMA[decimal64](decimal.Inf[decimal64](1), decimal64(1), decimal64(0))
	if !decimal.IsInf[decimal64](got, 1) {
		t.Errorf("decimal.FMA[decimal64](+Inf, 1, 0) = %v, want +Inf", got)
	}
}

func TestDecimal128FMA(t *testing.T) {
	tests := []struct {
		x, y, z decimal128
		want    decimal128
	}{
		{2, 3, 4, 10},
		{0, 5, 3, 3},
		{5, 0, 3, 3},
		{1, 1, 0, 1},
	}
	for _, tt := range tests {
		got := decimal.FMA[decimal128](tt.x, tt.y, tt.z)
		if got != tt.want {
			t.Errorf("decimal.FMA[decimal128](%v, %v, %v) = %v, want %v", tt.x, tt.y, tt.z, got, tt.want)
		}
	}
}

func TestDecimal64SameQuantum(t *testing.T) {
	tests := []struct {
		x, y decimal64
		want bool
	}{
		{1, 2, true},       // both exp=0
		{1.0, 2.0, true},   // both exp=-1
		{1.00, 2.00, true}, // both exp=-2
		{1, 1.0, false},    // exp=0 vs exp=-1
		{1.0, 1.00, false}, // exp=-1 vs exp=-2
		{3.14, 2.71, true}, // both exp=-2
		{100, 200, true},   // both exp=0
		{0.01, 0.02, true}, // both exp=-2
		{1, 0.1, false},    // exp=0 vs exp=-1
	}
	for _, tt := range tests {
		got := decimal.SameQuantum[decimal64](tt.x, tt.y)
		if got != tt.want {
			t.Errorf("decimal.SameQuantum[decimal64](%g, %g) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}

func TestDecimal128SameQuantum(t *testing.T) {
	tests := []struct {
		x, y decimal128
		want bool
	}{
		{1, 2, true},       // both exp=0
		{1.0, 2.0, true},   // both exp=-1
		{1.00, 2.00, true}, // both exp=-2
		{1, 1.0, false},    // exp=0 vs exp=-1
		{1.0, 1.00, false}, // exp=-1 vs exp=-2
		{3.14, 2.71, true}, // both exp=-2
	}
	for _, tt := range tests {
		got := decimal.SameQuantum[decimal128](tt.x, tt.y)
		if got != tt.want {
			t.Errorf("decimal.SameQuantum[decimal128](%g, %g) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}

func TestDecimal64Quantize(t *testing.T) {
	tests := []struct {
		x, y decimal64
		want string // expected %g output
	}{
		{3.14, 1.0, "3.1"},   // quantize 3.14 to 1 decimal place
		{3.14, 1.00, "3.14"}, // quantize to 2 decimal places (no change)
		{3.14, 1, "3"},       // quantize to integer
		{2.71, 1.0, "2.7"},   // round down
		{2.75, 1.0, "2.8"},   // round-half-to-even: .75 rounds to .8
		{1, 0.01, "1.00"},    // add trailing zeros
		{100, 1.0, "100.0"},  // integer with 1 decimal place
	}
	for _, tt := range tests {
		got := decimal.Quantize[decimal64](tt.x, tt.y)
		s := fmt.Sprintf("%g", got)
		if s != tt.want {
			// Dump raw BID bits for diagnosis.
			d64bits := Decimal64bits(got)
			d128 := decimal128(got)
			p := (*[2]uint64)(unsafe.Pointer(&d128))
			t.Errorf("decimal.Quantize[decimal64](%g, %g) = %s, want %s\n"+
				"  d64 bits: %#016x\n"+
				"  d128 lo:  %#016x  hi: %#016x",
				tt.x, tt.y, s, tt.want, d64bits, p[0], p[1])
		}
	}
}

func TestDecimal128Quantize(t *testing.T) {
	tests := []struct {
		x, y decimal128
		want string // expected %g output
	}{
		{3.14, 1.0, "3.1"},
		{3.14, 1.00, "3.14"},
		{3.14, 1, "3"},
		{2.71, 1.0, "2.7"},
		{1, 0.01, "1.00"},
	}
	for _, tt := range tests {
		got := decimal.Quantize[decimal128](tt.x, tt.y)
		s := fmt.Sprintf("%g", got)
		if s != tt.want {
			p := (*[2]uint64)(unsafe.Pointer(&got))
			t.Errorf("decimal.Quantize[decimal128](%g, %g) = %s, want %s\n"+
				"  d128 lo: %#016x  hi: %#016x",
				tt.x, tt.y, s, tt.want, p[0], p[1])
		}
	}
}

func TestQuantum(t *testing.T) {
	tests64 := []struct {
		x    decimal64
		want string // %g of the result, which shows its exponent
	}{
		{1.50, "0.01"},
		{-1.50, "0.01"},
		{123, "1"},
		{1.23e5, "1e+03"}, // the literal keeps its quantum, 1E+3
		{0.00, "0.01"},
		{-0.0, "0.1"},
	}
	for _, tt := range tests64 {
		if got := fmt.Sprintf("%g", decimal.Quantum(tt.x)); got != tt.want {
			t.Errorf("decimal.Quantum[decimal64](%g) = %s, want %s", tt.x, got, tt.want)
		}
	}
	tests128 := []struct {
		x    decimal128
		want string
	}{
		{1.50, "0.01"},
		{-2.000000000000000000000000000000000, "1e-33"},
		{100, "1"},
	}
	for _, tt := range tests128 {
		if got := fmt.Sprintf("%g", decimal.Quantum(tt.x)); got != tt.want {
			t.Errorf("decimal.Quantum[decimal128](%g) = %s, want %s", tt.x, got, tt.want)
		}
	}
	if q := decimal.Quantum(decimal.Inf[decimal64](-1)); !decimal.IsInf(q, 1) {
		t.Errorf("decimal.Quantum(-Inf) = %g, want +Inf", q)
	}
	if q := decimal.Quantum(decimal.NaNWithPayload[decimal128](7)); decimal.Payload(q) != 7 {
		t.Errorf("decimal.Quantum(NaN with payload 7) has payload %g, want 7", decimal.Payload(q))
	}
	// Quantize by a value's quantum is Quantize by the value.
	for _, y := range []decimal64{0.01, -5e3, 1.000} {
		x := decimal64(2.71828)
		if a, b := decimal.Quantize(x, decimal.Quantum(y)), decimal.Quantize(x, y); Decimal64bits(a) != Decimal64bits(b) {
			t.Errorf("Quantize(%g, Quantum(%g)) = %g, Quantize(%g, %g) = %g", x, y, a, x, y, b)
		}
	}
}

func TestPayload(t *testing.T) {
	const max64 = 999999999999999                    // 10^15 - 1, the largest decimal64 payload
	const max128 = 999999999999999999999999999999999 // 10^33 - 1, the largest decimal128 payload

	// Round trips, at both widths, quiet and signaling.
	for _, p := range []decimal64{0, 1, 42, 42.000, 4.2e1, max64} {
		for _, n := range []decimal64{decimal.NaNWithPayload(p), decimal.SignalingNaNWithPayload(p)} {
			if !decimal.IsNaN(n) || decimal.Payload(n) != p || !decimal.SameQuantum(decimal.Payload(n), 1) {
				t.Errorf("decimal64: Payload(NaNWithPayload(%g)) = %g (NaN: %v)", p, decimal.Payload(n), decimal.IsNaN(n))
			}
		}
	}
	for _, p := range []decimal128{0, 7, max64 + 1, max128} {
		for _, n := range []decimal128{decimal.NaNWithPayload(p), decimal.SignalingNaNWithPayload(p)} {
			if !decimal.IsNaN(n) || decimal.Payload(n) != p {
				t.Errorf("decimal128: Payload(NaNWithPayload(%g)) = %g", p, decimal.Payload(n))
			}
		}
	}

	// Refused payloads give +0.
	for _, p := range []decimal64{max64 + 1, 1e15, 4.2, -1, decimal.Inf[decimal64](1), decimal.NaN[decimal64]()} {
		for _, n := range []decimal64{decimal.NaNWithPayload(p), decimal.SignalingNaNWithPayload(p)} {
			if decimal.IsNaN(n) || n != 0 || decimal.Signbit(n) {
				t.Errorf("decimal64: NaNWithPayload(%g) = %g, want 0", p, n)
			}
		}
	}
	if n := decimal.NaNWithPayload[decimal128](max128 + 1); n != 0 {
		t.Errorf("decimal128: NaNWithPayload(10^33) = %g, want 0", n)
	}

	// Payload of a value that is not a NaN is -1.
	for _, x := range []decimal64{0, 1.5, -7, decimal.Inf[decimal64](-1)} {
		if p := decimal.Payload(x); p != -1 {
			t.Errorf("decimal.Payload(%g) = %g, want -1", x, p)
		}
	}

	// Arithmetic propagates the payload and quiets a signaling NaN.
	n := decimal.SignalingNaNWithPayload[decimal64](42)
	for _, r := range []decimal64{n + 1, 1 * n, n / 3, decimal.Quantize(n, 0.01), min(n, 2)} {
		if decimal.Payload(r) != 42 {
			t.Errorf("payload after arithmetic = %g, want 42", decimal.Payload(r))
		}
		if b := Decimal64bits(r); b&(1<<57) != 0 { // the signaling bit
			t.Errorf("arithmetic on a signaling NaN returned a signaling NaN: %#x", b)
		}
	}

	// Widening keeps a payload; narrowing keeps it when it fits.
	if p := decimal.Payload(decimal128(decimal.NaNWithPayload[decimal64](max64))); p != max64 {
		t.Errorf("Payload(decimal128(NaN with payload 10^15-1)) = %g", p)
	}
	if p := decimal.Payload(decimal64(decimal.NaNWithPayload[decimal128](42))); p != 42 {
		t.Errorf("Payload(decimal64(NaN with payload 42)) = %g, want 42", p)
	}
	if p := decimal.Payload(decimal64(decimal.NaNWithPayload[decimal128](max64 + 1))); p != 0 {
		t.Errorf("Payload(decimal64(NaN with payload 10^15)) = %g, want 0", p)
	}
}
