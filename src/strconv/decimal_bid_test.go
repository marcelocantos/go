// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package strconv_test

import (
	"math"
	. "strconv"
	"testing"
)

// --- FormatDecimal64 / AppendDecimal64 tests ---

type dtoaTest struct {
	d    decimal64
	fmt  byte
	prec int
	s    string
}

var dtoaTests = []dtoaTest{
	// Basic values, 'g' shortest
	{decimal64(0), 'g', -1, "0"},
	{decimal64(1), 'g', -1, "1"},
	{decimal64(-1), 'g', -1, "-1"},
	{decimal64(20), 'g', -1, "20"},
	{decimal64(100), 'g', -1, "100"},

	// Fractional, 'g' shortest
	{decimal64(0.1), 'g', -1, "0.1"},
	{decimal64(0.3), 'g', -1, "0.3"},
	{decimal64(3.14), 'g', -1, "3.14"},
	{decimal64(1234567.8), 'g', -1, "1234567.8"},

	// 'e' format
	{decimal64(0), 'e', 6, "0.000000e+00"},
	{decimal64(1), 'e', 6, "1.000000e+00"},
	{decimal64(-1), 'e', 6, "-1.000000e+00"},
	{decimal64(20), 'e', 6, "2.000000e+01"},
	{decimal64(100), 'e', 6, "1.000000e+02"},
	{decimal64(0.1), 'e', 6, "1.000000e-01"},
	{decimal64(3.14), 'e', 6, "3.140000e+00"},
	{decimal64(1234567.8), 'e', 6, "1.234568e+06"},

	// 'E' format
	{decimal64(1), 'E', 6, "1.000000E+00"},
	{decimal64(3.14), 'E', 3, "3.140E+00"},

	// 'f' format
	{decimal64(0), 'f', 6, "0.000000"},
	{decimal64(1), 'f', 6, "1.000000"},
	{decimal64(-1), 'f', 6, "-1.000000"},
	{decimal64(20), 'f', 6, "20.000000"},
	{decimal64(0.1), 'f', 6, "0.100000"},
	{decimal64(3.14), 'f', 6, "3.140000"},
	{decimal64(1234567.8), 'f', 6, "1234567.800000"},

	// 'G' format
	{decimal64(1), 'G', 5, "1"},
	{decimal64(100), 'G', 5, "100"},

	// 'g' with explicit precision
	{decimal64(0), 'g', 5, "0"},
	{decimal64(1), 'g', 5, "1"},
	{decimal64(100), 'g', 5, "100"},
	{decimal64(400), 'g', 2, "4e+02"},
	{decimal64(40), 'g', 2, "40"},
	{decimal64(4), 'g', 2, "4"},
	{decimal64(0.4), 'g', 2, "0.4"},
	{decimal64(0.04), 'g', 2, "0.04"},
	{decimal64(0.004), 'g', 2, "0.004"},
	{decimal64(0.0004), 'g', 2, "0.0004"},

	// Precision 0
	{decimal64(1), 'e', 0, "1e+00"},
	{decimal64(1), 'f', 0, "1"},
	{decimal64(1), 'g', 0, "1"},

	// Rounding (round-to-even)
	{decimal64(12.345), 'f', 2, "12.34"},  // 4 is even, round down
	{decimal64(12.355), 'f', 2, "12.36"},  // 5 is odd, round up
	{decimal64(0.015), 'f', 2, "0.02"},    // 1 is odd, round up
	{decimal64(0.025), 'f', 2, "0.02"},    // 2 is even, round down
	{decimal64(0.035), 'f', 2, "0.04"},    // 3 is odd, round up
	{decimal64(0.045), 'f', 2, "0.04"},    // 4 is even, round down

	// Special values
	{math.Decimal64NaN(), 'g', -1, "NaN"},
	{math.Decimal64NaN(), 'e', 6, "NaN"},
	{math.Decimal64NaN(), 'f', 6, "NaN"},
	{math.Decimal64Inf(1), 'g', -1, "+Inf"},
	{math.Decimal64Inf(1), 'e', 6, "+Inf"},
	{math.Decimal64Inf(1), 'f', 6, "+Inf"},
	{math.Decimal64Inf(-1), 'g', -1, "-Inf"},
	{math.Decimal64Inf(-1), 'e', 6, "-Inf"},
	{math.Decimal64Inf(-1), 'f', 6, "-Inf"},

	// Negative zero
	{math.Decimal64frombits(1 << 63), 'g', -1, "-0"},
	{math.Decimal64frombits(1 << 63), 'e', 6, "-0.000000e+00"},
	{math.Decimal64frombits(1 << 63), 'f', 6, "-0.000000"},
	{math.Decimal64frombits(1 << 63), 'f', 1, "-0.0"},

	// Decimal-specific: exact representation (unlike float)
	{decimal64(0.1), 'f', 20, "0.10000000000000000000"},
	{decimal64(0.3), 'f', 20, "0.30000000000000000000"},
}

