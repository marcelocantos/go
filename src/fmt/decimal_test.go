// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package fmt_test

import (
	. "fmt"
	"math/decimal"
	"testing"
)

// The decimal cases of the formatting and scanning tables. They live in
// their own file because the external test package is not compiled as
// part of the standard library, so it can name decimal64 and decimal128
// only while GOEXPERIMENT=decimal is on.

type namedDecimal decimal64

var (
	decimal64Val  decimal64
	decimal128Val decimal128
)

func init() {
	fmtTests = append(fmtTests, []struct {
		fmt string
		val any
		out string
	}{
		// decimal64
		{"%e", decimal64(1.0), "1.000000e+00"},
		{"%f", decimal64(1.0), "1.000000"},
		{"%g", decimal64(1.0), "1.0"},
		{"%v", decimal64(1.0), "1.0"},
		{"%E", decimal64(1.0), "1.000000E+00"},
		{"%G", decimal64(1.0), "1.0"},
		{"%F", decimal64(1.0), "1.000000"},
		{"%e", decimal64(0.0), "0.000000e+00"},
		{"%f", decimal64(0.0), "0.000000"},
		{"%g", decimal64(0.0), "0.0"},
		{"%e", decimal64(3.14), "3.140000e+00"},
		{"%f", decimal64(3.14), "3.140000"},
		{"%g", decimal64(3.14), "3.14"},
		{"%+.3e", decimal64(0.0), "+0.000e+00"},
		{"%+.3e", decimal64(1.0), "+1.000e+00"},
		{"%+.3f", decimal64(-1.0), "-1.000"},
		{"%+07.2f", decimal64(1.0), "+001.00"},
		{"%+07.2f", decimal64(-1.0), "-001.00"},
		{"%-07.2f", decimal64(1.0), "1.00   "},
		{"%-07.2f", decimal64(-1.0), "-1.00  "},
		{"%+10.2f", decimal64(1.0), "     +1.00"},
		{"%+10.2f", decimal64(-1.0), "     -1.00"},
		{"% .3E", decimal64(-1.0), "-1.000E+00"},
		{"% .3e", decimal64(1.0), " 1.000e+00"},
		{"%+.3g", decimal64(0.0), "+0"},
		{"%+.3g", decimal64(1.0), "+1"},
		{"%+.3g", decimal64(-1.0), "-1"},
		{"% .3g", decimal64(-1.0), "-1"},
		{"% .3g", decimal64(1.0), " 1"},
		// decimal64: exact representation (unlike float, 0.1 and 0.3 are exact)
		{"%.20f", decimal64(0.3), "0.30000000000000000000"},
		{"%.20f", decimal64(0.1), "0.10000000000000000000"},
		// decimal64 infinities and NaNs
		{"%f", decimal.Inf[decimal64](1), "+Inf"},
		{"%.1f", decimal.Inf[decimal64](-1), "-Inf"},
		{"% f", decimal.NaN[decimal64](), " NaN"},
		{"%20f", decimal.Inf[decimal64](1), "                +Inf"},
		{"%20e", decimal.NaN[decimal64](), "                 NaN"},
		{"% +20E", decimal.NaN[decimal64](), "                +NaN"},
		{"% -20g", decimal.NaN[decimal64](), " NaN                "},
		{"%+-20G", decimal.NaN[decimal64](), "+NaN                "},
		{"%+020e", decimal.Inf[decimal64](1), "                +Inf"},
		{"%-020f", decimal.Inf[decimal64](-1), "-Inf                "},
		{"%-020E", decimal.NaN[decimal64](), "NaN                 "},
		// decimal128
		{"%v", decimal64(1.50), "1.50"},
		{"%g", decimal64(1.50), "1.50"},
		{"%#g", decimal64(1.50), "1.50000"},
		{"%#v", decimal64(1.50), "1.50"},
		{"%.3f", decimal64(1.50), "1.500"},
		{"%v", decimal64(1e3), "1e+03"},
		{"%v", decimal64(100), "100"},
		{"%v", decimal64(0.00), "0.00"},
		{"%v", *new(decimal64), "0"},
		{"%v", decimal128(1.50), "1.50"},
		{"%v", struct{ D namedDecimal }{1.50}, "{1.50}"},
		{"%e", decimal128(1.0), "1.000000e+00"},
		{"%f", decimal128(1.0), "1.000000"},
		{"%g", decimal128(1.0), "1.0"},
		{"%v", decimal128(1.0), "1.0"},
		{"%+.3e", decimal128(1.0), "+1.000e+00"},
		{"%+.3f", decimal128(-1.0), "-1.000"},
		// The thresholds of %v and %g, which are a float64's: exponential
		// notation when the exponent of the leading digit is below -4 or at
		// least 6, or at least the precision when there is one.
		{"%v", decimal64(999999), "999999"},
		{"%v", decimal64(1000000), "1.000000e+06"},
		{"%v", decimal64(-999999), "-999999"},
		{"%v", decimal64(-1000000), "-1.000000e+06"},
		{"%v", decimal64(999999.99), "999999.99"},
		{"%v", decimal64(1234567.89), "1.23456789e+06"},
		{"%g", decimal64(-1234567.89), "-1.23456789e+06"},
		{"%v", decimal64(0.0001), "0.0001"},
		{"%v", decimal64(0.00001), "1e-05"},
		{"%v", decimal64(-0.0001), "-0.0001"},
		{"%v", decimal64(-0.00001), "-1e-05"},
		{"%v", decimal64(1e5), "1e+05"},
		{"%.3g", decimal64(1234567.89), "1.23e+06"},
		{"%.7g", decimal64(1234567.89), "1234568"},
		{"%.10g", decimal64(1234567.89), "1234567.89"},
		{"%.3g", decimal64(0.0000123), "1.23e-05"},
		{"%v", decimal128(999999), "999999"},
		{"%v", decimal128(1000000), "1.000000e+06"},
		{"%v", decimal128(-999999), "-999999"},
		{"%v", decimal128(-1000000), "-1.000000e+06"},
		{"%v", decimal128(1234567.89), "1.23456789e+06"},
		{"%v", decimal128(0.0001), "0.0001"},
		{"%v", decimal128(0.00001), "1e-05"},
		{"%v", decimal128(-0.0001), "-0.0001"},
		{"%v", decimal128(-0.00001), "-1e-05"},
		{"%v", decimal128(123456.7890123456789012345678901234), "123456.7890123456789012345678901234"},
		{"%v", decimal128(1234567.890123456789012345678901234), "1.234567890123456789012345678901234e+06"},
		{"%G", decimal128(1234567.89), "1.23456789E+06"},
		{"%.3g", decimal128(-1234567.89), "-1.23e+06"},
		// %b is the coefficient and the decimal exponent of its last
		// digit, the radix-ten form of a float's %b; the precision and #
		// do not apply, as for a float.
		{"%b", decimal64(1.50), "150e-2"},
		{"%b", decimal128(-123456e-78), "-123456e-78"},
		{"%b", decimal64(1e3), "1e+3"},
		{"%b", decimal64(0), "0e+0"},
		{"%+b", decimal64(1.5), "+15e-1"},
		{"%8b", decimal64(1.5), "   15e-1"},
		{"%-8b|", decimal64(1.5), "15e-1   |"},
		{"%08b", decimal64(-1.5), "-0015e-1"},
		{"%.2b", decimal64(1.500), "1500e-3"},
		{"%#b", decimal64(1.5), "15e-1"},
		{"%b", namedDecimal(2.5), "25e-1"},
		// %x and %X are a hexadecimal fraction and a binary exponent, as
		// for a float: the value correctly rounded, or at no precision the
		// fewest digits that read back as the value.
		{"%x", decimal64(1.5), "0x1.8p+00"},
		{"%X", decimal128(1.5), "0X1.8P+00"},
		{"%x", decimal64(0.1), "0x1.999999999999ap-04"},
		{"%.3x", decimal64(0.1), "0x1.99ap-04"},
		{"%x", decimal64(0), "0x0p+00"},
		{"%x", decimal64(1e-398), "0x1p-1322"},
	}...)

	scanTests = append(scanTests, []ScanTest{
		// Decimals
		{"2.3\n", &decimal64Val, decimal64(2.3)},
		{"2.3e1\n", &decimal128Val, decimal128(2.3e1)},
		{"-1.5\n", &decimal64Val, decimal64(-1.5)},
		{"0\n", &decimal64Val, decimal64(0)},
		{"123.456\n", &decimal128Val, decimal128(123.456)},
		// What a float accepts: hexadecimal, underscores and the form
		// with a binary exponent that %b writes for a float.
		{"0x1p-2\n", &decimal64Val, decimal64(0.25)},
		{"0x1.8p1\n", &decimal128Val, decimal128(3)},
		{"1_000.5\n", &decimal128Val, decimal128(1000.5)},
		{"1.2p4\n", &decimal64Val, decimal64(19.2)},
		{"-3p-2\n", &decimal128Val, decimal128(-0.75)},
		{"150e-2\n", &decimal64Val, decimal64(1.50)},
	}...)

	scanfTests = append(scanfTests, []ScanfTest{
		// Decimals
		{"%f", "2.3\n", &decimal64Val, decimal64(2.3)},
		{"%e", "2.3e1\n", &decimal128Val, decimal128(2.3e1)},
		{"%g", "123.456\n", &decimal64Val, decimal64(123.456)},
		{"%v", "-1.5\n", &decimal128Val, decimal128(-1.5)},
		{"%E", "0\n", &decimal64Val, decimal64(0)},
	}...)

	eofTests = append(eofTests, []struct {
		format string
		v      any
	}{
		{"%v", &decimal64Val},
		{"%v", &decimal128Val},
	}...)
}

