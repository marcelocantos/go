// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime_test

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"unsafe"
)

var (
	zero    = math.Copysign(0, +1)
	negZero = math.Copysign(0, -1)
	inf     = math.Inf(+1)
	negInf  = math.Inf(-1)
	nan     = math.NaN()
)

var tests = []struct{ min, max float64 }{
	{1, 2},
	{-2, 1},
	{negZero, zero},
	{zero, inf},
	{negInf, zero},
	{negInf, inf},
	{1, inf},
	{negInf, 1},
}

var all = []float64{1, 2, -1, -2, zero, negZero, inf, negInf, nan}

func eq(x, y float64) bool {
	return x == y && math.Signbit(x) == math.Signbit(y)
}

func TestMinFloat(t *testing.T) {
	for _, tt := range tests {
		if z := min(tt.min, tt.max); !eq(z, tt.min) {
			t.Errorf("min(%v, %v) = %v, want %v", tt.min, tt.max, z, tt.min)
		}
		if z := min(tt.max, tt.min); !eq(z, tt.min) {
			t.Errorf("min(%v, %v) = %v, want %v", tt.max, tt.min, z, tt.min)
		}
	}
	for _, x := range all {
		if z := min(nan, x); !math.IsNaN(z) {
			t.Errorf("min(%v, %v) = %v, want %v", nan, x, z, nan)
		}
		if z := min(x, nan); !math.IsNaN(z) {
			t.Errorf("min(%v, %v) = %v, want %v", nan, x, z, nan)
		}
	}
}

func TestMaxFloat(t *testing.T) {
	for _, tt := range tests {
		if z := max(tt.min, tt.max); !eq(z, tt.max) {
			t.Errorf("max(%v, %v) = %v, want %v", tt.min, tt.max, z, tt.max)
		}
		if z := max(tt.max, tt.min); !eq(z, tt.max) {
			t.Errorf("max(%v, %v) = %v, want %v", tt.max, tt.min, z, tt.max)
		}
	}
	for _, x := range all {
		if z := max(nan, x); !math.IsNaN(z) {
			t.Errorf("max(%v, %v) = %v, want %v", nan, x, z, nan)
		}
		if z := max(x, nan); !math.IsNaN(z) {
			t.Errorf("max(%v, %v) = %v, want %v", nan, x, z, nan)
		}
	}
}

// testMinMax tests that min/max behave correctly on every pair of
// values in vals.
//
// vals should be a sequence of values in strictly ascending order.
func testMinMax[T int | uint8 | string](t *testing.T, vals ...T) {
	for i, x := range vals {
		for _, y := range vals[i+1:] {
			if !(x < y) {
				t.Fatalf("values out of order: !(%v < %v)", x, y)
			}

			if z := min(x, y); z != x {
				t.Errorf("min(%v, %v) = %v, want %v", x, y, z, x)
			}
			if z := min(y, x); z != x {
				t.Errorf("min(%v, %v) = %v, want %v", y, x, z, x)
			}

			if z := max(x, y); z != y {
				t.Errorf("max(%v, %v) = %v, want %v", x, y, z, y)
			}
			if z := max(y, x); z != y {
				t.Errorf("max(%v, %v) = %v, want %v", y, x, z, y)
			}
		}
	}
}

func TestMinMaxInt(t *testing.T)    { testMinMax[int](t, -7, 0, 9) }
func TestMinMaxUint8(t *testing.T)  { testMinMax[uint8](t, 0, 1, 2, 4, 7) }
func TestMinMaxString(t *testing.T) { testMinMax[string](t, "a", "b", "c") }

// TestMinMaxStringTies ensures that min(a, b) returns a when a == b.
func TestMinMaxStringTies(t *testing.T) {
	s := "xxx"
	x := strings.Split(s, "")

	test := func(i, j, k int) {
		if z := min(x[i], x[j], x[k]); unsafe.StringData(z) != unsafe.StringData(x[i]) {
			t.Errorf("min(x[%v], x[%v], x[%v]) = %p, want %p", i, j, k, unsafe.StringData(z), unsafe.StringData(x[i]))
		}
		if z := max(x[i], x[j], x[k]); unsafe.StringData(z) != unsafe.StringData(x[i]) {
			t.Errorf("max(x[%v], x[%v], x[%v]) = %p, want %p", i, j, k, unsafe.StringData(z), unsafe.StringData(x[i]))
		}
	}

	test(0, 1, 2)
	test(0, 2, 1)
	test(1, 0, 2)
	test(1, 2, 0)
	test(2, 0, 1)
	test(2, 1, 0)
}

