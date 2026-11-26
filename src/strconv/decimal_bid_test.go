// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package strconv_test

import (
	"internal/bid"
	"math/big"
	"math/decimal"
	"math/rand"
	"slices"
	. "strconv"
	"strings"
	"testing"
)

// --- FormatDecimal / AppendDecimal tests ---

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
	{decimal64(1234567.8), 'g', -1, "1.2345678e+06"},

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
	{decimal64(12.345), 'f', 2, "12.34"}, // 4 is even, round down
	{decimal64(12.355), 'f', 2, "12.36"}, // 5 is odd, round up
	{decimal64(0.015), 'f', 2, "0.02"},   // 1 is odd, round up
	{decimal64(0.025), 'f', 2, "0.02"},   // 2 is even, round down
	{decimal64(0.035), 'f', 2, "0.04"},   // 3 is odd, round up
	{decimal64(0.045), 'f', 2, "0.04"},   // 4 is even, round down

	// Special values
	{decimal.NaN[decimal64](), 'g', -1, "NaN"},
	{decimal.NaN[decimal64](), 'e', 6, "NaN"},
	{decimal.NaN[decimal64](), 'f', 6, "NaN"},
	{decimal.Inf[decimal64](1), 'g', -1, "+Inf"},
	{decimal.Inf[decimal64](1), 'e', 6, "+Inf"},
	{decimal.Inf[decimal64](1), 'f', 6, "+Inf"},
	{decimal.Inf[decimal64](-1), 'g', -1, "-Inf"},
	{decimal.Inf[decimal64](-1), 'e', 6, "-Inf"},
	{decimal.Inf[decimal64](-1), 'f', 6, "-Inf"},

	// Negative zero
	{decimal.BID64frombits(1 << 63), 'g', -1, "-0e-398"},
	{decimal.BID64frombits(1 << 63), 'e', 6, "-0.000000e+00"},
	{decimal.BID64frombits(1 << 63), 'f', 6, "-0.000000"},
	{decimal.BID64frombits(1 << 63), 'f', 1, "-0.0"},

	// Decimal-specific: exact representation (unlike float)
	{decimal64(0.1), 'f', 20, "0.10000000000000000000"},
	{decimal64(0.3), 'f', 20, "0.30000000000000000000"},

	// Rounding ties go to the even digit, and a carry moves the point.
	{decimal64(0.5), 'f', 0, "0"},
	{decimal64(1.5), 'f', 0, "2"},
	{decimal64(2.5), 'f', 0, "2"},
	{decimal64(0.51), 'f', 0, "1"},
	{decimal64(0.049), 'f', 1, "0.0"},
	{decimal64(0.05), 'f', 1, "0.0"},
	{decimal64(0.051), 'f', 1, "0.1"},
	{decimal64(0.15), 'f', 1, "0.2"},
	{decimal64(0.25), 'f', 1, "0.2"},
	{decimal64(-0.004), 'f', 2, "-0.00"},
	{decimal64(9.995), 'f', 2, "10.00"},
	{decimal64(9.994), 'f', 2, "9.99"},
	{decimal64(99.5), 'f', 0, "100"},
	{decimal64(1e-300), 'f', 2, "0.00"},
	{decimal64(1e300), 'f', 2, "1" + zeros(300) + ".00"},
	{decimal64(2.5), 'e', 0, "2e+00"},
	{decimal64(3.5), 'e', 0, "4e+00"},
	{decimal64(9.5), 'e', 0, "1e+01"},
	{decimal64(9.99), 'e', 1, "1.0e+01"},
	{decimal64(99.9), 'e', 0, "1e+02"},
	{decimal64(1234567.8), 'e', 2, "1.23e+06"},
	{decimal64(1234567.8), 'e', 20, "1.23456780000000000000e+06"},
	{decimal64(1235), 'e', 2, "1.24e+03"},
	{decimal64(1225), 'e', 2, "1.22e+03"},
	{decimal64(9.99), 'g', 2, "10"},
	{decimal64(99.9), 'g', 2, "1e+02"},
	{decimal64(0.000099999), 'g', 2, "0.0001"},
	{decimal64(123456), 'g', 3, "1.23e+05"},
	{decimal64(1235), 'g', 3, "1.24e+03"},
	{decimal64(1225), 'g', 3, "1.22e+03"},
	{decimal64(1e-5), 'g', 3, "1e-05"},
	{decimal64(-0.0001234), 'g', 2, "-0.00012"},
	{decimal.BID64frombits(1<<63 | 0x31c0000000000001), 'g', 3, "-1"}, // -1E+0 (exponent at the bias)
}

func zeros(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = '0'
	}
	return string(b)
}

func TestFormatDecimal64(t *testing.T) {
	for i, test := range dtoaTests {
		s := FormatDecimal(decimal128(test.d), test.fmt, test.prec, 64)
		if s != test.s {
			t.Errorf("#%d: FormatDecimal(%v, %c, %d) = %q, want %q",
				i, test.d, test.fmt, test.prec, s, test.s)
		}
	}
}

func TestAppendDecimal64(t *testing.T) {
	for i, test := range dtoaTests {
		b := AppendDecimal([]byte("abc"), decimal128(test.d), test.fmt, test.prec, 64)
		if string(b) != "abc"+test.s {
			t.Errorf("#%d: AppendDecimal(\"abc\", %v, %c, %d) = %q, want %q",
				i, test.d, test.fmt, test.prec, string(b), "abc"+test.s)
		}
	}
}

// --- FormatDecimal / AppendDecimal tests ---

func mustParseD128(s string) decimal128 {
	d, err := ParseDecimal(s, 128)
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
	// Add test case requiring ParseDecimal (34-digit precision)
	dtoa128Tests = append(dtoa128Tests, dtoa128Test{
		mustParseD128("1234567890123456789012345678901234"),
		'g', -1, "1.234567890123456789012345678901234e+33",
	})

	// Extreme exponents (BID128 can reach ~6176 digits in the exponent).
	dtoa128Tests = append(dtoa128Tests,
		dtoa128Test{mustParseD128("1e+6000"), 'e', 0, "1e+6000"},
		dtoa128Test{mustParseD128("1e-6000"), 'e', 0, "1e-6000"},
		dtoa128Test{mustParseD128("1e+6111"), 'e', 0, "1e+6111"},
		dtoa128Test{mustParseD128("1e-6176"), 'e', 0, "1e-6176"},
		dtoa128Test{mustParseD128("1e6144"), 'e', 0, "1e+6144"},
		dtoa128Test{mustParseD128("9.999999999999999999999999999999999e6144"), 'e', -1, "9.999999999999999999999999999999999e+6144"},
		dtoa128Test{mustParseD128("9.999999999999999999999999999999999e6144"), 'e', 0, "1e+6145"},
		dtoa128Test{mustParseD128("9.999999999999999999999999999999999e6144"), 'g', 33, "1e+6145"},
		dtoa128Test{mustParseD128("9.999999999999999999999999999999999e6144"), 'e', 32, "1.00000000000000000000000000000000e+6145"},
		dtoa128Test{mustParseD128("9999999999999999999999999999999999"), 'e', 3, "1.000e+34"},
		dtoa128Test{mustParseD128("9999999999999999999999999999999999"), 'f', 0, "9999999999999999999999999999999999"},
		dtoa128Test{mustParseD128("9999999999999999999999999999999999e-1"), 'f', 0, "1000000000000000000000000000000000"},
		dtoa128Test{mustParseD128("9999999999999999999999999999999995e-1"), 'f', 0, "1000000000000000000000000000000000"},
		dtoa128Test{mustParseD128("1234567890123456789012345678901235e-1"), 'f', 0, "123456789012345678901234567890124"}, // 3 is odd: up
		dtoa128Test{mustParseD128("1234567890123456789012345678901225e-1"), 'f', 0, "123456789012345678901234567890122"}, // 2 is even: down
		dtoa128Test{mustParseD128("1234567890123456789012345678901234"), 'e', 32, "1.23456789012345678901234567890123e+33"},
		dtoa128Test{mustParseD128("1234567890123456789012345678901235"), 'e', 32, "1.23456789012345678901234567890124e+33"},
		dtoa128Test{mustParseD128("1234567890123456789012345678901225"), 'e', 32, "1.23456789012345678901234567890122e+33"},
		dtoa128Test{mustParseD128("1e-6176"), 'f', 3, "0.000"},
		dtoa128Test{mustParseD128("-1e-6176"), 'f', 3, "-0.000"},
		dtoa128Test{mustParseD128("5e-4"), 'f', 3, "0.000"},
		dtoa128Test{mustParseD128("5.1e-4"), 'f', 3, "0.001"},
	)
}

func TestFormatDecimal128(t *testing.T) {
	for i, test := range dtoa128Tests {
		s := FormatDecimal(test.d, test.fmt, test.prec, 128)
		if s != test.s {
			t.Errorf("#%d: FormatDecimal(%v, %c, %d) = %q, want %q",
				i, test.d, test.fmt, test.prec, s, test.s)
		}
	}
}

func TestAppendDecimal128(t *testing.T) {
	for i, test := range dtoa128Tests {
		b := AppendDecimal([]byte("abc"), test.d, test.fmt, test.prec, 128)
		if string(b) != "abc"+test.s {
			t.Errorf("#%d: AppendDecimal(\"abc\", %v, %c, %d) = %q, want %q",
				i, test.d, test.fmt, test.prec, string(b), "abc"+test.s)
		}
	}
}

