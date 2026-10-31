// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math"
	"math/big"
	"testing"
)

// The integer conversions at the places where an implementation error
// passes ordinary tests: the magnitude of math.MinInt64, math.MaxUint64,
// values that round up across the edge of the integer range, a scale that
// overflows the coefficient, and a double rounding on the way in. Each
// case has a result derived by hand from the contract ("Integer
// conversions") and is checked, in addition, against the reference model
// in every mode. The differential test TestKernel covers the bulk.

var allModes = []bid.Mode{bid.ToNearestEven, bid.ToNearestAway, bid.TowardZero, bid.TowardPositive, bid.TowardNegative}

// decimalOf returns the encoding of (-1)^neg × coeff × 10^exp in f.
func decimalOf(f *format, neg bool, coeff string, exp int) bits {
	c, ok := new(big.Int).SetString(coeff, 10)
	if !ok {
		panic("decimalOf: bad coefficient " + coeff)
	}
	return f.encode(value{kind: finite, neg: neg, coeff: c, exp: exp})
}

func TestIntegerToDecimal(t *testing.T) {
	type tc struct {
		name string
		neg  bool
		mag  string // decimal magnitude, as an int64 or a uint64
		mode bid.Mode
		// The decimal64 result as coefficient × 10^exp. The decimal128
		// result is always the integer itself at exponent 0.
		coeff string
		exp   int
	}
	tests := []tc{
		{"zero", false, "0", bid.ToNearestEven, "0", 0},
		{"sixteen digits", false, "9999999999999999", bid.TowardZero, "9999999999999999", 0},
		{"seventeen digits, exact", false, "10000000000000000", bid.TowardZero, "1000000000000000", 1},
		{"seventeen digits, round down", false, "10000000000000004", bid.ToNearestEven, "1000000000000000", 1},
		{"seventeen digits, tie to even", false, "10000000000000005", bid.ToNearestEven, "1000000000000000", 1},
		{"seventeen digits, tie to odd", false, "10000000000000015", bid.ToNearestEven, "1000000000000002", 1},
		{"seventeen digits, tie away", false, "10000000000000005", bid.ToNearestAway, "1000000000000001", 1},
		{"seventeen digits, toward positive", false, "10000000000000001", bid.TowardPositive, "1000000000000001", 1},
		{"seventeen digits, toward negative", true, "10000000000000001", bid.TowardNegative, "1000000000000001", 1},
		{"seventeen digits, toward positive, negative", true, "10000000000000001", bid.TowardPositive, "1000000000000000", 1},
		// The audit case: one rounding of 100000000000000149 gives …001E2;
		// rounding to 17 digits first and then to 16 gives …002E2.
		{"double rounding", false, "100000000000000149", bid.ToNearestEven, "1000000000000001", 2},
		{"carry out", false, "99999999999999995", bid.ToNearestAway, "1000000000000000", 2},
		{"MaxInt64, nearest", false, "9223372036854775807", bid.ToNearestEven, "9223372036854776", 3},
		{"MaxInt64, toward zero", false, "9223372036854775807", bid.TowardZero, "9223372036854775", 3},
		{"MinInt64, nearest", true, "9223372036854775808", bid.ToNearestEven, "9223372036854776", 3},
		{"MinInt64, toward zero", true, "9223372036854775808", bid.TowardZero, "9223372036854775", 3},
		{"MinInt64, toward positive", true, "9223372036854775808", bid.TowardPositive, "9223372036854775", 3},
		{"MinInt64, toward negative", true, "9223372036854775808", bid.TowardNegative, "9223372036854776", 3},
		{"MaxUint64, nearest", false, "18446744073709551615", bid.ToNearestEven, "1844674407370955", 4},
		{"MaxUint64, away", false, "18446744073709551615", bid.ToNearestAway, "1844674407370955", 4},
		{"MaxUint64, toward positive", false, "18446744073709551615", bid.TowardPositive, "1844674407370956", 4},
		{"MaxUint64, toward zero", false, "18446744073709551615", bid.TowardZero, "1844674407370955", 4},
	}
	for _, tt := range tests {
		mag, _ := new(big.Int).SetString(tt.mag, 10)
		sign := ""
		if tt.neg {
			sign = "-"
		}
		check := func(f *format, got bits, what string) {
			t.Helper()
			var want bits
			if f == dec64 {
				want = decimalOf(f, tt.neg, tt.coeff, tt.exp)
			} else {
				want = decimalOf(f, tt.neg, tt.mag, 0)
			}
			if got != want {
				t.Errorf("%s: %s(%s%s, %s) = %s, want %s", tt.name, what, sign, tt.mag, modeName(tt.mode), display(f, got), display(f, want))
			}
		}
		if mag.IsInt64() || tt.neg && mag.Cmp(new(big.Int).Lsh(bigOne, 63)) == 0 {
			x := new(big.Int).Set(mag)
			if tt.neg {
				x.Neg(x)
			}
			check(dec64, lo(bid.Int64To64(x.Int64(), tt.mode)), "Int64To64")
			check(dec128, bid.Int64To128(x.Int64()), "Int64To128")
		}
		if !tt.neg {
			check(dec64, lo(bid.Uint64To64(mag.Uint64(), tt.mode)), "Uint64To64")
			check(dec128, bid.Uint64To128(mag.Uint64()), "Uint64To128")
		}
	}

	// Every boundary against the reference, in every mode.
	ints := []int64{0, 1, -1, 9, 10, 999999999999999, 9999999999999999, 10000000000000000, 10000000000000001,
		99999999999999995, 100000000000000149, 100000000000000150, 100000000000000151, 999999999999999999,
		1 << 53, 1<<53 + 1, math.MaxInt64, math.MaxInt64 - 1, math.MinInt64, math.MinInt64 + 1, -9999999999999995, -10000000000000005}
	for _, v := range ints {
		for _, u := range []int64{v, -v} {
			for _, m := range allModes {
				if g, w := (kernel{}).int64To(dec64, u, m), (reference{}).int64To(dec64, u, m); g != w {
					t.Errorf("Int64To64(%d, %s) = %s, want %s", u, modeName(m), display(dec64, g), display(dec64, w))
				}
				if g, w := (kernel{}).int64To(dec128, u, m), (reference{}).int64To(dec128, u, m); g != w {
					t.Errorf("Int64To128(%d) = %s, want %s", u, display(dec128, g), display(dec128, w))
				}
			}
		}
	}
	uints := []uint64{0, 1, math.MaxUint64, math.MaxUint64 - 1, 1 << 63, 1<<63 + 1, 1<<63 - 1, 10000000000000000000, 9999999999999999999, 10000000000000000005, 18446744073709551610}
	for _, u := range uints {
		for _, m := range allModes {
			if g, w := (kernel{}).uint64To(dec64, u, m), (reference{}).uint64To(dec64, u, m); g != w {
				t.Errorf("Uint64To64(%d, %s) = %s, want %s", u, modeName(m), display(dec64, g), display(dec64, w))
			}
			if g, w := (kernel{}).uint64To(dec128, u, m), (reference{}).uint64To(dec128, u, m); g != w {
				t.Errorf("Uint64To128(%d) = %s, want %s", u, display(dec128, g), display(dec128, w))
			}
		}
	}
}

