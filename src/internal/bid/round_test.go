// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"internal/bid"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The boundary cases of the rounding routine, each a named entry with the
// result derived by hand from the contract and checked, in addition,
// against the reference model. The differential test TestKernel and the
// conformance vectors cover the bulk; these are the places where an
// implementation error passes ordinary tests: the fold-down edge at Emax,
// Emin minus the precision, ties in every mode, a carry out of the top
// digit, the overflow that a tie produces, and the sign of a zero under a
// directed mode.
var roundBoundaryTests = []struct {
	name   string
	f      *format
	neg    bool
	coeff  string // decimal digits of the coefficient
	exp    int
	sticky bool
	mode   bid.Mode
	want   string // the character form of the result
	status bid.Status
}{
	// Emax with fold-down room: the exponent exceeds MaxExp and the
	// coefficient has room for the zeros that bring it back.
	{"emax/fold-down room", dec64, false, "1", 370, false, bid.ToNearestEven, "1.0E+370", bid.Exact},
	{"emax/fold-down fills precision", dec64, false, "1", 384, false, bid.ToNearestEven, "1.000000000000000E+384", bid.Exact},
	{"emax/fold-down 15 zeros", dec64, false, "9", 384, false, bid.TowardZero, "9.000000000000000E+384", bid.Exact},
	{"emax/largest finite exact", dec64, false, "9999999999999999", 369, false, bid.ToNearestEven, "9.999999999999999E+384", bid.Exact},
	// Emax without fold-down room: a full coefficient one place too high,
	// and a one-digit coefficient one place beyond the room.
	{"emax/no room even", dec64, false, "9999999999999999", 370, false, bid.ToNearestEven, "Infinity", bid.Inexact | bid.Overflow},
	{"emax/no room zero", dec64, false, "9999999999999999", 370, false, bid.TowardZero, "9.999999999999999E+384", bid.Inexact | bid.Overflow},
	{"emax/no room negative up", dec64, true, "9999999999999999", 370, false, bid.TowardPositive, "-9.999999999999999E+384", bid.Inexact | bid.Overflow},
	{"emax/no room negative down", dec64, true, "9999999999999999", 370, false, bid.TowardNegative, "-Infinity", bid.Inexact | bid.Overflow},
	{"emax/one beyond fold-down", dec64, false, "1", 385, false, bid.ToNearestEven, "Infinity", bid.Inexact | bid.Overflow},
	{"emax/one beyond fold-down zero", dec64, false, "1", 385, false, bid.TowardZero, "9.999999999999999E+384", bid.Inexact | bid.Overflow},
	{"emax/one beyond fold-down down", dec64, false, "1", 385, false, bid.TowardNegative, "9.999999999999999E+384", bid.Inexact | bid.Overflow},
	{"emax/inexact lands on MaxExp", dec64, false, "12345678901234567", 368, false, bid.ToNearestEven, "1.234567890123457E+384", bid.Inexact},
	{"emax/inexact one past MaxExp", dec64, false, "12345678901234567", 369, false, bid.ToNearestEven, "Infinity", bid.Inexact | bid.Overflow},
	{"emax/far exponent", dec64, false, "1", math.MaxInt, false, bid.TowardZero, "9.999999999999999E+384", bid.Inexact | bid.Overflow},
	// Emin minus the precision: exponent MinExp - Precision, where a
	// coefficient of exactly Precision+1 digits is an exact shift.
	{"emin-prec/one digit", dec64, false, "1", -414, false, bid.ToNearestEven, "0E-398", bid.Inexact | bid.Underflow},
	{"emin-prec/one digit up", dec64, false, "1", -414, false, bid.TowardPositive, "1E-398", bid.Inexact | bid.Underflow},
	{"emin-prec/exact shift", dec64, false, "10000000000000000", -414, false, bid.ToNearestEven, "1E-398", bid.Exact},
	{"emin-prec/exact shift to three digits", dec64, false, "10000000000000000000", -414, false, bid.ToNearestEven, "1.000E-395", bid.Exact},
	{"emin-prec/inexact shift", dec64, false, "10000000000000001", -414, false, bid.ToNearestEven, "1E-398", bid.Inexact | bid.Underflow},
	{"emin-prec/inexact shift up", dec64, false, "10000000000000001", -414, false, bid.TowardPositive, "2E-398", bid.Inexact | bid.Underflow},
	{"emin-prec/half of the smallest", dec64, false, "50000000000000000", -415, false, bid.ToNearestEven, "0E-398", bid.Inexact | bid.Underflow},
	{"emin-prec/half of the smallest away", dec64, false, "50000000000000000", -415, false, bid.ToNearestAway, "1E-398", bid.Inexact | bid.Underflow},
	{"emin-prec/far exponent", dec64, false, "1", math.MinInt, false, bid.TowardPositive, "1E-398", bid.Inexact | bid.Underflow},
	{"emin-prec/far exponent even", dec64, false, "18446744073709551615", math.MinInt, false, bid.ToNearestEven, "0E-398", bid.Inexact | bid.Underflow},
	// Ties in each mode, both signs.
	{"tie/even", dec64, false, "12345678901234565", 0, false, bid.ToNearestEven, "1.234567890123456E+16", bid.Inexact},
	{"tie/away", dec64, false, "12345678901234565", 0, false, bid.ToNearestAway, "1.234567890123457E+16", bid.Inexact},
	{"tie/zero", dec64, false, "12345678901234565", 0, false, bid.TowardZero, "1.234567890123456E+16", bid.Inexact},
	{"tie/up", dec64, false, "12345678901234565", 0, false, bid.TowardPositive, "1.234567890123457E+16", bid.Inexact},
	{"tie/down", dec64, false, "12345678901234565", 0, false, bid.TowardNegative, "1.234567890123456E+16", bid.Inexact},
	{"tie/even negative", dec64, true, "12345678901234565", 0, false, bid.ToNearestEven, "-1.234567890123456E+16", bid.Inexact},
	{"tie/away negative", dec64, true, "12345678901234565", 0, false, bid.ToNearestAway, "-1.234567890123457E+16", bid.Inexact},
	{"tie/zero negative", dec64, true, "12345678901234565", 0, false, bid.TowardZero, "-1.234567890123456E+16", bid.Inexact},
	{"tie/up negative", dec64, true, "12345678901234565", 0, false, bid.TowardPositive, "-1.234567890123456E+16", bid.Inexact},
	{"tie/down negative", dec64, true, "12345678901234565", 0, false, bid.TowardNegative, "-1.234567890123457E+16", bid.Inexact},
	{"tie/even to odd neighbour above", dec64, false, "12345678901234575", 0, false, bid.ToNearestEven, "1.234567890123458E+16", bid.Inexact},
	{"tie/broken by sticky", dec64, false, "12345678901234565", 0, true, bid.ToNearestEven, "1.234567890123457E+16", bid.Inexact},
	{"tie/two digits dropped", dec64, false, "1234567890123456500", 0, false, bid.ToNearestEven, "1.234567890123456E+18", bid.Inexact},
	{"tie/two digits dropped above", dec64, false, "1234567890123456501", 0, false, bid.ToNearestEven, "1.234567890123457E+18", bid.Inexact},
	// Subnormal ties.
	{"subnormal tie/even up", dec64, false, "15", -399, false, bid.ToNearestEven, "2E-398", bid.Inexact | bid.Underflow},
	{"subnormal tie/even down", dec64, false, "25", -399, false, bid.ToNearestEven, "2E-398", bid.Inexact | bid.Underflow},
	{"subnormal tie/away", dec64, false, "25", -399, false, bid.ToNearestAway, "3E-398", bid.Inexact | bid.Underflow},
	{"subnormal tie/zero", dec64, false, "25", -399, false, bid.TowardZero, "2E-398", bid.Inexact | bid.Underflow},
	{"subnormal tie/up", dec64, false, "25", -399, false, bid.TowardPositive, "3E-398", bid.Inexact | bid.Underflow},
	{"subnormal tie/down", dec64, false, "25", -399, false, bid.TowardNegative, "2E-398", bid.Inexact | bid.Underflow},
	{"subnormal tie/down negative", dec64, true, "25", -399, false, bid.TowardNegative, "-3E-398", bid.Inexact | bid.Underflow},
	{"subnormal tie/smallest even", dec64, false, "5", -399, false, bid.ToNearestEven, "0E-398", bid.Inexact | bid.Underflow},
	{"subnormal tie/smallest away", dec64, false, "5", -399, false, bid.ToNearestAway, "1E-398", bid.Inexact | bid.Underflow},
	{"subnormal/rounds up to normal", dec64, false, "9999999999999999", -399, false, bid.ToNearestEven, "1.000000000000000E-383", bid.Inexact | bid.Underflow},
	{"subnormal/truncates below normal", dec64, false, "9999999999999999", -399, false, bid.TowardZero, "9.99999999999999E-384", bid.Inexact | bid.Underflow},
	{"subnormal/exact", dec64, false, "123456789", -398, false, bid.ToNearestEven, "1.23456789E-390", bid.Exact},
	{"subnormal/exact at exp above MinExp", dec64, false, "5", -397, false, bid.ToNearestEven, "5E-397", bid.Exact},
	{"subnormal/sticky pads to MinExp", dec64, false, "5", -397, true, bid.TowardPositive, "5.1E-397", bid.Inexact | bid.Underflow},
	// A carry out of the top digit.
	{"carry/9999999999999999.5", dec64, false, "99999999999999995", -1, false, bid.ToNearestEven, "1.000000000000000E+16", bid.Inexact},
	{"carry/9999999999999999.5 away", dec64, false, "99999999999999995", -1, false, bid.ToNearestAway, "1.000000000000000E+16", bid.Inexact},
	{"carry/9999999999999999.5 zero", dec64, false, "99999999999999995", -1, false, bid.TowardZero, "9999999999999999", bid.Inexact},
	{"carry/9999999999999999.4 even", dec64, false, "99999999999999994", -1, false, bid.ToNearestEven, "9999999999999999", bid.Inexact},
	{"carry/sticky up", dec64, false, "9999999999999999", 0, true, bid.TowardPositive, "1.000000000000000E+16", bid.Inexact},
	{"carry/subnormal to full precision", dec64, false, "9999999999999995", -399, false, bid.ToNearestEven, "1.000000000000000E-383", bid.Inexact | bid.Underflow},
	// The overflow that a tie produces: 9999999999999999E369 + 5E353.
	{"overflow at tie/even", dec64, false, "99999999999999995", 368, false, bid.ToNearestEven, "Infinity", bid.Inexact | bid.Overflow},
	{"overflow at tie/away", dec64, false, "99999999999999995", 368, false, bid.ToNearestAway, "Infinity", bid.Inexact | bid.Overflow},
	{"overflow at tie/zero", dec64, false, "99999999999999995", 368, false, bid.TowardZero, "9.999999999999999E+384", bid.Inexact},
	{"overflow at tie/up", dec64, false, "99999999999999995", 368, false, bid.TowardPositive, "Infinity", bid.Inexact | bid.Overflow},
	{"overflow at tie/down", dec64, false, "99999999999999995", 368, false, bid.TowardNegative, "9.999999999999999E+384", bid.Inexact},
	{"overflow at tie/even negative", dec64, true, "99999999999999995", 368, false, bid.ToNearestEven, "-Infinity", bid.Inexact | bid.Overflow},
	{"overflow at tie/up negative", dec64, true, "99999999999999995", 368, false, bid.TowardPositive, "-9.999999999999999E+384", bid.Inexact},
	{"overflow at tie/below tie", dec64, false, "99999999999999994", 368, false, bid.ToNearestEven, "9.999999999999999E+384", bid.Inexact},
	{"overflow at tie/sticky", dec64, false, "9999999999999999", 369, true, bid.TowardPositive, "Infinity", bid.Inexact | bid.Overflow},
	// The sign of a zero: exact zeros keep it; inexact results that round
	// to zero keep the sign of the exact value in every mode.
	{"zero sign/exact negative", dec64, true, "0", 5, false, bid.TowardPositive, "-0E+5", bid.Exact},
	{"zero sign/exact negative down", dec64, true, "0", 5, false, bid.TowardNegative, "-0E+5", bid.Exact},
	{"zero sign/exact positive down", dec64, false, "0", 5, false, bid.TowardNegative, "0E+5", bid.Exact},
	{"zero sign/negative tiny up", dec64, true, "1", -399, false, bid.TowardPositive, "-0E-398", bid.Inexact | bid.Underflow},
	{"zero sign/negative tiny down", dec64, true, "1", -399, false, bid.TowardNegative, "-1E-398", bid.Inexact | bid.Underflow},
	{"zero sign/negative tiny zero", dec64, true, "1", -399, false, bid.TowardZero, "-0E-398", bid.Inexact | bid.Underflow},
	{"zero sign/negative tiny even", dec64, true, "1", -399, false, bid.ToNearestEven, "-0E-398", bid.Inexact | bid.Underflow},
	{"zero sign/positive tiny down", dec64, false, "1", -399, false, bid.TowardNegative, "0E-398", bid.Inexact | bid.Underflow},
	{"zero sign/negative sticky zero up", dec64, true, "0", 0, true, bid.TowardPositive, "-0E-398", bid.Inexact | bid.Underflow},
	{"zero sign/negative sticky zero down", dec64, true, "0", 0, true, bid.TowardNegative, "-1E-398", bid.Inexact | bid.Underflow},
	{"zero sign/sticky zero at huge exponent", dec64, false, "0", 1000, true, bid.TowardPositive, "1E-398", bid.Inexact | bid.Underflow},
	{"zero sign/sticky zero at huge exponent even", dec64, false, "0", 1000, true, bid.ToNearestEven, "0E-398", bid.Inexact | bid.Underflow},
	// Exponent of exact zeros: clamped into the format.
	{"zero exp/above", dec64, false, "0", 400, false, bid.ToNearestEven, "0E+369", bid.Exact},
	{"zero exp/below", dec64, true, "0", -500, false, bid.ToNearestEven, "-0E-398", bid.Exact},
	{"zero exp/max int", dec64, false, "0", math.MaxInt, false, bid.ToNearestEven, "0E+369", bid.Exact},
	{"zero exp/min int", dec64, false, "0", math.MinInt, false, bid.ToNearestEven, "0E-398", bid.Exact},
	// Sticky with no digit discarded: the arbitrarily small fraction.
	{"sticky/even pads", dec64, false, "1", 0, true, bid.ToNearestEven, "1.000000000000000", bid.Inexact},
	{"sticky/up moves", dec64, false, "1", 0, true, bid.TowardPositive, "1.000000000000001", bid.Inexact},
	{"sticky/down negative moves", dec64, true, "1", 0, true, bid.TowardNegative, "-1.000000000000001", bid.Inexact},
	{"sticky/full precision up", dec64, false, "1234567890123456", 0, true, bid.TowardPositive, "1234567890123457", bid.Inexact},
	// A 64-bit coefficient wider than the precision.
	{"wide/max uint64 even", dec64, false, "18446744073709551615", 0, false, bid.ToNearestEven, "1.844674407370955E+19", bid.Inexact},
	{"wide/max uint64 up", dec64, false, "18446744073709551615", 0, false, bid.TowardPositive, "1.844674407370956E+19", bid.Inexact},
	// decimal128 at the same edges.
	{"128/emax fold-down room", dec128, false, "1", 6112, false, bid.ToNearestEven, "1.0E+6112", bid.Exact},
	{"128/emax fold-down fills precision", dec128, false, "1", 6144, false, bid.ToNearestEven, "1.000000000000000000000000000000000E+6144", bid.Exact},
	{"128/emax one beyond", dec128, false, "1", 6145, false, bid.ToNearestEven, "Infinity", bid.Inexact | bid.Overflow},
	{"128/emax one beyond zero", dec128, false, "1", 6145, false, bid.TowardZero, "9.999999999999999999999999999999999E+6144", bid.Inexact | bid.Overflow},
	{"128/emax no room", dec128, false, "9999999999999999999999999999999999", 6112, false, bid.TowardNegative, "9.999999999999999999999999999999999E+6144", bid.Inexact | bid.Overflow},
	{"128/emin-prec one digit", dec128, false, "1", -6210, false, bid.ToNearestEven, "0E-6176", bid.Inexact | bid.Underflow},
	{"128/emin-prec one digit up", dec128, false, "1", -6210, false, bid.TowardPositive, "1E-6176", bid.Inexact | bid.Underflow},
	{"128/emin-prec exact shift", dec128, false, "10000000000000000000000000000000000", -6210, false, bid.ToNearestEven, "1E-6176", bid.Exact},
	{"128/tie even", dec128, false, "12345678901234567890123456789012345", 0, false, bid.ToNearestEven, "1.234567890123456789012345678901234E+34", bid.Inexact},
	{"128/tie away", dec128, false, "12345678901234567890123456789012345", 0, false, bid.ToNearestAway, "1.234567890123456789012345678901235E+34", bid.Inexact},
	{"128/tie down negative", dec128, true, "12345678901234567890123456789012345", 0, false, bid.TowardNegative, "-1.234567890123456789012345678901235E+34", bid.Inexact},
	{"128/carry", dec128, false, "99999999999999999999999999999999995", -1, false, bid.ToNearestEven, "1.000000000000000000000000000000000E+34", bid.Inexact},
	{"128/overflow at tie even", dec128, false, "99999999999999999999999999999999995", 6110, false, bid.ToNearestEven, "Infinity", bid.Inexact | bid.Overflow},
	{"128/overflow at tie zero", dec128, false, "99999999999999999999999999999999995", 6110, false, bid.TowardZero, "9.999999999999999999999999999999999E+6144", bid.Inexact},
	{"128/subnormal tie", dec128, false, "25", -6177, false, bid.ToNearestEven, "2E-6176", bid.Inexact | bid.Underflow},
	{"128/subnormal tie away", dec128, false, "25", -6177, false, bid.ToNearestAway, "3E-6176", bid.Inexact | bid.Underflow},
	{"128/negative tiny up", dec128, true, "1", -6177, false, bid.TowardPositive, "-0E-6176", bid.Inexact | bid.Underflow},
	{"128/full 128-bit coefficient", dec128, false, "340282366920938463463374607431768211455", 0, false, bid.ToNearestEven, "3.402823669209384634633746074317682E+38", bid.Inexact},
	{"128/full 128-bit coefficient up", dec128, false, "340282366920938463463374607431768211455", 0, false, bid.TowardPositive, "3.402823669209384634633746074317683E+38", bid.Inexact},
	{"128/zero exp clamp", dec128, true, "0", -7000, false, bid.ToNearestEven, "-0E-6176", bid.Exact},
}

