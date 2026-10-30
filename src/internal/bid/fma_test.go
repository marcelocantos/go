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

// fmaTests are the cases where a fused multiply-add can go wrong: a result
// that a rounded product would change, a tail that only a sticky bit carries, a
// borrow from that tail, the first gap that no longer scales, a zero on either
// side, and a sum back in range from a product that is not. Each want was
// worked out by hand from the package documentation and is checked against the
// kernel and the reference.
var fmaTests = []struct {
	name    string
	f       *format
	x, y, z string
	mode    bid.Mode
	want    string
}{
	// The rounding error of a product: all that is left of it. (10^16-1)^2 is
	// 9999999999999998|0000000000000001.
	{"cancel/error", dec64, "9999999999999999", "9999999999999999", "-9999999999999998E+16", bid.ToNearestEven, "1"},
	{"cancel/error negative", dec64, "-9999999999999999", "9999999999999999", "9999999999999998E+16", bid.ToNearestEven, "-1"},
	{"cancel/error 128", dec128, "9999999999999999999999999999999999", "9999999999999999999999999999999999", "-9999999999999999999999999999999998E+34", bid.ToNearestEven, "1"},
	{"cancel/contract", dec64, "1.000000000000001", "1.000000000000001", "-1", bid.ToNearestEven, "2.000000000000001E-15"},

	// An exact cancellation: the sign by mode, the preferred exponent.
	{"cancel/even", dec64, "1E+8", "1E+8", "-1E+16", bid.ToNearestEven, "0E+16"},
	{"cancel/away", dec64, "1E+8", "1E+8", "-1E+16", bid.ToNearestAway, "0E+16"},
	{"cancel/zero", dec64, "1E+8", "1E+8", "-1E+16", bid.TowardZero, "0E+16"},
	{"cancel/up", dec64, "1E+8", "1E+8", "-1E+16", bid.TowardPositive, "0E+16"},
	{"cancel/down", dec64, "1E+8", "1E+8", "-1E+16", bid.TowardNegative, "-0E+16"},
	{"cancel/negative product down", dec64, "-1E+8", "1E+8", "1E+16", bid.TowardNegative, "-0E+16"},
	{"cancel/negative product up", dec64, "-1E+8", "1E+8", "1E+16", bid.TowardPositive, "0E+16"},
	{"cancel/smaller product exponent", dec64, "1.5", "2", "-3", bid.ToNearestEven, "0.0"},
	{"cancel/smaller z exponent", dec64, "2.0", "3", "-6.00", bid.ToNearestEven, "0.00"},
	{"cancel/128 down", dec128, "1E+8", "1E+8", "-1E+16", bid.TowardNegative, "-0E+16"},
	{"cancel/at MinExp", dec64, "1E-199", "1E-199", "-1E-398", bid.ToNearestEven, "0E-398"},
	{"cancel/at MinExp down", dec64, "1E-199", "1E-199", "-1E-398", bid.TowardNegative, "-0E-398"},

	// A rounded product would decide these differently. The product of the
	// first ends in a tie at the digit below the 16 kept, and the addend, far
	// below it, breaks the tie.
	{"fused/tie alone even", dec64, "1000000000000001", "25", "0", bid.ToNearestEven, "2.500000000000002E+16"},
	{"fused/tie alone away", dec64, "1000000000000001", "25", "0", bid.ToNearestAway, "2.500000000000003E+16"},
	{"fused/tie broken up", dec64, "1000000000000001", "25", "1E-300", bid.ToNearestEven, "2.500000000000003E+16"},
	{"fused/tie broken down", dec64, "1000000000000001", "25", "-1E-300", bid.ToNearestEven, "2.500000000000002E+16"},
	{"fused/tie broken down away", dec64, "1000000000000001", "25", "-1E-300", bid.ToNearestAway, "2.500000000000002E+16"},
	{"fused/tail just below a tie", dec64, "1000000000000001", "4999999999999995E-46", "1", bid.ToNearestAway, "1.000000000000000"},
	{"fused/tail just above a tie", dec64, "-1000000000000001", "4999999999999995E-46", "1.000000000000001", bid.ToNearestEven, "1.000000000000001"},
	{"fused/tail above a tie, zero", dec64, "-1000000000000001", "4999999999999995E-46", "1.000000000000001", bid.TowardZero, "1.000000000000000"},
	{"fused/128 tail just below a tie", dec128, "1000000000000000000000000000000001", "4999999999999999999999999999999995E-100", "1", bid.ToNearestAway, "1.000000000000000000000000000000000"},
	{"fused/128 tail just above a tie", dec128, "-1000000000000000000000000000000001", "4999999999999999999999999999999995E-100", "1.000000000000000000000000000000001", bid.ToNearestEven, "1.000000000000000000000000000000001"},

	// The product far above the addend, which is a sticky bit and no more.
	{"above/up", dec64, "1E+200", "1E+100", "1E-398", bid.TowardPositive, "1.000000000000001E+300"},
	{"above/even", dec64, "1E+200", "1E+100", "1E-398", bid.ToNearestEven, "1.000000000000000E+300"},
	{"above/borrow zero", dec64, "1E+200", "1E+100", "-1E-398", bid.TowardZero, "9.999999999999999E+299"},
	{"above/borrow even", dec64, "1E+200", "1E+100", "-1E-398", bid.ToNearestEven, "1.000000000000000E+300"},
	{"above/not a power", dec64, "3", "1E+300", "-1E-398", bid.TowardZero, "2.999999999999999E+300"},
	{"above/negative", dec64, "-3", "1E+300", "1E-398", bid.TowardNegative, "-3.000000000000000E+300"},
	{"above/negative toward zero", dec64, "-3", "1E+300", "1E-398", bid.TowardZero, "-2.999999999999999E+300"},
	{"above/full product even", dec64, "9999999999999999", "9999999999999999", "1E-100", bid.ToNearestEven, "9.999999999999998E+31"},
	{"above/full product up", dec64, "9999999999999999", "9999999999999999", "1E-100", bid.TowardPositive, "9.999999999999999E+31"},
	{"above/128 up", dec128, "1E+3000", "1E+3000", "1E-6176", bid.TowardPositive, "1.000000000000000000000000000000001E+6000"},
	{"above/128 borrow", dec128, "1E+3000", "1E+3000", "-1E-6176", bid.TowardZero, "9.999999999999999999999999999999999E+5999"},

	// The addend far above the product.
	{"below/up", dec64, "1E-200", "1E-198", "1E+369", bid.TowardPositive, "1.000000000000001E+369"},
	{"below/even", dec64, "1E-200", "1E-198", "1E+369", bid.ToNearestEven, "1.000000000000000E+369"},
	{"below/borrow zero", dec64, "-1E-200", "1E-198", "1E+369", bid.TowardZero, "9.999999999999999E+368"},
	{"below/borrow even", dec64, "-1E-200", "1E-198", "1E+369", bid.ToNearestEven, "1.000000000000000E+369"},
	{"below/full product up", dec64, "9999999999999999E-398", "9999999999999999E-398", "1", bid.TowardPositive, "1.000000000000001"},
	{"below/full product down", dec64, "9999999999999999E-398", "9999999999999999E-398", "1", bid.TowardNegative, "1.000000000000000"},
	{"below/128 up", dec128, "1E-6176", "1E-6176", "1E+6111", bid.TowardPositive, "1.000000000000000000000000000000001E+6111"},
	{"below/128 borrow", dec128, "-1E-6176", "1E-6176", "1E+6111", bid.TowardZero, "9.999999999999999999999999999999999E+6110"},
	{"below/tie exact", dec64, "5E-8", "1E-8", "1", bid.ToNearestEven, "1.000000000000000"},
	{"below/tie exact away", dec64, "5E-8", "1E-8", "1", bid.ToNearestAway, "1.000000000000001"},
	{"below/above tie exact", dec64, "5000000000000001E-23", "1E-8", "1", bid.ToNearestEven, "1.000000000000001"},

	// The first gap that no longer scales: 35 digits for decimal64, from one
	// digit in either direction and from a whole product.
	{"edge/z scaled", dec64, "1", "1", "1E+34", bid.TowardPositive, "1.000000000000001E+34"},
	{"edge/z shifted", dec64, "1", "1", "1E+35", bid.TowardPositive, "1.000000000000001E+35"},
	{"edge/z shifted even", dec64, "1", "1", "1E+35", bid.ToNearestEven, "1.000000000000000E+35"},
	{"edge/product scaled", dec64, "1E+17", "1E+17", "1", bid.TowardPositive, "1.000000000000001E+34"},
	{"edge/product shifted", dec64, "1E+17", "1E+18", "1", bid.TowardPositive, "1.000000000000001E+35"},
	{"edge/whole product", dec64, "9999999999999999E+2", "9999999999999999E+2", "1", bid.TowardPositive, "9.999999999999999E+35"},
	{"edge/whole product even", dec64, "9999999999999999E+2", "9999999999999999E+2", "1", bid.ToNearestEven, "9.999999999999998E+35"},
	{"edge/whole product borrow", dec64, "9999999999999999E+2", "9999999999999999E+2", "-1", bid.TowardZero, "9.999999999999998E+35"},
	{"edge/128 z scaled", dec128, "1", "1", "1E+70", bid.TowardPositive, "1.000000000000000000000000000000001E+70"},
	{"edge/128 z shifted", dec128, "1", "1", "1E+71", bid.TowardPositive, "1.000000000000000000000000000000001E+71"},

	// A zero product: the sum is z, at the preferred exponent or as near it as
	// the cohort goes.
	{"zero product/z exponent", dec64, "0", "5", "7.50", bid.ToNearestEven, "7.50"},
	{"zero product/larger exponent", dec64, "0E+10", "5", "7", bid.ToNearestEven, "7"},
	{"zero product/smaller exponent", dec64, "0E-10", "5", "7", bid.ToNearestEven, "7.0000000000"},
	{"zero product/far smaller", dec64, "0E-300", "5", "7", bid.ToNearestEven, "7.000000000000000"},
	{"zero product/far smaller 128", dec128, "0E-6000", "5", "7", bid.ToNearestEven, "7.000000000000000000000000000000000"},
	{"zero product/out of range", dec64, "0E-398", "1E-398", "1E+369", bid.ToNearestEven, "1.000000000000000E+369"},
	{"zero product/a whole z", dec64, "0E+300", "1", "1234567890123456E-398", bid.ToNearestEven, "1.234567890123456E-383"},
	{"zero product/opposite sign", dec64, "-0", "5", "7", bid.ToNearestEven, "7"},

	// Two zeros: the sign of a sum, the exponent clamped.
	{"zeros/like", dec64, "-0", "5", "-0", bid.ToNearestEven, "-0"},
	{"zeros/like positive down", dec64, "0", "5", "0", bid.TowardNegative, "0"},
	{"zeros/unlike", dec64, "-0", "5", "0", bid.ToNearestEven, "0"},
	{"zeros/unlike down", dec64, "0", "5", "-0", bid.TowardNegative, "-0"},
	{"zeros/both negative factors", dec64, "-0", "-5", "-0", bid.ToNearestEven, "0"},
	{"zeros/smaller of the exponents", dec64, "-0E+3", "5", "-0E+1", bid.ToNearestEven, "-0E+1"},
	{"zeros/product smaller", dec64, "0E-3", "5", "0E+1", bid.ToNearestEven, "0.000"},
	{"zeros/clamped high", dec64, "0E+369", "1E+369", "0E+369", bid.ToNearestEven, "0E+369"},
	{"zeros/clamped low", dec64, "0E-398", "1E-398", "0", bid.ToNearestEven, "0E-398"},
	{"zeros/128 clamped low", dec128, "0E-6176", "1E-6176", "0", bid.ToNearestEven, "0E-6176"},

	// A zero addend: the product, rounded or scaled to the preferred exponent.
	{"zero z/same exponent", dec64, "2", "3", "0", bid.ToNearestEven, "6"},
	{"zero z/smaller exponent", dec64, "2", "3", "0.00", bid.ToNearestEven, "6.00"},
	{"zero z/larger exponent", dec64, "2", "3", "0E+5", bid.ToNearestEven, "6"},
	{"zero z/far smaller", dec64, "2", "3", "0E-300", bid.ToNearestEven, "6.000000000000000"},
	{"zero z/negative", dec64, "2", "3", "-0", bid.TowardNegative, "6"},
	{"zero z/negative product", dec64, "-2", "3", "0", bid.ToNearestEven, "-6"},
	{"zero z/inexact", dec64, "1234567890123456", "11", "0", bid.ToNearestEven, "1.358024679135802E+16"},
	{"zero z/inexact zero", dec64, "1234567890123456", "11", "0", bid.TowardZero, "1.358024679135801E+16"},
	{"zero z/overflow", dec64, "1E+200", "1E+200", "0", bid.ToNearestEven, "Infinity"},

	// The product alone is outside the format; only the sum says whether the
	// result is.
	{"range/overflow", dec64, "1E+200", "1E+200", "-1E+300", bid.ToNearestEven, "Infinity"},
	{"range/overflow zero", dec64, "1E+200", "1E+200", "-1E+300", bid.TowardZero, "9.999999999999999E+384"},
	{"range/back in range", dec64, "1E+192", "1E+193", "-9.999999999999999E+384", bid.ToNearestEven, "1E+369"},
	{"range/128 back in range", dec128, "1E+3000", "1E+3145", "-9.999999999999999999999999999999999E+6144", bid.ToNearestEven, "1E+6111"},
	{"range/large sum", dec64, "1E+192", "1E+192", "-9.999999999999999E+384", bid.ToNearestEven, "-8.999999999999999E+384"},
	{"range/fold-down", dec64, "1E+185", "1E+185", "0E+369", bid.ToNearestEven, "1.0E+370"},
	{"range/tiny sum", dec64, "1E-200", "1E-199", "1E-398", bid.ToNearestEven, "1E-398"},
	{"range/tiny sum up", dec64, "1E-200", "1E-199", "1E-398", bid.TowardPositive, "2E-398"},
	{"range/tiny difference", dec64, "-1E-200", "1E-199", "1E-398", bid.ToNearestEven, "1E-398"},
	{"range/tiny difference zero", dec64, "-1E-200", "1E-199", "1E-398", bid.TowardZero, "0E-398"},
	{"range/far below subnormal even", dec64, "-1E-398", "1E-398", "1E-398", bid.ToNearestEven, "1E-398"},
	{"range/far below subnormal down", dec64, "-1E-398", "1E-398", "1E-398", bid.TowardNegative, "0E-398"},
	{"range/product underflows alone", dec64, "1E-398", "1E-398", "0", bid.ToNearestEven, "0E-398"},
	{"range/product underflows up", dec64, "1E-398", "1E-398", "0", bid.TowardPositive, "1E-398"},
	{"range/product underflows negative", dec64, "-1E-398", "1E-398", "0", bid.ToNearestEven, "-0E-398"},
	{"range/128 underflows", dec128, "1E-6176", "1E-6176", "0", bid.TowardPositive, "1E-6176"},

	// Invalid and infinite cases.
	{"inv/zero × ∞", dec64, "0", "Infinity", "1", bid.ToNearestEven, "NaN"},
	{"inv/∞ × zero", dec64, "Infinity", "0E+5", "1", bid.ToNearestEven, "NaN"},
	{"inv/∞ × zero × NaN", dec64, "Infinity", "0", "NaN", bid.ToNearestEven, "NaN"},
	{"inv/zero × ∞ + ∞", dec128, "-0", "Infinity", "Infinity", bid.ToNearestEven, "NaN"},
	{"inv/cancel", dec64, "Infinity", "1", "-Infinity", bid.ToNearestEven, "NaN"},
	{"inv/cancel negative", dec128, "Infinity", "-1", "Infinity", bid.TowardNegative, "NaN"},
	{"inv/cancel two infinities", dec64, "-Infinity", "-Infinity", "-Infinity", bid.ToNearestEven, "NaN"},
	{"inf/same", dec64, "Infinity", "-1", "-Infinity", bid.ToNearestEven, "-Infinity"},
	{"inf/finite z", dec128, "-Infinity", "Infinity", "1E+300", bid.ToNearestEven, "-Infinity"},
	{"inf/z", dec64, "1E+200", "1E+200", "-Infinity", bid.ToNearestEven, "-Infinity"},
	{"inf/z with zero product", dec64, "0", "1", "Infinity", bid.ToNearestEven, "Infinity"},
	{"nan/x", dec64, "NaN", "1", "1", bid.ToNearestEven, "NaN"},
	{"nan/y", dec128, "1", "sNaN", "1", bid.ToNearestEven, "NaN"},
	{"nan/z", dec64, "1", "1", "-NaN", bid.ToNearestEven, "NaN"},
	{"nan/z with ∞", dec64, "Infinity", "1", "NaN", bid.ToNearestEven, "NaN"},
}

