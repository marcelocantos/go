// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package strconv_test

import (
	. "internal/strconv"
	"math"
	"math/big"
	"math/rand/v2"
	"testing"
)

// coeffInput is a value for Float64FromCoeff and Float32FromCoeff.
type coeffInput struct {
	hi, lo uint64
	exp10  int
}

func (in coeffInput) coeff() *big.Int {
	c := new(big.Int).SetUint64(in.hi)
	c.Lsh(c, 64)
	return c.Or(c, new(big.Int).SetUint64(in.lo))
}

// String returns the value as a numeral ParseFloat accepts.
func (in coeffInput) String() string {
	return in.coeff().String() + "e" + big.NewInt(int64(in.exp10)).String()
}

func (in coeffInput) rat() *big.Rat {
	r := new(big.Rat).SetInt(in.coeff())
	p := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(max(in.exp10, -in.exp10))), nil)
	if in.exp10 >= 0 {
		return r.Mul(r, new(big.Rat).SetInt(p))
	}
	return r.Quo(r, new(big.Rat).SetInt(p))
}

// randomCoeffInput returns a value with a coefficient of any bit length up to
// 128, most of the time with an exponent that keeps it near the range of the
// format with the given number of decimal orders of magnitude at each end.
func randomCoeffInput(r *rand.Rand, maxOrder, minOrder int) coeffInput {
	n := 1 + r.IntN(128)
	var in coeffInput
	switch {
	case n > 64:
		in.hi = r.Uint64() >> (128 - n)
		in.lo = r.Uint64()
	default:
		in.lo = r.Uint64() >> (64 - n)
	}
	if r.IntN(4) == 0 {
		// A run of trailing zero digits, as for a short value in a long
		// coefficient, keeps the value on or near a short binary fraction.
		c := new(big.Int).SetUint64(r.Uint64N(1 << 24))
		c.Mul(c, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(r.IntN(30))), nil))
		in.lo = new(big.Int).And(c, new(big.Int).SetUint64(math.MaxUint64)).Uint64()
		in.hi = new(big.Int).Rsh(c, 64).Uint64()
	}
	digits := int(float64(n) * 0.30103)
	in.exp10 = minOrder - 2 - digits + r.IntN(maxOrder-minOrder+5)
	if r.IntN(8) == 0 {
		in.exp10 = r.IntN(14000) - 7000
	}
	return in
}

// TestCoeffNearestMatchesParseFloat checks the default rounding against the
// exact string path, with and without the fast path, in both formats.
func TestCoeffNearestMatchesParseFloat(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	defer SetOptimize(SetOptimize(true))
	n := 20000
	if testing.Short() {
		n = 2000
	}
	for i := range n {
		var in coeffInput
		if i%2 == 0 {
			in = randomCoeffInput(r, 309, -324)
		} else {
			in = randomCoeffInput(r, 39, -46)
		}
		for _, neg := range []bool{false, true} {
			s := in.String()
			if neg {
				s = "-" + s
			}
			want64, err := ParseFloat(s, 64)
			if err != nil && err != ErrRange {
				t.Fatalf("ParseFloat(%s, 64): %v", s, err)
			}
			want32, err := ParseFloat(s, 32)
			if err != nil && err != ErrRange {
				t.Fatalf("ParseFloat(%s, 32): %v", s, err)
			}
			for _, opt := range []bool{true, false} {
				SetOptimize(opt)
				if got := Float64FromCoeff(in.hi, in.lo, in.exp10, neg, RoundNearestEven); math.Float64bits(got) != math.Float64bits(want64) {
					t.Errorf("Float64FromCoeff(%s, neg=%v), optimize=%v = %x, want %x", in, neg, opt, got, want64)
				}
				if got := Float32FromCoeff(in.hi, in.lo, in.exp10, neg, RoundNearestEven); math.Float32bits(got) != math.Float32bits(float32(want32)) {
					t.Errorf("Float32FromCoeff(%s, neg=%v), optimize=%v = %x, want %x", in, neg, opt, got, float32(want32))
				}
			}
			SetOptimize(true)
		}
	}
}

