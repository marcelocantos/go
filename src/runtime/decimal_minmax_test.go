// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package runtime_test

import (
	"math/decimal"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// decimal64 min/max tests, mirroring TestMinFloat/TestMaxFloat.
var (
	d64zero    = decimal64(0)
	d64negZero = decimal.BID64frombits(1 << 63) // -0
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
	return x == y && decimal.BID64bits(x) == decimal.BID64bits(y)
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

//go:noinline
func d64id(x decimal64) decimal64 { return x }

// TestMinMaxDecimalCohort checks the rule for members of one cohort: min
// returns the member IEEE 754-2019's totalOrder puts first and max the one
// it puts last, the smaller exponent first for positive values and the
// larger for negative ones, whatever the order of the arguments: at run
// time, in constants, when the two are mixed, and through slices.Min and
// slices.Max.
func TestMinMaxDecimalCohort(t *testing.T) {
	a, b, c := d64id(1.0), d64id(1.00), d64id(1.000)
	const ka, kb decimal64 = 1.0, 1.00
	text := func(x decimal64) string { return strconv.FormatDecimal(decimal128(x), 'e', -1, 64) }
	for _, tt := range []struct {
		name      string
		got, want decimal64
	}{
		{"min(a, b)", min(a, b), b},
		{"min(b, a)", min(b, a), b},
		{"max(a, b)", max(a, b), a},
		{"max(b, a)", max(b, a), a},
		{"min(c, b, a)", min(c, b, a), c},
		{"min(a, c, b)", min(a, c, b), c},
		{"max(b, c, a)", max(b, c, a), a},
		{"min(2, a, b)", min(2, a, b), b},
		{"max(0.5, b, a)", max(0.5, b, a), a},
		{"min(ka, kb)", min(ka, kb), kb},
		{"min(kb, ka)", min(kb, ka), kb},
		{"max(kb, ka)", max(kb, ka), ka},
		{"min(1.0, 1.00) as decimal64", min(1.0, 1.00), kb},
		{"min(1.00, 1.0) as decimal64", min(1.00, 1.0), kb},
		{"max(1.00, 1.0) as decimal64", max(1.00, 1.0), ka},
		{"min(a, kb)", min(a, kb), kb},
		{"min(kb, a)", min(kb, a), kb},
		{"max(b, ka)", max(b, ka), ka},
		{"min(-a, -b)", min(-a, -b), -a},
		{"min(-b, -a)", min(-b, -a), -a},
		{"max(-a, -b)", max(-a, -b), -b},
		{"min(-1.0, -1.00) as decimal64", min(-1.0, -1.00), -ka},
		{"slices.Min(a, b)", slices.Min([]decimal64{a, b}), b},
		{"slices.Min(b, a)", slices.Min([]decimal64{b, a}), b},
		{"slices.Max(b, a)", slices.Max([]decimal64{b, a}), a},
	} {
		if text(tt.got) != text(tt.want) {
			t.Errorf("%s = %s, want %s", tt.name, text(tt.got), text(tt.want))
		}
	}
}

// TestMinMaxDecimalCommutative checks, bit for bit over random members of
// random cohorts at both widths (zeros of both signs among them), that min
// and max are commutative and associative, that slices.Min and slices.Max
// agree with them, and that the result is the member totalOrder puts first
// (last).
func TestMinMaxDecimalCommutative(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	mk := func(coeff string, neg bool, e int) string {
		s := coeff + "e" + strconv.Itoa(e)
		if neg {
			s = "-" + s
		}
		return s
	}
	for _, w := range []struct{ bits, prec int }{{64, 16}, {128, 34}} {
		for range 2000 {
			// A cohort: the coefficient c × 10^k at exponent q - k.
			nd := 1 + r.IntN(w.prec-3)
			c := strconv.Itoa(1 + r.IntN(9))
			for range nd - 1 {
				c += strconv.Itoa(r.IntN(10))
			}
			if r.IntN(8) == 0 {
				c = "0"
			}
			neg := r.IntN(2) == 0
			q := r.IntN(200) - 100
			var xs []string
			for range 3 {
				k := r.IntN(w.prec - nd + 1)
				xneg := neg
				if c == "0" {
					xneg = r.IntN(2) == 0
				}
				xs = append(xs, mk(c+strings.Repeat("0", k), xneg, q-k))
			}
			if w.bits == 64 {
				checkMinMax64(t, xs)
			} else {
				checkMinMax128(t, xs)
			}
		}
	}
}

func checkMinMax64(t *testing.T, s []string) {
	t.Helper()
	v := make([]decimal64, len(s))
	for i, x := range s {
		d, err := strconv.ParseDecimal(x, 64)
		if err != nil {
			t.Fatal(err)
		}
		v[i] = decimal64(d)
	}
	b := decimal.BID64bits
	x, y, z := d64id(v[0]), d64id(v[1]), d64id(v[2])
	if b(min(x, y)) != b(min(y, x)) || b(max(x, y)) != b(max(y, x)) {
		t.Errorf("min or max of %s, %s not commutative: %v %v %v %v", s[0], s[1], min(x, y), min(y, x), max(x, y), max(y, x))
	}
	if b(min(min(x, y), z)) != b(min(x, min(y, z))) || b(max(max(x, y), z)) != b(max(x, max(y, z))) || b(min(x, y, z)) != b(min(z, y, x)) {
		t.Errorf("min or max of %v not associative", s)
	}
	if b(slices.Min(v)) != b(min(x, y, z)) || b(slices.Max(v)) != b(max(x, y, z)) {
		t.Errorf("slices.Min or Max of %v disagrees with min or max", s)
	}
	// The member totalOrder puts first and last.
	first, last := v[0], v[0]
	for _, m := range v[1:] {
		if cohortBefore(fmt64(m), fmt64(first)) {
			first = m
		}
		if cohortBefore(fmt64(last), fmt64(m)) {
			last = m
		}
	}
	if b(min(x, y, z)) != b(first) || b(max(x, y, z)) != b(last) {
		t.Errorf("min, max of %v = %v, %v; totalOrder puts %v first and %v last", s, min(x, y, z), max(x, y, z), first, last)
	}
}

func fmt64(d decimal64) string { return strconv.FormatDecimal(decimal128(d), 'b', -1, 64) }

// cohortBefore reports whether totalOrder puts a before b, for members of
// one cohort (or zeros) in strconv's 'b' form: a negative value first, and
// of one sign the smaller exponent first if positive, the larger if
// negative.
func cohortBefore(a, b string) bool {
	na, nb := strings.HasPrefix(a, "-"), strings.HasPrefix(b, "-")
	if na != nb {
		return na
	}
	ea, _ := strconv.Atoi(a[strings.IndexByte(a, 'e')+1:])
	eb, _ := strconv.Atoi(b[strings.IndexByte(b, 'e')+1:])
	if na {
		return ea > eb
	}
	return ea < eb
}

func checkMinMax128(t *testing.T, s []string) {
	t.Helper()
	v := make([]decimal128, len(s))
	for i, x := range s {
		d, err := strconv.ParseDecimal(x, 128)
		if err != nil {
			t.Fatal(err)
		}
		v[i] = d
	}
	b := func(d decimal128) [2]uint64 { h, l := decimal.BID128bits(d); return [2]uint64{h, l} }
	x, y, z := v[0], v[1], v[2]
	if b(min(x, y)) != b(min(y, x)) || b(max(x, y)) != b(max(y, x)) {
		t.Errorf("min or max of %s, %s not commutative", s[0], s[1])
	}
	if b(min(min(x, y), z)) != b(min(x, min(y, z))) || b(max(max(x, y), z)) != b(max(x, max(y, z))) || b(min(x, y, z)) != b(min(z, y, x)) {
		t.Errorf("min or max of %v not associative", s)
	}
	if b(slices.Min(v)) != b(min(x, y, z)) || b(slices.Max(v)) != b(max(x, y, z)) {
		t.Errorf("slices.Min or Max of %v disagrees with min or max", s)
	}
	f128 := func(d decimal128) string { return strconv.FormatDecimal(d, 'b', -1, 128) }
	first, last := v[0], v[0]
	for _, m := range v[1:] {
		if cohortBefore(f128(m), f128(first)) {
			first = m
		}
		if cohortBefore(f128(last), f128(m)) {
			last = m
		}
	}
	if b(min(x, y, z)) != b(first) || b(max(x, y, z)) != b(last) {
		t.Errorf("min, max of %v = %v, %v; totalOrder puts %v first and %v last", s, min(x, y, z), max(x, y, z), first, last)
	}
}
