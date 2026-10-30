// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// Directed tests of division beyond the conformance suite and the
// differential oracle: the reproducers of the strawman's four division
// faults, a generator of exact quotients that stresses the preferred
// exponent, fuzz targets against the reference model and the allocation
// and timing checks.

import (
	"internal/bid"
	"math/big"
	"math/rand/v2"
	"testing"
)

// divCase is one directed division: operands and the expected result in
// the character form, read and written by the reference model's parser
// and formatter, which do not call the kernel.
type divCase struct {
	f    *format
	x, y string
	mode bid.Mode
	want string
}

var divReproducers = []divCase{
	// Audit R3: the quotient must carry a round digit and an exact sticky
	// bit from the remainder whatever the digit counts; the strawman
	// rounded 1/7E+13 wrongly.
	{dec64, "1", "70000000000000", bid.ToNearestEven, "1.428571428571429E-14"},
	// Audit R2: the decimal128 quotient needs the 256-by-128 division to
	// reach all 34 digits.
	{dec128, "1", "12345678901234567890", bid.ToNearestEven, "8.100000072900000663471006037578055E-20"},
	// Audit R4: an exact quotient takes the exponent closest to Q(x) - Q(y),
	// here -2 - (-1) = -1, so 1.00 / 1.0 is 1.0, not 1 or 1.00.
	{dec64, "1.00", "1.0", bid.ToNearestEven, "1.0"},
	{dec128, "1.00", "1.0", bid.ToNearestEven, "1.0"},
	// Overflow through the preferred exponent: 1E+6138 / 1E-9 is exactly
	// 1E+6147, above the largest finite decimal128 value.
	{dec128, "1E+6138", "1E-9", bid.ToNearestEven, "Infinity"},
	{dec128, "1E+6138", "1E-9", bid.TowardZero, "9.999999999999999999999999999999999E+6144"},
	// The widest scaling: a one-digit dividend over a Precision-digit
	// divisor scales by 2×Precision places.
	{dec64, "1", "9999999999999999", bid.ToNearestEven, "1.000000000000000E-16"},
	{dec64, "9", "9999999999999999", bid.TowardPositive, "9.000000000000001E-16"},
	{dec128, "1", "9999999999999999999999999999999999", bid.ToNearestEven, "1.000000000000000000000000000000000E-34"},
	{dec128, "3", "9999999999999999999999999999999999", bid.TowardZero, "3.000000000000000000000000000000000E-34"},
	// The narrowest: a Precision-digit dividend over a one-digit divisor
	// scales by two places and the quotient has Precision+1 or +2 digits.
	{dec64, "9999999999999999", "7", bid.ToNearestEven, "1428571428571428"},
	{dec64, "9999999999999999", "1", bid.ToNearestEven, "9999999999999999"},
	{dec128, "9999999999999999999999999999999999", "7", bid.ToNearestEven, "1428571428571428571428571428571428"},
	// A quotient of 10^(Precision+1), the most trailing zeros the strip
	// can meet, with and without room to take the preferred exponent.
	{dec64, "1E+16", "1", bid.ToNearestEven, "1E+16"},
	{dec64, "1E+16", "1E-15", bid.ToNearestEven, "1E+31"},
	{dec64, "1000", "1E-13", bid.ToNearestEven, "1.000E+16"},
	// An exact value with more than Precision significant digits is
	// inexact: the remainder is zero but the strip finds no zeros.
	{dec64, "1", "1073741824", bid.ToNearestEven, "9.313225746154785E-10"},
	{dec64, "1", "1073741824", bid.TowardPositive, "9.313225746154786E-10"},
	// Ties: the dropped digits are exactly half and the remainder is zero.
	{dec64, "2000000000000001", "2", bid.ToNearestEven, "1000000000000000"},
	{dec64, "2000000000000001", "2", bid.ToNearestAway, "1000000000000001"},
	{dec64, "2000000000000003", "2", bid.ToNearestEven, "1000000000000002"},
	// Specials.
	{dec64, "0", "0", bid.ToNearestEven, "NaN"},
	{dec64, "Infinity", "-Infinity", bid.ToNearestEven, "NaN"},
	{dec64, "-1", "0", bid.ToNearestEven, "-Infinity"},
	{dec64, "Infinity", "-0", bid.ToNearestEven, "-Infinity"},
	{dec64, "-1", "Infinity", bid.TowardNegative, "-0E-398"},
	{dec64, "0E+5", "-1E-10", bid.ToNearestEven, "-0E+15"},
	{dec128, "0", "1E-6176", bid.ToNearestEven, "0E+6111"},
}

