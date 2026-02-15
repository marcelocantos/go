// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !compiler_bootstrap

package cmp_test

import (
	"cmp"
	"math"
	"slices"
	"sort"
	"testing"
)

var decimalTests = []struct {
	x, y    any
	compare int
}{
	{decimal64(1.0), decimal64(1.1), -1},
	{decimal64(1.1), decimal64(1.1), 0},
	{decimal64(1.1), decimal64(1.0), +1},
	{math.Decimal64Inf(1), math.Decimal64Inf(1), 0},
	{math.Decimal64Inf(-1), math.Decimal64Inf(-1), 0},
	{math.Decimal64Inf(-1), decimal64(1.0), -1},
	{decimal64(1.0), math.Decimal64Inf(-1), +1},
	{math.Decimal64NaN(), math.Decimal64NaN(), 0},
	{decimal64(0.0), math.Decimal64NaN(), +1},
	{math.Decimal64NaN(), decimal64(0.0), -1},
	{math.Decimal64NaN(), math.Decimal64Inf(-1), -1},
	{math.Decimal64Inf(-1), math.Decimal64NaN(), +1},
}

func TestDecimalLess(t *testing.T) {
	for _, test := range decimalTests {
		b := cmp.Less(test.x.(decimal64), test.y.(decimal64))
		if b != (test.compare < 0) {
			t.Errorf("Less(%v, %v) == %t, want %t", test.x, test.y, b, test.compare < 0)
		}
	}
}

func TestDecimalCompare(t *testing.T) {
	for _, test := range decimalTests {
		c := cmp.Compare(test.x.(decimal64), test.y.(decimal64))
		if c != test.compare {
			t.Errorf("Compare(%v, %v) == %d, want %d", test.x, test.y, c, test.compare)
		}
	}
}

func TestSortDecimal(t *testing.T) {
	// Test that cmp.Less/Compare are consistent with slices.Sort for decimal64.
	d64 := []decimal64{1.0, 0.0, math.Decimal64Inf(1), math.Decimal64Inf(-1), math.Decimal64NaN()}
	slices.Sort(d64)
	for i := 0; i < len(d64)-1; i++ {
		if cmp.Less(d64[i+1], d64[i]) {
			t.Errorf("decimal64: Less sort mismatch at %d in %v", i, d64)
		}
		if cmp.Compare(d64[i], d64[i+1]) > 0 {
			t.Errorf("decimal64: Compare sort mismatch at %d in %v", i, d64)
		}
	}

	// Test consistency with sort.Decimal128s.
	d128 := []decimal128{1.0, 0.0, decimal128(math.Decimal64Inf(1)), decimal128(math.Decimal64Inf(-1)), decimal128(math.Decimal64NaN())}
	sort.Decimal128s(d128)
	for i := 0; i < len(d128)-1; i++ {
		if cmp.Less(d128[i+1], d128[i]) {
			t.Errorf("decimal128: Less sort mismatch at %d in %v", i, d128)
		}
		if cmp.Compare(d128[i], d128[i+1]) > 0 {
			t.Errorf("decimal128: Compare sort mismatch at %d in %v", i, d128)
		}
	}
}