func TestFormatDecimal64(t *testing.T) {
	for i, test := range dtoaTests {
		s := FormatDecimal64(test.d, test.fmt, test.prec)
		if s != test.s {
			t.Errorf("#%d: FormatDecimal64(%v, %c, %d) = %q, want %q",
				i, test.d, test.fmt, test.prec, s, test.s)
		}
	}
}

func TestAppendDecimal64(t *testing.T) {
	for i, test := range dtoaTests {
		b := AppendDecimal64([]byte("abc"), test.d, test.fmt, test.prec)
		if string(b) != "abc"+test.s {
			t.Errorf("#%d: AppendDecimal64(\"abc\", %v, %c, %d) = %q, want %q",
				i, test.d, test.fmt, test.prec, string(b), "abc"+test.s)
		}
	}
}

// --- FormatDecimal128 / AppendDecimal128 tests ---

func mustParseD128(s string) decimal128 {
	d, err := ParseDecimal128(s)
	if err != nil {
		panic("mustParseD128: " + err.Error())
	}
	return d
}

type dtoa128Test struct {
	d    decimal128
	fmt  byte
	prec int
	s    string
}

var dtoa128Tests = []dtoa128Test{
	// Basic values
	{decimal128(0), 'g', -1, "0"},
	{decimal128(1), 'g', -1, "1"},
	{decimal128(-1), 'g', -1, "-1"},
	{decimal128(3.14), 'g', -1, "3.14"},
	{decimal128(0.1), 'g', -1, "0.1"},

	// Format codes
	{decimal128(1), 'e', 6, "1.000000e+00"},
	{decimal128(1), 'f', 6, "1.000000"},
	{decimal128(1), 'E', 3, "1.000E+00"},
	{decimal128(1), 'G', 5, "1"},

}


func init() {
	// Add test case requiring ParseDecimal128 (34-digit precision)
	dtoa128Tests = append(dtoa128Tests, dtoa128Test{
		mustParseD128("1234567890123456789012345678901234"),
		'g', -1, "1234567890123456789012345678901234",
	})

	// Extreme exponents (BID128 can reach ~6176 digits in the exponent).
	dtoa128Tests = append(dtoa128Tests,
		dtoa128Test{mustParseD128("1e+6000"), 'e', 0, "1e+6000"},
		dtoa128Test{mustParseD128("1e-6000"), 'e', 0, "1e-6000"},
		dtoa128Test{mustParseD128("1e+6111"), 'e', 0, "1e+6111"},
		dtoa128Test{mustParseD128("1e-6176"), 'e', 0, "1e-6176"},
	)
}

func TestFormatDecimal128(t *testing.T) {
	for i, test := range dtoa128Tests {
		s := FormatDecimal128(test.d, test.fmt, test.prec)
		if s != test.s {
			t.Errorf("#%d: FormatDecimal128(%v, %c, %d) = %q, want %q",
				i, test.d, test.fmt, test.prec, s, test.s)
		}
	}
}

func TestAppendDecimal128(t *testing.T) {
	for i, test := range dtoa128Tests {
		b := AppendDecimal128([]byte("abc"), test.d, test.fmt, test.prec)
		if string(b) != "abc"+test.s {
			t.Errorf("#%d: AppendDecimal128(\"abc\", %v, %c, %d) = %q, want %q",
				i, test.d, test.fmt, test.prec, string(b), "abc"+test.s)
		}
	}
}

// --- ParseDecimal64 tests ---

type atodTest struct {
	in  string
	out string
	err error
}

