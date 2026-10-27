// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import (
	"math/bits"
	"testing"
	"unsafe"
)

// pow10 returns 10^n as a Uint128 and reports whether it fit.
func pow10(n int) (Uint128, bool) {
	p := Uint128{Lo: 1}
	for i := 0; i < n; i++ {
		var carry uint64
		carry, p.Lo = bits.Mul64(p.Lo, 10)
		overflow, hi := bits.Mul64(p.Hi, 10)
		if overflow != 0 {
			return Uint128{}, false
		}
		var c uint64
		p.Hi, c = bits.Add64(hi, carry, 0)
		if c != 0 {
			return Uint128{}, false
		}
	}
	return p, true
}

func TestMaxCoeff(t *testing.T) {
	p16, ok := pow10(Precision64)
	if !ok || p16.Hi != 0 {
		t.Fatalf("10^%d does not fit a uint64", Precision64)
	}
	if MaxCoeff64 != p16.Lo-1 {
		t.Errorf("MaxCoeff64 = %d, want 10^%d-1 = %d", MaxCoeff64, Precision64, p16.Lo-1)
	}
	if MaxCoeff64 >= 1<<(Form2CoeffBits64+3) {
		t.Errorf("MaxCoeff64 does not fit the second encoding form")
	}

	p34, ok := pow10(Precision128)
	if !ok {
		t.Fatalf("10^%d does not fit a Uint128", Precision128)
	}
	lo, borrow := bits.Sub64(p34.Lo, 1, 0)
	hi, _ := bits.Sub64(p34.Hi, 0, borrow)
	if MaxCoeff128Hi != hi || MaxCoeff128Lo != lo {
		t.Errorf("MaxCoeff128 = %#x:%#x, want 10^%d-1 = %#x:%#x", MaxCoeff128Hi, MaxCoeff128Lo, Precision128, hi, lo)
	}
	if MaxCoeff128Hi >= 1<<(CoeffBits128-64) {
		t.Errorf("MaxCoeff128 does not fit the first encoding form")
	}
}

func TestExponentRange(t *testing.T) {
	cases := []struct {
		name                          string
		precision, emax, emin         int
		maxExp, minExp, bias, expBits int
	}{
		{"decimal64", Precision64, Emax64, Emin64, MaxExp64, MinExp64, Bias64, ExpBits64},
		{"decimal128", Precision128, Emax128, Emin128, MaxExp128, MinExp128, Bias128, ExpBits128},
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
	if Bias64 != 398 || Bias128 != 6176 {
		t.Errorf("biases = %d, %d; want 398, 6176", Bias64, Bias128)
	}
}

func TestSpecialEncodings(t *testing.T) {
	const wantZero64, wantZero128Hi = 0x31C0000000000000, 0x3040000000000000
	if Zero64 != uint64(Bias64)<<ExpShift64 {
		t.Errorf("Zero64 = %#x, want Bias64 << ExpShift64", Zero64)
	}
	if Zero64 != wantZero64 {
		t.Errorf("Zero64 = %#x, want %#x", Zero64, wantZero64)
	}
	if Zero128Hi != uint64(Bias128)<<ExpShift128 {
		t.Errorf("Zero128Hi = %#x, want Bias128 << ExpShift128", Zero128Hi)
	}
	if Zero128Hi != wantZero128Hi {
		t.Errorf("Zero128Hi = %#x, want %#x", Zero128Hi, wantZero128Hi)
	}
	if Inf64 != 0x7800000000000000 || NaN64 != 0x7C00000000000000 || SNaN64 != 0x7E00000000000000 {
		t.Errorf("Inf64, NaN64, SNaN64 = %#x, %#x, %#x", Inf64, NaN64, SNaN64)
	}
	if Inf128Hi != Inf64 || NaN128Hi != NaN64 || SNaN128Hi != SNaN64 || SignMask128Hi != SignMask64 {
		t.Errorf("decimal128 special high words differ from decimal64")
	}
	// A special value's top bits are a superset of the second form's
	// marker, so the form test must follow the special test.
	if Inf64&Form2Mask64 != Form2Mask64 {
		t.Errorf("Inf64 does not set the second-form bits")
	}
	if Zero64&SpecialMask64 == SpecialMask64 || Zero64&Form2Mask64 == Form2Mask64 {
		t.Errorf("Zero64 reads as special or second form")
	}
	if ExpMask64 != 1<<ExpBits64-1 || ExpMask128 != 1<<ExpBits128-1 {
		t.Errorf("exponent masks do not match widths")
	}
	if CoeffBits128 != ExpShift128+64 || CoeffMask128Hi != 1<<ExpShift128-1 {
		t.Errorf("decimal128 coefficient field constants are inconsistent")
	}
}

func TestStatusFlags(t *testing.T) {
	if Exact != 0 {
		t.Errorf("Exact = %d, want 0", Exact)
	}
	flags := []Status{Inexact, Overflow, Underflow, Invalid, Syntax}
	var seen Status
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
	var k Key
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
		{"decimal64", Precision64, MaxExp64, MinExp64, MaxLen64},
		{"decimal128", Precision128, MaxExp128, MinExp128, MaxLen128},
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
