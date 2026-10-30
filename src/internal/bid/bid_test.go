// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	mbits "math/bits"
	"testing"
	"unsafe"
)

// uint128Pow10 returns 10^n as a Uint128 and reports whether it fit.
func uint128Pow10(n int) (bid.Uint128, bool) {
	p := bid.Uint128{Lo: 1}
	for i := 0; i < n; i++ {
		var carry uint64
		carry, p.Lo = mbits.Mul64(p.Lo, 10)
		overflow, hi := mbits.Mul64(p.Hi, 10)
		if overflow != 0 {
			return bid.Uint128{}, false
		}
		var c uint64
		p.Hi, c = mbits.Add64(hi, carry, 0)
		if c != 0 {
			return bid.Uint128{}, false
		}
	}
	return p, true
}

func TestMaxCoeff(t *testing.T) {
	p16, ok := uint128Pow10(bid.Precision64)
	if !ok || p16.Hi != 0 {
		t.Fatalf("10^%d does not fit a uint64", bid.Precision64)
	}
	if bid.MaxCoeff64 != p16.Lo-1 {
		t.Errorf("MaxCoeff64 = %d, want 10^%d-1 = %d", bid.MaxCoeff64, bid.Precision64, p16.Lo-1)
	}
	if bid.MaxCoeff64 >= 1<<(bid.Form2CoeffBits64+3) {
		t.Errorf("MaxCoeff64 does not fit the second encoding form")
	}

	p34, ok := uint128Pow10(bid.Precision128)
	if !ok {
		t.Fatalf("10^%d does not fit a Uint128", bid.Precision128)
	}
	lo, borrow := mbits.Sub64(p34.Lo, 1, 0)
	hi, _ := mbits.Sub64(p34.Hi, 0, borrow)
	if bid.MaxCoeff128Hi != hi || bid.MaxCoeff128Lo != lo {
		t.Errorf("MaxCoeff128 = %#x:%#x, want 10^%d-1 = %#x:%#x", bid.MaxCoeff128Hi, bid.MaxCoeff128Lo, bid.Precision128, hi, lo)
	}
	if bid.MaxCoeff128Hi >= 1<<(bid.CoeffBits128-64) {
		t.Errorf("MaxCoeff128 does not fit the first encoding form")
	}
}

func TestExponentRange(t *testing.T) {
	cases := []struct {
		name                          string
		precision, emax, emin         int
		maxExp, minExp, bias, expBits int
	}{
		{"decimal64", bid.Precision64, bid.Emax64, bid.Emin64, bid.MaxExp64, bid.MinExp64, bid.Bias64, bid.ExpBits64},
		{"decimal128", bid.Precision128, bid.Emax128, bid.Emin128, bid.MaxExp128, bid.MinExp128, bid.Bias128, bid.ExpBits128},
	}
	for _, c := range cases {
		if c.emin != 1-c.emax {
			t.Errorf("%s: Emin = %d, want 1-Emax = %d", c.name, c.emin, 1-c.emax)
		}
		if c.maxExp != c.emax-(c.precision-1) {
			t.Errorf("%s: MaxExp = %d, want %d", c.name, c.maxExp, c.emax-(c.precision-1))
		}
		if c.minExp != c.emin-(c.precision-1) {
			t.Errorf("%s: MinExp = %d, want %d", c.name, c.minExp, c.emin-(c.precision-1))
		}
		if c.bias != -c.minExp {
			t.Errorf("%s: Bias = %d, want %d", c.name, c.bias, -c.minExp)
		}
		// The biased exponent field holds 3 << (expBits-2) values
		// (IEEE 754-2008 clause 3.5.2); the largest is MaxExp + Bias.
		if got, want := c.maxExp+c.bias, 3<<(c.expBits-2)-1; got != want {
			t.Errorf("%s: largest biased exponent = %d, want %d", c.name, got, want)
		}
	}
	if bid.Bias64 != 398 || bid.Bias128 != 6176 {
		t.Errorf("biases = %d, %d; want 398, 6176", bid.Bias64, bid.Bias128)
	}
}

