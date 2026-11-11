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

// TestCoeffScale compares coeffScale with the exact path wherever it
// decides, for both formats, and checks that it does decide what it is
// there for and declines what it cannot know.
//
// The inputs: coefficients of every bit length at exponents across each
// format's range and past both ends of it; the midpoints of two adjacent
// floats and the values one unit of the coefficient either side of them,
// at powers of ten the table holds exactly, where a tie must be decided,
// and as a multiple of a power of five at a negative exponent, where the
// same tie cannot be and must be declined; and the same around the
// smallest subnormals and the largest finite value.
func TestCoeffScale(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	check := func(bitSize int, hi, lo uint64, p int) (decided bool) {
		t.Helper()
		got, ok := CoeffScale(hi, lo, p, bitSize)
		if !ok {
			return false
		}
		if want := CoeffExact(hi, lo, p, bitSize); got != want {
			t.Fatalf("coeffScale(%#x, %#x, %d, %d) = %#x, exact path %#x", hi, lo, p, bitSize, got, want)
		}
		return true
	}
	words := func(c *big.Int) (hi, lo uint64) {
		return new(big.Int).Rsh(c, 64).Uint64(), new(big.Int).And(c, new(big.Int).SetUint64(math.MaxUint64)).Uint64()
	}
	n := 2000000
	if testing.Short() {
		n = 200000
	}
	for _, bitSize := range []int{64, 32} {
		maxOrder, minOrder := 309, -324
		if bitSize == 32 {
			maxOrder, minOrder = 39, -46
		}
		decided, total := 0, 0
		for i := 0; i < n; i++ {
			in := randomCoeffInput(r, maxOrder, minOrder)
			if in.hi|in.lo == 0 {
				continue
			}
			total++
			if check(bitSize, in.hi, in.lo, in.exp10) {
				decided++
			}
		}
		if decided < total*3/4 {
			t.Errorf("float%d: coeffScale decided %d of %d random values", bitSize, decided, total)
		}

		// Ties and their neighbours. A midpoint is (2m+1) × 2^j with m a
		// significand; c = (2m+1) × 2^j × 5^q at exponent -q, or
		// (2m+1) × 2^j / 10^q at exponent q when that divides, is the
		// same value.
		mant := 53
		if bitSize == 32 {
			mant = 24
		}
		ties, tiesDecided := 0, 0
		for i := 0; i < n/20; i++ {
			m := new(big.Int).SetUint64(1<<(mant-1) | r.Uint64N(1<<(mant-1)))
			mid := m.Lsh(m, 1).Or(m, big.NewInt(1))
			mid.Lsh(mid, uint(r.IntN(128-mant-1)))
			for _, q := range []int{0, 1, 2, 5, 17, 30} {
				// At exponent -q.
				c := new(big.Int).Mul(mid, new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(q)), nil))
				if c.BitLen() <= 128 {
					for _, d := range []int64{-1, 0, 1} {
						hi, lo := words(new(big.Int).Add(c, big.NewInt(d)))
						// The value is c × 10^-q × 2^q: the tie itself
						// when q is zero, and otherwise the tie scaled by
						// a power of two, a tie still.
						ok := check(bitSize, hi, lo, -q)
						if d == 0 {
							ties++
							if ok {
								tiesDecided++
							}
							if ok != (q == 0) {
								t.Fatalf("float%d: tie %#x %#x e-%d: decided = %t", bitSize, hi, lo, q, ok)
							}
						}
					}
				}
				// At exponent +q, when 10^q divides.
				if c, rem := new(big.Int).QuoRem(mid, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(q)), nil), new(big.Int)); rem.Sign() == 0 && c.Sign() != 0 && q > 0 {
					hi, lo := words(c)
					if !check(bitSize, hi, lo, q) {
						t.Fatalf("float%d: tie %#x %#x e+%d not decided", bitSize, hi, lo, q)
					}
				}
			}
		}
		if tiesDecided == 0 || tiesDecided == ties {
			t.Errorf("float%d: %d of %d ties decided", bitSize, tiesDecided, ties)
		}

		// The ends of the range: every coefficient length against the
		// exponents that put the value at the smallest subnormals, where
		// the significand's place rises through the product, and at the
		// largest finite value.
		for bl := 1; bl <= 128; bl++ {
			for i := 0; i < 50; i++ {
				c := new(big.Int).SetUint64(r.Uint64())
				c.Lsh(c, 64).Or(c, new(big.Int).SetUint64(r.Uint64()))
				c.Rsh(c, uint(128-bl)).SetBit(c, bl-1, 1)
				hi, lo := words(c)
				digits := len(c.String())
				for _, order := range []int{minOrder - 3, minOrder - 2, minOrder - 1, minOrder, minOrder + 1, minOrder + 2, minOrder + 20, maxOrder - 2, maxOrder - 1, maxOrder, maxOrder + 1} {
					check(bitSize, hi, lo, order-digits)
					check(bitSize, hi, lo, order-digits+1)
				}
			}
		}
	}
}

