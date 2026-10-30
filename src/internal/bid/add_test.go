// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math/big"
	"math/rand/v2"
	"testing"
)

// addTests are the audit reproducers and the cases where an aligned sum can
// go wrong: a gap too large to scale, a smaller operand reduced to a sticky
// bit, a borrow from that bit, a tie found after a shift, and a zero whose
// exponent is far from the other operand's. Each want was worked out by hand
// from "Exponent of a result" and "Sign of a result" in the package
// documentation, and is checked against the kernel and the reference. A want
// is compared as an encoding, so it names the quantum however it is written:
// 1.000000000000000000000000000000000E+20 is the same member of its cohort as
// the character form 100000000000000000000.0000000000000.
var addTests = []struct {
	name string
	f    *format
	x, y string
	mode bid.Mode
	want string
}{
	// Audit R5: the smaller operand is not dropped, and the result has
	// Precision digits because it is inexact.
	{"R5/128", dec128, "1E+20", "1E-20", bid.ToNearestEven, "1.000000000000000000000000000000000E+20"},
	{"R5/64", dec64, "1", "1E-20", bid.ToNearestEven, "1.000000000000000"},

	// A positive smaller operand below the sticky threshold.
	{"sticky/up", dec64, "1", "1E-20", bid.TowardPositive, "1.000000000000001"},
	{"sticky/down", dec64, "1", "1E-20", bid.TowardNegative, "1.000000000000000"},
	{"sticky/zero", dec64, "1", "1E-20", bid.TowardZero, "1.000000000000000"},
	{"sticky/away", dec64, "1", "1E-20", bid.ToNearestAway, "1.000000000000000"},
	{"sticky/negative up", dec64, "-1", "-1E-20", bid.TowardPositive, "-1.000000000000000"},
	{"sticky/negative down", dec64, "-1", "-1E-20", bid.TowardNegative, "-1.000000000000001"},
	{"sticky/128 up", dec128, "1E+20", "1E-20", bid.TowardPositive, "1.000000000000000000000000000000001E+20"},

	// x - ε lands on the neighbour below x in the modes that truncate, and
	// the round digit rounds it back up in the others. The neighbour below
	// a power of ten is one place finer.
	{"borrow/even", dec64, "1", "-1E-20", bid.ToNearestEven, "1.000000000000000"},
	{"borrow/away", dec64, "1", "-1E-20", bid.ToNearestAway, "1.000000000000000"},
	{"borrow/up", dec64, "1", "-1E-20", bid.TowardPositive, "1.000000000000000"},
	{"borrow/down", dec64, "1", "-1E-20", bid.TowardNegative, "0.9999999999999999"},
	{"borrow/zero", dec64, "1", "-1E-20", bid.TowardZero, "0.9999999999999999"},
	{"borrow/negative up", dec64, "-1", "1E-20", bid.TowardPositive, "-0.9999999999999999"},
	{"borrow/negative down", dec64, "-1", "1E-20", bid.TowardNegative, "-1.000000000000000"},
	{"borrow/not a power", dec64, "2", "-1E-20", bid.TowardZero, "1.999999999999999"},
	{"borrow/128 even", dec128, "1E+20", "-1E-20", bid.ToNearestEven, "1.000000000000000000000000000000000E+20"},
	{"borrow/128 zero", dec128, "1E+20", "-1E-20", bid.TowardZero, "9.999999999999999999999999999999999E+19"},
	{"borrow/128 not a power", dec128, "3", "-1E-6000", bid.TowardNegative, "2.999999999999999999999999999999999"},
	{"borrow/smaller first", dec64, "-1E-20", "1", bid.TowardZero, "0.9999999999999999"},
	{"borrow/negative result", dec64, "1", "-1E+20", bid.ToNearestEven, "-1.000000000000000E+20"},

	// The whole exponent range.
	{"range/64 up", dec64, "1E+369", "1E-398", bid.TowardPositive, "1.000000000000001E+369"},
	{"range/64 zero", dec64, "1E+369", "-1E-398", bid.TowardZero, "9.999999999999999E+368"},
	{"range/128 up", dec128, "1E+6111", "1E-6176", bid.TowardPositive, "1.000000000000000000000000000000001E+6111"},
	{"range/128 zero", dec128, "1E+6111", "-1E-6176", bid.TowardZero, "9.999999999999999999999999999999999E+6110"},

	// At the edges of alignMaxW. The first exact case keeps its digits; the
	// first shifted ones decide the same way.
	{"edge/64 scaled", dec64, "1E+19", "1", bid.TowardPositive, "1.000000000000001E+19"},
	{"edge/64 shifted", dec64, "1E+20", "1", bid.TowardPositive, "1.000000000000001E+20"},
	{"edge/128 scaled", dec128, "1E+38", "1", bid.TowardPositive, "1.000000000000000000000000000000001E+38"},
	{"edge/128 shifted", dec128, "1E+39", "1", bid.TowardPositive, "1.000000000000000000000000000000001E+39"},

	// The shifted operand still overlaps the larger one, so the sum has to
	// be right in the kept digits and at a tie.
	{"overlap/tie even", dec64, "1E+36", "5000000000000000E+5", bid.ToNearestEven, "1.000000000000000E+36"},
	{"overlap/tie away", dec64, "1E+36", "5000000000000000E+5", bid.ToNearestAway, "1.000000000000001E+36"},
	{"overlap/above tie", dec64, "1E+36", "5000000000000001E+5", bid.ToNearestEven, "1.000000000000001E+36"},
	{"overlap/exact", dec64, "1E+36", "1000000000000000E+6", bid.ToNearestEven, "1.000000000000001E+36"},
	{"overlap/exact difference", dec64, "1E+36", "-5000000000000000E+5", bid.ToNearestEven, "9.999999999999995E+35"},
	{"overlap/difference", dec64, "1E+36", "-5000000000000001E+5", bid.ToNearestEven, "9.999999999999995E+35"},
	{"overlap/difference zero", dec64, "1E+36", "-5000000000000001E+5", bid.TowardZero, "9.999999999999994E+35"},

	// A zero operand: the sum is the other operand at the preferred
	// exponent, or at the member of its cohort nearest that.
	{"zero/larger exponent", dec64, "0E+300", "1", bid.ToNearestEven, "1"},
	{"zero/larger exponent 16 digits", dec64, "0E+369", "1234567890123456E-398", bid.ToNearestEven, "1.234567890123456E-383"},
	{"zero/larger exponent 34 digits", dec128, "0E+6111", "1234567890123456789012345678901234E-6176", bid.ToNearestEven, "1.234567890123456789012345678901234E-6143"},
	{"zero/smaller exponent", dec64, "1E+300", "0E-300", bid.ToNearestEven, "1.000000000000000E+300"},
	{"zero/smaller exponent 128", dec128, "1E+3000", "0E-3000", bid.ToNearestEven, "1.000000000000000000000000000000000E+3000"},
	{"zero/near", dec64, "1E+2", "0", bid.ToNearestEven, "100"},
	{"zero/range", dec64, "0E-398", "1E+369", bid.ToNearestEven, "1.000000000000000E+369"},
	{"zero/two far", dec64, "0E+369", "-0E-398", bid.ToNearestEven, "0E-398"},
	{"zero/two far down", dec64, "0E+369", "-0E-398", bid.TowardNegative, "-0E-398"},
	{"zero/two negative", dec64, "-0E+369", "-0E-398", bid.ToNearestEven, "-0E-398"},

	// The sign of an exact cancellation, in each mode, and at the preferred
	// exponent however the operands are written.
	{"cancel/even", dec64, "1E+3", "-1E+3", bid.ToNearestEven, "0E+3"},
	{"cancel/away", dec64, "1E+3", "-1E+3", bid.ToNearestAway, "0E+3"},
	{"cancel/zero", dec64, "1E+3", "-1E+3", bid.TowardZero, "0E+3"},
	{"cancel/up", dec64, "1E+3", "-1E+3", bid.TowardPositive, "0E+3"},
	{"cancel/down", dec64, "1E+3", "-1E+3", bid.TowardNegative, "-0E+3"},
	{"cancel/negative first", dec64, "-1E+3", "1E+3", bid.TowardPositive, "0E+3"},
	{"cancel/cohort", dec64, "1000", "-1E+3", bid.ToNearestEven, "0"},
	{"cancel/128 down", dec128, "1E+3", "-1000", bid.TowardNegative, "-0"},

	// An exact sum that borrows: the operand with the larger exponent is the
	// smaller one.
	{"exact/borrow", dec64, "1E+1", "-11.00", bid.ToNearestEven, "-1.00"},
	{"exact/128 borrow", dec128, "1E+1", "-11.00", bid.ToNearestEven, "-1.00"},
	{"exact/preferred", dec64, "1.0", "2.00", bid.ToNearestEven, "3.00"},
	{"exact/subnormal", dec64, "1E-398", "1E-398", bid.ToNearestEven, "2E-398"},
	{"exact/fold-down", dec64, "1.0E+370", "1.0E+370", bid.ToNearestEven, "2.0E+370"},

	// Overflow decided after a sticky bit moves the rounding.
	{"overflow/up", dec64, "9.999999999999999E+384", "1E-398", bid.TowardPositive, "Infinity"},
	{"overflow/even", dec64, "9.999999999999999E+384", "1E-398", bid.ToNearestEven, "9.999999999999999E+384"},
	{"overflow/borrow", dec64, "9.999999999999999E+384", "-1E-398", bid.TowardZero, "9.999999999999998E+384"},
	{"overflow/128 down", dec128, "-9.999999999999999999999999999999999E+6144", "-1E-6176", bid.TowardNegative, "-Infinity"},

	// Infinity and NaN.
	{"inf/cancel", dec64, "Infinity", "-Infinity", bid.ToNearestEven, "NaN"},
	{"inf/cancel 128", dec128, "-Infinity", "Infinity", bid.TowardNegative, "NaN"},
	{"inf/same", dec64, "-Infinity", "-Infinity", bid.ToNearestEven, "-Infinity"},
	{"inf/first", dec64, "-Infinity", "1E+300", bid.ToNearestEven, "-Infinity"},
	{"inf/second", dec128, "1E+300", "-Infinity", bid.ToNearestEven, "-Infinity"},
	{"nan/first", dec64, "NaN", "1", bid.ToNearestEven, "NaN"},
	{"nan/second", dec128, "1", "sNaN", bid.ToNearestEven, "NaN"},
	{"nan/with inf", dec64, "Infinity", "-NaN", bid.ToNearestEven, "NaN"},
}