func TestFMADirected(t *testing.T) {
	for _, tt := range fmaTests {
		t.Run(tt.name, func(t *testing.T) {
			x, y, z := addOperand(t, tt.f, tt.x), addOperand(t, tt.f, tt.y), addOperand(t, tt.f, tt.z)
			want := addOperand(t, tt.f, tt.want)
			if got := (kernel{}).fma(tt.f, x, y, z, tt.mode); got != want {
				t.Errorf("FMA(%s, %s, %s, %s) = %s, want %s", tt.x, tt.y, tt.z, modeName(tt.mode), display(tt.f, got), tt.want)
			}
			if ref := (reference{}).fma(tt.f, x, y, z, tt.mode); ref != want {
				t.Errorf("reference FMA(%s, %s, %s, %s) = %s, want %s", tt.x, tt.y, tt.z, modeName(tt.mode), display(tt.f, ref), tt.want)
			}
		})
	}
}

// TestFMAAllModesAgainstReference runs every triple of fmaTests in every mode,
// whatever mode the table gave it.
func TestFMAAllModesAgainstReference(t *testing.T) {
	for _, tt := range fmaTests {
		x, y, z := addOperand(t, tt.f, tt.x), addOperand(t, tt.f, tt.y), addOperand(t, tt.f, tt.z)
		for _, m := range modes {
			if got, ref := (kernel{}).fma(tt.f, x, y, z, m), (reference{}).fma(tt.f, x, y, z, m); got != ref {
				t.Errorf("%s: FMA(%s, %s, %s, %s): kernel %s, reference %s", tt.name, tt.x, tt.y, tt.z, modeName(m), display(tt.f, got), display(tt.f, ref))
			}
		}
	}
}