var atod64Tests = []atodTest{
	// Invalid
	{"", "0", ErrSyntax},
	{"1x", "0", ErrSyntax},
	{"1.1.", "0", ErrSyntax},
	{"..", "0", ErrSyntax},
	{"1e", "0", ErrSyntax},
	{"1e-", "0", ErrSyntax},
	{".e1", "0", ErrSyntax},
	{"e1", "0", ErrSyntax},
	{"1ee1", "0", ErrSyntax},
	{"+", "0", ErrSyntax},
	{"-", "0", ErrSyntax},

	// Basic
	{"0", "0", nil},
	{"-0", "-0", nil},
	{"1", "1", nil},
	{"+1", "1", nil},
	{"-1", "-1", nil},
	{"0.1", "0.1", nil},
	{"-0.1", "-0.1", nil},
	{"123456700", "123456700", nil},
	{".5", "0.5", nil},
	{"5.", "5", nil},

	// Scientific notation
	{"1e23", "1e+23", nil},
	{"1E23", "1e+23", nil},
	{"1e-100", "1e-100", nil},
	{"625e-3", "0.625", nil},
	{"1e0", "1", nil},
	{"1e+0", "1", nil},
	{"1e-0", "1", nil},
	{"1.5e2", "150", nil},

	// Zeros
	{"0e0", "0", nil},
	{"-0e0", "-0", nil},
	{"+0e0", "0", nil},
	{"0e-0", "0", nil},
	{"00", "0", nil},
	{"0.0", "0", nil},
	{"0.00", "0", nil},
	{"000.000", "0", nil},

	// Special values
	{"nan", "NaN", nil},
	{"NaN", "NaN", nil},
	{"inf", "+Inf", nil},
	{"Inf", "+Inf", nil},
	{"-Inf", "-Inf", nil},
	{"+Inf", "+Inf", nil},
	{"-inf", "-Inf", nil},
	{"+inf", "+Inf", nil},
	{"Infinity", "+Inf", nil},
	{"infinity", "+Inf", nil},
	{"-Infinity", "-Inf", nil},
	// Max coefficient (16 digits)
	{"9999999999999999", "9999999999999999", nil},

	// 17-digit input: truncated to 16 significant digits
	{"12345678901234567", "12345678901234560", nil},
}

func TestParseDecimal64(t *testing.T) {
	for i, test := range atod64Tests {
		d, err := ParseDecimal64(test.in)
		if test.err != nil {
			if err == nil {
				t.Errorf("#%d: ParseDecimal64(%q) succeeded, want error %v", i, test.in, test.err)
				continue
			}
			// Check underlying error type
			ne, ok := err.(*NumError)
			if !ok {
				t.Errorf("#%d: ParseDecimal64(%q) error type = %T, want *NumError", i, test.in, err)
				continue
			}
			if ne.Err != test.err {
				t.Errorf("#%d: ParseDecimal64(%q) error = %v, want %v", i, test.in, ne.Err, test.err)
			}
			continue
		}
		if err != nil {
			t.Errorf("#%d: ParseDecimal64(%q) error = %v", i, test.in, err)
			continue
		}
		out := FormatDecimal64(d, 'g', -1)
		if out != test.out {
			t.Errorf("#%d: ParseDecimal64(%q) = %q, want %q", i, test.in, out, test.out)
		}
	}
}

// --- ParseDecimal128 tests ---

var atod128Tests = []atodTest{
	// Invalid
	{"", "0", ErrSyntax},
	{"1x", "0", ErrSyntax},
	{"1.1.", "0", ErrSyntax},
	{"1e", "0", ErrSyntax},
	{"+", "0", ErrSyntax},

	// Basic
	{"0", "0", nil},
	{"-0", "-0", nil},
	{"1", "1", nil},
	{"-1", "-1", nil},
	{"0.1", "0.1", nil},
	{"3.14", "3.14", nil},

	// Scientific notation
	{"1e23", "1e+23", nil},
	{"625e-3", "0.625", nil},

	// Special values
	{"NaN", "NaN", nil},
	{"nan", "NaN", nil},
	{"Inf", "+Inf", nil},
	{"-Inf", "-Inf", nil},
	{"Infinity", "+Inf", nil},
	{"-infinity", "-Inf", nil},

	// 34-digit precision (decimal128 max)
	{"1234567890123456789012345678901234", "1234567890123456789012345678901234", nil},
	{"9999999999999999999999999999999999", "9999999999999999999999999999999999", nil},

	// 35-digit input: truncated to 34 digits
	{"12345678901234567890123456789012345", "12345678901234567890123456789012340", nil},

	// Zeros
	{"0", "0", nil},
	{"00", "0", nil},
	{"0.0", "0", nil},
}

func TestParseDecimal128(t *testing.T) {
	for i, test := range atod128Tests {
		d, err := ParseDecimal128(test.in)
		if test.err != nil {
			if err == nil {
				t.Errorf("#%d: ParseDecimal128(%q) succeeded, want error %v", i, test.in, test.err)
				continue
			}
			ne, ok := err.(*NumError)
			if !ok {
				t.Errorf("#%d: ParseDecimal128(%q) error type = %T, want *NumError", i, test.in, err)
				continue
			}
			if ne.Err != test.err {
				t.Errorf("#%d: ParseDecimal128(%q) error = %v, want %v", i, test.in, ne.Err, test.err)
			}
			continue
		}
		if err != nil {
			t.Errorf("#%d: ParseDecimal128(%q) error = %v", i, test.in, err)
			continue
		}
		out := FormatDecimal128(d, 'g', -1)
		if out != test.out {
			t.Errorf("#%d: ParseDecimal128(%q) = %q, want %q", i, test.in, out, test.out)
		}
	}
}