// coeffBandInputs are coefficients built to land where coeffScale cannot
// decide from its product alone, which random values reach once in 2^74:
// the low bits of the product are within 2^128 above the half ("band"),
// or, for a coefficient cut below the table's last power, under the half
// by less than the cut may have dropped ("cut"). Each was found by
// solving for the coefficient modulo a power of two and checked against
// an exact model of the conversion.
var coeffBandInputs = []struct {
	bitSize int
	hi, lo  uint64
	exp10   int
}{
	{64, 0x8000000c7, 0x5c1c6dc68aa649aa, -300},      // band
	{64, 0x800000270, 0x5ab651dae71dd8b, -200},       // band
	{64, 0x800000000, 0x5ffc00000000, -5},            // band
	{64, 0x800000000, 0x480000000000, -1},            // band
	{64, 0x800000000, 0x400000000000, 0},             // band
	{64, 0x800000000, 0x1000000000000, 1},            // band
	{64, 0x800000000, 0x4000000000000, 2},            // band
	{64, 0x8000000ae, 0x703331aa0f006f45, 17},        // band
	{64, 0x800000168, 0x1aeb1c5b585b317c, 30},        // band
	{64, 0x80000071a, 0x9020231e080de337, 54},        // band
	{64, 0x8000002dd, 0x33c2416209da2d92, 55},        // band
	{64, 0x800000030, 0x4aa62790b95fce6b, 56},        // band
	{64, 0x8000003c1, 0xc9dad1a3109c2f70, 57},        // band
	{64, 0x8000002ae, 0x60f9d2aafbbe6794, 60},        // band
	{64, 0x8000000b9, 0xe2fb2fa378a06a4e, 100},       // band
	{64, 0x8000004b1, 0x28b3fb26ba15c885, 200},       // band
	{64, 0x8000003ae, 0x585c66726609eb33, 270},       // band
	{32, 0x20, 0xada00000000, -5},                    // band
	{32, 0x20, 0x1c0000000000, -1},                   // band
	{32, 0x20, 0x200000000000, 0},                    // band
	{32, 0x20, 0x800000000000, 1},                    // band
	{32, 0x20, 0x2000000000000, 2},                   // band
	{32, 0x20, 0x2000000000000000, 7},                // band
	{32, 0x20, 0x8000000000000000, 8},                // band
	{64, 0x1408f3ff, 0xb9404909c2ee4137, -349},       // cut
	{64, 0x14000015b56be, 0x2530d53903817c55, -349},  // cut
	{64, 0x7d37f4fe4, 0x5d1c87d0251179df, -351},      // cut
	{64, 0x7d000087ade246, 0x87134a455e949197, -351}, // cut
	{64, 0x131b59d20c875, 0x2a7932a7faa8bdff, -355},  // cut
}

// TestCoeffScaleBand checks coeffScale where its bounds bind. In the band
// it must decline unless the table holds the power of ten exactly, when it
// must decide, correctly; a cut coefficient under the half by less than
// its cut it must decline. And a significand of all ones that rounds up
// must carry into the exponent.
func TestCoeffScaleBand(t *testing.T) {
	for _, in := range coeffBandInputs {
		got, ok := CoeffScale(in.hi, in.lo, in.exp10, in.bitSize)
		exact := 0 <= in.exp10 && in.exp10 <= CoeffExactPow10
		if ok != exact {
			t.Errorf("coeffScale(%#x, %#x, %d, %d): decided = %t, want %t", in.hi, in.lo, in.exp10, in.bitSize, ok, exact)
		}
		if want := CoeffExact(in.hi, in.lo, in.exp10, in.bitSize); ok && got != want {
			t.Errorf("coeffScale(%#x, %#x, %d, %d) = %#x, exact path %#x", in.hi, in.lo, in.exp10, in.bitSize, got, want)
		}
	}
	for _, bitSize := range []int{64, 32} {
		for n := 20; n <= 128; n++ {
			// 2^n - 1 at 10^0, and at 10^3 as its multiple of 125
			// that is nearest below a power of two.
			c := new(big.Int).Lsh(big.NewInt(1), uint(n))
			c.Sub(c, big.NewInt(1))
			for _, p := range []int{0, 3} {
				c := new(big.Int).Set(c)
				if p > 0 {
					c.Quo(c, big.NewInt(1000))
				}
				hi, lo := new(big.Int).Rsh(c, 64).Uint64(), new(big.Int).And(c, new(big.Int).SetUint64(math.MaxUint64)).Uint64()
				if hi == 0 && lo < 1e19 {
					continue
				}
				got, ok := CoeffScale(hi, lo, p, bitSize)
				if want := CoeffExact(hi, lo, p, bitSize); !ok || got != want {
					t.Errorf("coeffScale(%#x, %#x, %d, %d) = %#x, %t, exact path %#x", hi, lo, p, bitSize, got, ok, want)
				}
			}
		}
	}
}
