// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !compiler_bootstrap

package sort_test

import (
	"cmp"
	"math"
	"slices"
	. "sort"
	"testing"
)

var decimal128s = [...]decimal128{74.3, 59.0, decimal128(math.Decimal64Inf(1)), 238.2, -784.0, 2.3, decimal128(math.Decimal64NaN()), decimal128(math.Decimal64NaN()), decimal128(math.Decimal64Inf(-1)), 9845.768, -959.7485, 905, 7.8, 7.8}

func TestSortDecimal128Slice(t *testing.T) {
	data := decimal128s
	a := Decimal128Slice(data[0:])
	Sort(a)
	if !IsSorted(a) {
		t.Errorf("sorted %v", decimal128s)
		t.Errorf("   got %v", data)
	}
}

// Compare Sort with slices.Sort sorting a decimal128 slice containing NaNs.
func TestSortDecimal128sCompareSlicesSort(t *testing.T) {
	slice1 := slices.Clone(decimal128s[:])
	slice2 := slices.Clone(decimal128s[:])

	Sort(Decimal128Slice(slice1))
	slices.Sort(slice2)

	// Compare for equality using cmp.Compare, which considers NaNs equal.
	if !slices.EqualFunc(slice1, slice2, func(a, b decimal128) bool { return cmp.Compare(a, b) == 0 }) {
		t.Errorf("mismatch between Sort and slices.Sort: got %v, want %v", slice1, slice2)
	}
}

func TestDecimal128s(t *testing.T) {
	data := decimal128s
	Decimal128s(data[0:])
	if !Decimal128sAreSorted(data[0:]) {
		t.Errorf("sorted %v", decimal128s)
		t.Errorf("   got %v", data)
	}
}

func TestSearchDecimal128s(t *testing.T) {
	data := []decimal128{-959.7485, -784.0, 2.3, 7.8, 7.8, 59.0, 74.3, 238.2, 905, 9845.768}
	tests := []struct {
		x    decimal128
		want int
	}{
		{-1000, 0},
		{-959.7485, 0},
		{0, 2},
		{7.8, 3},
		{100, 7},
		{9845.768, 9},
		{10000, 10},
	}
	for _, tt := range tests {
		if got := SearchDecimal128s(data, tt.x); got != tt.want {
			t.Errorf("SearchDecimal128s(%v) = %d, want %d", tt.x, got, tt.want)
		}
	}
}
