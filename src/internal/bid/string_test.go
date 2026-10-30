// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// Tests of the character conversions beyond the vectors of
// testdata/string.txt and the differential tests of TestKernel: strings
// with digit runs and exponents of any length, which the reference model
// reads with big integers and therefore judges exactly; allocation; the
// round trip of Append and Parse; and fuzzing against the reference.

import (
	"fmt"
	"internal/bid"
	"math/big"
	"strings"
	"testing"
)

var allModes = []bid.Mode{bid.ToNearestEven, bid.ToNearestAway, bid.TowardZero, bid.TowardPositive, bid.TowardNegative, 9}

// checkParse compares the kernel's Parse of s with the reference's in
// every mode and every width.
func checkParse(t *testing.T, s string, modes []bid.Mode) {
	t.Helper()
	for _, f := range []*format{dec64, dec128} {
		for _, m := range modes {
			got, gst := kernel{}.parse(f, s, m)
			want, wst := reference{}.parse(f, s, m)
			if got != want || gst != wst {
				t.Fatalf("%s Parse(%.80q, %d) = %s %s, reference %s %s", f.name, s, m, display(f, got), showStatus(gst), display(f, want), showStatus(wst))
			}
		}
	}
}

// TestParseLongRuns checks strings whose digit runs and exponents are far
// longer than either format holds, in the three places a run can sit: the
// zeros before the first significant digit, the digits past the coefficient
// and the exponent part, and with the exponent compensating for the digits.
func TestParseLongRuns(t *testing.T) {
	zeros := func(n int) string { return strings.Repeat("0", n) }
	var cases []string
	for _, n := range []int{0, 1, 5, 17, 18, 19, 34, 35, 36, 38, 39, 40, 100, 3000} {
		z := zeros(n)
		cases = append(cases,
			// Leading zeros, before and after the point.
			z+"1", "0."+z+"1", "-0."+z+"1E+"+fmt.Sprint(n), "0."+z+"1E+"+fmt.Sprint(n+1), "0."+z+"1E+"+fmt.Sprint(n+2),
			"0."+z+"1E+"+fmt.Sprint(n-1), "0."+z+"123456789012345678901234567890123456789E+"+fmt.Sprint(n),
			z+"."+z, z+"."+z+"E+99999999999999999999", z+"."+z+"E-99999999999999999999",
			// Trailing zeros: exact, so the exponent rises, and with a
			// trailing non-zero digit far below, inexact.
			"1"+z, "1"+z+"E-"+fmt.Sprint(n), "1"+z+"E-"+fmt.Sprint(n+1), "1"+z+"E-"+fmt.Sprint(n-1),
			"12345678901234565"+z, "12345678901234565"+z+"1", "12345678901234564"+z+"1", "12345678901234566"+z+"1",
			"1234567890123456789012345678901234"+"5"+z, "1234567890123456789012345678901234"+"5"+z+"1",
			"1."+z+"1", "1."+z+"1E+99", "9."+strings.Repeat("9", n)+"E+384", "9."+strings.Repeat("9", n)+"E+6144",
			"0."+strings.Repeat("9", n), "0.5"+z+"1", "0.5"+z, "-0.5"+z+"1",
			// Digits and exponent that cancel.
			"1"+z+"E-"+fmt.Sprint(n)+"00", "1"+z+"E"+fmt.Sprint(-n),
			"1"+z+"."+z+"1E-"+fmt.Sprint(n),
		)
	}
	huge := []string{"99999999999999999999", "9223372036854775807", "9223372036854775808", "18446744073709551616",
		"1" + zeros(40), zeros(40) + "5", zeros(100) + "1" + zeros(100), strings.Repeat("9", 5000)}
	for _, e := range huge {
		for _, sig := range []string{"1", "0", "0.00", "1.5", "9999999999999999", "9999999999999999999999999999999999", "0.0000001", "1" + zeros(50), "0." + zeros(50) + "1", "0." + zeros(5000) + "1"} {
			for _, sign := range []string{"", "-", "+"} {
				cases = append(cases, sign+sig+"E+"+e, sign+sig+"E-"+e, sign+sig+"e"+e, sign+sig+"E"+zeros(30)+"7")
			}
		}
	}
	// A long run of leading zeros compensating a long exponent: the
	// reference reads the significant digits only, so it stays fast.
	for _, n := range []int{10000, 100000, 1000000} {
		s := fmt.Sprint(n)
		cases = append(cases, "0."+zeros(n)+"1E+"+s, "0."+zeros(n-1)+"1E+"+s, "0."+zeros(n)+"15E+"+s, "-"+zeros(n)+"."+zeros(n)+"E+"+s)
	}
	for _, s := range cases {
		checkParse(t, s, allModes)
	}
}