// TestFMAGapSweep walks z through every alignment around the scaling limit,
// from each side, in every mode and with every sign, for coefficients chosen to
// make a difference: one digit, a whole format, a power of ten, and a run of
// nines that a borrow carries through. It is what a random draw reaches only by
// luck, since the limit is one exponent among thousands.
func TestFMAGapSweep(t *testing.T) {
	for _, f := range []*format{dec64, dec128} {
		nines := new(big.Int).Sub(pow10(f.prec), bigOne)
		half := new(big.Int).Mul(big.NewInt(5), pow10(f.prec-1))
		coeffs := []*big.Int{bigOne, big.NewInt(25), pow10(f.prec - 1), half, nines}
		limit := 2*f.prec + 6
		for _, xc := range coeffs {
			for _, yc := range coeffs {
				for _, zc := range coeffs {
					for gap := -limit; gap <= limit; gap++ {
						for _, neg := range []bool{false, true} {
							x := f.encode(value{kind: finite, coeff: xc, exp: 0})
							y := f.encode(value{kind: finite, coeff: yc, exp: 0})
							z := f.encode(value{kind: finite, neg: neg, coeff: zc, exp: gap})
							for _, m := range modes {
								if got, ref := (kernel{}).fma(f, x, y, z, m), (reference{}).fma(f, x, y, z, m); got != ref {
									t.Fatalf("%s: FMA(%s, %s, %s, %s): kernel %s, reference %s", f.name, display(f, x), display(f, y), display(f, z), modeName(m), display(f, got), display(f, ref))
								}
							}
						}
					}
				}
			}
		}
	}
}

