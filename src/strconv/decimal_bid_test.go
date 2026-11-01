// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package strconv_test

import (
	"internal/bid"
	"math"
	"math/big"
	"math/rand"
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
	{decimal64(12.345), 'f', 2, "12.34"}, // 4 is even, round down
	{decimal64(12.355), 'f', 2, "12.36"}, // 5 is odd, round up
	{decimal64(0.015), 'f', 2, "0.02"},   // 1 is odd, round up
	{decimal64(0.025), 'f', 2, "0.02"},   // 2 is even, round down
	{decimal64(0.035), 'f', 2, "0.04"},   // 3 is odd, round up
	{decimal64(0.045), 'f', 2, "0.04"},   // 4 is even, round down

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
	{math.Decimal64frombits(1 << 63), 'g', -1, "-0e-398"},
	{math.Decimal64frombits(1 << 63), 'e', 6, "-0.000000e+00"},
	{math.Decimal64frombits(1 << 63), 'f', 6, "-0.000000"},
	{math.Decimal64frombits(1 << 63), 'f', 1, "-0.0"},

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
	{math.Decimal64frombits(1<<63 | 0x31c0000000000001), 'g', 3, "-1"}, // -1E+0 (exponent at the bias)
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
	{"9999999999999999", "9999999999999999", nil},

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
	{"1234567890123456.50000000000000000000001", "1234567890123457", nil},
	// Below a tie.
	{"12345678901234564999999999999999999999", "1.234567890123456e+37", nil},
	// A carry through every digit.
	{"99999999999999995", "1.000000000000000e+17", nil},
	{"99999999999999994", "9.999999999999999e+16", nil},
	// Zeros that do not count as digits, and trailing zeros that do not matter.
	{"0.0000000000000000000000000001234567890123456789", "1.234567890123457e-28", nil},
	{"1234567890123456.00000000000000000000", "1234567890123456", nil},

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
	{"-NaN", "NaN", nil},
	{"snan", "NaN", nil},
	{"INF", "+Inf", nil},
	{"+INFINITY", "+Inf", nil},

	// Not decimal floating-point numerals.
	{"1_000", "0", ErrSyntax},
	{"0x10", "0", ErrSyntax},
	{"0x1p4", "0", ErrSyntax},
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
		d, err := ParseDecimal64(test.in)
		checkParseDecimal(t, i, "ParseDecimal64", test,
			func(d any) string { return FormatDecimal64(d.(decimal64), 'g', -1) }, d, err)
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

	// Zeros
	{"0", "0", nil},
	{"00", "0", nil},
	{"0.0", "0.0", nil},
}

func TestParseDecimal128(t *testing.T) {
	for i, test := range atod128Tests {
		d, err := ParseDecimal128(test.in)
		checkParseDecimal(t, i, "ParseDecimal128", test,
			func(d any) string { return FormatDecimal128(d.(decimal128), 'g', -1) }, d, err)
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
					d, err := ParseDecimal64(in)
					if err != nil {
						t.Fatalf("ParseDecimal64(%q): %v", in, err)
					}
					got = FormatDecimal64(d, f, prec)
				} else {
					d, err := ParseDecimal128(in)
					if err != nil {
						t.Fatalf("ParseDecimal128(%q): %v", in, err)
					}
					got = FormatDecimal128(d, f, prec)
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
		d := math.Decimal64frombits(b)
		for _, prec := range []int{-1, -2} {
			s := FormatDecimal64(d, 'e', prec)
			d2, err := ParseDecimal64(s)
			if err != nil {
				t.Fatalf("ParseDecimal64(FormatDecimal64(%#x, 'e', %d) = %q): %v", b, prec, s, err)
			}
			if math.IsDecimal64NaN(d) {
				if !math.IsDecimal64NaN(d2) {
					t.Fatalf("%#x: NaN came back as %q (%#x)", b, s, math.Decimal64bits(d2))
				}
				continue
			}
			if !bid.Equal64(math.Decimal64bits(d2), b) {
				t.Fatalf("%#x: %q parses to %#x", b, s, math.Decimal64bits(d2))
			}
			// -2 keeps the quantum, so formatting again gives the same text.
			if prec == -2 {
				if s2 := FormatDecimal64(d2, 'e', prec); s2 != s {
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
		d := math.Decimal128frombits(hi, lo)
		for _, prec := range []int{-1, -2} {
			s := FormatDecimal128(d, 'e', prec)
			d2, err := ParseDecimal128(s)
			if err != nil {
				t.Fatalf("ParseDecimal128(FormatDecimal128(%#x:%#x, 'e', %d) = %q): %v", hi, lo, prec, s, err)
			}
			if math.IsDecimal128NaN(d) {
				if !math.IsDecimal128NaN(d2) {
					t.Fatalf("%#x:%#x: NaN came back as %q", hi, lo, s)
				}
				continue
			}
			hi2, lo2 := math.Decimal128bits(d2)
			if !bid.Equal128(bid.Uint128{Hi: hi2, Lo: lo2}, x) {
				t.Fatalf("%#x:%#x: %q parses to %#x:%#x", hi, lo, s, hi2, lo2)
			}
			if prec == -2 {
				if s2 := FormatDecimal128(d2, 'e', prec); s2 != s {
					t.Fatalf("%#x:%#x: %q parses and formats as %q", hi, lo, s, s2)
				}
			}
		}
	})
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
		{coeff: 0, exp: -6, g: "0.000000", e: "0e-06", f: "0.000000"},
		{coeff: 0, exp: -7, g: "0e-07", e: "0e-07", f: "0.0000000"},
		{coeff: 1, exp: -6, g: "0.000001", e: "1e-06", f: "0.000001"},
		{coeff: 1, exp: -7, g: "1e-07", e: "1e-07", f: "0.0000001"},
		{coeff: 10, exp: -8, g: "1.0e-07", e: "1.0e-07", f: "0.00000010"},
		{coeff: 1000000000000000, exp: 5, g: "1.000000000000000e+20", e: "1.000000000000000e+20", f: "100000000000000000000"},
		{coeff: 1, exp: 21, g: "1e+21", e: "1e+21", f: "1000000000000000000000"},
		{coeff: 9999999999999999, exp: 0, g: "9999999999999999", e: "9.999999999999999e+15", f: "9999999999999999"},
	}
	for _, tt := range tests {
		// bid.Pack64 is the kernel's packer; the digits here don't come from it.
		bits, _ := bid.Pack64(tt.neg, tt.coeff, tt.exp)
		d := math.Decimal64frombits(bits)
		for _, c := range []struct {
			fmt  byte
			want string
		}{{'g', tt.g}, {'e', tt.e}, {'f', tt.f}} {
			if got := FormatDecimal64(d, c.fmt, -1); got != c.want {
				t.Errorf("FormatDecimal64(%d×10^%d, %c, -1) = %q, want %q", tt.coeff, tt.exp, c.fmt, got, c.want)
			}
			if got := FormatDecimal128(decimal128(d), c.fmt, -1); got != c.want {
				t.Errorf("FormatDecimal128(%d×10^%d, %c, -1) = %q, want %q", tt.coeff, tt.exp, c.fmt, got, c.want)
			}
		}
		back, err := ParseDecimal64(tt.g)
		if err != nil {
			t.Errorf("ParseDecimal64(%q): %v", tt.g, err)
		}
		if math.Decimal64bits(back) != bits {
			t.Errorf("ParseDecimal64(%q) = %#x, want %#x", tt.g, math.Decimal64bits(back), bits)
		}
	}
}