// TestParseLongExponentCompensation pins results that the reference
// agrees with, so that a reference bug cannot hide a kernel one.
func TestParseLongExponentCompensation(t *testing.T) {
	zeros := func(n int) string { return strings.Repeat("0", n) }
	tests := []struct {
		s    string
		want string
		st   bid.Status
	}{
		{"0." + zeros(1000000) + "1E+1000000", "0.1", bid.Exact},
		{"0." + zeros(1000000) + "1E+1000001", "1", bid.Exact},
		{"1" + zeros(1000000) + "E-1000000", "1.000000000000000", bid.Exact},
		{"1" + zeros(1000000) + "E-1000001", "0.1000000000000000", bid.Exact},
		{"1" + zeros(1000000) + "E-999999", "10.00000000000000", bid.Exact},
		{"1E+99999999999999999999", "Infinity", bid.Inexact | bid.Overflow},
		{"1E-99999999999999999999", "0E-398", bid.Inexact | bid.Underflow},
		{"0E+99999999999999999999", "0E+369", bid.Exact},
		{"0E-99999999999999999999", "0E-398", bid.Exact},
		{"-0." + zeros(500) + "E+99999999999999999999", "-0E+369", bid.Exact},
	}
	for _, tt := range tests {
		x, st := bid.Parse64(tt.s, bid.ToNearestEven)
		if got := string(bid.Append64(nil, x)); got != tt.want || st != tt.st {
			t.Errorf("Parse64(%.40q...) = %s %s, want %s %s", tt.s, got, showStatus(st), tt.want, showStatus(tt.st))
		}
	}
}

func TestStringAllocs(t *testing.T) {
	buf := make([]byte, 0, bid.MaxLen128)
	long := "0." + strings.Repeat("0", 3000) + "123456789012345678901234567890123456789E+99999999999999999999"
	var sink64 uint64
	var sink128 bid.Uint128
	var sinkSt bid.Status
	var sinkLen int
	x64s := []uint64{0, bid.Zero64, 0x6c7386f26fc0ffff, 0xfc00000000000000, bid.Inf64, bid.NaN64, 1<<63 | 0x6c7386f26fc0ffff}
	x128s := []bid.Uint128{{}, {Hi: bid.Zero128Hi}, {Hi: 0x3041ed09bead87c0, Lo: 0x378d8e63ffffffff}, {Hi: bid.Inf128Hi}, {Hi: bid.NaN128Hi}, {Hi: 1<<63 | 0x3041ed09bead87c0, Lo: 0x378d8e63ffffffff}}
	strs := []string{"1.50", "-0", "NaN", "inf", "12345678901234567890123456789012345678901", "1E+99999999999999999999", "0E-5", long}
	fn := func() {
		for _, x := range x64s {
			sinkLen += len(bid.Append64(buf[:0], x))
		}
		for _, x := range x128s {
			sinkLen += len(bid.Append128(buf[:0], x))
		}
		for _, s := range strs {
			var st bid.Status
			sink64, st = bid.Parse64(s, bid.ToNearestEven)
			sinkSt |= st
			sink128, st = bid.Parse128(s, bid.TowardPositive)
			sinkSt |= st
		}
	}
	if n := testing.AllocsPerRun(100, fn); n != 0 {
		t.Errorf("Append and Parse allocate: %v allocs per run", n)
	}
	_, _, _, _ = sink64, sink128, sinkSt, sinkLen
}

// TestAppendLongest checks that the longest forms fit MaxLen.
func TestAppendLongest(t *testing.T) {
	for _, f := range []*format{dec64, dec128} {
		maxLen := bid.MaxLen64
		if f == dec128 {
			maxLen = bid.MaxLen128
		}
		longest := 0
		for _, neg := range []bool{false, true} {
			for _, e := range []int{f.minExp, f.minExp + 1, -f.prec - 6, -f.prec - 5, -f.prec - 4, -f.prec + 1, -f.prec, -1, 0, 1, f.maxExp - 1, f.maxExp} {
				c := new(big.Int).Set(f.maxCoeff)
				x := f.encode(value{kind: finite, neg: neg, coeff: c, exp: e})
				s := kernel{}.appendTo(f, x)
				longest = max(longest, len(s))
			}
		}
		if longest != maxLen {
			t.Errorf("%s: the longest form is %d bytes, MaxLen is %d", f.name, longest, maxLen)
		}
	}
}