// TestFMANonCanonical checks that a coefficient field above the largest reads
// as a zero of its sign and exponent, in each operand.
func TestFMANonCanonical(t *testing.T) {
	// The largest coefficient field that the second form can hold, above the
	// largest coefficient, at exponent 0.
	const bad64 = bid.Form2Mask64 | bid.Bias64<<bid.Form2ExpShift64 | (1<<bid.Form2CoeffBits64 - 1)
	one64, _ := bid.Pack64(false, 1, 0)
	seven64, _ := bid.Pack64(false, 7, 0)
	if got := bid.FMA64(bad64, one64, seven64, bid.ToNearestEven); got != seven64 {
		t.Errorf("FMA64(non-canonical, 1, 7) = %s, want 7", display(dec64, lo(got)))
	}
	if got := bid.FMA64(bad64, bid.Inf64, one64, bid.ToNearestEven); got != bid.NaN64 {
		t.Errorf("FMA64(non-canonical, Infinity, 1) = %s, want NaN", display(dec64, lo(got)))
	}
	if got, want := bid.FMA64(seven64, one64, bad64, bid.ToNearestEven), seven64; got != want {
		t.Errorf("FMA64(7, 1, non-canonical) = %s, want 7", display(dec64, lo(got)))
	}
}

func TestFMAAllocs(t *testing.T) {
	far64 := [3]uint64{addOperand(t, dec64, "1E+200").Lo, addOperand(t, dec64, "1E+100").Lo, addOperand(t, dec64, "-1.5E-300").Lo}
	far128 := [3]bid.Uint128{addOperand(t, dec128, "1E+3000"), addOperand(t, dec128, "1E+100"), addOperand(t, dec128, "-1.5E-3000")}
	near64, near128 := addOperand(t, dec64, "1.25").Lo, addOperand(t, dec128, "1.25")
	var sink64 uint64
	var sink128 bid.Uint128
	fn := func() {
		sink64 ^= bid.FMA64(far64[0], far64[1], far64[2], bid.TowardZero)
		sink64 ^= bid.FMA64(far64[2], far64[2], far64[0], bid.TowardZero)
		sink64 ^= bid.FMA64(near64, near64, near64, bid.ToNearestEven)
		sink128 = bid.FMA128(far128[0], far128[1], far128[2], bid.TowardZero)
		sink128 = bid.FMA128(far128[2], far128[2], far128[0], bid.TowardZero)
		sink128 = bid.FMA128(near128, near128, near128, bid.ToNearestEven)
	}
	if n := testing.AllocsPerRun(100, fn); n != 0 {
		t.Errorf("FMA allocates: %v allocs per run", n)
	}
	_, _ = sink64, sink128
}