func roundCoeff(t testing.TB, digits string) bid.Uint128 {
	c, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		t.Fatalf("bad coefficient %q", digits)
	}
	return toUint128(c)
}

func TestRoundBoundaries(t *testing.T) {
	for _, tt := range roundBoundaryTests {
		t.Run(tt.f.name+"/"+tt.name, func(t *testing.T) {
			c := roundCoeff(t, tt.coeff)
			got, st := kernel{}.round(tt.f, tt.neg, c, tt.exp, tt.sticky, tt.mode)
			if s := display(tt.f, got); s != tt.want || st != tt.status {
				t.Errorf("Round(%t, %s, %d, %t, %s) = %s, %v; want %s, %v", tt.neg, tt.coeff, tt.exp, tt.sticky, modeName(tt.mode), s, st, tt.want, tt.status)
			}
			ref, rst := reference{}.round(tt.f, tt.neg, c, tt.exp, tt.sticky, tt.mode)
			if ref != got || rst != st {
				t.Errorf("Round(%t, %s, %d, %t, %s): kernel %s, %v; reference %s, %v", tt.neg, tt.coeff, tt.exp, tt.sticky, modeName(tt.mode), display(tt.f, got), st, display(tt.f, ref), rst)
			}
			if tt.f == dec64 && !bid.IsCanonical64(got.Lo) || tt.f == dec128 && !bid.IsCanonical128(got) {
				t.Errorf("Round(%t, %s, %d, %t, %s) = %#x is not canonical", tt.neg, tt.coeff, tt.exp, tt.sticky, modeName(tt.mode), got)
			}
		})
	}
}