// --- Roundtrip tests ---

func TestDecimal64Roundtrip(t *testing.T) {
	values := []decimal64{
		decimal64(0), decimal64(1), decimal64(-1),
		decimal64(3.14), decimal64(0.001), decimal64(0.1),
		decimal64(100), decimal64(9999999),
		math.Decimal64NaN(),
		math.Decimal64Inf(1), math.Decimal64Inf(-1),
	}
	for _, d := range values {
		s := FormatDecimal64(d, 'g', -1)
		d2, err := ParseDecimal64(s)
		if err != nil {
			t.Errorf("ParseDecimal64(FormatDecimal64(%v)) error: %v", d, err)
			continue
		}
		// NaN != NaN, so check specially
		if d != d {
			if d2 == d2 {
				t.Errorf("roundtrip NaN: got %v (not NaN)", d2)
			}
			continue
		}
		if d2 != d {
			t.Errorf("roundtrip %v: FormatDecimal64 = %q, ParseDecimal64 back = %v", d, s, d2)
		}
	}
}

func TestDecimal128Roundtrip(t *testing.T) {
	values := []string{
		"0", "1", "-1", "3.14", "0.001", "0.1",
		"1234567890123456789012345678901234",
		"NaN", "+Inf", "-Inf",
	}
	for _, s := range values {
		d, err := ParseDecimal128(s)
		if err != nil {
			t.Errorf("ParseDecimal128(%q) error: %v", s, err)
			continue
		}
		s2 := FormatDecimal128(d, 'g', -1)
		d2, err := ParseDecimal128(s2)
		if err != nil {
			t.Errorf("ParseDecimal128(%q) error: %v", s2, err)
			continue
		}
		if d != d {
			if d2 == d2 {
				t.Errorf("roundtrip NaN: got %v (not NaN)", d2)
			}
			continue
		}
		if d2 != d {
			t.Errorf("roundtrip %q → %q → %v != %v", s, s2, d2, d)
		}
	}
}

// --- Quantum-preserving format tests ---

func TestFormatDecimal64Quantum(t *testing.T) {
	tests := []struct {
		in  string
		fmt byte
		out string
	}{
		// prec=-2 preserves trailing zeros from coefficient
		{"1.200", 'g', "1.200"},
		{"1.200", 'e', "1.200e+00"},
		{"1.200", 'f', "1.200"},
		{"100", 'g', "100"},
		{"100", 'e', "1.00e+02"},
		{"100", 'f', "100"},
		{"1.0", 'g', "1.0"},
		{"1.0", 'e', "1.0e+00"},
		{"1.0", 'f', "1.0"},
		{"42", 'g', "42"},
		{"42", 'e', "4.2e+01"},
		{"42", 'f', "42"},
	}
	for _, test := range tests {
		d, err := ParseDecimal64(test.in)
		if err != nil {
			t.Fatalf("ParseDecimal64(%q): %v", test.in, err)
		}
		got := FormatDecimal64(d, test.fmt, -2) // -2 = quantum-preserving sentinel
		if got != test.out {
			t.Errorf("FormatDecimal64(Parse(%q), %c, -2) = %q, want %q",
				test.in, test.fmt, got, test.out)
		}
	}
}

// --- Benchmarks ---

func BenchmarkFormatDecimal64(b *testing.B) {
	benchmarks := []struct {
		name string
		d    decimal64
		fmt  byte
		prec int
	}{
		{"Integer", decimal64(33909), 'g', -1},
		{"Decimal", decimal64(339.7784), 'g', -1},
		{"Exp", decimal64(-5.09e75), 'g', -1},
		{"NegExp", decimal64(-5.11e-95), 'g', -1},
		{"Fixed3", decimal64(3.14159), 'f', 3},
	}
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				FormatDecimal64(bm.d, bm.fmt, bm.prec)
			}
		})
	}
}

func BenchmarkParseDecimal64(b *testing.B) {
	benchmarks := []struct {
		name string
		s    string
	}{
		{"Integer", "33909"},
		{"Decimal", "339.7784"},
		{"Exp", "-5.09e75"},
		{"NegExp", "-5.11e-95"},
	}
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				ParseDecimal64(bm.s)
			}
		})
	}
}