func TestDivReproducers(t *testing.T) {
	for _, c := range divReproducers {
		f := c.f
		x, st := f.parse(c.x, bid.ToNearestEven)
		if st != bid.Exact {
			t.Fatalf("%s: operand %q does not parse exactly", f.name, c.x)
		}
		y, st := f.parse(c.y, bid.ToNearestEven)
		if st != bid.Exact {
			t.Fatalf("%s: operand %q does not parse exactly", f.name, c.y)
		}
		got := kernel{}.div(f, x, y, c.mode)
		if s := display(f, got); s != c.want {
			t.Errorf("Div%d(%s, %s, %s) = %s, want %s", f.width, c.x, c.y, modeName(c.mode), s, c.want)
		}
		if ref := (reference{}).div(f, x, y, c.mode); ref != got {
			t.Errorf("Div%d(%s, %s, %s) = %s, reference %s", f.width, c.x, c.y, modeName(c.mode), display(f, got), display(f, ref))
		}
	}

	// The two reproducers whose claim is about the parts, not the text.
	x, _ := dec64.parse("1.00", bid.ToNearestEven)
	y, _ := dec64.parse("1.0", bid.ToNearestEven)
	if v := dec64.decode((kernel{}).div(dec64, x, y, bid.ToNearestEven)); v.coeff.Cmp(big.NewInt(10)) != 0 || v.exp != -1 {
		t.Errorf("Div64(1.00, 1.0) has coefficient %v and exponent %d, want 10 and -1", v.coeff, v.exp)
	}
	x, _ = dec128.parse("1", bid.ToNearestEven)
	y, _ = dec128.parse("12345678901234567890", bid.ToNearestEven)
	if v := dec128.decode((kernel{}).div(dec128, x, y, bid.ToNearestEven)); digits(v.coeff) != dec128.prec {
		t.Errorf("Div128(1, 12345678901234567890) has %d significant digits, want %d", digits(v.coeff), dec128.prec)
	}
}

