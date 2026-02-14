// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package math_test

import (
	. "math"
	"testing"
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