func TestDecimalToInteger(t *testing.T) {
	// Operands are coefficient and exponent; both widths are tried when the
	// operand fits decimal64. The results are for the given mode.
	type tc struct {
		name  string
		neg   bool
		coeff string
		exp   int
		mode  bid.Mode
		wide  bool // decimal128 only: the coefficient has more than 16 digits

		i  int64
		ie bid.Status
		u  uint64
		ue bid.Status
	}
	const (
		ex  = bid.Exact
		in  = bid.Inexact
		ov  = bid.Inexact | bid.Overflow
		min = math.MinInt64
		max = math.MaxInt64
		umx = math.MaxUint64
	)
	nearest, away, zero, pos, neg := bid.ToNearestEven, bid.ToNearestAway, bid.TowardZero, bid.TowardPositive, bid.TowardNegative
	tests := []tc{
		{"zero", false, "0", 0, zero, false, 0, ex, 0, ex},
		{"negative zero", true, "0", 0, zero, false, 0, ex, 0, ex},
		{"zero at a huge exponent", false, "0", 369, zero, false, 0, ex, 0, ex},
		{"zero at a tiny exponent", true, "0", -398, pos, false, 0, ex, 0, ex},
		{"exact integer", false, "123", 0, zero, false, 123, ex, 123, ex},
		{"exact by exponent", false, "5", 3, zero, false, 5000, ex, 5000, ex},
		{"fraction, toward zero", true, "15", -1, zero, false, -1, in, 0, ov},
		{"fraction, nearest even", true, "25", -1, nearest, false, -2, in, 0, ov},
		{"fraction, away", true, "25", -1, away, false, -3, in, 0, ov},
		{"fraction, toward negative", false, "15", -1, neg, false, 1, in, 1, in},
		{"fraction, toward positive", true, "15", -1, pos, false, -1, in, 0, ov},
		{"negative below one, toward zero", true, "5", -1, zero, false, 0, in, 0, in},
		{"negative below one, nearest even", true, "5", -1, nearest, false, 0, in, 0, in},
		{"negative below one, away", true, "5", -1, away, false, -1, in, 0, ov},
		{"negative below one, toward negative", true, "1", -398, neg, false, -1, in, 0, ov},
		{"positive below one, toward positive", false, "1", -398, pos, false, 1, in, 1, in},
		{"far below one", false, "9999999999999999", -398, nearest, false, 0, in, 0, in},
		// The range edges of int64.
		{"MaxInt64 exact", false, "9223372036854775807", 0, zero, true, max, ex, 9223372036854775807, ex},
		{"MaxInt64 plus a half, toward zero", false, "92233720368547758075", -1, zero, true, max, in, 9223372036854775807, in},
		{"MaxInt64 plus a half, nearest", false, "92233720368547758075", -1, nearest, true, max, ov, 9223372036854775808, in},
		{"MaxInt64 plus a half, toward positive", false, "92233720368547758075", -1, pos, true, max, ov, 9223372036854775808, in},
		{"MaxInt64 plus a half, toward negative", false, "92233720368547758075", -1, neg, true, max, in, 9223372036854775807, in},
		{"2^63", false, "9223372036854775808", 0, zero, true, max, ov, 9223372036854775808, ex},
		{"MinInt64 exact", true, "9223372036854775808", 0, zero, true, min, ex, 0, ov},
		{"MinInt64 minus a half, toward zero", true, "92233720368547758085", -1, zero, true, min, in, 0, ov},
		{"MinInt64 minus a half, nearest", true, "92233720368547758085", -1, nearest, true, min, in, 0, ov},
		{"MinInt64 minus a half, away", true, "92233720368547758085", -1, away, true, min, ov, 0, ov},
		{"MinInt64 minus a half, toward negative", true, "92233720368547758085", -1, neg, true, min, ov, 0, ov},
		{"MinInt64 minus a half, toward positive", true, "92233720368547758085", -1, pos, true, min, in, 0, ov},
		{"MinInt64 minus one", true, "9223372036854775809", 0, zero, true, min, ov, 0, ov},
		// The range edges of uint64.
		{"MaxUint64 exact", false, "18446744073709551615", 0, zero, true, max, ov, umx, ex},
		{"MaxUint64 plus a half, toward zero", false, "184467440737095516155", -1, zero, true, max, ov, umx, in},
		{"MaxUint64 plus a half, nearest", false, "184467440737095516155", -1, nearest, true, max, ov, umx, ov},
		{"MaxUint64 plus a half, toward negative", false, "184467440737095516155", -1, neg, true, max, ov, umx, in},
		{"2^64", false, "18446744073709551616", 0, zero, true, max, ov, umx, ov},
		{"2^64 negative", true, "18446744073709551616", 0, zero, true, min, ov, 0, ov},
		// Scales that overflow the coefficient.
		{"1E19", false, "1", 19, zero, false, max, ov, 10000000000000000000, ex},
		{"1E20", false, "1", 20, zero, false, max, ov, umx, ov},
		{"-1E19", true, "1", 19, zero, false, min, ov, 0, ov},
		{"1E369", false, "1", 369, zero, false, max, ov, umx, ov},
		{"9999999999999999E369", true, "9999999999999999", 369, away, false, min, ov, 0, ov},
		{"1E6111", false, "1", 6111, zero, true, max, ov, umx, ov},
		{"-1E6111", true, "1", 6111, zero, true, min, ov, 0, ov},
		{"9999999999999999999999999999999999E6111", false, "9999999999999999999999999999999999", 6111, zero, true, max, ov, umx, ov},
		{"1E-6176 toward positive", false, "1", -6176, pos, true, 1, in, 1, in},
		{"huge coefficient, small fraction", false, "9999999999999999999999999999999999", -6176, neg, true, 0, in, 0, in},
	}
	for _, tt := range tests {
		for _, f := range []*format{dec64, dec128} {
			if f == dec64 && (tt.wide || tt.exp > dec64.maxExp || tt.exp < dec64.minExp) {
				continue
			}
			x := decimalOf(f, tt.neg, tt.coeff, tt.exp)
			if gi, gs := (kernel{}).int64From(f, x, tt.mode); gi != tt.i || gs != tt.ie {
				t.Errorf("%s: Int64From%d(%s, %s) = %d, %v; want %d, %v", tt.name, f.width, display(f, x), modeName(tt.mode), gi, gs, tt.i, tt.ie)
			}
			if gu, gs := (kernel{}).uint64From(f, x, tt.mode); gu != tt.u || gs != tt.ue {
				t.Errorf("%s: Uint64From%d(%s, %s) = %d, %v; want %d, %v", tt.name, f.width, display(f, x), modeName(tt.mode), gu, gs, tt.u, tt.ue)
			}
		}
	}

	// Every case of the table against the reference in every mode, and the
	// special values.
	for _, tt := range tests {
		for _, f := range []*format{dec64, dec128} {
			if f == dec64 && (tt.wide || tt.exp > dec64.maxExp || tt.exp < dec64.minExp) {
				continue
			}
			x := decimalOf(f, tt.neg, tt.coeff, tt.exp)
			checkIntFrom(t, f, x)
		}
	}
	for _, f := range []*format{dec64, dec128} {
		for _, v := range []value{{kind: notANumber}, {kind: notANumber, signaling: true}, {kind: infinite}, {kind: infinite, neg: true}} {
			x := f.encode(v)
			checkIntFrom(t, f, x)
		}
	}
}