// --- ParseDecimal tests ---

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
	{"123456700", "1.23456700e+08", nil},
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
	{"1.5e2", "1.5e+02", nil},

	// Zeros
	{"0e0", "0", nil},
	{"-0e0", "-0", nil},
	{"+0e0", "0", nil},
	{"0e-0", "0", nil},
	{"00", "0", nil},
	{"0.0", "0.0", nil},
	{"0.00", "0.00", nil},
	{"000.000", "0.000", nil},

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
	{"9999999999999999", "9.999999999999999e+15", nil},

	// Digits beyond the 16th round to nearest even; they are not truncated
	// (audit L4).
	{"12345678901234567", "1.234567890123457e+16", nil},
	{"12345678901234569", "1.234567890123457e+16", nil}, // was "...560"
	{"12345678901234561", "1.234567890123456e+16", nil},
	// Ties at the 17th significant digit: to the even 16th digit.
	{"12345678901234565", "1.234567890123456e+16", nil}, // 6 is even
	{"12345678901234575", "1.234567890123458e+16", nil}, // 7 is odd
	{"-12345678901234565", "-1.234567890123456e+16", nil},
	// Above a tie, however far below the 17th digit the excess lies.
	{"12345678901234565000000000000000000001", "1.234567890123457e+37", nil},
	{"1234567890123456.50000000000000000000001", "1.234567890123457e+15", nil},
	// Below a tie.
	{"12345678901234564999999999999999999999", "1.234567890123456e+37", nil},
	// A carry through every digit.
	{"99999999999999995", "1.000000000000000e+17", nil},
	{"99999999999999994", "9.999999999999999e+16", nil},
	// Zeros that do not count as digits, and trailing zeros that do not matter.
	{"0.0000000000000000000000000001234567890123456789", "1.234567890123457e-28", nil},
	{"1234567890123456.00000000000000000000", "1.234567890123456e+15", nil},

	// Range errors: the result is +-Inf, as ParseFloat gives (audit L3).
	{"1e999", "+Inf", ErrRange},
	{"1e400", "+Inf", ErrRange},
	{"-1e400", "-Inf", ErrRange},
	{"1e385", "+Inf", ErrRange},
	{"10e384", "+Inf", ErrRange},
	{"1e99999999999999999999", "+Inf", ErrRange},
	{"-1e99999999999999999999", "-Inf", ErrRange},
	{"9999999999999999.5e369", "+Inf", ErrRange}, // rounds to 10^385
	// The largest and the smallest values are not out of range.
	{"9.999999999999999e384", "9.999999999999999e+384", nil},
	{"-9.999999999999999e384", "-9.999999999999999e+384", nil},
	{"9999999999999999e369", "9.999999999999999e+384", nil},
	{"1e-398", "1e-398", nil},
	// Like ParseFloat, no error for a value too small: it becomes zero.
	{"1e-399", "0e-398", nil},
	{"1e-400", "0e-398", nil}, // was "1e-398" (audit L3)
	{"1e-500", "0e-398", nil},
	{"-1e-500", "-0e-398", nil},
	{"1e-99999999999999999999", "0e-398", nil},
	{"0e99999999999999999999", "0e+369", nil},
	{"0.0000000000000000000000000000000000000000000000000000000000000e-500", "0e-398", nil},

	// Special words must match whole (audit L14).
	{"NaNxyz", "0", ErrSyntax},
	{"nan1", "0", ErrSyntax},
	{"Infoo", "0", ErrSyntax},
	{"Infinityx", "0", ErrSyntax},
	{"Infinit", "0", ErrSyntax},
	{"Inf ", "0", ErrSyntax},
	{" Inf", "0", ErrSyntax},
	{"--Inf", "0", ErrSyntax},
	{"+-1", "0", ErrSyntax},
	// A NaN may have a sign (IEEE 754-2019 §5.12.1); there is no signaling
	// NaN.
	{"-NaN", "NaN", nil},
	{"+nan", "NaN", nil},
	{"-nan", "NaN", nil},
	{"snan", "0", ErrSyntax},
	{"sNaN", "0", ErrSyntax},
	{"-sNaN", "0", ErrSyntax},
	{"nan", "NaN", nil},
	{"INF", "+Inf", nil},
	{"+INFINITY", "+Inf", nil},

	// Underscores, as ParseFloat takes them: between digits only.
	{"1_000", "1000", nil},
	{"1_000.50", "1000.50", nil},
	{"-1_0.5_0e1_0", "-1.050e+11", nil},
	{"0_1", "1", nil},
	{"_1", "0", ErrSyntax},
	{"1_", "0", ErrSyntax},
	{"1__0", "0", ErrSyntax},
	{"1_.5", "0", ErrSyntax},
	{"1._5", "0", ErrSyntax},
	{"1_e5", "0", ErrSyntax},
	{"1e_5", "0", ErrSyntax},

	// Hexadecimal, as ParseFloat reads it. The value is exact in binary; an
	// integer that fits has exponent 0 and any other exact value the
	// smallest coefficient, as a constant without a quantum converts.
	{"0x1p-2", "0.25", nil},
	{"0x1.8p1", "3", nil},
	{"0X1P+1", "2", nil},
	{"0x10p0", "16", nil},
	{"0xap0", "10", nil},
	{"-0x1p-1", "-0.5", nil},
	{"0x_1p0", "1", nil},
	{"0x1_0p0", "16", nil},
	{"0x.8p1", "1", nil},
	{"0x0p0", "0", nil},
	{"-0x0.0p99", "-0", nil},
	{"0x1p-1074", "4.940656458412465e-324", nil},
	{"0x1p53", "9.007199254740992e+15", nil},
	{"0x1p54", "1.801439850948198e+16", nil},
	{"0x1.fffffffffffffp1023", "1.797693134862316e+308", nil},
	{"0x1p+2000", "+Inf", ErrRange},
	{"-0x1p+2000", "-Inf", ErrRange},
	{"0x1p-2000", "0e-398", nil},
	{"0x10", "0", ErrSyntax},
	{"0x", "0", ErrSyntax},
	{"0xp1", "0", ErrSyntax},
	{"0x.p1", "0", ErrSyntax},
	{"0x1p", "0", ErrSyntax},
	{"0x1_p0", "0", ErrSyntax},
	{"0x1g1", "0", ErrSyntax},

	{" 1", "0", ErrSyntax},
	{"1 ", "0", ErrSyntax},
}

// checkParseDecimal checks the outcome of parsing test.in. An error must be
// a *NumError of the expected kind that names the function and the input;
// the value that comes with ErrRange is checked as well, as it is for
// ParseFloat. A syntax error returns zero.
func checkParseDecimal(t *testing.T, i int, fn string, test atodTest, format func(d any) string, d any, err error) {
	t.Helper()
	if test.err == nil {
		if err != nil {
			t.Errorf("#%d: %s(%q) error = %v", i, fn, test.in, err)
			return
		}
		if out := format(d); out != test.out {
			t.Errorf("#%d: %s(%q) = %q, want %q", i, fn, test.in, out, test.out)
		}
		return
	}
	if err == nil {
		t.Errorf("#%d: %s(%q) succeeded, want error %v", i, fn, test.in, test.err)
		return
	}
	ne, ok := err.(*NumError)
	if !ok {
		t.Errorf("#%d: %s(%q) error type = %T, want *NumError", i, fn, test.in, err)
		return
	}
	if ne.Func != fn || ne.Num != test.in || ne.Err != test.err {
		t.Errorf("#%d: %s(%q) error = %#v, want Func %q, Num %q, Err %v",
			i, fn, test.in, ne, fn, test.in, test.err)
	}
	if out := format(d); out != test.out {
		t.Errorf("#%d: %s(%q) returned %q with error %v, want %q", i, fn, test.in, out, test.err, test.out)
	}
}

func TestParseDecimal64(t *testing.T) {
	for i, test := range atod64Tests {
		d, err := parseDecimal64(test.in)
		checkParseDecimal(t, i, "ParseDecimal", test,
			func(d any) string { return FormatDecimal(decimal128(d.(decimal64)), 'g', -1, 64) }, d, err)
	}
}

// --- ParseDecimal tests ---

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
	{"1234567890123456789012345678901234", "1.234567890123456789012345678901234e+33", nil},
	{"9999999999999999999999999999999999", "9.999999999999999999999999999999999e+33", nil},

	// Digits beyond the 34th round to nearest even; they are not truncated.
	{"12345678901234567890123456789012345", "1.234567890123456789012345678901234e+34", nil}, // tie, 4 is even
	{"12345678901234567890123456789012335", "1.234567890123456789012345678901234e+34", nil}, // tie, 3 is odd
	{"12345678901234567890123456789012346", "1.234567890123456789012345678901235e+34", nil},
	{"12345678901234567890123456789012344", "1.234567890123456789012345678901234e+34", nil},
	{"-12345678901234567890123456789012335", "-1.234567890123456789012345678901234e+34", nil},
	// Above and below the tie at the 35th digit.
	{"123456789012345678901234567890123450000000000000000001", "1.234567890123456789012345678901235e+53", nil},
	{"123456789012345678901234567890123449999999999999999999", "1.234567890123456789012345678901234e+53", nil},
	{"1234567890123456789012345678901234.5", "1.234567890123456789012345678901234e+33", nil},
	{"1234567890123456789012345678901233.5", "1.234567890123456789012345678901234e+33", nil},
	{"99999999999999999999999999999999995", "1.000000000000000000000000000000000e+35", nil},

	// Range errors: the result is +-Inf, as ParseFloat gives.
	{"1e6145", "+Inf", ErrRange},
	{"1e7000", "+Inf", ErrRange},
	{"-1e7000", "-Inf", ErrRange},
	{"1e99999999999999999999", "+Inf", ErrRange},
	{"-1e99999999999999999999", "-Inf", ErrRange},
	{"9999999999999999999999999999999999.5e6111", "+Inf", ErrRange},
	// Representable values, including the one audit L3 reports as mis-decoded.
	{"1e6144", "1.000000000000000000000000000000000e+6144", nil},
	{"1e6143", "1.00000000000000000000000000000000e+6143", nil},
	{"9.999999999999999999999999999999999e6144", "9.999999999999999999999999999999999e+6144", nil},
	{"-9.999999999999999999999999999999999e6144", "-9.999999999999999999999999999999999e+6144", nil},
	{"1e-6176", "1e-6176", nil},
	// No error for a value too small: it becomes zero.
	{"1e-6177", "0e-6176", nil},
	{"1e-7000", "0e-6176", nil},
	{"-1e-7000", "-0e-6176", nil},
	{"1e-99999999999999999999", "0e-6176", nil},

	// Special words must match whole (audit L14).
	{"NaNxyz", "0", ErrSyntax},
	{"Infoo", "0", ErrSyntax},
	{"Infinityx", "0", ErrSyntax},
	{"-Inf", "-Inf", nil},
	{"-NaN", "NaN", nil},
	{"+nan", "NaN", nil},
	{"-nan", "NaN", nil},
	{"-sNaN", "0", ErrSyntax},
	{"1_000.000_001", "1000.000001", nil},
	{"0x1p-2", "0.25", nil},
	{"0x1p100", "1.267650600228229401496703205376e+30", nil},
	{"0x1p113", "1.038459371706965525706099265844019e+34", nil},
	{"0x1p-1074", "4.940656458412465441765687928682214e-324", nil},
	{"0x1p+30000", "+Inf", ErrRange},
	{"0x1p-30000", "0e-6176", nil},

	// Zeros
	{"0", "0", nil},
	{"00", "0", nil},
	{"0.0", "0.0", nil},
}

