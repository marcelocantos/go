// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package decimal_test

import (
	"math/decimal"
	"math/rand/v2"
	"runtime"
	"slices"
	"testing"
)

// totalOrdered128 is decimal128 values in the total order of IEEE 754-2019
// clause 5.10, each with a name for messages.
func totalOrdered128() ([]decimal128, []string) {
	neg := func(x decimal128) decimal128 { return decimal.Copysign(x, -1) }
	q := func(x, e decimal128) decimal128 { return decimal.Quantize(x, e) }
	snan := func(p decimal128) decimal128 { return decimal.SignalingNaNWithPayload(p) }
	qnan := func(p decimal128) decimal128 { return decimal.NaNWithPayload(p) }
	inf := decimal.Inf[decimal128](1)
	list := []struct {
		name string
		x    decimal128
	}{
		{"-NaN(2)", neg(qnan(2))},
		{"-NaN(1)", neg(qnan(1))},
		{"-sNaN(2)", neg(snan(2))},
		{"-sNaN(1)", neg(snan(1))},
		{"-Inf", neg(inf)},
		{"-Max", -decimal.MaxDecimal128},
		{"-2", -2},
		{"-1.0", q(-1, 0.1)},
		{"-1.00", q(-1, 0.01)},
		{"-SmallestNonzero", -decimal.SmallestNonzeroDecimal128},
		{"-0E+1", neg(q(0, 1e1))},
		{"-0", neg(0)},
		{"-0E-1", neg(q(0, 0.1))},
		{"0E-1", q(0, 0.1)},
		{"0", 0},
		{"0E+1", q(0, 1e1)},
		{"SmallestNonzero", decimal.SmallestNonzeroDecimal128},
		{"1.00", q(1, 0.01)},
		{"1.0", q(1, 0.1)},
		{"1", 1},
		{"2", 2},
		{"Max", decimal.MaxDecimal128},
		{"Inf", inf},
		{"sNaN(1)", snan(1)},
		{"sNaN(2)", snan(2)},
		{"NaN(1)", qnan(1)},
		{"NaN(2)", qnan(2)},
	}
	xs := make([]decimal128, len(list))
	names := make([]string, len(list))
	for i, e := range list {
		xs[i], names[i] = e.x, e.name
	}
	return xs, names
}

// TestTotalOrder128 checks TotalOrder, TotalCompare and TotalOrderMag on
// decimal128 against the order of clause 5.10: every pair of a list in that
// order, and sorting the shuffled list with TotalCompare.
func TestTotalOrder128(t *testing.T) {
	if runtime.GOARCH == "ppc64le" {
		t.Skip("decimal128 is not stored as UBD on ppc64le, where NaNs are ordered by sign alone")
	}
	xs, names := totalOrdered128()
	for i, x := range xs {
		for j, y := range xs {
			want := 0
			switch {
			case i < j:
				want = -1
			case i > j:
				want = 1
			}
			if got := decimal.TotalCompare(x, y); got != want {
				t.Errorf("TotalCompare(%s, %s) = %d, want %d", names[i], names[j], got, want)
			}
			if got := decimal.TotalOrder(x, y); got != (i <= j) {
				t.Errorf("TotalOrder(%s, %s) = %v, want %v", names[i], names[j], got, i <= j)
			}
			if got, want := decimal.TotalCompareMag(x, y), decimal.TotalCompare(decimal.Abs(x), decimal.Abs(y)); got != want {
				t.Errorf("TotalCompareMag(%s, %s) = %d, want %d", names[i], names[j], got, want)
			}
			if got, want := decimal.TotalOrderMag(x, y), decimal.TotalOrder(decimal.Abs(x), decimal.Abs(y)); got != want {
				t.Errorf("TotalOrderMag(%s, %s) = %v, want %v", names[i], names[j], got, want)
			}
		}
	}
	r := rand.New(rand.NewPCG(38, 5))
	shuffled := slices.Clone(xs)
	r.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	slices.SortFunc(shuffled, decimal.TotalCompare)
	for i := range xs {
		if decimal.TotalCompare(shuffled[i], xs[i]) != 0 {
			t.Errorf("sorted position %d is %v, want %s", i, shuffled[i], names[i])
		}
	}
}

// TestTotalOrderMag128 checks TotalOrderMag on pairs whose order changes
// when the signs are dropped.
func TestTotalOrderMag128(t *testing.T) {
	if runtime.GOARCH == "ppc64le" {
		t.Skip("decimal128 is not stored as UBD on ppc64le")
	}
	nan := decimal.NaNWithPayload(decimal128(1))
	snan := decimal.SignalingNaNWithPayload(decimal128(1))
	for _, tt := range []struct {
		x, y decimal128
		want bool
	}{
		{-2, 1, false},
		{1, -2, true},
		{decimal.Copysign(decimal128(0), -1), 0, true}, // the same magnitude
		{0, decimal.Copysign(decimal128(0), -1), true},
		{decimal.Copysign(nan, -1), decimal.Inf[decimal128](1), false},
		{decimal.Copysign(snan, -1), nan, true},
		{nan, decimal.Copysign(snan, -1), false},
	} {
		if got := decimal.TotalOrderMag(tt.x, tt.y); got != tt.want {
			t.Errorf("TotalOrderMag(%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}

// TestTotalCompareMag128 sorts decimal128 values by magnitude with
// TotalCompareMag: the signs drop out, members of one cohort keep the
// smaller exponent first, and NaNs follow the infinity, signaling before
// quiet.
func TestTotalCompareMag128(t *testing.T) {
	if runtime.GOARCH == "ppc64le" {
		t.Skip("decimal128 is not stored as UBD on ppc64le")
	}
	q := func(x, e decimal128) decimal128 { return decimal.Quantize(x, e) }
	snan2 := decimal.SignalingNaNWithPayload(decimal128(2))
	negNaN1 := decimal.Copysign(decimal.NaNWithPayload(decimal128(1)), -1)
	inf := decimal.Inf[decimal128](-1)
	in := []decimal128{-3, negNaN1, 2, inf, q(-1, 0.1), snan2, q(1, 0.01)}
	want := []decimal128{q(1, 0.01), q(-1, 0.1), 2, -3, inf, snan2, negNaN1}
	got := slices.Clone(in)
	slices.SortFunc(got, decimal.TotalCompareMag)
	for i := range want {
		if decimal.TotalCompare(got[i], want[i]) != 0 {
			t.Errorf("sorted by magnitude, position %d is %v, want %v (all: %v)", i, got[i], want[i], got)
		}
	}
	if c := decimal.TotalCompareMag(decimal128(-1), decimal128(1)); c != 0 {
		t.Errorf("TotalCompareMag(-1, 1) = %d, want 0", c)
	}
}