// TestCoeffDirected checks the directions on their defining properties,
// against exact rationals: the two directed results are the two neighbors of
// the value (or both the value, when it is exact), and each nearest result is
// the closer one, with the tie rule.
func TestCoeffDirected(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	n := 20000
	if testing.Short() {
		n = 2000
	}
	for i := range n {
		var in coeffInput
		if i%2 == 0 {
			in = randomCoeffInput(r, 309, -324)
		} else {
			in = randomCoeffInput(r, 39, -46)
		}
		if in.hi|in.lo == 0 {
			continue
		}
		// Binary ties are the case the nearest modes differ on.
		if r.IntN(4) == 0 {
			m := uint64(1)<<53 + r.Uint64N(1<<53) | 1
			in = coeffInput{0, m, 0}
			if i%2 == 1 {
				in.lo = 1<<24 + r.Uint64N(1<<24) | 1
			}
		}
		exact := in.rat()
		if i%2 == 0 {
			toward := Float64FromCoeff(in.hi, in.lo, in.exp10, false, RoundTowardZero)
			away := Float64FromCoeff(in.hi, in.lo, in.exp10, false, RoundAwayFromZero)
			even := Float64FromCoeff(in.hi, in.lo, in.exp10, false, RoundNearestEven)
			tiesAway := Float64FromCoeff(in.hi, in.lo, in.exp10, false, RoundNearestAway)
			checkNeighbors(t, in, exact, toward, away, even, tiesAway, func(v float64) float64 { return math.Nextafter(v, math.Inf(1)) },
				func(v float64) bool { return math.Float64bits(v)&1 == 0 }, math.MaxFloat64)
		} else {
			toward := Float32FromCoeff(in.hi, in.lo, in.exp10, false, RoundTowardZero)
			away := Float32FromCoeff(in.hi, in.lo, in.exp10, false, RoundAwayFromZero)
			even := Float32FromCoeff(in.hi, in.lo, in.exp10, false, RoundNearestEven)
			tiesAway := Float32FromCoeff(in.hi, in.lo, in.exp10, false, RoundNearestAway)
			checkNeighbors(t, in, exact, float64(toward), float64(away), float64(even), float64(tiesAway),
				func(v float64) float64 { return float64(math.Nextafter32(float32(v), float32(math.Inf(1)))) },
				func(v float64) bool { return math.Float32bits(float32(v))&1 == 0 }, math.MaxFloat32)
		}
		// The sign only changes the sign.
		if got, want := Float64FromCoeff(in.hi, in.lo, in.exp10, true, RoundAwayFromZero), Float64FromCoeff(in.hi, in.lo, in.exp10, false, RoundAwayFromZero); got != -want {
			t.Errorf("Float64FromCoeff(-%s, away) = %v, want %v", in, got, -want)
		}
		if got, want := Float32FromCoeff(in.hi, in.lo, in.exp10, true, RoundTowardZero), Float32FromCoeff(in.hi, in.lo, in.exp10, false, RoundTowardZero); got != -want {
			t.Errorf("Float32FromCoeff(-%s, toward) = %v, want %v", in, got, -want)
		}
	}
}