func TestParseDecimal128(t *testing.T) {
	for i, test := range atod128Tests {
		d, err := ParseDecimal(test.in, 128)
		checkParseDecimal(t, i, "ParseDecimal", test,
			func(d any) string { return FormatDecimal(d.(decimal128), 'g', -1, 128) }, d, err)
	}
}

// --- Roundtrip tests ---

func TestDecimal64Roundtrip(t *testing.T) {
	values := []decimal64{
		decimal64(0), decimal64(1), decimal64(-1),
		decimal64(3.14), decimal64(0.001), decimal64(0.1),
		decimal64(100), decimal64(9999999),
		decimal.NaN[decimal64](),
		decimal.Inf[decimal64](1), decimal.Inf[decimal64](-1),
	}
	for _, d := range values {
		s := FormatDecimal(decimal128(d), 'g', -1, 64)
		d2, err := parseDecimal64(s)
		if err != nil {
			t.Errorf("ParseDecimal(FormatDecimal(%v)) error: %v", d, err)
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
			t.Errorf("roundtrip %v: FormatDecimal = %q, ParseDecimal back = %v", d, s, d2)
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
		d, err := ParseDecimal(s, 128)
		if err != nil {
			t.Errorf("ParseDecimal(%q) error: %v", s, err)
			continue
		}
		s2 := FormatDecimal(d, 'g', -1, 128)
		d2, err := ParseDecimal(s2, 128)
		if err != nil {
			t.Errorf("ParseDecimal(%q) error: %v", s2, err)
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

// --- Rounding to a precision against an independent reference ---

// refRound rounds the integer c × 10^exp to a multiple of 10^t, to nearest
// even, using math/big. It returns the integer n with n × 10^t as the
// result; if t <= exp the value is already a multiple and n = c × 10^(exp-t).
func refRound(c *big.Int, exp, t int) *big.Int {
	if t <= exp {
		return new(big.Int).Mul(c, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp-t)), nil))
	}
	div := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(t-exp)), nil)
	q, r := new(big.Int).QuoRem(c, div, new(big.Int))
	r.Lsh(r, 1)
	switch cmp := r.Cmp(div); {
	case cmp > 0, cmp == 0 && q.Bit(0) == 1:
		q.Add(q, big.NewInt(1))
	}
	return q
}

// refFormat formats the non-zero value coeff × 10^exp with verb 'e' or 'f'
// and a precision of at least 0, without any of the code under test.
func refFormat(neg bool, coeff string, exp int, fmt byte, prec int) string {
	c, _ := new(big.Int).SetString(coeff, 10)
	n := len(coeff)
	var digs string
	var e10 int // the value is digs × 10^e10
	switch fmt {
	case 'e':
		e10 = exp + n - (prec + 1)
	case 'f':
		e10 = -prec
	}
	digs = refRound(c, exp, e10).String()
	if digs == "0" {
		digs = ""
	}
	out := ""
	if neg {
		out = "-"
	}
	if fmt == 'f' {
		for len(digs) < prec+1 {
			digs = "0" + digs
		}
		out += digs[:len(digs)-prec]
		if prec > 0 {
			out += "." + digs[len(digs)-prec:]
		}
		return out
	}
	if digs == "" { // rounded to zero cannot happen for 'e'
		panic("refFormat: zero")
	}
	for len(digs) > prec+1 { // a carry: 9.99 became 10.0
		digs = digs[:len(digs)-1]
		e10++
	}
	for len(digs) < prec+1 {
		digs += "0"
		e10--
	}
	x := e10 + prec
	out += digs[:1]
	if prec > 0 {
		out += "." + digs[1:]
	}
	out += "e"
	if x < 0 {
		out += "-"
		x = -x
	} else {
		out += "+"
	}
	if x < 10 {
		out += "0"
	}
	return out + Itoa(x)
}

func TestFormatDecimalRounding(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, width := range []struct {
		bits, digits, minExp, maxExp int
	}{
		{64, 16, -398, 369},
		{128, 34, -6176, 6111},
	} {
		for i := 0; i < 20000; i++ {
			// A coefficient of 1 to width.digits digits, mostly with digits
			// that make ties likely (zeros, fives and nines).
			n := 1 + rng.Intn(width.digits)
			b := make([]byte, n)
			for j := range b {
				b[j] = "0123456789055599"[rng.Intn(16)]
			}
			b[0] = "123456789"[rng.Intn(9)]
			coeff := string(b)
			exp := rng.Intn(width.maxExp-width.minExp+1) + width.minExp
			if rng.Intn(2) == 0 {
				exp = rng.Intn(60) - 40
			}
			neg := rng.Intn(2) == 0
			in := coeff + "e" + Itoa(exp)
			if neg {
				in = "-" + in
			}
			prec := rng.Intn(width.digits + 4)
			if rng.Intn(4) == 0 {
				prec = rng.Intn(8)
			}
			for _, f := range []byte{'e', 'f'} {
				var got string
				if width.bits == 64 {
					d, err := parseDecimal64(in)
					if err != nil {
						t.Fatalf("ParseDecimal(%q): %v", in, err)
					}
					got = FormatDecimal(decimal128(d), f, prec, 64)
				} else {
					d, err := ParseDecimal(in, 128)
					if err != nil {
						t.Fatalf("ParseDecimal(%q): %v", in, err)
					}
					got = FormatDecimal(d, f, prec, 128)
				}
				c, _ := new(big.Int).SetString(coeff, 10)
				want := ""
				if f == 'f' && refRound(c, exp, -prec).Sign() == 0 {
					// Rounds to zero: the sign stays and so do the places.
					want = "0"
					if neg {
						want = "-0"
					}
					if prec > 0 {
						want += "." + zeros(prec)
					}
				} else {
					want = refFormat(neg, coeff, exp, f, prec)
				}
				if got != want {
					t.Fatalf("decimal%d %s as %%.%d%c = %q, want %q", width.bits, in, prec, f, got, want)
				}
			}
		}
	}
}

// A carry can discard a trailing zero after rounding at any precision. Exercise
// every retained length, including overlapping moves of a full-width coefficient.
func TestFormatDecimalCarryDigits(t *testing.T) {
	for _, width := range []struct{ bits, digits int }{{64, 16}, {128, 34}} {
		for n := 2; n <= width.digits; n++ {
			coeff := strings.Repeat("9", n)
			for _, exp := range []int{-2 * n, 0, 20} {
				for _, neg := range []bool{false, true} {
					input := coeff + "e" + Itoa(exp)
					if neg {
						input = "-" + input
					}
					x, err := ParseDecimal(input, width.bits)
					if err != nil {
						t.Fatal(err)
					}
					for prec := 0; prec < n-1; prec++ {
						want := refFormat(neg, coeff, exp, 'e', prec)
						if got := FormatDecimal(x, 'e', prec, width.bits); got != want {
							t.Fatalf("decimal%d %s, precision %d: got %q, want %q", width.bits, input, prec, got, want)
						}
					}
				}
			}
		}
	}
}

// Exercise every coefficient length and canonical notation boundary while
// appending to empty and prefixed buffers, with growth and spare capacity.
// Expected digits and placement come from the independent formatter, not
// another invocation of AppendDecimal with a different buffer size.
func TestAppendDecimalCanonicalCapacity(t *testing.T) {
	for _, width := range []struct{ bits, digits, minExp, maxExp int }{
		{64, bid.Precision64, bid.MinExp64, bid.MaxExp64},
		{128, bid.Precision128, bid.MinExp128, bid.MaxExp128},
	} {
		for n := 1; n <= width.digits; n++ {
			coeff := strings.Repeat("1234567890", 4)[:n]
			c, _ := new(big.Int).SetString(coeff, 10)
			exponents := []int{width.minExp, -n - 6, -n - 5, -n, 1 - n, -1, 0, 1, 21 - n, 22 - n, width.maxExp}
			// Every interior point position exercises both digit-copy spans,
			// including word boundaries and overlapping head/tail stores.
			for exp := 1 - n; exp < 0; exp++ {
				if !slices.Contains(exponents, exp) {
					exponents = append(exponents, exp)
				}
			}
			for _, exp := range exponents {
				for _, neg := range []bool{false, true} {
					var x decimal128
					if width.bits == 64 {
						bits, _ := bid.Pack64(neg, c.Uint64(), exp)
						x = decimal128(decimal.BID64frombits(bits))
					} else {
						hi := new(big.Int).Rsh(new(big.Int).Set(c), 64).Uint64()
						bits, _ := bid.Pack128(neg, bid.Uint128{Hi: hi, Lo: c.Uint64()}, exp)
						x = decimal.BID128frombits(bits.Hi, bits.Lo)
					}
					for _, format := range []byte{'e', 'E', 'g', 'G', 'f'} {
						e := exp + n - 1
						scientific := format == 'e' || format == 'E' || format != 'f' && (e < -4 || e >= 6 || exp > 0)
						want := ""
						if scientific {
							want = refFormat(neg, coeff, exp, 'e', n-1)
							if format == 'E' || format == 'G' {
								want = strings.ReplaceAll(want, "e", "E")
							}
						} else {
							want = refFormat(neg, coeff, exp, 'f', max(-exp, 0))
						}
						for _, prefix := range []string{"", "prefix:"} {
							for _, spare := range []int{0, bid.MaxLen128 - 1, bid.MaxLen128, bid.MaxLen128 + 1, 2 * bid.MaxLen128} {
								// The byte outside the exposed capacity is a bounds sentinel.
								buf := []byte(strings.Repeat("~", len(prefix)+spare+1))
								copy(buf, prefix)
								dst := buf[: len(prefix) : len(prefix)+spare]
								got := AppendDecimal(dst, x, format, -1, width.bits)
								if string(got) != prefix+want || buf[len(buf)-1] != '~' {
									t.Fatalf("decimal%d %s e%d neg=%v format=%c spare=%d prefix=%q: got %q, want %q; sentinel=%c", width.bits, coeff, exp, neg, format, spare, prefix, got, prefix+want, buf[len(buf)-1])
								}
							}
						}
					}
				}
			}
		}
	}
}

