// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package fmt_test

import "math/decimal"

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
	}...)

	scanTests = append(scanTests, []ScanTest{
		// Decimals
		{"2.3\n", &decimal64Val, decimal64(2.3)},
		{"2.3e1\n", &decimal128Val, decimal128(2.3e1)},
		{"-1.5\n", &decimal64Val, decimal64(-1.5)},
		{"0\n", &decimal64Val, decimal64(0)},
		{"123.456\n", &decimal128Val, decimal128(123.456)},
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