// fuzzString builds a numeral from fuzz inputs: a sign, digit runs repeated
// into long ones, a point, and an exponent part of any length.
func fuzzString(sign uint8, intPart, fracPart []byte, intRep, fracRep uint8, hasPoint bool, expMark, expSign uint8, expPart []byte, expRep uint8, mutate uint16) string {
	digits := func(b []byte, rep uint8) string {
		d := make([]byte, len(b))
		for i, c := range b {
			d[i] = '0' + c%10
		}
		return strings.Repeat(string(d), int(rep)%48+1)
	}
	var sb strings.Builder
	sb.WriteString([]string{"", "", "-", "+"}[sign%4])
	sb.WriteString(digits(intPart, intRep))
	if hasPoint {
		sb.WriteByte('.')
		sb.WriteString(digits(fracPart, fracRep))
	}
	switch expMark % 4 {
	case 1:
		sb.WriteByte('e')
	case 2:
		sb.WriteByte('E')
	case 3:
		sb.WriteString("E")
		sb.WriteString([]string{"+", "-", "", "+", "-"}[expSign%5])
		sb.WriteString(digits(expPart, expRep))
	}
	if expMark%4 != 3 && expMark%4 != 0 {
		sb.WriteString([]string{"+", "-", "", "+", "-"}[expSign%5])
		sb.WriteString(digits(expPart, expRep))
	}
	s := sb.String()
	// Rarely damage one byte, to reach the syntax errors.
	if mutate != 0 && len(s) > 0 {
		b := []byte(s)
		b[int(mutate)%len(b)] = ".eE+-xX0 9"[int(mutate>>8)%10]
		s = string(b)
	}
	return s
}

// FuzzParseReference compares Parse with the reference model on numerals
// with long digit runs and huge exponents, and on slightly damaged ones.
func FuzzParseReference(f *testing.F) {
	f.Add(uint8(0), []byte{1}, []byte{0, 1}, uint8(0), uint8(2), true, uint8(3), uint8(0), []byte{9}, uint8(7), uint16(0), uint8(0))
	f.Add(uint8(2), []byte{9, 9, 9}, []byte{9}, uint8(7), uint8(30), true, uint8(3), uint8(1), []byte{3, 8}, uint8(0), uint16(0), uint8(2))
	f.Add(uint8(0), []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 5}, []byte{}, uint8(0), uint8(0), false, uint8(0), uint8(0), []byte{}, uint8(0), uint16(0), uint8(3))
	f.Add(uint8(0), []byte{0}, []byte{0, 0, 0, 1}, uint8(0), uint8(47), true, uint8(3), uint8(3), []byte{1, 2}, uint8(40), uint16(0), uint8(0))
	f.Add(uint8(0), []byte{1}, []byte{}, uint8(0), uint8(0), true, uint8(3), uint8(0), []byte{1}, uint8(0), uint16(0x0102), uint8(4))
	f.Fuzz(func(t *testing.T, sign uint8, intPart, fracPart []byte, intRep, fracRep uint8, hasPoint bool, expMark, expSign uint8, expPart []byte, expRep uint8, mutate uint16, mode uint8) {
		if len(intPart)+len(fracPart)+len(expPart) > 200 {
			return
		}
		s := fuzzString(sign, intPart, fracPart, intRep, fracRep, hasPoint, expMark, expSign, expPart, expRep, mutate)
		checkParse(t, s, []bid.Mode{bid.Mode(mode % 7)})
	})
}