// TestDivExactQuotients stresses the preferred-exponent path with
// quotients known to be exact: q and y are drawn, x = q × y, and the
// exponents are drawn across the format. Every case is checked against the
// reference model and, independently of it, against the rule of "Exponent
// of a result": the result equals q × 10^(Q(x) - Q(y)) in value, and its
// exponent is the closest to Q(x) - Q(y) at which that value is
// representable (above it only when the coefficient has no trailing zero
// to give up or the exponent is MinExp, below it only when the coefficient
// already has Precision digits or the exponent is MaxExp).
func TestDivExactQuotients(t *testing.T) {
	n := 20000
	if testing.Short() {
		n = 2000
	}
	for _, f := range both {
		g := &gen{r: rand.New(rand.NewPCG(*seedFlag, 0x1d)), f: f}
		for i := range n {
			// dq + dy <= prec keeps x within the format; a few cases add
			// trailing zeros to x, as in 1.00 / 1.0, to move the preferred
			// exponent away from the quotient's natural one.
			dq := 1 + g.r.IntN(f.prec-1)
			dy := 1 + g.r.IntN(f.prec-dq)
			q := g.randDigits(dq)
			y := g.randDigits(dy)
			x := new(big.Int).Mul(q, y)
			if pad := f.prec - digits(x); pad > 0 && g.r.IntN(3) == 0 {
				p := pow10(1 + g.r.IntN(pad))
				x.Mul(x, p)
				q.Mul(q, p)
			}
			// Exponents: mostly moderate, sometimes anywhere in the format.
			var xe, ye int
			if i%4 == 0 {
				xe = f.minExp + g.r.IntN(f.maxExp-f.minExp+1)
				ye = f.minExp + g.r.IntN(f.maxExp-f.minExp+1)
			} else {
				xe = g.r.IntN(41) - 20
				ye = g.r.IntN(41) - 20
			}
			xb := f.encode(value{kind: finite, neg: g.r.IntN(2) == 0, coeff: x, exp: xe})
			yb := f.encode(value{kind: finite, neg: g.r.IntN(2) == 0, coeff: y, exp: ye})
			for m := bid.Mode(0); m < 5; m++ {
				got := kernel{}.div(f, xb, yb, m)
				if want := (reference{}).div(f, xb, yb, m); got != want {
					t.Errorf("Div%d(%s, %s, %s) = %s, reference %s", f.width, display(f, xb), display(f, yb), modeName(m), display(f, got), display(f, want))
					continue
				}
				v := f.decode(got)
				pref := xe - ye
				// Exact in value: coeff × 10^exp = q × 10^pref, unless the
				// value is outside the format, which the reference decides.
				if v.kind != finite {
					continue
				}
				lhs, rhs := new(big.Int).Set(v.coeff), new(big.Int).Set(q)
				if v.exp > pref {
					lhs.Mul(lhs, pow10(v.exp-pref))
				} else {
					rhs.Mul(rhs, pow10(pref-v.exp))
				}
				if lhs.Cmp(rhs) != 0 {
					// The value rounded, which is allowed only when no
					// encoding holds it: with q0 the coefficient stripped
					// of its tz trailing zeros, the exponents at which
					// q0 × 10^(pref+tz) fits are those within the format
					// from pref+tz down to where the coefficient reaches
					// Precision digits.
					tz := trailingZeros(q)
					q0 := new(big.Int).Quo(q, pow10(tz))
					top := min(pref+tz, f.maxExp)
					bottom := max(pref+tz-(f.prec-digits(q0)), f.minExp)
					if digits(q0) <= f.prec && bottom <= top {
						t.Errorf("Div%d(%s, %s, %s) = %s: not the exact quotient %v × 10^%d", f.width, display(f, xb), display(f, yb), modeName(m), display(f, got), q, pref)
					}
					continue
				}
				switch {
				case v.exp > pref && new(big.Int).Rem(v.coeff, bigTen).Sign() == 0 && v.exp != f.minExp:
					t.Errorf("Div%d(%s, %s, %s) = %s: exponent %d is above the preferred %d with a trailing zero to spare", f.width, display(f, xb), display(f, yb), modeName(m), display(f, got), v.exp, pref)
				case v.exp < pref && digits(v.coeff) < f.prec && v.exp != f.maxExp:
					t.Errorf("Div%d(%s, %s, %s) = %s: exponent %d is below the preferred %d with room in the coefficient", f.width, display(f, xb), display(f, yb), modeName(m), display(f, got), v.exp, pref)
				}
			}
		}
	}
}

func TestDivAllocs(t *testing.T) {
	var sink64 uint64
	var sink128 bid.Uint128
	x64, _ := dec64.parse("1", bid.ToNearestEven)
	y64, _ := dec64.parse("70000000000000", bid.ToNearestEven)
	e64, _ := dec64.parse("8.00", bid.ToNearestEven)
	two64, _ := dec64.parse("2", bid.ToNearestEven)
	x128, _ := dec128.parse("1", bid.ToNearestEven)
	y128, _ := dec128.parse("12345678901234567890", bid.ToNearestEven)
	e128, _ := dec128.parse("1.00", bid.ToNearestEven)
	one128, _ := dec128.parse("1.0", bid.ToNearestEven)
	fn := func() {
		sink64 ^= bid.Div64(x64.Lo, y64.Lo, bid.ToNearestEven)
		sink64 ^= bid.Div64(e64.Lo, two64.Lo, bid.TowardPositive)
		sink64 ^= bid.Div64(x64.Lo, 0, bid.ToNearestEven)
		r := bid.Div128(x128, y128, bid.ToNearestEven)
		sink128.Lo ^= r.Lo
		r = bid.Div128(e128, one128, bid.TowardNegative)
		sink128.Lo ^= r.Lo
	}
	if n := testing.AllocsPerRun(100, fn); n != 0 {
		t.Errorf("Div64/Div128 allocate: %v allocs per run", n)
	}
	_, _ = sink64, sink128
}

// The fuzz targets compare the kernel with the reference model on raw bit
// patterns, canonical or not, under all five modes and one outside them.