// TestAppendDecimalShort drives the short values, a coefficient below 10^8
// at an exponent from -4 to 0, which the kernel lays out in registers and
// stores straight into the buffer: every coefficient length, with the
// point at each place inside the digits and to their left, zeros of each
// quantum, both signs and the values just outside the range. Each is
// appended with no spare capacity, with less than the short path needs,
// with exactly that and with more, to an empty and a prefixed buffer, in
// every notation and also at a precision, which the general path rounds
// and lays out; the expected text comes from the independent formatter,
// and every byte after the result must be untouched, because a buffer's
// spare capacity may hold the caller's data.
func TestAppendDecimalShort(t *testing.T) {
	coeffs := []string{"0", "1", "5", "9", "10", "12", "99", "100", "123", "999", "1000", "1234", "9999",
		"12345", "99999", "100000", "123456", "999999", "1234567", "9999999", "10000000", "12345678", "99999999",
		"100000000", "123456789"}
	for _, coeff := range coeffs {
		c, _ := new(big.Int).SetString(coeff, 10)
		for exp := -8; exp <= 2; exp++ {
			for _, neg := range []bool{false, true} {
				bits, _ := bid.Pack128(neg, bid.Uint128{Lo: c.Uint64()}, exp)
				x := decimal.BID128frombits(bits.Hi, bits.Lo)
				n := len(coeff)
				for _, format := range []byte{'e', 'E', 'g', 'G', 'f'} {
					for _, prec := range []int{-1, 2} {
						want := ""
						if prec < 0 {
							e := exp + n - 1
							scientific := format == 'e' || format == 'E' || format != 'f' && (e < -4 || e >= 6 || exp > 0)
							switch {
							case scientific && coeff == "0":
								// The reference has no exponent form of
								// zero, and neither has the short path:
								// the general path's result stands.
								want = string(AppendDecimal(nil, x, format, prec, 128))
							case scientific:
								want = refFormat(neg, coeff, exp, 'e', n-1)
								if format == 'E' || format == 'G' {
									want = strings.ReplaceAll(want, "e", "E")
								}
							default:
								want = refFormat(neg, coeff, exp, 'f', max(-exp, 0))
							}
						} else {
							// A precision rounds; the general path's own
							// result with no spare capacity is the reference.
							want = string(AppendDecimal(nil, x, format, prec, 128))
						}
						// The second prefix holds a point, zeros and an
						// exponent mark of its own, which are not the
						// number's: a layout that edits what it appended
						// must not read them.
						for _, prefix := range []string{"", "v1.0e5:"} {
							for _, spare := range []int{0, len(want), bid.ShortLen128 - 1, bid.ShortLen128, bid.ShortLen128 + 1, 64} {
								buf := []byte(strings.Repeat("~", len(prefix)+spare+1))
								copy(buf, prefix)
								got := AppendDecimal(buf[:len(prefix):len(prefix)+spare], x, format, prec, 128)
								if string(got) != prefix+want {
									t.Fatalf("%s e%d neg=%v format=%c prec=%d spare=%d prefix=%q: got %q, want %q", coeff, exp, neg, format, prec, spare, prefix, got, prefix+want)
								}
								if &got[0] != &buf[0] {
									continue // the result did not fit and was reallocated
								}
								for i := len(got); i < len(buf); i++ {
									if buf[i] != '~' {
										t.Fatalf("%s e%d neg=%v format=%c prec=%d spare=%d prefix=%q: byte %d after the %d-byte result %q was written", coeff, exp, neg, format, prec, spare, prefix, i, len(got), got)
									}
								}
							}
						}
					}
				}
			}
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
		d, err := parseDecimal64(test.in)
		if err != nil {
			t.Fatalf("ParseDecimal(%q): %v", test.in, err)
		}
		got := FormatDecimal(decimal128(d), test.fmt, -2, 64) // -2 = quantum-preserving sentinel
		if got != test.out {
			t.Errorf("FormatDecimal(Parse(%q), %c, -2) = %q, want %q",
				test.in, test.fmt, got, test.out)
		}
	}
}

// --- Fuzzing ---

func FuzzDecimal64RoundTrip(f *testing.F) {
	for _, b := range []uint64{
		0, 1, 1 << 63, 0x31c0000000000001, 0x7c00000000000000,
		0x7800000000000000, 0xf800000000000000, 0x6bd86f70fd5a8e0f,
		0x77fb86f26fc0ffff, 0x0000000000000001, 0x2e3ba18ac06d0e5d,
	} {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b uint64) {
		if !bid.IsCanonical64(b) {
			t.Skip("not an encoding Format produces from")
		}
		d := decimal.BID64frombits(b)
		for _, prec := range []int{-1, -2} {
			s := FormatDecimal(decimal128(d), 'e', prec, 64)
			d2, err := parseDecimal64(s)
			if err != nil {
				t.Fatalf("ParseDecimal(FormatDecimal(%#x, 'e', %d) = %q): %v", b, prec, s, err)
			}
			if decimal.IsNaN[decimal64](d) {
				if !decimal.IsNaN[decimal64](d2) {
					t.Fatalf("%#x: NaN came back as %q (%#x)", b, s, decimal.BID64bits(d2))
				}
				continue
			}
			if !bid.Equal64(decimal.BID64bits(d2), b) {
				t.Fatalf("%#x: %q parses to %#x", b, s, decimal.BID64bits(d2))
			}
			// -2 keeps the quantum, so formatting again gives the same text.
			if prec == -2 {
				if s2 := FormatDecimal(decimal128(d2), 'e', prec, 64); s2 != s {
					t.Fatalf("%#x: %q parses and formats as %q", b, s, s2)
				}
			}
		}
	})
}

func FuzzDecimal128RoundTrip(f *testing.F) {
	for _, b := range [][2]uint64{
		{0, 0}, {1, 0}, {0, 1}, {0x3040000000000000, 1}, {0x7c00000000000000, 0},
		{0x7800000000000000, 0}, {0xf800000000000000, 0}, {0x5fffed09bead87c0, 0x378d8e63ffffffff},
		{0x0000000000000000, 1}, {0x2ffe314dc6448d93, 0x38c15b09ffffffff},
	} {
		f.Add(b[0], b[1])
	}
	f.Fuzz(func(t *testing.T, hi, lo uint64) {
		x := bid.Uint128{Hi: hi, Lo: lo}
		if !bid.IsCanonical128(x) {
			t.Skip("not an encoding Format produces from")
		}
		d := decimal.BID128frombits(hi, lo)
		for _, prec := range []int{-1, -2} {
			s := FormatDecimal(d, 'e', prec, 128)
			d2, err := ParseDecimal(s, 128)
			if err != nil {
				t.Fatalf("ParseDecimal(FormatDecimal(%#x:%#x, 'e', %d) = %q): %v", hi, lo, prec, s, err)
			}
			if decimal.IsNaN[decimal128](d) {
				if !decimal.IsNaN[decimal128](d2) {
					t.Fatalf("%#x:%#x: NaN came back as %q", hi, lo, s)
				}
				continue
			}
			hi2, lo2 := decimal.BID128bits(d2)
			if !bid.Equal128(bid.Uint128{Hi: hi2, Lo: lo2}, x) {
				t.Fatalf("%#x:%#x: %q parses to %#x:%#x", hi, lo, s, hi2, lo2)
			}
			if prec == -2 {
				if s2 := FormatDecimal(d2, 'e', prec, 128); s2 != s {
					t.Fatalf("%#x:%#x: %q parses and formats as %q", hi, lo, s, s2)
				}
			}
		}
	})
}

// --- Benchmarks ---

var formatDecimalSink string

func TestFormatDecimalAllocs(t *testing.T) {
	for _, x := range []decimal128{123.45, 1.50, 1e100, -1e-6000, 1234567890123456789012345678901234} {
		if n := testing.AllocsPerRun(100, func() {
			formatDecimalSink = FormatDecimal(x, 'g', -1, 128)
		}); n != 1 {
			t.Errorf("FormatDecimal(%v) allocated %v times; want one result string", x, n)
		}
	}
}

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
		{"HexShortest", decimal64(339.7784), 'x', -1},
		{"Hex6", decimal64(339.7784), 'x', 6},
		{"HexExp", decimal64(-5.09e75), 'x', -1},
		{"HexNegExp", decimal64(-5.11e-95), 'x', -1},
		{"HexExact", decimal64(1.5), 'x', -1},
	}
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				FormatDecimal(decimal128(bm.d), bm.fmt, bm.prec, 64)
			}
		})
	}
}

func BenchmarkFormatDecimal128Hex(b *testing.B) {
	benchmarks := []struct {
		name string
		d    decimal128
		prec int
	}{
		{"Shortest", 339.7784, -1},
		{"Prec13", 339.7784, 13},
		{"Full", 1.234567890123456789012345678901234, -1},
		{"Smallest", 1e-6176, -1},
		{"Largest", 9.999999999999999999999999999999999e6144, -1},
	}
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				FormatDecimal(bm.d, 'x', bm.prec, 128)
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
				parseDecimal64(bm.s)
			}
		})
	}
}