func addOperand(t *testing.T, f *format, s string) bits {
	t.Helper()
	x, st := f.parse(s, bid.ToNearestEven)
	if st != bid.Exact {
		t.Fatalf("operand %q is not exact in %s: %v", s, f.name, st)
	}
	return x
}

func TestAddDirected(t *testing.T) {
	for _, tt := range addTests {
		t.Run(tt.name, func(t *testing.T) {
			x, y := addOperand(t, tt.f, tt.x), addOperand(t, tt.f, tt.y)
			want := addOperand(t, tt.f, tt.want)
			check := func(op string, got, ref bits) {
				t.Helper()
				if got != want {
					t.Errorf("%s(%s, %s, %s) = %s, want %s", op, tt.x, tt.y, modeName(tt.mode), display(tt.f, got), tt.want)
				}
				if ref != want {
					t.Errorf("reference %s(%s, %s, %s) = %s, want %s", op, tt.x, tt.y, modeName(tt.mode), display(tt.f, ref), tt.want)
				}
			}
			check("Add", kernel{}.add(tt.f, x, y, tt.mode), reference{}.add(tt.f, x, y, tt.mode))
			neg := tt.f.withSign(y, !tt.f.signbit(y))
			check("Sub", kernel{}.sub(tt.f, x, neg, tt.mode), reference{}.sub(tt.f, x, neg, tt.mode))
		})
	}
}