// checkNeighbors checks the four results for the positive exact value.
func checkNeighbors(t *testing.T, in coeffInput, exact *big.Rat, toward, away, even, tiesAway float64, next func(float64) float64, isEven func(float64) bool, largest float64) {
	t.Helper()
	if math.IsInf(toward, 0) {
		t.Errorf("%s toward zero is infinite", in)
		return
	}
	lo := new(big.Rat).SetFloat64(toward)
	if lo.Cmp(exact) > 0 {
		t.Errorf("%s toward zero = %v, above the value", in, toward)
	}
	if lo.Cmp(exact) == 0 {
		if away != toward || even != toward || tiesAway != toward {
			t.Errorf("%s is exact, %v, but away, even, tiesAway = %v, %v, %v", in, toward, away, even, tiesAway)
		}
		return
	}
	if toward != largest {
		if want := next(toward); away != want {
			t.Errorf("%s toward zero = %v and away = %v, want %v", in, toward, away, want)
		}
	} else if !math.IsInf(away, 1) {
		t.Errorf("%s above the largest value: away = %v, want +Inf", in, away)
	}
	hi := new(big.Rat)
	if toward != largest {
		hi.SetFloat64(away)
		if hi.Cmp(exact) <= 0 {
			t.Errorf("%s away = %v, not above the value", in, away)
		}
	} else {
		// Where the next significand would have been: one ulp above largest,
		// and the ulp of a format's largest value is largest/(2^prec - 1).
		hi.Set(new(big.Rat).SetFloat64(largest))
		ulp := big.NewRat(1, 1<<53-1)
		if largest == math.MaxFloat32 {
			ulp = big.NewRat(1, 1<<24-1)
		}
		hi.Add(hi, ulp.Mul(ulp, hi))
	}
	mid := new(big.Rat).Add(lo, hi)
	mid.Quo(mid, big.NewRat(2, 1))
	var wantEven, wantAway float64
	switch c := exact.Cmp(mid); {
	case c < 0:
		wantEven, wantAway = toward, toward
	case c > 0:
		wantEven, wantAway = away, away
	default:
		wantAway = away
		wantEven = away
		if isEven(toward) {
			wantEven = toward
		}
	}
	if even != wantEven {
		t.Errorf("%s nearest even = %v, want %v", in, even, wantEven)
	}
	if tiesAway != wantAway {
		t.Errorf("%s nearest away = %v, want %v", in, tiesAway, wantAway)
	}
}

// TestCoeffRange checks the results at and beyond the ends of the range in
// each direction, including the values decided by the size of the coefficient.
func TestCoeffRange(t *testing.T) {
	tests := []struct {
		name     string
		in       coeffInput
		even     uint64
		toward   uint64
		away     uint64
		even32   uint32
		toward32 uint32
		away32   uint32
	}{
		{"zero", coeffInput{0, 0, 0}, 0, 0, 0, 0, 0, 0},
		{"zero, far", coeffInput{0, 0, 7000}, 0, 0, 0, 0, 0, 0},
		{"huge", coeffInput{0, 1, 7000}, 0x7FF0000000000000, 0x7FEFFFFFFFFFFFFF, 0x7FF0000000000000, 0x7F800000, 0x7F7FFFFF, 0x7F800000},
		{"1e400", coeffInput{0, 1, 400}, 0x7FF0000000000000, 0x7FEFFFFFFFFFFFFF, 0x7FF0000000000000, 0x7F800000, 0x7F7FFFFF, 0x7F800000},
		{"tiny", coeffInput{0, 1, -7000}, 0, 0, 1, 0, 0, 1},
		{"1e-400", coeffInput{0, 1, -400}, 0, 0, 1, 0, 0, 1},
		{"long tiny", coeffInput{math.MaxUint64, math.MaxUint64, -400}, 0, 0, 1, 0, 0, 1},
		{"long huge", coeffInput{math.MaxUint64, math.MaxUint64, 400}, 0x7FF0000000000000, 0x7FEFFFFFFFFFFFFF, 0x7FF0000000000000, 0x7F800000, 0x7F7FFFFF, 0x7F800000},
		{"1e309", coeffInput{0, 1, 309}, 0x7FF0000000000000, 0x7FEFFFFFFFFFFFFF, 0x7FF0000000000000, 0x7F800000, 0x7F7FFFFF, 0x7F800000},
		{"1e-324", coeffInput{0, 1, -324}, 0, 0, 1, 0, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c64 := func(r Rounding) uint64 {
				return math.Float64bits(Float64FromCoeff(tt.in.hi, tt.in.lo, tt.in.exp10, false, r))
			}
			c32 := func(r Rounding) uint32 {
				return math.Float32bits(Float32FromCoeff(tt.in.hi, tt.in.lo, tt.in.exp10, false, r))
			}
			if got := c64(RoundNearestEven); got != tt.even {
				t.Errorf("Float64FromCoeff even = %#x, want %#x", got, tt.even)
			}
			if got := c64(RoundNearestAway); got != tt.even {
				t.Errorf("Float64FromCoeff nearest away = %#x, want %#x", got, tt.even)
			}
			if got := c64(RoundTowardZero); got != tt.toward {
				t.Errorf("Float64FromCoeff toward = %#x, want %#x", got, tt.toward)
			}
			if got := c64(RoundAwayFromZero); got != tt.away {
				t.Errorf("Float64FromCoeff away = %#x, want %#x", got, tt.away)
			}
			if got := c32(RoundNearestEven); got != tt.even32 {
				t.Errorf("Float32FromCoeff even = %#x, want %#x", got, tt.even32)
			}
			if got := c32(RoundNearestAway); got != tt.even32 {
				t.Errorf("Float32FromCoeff nearest away = %#x, want %#x", got, tt.even32)
			}
			if got := c32(RoundTowardZero); got != tt.toward32 {
				t.Errorf("Float32FromCoeff toward = %#x, want %#x", got, tt.toward32)
			}
			if got := c32(RoundAwayFromZero); got != tt.away32 {
				t.Errorf("Float32FromCoeff away = %#x, want %#x", got, tt.away32)
			}
		})
	}
	// Negative zero keeps its sign in every direction and format.
	for _, r := range []Rounding{RoundNearestEven, RoundNearestAway, RoundTowardZero, RoundAwayFromZero} {
		if got := Float64FromCoeff(0, 0, 5, true, r); math.Float64bits(got) != 1<<63 {
			t.Errorf("Float64FromCoeff(-0, %d) = %x", r, got)
		}
		if got := Float32FromCoeff(0, 0, 5, true, r); math.Float32bits(got) != 1<<31 {
			t.Errorf("Float32FromCoeff(-0, %d) = %x", r, got)
		}
	}
}