// The canonical string keeps every digit of the coefficient and reads back as
// the same value and quantum. The expected strings are written by hand.
func TestFormatDecimalCanonical(t *testing.T) {
	tests := []struct {
		coeff   uint64 // decimal64 coefficient
		exp     int
		neg     bool
		g, e, f string
	}{
		{coeff: 150, exp: -2, g: "1.50", e: "1.50e+00", f: "1.50"},
		{coeff: 15, exp: -1, g: "1.5", e: "1.5e+00", f: "1.5"},
		{coeff: 150, exp: -2, neg: true, g: "-1.50", e: "-1.50e+00", f: "-1.50"},
		{coeff: 100, exp: 0, g: "100", e: "1.00e+02", f: "100"},
		{coeff: 1, exp: 3, g: "1e+03", e: "1e+03", f: "1000"},
		{coeff: 15, exp: 2, g: "1.5e+03", e: "1.5e+03", f: "1500"},
		{coeff: 0, exp: 0, g: "0", e: "0e+00", f: "0"},
		{coeff: 0, exp: 0, neg: true, g: "-0", e: "-0e+00", f: "-0"},
		{coeff: 0, exp: -2, g: "0.00", e: "0e-02", f: "0.00"},
		{coeff: 0, exp: 3, g: "0e+03", e: "0e+03", f: "0"},
		{coeff: 0, exp: -4, g: "0.0000", e: "0e-04", f: "0.0000"},
		{coeff: 0, exp: -5, g: "0e-05", e: "0e-05", f: "0.00000"},
		{coeff: 0, exp: -6, g: "0e-06", e: "0e-06", f: "0.000000"},
		{coeff: 0, exp: -7, g: "0e-07", e: "0e-07", f: "0.0000000"},
		{coeff: 1, exp: -4, g: "0.0001", e: "1e-04", f: "0.0001"},
		{coeff: 1, exp: -5, g: "1e-05", e: "1e-05", f: "0.00001"},
		{coeff: 12, exp: -5, g: "0.00012", e: "1.2e-04", f: "0.00012"},
		{coeff: 1, exp: -6, g: "1e-06", e: "1e-06", f: "0.000001"},
		{coeff: 1, exp: -7, g: "1e-07", e: "1e-07", f: "0.0000001"},
		{coeff: 10, exp: -8, g: "1.0e-07", e: "1.0e-07", f: "0.00000010"},
		{coeff: 1000000000000000, exp: 5, g: "1.000000000000000e+20", e: "1.000000000000000e+20", f: "100000000000000000000"},
		{coeff: 1, exp: 21, g: "1e+21", e: "1e+21", f: "1000000000000000000000"},
		{coeff: 9999999999999999, exp: 0, g: "9.999999999999999e+15", e: "9.999999999999999e+15", f: "9999999999999999"},
		// The upper threshold, a float64's: the leading digit at 10^5 and 10^6.
		{coeff: 999999, exp: 0, g: "999999", e: "9.99999e+05", f: "999999"},
		{coeff: 1000000, exp: 0, g: "1.000000e+06", e: "1.000000e+06", f: "1000000"},
		{coeff: 999999, exp: 0, neg: true, g: "-999999", e: "-9.99999e+05", f: "-999999"},
		{coeff: 1000000, exp: 0, neg: true, g: "-1.000000e+06", e: "-1.000000e+06", f: "-1000000"},
		{coeff: 99999999, exp: -2, g: "999999.99", e: "9.9999999e+05", f: "999999.99"},
		{coeff: 123456789, exp: -2, g: "1.23456789e+06", e: "1.23456789e+06", f: "1234567.89"},
		{coeff: 1, exp: 5, g: "1e+05", e: "1e+05", f: "100000"},
		{coeff: 1, exp: -4, neg: true, g: "-0.0001", e: "-1e-04", f: "-0.0001"},
		{coeff: 1, exp: -5, neg: true, g: "-1e-05", e: "-1e-05", f: "-0.00001"},
	}
	for _, tt := range tests {
		// bid.Pack64 is the kernel's packer; the digits here don't come from it.
		bits, _ := bid.Pack64(tt.neg, tt.coeff, tt.exp)
		d := decimal.BID64frombits(bits)
		for _, c := range []struct {
			fmt  byte
			want string
		}{{'g', tt.g}, {'e', tt.e}, {'f', tt.f}} {
			if got := FormatDecimal(decimal128(d), c.fmt, -1, 64); got != c.want {
				t.Errorf("FormatDecimal(%d×10^%d, %c, -1) = %q, want %q", tt.coeff, tt.exp, c.fmt, got, c.want)
			}
			if got := FormatDecimal(decimal128(d), c.fmt, -1, 128); got != c.want {
				t.Errorf("FormatDecimal(%d×10^%d, %c, -1) = %q, want %q", tt.coeff, tt.exp, c.fmt, got, c.want)
			}
		}
		back, err := parseDecimal64(tt.g)
		if err != nil {
			t.Errorf("ParseDecimal(%q): %v", tt.g, err)
		}
		if decimal.BID64bits(back) != bits {
			t.Errorf("ParseDecimal(%q) = %#x, want %#x", tt.g, decimal.BID64bits(back), bits)
		}
	}
}

// TestFormatDecimalPrecisionLikeFloat checks 'g' and 'G' at an explicit
// precision against FormatFloat on values a float64 and a decimal128 both
// hold exactly, c × 10^e / 2^k: the same digits, rounded the same way, and
// the same choice of notation, exponential when the exponent of the leading
// digit is below -4 or at least the precision.
func TestFormatDecimalPrecisionLikeFloat(t *testing.T) {
	limit := new(big.Int).Lsh(big.NewInt(1), 53) // a float64's integers
	n := 0
	for _, c := range []int64{1, 3, 7, 11, 25, 99, 123, 1001, 12345, 999999} {
		for k := 0; k <= 20; k++ {
			for e := 0; e <= 22; e++ {
				// The float c × 5^e × 2^(e-k) is exact when c × 5^e
				// fits its 53 bits; the decimal is c × 5^k × 10^(e-k).
				m := new(big.Int).Mul(big.NewInt(c), new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(e)), nil))
				if m.Cmp(limit) >= 0 {
					continue
				}
				coeff := new(big.Int).Mul(big.NewInt(c), new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(k)), nil))
				for _, neg := range []string{"", "-"} {
					s := neg + coeff.String() + "e" + Itoa(e-k)
					f, err := ParseFloat(s, 64)
					if err != nil {
						t.Fatal(err)
					}
					d, err := ParseDecimal(s, 128)
					if err != nil {
						t.Fatal(err)
					}
					for _, format := range []byte{'g', 'G'} {
						for prec := 0; prec <= 25; prec++ {
							want := FormatFloat(f, format, prec, 64)
							if got := FormatDecimal(d, format, prec, 128); got != want {
								t.Errorf("FormatDecimal(%s, %c, %d) = %q, FormatFloat %q", s, format, prec, got, want)
							}
							n++
						}
					}
				}
			}
		}
	}
	if n == 0 {
		t.Fatal("no values checked")
	}
}

func TestFormatDecimalCanonicalNotation(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"100000000000000000000", "1.00000000000000000000e+20"},
		{"-100000000000000000000", "-1.00000000000000000000e+20"},
		{"1000000000000000000000", "1.000000000000000000000e+21"},
		{"1.234567890123456789012", "1.234567890123456789012"},
		{"-123456789012345678901.2345678901234", "-1.234567890123456789012345678901234e+20"},
		// The upper threshold, a float64's, in each of the kernel's paths:
		// the short values, the long ones and the general one.
		{"999999", "999999"},
		{"1000000", "1.000000e+06"},
		{"-999999", "-999999"},
		{"-1000000", "-1.000000e+06"},
		{"999999.9", "999999.9"},
		{"9999999.9", "9.9999999e+06"},
		{"12345678", "1.2345678e+07"},
		{"999999.99", "999999.99"},
		{"1234567.89", "1.23456789e+06"},
		{"-1234567.89", "-1.23456789e+06"},
		{"123456.789012345678", "123456.789012345678"},
		{"1234567.89012345678", "1.23456789012345678e+06"},
		{"-123456.7890123456789012345678901234", "-123456.7890123456789012345678901234"},
		{"-1234567.890123456789012345678901234", "-1.234567890123456789012345678901234e+06"},
		{"1e5", "1e+05"},
		{"-0.0001", "-0.0001"},
		{"-0.00001", "-1e-05"},
		{"1.234567890123456789012345678901234", "1.234567890123456789012345678901234"},
		{"0.0001", "0.0001"},
		{"0.00001", "1e-05"},
		{"0.000012", "1.2e-05"},
		{"0.00012", "0.00012"},
		{"0.000001", "1e-06"},
		{"0.0000001", "1e-07"},
		{"0.00", "0.00"},
		{"-0.00", "-0.00"},
		{"0e3", "0e+03"},
		{"1e3", "1e+03"},
		{"1.50", "1.50"},
		{"NaN", "NaN"},
		{"Infinity", "+Inf"},
		{"-Infinity", "-Inf"},
	} {
		x, err := ParseDecimal(tt.in, 128)
		if err != nil {
			t.Fatal(err)
		}
		for _, format := range []byte{'g', 'G'} {
			want := tt.want
			if format == 'G' {
				want = strings.ReplaceAll(want, "e", "E")
			}
			if got := FormatDecimal(x, format, -1, 128); got != want {
				t.Errorf("FormatDecimal(%s, %c) = %q, want %q", tt.in, format, got, want)
			}
			// Exercise every capacity boundary, including growth, exact fit,
			// and spare capacity. The prefix must survive all of them.
			const prefix = "prefix:"
			for spare := 0; spare <= len(want)+1; spare++ {
				dst := make([]byte, len(prefix), len(prefix)+spare)
				copy(dst, prefix)
				if got := string(AppendDecimal(dst, x, format, -1, 128)); got != prefix+want {
					t.Errorf("AppendDecimal(%s, %c, spare=%d) = %q, want %q", tt.in, format, spare, got, prefix+want)
				}
			}
		}
	}
}

func parseDecimal64(s string) (decimal64, error) {
	d, err := ParseDecimal(s, 64)
	return decimal64(d), err
}

