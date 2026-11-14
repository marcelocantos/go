// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package reflect_test

import (
	"reflect"
	"testing"
)

// Bits and OverflowDecimal follow Bits and OverflowFloat: the size in bits,
// and overflow only for a finite value beyond the largest finite one.
func TestDecimalBitsAndOverflow(t *testing.T) {
	type named decimal64
	for _, tt := range []struct {
		v    any
		bits int
	}{
		{decimal64(0), 64}, {decimal128(0), 128}, {named(0), 64},
	} {
		if got := reflect.TypeOf(tt.v).Bits(); got != tt.bits {
			t.Errorf("TypeOf(%T).Bits() = %d, want %d", tt.v, got, tt.bits)
		}
	}
	v64 := reflect.ValueOf(decimal64(0))
	v128 := reflect.ValueOf(decimal128(0))
	ten := decimal128(10)
	inf := decimal128(1e6144) * ten
	for _, tt := range []struct {
		x    decimal128
		want bool
	}{
		{1.5, false},
		{9.999999999999999e384, false},
		{-9.999999999999999e384, false},
		{1e385, true},
		{-1e385, true},
		{9.999999999999999999999999999999999e6144, true},
		{inf, false},
		{-inf, false},
		{inf - inf, false},
	} {
		if got := v64.OverflowDecimal(tt.x); got != tt.want {
			t.Errorf("decimal64 OverflowDecimal(%v) = %v, want %v", tt.x, got, tt.want)
		}
		if v128.OverflowDecimal(tt.x) {
			t.Errorf("decimal128 OverflowDecimal(%v) = true", tt.x)
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("OverflowDecimal on a float64 did not panic")
		}
	}()
	reflect.ValueOf(1.5).OverflowDecimal(1)
}