// TestAddAllModesAgainstReference runs every pair of operands of addTests
// through both operations in every mode, whatever mode the table gave it.
func TestAddAllModesAgainstReference(t *testing.T) {
	for _, tt := range addTests {
		x, y := addOperand(t, tt.f, tt.x), addOperand(t, tt.f, tt.y)
		for _, m := range modes {
			if got, ref := (kernel{}).add(tt.f, x, y, m), (reference{}).add(tt.f, x, y, m); got != ref {
				t.Errorf("%s: Add(%s, %s, %s): kernel %s, reference %s", tt.name, tt.x, tt.y, modeName(m), display(tt.f, got), display(tt.f, ref))
			}
			if got, ref := (kernel{}).sub(tt.f, x, y, m), (reference{}).sub(tt.f, x, y, m); got != ref {
				t.Errorf("%s: Sub(%s, %s, %s): kernel %s, reference %s", tt.name, tt.x, tt.y, modeName(m), display(tt.f, got), display(tt.f, ref))
			}
		}
	}
}

func TestAddAllocs(t *testing.T) {
	x64, y64 := addOperand(t, dec64, "1E+300").Lo, addOperand(t, dec64, "-1.5E-300").Lo
	x128, y128 := addOperand(t, dec128, "1E+3000"), addOperand(t, dec128, "-1.5E-3000")
	near64, near128 := addOperand(t, dec64, "1.25").Lo, addOperand(t, dec128, "1.25")
	var sink64 uint64
	var sink128 bid.Uint128
	fn := func() {
		sink64 ^= bid.Add64(x64, y64, bid.TowardZero)
		sink64 ^= bid.Sub64(near64, y64, bid.ToNearestEven)
		sink64 ^= bid.Add64(near64, near64, bid.ToNearestEven)
		sink128 = bid.Add128(x128, y128, bid.TowardZero)
		sink128 = bid.Sub128(near128, y128, bid.ToNearestEven)
		sink128 = bid.Add128(near128, near128, bid.ToNearestEven)
	}
	if n := testing.AllocsPerRun(100, fn); n != 0 {
		t.Errorf("Add and Sub allocate: %v allocs per run", n)
	}
	_, _ = sink64, sink128
}

