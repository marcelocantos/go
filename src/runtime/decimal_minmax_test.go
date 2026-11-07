// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package runtime_test

import (
	"math"
	"math/decimal"
	"strconv"
	"testing"
)

// decimal64 min/max tests, mirroring TestMinFloat/TestMaxFloat.
var (
	d64zero    = decimal64(0)
	d64negZero = math.Decimal64frombits(1 << 63) // -0
	d64inf     = decimal.Inf[decimal64](1)
	d64negInf  = decimal.Inf[decimal64](-1)
	d64nan     = decimal.NaN[decimal64]()
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
		if z := min(d64nan, x); !decimal.IsNaN[decimal64](z) {
			t.Errorf("min(%v, %v) = %v, want NaN", d64nan, x, z)
		}
		if z := min(x, d64nan); !decimal.IsNaN[decimal64](z) {
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
		if z := max(d64nan, x); !decimal.IsNaN[decimal64](z) {
			t.Errorf("max(%v, %v) = %v, want NaN", d64nan, x, z)
		}
		if z := max(x, d64nan); !decimal.IsNaN[decimal64](z) {
			t.Errorf("max(%v, %v) = %v, want NaN", x, d64nan, z)
		}
	}
}

func TestMinMaxDecimal128(t *testing.T) {
	d128 := func(s string) decimal128 {
		d, _ := strconv.ParseDecimal(s, 128)
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