func FuzzDiv64(f *testing.F) {
	seed := func(x, y string) {
		xb, _ := dec64.parse(x, bid.ToNearestEven)
		yb, _ := dec64.parse(y, bid.ToNearestEven)
		f.Add(xb.Lo, yb.Lo)
	}
	seed("1", "70000000000000")
	seed("1.00", "1.0")
	seed("1", "3")
	seed("2000000000000001", "2")
	seed("1E+300", "1E-300")
	seed("1E-398", "3")
	seed("0", "0")
	seed("Infinity", "1")
	f.Add(uint64(0x6c77ffffffffffff), uint64(0x6c7386f26fc10000))
	f.Fuzz(func(t *testing.T, x, y uint64) {
		for m := bid.Mode(0); m < 6; m++ {
			got := bid.Div64(x, y, m)
			want := reference{}.div(dec64, lo(x), lo(y), m)
			if got != want.Lo {
				t.Errorf("Div64(%s, %s, %s) = %s; reference %s", display(dec64, lo(x)), display(dec64, lo(y)), modeName(m), display(dec64, lo(got)), display(dec64, want))
			}
		}
	})
}

func FuzzDiv128(f *testing.F) {
	seed := func(x, y string) {
		xb, _ := dec128.parse(x, bid.ToNearestEven)
		yb, _ := dec128.parse(y, bid.ToNearestEven)
		f.Add(xb.Hi, xb.Lo, yb.Hi, yb.Lo)
	}
	seed("1", "12345678901234567890")
	seed("1.00", "1.0")
	seed("1E+6138", "1E-9")
	seed("1", "3")
	seed("2000000000000000000000000000000001", "2")
	seed("1E-6176", "3")
	seed("0", "0")
	seed("Infinity", "-Infinity")
	f.Add(uint64(0x3041ed09bead87c0), uint64(0x378d8e6400000000), uint64(0x3041ffffffffffff), uint64(0xffffffffffffffff))
	f.Fuzz(func(t *testing.T, xh, xl, yh, yl uint64) {
		x, y := bid.Uint128{Hi: xh, Lo: xl}, bid.Uint128{Hi: yh, Lo: yl}
		for m := bid.Mode(0); m < 6; m++ {
			got := bid.Div128(x, y, m)
			want := reference{}.div(dec128, x, y, m)
			if got != want {
				t.Errorf("Div128(%s, %s, %s) = %s; reference %s", display(dec128, x), display(dec128, y), modeName(m), display(dec128, got), display(dec128, want))
			}
		}
	})
}

// The benchmarks time typical and worst operands: an inexact quotient of
// two Precision-digit coefficients (the common case), an exact quotient
// with trailing zeros to strip, the widest scaling (one digit over
// Precision digits) and a special-value exit.
var divBench = []struct {
	name string
	x, y string
}{
	{"inexact", "1234567890123456", "7777777777777777"},
	{"exact", "8.00", "2"},
	{"widest", "1", "9999999999999999"},
	{"byzero", "1", "0"},
}

var divBench128 = []struct {
	name string
	x, y string
}{
	{"inexact", "1234567890123456789012345678901234", "7777777777777777777777777777777777"},
	{"exact", "8.00", "2"},
	{"widest", "1", "9999999999999999999999999999999999"},
	{"byzero", "1", "0"},
}

func BenchmarkDiv64(b *testing.B) {
	for _, bb := range divBench {
		x, _ := dec64.parse(bb.x, bid.ToNearestEven)
		y, _ := dec64.parse(bb.y, bid.ToNearestEven)
		b.Run(bb.name, func(b *testing.B) {
			var sink uint64
			for i := 0; i < b.N; i++ {
				sink ^= bid.Div64(x.Lo, y.Lo, bid.ToNearestEven)
			}
			_ = sink
		})
	}
}

func BenchmarkDiv128(b *testing.B) {
	for _, bb := range divBench128 {
		x, _ := dec128.parse(bb.x, bid.ToNearestEven)
		y, _ := dec128.parse(bb.y, bid.ToNearestEven)
		b.Run(bb.name, func(b *testing.B) {
			var sink bid.Uint128
			for i := 0; i < b.N; i++ {
				r := bid.Div128(x, y, bid.ToNearestEven)
				sink.Lo ^= r.Lo
				sink.Hi ^= r.Hi
			}
			_ = sink
		})
	}
}