// FuzzAdd64 and FuzzAdd128 compare the kernel with the reference on operands
// that are raw encodings, non-canonical ones included. Most raw pairs are far
// apart, so a non-zero near moves y to within near places of x, which is
// where the sum has digits of both operands.
func FuzzAdd64(f *testing.F) {
	enc := func(s string) uint64 { x, _ := dec64.parse(s, bid.ToNearestEven); return x.Lo }
	for _, s := range addTests {
		if s.f == dec64 {
			f.Add(enc(s.x), enc(s.y), int16(0), uint8(0), false)
		}
	}
	f.Add(enc("1"), enc("1"), int16(20), uint8(4), true)
	f.Add(enc("9999999999999999"), enc("9999999999999999"), int16(-19), uint8(3), true)
	f.Fuzz(func(t *testing.T, x, y uint64, near int16, mode uint8, sub bool) {
		m := bid.Mode(mode)
		if xk, _, _, xe := bid.Unpack64(x); near != 0 && xk == bid.KindFinite {
			if yk, yneg, yc, _ := bid.Unpack64(y); yk == bid.KindFinite {
				if r, ok := bid.Pack64(yneg, yc, xe+int(near%64)); ok {
					y = r
				}
			}
		}
		got, want := bid.Add64(x, y, m), reference{}.add(dec64, lo(x), lo(y), m).Lo
		if sub {
			got, want = bid.Sub64(x, y, m), reference{}.sub(dec64, lo(x), lo(y), m).Lo
		}
		if got != want {
			t.Errorf("x=%s y=%s %s sub=%t: kernel %s, reference %s", display(dec64, lo(x)), display(dec64, lo(y)), modeName(m), sub, display(dec64, lo(got)), display(dec64, lo(want)))
		}
	})
}