// TestFormatDecimalMatchesKernel checks that the canonical string, which
// FormatDecimal writes with 'g' at precision -1 and fmt's %v writes through
// it, is the text println writes, which comes from the kernel's Append64
// and Append128: every coefficient length at every exponent around the
// thresholds of plain notation, zeros, both signs, the specials, the
// extremes of the exponent range and random encodings of both widths.
func TestFormatDecimalMatchesKernel(t *testing.T) {
	check128 := func(x bid.Uint128) {
		t.Helper()
		d := decimal.BID128frombits(x.Hi, x.Lo)
		if got, want := FormatDecimal(d, 'g', -1, 128), string(bid.Append128(nil, x)); got != want {
			t.Fatalf("FormatDecimal(%#016x%016x, 'g', -1, 128) = %q, kernel %q", x.Hi, x.Lo, got, want)
		}
	}
	check64 := func(x uint64) {
		t.Helper()
		d := decimal128(decimal.BID64frombits(x))
		if got, want := FormatDecimal(d, 'g', -1, 64), string(bid.Append64(nil, x)); got != want {
			t.Fatalf("FormatDecimal(%#016x, 'g', -1, 64) = %q, kernel %q", x, got, want)
		}
	}
	n := 0
	for digits := 0; digits <= bid.Precision128; digits++ {
		c := bid.Uint128{}
		for i := 0; i < digits; i++ {
			// 1, 12, 123, ... with the digits cycling, so that no
			// coefficient has trailing zeros to hide a misplaced point.
			hi, lo := bits128mul10(c)
			c = bid.Uint128{Hi: hi, Lo: lo + uint64(i%9+1)}
		}
		for exp := -digits - 8; exp <= 24; exp++ {
			for _, neg := range []bool{false, true} {
				x, ok := bid.Pack128(neg, c, exp)
				if !ok {
					continue
				}
				check128(x)
				n++
				if digits <= bid.Precision64 {
					if y, ok := bid.Pack64(neg, c.Lo, exp); ok {
						check64(y)
						n++
					}
				}
			}
		}
	}
	for _, x := range []bid.Uint128{
		{Hi: bid.Inf128Hi}, {Hi: bid.Inf128Hi | bid.SignMask128Hi}, {Hi: bid.NaN128Hi}, {Hi: bid.SNaN128Hi},
	} {
		check128(x)
	}
	for _, x := range []uint64{bid.Inf64, bid.Inf64 | bid.SignMask64, bid.NaN64, bid.SNaN64} {
		check64(x)
	}
	r := rand.New(rand.NewSource(1))
	for range 200000 {
		check128(bid.Uint128{Hi: r.Uint64(), Lo: r.Uint64()})
		check64(r.Uint64())
	}
	t.Logf("%d values and 400,008 random encodings", n)
}

// bits128mul10 returns c × 10 as two words, for coefficients that fit.
func bits128mul10(c bid.Uint128) (hi, lo uint64) {
	hi = c.Hi*10 + mulhi64(c.Lo, 10)
	return hi, c.Lo * 10
}

func mulhi64(a, b uint64) uint64 {
	const mask = 1<<32 - 1
	a0, a1 := a&mask, a>>32
	b0, b1 := b&mask, b>>32
	w0 := a0 * b0
	t := a1*b0 + w0>>32
	w1, w2 := t&mask, t>>32
	w1 += a0 * b1
	return a1*b1 + w2 + w1>>32
}

// TestParseDecimalGrammarLikeFloat checks that ParseDecimal accepts exactly
// the strings ParseFloat accepts, and "nan" with a sign, which IEEE 754-2019
// §5.12.1 requires and ParseFloat rejects.
func TestParseDecimalGrammarLikeFloat(t *testing.T) {
	strs := []string{
		"", "+", "-", ".", "+.", "e5", "1", "+1", "-1", ".5", "5.", "1e5", "1E+5", "1e-5", "1e", "1e+",
		"1.2.3", "1..2", "--1", "+-1", " 1", "1 ", "1x",
		"1_000", "_1", "1_", "1__0", "1_.5", "1._5", "1_e5", "1e_5", "1e1_0", "0_1", "1_0.5_0", ".5_0", "+_1", "-1_0",
		"0x1p-2", "0x1.8p1", "0X1P+1", "0x1", "0x", "0x.p1", "0x.8p1", "0x_1p0", "0x1_0p0", "0x1p1_0", "0x1_p0",
		"0x1.Fp+2", "0xp1", "0x1p", "0x1p+", "0x1g1", "-0x1p0", "+0x1p0", "0x1.p0", "0x.1p0", "0x1e1p0", "0x1e1",
		"0b101", "0o17", "0x1.8", "1.2p4", "1p4",
		"nan", "NaN", "NAN", "+nan", "-nan", "snan", "sNaN", "+snan", "-SNAN", "--snan", "snan1", "nan1", "nana",
		"inf", "Inf", "+inf", "-inf", "infinity", "-Infinity", "+INFINITY", "infin", "infinityx", "-in", "i",
	}
	for _, s := range strs {
		_, ferr := ParseFloat(s, 64)
		for _, bits := range []int{64, 128} {
			_, derr := ParseDecimal(s, bits)
			fsyn := ferr != nil && ferr.(*NumError).Err == ErrSyntax
			if w := strings.TrimLeft(s, "+-"); len(s)-len(w) == 1 && strings.EqualFold(w, "nan") {
				fsyn = false
			}
			dsyn := derr != nil && derr.(*NumError).Err == ErrSyntax
			if fsyn != dsyn {
				t.Errorf("ParseDecimal(%q, %d) error %v; ParseFloat gives %v", s, bits, derr, ferr)
			}
		}
	}
}

// TestParseDecimalHex checks the conversion of hexadecimal text against an
// exact model built on math/big: one rounding to nearest even from the exact
// binary value, and the quantum a constant without one takes. Among the
// values are exact ties of the decimal format, which are dyadic when the
// tie lies above the units digit, and their binary neighbours.
func TestParseDecimalHex(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	n := 0
	for _, f := range []struct {
		bits, prec, etiny, emax int
	}{{64, 16, -398, 369}, {128, 34, -6176, 6111}} {
		check := func(s string) {
			t.Helper()
			want, ok := hexDecimalModel(s, f.prec, f.etiny, f.emax)
			if !ok {
				return
			}
			n++
			got, err := ParseDecimal(s, f.bits)
			if err != nil {
				t.Errorf("ParseDecimal(%q, %d): %v", s, f.bits, err)
				return
			}
			w, err := ParseDecimal(want, f.bits)
			if err != nil {
				t.Fatalf("model numeral %q: %v", want, err)
			}
			if FormatDecimal(got, 'e', -1, f.bits) != FormatDecimal(w, 'e', -1, f.bits) {
				t.Errorf("ParseDecimal(%q, %d) = %s, want %s", s, f.bits, FormatDecimal(got, 'e', -1, f.bits), FormatDecimal(w, 'e', -1, f.bits))
			}
		}
		for range 3000 {
			nd := 1 + r.Intn(40)
			var b strings.Builder
			if r.Intn(2) == 0 {
				b.WriteByte('-')
			}
			b.WriteString("0x")
			point := r.Intn(nd + 1)
			for i := range nd {
				if i == point && r.Intn(2) == 0 {
					b.WriteByte('.')
				}
				b.WriteByte("0123456789abcdef"[r.Intn(16)])
			}
			b.WriteString("p")
			b.WriteString(Itoa(r.Intn(2*4*f.emax) - 4*f.emax - 160))
			check(b.String())
		}
		// Ties (2c+1) × 10^q / 2 with q >= 1, and the values one unit of a
		// long binary mantissa either side.
		for range 1000 {
			c := new(big.Int).Rand(r, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(f.prec)), nil))
			c.Mul(c, big.NewInt(2))
			c.Add(c, big.NewInt(1))
			q := 1 + r.Intn(30)
			tie := new(big.Int).Mul(c, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(q)), nil))
			// tie / 2 is an integer since 10^q is even.
			tie.Rsh(tie, 1)
			check("0x" + tie.Text(16) + "p0")
			shifted := new(big.Int).Lsh(tie, 64)
			check("0x" + new(big.Int).Add(shifted, big.NewInt(1)).Text(16) + "p-64")
			check("0x" + new(big.Int).Sub(shifted, big.NewInt(1)).Text(16) + "p-64")
		}
	}
	if n < 6000 {
		t.Fatalf("only %d values checked", n)
	}
}

// hexDecimalModel returns, for the hexadecimal literal s, a decimal numeral
// that denotes the expected result exactly, with its quantum, and ok = false
// for a value beyond the largest finite one.
func hexDecimalModel(s string, prec, etiny, emax int) (string, bool) {
	v, ok := new(big.Rat).SetString(s)
	if !ok {
		panic("bad hex " + s)
	}
	sign := ""
	if v.Sign() < 0 || strings.HasPrefix(s, "-") {
		sign = "-"
	}
	v.Abs(v)
	if v.Sign() == 0 {
		return sign + "0", true
	}
	pow := func(k int) *big.Rat {
		p := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(max(k, -k))), nil)
		if k >= 0 {
			return new(big.Rat).SetInt(p)
		}
		return new(big.Rat).SetFrac(big.NewInt(1), p)
	}
	// lead: 10^lead <= v < 10^(lead+1).
	lead := int(float64(v.Num().BitLen()-v.Denom().BitLen()) * 0.30103)
	for v.Cmp(pow(lead)) < 0 {
		lead--
	}
	for v.Cmp(pow(lead+1)) >= 0 {
		lead++
	}
	q := max(lead-prec+1, etiny)
	scaled := new(big.Rat).Mul(v, pow(-q))
	c, rem := new(big.Int).QuoRem(scaled.Num(), scaled.Denom(), new(big.Int))
	exact := rem.Sign() == 0
	if !exact {
		// To nearest, ties to even.
		twice := new(big.Int).Lsh(rem, 1)
		switch twice.Cmp(scaled.Denom()) {
		case 1:
			c.Add(c, big.NewInt(1))
		case 0:
			if c.Bit(0) == 1 {
				c.Add(c, big.NewInt(1))
			}
		}
		if len(c.String()) > prec {
			c.Quo(c, big.NewInt(10))
			q++
		}
		if q > emax {
			return "", false
		}
		return sign + c.String() + "e" + Itoa(q), true
	}
	if q > emax {
		return "", false
	}
	digits := c.String()
	for len(digits) > 1 && digits[len(digits)-1] == '0' {
		digits = digits[:len(digits)-1]
		q++
	}
	if q > 0 && len(digits)+q <= prec {
		digits += strings.Repeat("0", q)
		q = 0
	}
	return sign + digits + "e" + Itoa(q), true
}