// TestCoeffDecimalExact checks the claim in coeffFloatBits that the decimal
// never truncates. The extremes are the largest coefficients at the two ends
// of the undecided range of each format.
func TestCoeffDecimalExact(t *testing.T) {
	for _, hi := range []uint64{0, 1, math.MaxUint64} {
		for _, lo := range []uint64{1, math.MaxUint64} {
			for exp10 := -380; exp10 <= 320; exp10++ {
				if !CoeffDecimalExact(hi, lo, exp10, 64) {
					t.Errorf("(%#x, %#x) × 10^%d truncates in float64", hi, lo, exp10)
				}
				if exp10 >= -100 && exp10 <= 50 && !CoeffDecimalExact(hi, lo, exp10, 32) {
					t.Errorf("(%#x, %#x) × 10^%d truncates in float32", hi, lo, exp10)
				}
			}
		}
	}
}

// TestCoeffAllocs checks that neither path allocates.
func TestCoeffAllocs(t *testing.T) {
	inputs := []coeffInput{
		{0, 1, 36},
		{0, 1<<53 + 1, 0},
		{0x0001ED09BEAD87C0, 0x378D8E63FFFFFFFF, -360},
		{0x0001ED09BEAD87C0, 0x378D8E63FFFFFFFF, 274},
		{0x0001ED09BEAD87C0, 0x378D8E63FFFFFFFF, 6000},
		{math.MaxUint64, math.MaxUint64, 0},
	}
	rs := []Rounding{RoundNearestEven, RoundNearestAway, RoundTowardZero, RoundAwayFromZero}
	var s64 float64
	var s32 float32
	if n := testing.AllocsPerRun(100, func() {
		for _, in := range inputs {
			for _, r := range rs {
				s64 += Float64FromCoeff(in.hi, in.lo, in.exp10, false, r)
				s32 += Float32FromCoeff(in.hi, in.lo, in.exp10, false, r)
			}
		}
	}); n != 0 {
		t.Errorf("%v allocations, want 0", n)
	}
	_, _ = s64, s32
}