func checkIntFrom(t *testing.T, f *format, x bits) {
	t.Helper()
	for _, m := range allModes {
		gi, gs := (kernel{}).int64From(f, x, m)
		wi, ws := (reference{}).int64From(f, x, m)
		if gi != wi || gs != ws {
			t.Errorf("Int64From%d(%s, %s) = %d, %v; reference %d, %v", f.width, display(f, x), modeName(m), gi, gs, wi, ws)
		}
		gu, gs := (kernel{}).uint64From(f, x, m)
		wu, ws := (reference{}).uint64From(f, x, m)
		if gu != wu || gs != ws {
			t.Errorf("Uint64From%d(%s, %s) = %d, %v; reference %d, %v", f.width, display(f, x), modeName(m), gu, gs, wu, ws)
		}
	}
}

func FuzzIntegerConversions64(f *testing.F) {
	f.Add(int64(math.MinInt64), uint64(math.MaxUint64), false, uint64(1234567890123456), int16(-3), uint64(0), uint8(0))
	f.Add(int64(100000000000000149), uint64(10000000000000000005), true, uint64(9999999999999999), int16(369), uint64(0), uint8(3))
	f.Add(int64(math.MaxInt64), uint64(1<<63), true, uint64(5), int16(-1), uint64(0), uint8(4))
	f.Add(int64(0), uint64(0), false, uint64(0), int16(0), uint64(0x7C00000000000000), uint8(2))
	f.Fuzz(func(t *testing.T, i int64, u uint64, neg bool, coeff uint64, exp int16, raw uint64, mode uint8) {
		m := bid.Mode(mode)
		if g, w := (kernel{}).int64To(dec64, i, m), (reference{}).int64To(dec64, i, m); g != w {
			t.Errorf("Int64To64(%d, %s) = %s, want %s", i, modeName(m), display(dec64, g), display(dec64, w))
		}
		if g, w := (kernel{}).uint64To(dec64, u, m), (reference{}).uint64To(dec64, u, m); g != w {
			t.Errorf("Uint64To64(%d, %s) = %s, want %s", u, modeName(m), display(dec64, g), display(dec64, w))
		}
		x, ok := bid.Pack64(neg, coeff%(bid.MaxCoeff64+1), int(exp)%(bid.MaxExp64-bid.MinExp64+1)+bid.MinExp64)
		if !ok || raw != 0 {
			x = raw // any bit pattern: non-canonical, infinity, NaN
		}
		checkIntFromMode(t, dec64, lo(x), m)
	})
}