// TestRoundAllModesAgainstReference runs every boundary coefficient and
// exponent under all five modes and both signs, with and without the
// sticky bit, against the reference: the table above fixes one mode per
// line, and this covers the rest.
func TestRoundAllModesAgainstReference(t *testing.T) {
	modes := []bid.Mode{bid.ToNearestEven, bid.ToNearestAway, bid.TowardZero, bid.TowardPositive, bid.TowardNegative, bid.Mode(7)}
	for _, tt := range roundBoundaryTests {
		c := roundCoeff(t, tt.coeff)
		for _, neg := range []bool{false, true} {
			for _, sticky := range []bool{false, true} {
				for _, m := range modes {
					got, st := kernel{}.round(tt.f, neg, c, tt.exp, sticky, m)
					ref, rst := reference{}.round(tt.f, neg, c, tt.exp, sticky, m)
					if got != ref || st != rst {
						t.Errorf("%s: Round(%t, %s, %d, %t, %s): kernel %s, %v; reference %s, %v", tt.f.name, neg, tt.coeff, tt.exp, sticky, modeName(m), display(tt.f, got), st, display(tt.f, ref), rst)
					}
				}
			}
		}
	}
}

func TestRoundAllocs(t *testing.T) {
	var sink64 uint64
	var sink128 bid.Uint128
	var sinkSt bid.Status
	fn := func() {
		var st bid.Status
		sink64, st = bid.Round64(false, 1234567890123456, 0, false, bid.ToNearestEven)
		sinkSt |= st
		sink64, st = bid.Round64(true, 12345678901234567, -400, true, bid.TowardNegative)
		sinkSt |= st
		sink64, st = bid.Round64(false, 1, 400, false, bid.ToNearestEven)
		sinkSt |= st
		sink128, st = bid.Round128(false, bid.Uint128{Hi: 1, Lo: 1}, 0, false, bid.ToNearestEven)
		sinkSt |= st
		sink128, st = bid.Round128(true, bid.Uint128{Lo: 5}, -6177, true, bid.TowardPositive)
		sinkSt |= st
		sink128, st = bid.Round128(false, bid.Uint128{Lo: 1}, 7000, false, bid.TowardZero)
		sinkSt |= st
	}
	if n := testing.AllocsPerRun(100, fn); n != 0 {
		t.Errorf("Round64/Round128 allocate: %v allocs per run", n)
	}
	_, _, _ = sink64, sink128, sinkSt
}