// FuzzParseBytes feeds arbitrary bytes: Parse never panics, a Syntax status
// comes alone with the canonical NaN, and an ASCII string agrees with the
// reference.
func FuzzParseBytes(f *testing.F) {
	for _, s := range []string{"", "1", "-", ".", "NaNxyz", "Infinityx", "snan", "1e", "1E+", "\x00", "１", "1.5E+3", "0x1p-2", "infinit", "+.5e-0"} {
		f.Add(s, uint8(0))
	}
	f.Fuzz(func(t *testing.T, s string, mode uint8) {
		m := bid.Mode(mode)
		x64, st64 := bid.Parse64(s, m)
		x128, st128 := bid.Parse128(s, m)
		if st64&bid.Syntax != 0 && (st64 != bid.Syntax || x64 != bid.NaN64) {
			t.Fatalf("Parse64(%q) = %#x, %s", s, x64, showStatus(st64))
		}
		if st128&bid.Syntax != 0 && (st128 != bid.Syntax || x128 != (bid.Uint128{Hi: bid.NaN128Hi})) {
			t.Fatalf("Parse128(%q) = %#x, %s", s, x128, showStatus(st128))
		}
		if (st64 == bid.Syntax) != (st128 == bid.Syntax) {
			t.Fatalf("Parse64(%q) and Parse128 disagree on syntax", s)
		}
		if !bid.IsCanonical64(x64) || !bid.IsCanonical128(x128) {
			t.Fatalf("Parse(%q) is not canonical", s)
		}
		for i := range len(s) {
			if s[i] >= 0x80 {
				return
			}
		}
		checkParse(t, s, []bid.Mode{m})
	})
}

// FuzzRoundTrip checks that Parse reads back every canonical encoding that
// Append writes, bit for bit (a NaN reads back as the canonical quiet NaN),
// that the text is the reference's, that it fits MaxLen, and that Append
// keeps what is already in the buffer.
func FuzzRoundTrip(f *testing.F) {
	for _, wide := range []bool{false, true} {
		f.Add(wide, false, uint64(0), uint64(150), -2, uint64(0), uint64(0))
		f.Add(wide, true, uint64(0), uint64(0), 0, uint64(0), uint64(0))
		f.Add(wide, false, uint64(0), uint64(1), -7, uint64(0), uint64(0))
		f.Add(wide, true, uint64(0), uint64(9999999999999999), 369, uint64(0), uint64(0))
		f.Add(wide, false, uint64(0), uint64(9999999999999999), -398, uint64(0), uint64(0))
		f.Add(wide, false, uint64(0x0001ED09BEAD87C0), uint64(0x378D8E63FFFFFFFF), 6111, uint64(0), uint64(0))
		f.Add(wide, false, uint64(0), uint64(12345), -7, uint64(0x7800000000000000), uint64(0))
		f.Add(wide, false, uint64(0), uint64(12345), -7, uint64(0x7C00000000000000), uint64(0))
		f.Add(wide, false, uint64(0), uint64(12345), -7, uint64(0xFE00000000000001), uint64(5))
	}
	f.Fuzz(func(t *testing.T, wide, neg bool, chi, clo uint64, exp int, rawHi, rawLo uint64) {
		fm := fuzzFormat(wide)
		maxLen := bid.MaxLen64
		if wide {
			maxLen = bid.MaxLen128
		}
		// A finite value built from the fuzz inputs, and an arbitrary pattern.
		c := new(big.Int).Lsh(new(big.Int).SetUint64(chi), 64)
		c.Or(c, new(big.Int).SetUint64(clo))
		c.Mod(c, new(big.Int).Add(fm.maxCoeff, bigOne))
		built := fm.encode(value{kind: finite, neg: neg, coeff: c, exp: clamp(exp, fm.minExp, fm.maxExp)})
		for _, x := range []bits{built, rawBits(fm, rawHi, rawLo)} {
			text := kernel{}.appendTo(fm, x)
			if len(text) > maxLen {
				t.Fatalf("%s: Append(%s) = %q is %d bytes, MaxLen is %d", fm.name, display(fm, x), text, len(text), maxLen)
			}
			if want := (reference{}).appendTo(fm, x); text != want {
				t.Fatalf("%s: Append(%#x) = %q, reference %q", fm.name, x, text, want)
			}
			var prefix []byte
			if wide {
				prefix = bid.Append128([]byte("pre"), x)
			} else {
				prefix = bid.Append64([]byte("pre"), x.Lo)
			}
			if string(prefix) != "pre"+text {
				t.Fatalf("%s: Append to a buffer gives %q, want %q", fm.name, prefix, "pre"+text)
			}
			back, st := kernel{}.parse(fm, text, bid.ToNearestEven)
			want := x
			if v := fm.decode(x); !fm.canonical(x) || v.kind == notANumber {
				// A non-canonical encoding reads back as the value it
				// denotes, a NaN as the canonical quiet NaN.
				want = fm.encode(v)
			}
			if back != want || st != bid.Exact {
				t.Fatalf("%s: Parse(Append(%#x) = %q) = %#x %s, want %#x Exact", fm.name, x, text, back, showStatus(st), want)
			}
		}
	})
}