// fmaFuzzOperands turns the raw inputs of a fuzz target into operands. Raw
// triples are nearly always far apart with a product out of range, so most
// shapes build finite operands from the raw bits and weight them to where FMA
// is hard:
//
//	0: raw, non-canonical encodings and specials included
//	1: z is about minus the product, rounded and then moved a few units
//	2: z sits a chosen distance from the product, in either direction,
//	   within or beyond the digits that scaling reaches
//	3: short factors whose product z cancels exactly or all but one unit
//
// Bit 2 of shape chooses small exponents, whose results stay in range, over
// exponents from the whole range of the format.
func fmaFuzzOperands(f *format, x, y, z bits, shape uint8, gap int16) (bits, bits, bits) {
	if shape&3 == 0 {
		return x, y, z
	}
	coeff := func(w bits, digits int) *big.Int {
		c := new(big.Int).SetUint64(w.Hi)
		c.Lsh(c, 64).Add(c, new(big.Int).SetUint64(w.Lo))
		c.Mod(c, pow10(digits))
		return c.Quo(c, pow10(int(w.Lo>>32%uint64(digits))))
	}
	exponent := func(w bits) int {
		// Both words, because a decimal64 has only the low one.
		e := w.Lo>>50 ^ w.Hi>>8
		if shape&4 != 0 {
			return int(int8(e)) % 16
		}
		return f.minExp + int(e%uint64(f.maxExp-f.minExp+1))
	}
	clamp := func(e int) int { return max(min(e, f.maxExp), f.minExp) }
	sign := func(w bits) bool { return (w.Hi^w.Lo)>>63 != 0 }

	digits := f.prec
	if shape&3 == 3 {
		digits = f.prec / 2
	}
	xv := value{kind: finite, neg: sign(x), coeff: coeff(x, digits), exp: exponent(x)}
	yv := value{kind: finite, neg: sign(y), coeff: coeff(y, digits), exp: exponent(y)}
	x, y = f.encode(xv), f.encode(yv)
	pneg := xv.neg != yv.neg
	pe := xv.exp + yv.exp

	switch shape & 3 {
	case 1:
		// The product rounded, negated and moved by a few units of its last
		// place.
		m := bid.ToNearestEven
		if gap&1 != 0 {
			m = bid.TowardZero
		}
		p := f.decode(f.mul(x, y, m))
		if p.kind != finite {
			return x, y, f.encode(value{kind: finite, neg: !pneg, coeff: coeff(z, f.prec), exp: f.maxExp})
		}
		c := new(big.Int).Add(p.coeff, big.NewInt(int64(gap%3)))
		if c.Sign() < 0 || c.Cmp(pow10(f.prec)) >= 0 {
			c = p.coeff
		}
		return x, y, f.encode(value{kind: finite, neg: !p.neg, coeff: c, exp: p.exp})
	case 2:
		// Within the digits scaling reaches, and past them, on each side.
		d := int(gap) % (2*f.prec + 12)
		if shape&8 != 0 {
			d = int(gap)
		}
		return x, y, f.encode(value{kind: finite, neg: sign(z), coeff: coeff(z, f.prec), exp: clamp(pe + d)})
	}
	// A product short enough to scale, cancelled exactly or to within one
	// unit of the scaled product.
	p := new(big.Int).Mul(xv.coeff, yv.coeff)
	k := max(0, min(int(uint16(gap))%8, f.prec-len(p.String())))
	if p.Sign() == 0 {
		k = 0
	}
	c := new(big.Int).Mul(p, pow10(k))
	c.Add(c, big.NewInt(int64(gap%2)))
	if c.Sign() < 0 {
		c.SetInt64(0)
	}
	if c.Cmp(pow10(f.prec)) >= 0 {
		c.Quo(c, big.NewInt(10))
	}
	return x, y, f.encode(value{kind: finite, neg: !pneg, coeff: c, exp: clamp(pe - k)})
}