func FuzzRound64(f *testing.F) {
	f.Add(false, uint64(1), int64(0), false, uint8(0))
	f.Add(false, uint64(99999999999999995), int64(368), false, uint8(0))
	f.Add(true, uint64(25), int64(-399), false, uint8(1))
	f.Add(false, uint64(1), int64(385), false, uint8(2))
	f.Add(true, uint64(0), int64(0), true, uint8(3))
	f.Add(false, uint64(math.MaxUint64), int64(math.MinInt64), true, uint8(4))
	f.Fuzz(func(t *testing.T, neg bool, coeff uint64, exp int64, sticky bool, mode uint8) {
		m := bid.Mode(mode)
		got, st := bid.Round64(neg, coeff, int(exp), sticky, m)
		want, wst := reference{}.round(dec64, neg, bid.Uint128{Lo: coeff}, int(exp), sticky, m)
		if got != want.Lo || st != wst {
			t.Errorf("Round64(%t, %d, %d, %t, %s) = %s, %v; reference %s, %v", neg, coeff, exp, sticky, modeName(m), display(dec64, lo(got)), st, display(dec64, want), wst)
		}
	})
}

func FuzzRound128(f *testing.F) {
	f.Add(false, uint64(0), uint64(1), int64(0), false, uint8(0))
	f.Add(false, uint64(0x4ee2d6d415b85ac), uint64(0xef8100000000000b), int64(6110), false, uint8(0))
	f.Add(true, uint64(0), uint64(25), int64(-6177), false, uint8(1))
	f.Add(false, uint64(0), uint64(1), int64(6145), false, uint8(2))
	f.Add(true, uint64(0), uint64(0), int64(0), true, uint8(3))
	f.Add(false, uint64(math.MaxUint64), uint64(math.MaxUint64), int64(math.MaxInt64), true, uint8(4))
	f.Fuzz(func(t *testing.T, neg bool, hi, lo uint64, exp int64, sticky bool, mode uint8) {
		m := bid.Mode(mode)
		c := bid.Uint128{Hi: hi, Lo: lo}
		got, st := bid.Round128(neg, c, int(exp), sticky, m)
		want, wst := reference{}.round(dec128, neg, c, int(exp), sticky, m)
		if got != want || st != wst {
			t.Errorf("Round128(%t, %s, %d, %t, %s) = %s, %v; reference %s, %v", neg, fromUint128(c), exp, sticky, modeName(m), display(dec128, got), st, display(dec128, want), wst)
		}
	})
}

