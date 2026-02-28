// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package math_test

import (
	"fmt"
	. "math"
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
	if !IsDecimal64NaN(Decimal64NaN()) {
		t.Error("IsDecimal64NaN(Decimal64NaN()) = false")
	}
	if IsDecimal64NaN(decimal64(0)) {
		t.Error("IsDecimal64NaN(0) = true")
	}
	if IsDecimal64NaN(decimal64(1)) {
		t.Error("IsDecimal64NaN(1) = true")
	}
	if IsDecimal64NaN(Decimal64Inf(1)) {
		t.Error("IsDecimal64NaN(+Inf) = true")
	}
	if IsDecimal64NaN(Decimal64Inf(-1)) {
		t.Error("IsDecimal64NaN(-Inf) = true")
	}
}

func TestIsDecimal64Inf(t *testing.T) {
	pinf := Decimal64Inf(1)
	ninf := Decimal64Inf(-1)
	nan := Decimal64NaN()
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
		got := IsDecimal64Inf(tt.d, tt.sign)
		if got != tt.want {
			t.Errorf("IsDecimal64Inf(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsDecimal128NaN(t *testing.T) {
	if !IsDecimal128NaN(Decimal128NaN()) {
		t.Error("IsDecimal128NaN(Decimal128NaN()) = false")
	}
	if IsDecimal128NaN(decimal128(0)) {
		t.Error("IsDecimal128NaN(0) = true")
	}
	if IsDecimal128NaN(decimal128(1)) {
		t.Error("IsDecimal128NaN(1) = true")
	}
	if IsDecimal128NaN(Decimal128Inf(1)) {
		t.Error("IsDecimal128NaN(+Inf) = true")
	}
	if IsDecimal128NaN(Decimal128Inf(-1)) {
		t.Error("IsDecimal128NaN(-Inf) = true")
	}
}

func TestIsDecimal128Inf(t *testing.T) {
	pinf := Decimal128Inf(1)
	ninf := Decimal128Inf(-1)
	nan := Decimal128NaN()
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
		got := IsDecimal128Inf(tt.d, tt.sign)
		if got != tt.want {
			t.Errorf("IsDecimal128Inf(%s) = %v, want %v", tt.name, got, tt.want)
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
		{Decimal64Inf(1), Decimal64Inf(1)},
		{Decimal64Inf(-1), Decimal64Inf(1)},
	}
	for _, tt := range tests {
		got := Decimal64Abs(tt.in)
		if got != tt.want {
			t.Errorf("Decimal64Abs(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
	if !IsDecimal64NaN(Decimal64Abs(Decimal64NaN())) {
		t.Error("Decimal64Abs(NaN) should be NaN")
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
		got := Decimal128Abs(tt.in)
		if got != tt.want {
			t.Errorf("Decimal128Abs(%v) = %v, want %v", tt.in, got, tt.want)
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
		got := Decimal64Copysign(tt.f, tt.sign)
		if got != tt.want {
			t.Errorf("Decimal64Copysign(%v, %v) = %v, want %v", tt.f, tt.sign, got, tt.want)
		}
	}
	// Copysign with negative zero created via Copysign itself
	negzero := Decimal64Copysign(0, -1)
	if !Decimal64Signbit(negzero) {
		t.Error("Decimal64Copysign(0, -1) should produce negative zero")
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
		got := Decimal128Copysign(tt.f, tt.sign)
		if got != tt.want {
			t.Errorf("Decimal128Copysign(%v, %v) = %v, want %v", tt.f, tt.sign, got, tt.want)
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
		{Decimal64Inf(1), false},
		{Decimal64Inf(-1), true},
	}
	for _, tt := range tests {
		got := Decimal64Signbit(tt.in)
		if got != tt.want {
			t.Errorf("Decimal64Signbit(%v) = %v, want %v", tt.in, got, tt.want)
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
		got := Decimal128Signbit(tt.in)
		if got != tt.want {
			t.Errorf("Decimal128Signbit(%v) = %v, want %v", tt.in, got, tt.want)
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
		got := Decimal64Dim(tt.x, tt.y)
		if got != tt.want {
			t.Errorf("Decimal64Dim(%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
	if !IsDecimal64NaN(Decimal64Dim(Decimal64NaN(), 1)) {
		t.Error("Decimal64Dim(NaN, 1) should be NaN")
	}
	if !IsDecimal64NaN(Decimal64Dim(1, Decimal64NaN())) {
		t.Error("Decimal64Dim(1, NaN) should be NaN")
	}
	if !IsDecimal64NaN(Decimal64Dim(Decimal64Inf(1), Decimal64Inf(1))) {
		t.Error("Decimal64Dim(+Inf, +Inf) should be NaN")
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
		got := Decimal128Dim(tt.x, tt.y)
		if got != tt.want {
			t.Errorf("Decimal128Dim(%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
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
		{Decimal64Inf(1), Decimal64Inf(1)},
		{Decimal64Inf(-1), Decimal64Inf(-1)},
	}
	for _, tt := range tests {
		got := Decimal64Trunc(tt.in)
		if got != tt.want {
			t.Errorf("Decimal64Trunc(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
	if !IsDecimal64NaN(Decimal64Trunc(Decimal64NaN())) {
		t.Error("Decimal64Trunc(NaN) should be NaN")
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
		{Decimal64Inf(1), Decimal64Inf(1)},
		{Decimal64Inf(-1), Decimal64Inf(-1)},
	}
	for _, tt := range tests {
		got := Decimal64Floor(tt.in)
		if got != tt.want {
			t.Errorf("Decimal64Floor(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
	if !IsDecimal64NaN(Decimal64Floor(Decimal64NaN())) {
		t.Error("Decimal64Floor(NaN) should be NaN")
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
		{Decimal64Inf(1), Decimal64Inf(1)},
		{Decimal64Inf(-1), Decimal64Inf(-1)},
	}
	for _, tt := range tests {
		got := Decimal64Ceil(tt.in)
		if got != tt.want {
			t.Errorf("Decimal64Ceil(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
	if !IsDecimal64NaN(Decimal64Ceil(Decimal64NaN())) {
		t.Error("Decimal64Ceil(NaN) should be NaN")
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
		got := Decimal64Round(tt.in)
		if got != tt.want {
			t.Errorf("Decimal64Round(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
	if !IsDecimal64NaN(Decimal64Round(Decimal64NaN())) {
		t.Error("Decimal64Round(NaN) should be NaN")
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
		got := Decimal64RoundToEven(tt.in)
		if got != tt.want {
			t.Errorf("Decimal64RoundToEven(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
	if !IsDecimal64NaN(Decimal64RoundToEven(Decimal64NaN())) {
		t.Error("Decimal64RoundToEven(NaN) should be NaN")
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
		got := Decimal128Trunc(tt.in)
		if got != tt.want {
			t.Errorf("Decimal128Trunc(%v) = %v, want %v", tt.in, got, tt.want)
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
		got := Decimal128Floor(tt.in)
		if got != tt.want {
			t.Errorf("Decimal128Floor(%v) = %v, want %v", tt.in, got, tt.want)
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
		got := Decimal128Ceil(tt.in)
		if got != tt.want {
			t.Errorf("Decimal128Ceil(%v) = %v, want %v", tt.in, got, tt.want)
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
		got := Decimal128Round(tt.in)
		if got != tt.want {
			t.Errorf("Decimal128Round(%v) = %v, want %v", tt.in, got, tt.want)
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
		got := Decimal128RoundToEven(tt.in)
		if got != tt.want {
			t.Errorf("Decimal128RoundToEven(%v) = %v, want %v", tt.in, got, tt.want)
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
		got := Decimal64Mod(tt.x, tt.y)
		if got != tt.want {
			t.Errorf("Decimal64Mod(%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
	// Special cases
	if !IsDecimal64NaN(Decimal64Mod(Decimal64Inf(1), 1)) {
		t.Error("Decimal64Mod(+Inf, 1) should be NaN")
	}
	if !IsDecimal64NaN(Decimal64Mod(1, 0)) {
		t.Error("Decimal64Mod(1, 0) should be NaN")
	}
	if !IsDecimal64NaN(Decimal64Mod(Decimal64NaN(), 1)) {
		t.Error("Decimal64Mod(NaN, 1) should be NaN")
	}
	if !IsDecimal64NaN(Decimal64Mod(1, Decimal64NaN())) {
		t.Error("Decimal64Mod(1, NaN) should be NaN")
	}
	// x mod ±Inf = x
	got := Decimal64Mod(decimal64(3), Decimal64Inf(1))
	if got != 3 {
		t.Errorf("Decimal64Mod(3, +Inf) = %v, want 3", got)
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
		got := Decimal64Remainder(tt.x, tt.y)
		if got != tt.want {
			t.Errorf("Decimal64Remainder(%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
	if !IsDecimal64NaN(Decimal64Remainder(Decimal64Inf(1), 1)) {
		t.Error("Decimal64Remainder(+Inf, 1) should be NaN")
	}
	if !IsDecimal64NaN(Decimal64Remainder(1, 0)) {
		t.Error("Decimal64Remainder(1, 0) should be NaN")
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
		got := Decimal128Mod(tt.x, tt.y)
		if got != tt.want {
			t.Errorf("Decimal128Mod(%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
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
		got := Decimal128Remainder(tt.x, tt.y)
		if got != tt.want {
			t.Errorf("Decimal128Remainder(%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
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
		got := Decimal64FMA(tt.x, tt.y, tt.z)
		if got != tt.want {
			t.Errorf("Decimal64FMA(%v, %v, %v) = %v, want %v", tt.x, tt.y, tt.z, got, tt.want)
		}
	}
	// Special cases: Inf and NaN propagation
	if !IsDecimal64NaN(Decimal64FMA(Decimal64Inf(1), 0, 1)) {
		t.Error("Decimal64FMA(+Inf, 0, 1) should be NaN")
	}
	got := Decimal64FMA(Decimal64Inf(1), decimal64(1), decimal64(0))
	if !IsDecimal64Inf(got, 1) {
		t.Errorf("Decimal64FMA(+Inf, 1, 0) = %v, want +Inf", got)
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
		got := Decimal128FMA(tt.x, tt.y, tt.z)
		if got != tt.want {
			t.Errorf("Decimal128FMA(%v, %v, %v) = %v, want %v", tt.x, tt.y, tt.z, got, tt.want)
		}
	}
}

func TestDecimal64SameQuantum(t *testing.T) {
	tests := []struct {
		x, y decimal64
		want bool
	}{
		{1, 2, true},           // both exp=0
		{1.0, 2.0, true},      // both exp=-1
		{1.00, 2.00, true},    // both exp=-2
		{1, 1.0, false},       // exp=0 vs exp=-1
		{1.0, 1.00, false},    // exp=-1 vs exp=-2
		{3.14, 2.71, true},    // both exp=-2
		{100, 200, true},      // both exp=0
		{0.01, 0.02, true},    // both exp=-2
		{1, 0.1, false},       // exp=0 vs exp=-1
	}
	for _, tt := range tests {
		got := Decimal64SameQuantum(tt.x, tt.y)
		if got != tt.want {
			t.Errorf("Decimal64SameQuantum(%#g, %#g) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}

func TestDecimal128SameQuantum(t *testing.T) {
	tests := []struct {
		x, y decimal128
		want bool
	}{
		{1, 2, true},           // both exp=0
		{1.0, 2.0, true},      // both exp=-1
		{1.00, 2.00, true},    // both exp=-2
		{1, 1.0, false},       // exp=0 vs exp=-1
		{1.0, 1.00, false},    // exp=-1 vs exp=-2
		{3.14, 2.71, true},    // both exp=-2
	}
	for _, tt := range tests {
		got := Decimal128SameQuantum(tt.x, tt.y)
		if got != tt.want {
			t.Errorf("Decimal128SameQuantum(%#g, %#g) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}

func TestDecimal64Quantize(t *testing.T) {
	tests := []struct {
		x, y decimal64
		want string // expected %#g output
	}{
		{3.14, 1.0, "3.1"},    // quantize 3.14 to 1 decimal place
		{3.14, 1.00, "3.14"},  // quantize to 2 decimal places (no change)
		{3.14, 1, "3"},        // quantize to integer
		{2.71, 1.0, "2.7"},   // round down
		{2.75, 1.0, "2.8"},   // round-half-to-even: .75 rounds to .8
		{1, 0.01, "1.00"},    // add trailing zeros
		{100, 1.0, "100.0"},  // integer with 1 decimal place
	}
	for _, tt := range tests {
		got := Decimal64Quantize(tt.x, tt.y)
		s := fmt.Sprintf("%#g", got)
		if s != tt.want {
			// Dump raw BID bits for diagnosis.
			d64bits := Decimal64bits(got)
			d128 := decimal128(got)
			p := (*[2]uint64)(unsafe.Pointer(&d128))
			t.Errorf("Decimal64Quantize(%#g, %#g) = %s, want %s\n"+
				"  d64 bits: %#016x\n"+
				"  d128 lo:  %#016x  hi: %#016x",
				tt.x, tt.y, s, tt.want, d64bits, p[0], p[1])
		}
	}
}

func TestDecimal128Quantize(t *testing.T) {
	tests := []struct {
		x, y decimal128
		want string // expected %#g output
	}{
		{3.14, 1.0, "3.1"},
		{3.14, 1.00, "3.14"},
		{3.14, 1, "3"},
		{2.71, 1.0, "2.7"},
		{1, 0.01, "1.00"},
	}
	for _, tt := range tests {
		got := Decimal128Quantize(tt.x, tt.y)
		s := fmt.Sprintf("%#g", got)
		if s != tt.want {
			p := (*[2]uint64)(unsafe.Pointer(&got))
			t.Errorf("Decimal128Quantize(%#g, %#g) = %s, want %s\n"+
				"  d128 lo: %#016x  hi: %#016x",
				tt.x, tt.y, s, tt.want, p[0], p[1])
		}
	}
}