// FuzzFMA64 and FuzzFMA128 compare the kernel with the reference on the shapes
// of fmaFuzzOperands in every mode.
func FuzzFMA64(f *testing.F) {
	enc := func(s string) uint64 { x, _ := dec64.parse(s, bid.ToNearestEven); return x.Lo }
	for _, s := range fmaTests {
		if s.f == dec64 {
			f.Add(enc(s.x), enc(s.y), enc(s.z), uint8(0), int16(0), uint8(0))
		}
	}
	for shape := range uint8(16) {
		f.Add(uint64(0x1234567890ABCDEF), uint64(0xFEDCBA0987654321), uint64(0x0F1E2D3C4B5A6978), shape, int16(35), uint8(shape%5))
		f.Add(uint64(0x31C000000000007B), uint64(0x31C00000000001C8), uint64(0x31C0000000000315), shape, int16(-37), uint8(shape%5))
	}
	f.Fuzz(func(t *testing.T, x, y, z uint64, shape uint8, gap int16, mode uint8) {
		m := bid.Mode(mode)
		a, b, c := fmaFuzzOperands(dec64, lo(x), lo(y), lo(z), shape, gap)
		got, want := bid.FMA64(a.Lo, b.Lo, c.Lo, m), reference{}.fma(dec64, a, b, c, m).Lo
		if got != want {
			t.Errorf("x=%s y=%s z=%s %s: kernel %s, reference %s", display(dec64, a), display(dec64, b), display(dec64, c), modeName(m), display(dec64, lo(got)), display(dec64, lo(want)))
		}
	})
}