// decimal64 min/max tests, mirroring TestMinFloat/TestMaxFloat.
var (
	d64zero    = decimal64(0)
	d64negZero = math.Decimal64frombits(1 << 63) // -0
	d64inf     = math.Decimal64Inf(1)
	d64negInf  = math.Decimal64Inf(-1)
	d64nan     = math.Decimal64NaN()
)

var d64tests = []struct{ min, max decimal64 }{
	{1, 2},
	{-2, 1},
	{d64negZero, d64zero},
	{d64zero, d64inf},
	{d64negInf, d64zero},
	{d64negInf, d64inf},
	{1, d64inf},
	{d64negInf, 1},
}

var d64all = []decimal64{1, 2, -1, -2, d64zero, d64negZero, d64inf, d64negInf, d64nan}

func d64eq(x, y decimal64) bool {
	return x == y && math.Decimal64bits(x) == math.Decimal64bits(y)
}

func TestMinDecimal64(t *testing.T) {
	for _, tt := range d64tests {
		if z := min(tt.min, tt.max); !d64eq(z, tt.min) {
			t.Errorf("min(%v, %v) = %v, want %v", tt.min, tt.max, z, tt.min)
		}
		if z := min(tt.max, tt.min); !d64eq(z, tt.min) {
			t.Errorf("min(%v, %v) = %v, want %v", tt.max, tt.min, z, tt.min)
		}
	}
	for _, x := range d64all {
		if z := min(d64nan, x); !math.IsDecimal64NaN(z) {
			t.Errorf("min(%v, %v) = %v, want NaN", d64nan, x, z)
		}
		if z := min(x, d64nan); !math.IsDecimal64NaN(z) {
			t.Errorf("min(%v, %v) = %v, want NaN", x, d64nan, z)
		}
	}
}

func TestMaxDecimal64(t *testing.T) {
	for _, tt := range d64tests {
		if z := max(tt.min, tt.max); !d64eq(z, tt.max) {
			t.Errorf("max(%v, %v) = %v, want %v", tt.min, tt.max, z, tt.max)
		}
		if z := max(tt.max, tt.min); !d64eq(z, tt.max) {
			t.Errorf("max(%v, %v) = %v, want %v", tt.max, tt.min, z, tt.max)
		}
	}
	for _, x := range d64all {
		if z := max(d64nan, x); !math.IsDecimal64NaN(z) {
			t.Errorf("max(%v, %v) = %v, want NaN", d64nan, x, z)
		}
		if z := max(x, d64nan); !math.IsDecimal64NaN(z) {
			t.Errorf("max(%v, %v) = %v, want NaN", x, d64nan, z)
		}
	}
}

func TestMinMaxDecimal128(t *testing.T) {
	d128 := func(s string) decimal128 {
		d, _ := strconv.ParseDecimal128(s)
		return d
	}
	var (
		zero    = decimal128(0)
		negZero = d128("-0")
		posInf  = d128("Inf")
		negInf  = d128("-Inf")
		nan     = d128("NaN")
	)

	tests := []struct{ min, max decimal128 }{
		{1, 2},
		{-2, 1},
		{negZero, zero},
		{zero, posInf},
		{negInf, zero},
		{negInf, posInf},
		{1, posInf},
		{negInf, 1},
	}

	for _, tt := range tests {
		if z := min(tt.min, tt.max); z != tt.min {
			t.Errorf("min(%v, %v) = %v, want %v", tt.min, tt.max, z, tt.min)
		}
		if z := min(tt.max, tt.min); z != tt.min {
			t.Errorf("min(%v, %v) = %v, want %v", tt.max, tt.min, z, tt.min)
		}
	}

	all := []decimal128{1, 2, -1, -2, zero, negZero, posInf, negInf, nan}
	for _, x := range all {
		if z := min(nan, x); z == z {
			t.Errorf("min(NaN, %v) = %v, want NaN", x, z)
		}
		if z := min(x, nan); z == z {
			t.Errorf("min(%v, NaN) = %v, want NaN", x, z)
		}
		if z := max(nan, x); z == z {
			t.Errorf("max(NaN, %v) = %v, want NaN", x, z)
		}
		if z := max(x, nan); z == z {
			t.Errorf("max(%v, NaN) = %v, want NaN", x, z)
		}
	}
}

func BenchmarkMinFloat(b *testing.B) {
	var m float64 = 0
	for i := 0; i < b.N; i++ {
		for _, f := range all {
			m = min(m, f)
		}
	}
}

func BenchmarkMaxFloat(b *testing.B) {
	var m float64 = 0
	for i := 0; i < b.N; i++ {
		for _, f := range all {
			m = max(m, f)
		}
	}
}