func FuzzIntegerConversions128(f *testing.F) {
	f.Add(int64(math.MinInt64), uint64(math.MaxUint64), false, uint64(0), uint64(92233720368547758), int16(-1), uint64(0), uint8(0))
	f.Add(int64(100000000000000149), uint64(10000000000000000005), true, uint64(1), uint64(5), int16(6111), uint64(0), uint8(3))
	f.Add(int64(math.MaxInt64), uint64(1<<63), true, uint64(0), uint64(184467440737095516), int16(-1), uint64(0), uint8(4))
	f.Add(int64(0), uint64(0), false, uint64(0), uint64(0), int16(0), uint64(0x7C00000000000000), uint8(2))
	f.Fuzz(func(t *testing.T, i int64, u uint64, neg bool, chi, clo uint64, exp int16, raw uint64, mode uint8) {
		m := bid.Mode(mode)
		if g, w := (kernel{}).int64To(dec128, i, m), (reference{}).int64To(dec128, i, m); g != w {
			t.Errorf("Int64To128(%d) = %s, want %s", i, display(dec128, g), display(dec128, w))
		}
		if g, w := (kernel{}).uint64To(dec128, u, m), (reference{}).uint64To(dec128, u, m); g != w {
			t.Errorf("Uint64To128(%d) = %s, want %s", u, display(dec128, g), display(dec128, w))
		}
		c := bid.Uint128{Hi: chi & (1<<49 - 1), Lo: clo}
		x, ok := bid.Pack128(neg, c, int(exp)%(bid.MaxExp128-bid.MinExp128+1)+bid.MinExp128)
		if !ok || raw != 0 {
			x = bid.Uint128{Hi: raw, Lo: clo} // any bit pattern
		}
		checkIntFromMode(t, dec128, x, m)
	})
}

func checkIntFromMode(t *testing.T, f *format, x bits, m bid.Mode) {
	t.Helper()
	gi, gs := (kernel{}).int64From(f, x, m)
	wi, ws := (reference{}).int64From(f, x, m)
	if gi != wi || gs != ws {
		t.Errorf("Int64From%d(%s, %s) = %d, %v; reference %d, %v", f.width, display(f, x), modeName(m), gi, gs, wi, ws)
	}
	gu, gs := (kernel{}).uint64From(f, x, m)
	wu, ws := (reference{}).uint64From(f, x, m)
	if gu != wu || gs != ws {
		t.Errorf("Uint64From%d(%s, %s) = %d, %v; reference %d, %v", f.width, display(f, x), modeName(m), gu, gs, wu, ws)
	}
}