var roundBench = []struct {
	name   string
	coeff  bid.Uint128
	exp    int
	sticky bool
}{
	{"exact16", bid.Uint128{Lo: 1234567890123456}, -5, false},
	{"inexact17", bid.Uint128{Lo: 12345678901234567}, -5, false},
	{"wide19", bid.Uint128{Lo: 1234567890123456789}, -5, true},
	{"subnormal", bid.Uint128{Lo: 12345678901234567}, -410, false},
}

func BenchmarkRound64(b *testing.B) {
	for _, bb := range roundBench {
		b.Run(bb.name, func(b *testing.B) {
			var sink uint64
			for i := 0; i < b.N; i++ {
				x, _ := bid.Round64(i&1 != 0, bb.coeff.Lo, bb.exp, bb.sticky, bid.Mode(i%5))
				sink ^= x
			}
			_ = sink
		})
	}
}

var roundBench128 = []struct {
	name   string
	coeff  string
	exp    int
	sticky bool
}{
	{"exact34", "1234567890123456789012345678901234", -5, false},
	{"inexact35", "12345678901234567890123456789012345", -5, false},
	{"wide39", "340282366920938463463374607431768211455", -5, true},
	{"subnormal", "12345678901234567890123456789012345", -6190, false},
}

func BenchmarkRound128(b *testing.B) {
	for _, bb := range roundBench128 {
		c := roundCoeff(b, bb.coeff)
		b.Run(bb.name, func(b *testing.B) {
			var sink bid.Uint128
			for i := 0; i < b.N; i++ {
				x, _ := bid.Round128(i&1 != 0, c, bb.exp, bb.sticky, bid.Mode(i%5))
				sink.Lo ^= x.Lo
				sink.Hi ^= x.Hi
			}
			_ = sink
		})
	}
}