func TestSpecialEncodings(t *testing.T) {
	const wantZero64, wantZero128Hi uint64 = 0x31C0000000000000, 0x3040000000000000
	if bid.Zero64 != uint64(bid.Bias64)<<bid.ExpShift64 {
		t.Errorf("Zero64 = %#x, want Bias64 << ExpShift64", bid.Zero64)
	}
	if bid.Zero64 != wantZero64 {
		t.Errorf("Zero64 = %#x, want %#x", bid.Zero64, wantZero64)
	}
	if bid.Zero128Hi != uint64(bid.Bias128)<<bid.ExpShift128 {
		t.Errorf("Zero128Hi = %#x, want Bias128 << ExpShift128", bid.Zero128Hi)
	}
	if bid.Zero128Hi != wantZero128Hi {
		t.Errorf("Zero128Hi = %#x, want %#x", bid.Zero128Hi, wantZero128Hi)
	}
	if bid.Inf64 != 0x7800000000000000 || bid.NaN64 != 0x7C00000000000000 || bid.SNaN64 != 0x7E00000000000000 {
		t.Errorf("Inf64, NaN64, SNaN64 = %#x, %#x, %#x", bid.Inf64, bid.NaN64, bid.SNaN64)
	}
	if bid.Inf128Hi != bid.Inf64 || bid.NaN128Hi != bid.NaN64 || bid.SNaN128Hi != bid.SNaN64 || bid.SignMask128Hi != bid.SignMask64 {
		t.Errorf("decimal128 special high words differ from decimal64")
	}
	// A special value's top bits are a superset of the second form's
	// marker, so the form test must follow the special test.
	if bid.Inf64&bid.Form2Mask64 != bid.Form2Mask64 {
		t.Errorf("Inf64 does not set the second-form bits")
	}
	if bid.Zero64&bid.SpecialMask64 == bid.SpecialMask64 || bid.Zero64&bid.Form2Mask64 == bid.Form2Mask64 {
		t.Errorf("Zero64 reads as special or second form")
	}
	if bid.ExpMask64 != 1<<bid.ExpBits64-1 || bid.ExpMask128 != 1<<bid.ExpBits128-1 {
		t.Errorf("exponent masks do not match widths")
	}
	if bid.CoeffBits128 != bid.ExpShift128+64 || bid.CoeffMask128Hi != 1<<bid.ExpShift128-1 {
		t.Errorf("decimal128 coefficient field constants are inconsistent")
	}
}

func TestStatusFlags(t *testing.T) {
	if bid.Exact != 0 {
		t.Errorf("Exact = %d, want 0", bid.Exact)
	}
	flags := []bid.Status{bid.Inexact, bid.Overflow, bid.Underflow, bid.Invalid, bid.Syntax}
	var seen bid.Status
	for i, f := range flags {
		if f != 1<<i {
			t.Errorf("flag %d = %#b, want %#b", i, f, 1<<i)
		}
		if seen&f != 0 {
			t.Errorf("flag %#b is not distinct", f)
		}
		seen |= f
	}
	if len(flags) != 5 || seen != 1<<len(flags)-1 {
		t.Errorf("flags %#b are not the five low bits", seen)
	}
}

func TestKeyLayout(t *testing.T) {
	var k bid.Key
	fields := unsafe.Sizeof(k.Coeff) + unsafe.Sizeof(k.Exp) + unsafe.Sizeof(k.Kind) + unsafe.Sizeof(k.Neg) + 2
	if unsafe.Sizeof(k) != 24 || fields != unsafe.Sizeof(k) {
		t.Errorf("Key is %d bytes with %d bytes of fields; want 24 and 24", unsafe.Sizeof(k), fields)
	}
}

func TestMaxLen(t *testing.T) {
	digits := func(n int) int {
		d := 1
		for n >= 10 {
			n /= 10
			d++
		}
		return d
	}
	// Exponential notation: sign, digits, point, "E", sign, exponent.
	// The largest exponent magnitude is -MinExp (a one-digit coefficient
	// at the smallest exponent) or MaxExp + Precision - 1.
	for _, c := range []struct {
		name                           string
		precision, maxExp, minExp, max int
	}{
		{"decimal64", bid.Precision64, bid.MaxExp64, bid.MinExp64, bid.MaxLen64},
		{"decimal128", bid.Precision128, bid.MaxExp128, bid.MinExp128, bid.MaxLen128},
	} {
		e := max(-c.minExp, c.maxExp+c.precision-1)
		sci := 1 + c.precision + len(".E-") + digits(e)
		if sci > c.max {
			t.Errorf("%s: exponential form can be %d bytes, MaxLen is %d", c.name, sci, c.max)
		}
		if len("-Infinity") > c.max {
			t.Errorf("%s: MaxLen %d is shorter than -Infinity", c.name, c.max)
		}
	}
}