func FuzzAdd128(f *testing.F) {
	enc := func(s string) bits { x, _ := dec128.parse(s, bid.ToNearestEven); return x }
	for _, s := range addTests {
		if s.f == dec128 {
			x, y := enc(s.x), enc(s.y)
			f.Add(x.Hi, x.Lo, y.Hi, y.Lo, int16(0), uint8(0), false)
		}
	}
	one := enc("1")
	f.Add(one.Hi, one.Lo, one.Hi, one.Lo, int16(39), uint8(4), true)
	f.Fuzz(func(t *testing.T, xh, xl, yh, yl uint64, near int16, mode uint8, sub bool) {
		x, y := bid.Uint128{Hi: xh, Lo: xl}, bid.Uint128{Hi: yh, Lo: yl}
		m := bid.Mode(mode)
		if xk, _, _, xe := bid.Unpack128(x); near != 0 && xk == bid.KindFinite {
			if yk, yneg, yc, _ := bid.Unpack128(y); yk == bid.KindFinite {
				if r, ok := bid.Pack128(yneg, yc, xe+int(near%128)); ok {
					y = r
				}
			}
		}
		got, want := bid.Add128(x, y, m), reference{}.add(dec128, x, y, m)
		if sub {
			got, want = bid.Sub128(x, y, m), reference{}.sub(dec128, x, y, m)
		}
		if got != want {
			t.Errorf("x=%s y=%s %s sub=%t: kernel %s, reference %s", display(dec128, x), display(dec128, y), modeName(m), sub, display(dec128, got), display(dec128, want))
		}
	})
}

// addBenchOperands draws the mix of the width spike in the package
// documentation: 70% with exponents within 4 of each other, 20% spread over
// ±(Precision+10) and 10% equal.
func addBenchOperands(f *format) [][2]bits {
	const count = 4096
	r := rand.New(rand.NewPCG(1, 2))
	coeff := func() value {
		c := new(big.Int)
		for range r.IntN(f.prec) + 1 {
			c.Mul(c, big.NewInt(10)).Add(c, big.NewInt(r.Int64N(10)))
		}
		return value{kind: finite, neg: r.IntN(2) == 0, coeff: c, exp: r.IntN(21) - 10}
	}
	pairs := make([][2]bits, count)
	for i := range pairs {
		x, y := coeff(), coeff()
		switch n := r.IntN(10); {
		case n < 7:
			y.exp = x.exp + r.IntN(9) - 4
		case n < 9:
			y.exp = x.exp + r.IntN(2*(f.prec+10)+1) - (f.prec + 10)
		default:
			y = x
		}
		pairs[i] = [2]bits{f.encode(x), f.encode(y)}
	}
	return pairs
}

func BenchmarkAdd64(b *testing.B) {
	pairs := addBenchOperands(dec64)
	var sink uint64
	i := 0
	for b.Loop() {
		p := &pairs[i&(len(pairs)-1)]
		sink ^= bid.Add64(p[0].Lo, p[1].Lo, bid.ToNearestEven)
		i++
	}
	_ = sink
}

func BenchmarkAdd128(b *testing.B) {
	pairs := addBenchOperands(dec128)
	var sink uint64
	i := 0
	for b.Loop() {
		p := &pairs[i&(len(pairs)-1)]
		sink ^= bid.Add128(p[0], p[1], bid.ToNearestEven).Lo
		i++
	}
	_ = sink
}