// TestRoundIsTheOnlyPacker enforces the fourth criterion of the rounding
// routine by reading the package source: the raw encoders Pack64, Pack128
// and PackW (in the template) are called only by packW and its
// instantiations; those are called only by roundW and overflowW and their
// instantiations; and the field layout constants that would let a function
// assemble an encoding by hand are referenced only in encoding.go. The
// exact fast paths that roundW's comment enumerates are the allowlist
// here; a new one is added to both or the test fails.
func TestRoundIsTheOnlyPacker(t *testing.T) {
	// Callers of the raw encoder.
	rawPackers := map[string]bool{"packW": true, "pack64": true, "pack128": true}
	// Callers of packW, the checked encoder: the rounding routine, its
	// overflow result, and the exact fast paths (none at present).
	checkedPackers := map[string]bool{
		"roundW": true, "round64": true, "round128": true,
		"overflowW": true, "overflow64": true, "overflow128": true,
	}
	rawEncoder := map[string]bool{"Pack64": true, "Pack128": true, "PackW": true}
	checkedEncoder := map[string]bool{"packW": true, "pack64": true, "pack128": true}
	layout := map[string]bool{
		"ExpShift64": true, "ExpShift128": true, "Form2ExpShift64": true, "Form2ExpShift128": true,
		"Form2Mask64": true, "Form2Mask128Hi": true, "CoeffBits64": true, "Form2CoeffBits64": true,
		"Form2CoeffBits128": true, "CoeffMask128Hi": true, "Form2CoeffMask128Hi": true,
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	fset := token.NewFileSet()
	var violations []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				// Package-level declarations: the layout constants are
				// declared in encoding.go and may be used by its own
				// declarations (Zero64 is Bias64 << ExpShift64).
				if name != "encoding.go" {
					ast.Inspect(decl, func(n ast.Node) bool {
						if id, ok := n.(*ast.Ident); ok && layout[id.Name] {
							violations = append(violations, fset.Position(id.Pos()).String()+": "+id.Name+" referenced outside encoding.go")
						}
						return true
					})
				}
				continue
			}
			caller := fn.Name.Name
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CallExpr:
					id, ok := n.Fun.(*ast.Ident)
					if !ok {
						return true
					}
					switch {
					case rawEncoder[id.Name] && !rawPackers[caller]:
						violations = append(violations, fset.Position(n.Pos()).String()+": "+caller+" calls "+id.Name)
					case checkedEncoder[id.Name] && !checkedPackers[caller]:
						violations = append(violations, fset.Position(n.Pos()).String()+": "+caller+" calls "+id.Name)
					}
				case *ast.Ident:
					if layout[n.Name] && name != "encoding.go" {
						violations = append(violations, fset.Position(n.Pos()).String()+": "+caller+" references "+n.Name)
					}
				}
				return true
			})
		}
	}
	for _, v := range violations {
		t.Error(v)
	}
	if len(violations) > 0 {
		t.Errorf("%d call(s) reach the encoder outside the rounding routine; see roundW in arith_tmpl.go for the exact fast paths allowed", len(violations))
	}
}