// TestFormatDecimalFormatsLikeFloat checks the format bytes and bit sizes
// against FormatFloat's handling of them: 'b' writes the coefficient and its
// decimal exponent, 'x' and 'X' a hexadecimal fraction and a binary
// exponent, an unknown byte gives "%" and the byte for a finite value, the
// specials are written whatever the format, and bitSize 64 rounds to
// decimal64 first.
func TestFormatDecimalFormatsLikeFloat(t *testing.T) {
	d := func(s string) decimal128 {
		x, err := ParseDecimal(s, 128)
		if err != nil {
			t.Fatal(err)
		}
		return x
	}
	for _, tt := range []struct {
		in   string
		fmt  byte
		prec int
		want string
	}{
		{"1.50", 'b', -1, "150e-2"},
		{"1.50", 'b', 3, "150e-2"},
		{"-123456e-78", 'b', -1, "-123456e-78"},
		{"1E+3", 'b', -1, "1e+3"},
		{"0", 'b', -1, "0e+0"},
		{"-0.00", 'b', -1, "-0e-2"},
		{"NaN", 'b', -1, "NaN"},
		{"-Inf", 'b', -1, "-Inf"},
		{"1.50", 'x', -1, "0x1.8p+00"},
		{"1.50", 'X', 4, "0X1.8000P+00"},
		{"-1.50", 'z', -1, "%z"},
		{"0", 'v', -1, "%v"},
		{"NaN", 'z', -1, "NaN"},
		{"+Inf", 'x', -1, "+Inf"},
		{"-Inf", '?', 2, "-Inf"},
	} {
		for _, bits := range []int{64, 128} {
			if got := FormatDecimal(d(tt.in), tt.fmt, tt.prec, bits); got != tt.want {
				t.Errorf("FormatDecimal(%s, %q, %d, %d) = %q, want %q", tt.in, tt.fmt, tt.prec, bits, got, tt.want)
			}
		}
	}
	for _, x := range []float64{1.5, -2.25, 0} {
		for _, f := range []byte{'z', 'v', '?'} {
			want := FormatFloat(x, f, -1, 64)
			got := FormatDecimal(decimal128(x), f, -1, 128)
			if got != want {
				t.Errorf("FormatDecimal(%v, %q) = %q; FormatFloat gives %q", x, f, got, want)
			}
		}
	}

	// bitSize 64 narrows, as bitSize 32 does for FormatFloat.
	x := d("1.234567890123456789012345678901234")
	if got, want := FormatDecimal(x, 'g', -1, 64), "1.234567890123457"; got != want {
		t.Errorf("FormatDecimal(34 digits, 'g', -1, 64) = %q, want %q", got, want)
	}
	if got, want := string(AppendDecimal([]byte("x="), x, 'e', -1, 64)), "x=1.234567890123457e+00"; got != want {
		t.Errorf("AppendDecimal(34 digits, 'e', -1, 64) = %q, want %q", got, want)
	}
	if got, want := FormatDecimal(d("1e400"), 'g', -1, 64), "+Inf"; got != want {
		t.Errorf("FormatDecimal(1e400, 'g', -1, 64) = %q, want %q", got, want)
	}

	for _, bits := range []int{0, 16, 32, 63, 65, 127, 129} {
		for name, f := range map[string]func(){
			"FormatDecimal": func() { FormatDecimal(1, 'g', -1, bits) },
			"AppendDecimal": func() { AppendDecimal(nil, 1, 'g', -1, bits) },
		} {
			func() {
				defer func() {
					want := "strconv: illegal " + name + " bitSize"
					if r := recover(); r != want {
						t.Errorf("%s with bitSize %d: recovered %v, want panic %q", name, bits, r, want)
					}
				}()
				f()
			}()
		}
	}
}

// TestFormatDecimalHexLikeFloat checks 'x' and 'X' against FormatFloat on
// decimals that are exactly float64 values, m × 2^k with a short m, at
// precision -1 and at the explicit precisions a float64 has.
func TestFormatDecimalHexLikeFloat(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	n := 0
	for _, w := range []struct{ bits, mbits, kmax int }{{64, 10, 12}, {128, 20, 30}} {
		for range 2000 {
			m := 1 + r.Int63n(1<<w.mbits-1)
			k := r.Intn(2*w.kmax+1) - w.kmax
			f := float64(m) * mathPow2(k)
			if r.Intn(2) == 0 {
				f = -f
			}
			// The exact decimal of f: m × 5^-k × 10^k for negative k.
			s := new(big.Rat).SetFloat64(f).FloatString(max(-k, 0))
			d, err := ParseDecimal(s, w.bits)
			if err != nil {
				t.Fatal(err)
			}
			if back, _ := ParseFloat(FormatDecimal(d, 'g', -1, w.bits), 64); back != f {
				t.Fatalf("%s is not exact in decimal%d", s, w.bits)
			}
			for prec := -1; prec <= 13; prec++ {
				for _, format := range []byte{'x', 'X'} {
					want := FormatFloat(f, format, prec, 64)
					if got := FormatDecimal(d, format, prec, w.bits); got != want {
						t.Errorf("FormatDecimal(%s, %q, %d, %d) = %s; FormatFloat gives %s", s, format, prec, w.bits, got, want)
					}
					n++
				}
			}
		}
	}
	for _, s := range []string{"0", "-0", "NaN", "Inf", "-Inf"} {
		d, _ := ParseDecimal(s, 128)
		f, _ := ParseFloat(s, 64)
		if s == "-0" {
			f = mathCopysignNeg(f)
		}
		for _, prec := range []int{-1, 0, 3} {
			if got, want := FormatDecimal(d, 'x', prec, 128), FormatFloat(f, 'x', prec, 64); got != want {
				t.Errorf("FormatDecimal(%s, 'x', %d) = %s; FormatFloat gives %s", s, prec, got, want)
			}
		}
	}
	if n < 50000 {
		t.Fatalf("only %d comparisons", n)
	}
}

func mathPow2(k int) float64 {
	f := 1.0
	for ; k > 0; k-- {
		f *= 2
	}
	for ; k < 0; k++ {
		f /= 2
	}
	return f
}

func mathCopysignNeg(f float64) float64 { return -(f - f) - 0 }

// TestFormatDecimalHexOracle checks 'x' against a math/big model over the
// whole exponent range of both widths: the value correctly rounded, ties to
// even, at 0 to 40 hexadecimal digits, and at precision -1 the fewest digits
// that ParseDecimal reads back as the value.
func TestFormatDecimalHexOracle(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	count := 150
	if testing.Short() {
		count = 20
	}
	for _, w := range []struct{ bits, prec, minExp, maxExp int }{{64, 16, -398, 369}, {128, 34, -6176, 6111}} {
		for i := range count {
			nd := 1 + r.Intn(w.prec)
			digits := []byte{byte('1' + r.Intn(9))}
			for range nd - 1 {
				digits = append(digits, byte('0'+r.Intn(10)))
			}
			q := r.Intn(w.maxExp-w.minExp+1) + w.minExp
			switch i % 10 { // the ends of the range, which need the most work
			case 0:
				q = w.minExp
			case 1:
				q = w.maxExp
			}
			neg := r.Intn(2) == 0
			text := string(digits) + "e" + Itoa(q)
			if neg {
				text = "-" + text
			}
			d, err := ParseDecimal(text, w.bits)
			if err != nil {
				t.Fatalf("%s: %v", text, err)
			}
			v, _ := new(big.Rat).SetString(text)
			for prec := 0; prec <= 40; prec++ {
				want := hexModel(v, prec)
				if got := FormatDecimal(d, 'x', prec, w.bits); got != want {
					t.Errorf("FormatDecimal(%s, 'x', %d, %d) = %s, want %s", text, prec, w.bits, got, want)
				}
			}
			short := FormatDecimal(d, 'x', -1, w.bits)
			back, err := ParseDecimal(short, w.bits)
			if err != nil || back != d {
				t.Errorf("ParseDecimal(FormatDecimal(%s, 'x', -1, %d) = %s) = %v, %v; want the value back", text, w.bits, short, back, err)
			}
			// No fewer digits read back as the value.
			if n := hexDigitsAfterPoint(short); n > 0 {
				shorter := hexModel(v, n-1)
				if b, err := ParseDecimal(shorter, w.bits); err == nil && b == d {
					t.Errorf("FormatDecimal(%s, 'x', -1, %d) = %s, but %s reads back too", text, w.bits, short, shorter)
				}
			}
		}
	}
}

// hexModel writes the nonzero rational v in the 'x' form at prec digits,
// rounded to nearest, ties to even, with math/big.
func hexModel(v *big.Rat, prec int) string {
	sign := ""
	if v.Sign() < 0 {
		sign = "-"
		v = new(big.Rat).Neg(v)
	}
	pow2 := func(k int) *big.Rat {
		p := new(big.Int).Lsh(big.NewInt(1), uint(max(k, -k)))
		if k >= 0 {
			return new(big.Rat).SetInt(p)
		}
		return new(big.Rat).SetFrac(big.NewInt(1), p)
	}
	e := v.Num().BitLen() - v.Denom().BitLen()
	for v.Cmp(pow2(e)) < 0 {
		e--
	}
	for v.Cmp(pow2(e+1)) >= 0 {
		e++
	}
	scaled := new(big.Rat).Mul(v, pow2(4*prec-e))
	m, rem := new(big.Int).QuoRem(scaled.Num(), scaled.Denom(), new(big.Int))
	switch new(big.Int).Lsh(rem, 1).Cmp(scaled.Denom()) {
	case 1:
		m.Add(m, big.NewInt(1))
	case 0:
		if m.Bit(0) == 1 {
			m.Add(m, big.NewInt(1))
		}
	}
	if m.BitLen() > 4*prec+1 {
		m.Rsh(m, 1)
		e++
	}
	frac := new(big.Int).Sub(m, new(big.Int).Lsh(big.NewInt(1), uint(4*prec)))
	s := sign + "0x1"
	if prec > 0 {
		h := frac.Text(16)
		s += "." + strings.Repeat("0", prec-len(h)) + h
	}
	es := Itoa(max(e, -e))
	if len(es) < 2 {
		es = "0" + es
	}
	if e < 0 {
		return s + "p-" + es
	}
	return s + "p+" + es
}

func hexDigitsAfterPoint(s string) int {
	dot := strings.IndexByte(s, '.')
	p := strings.IndexByte(s, 'p')
	if dot < 0 {
		return 0
	}
	return p - dot - 1
}