func FuzzFMA128(f *testing.F) {
	enc := func(s string) bits { x, _ := dec128.parse(s, bid.ToNearestEven); return x }
	for _, s := range fmaTests {
		if s.f == dec128 {
			x, y, z := enc(s.x), enc(s.y), enc(s.z)
			f.Add(x.Hi, x.Lo, y.Hi, y.Lo, z.Hi, z.Lo, uint8(0), int16(0), uint8(0))
		}
	}
	for shape := range uint8(16) {
		f.Add(uint64(0x123456789ABCDEF0), uint64(0xFEDCBA0987654321), uint64(0x0F1E2D3C4B5A6978), uint64(0x1122334455667788), uint64(0x8877665544332211), uint64(0xA5A5A5A55A5A5A5A), shape, int16(71), uint8(shape%5))
		f.Add(uint64(0x3040000000000000), uint64(123), uint64(0x3040000000000000), uint64(456), uint64(0x3040000000000000), uint64(789), shape, int16(-73), uint8(shape%5))
	}
	f.Fuzz(func(t *testing.T, xh, xl, yh, yl, zh, zl uint64, shape uint8, gap int16, mode uint8) {
		m := bid.Mode(mode)
		a, b, c := fmaFuzzOperands(dec128, bits{Hi: xh, Lo: xl}, bits{Hi: yh, Lo: yl}, bits{Hi: zh, Lo: zl}, shape, gap)
		got, want := bid.FMA128(a, b, c, m), reference{}.fma(dec128, a, b, c, m)
		if got != want {
			t.Errorf("x=%s y=%s z=%s %s: kernel %s, reference %s", display(dec128, a), display(dec128, b), display(dec128, c), modeName(m), display(dec128, got), display(dec128, want))
		}
	})
}