// TestScanDecimalLikeFloat checks that Sscan reads into a decimal what it
// reads into a float, with the value and quantum ParseDecimal gives.
func TestScanDecimalLikeFloat(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"1.50", "1.50"},
		{"1_000.50", "1000.50"},
		{"0x1p-2", "0.25"},
		{"0x10p0", "16"},
		{"1.2p4", "19.2"},
		{"1p-1", "0.5"},
		{"1_0p1", "20"},
		{"+Inf", "+Inf"},
		{"-inf", "-Inf"},
		{"nan", "NaN"},
		{"-nan", ""},
		{"snan", ""},
		{"0x", ""},
		{"1.2p", ""},
		{"1.2p_4", ""},
	} {
		var f float64
		_, ferr := Sscan(tt.in, &f)
		var d decimal64
		_, derr := Sscan(tt.in, &d)
		if (ferr == nil) != (derr == nil) {
			t.Errorf("Sscan(%q): decimal64 error %v, float64 error %v", tt.in, derr, ferr)
			continue
		}
		if derr == nil && Sprint(d) != tt.want {
			t.Errorf("Sscan(%q) into decimal64 = %v, want %s", tt.in, d, tt.want)
		}
	}
}

// TestDecimalHexLikeFloat checks %x and %X, with their flags, width and
// precision, against a float64 of the same value.
func TestDecimalHexLikeFloat(t *testing.T) {
	values := []float64{1.5, -2.25, 0.5, 1024, 3.75, 1.0 / 1024, 0}
	formats := []string{"%x", "%X", "%.0x", "%.3x", "%.13X", "%+x", "% x", "%-14x|", "%14x", "%014x", "%#x", "%#.2x", "%#X", "%#.0x", "%+.2X"}
	for _, f := range values {
		d := decimal64(f)
		for _, format := range formats {
			if got, want := Sprintf(format, d), Sprintf(format, f); got != want {
				t.Errorf("Sprintf(%q, decimal64(%v)) = %q; for float64 %q", format, f, got, want)
			}
			if got, want := Sprintf(format, decimal128(f)), Sprintf(format, f); got != want {
				t.Errorf("Sprintf(%q, decimal128(%v)) = %q; for float64 %q", format, f, got, want)
			}
		}
	}
	// fmt's limit on precision applies, as for a float.
	for _, p := range []int{1_000_000, 1_000_001} {
		got, want := Sprintf("%.*x", p, decimal64(1.5)), Sprintf("%.*x", p, 1.5)
		if got != want {
			t.Errorf("Sprintf(%%.*x, %d, decimal64(1.5)) = %.40q…; for float64 %.40q…", p, got, want)
		}
	}
}