// TestFormatDecimalHexPaths drives the three routes of 'x': the product
// decides most values, integer arithmetic the ones that are exactly binary
// fractions (on which the product, unable to see the exactness, would give
// up), and the exact path the ones the others cannot (a rounding boundary
// within the product's error, and exponents outside its table). Ties of explicit precisions (dyadic values exactly
// halfway between two n-digit results), their decimal neighbours, values
// whose interval of read-back values ends exactly at a candidate (the
// coefficient's parity then decides), and exponents outside the table are
// checked against math/big, and both paths must have run.
func TestFormatDecimalHexPaths(t *testing.T) {
	var paths [3]int
	defer SetHexPathHook(func(path int) { paths[path]++ })()
	r := rand.New(rand.NewSource(4))
	check := func(text string, bits, prec int) {
		t.Helper()
		d, err := ParseDecimal(text, bits)
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		v, _ := new(big.Rat).SetString(FormatDecimal(d, 'e', -1, bits))
		if prec >= 0 {
			if got, want := FormatDecimal(d, 'x', prec, bits), hexModel(v, prec); got != want {
				t.Errorf("FormatDecimal(%s, 'x', %d, %d) = %s, want %s", text, prec, bits, got, want)
			}
			return
		}
		short := FormatDecimal(d, 'x', -1, bits)
		if back, err := ParseDecimal(short, bits); err != nil || back != d {
			t.Errorf("FormatDecimal(%s, 'x', -1, %d) = %s reads back as %v, %v", text, bits, short, back, err)
		}
		if n := hexDigitsAfterPoint(short); n > 0 {
			if b, err := ParseDecimal(hexModel(v, n-1), bits); err == nil && b == d {
				t.Errorf("FormatDecimal(%s, 'x', -1, %d) = %s is not the shortest", text, bits, short)
			}
		}
	}
	exact := func(v *big.Rat, digits int) (string, bool) {
		s := v.FloatString(digits)
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
		return s, len(strings.ReplaceAll(strings.TrimLeft(strings.ReplaceAll(s, ".", ""), "-0"), ".", "")) > 0
	}
	for _, w := range []struct{ bits, maxN, prec, q5 int }{{64, 3, 16, 15}, {128, 6, 34, 33}} {
		for range 400 {
			// A tie at n digits: 2^j × (1 + (2k+1)/2^(4n+1)).
			n := r.Intn(w.maxN + 1)
			k := r.Int63n(1 << (4 * n))
			j := r.Intn(41) - 20
			num := new(big.Int).Lsh(big.NewInt(1), uint(4*n+1))
			num.Add(num, big.NewInt(2*k+1))
			v := new(big.Rat).SetFrac(num, new(big.Int).Lsh(big.NewInt(1), uint(4*n+1)))
			if j >= 0 {
				v.Mul(v, new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), uint(j))))
			} else {
				v.Quo(v, new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), uint(-j))))
			}
			s, _ := exact(v, 4*n+1+20)
			if d, err := ParseDecimal(s, w.bits); err != nil || FormatDecimal(d, 'e', -1, w.bits) != FormatDecimal(mustParse(t, s, 128), 'e', -1, 128) {
				continue // more digits than the width holds
			}
			for _, prec := range []int{n, -1} {
				check(s, w.bits, prec)
			}
			// The decimal neighbours of the tie, a unit of the last place
			// away, which the product decides.
			d := mustParse(t, s, w.bits)
			for _, nb := range []decimal128{decimal.Nextafter(d, decimal.Inf[decimal128](1)), decimal.Nextafter(d, decimal.Inf[decimal128](-1))} {
				if w.bits == 64 {
					nb = decimal128(decimal.Nextafter(decimal64(d), decimal.Inf[decimal64](int(1-2*boolInt(nb < d)))))
				}
				check(FormatDecimal(nb, 'e', -1, w.bits), w.bits, n)
			}
		}
		// A bound at a candidate: 2C+1 or 2C-1 a multiple of 5^q5, so that
		// (2C±1) × 10^-q5 / 2 is dyadic.
		p5 := new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(w.q5)), nil)
		lo := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(w.prec-1)), nil)
		for range 200 {
			tt := new(big.Int).Rand(r, new(big.Int).Div(new(big.Int).Mul(lo, big.NewInt(18)), p5))
			tt.SetBit(tt, 0, 1) // odd, so that 2C±1 is
			tc := new(big.Int).Mul(tt, p5)
			for _, sign := range []int64{-1, 1} {
				c := new(big.Int).Sub(tc, big.NewInt(sign)) // 2C = t×5^q5 ∓ 1
				c.Rsh(c, 1)
				if c.Cmp(lo) < 0 || len(c.String()) > w.prec {
					continue
				}
				check(c.String()+"e-"+Itoa(w.q5), w.bits, -1)
			}
		}
		// Exponents beyond the table, from either end of the range.
		for range 30 {
			q := -340 - r.Intn(40)
			if r.Intn(2) == 0 {
				q = 340 + r.Intn(29)
			}
			text := Itoa(1+r.Intn(999999)) + "e" + Itoa(q)
			check(text, w.bits, -1)
			check(text, w.bits, r.Intn(30))
		}
	}
	product, binary, slow := paths[HexPathProduct], paths[HexPathBinary], paths[HexPathExact]
	if product < 500 || binary < 500 || slow < 100 {
		t.Errorf("product %d times, exact binary value %d, exact path %d; want each to have run often", product, binary, slow)
	}
	t.Logf("product %d, exact binary value %d, exact path %d", product, binary, slow)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func mustParse(t *testing.T, s string, bits int) decimal128 {
	t.Helper()
	d, err := ParseDecimal(s, bits)
	if err != nil {
		t.Fatalf("%s: %v", s, err)
	}
	return d
}

// TestFormatDecimalHexLongPrecision checks 'x' at precision 1,000,000 for a
// decimal128 at the least exponent, whose expansion does not terminate,
// against math/big.
func TestFormatDecimalHexLongPrecision(t *testing.T) {
	const prec = 1_000_000
	c, _ := new(big.Int).SetString("1234567890123456789012345678901234", 10)
	d := mustParse(t, c.String()+"e-6176", 128)
	got := FormatDecimal(d, 'x', prec, 128)

	// v = c / 10^6176 is below 1; e is the exponent of its leading bit,
	// 2^e <= v < 2^(e+1), that is den <= c × 2^-e < 2 den. The digits are
	// floor(v × 2^(4 prec - e)) rounded to nearest, ties to even.
	den := new(big.Int).Exp(big.NewInt(10), big.NewInt(6176), nil)
	one := big.NewInt(1)
	e := c.BitLen() - den.BitLen()
	for new(big.Int).Lsh(c, uint(-e)).Cmp(den) < 0 {
		e--
	}
	for new(big.Int).Lsh(c, uint(-e-1)).Cmp(den) >= 0 {
		e++
	}
	num := new(big.Int).Lsh(c, uint(4*prec-e))
	m, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	switch new(big.Int).Lsh(rem, 1).Cmp(den) {
	case 1:
		m.Add(m, one)
	case 0:
		if m.Bit(0) == 1 {
			m.Add(m, one)
		}
	}
	if m.BitLen() > 4*prec+1 {
		m.Rsh(m, 1)
		e++
	}
	frac := new(big.Int).Sub(m, new(big.Int).Lsh(one, uint(4*prec))).Text(16)
	want := "0x1." + strings.Repeat("0", prec-len(frac)) + frac + "p-" + Itoa(-e)
	if got != want {
		i := 0
		for i < len(got) && i < len(want) && got[i] == want[i] {
			i++
		}
		t.Fatalf("FormatDecimal(%se-6176, 'x', %d) differs from math/big at byte %d of %d", c, prec, i, len(want))
	}
}

// BenchmarkFormatDecimalHex measures 'x' on typical values at precision -1
// and small precisions, at both widths, beside FormatFloat's 'x' on the
// float64 of the same value as a yardstick, and at a large precision.
func BenchmarkFormatDecimalHex(b *testing.B) {
	values := []struct {
		name string
		s    string
	}{
		{"Money", "19.99"},
		{"Amount", "1234567.89"},
		{"Integer", "33909"},
		{"Half", "1.5"},
		{"Small", "1e-10"},
		{"Large", "3.14159e10"},
	}
	for _, v := range values {
		f, _ := ParseFloat(v.s, 64)
		d64, _ := ParseDecimal(v.s, 64)
		d128, _ := ParseDecimal(v.s, 128)
		for _, prec := range []int{-1, 4, 13} {
			name := v.name + "/prec=" + Itoa(prec)
			b.Run(name+"/float64", func(b *testing.B) {
				for range b.N {
					FormatFloat(f, 'x', prec, 64)
				}
			})
			b.Run(name+"/decimal64", func(b *testing.B) {
				for range b.N {
					FormatDecimal(d64, 'x', prec, 64)
				}
			})
			b.Run(name+"/decimal128", func(b *testing.B) {
				for range b.N {
					FormatDecimal(d128, 'x', prec, 128)
				}
			})
		}
	}
	d, _ := ParseDecimal("1234567890123456789012345678901234e-6176", 128)
	f := 1.234567890123457e-300
	b.Run("LongPrecision/prec=1000000/float64", func(b *testing.B) {
		for range b.N {
			FormatFloat(f, 'x', 1_000_000, 64)
		}
	})
	b.Run("LongPrecision/prec=1000000/decimal128", func(b *testing.B) {
		for range b.N {
			FormatDecimal(d, 'x', 1_000_000, 128)
		}
	})
}

// TestParseDecimalSignedNaN checks that "nan" with or without a sign
// parses, at both sizes, to the same quiet NaN (IEEE 754-2019 §5.12.1; the
// standard does not interpret the sign of a NaN, §6.3), and that "snan" is
// not accepted.
func TestParseDecimalSignedNaN(t *testing.T) {
	quiet := decimal.NaN[decimal128]()
	wantHi, wantLo := decimal.Native128bits(quiet)
	for _, bits := range []int{64, 128} {
		for _, s := range []string{"nan", "NaN", "+nan", "-nan", "+NaN", "-NaN", "-NAN"} {
			got, err := ParseDecimal(s, bits)
			if err != nil {
				t.Errorf("ParseDecimal(%q, %d): %v", s, bits, err)
				continue
			}
			if hi, lo := decimal.Native128bits(got); hi != wantHi || lo != wantLo {
				t.Errorf("ParseDecimal(%q, %d) = %#x %#x, want %#x %#x", s, bits, hi, lo, wantHi, wantLo)
			}
		}
		for _, s := range []string{"--nan", "-nan1", "- nan", "snan", "-sNaN", "nans"} {
			if _, err := ParseDecimal(s, bits); err == nil || err.(*NumError).Err != ErrSyntax {
				t.Errorf("ParseDecimal(%q, %d) error = %v, want ErrSyntax", s, bits, err)
			}
		}
	}
}