// fmaBenchOperands draws triples: 60% with z within 4 places of the product's
// exponent, 20% spread over ±(2×Precision+10), and 20% near cancellation.
func fmaBenchOperands(f *format) [][3]bits {
	const count = 4096
	r := rand.New(rand.NewPCG(1, 2))
	coeff := func() value {
		c := new(big.Int)
		for range r.IntN(f.prec) + 1 {
			c.Mul(c, big.NewInt(10)).Add(c, big.NewInt(r.Int64N(10)))
		}
		return value{kind: finite, neg: r.IntN(2) == 0, coeff: c, exp: r.IntN(21) - 10}
	}
	triples := make([][3]bits, count)
	for i := range triples {
		x, y, z := coeff(), coeff(), coeff()
		xb, yb := f.encode(x), f.encode(y)
		pe := x.exp + y.exp
		var zb bits
		switch n := r.IntN(10); {
		case n < 6:
			z.exp = pe + r.IntN(9) - 4
			zb = f.encode(z)
		case n < 8:
			z.exp = pe + r.IntN(2*(2*f.prec+10)+1) - (2*f.prec + 10)
			zb = f.encode(z)
		default:
			p := f.mul(xb, yb, bid.ToNearestEven)
			zb = f.withSign(p, !f.signbit(p))
		}
		triples[i] = [3]bits{xb, yb, zb}
	}
	return triples
}

func BenchmarkFMA64(b *testing.B) {
	triples := fmaBenchOperands(dec64)
	var sink uint64
	i := 0
	for b.Loop() {
		p := &triples[i&(len(triples)-1)]
		sink ^= bid.FMA64(p[0].Lo, p[1].Lo, p[2].Lo, bid.ToNearestEven)
		i++
	}
	_ = sink
}

func BenchmarkFMA128(b *testing.B) {
	triples := fmaBenchOperands(dec128)
	var sink uint64
	i := 0
	for b.Loop() {
		p := &triples[i&(len(triples)-1)]
		sink ^= bid.FMA128(p[0], p[1], p[2], bid.ToNearestEven).Lo
		i++
	}
	_ = sink
}
