// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// Tests of Float64To64, Float64To128, Float32To64 and Float32To128 beyond
// the differential test TestKernel: an oracle through math/big written
// independently of the reference model, every power of two in the range of
// each binary format, the subnormals, the decimal ties and near ties at the
// destination precision constructed by inverting the arithmetic, a sweep of
// the float32 bit patterns, the allocation check, a fuzz target per function
// and the benchmark the package documentation's justification cites.

import (
	"internal/bid"
	"math"
	"math/big"
	"math/rand/v2"
	"testing"
)

var floatToModes = []bid.Mode{bid.ToNearestEven, bid.ToNearestAway, bid.TowardZero, bid.TowardPositive, bid.TowardNegative}

// floatToOracle returns the parts of the finite v rounded to f in mode m
// with preferred exponent 0: the sign, the coefficient and the exponent. The
// exact value of v is a rational with a power-of-two denominator; its
// adjusted exponent a is found by comparison, q = ⌊|v| × 10^(prec-1-a)⌋ has
// exactly prec digits, and the remainder compared with half a unit decides
// the rounding. An exact value is given the exponent closest to 0 among its
// encodings, by removing trailing zeros while the exponent is negative.
func floatToOracle(f *format, v float64, m bid.Mode) (neg bool, c *big.Int, exp int) {
	neg = math.Signbit(v)
	if v == 0 {
		return neg, new(big.Int), 0
	}
	r := new(big.Rat).SetFloat64(math.Abs(v))
	num, den := r.Num(), r.Denom()
	cmpPow10 := func(a int) int {
		if a >= 0 {
			return num.Cmp(new(big.Int).Mul(den, pow10(a)))
		}
		return new(big.Int).Mul(num, pow10(-a)).Cmp(den)
	}
	a := digits(num) - digits(den)
	for cmpPow10(a) < 0 {
		a--
	}
	for cmpPow10(a+1) >= 0 {
		a++
	}
	s := f.prec - 1 - a
	n, d := new(big.Int).Set(num), new(big.Int).Set(den)
	if s >= 0 {
		n.Mul(n, pow10(s))
	} else {
		d.Mul(d, pow10(-s))
	}
	q, rem := new(big.Int).QuoRem(n, d, new(big.Int))
	exp = -s
	if rem.Sign() == 0 {
		var digit big.Int
		for exp < 0 {
			q2, _ := new(big.Int).QuoRem(q, bigTen, &digit)
			if digit.Sign() != 0 {
				break
			}
			q, exp = q2, exp+1
		}
		return neg, q, exp
	}
	half := new(big.Int).Lsh(rem, 1).Cmp(d)
	up := false
	switch m {
	case bid.ToNearestAway:
		up = half >= 0
	case bid.TowardZero:
	case bid.TowardPositive:
		up = !neg
	case bid.TowardNegative:
		up = neg
	default:
		up = half > 0 || half == 0 && q.Bit(0) == 1
	}
	if up {
		q.Add(q, bigOne)
		if q.Cmp(pow10(f.prec)) == 0 {
			q, exp = new(big.Int).Set(pow10(f.prec-1)), exp+1
		}
	}
	return neg, q, exp
}

// floatToParts returns the sign, coefficient and exponent of a finite
// encoding, through the kernel's own Unpack.
func floatToParts(f *format, x bits) (neg bool, c *big.Int, exp int, finite bool) {
	if f == dec64 {
		kind, neg, c64, exp := bid.Unpack64(x.Lo)
		return neg, new(big.Int).SetUint64(c64), exp, kind == bid.KindFinite
	}
	kind, neg, c128, exp := bid.Unpack128(x)
	return neg, bigOf128(c128), exp, kind == bid.KindFinite
}

func bigOf128(c bid.Uint128) *big.Int {
	n := new(big.Int).SetUint64(c.Hi)
	n.Lsh(n, 64)
	return n.Or(n, new(big.Int).SetUint64(c.Lo))
}

// floatToCheck converts the finite v to f in mode m with the kernel and
// compares the parts with the oracle's.
func floatToCheck(t *testing.T, f *format, v float64, m bid.Mode) {
	t.Helper()
	got := kernel{}.float64To(f, v, m)
	neg, c, exp, finite := floatToParts(f, got)
	wneg, wc, wexp := floatToOracle(f, v, m)
	if !finite || neg != wneg || c.Cmp(wc) != 0 || exp != wexp {
		t.Errorf("%s(%v = %b, %s) = %s (neg=%v coeff=%v exp=%d), want neg=%v coeff=%v exp=%d",
			floatToName(f), v, v, modeName(m), display(f, got), neg, c, exp, wneg, wc, wexp)
	}
}

// floatToCheck32 is floatToCheck for a float32 operand: Float32To* is
// compared with the oracle at the exact value, and with Float64To* of the
// widened operand, which the contract says it equals.
func floatToCheck32(t *testing.T, f *format, v float32, m bid.Mode) {
	t.Helper()
	got := kernel{}.float32To(f, v, m)
	neg, c, exp, finite := floatToParts(f, got)
	wneg, wc, wexp := floatToOracle(f, float64(v), m)
	if !finite || neg != wneg || c.Cmp(wc) != 0 || exp != wexp {
		t.Errorf("%s(%v = %b, %s) = %s (neg=%v coeff=%v exp=%d), want neg=%v coeff=%v exp=%d",
			floatToName32(f), v, v, modeName(m), display(f, got), neg, c, exp, wneg, wc, wexp)
	}
	if wide := (kernel{}).float64To(f, float64(v), m); wide != got {
		t.Errorf("%s(%v, %s) = %s but the widened float64 converts to %s",
			floatToName32(f), v, modeName(m), display(f, got), display(f, wide))
	}
}

func floatToName(f *format) string {
	if f == dec64 {
		return "Float64To64"
	}
	return "Float64To128"
}

func floatToName32(f *format) string {
	if f == dec64 {
		return "Float32To64"
	}
	return "Float32To128"
}

// floatToCount scales a sample to the test length.
func floatToCount(n int) int {
	switch {
	case *longTest:
		return n * 20
	case testing.Short():
		return max(n/10, 1)
	}
	return n
}

// TestFloatToPowersOfTwo converts every power of two of each binary format,
// positive and negative, in every mode, to both widths. The exact decimal
// expansions run from 1 digit (2^0) to 751 (2^-1074); every one is cut and
// rounded correctly or the oracle disagrees.
func TestFloatToPowersOfTwo(t *testing.T) {
	for _, f := range []*format{dec64, dec128} {
		for k := -1074; k <= 1023; k++ {
			v := math.Ldexp(1, k)
			for _, m := range floatToModes {
				floatToCheck(t, f, v, m)
				floatToCheck(t, f, -v, m)
			}
		}
		for k := -149; k <= 127; k++ {
			v := float32(math.Ldexp(1, k))
			for _, m := range floatToModes {
				floatToCheck32(t, f, v, m)
				floatToCheck32(t, f, -v, m)
			}
		}
	}
}

// TestFloatToSubnormals converts the subnormal values: every float32
// subnormal (a stride of them by default, all of them under -bid.long), and
// a sample of the float64 subnormals including the smallest, the largest and
// the ones with a single bit set.
func TestFloatToSubnormals(t *testing.T) {
	for _, f := range []*format{dec64, dec128} {
		for b := uint64(1); b < 1<<52; b <<= 1 {
			for _, m := range floatToModes {
				floatToCheck(t, f, math.Float64frombits(b), m)
				floatToCheck(t, f, math.Float64frombits(1<<52-b), m)
			}
		}
		r := rand.New(rand.NewPCG(0x5ab, uint64(f.width)))
		for range floatToCount(2000) {
			v := math.Float64frombits(r.Uint64() & (1<<52 - 1))
			floatToCheck(t, f, v, floatToModes[r.IntN(len(floatToModes))])
		}
		stride := uint32(97)
		if *longTest {
			stride = 1
		}
		for b := uint32(1); b < 1<<23; b += stride {
			v := math.Float32frombits(b)
			floatToCheck32(t, f, v, bid.ToNearestEven)
			floatToCheck32(t, f, -v, floatToModes[int(b)%len(floatToModes)])
		}
	}
}

// floatToTies calls check with every (m, k) pair, m odd and below 2^mbits,
// such that m × 5^k has exactly f.prec+1 digits, the last a 5: the floats
// m × 2^-k whose exact decimal expansion is a tie at the destination
// precision. For each k the smallest and largest such m and a few at random.
func floatToTies(t *testing.T, f *format, mbits int, check func(m uint64, k int)) {
	r := rand.New(rand.NewPCG(0x7135, uint64(f.width)))
	lowest, highest := pow10(f.prec), new(big.Int).Sub(pow10(f.prec+1), bigOne)
	mmax := new(big.Int).Lsh(bigOne, uint(mbits))
	for k := 1; ; k++ {
		p := new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(k)), nil)
		if p.Cmp(highest) > 0 {
			break
		}
		// m in [⌈10^prec / 5^k⌉, ⌊(10^(prec+1) - 1) / 5^k⌋], below 2^mbits.
		lo := new(big.Int).Add(lowest, new(big.Int).Sub(p, bigOne))
		lo.Quo(lo, p)
		hi := new(big.Int).Quo(highest, p)
		if hi.Cmp(mmax) >= 0 {
			hi.Sub(mmax, bigOne)
		}
		if lo.Bit(0) == 0 {
			lo.Add(lo, bigOne)
		}
		if hi.Bit(0) == 0 {
			hi.Sub(hi, bigOne)
		}
		if lo.Cmp(hi) > 0 {
			continue
		}
		check(lo.Uint64(), k)
		check(hi.Uint64(), k)
		// hi and lo are odd and below 2^53, so m | 1 stays within [lo, hi].
		span := hi.Uint64() - lo.Uint64()
		for range 6 {
			check(lo.Uint64()+r.Uint64N(span+1)|1, k)
		}
	}
}

// TestFloatToTies converts every constructed decimal tie at the destination
// precision in every mode, for float64 and float32 operands.
func TestFloatToTies(t *testing.T) {
	for _, f := range []*format{dec64, dec128} {
		n := 0
		floatToTies(t, f, 53, func(m uint64, k int) {
			n++
			v := math.Ldexp(float64(m), -k)
			for _, mode := range floatToModes {
				floatToCheck(t, f, v, mode)
				floatToCheck(t, f, -v, mode)
			}
		})
		if n == 0 {
			t.Errorf("%s: no float64 ties constructed", f.name)
		}
		n = 0
		floatToTies(t, f, 24, func(m uint64, k int) {
			n++
			v := float32(math.Ldexp(float64(m), -k))
			for _, mode := range floatToModes {
				floatToCheck32(t, f, v, mode)
				floatToCheck32(t, f, -v, mode)
			}
		})
		if n == 0 {
			t.Errorf("%s: no float32 ties constructed", f.name)
		}
	}
}

// TestFloatToNearTies converts values whose discarded part is within a few
// units of 2^-r, or of 1/(2 × 5^k), of one half: the values at which a
// sticky bit decided from an approximation goes wrong. Both constructions
// invert the kernel's arithmetic.
//
// For a value below the destination's integer range, m × 2^-(r+s) scaled by
// 10^s is m × 5^s / 2^r, whose fractional part is (m × 5^s mod 2^r) / 2^r.
// Choosing m = (2^(r-1) + δ) × 5^-s mod 2^r, for an odd δ, makes that
// fractional part 1/2 + δ / 2^r exactly. The cases where the integer part
// has f.prec digits are kept.
//
// For an integer value m × 2^e with e >= k, the remainder modulo 10^k is
// 2^k × (m × 2^(e-k) mod 5^k). Choosing m = ((5^k + 1) / 2 + δ) × 2^-(e-k)
// mod 5^k makes the remainder 10^k / 2 + 2^(k-1) × (1 + 2δ): a fraction of
// 1/2 + (1 + 2δ) / (2 × 5^k), on either side of one half for δ = 0 and
// δ = -1. The cases where the quotient has f.prec digits are kept.
func TestFloatToNearTies(t *testing.T) {
	for _, f := range []*format{dec64, dec128} {
		small, large := 0, 0
		lowest, highest := pow10(f.prec-1), pow10(f.prec)

		// Values below the integer range: the integer part is near 5^s, so
		// s runs over the values that put 5^s within a factor of 2^53 of
		// 10^(prec-1).
		for s := 1; s <= 60; s++ {
			p5 := new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(s)), nil)
			for r := 2; r <= 53; r++ {
				mod := new(big.Int).Lsh(bigOne, uint(r))
				inv := new(big.Int).ModInverse(p5, mod)
				for _, delta := range []int64{-3, -1, 1, 3} {
					target := new(big.Int).Lsh(bigOne, uint(r-1))
					target.Add(target, big.NewInt(delta))
					m := new(big.Int).Mul(target, inv)
					m.Mod(m, mod)
					if m.Sign() == 0 {
						continue
					}
					// The integer part of m × 5^s / 2^r must have prec digits.
					ip := new(big.Int).Mul(m, p5)
					ip.Rsh(ip, uint(r))
					if ip.Cmp(lowest) < 0 || ip.Cmp(highest) >= 0 {
						continue
					}
					small++
					v := math.Ldexp(float64(m.Uint64()), -(r + s))
					for _, mode := range floatToModes {
						floatToCheck(t, f, v, mode)
						floatToCheck(t, f, -v, mode)
					}
					if r <= 24 {
						for _, mode := range floatToModes {
							floatToCheck32(t, f, float32(v), mode)
						}
					}
				}
			}
		}

		// Integer values: for each k, the exponents e that give the
		// quotient prec digits.
		for k := 1; k <= 22; k++ {
			p5 := new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(k)), nil)
			halfUp := new(big.Int).Add(p5, bigOne)
			halfUp.Rsh(halfUp, 1)
			for e := k; e <= 1023; e++ {
				inv := new(big.Int).ModInverse(new(big.Int).Lsh(bigOne, uint(e-k)), p5)
				for _, delta := range []int64{0, -1} {
					target := new(big.Int).Add(halfUp, big.NewInt(delta))
					m := new(big.Int).Mul(target, inv)
					m.Mod(m, p5)
					if m.Sign() == 0 {
						continue
					}
					q := new(big.Int).Lsh(m, uint(e))
					q.Quo(q, pow10(k))
					if q.Cmp(lowest) < 0 || q.Cmp(highest) >= 0 {
						continue
					}
					large++
					v := math.Ldexp(float64(m.Uint64()), e)
					for _, mode := range floatToModes {
						floatToCheck(t, f, v, mode)
						floatToCheck(t, f, -v, mode)
					}
				}
			}
		}
		if small == 0 || large == 0 {
			t.Errorf("%s: constructed %d near ties below one and %d above", f.name, small, large)
		}
		t.Logf("%s: %d near ties below the integer range, %d above", f.name, small, large)
	}
}

// TestFloatToFloat32Sweep converts a stride through every float32 bit
// pattern to both widths, in ToNearestEven and in a directed mode that the
// pattern selects. The stride is a prime, so every exponent and every
// residue of the significand is visited.
func TestFloatToFloat32Sweep(t *testing.T) {
	stride := uint32(65537)
	if *longTest {
		stride = 4099
	}
	for _, f := range []*format{dec64, dec128} {
		n := 0
		for b := uint32(0); ; b += stride {
			v := math.Float32frombits(b)
			if !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) {
				n++
				floatToCheck32(t, f, v, bid.ToNearestEven)
				floatToCheck32(t, f, v, floatToModes[1+int(b>>8)%(len(floatToModes)-1)])
			}
			if b > math.MaxUint32-stride {
				break
			}
		}
		t.Logf("%s: %d float32 values", f.name, n)
	}
}

// TestFloatToSpecials checks the zeros, infinities and NaNs: a zero keeps
// its sign and has exponent 0, an infinity keeps its sign, and every NaN,
// quiet or signaling, with or without a payload, is the canonical NaN.
func TestFloatToSpecials(t *testing.T) {
	nans := []float64{math.NaN(), -math.NaN(), math.Float64frombits(0x7FF0000000000001), math.Float64frombits(0xFFF8000000001234)}
	for _, f := range []*format{dec64, dec128} {
		for _, m := range floatToModes {
			for _, v := range []float64{0, math.Copysign(0, -1)} {
				floatToCheck(t, f, v, m)
				floatToCheck32(t, f, float32(v), m)
			}
			for _, v := range []float64{math.Inf(1), math.Inf(-1)} {
				want := f.encode(value{kind: infinite, neg: v < 0})
				if got := (kernel{}).float64To(f, v, m); got != want {
					t.Errorf("%s(%v, %s) = %s", floatToName(f), v, modeName(m), display(f, got))
				}
				if got := (kernel{}).float32To(f, float32(v), m); got != want {
					t.Errorf("%s(%v, %s) = %s", floatToName32(f), v, modeName(m), display(f, got))
				}
			}
			want := f.encode(nan)
			for _, v := range nans {
				if got := (kernel{}).float64To(f, v, m); got != want {
					t.Errorf("%s(%#x, %s) = %#x, want the canonical NaN", floatToName(f), math.Float64bits(v), modeName(m), got)
				}
				if got := (kernel{}).float32To(f, float32(v), m); got != want {
					t.Errorf("%s(%#x, %s) = %#x, want the canonical NaN", floatToName32(f), math.Float32bits(float32(v)), modeName(m), got)
				}
			}
			for _, b := range []uint32{0x7F800001, 0xFFC00000, 0x7FC01234} {
				if got := (kernel{}).float32To(f, math.Float32frombits(b), m); got != want {
					t.Errorf("%s(%#x, %s) = %#x, want the canonical NaN", floatToName32(f), b, modeName(m), got)
				}
			}
		}
	}
}

// TestFloatToAllocs checks that no path allocates: the integer that fits,
// the exact value with a negative exponent, the inexact value, the large
// integer that is divided down, the extremes of the range and the specials,
// in every mode.
func TestFloatToAllocs(t *testing.T) {
	inputs := []float64{0, math.Copysign(0, -1), 1, 0.5, 0x1p-24, 0.1, 123.456, 1e23, 1e22, 1.7976931348623157e308,
		5e-324, 0x1p-1022, math.Ldexp(1, 500), math.Ldexp(1, -500), math.Inf(1), math.NaN()}
	var sink64 uint64
	var sink128 bid.Uint128
	for _, m := range floatToModes {
		if n := testing.AllocsPerRun(100, func() {
			for _, v := range inputs {
				sink64 ^= bid.Float64To64(v, m)
				sink64 ^= bid.Float32To64(float32(v), m)
			}
		}); n != 0 {
			t.Errorf("Float64To64 or Float32To64 in mode %s: %v allocations", modeName(m), n)
		}
		if n := testing.AllocsPerRun(100, func() {
			for _, v := range inputs {
				x := bid.Float64To128(v, m)
				sink128.Lo ^= x.Lo
				x = bid.Float32To128(float32(v), m)
				sink128.Hi ^= x.Hi
			}
		}); n != 0 {
			t.Errorf("Float64To128 or Float32To128 in mode %s: %v allocations", modeName(m), n)
		}
	}
	_, _ = sink64, sink128
}

// floatToFuzz is the body of the fuzz targets: the kernel against the
// oracle and against the reference model, on the float the bits select, in
// the mode sel selects. The remaining bits of sel choose a float that raw
// bits rarely reach: a power of two, a decimal tie, or a subnormal.
func floatToFuzz(t *testing.T, f *format, b uint64, sel uint8, is32 bool) {
	m := floatToModes[int(sel&0x0f)%len(floatToModes)]
	v := math.Float64frombits(b)
	mbits := 53
	if is32 {
		v = float64(math.Float32frombits(uint32(b)))
		mbits = 24
	}
	switch sel >> 4 & 3 {
	case 1:
		v = math.Ldexp(1, int(b%2098)-1074)
	case 2:
		// An odd significand over a power of two with prec+1 digits.
		k := int(b>>mbits)%60 + 1
		mant := b&(1<<mbits-1) | 1
		v = math.Ldexp(float64(mant), -k)
	case 3:
		v = math.Float64frombits(b & (1<<52 - 1))
	}
	if is32 {
		v = float64(float32(v))
		if sel&0x40 != 0 {
			v = float64(math.Float32frombits(uint32(b) & (1<<23 - 1)))
		}
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return
	}
	if is32 {
		floatToCheck32(t, f, float32(v), m)
		if got, ref := (kernel{}).float32To(f, float32(v), m), (reference{}).float32To(f, float32(v), m); got != ref {
			t.Errorf("%s(%v, %s): kernel %s, reference %s", floatToName32(f), v, modeName(m), display(f, got), display(f, ref))
		}
		return
	}
	floatToCheck(t, f, v, m)
	if got, ref := (kernel{}).float64To(f, v, m), (reference{}).float64To(f, v, m); got != ref {
		t.Errorf("%s(%v, %s): kernel %s, reference %s", floatToName(f), v, modeName(m), display(f, got), display(f, ref))
	}
}

func floatToFuzzSeeds(f *testing.F) {
	for _, v := range []float64{0, 0.1, 0.5, 1e23, 123.456, 5e-324, 1.7976931348623157e308, 0x1p-24, 0x1p-50} {
		for sel := uint8(0); sel < 5; sel++ {
			f.Add(math.Float64bits(v), sel)
			f.Add(uint64(math.Float32bits(float32(v))), sel)
		}
	}
	f.Add(uint64(0x1ED09BEAD87C0378), uint8(0x13))
	f.Add(uint64(0xFFFFFFFFFFFFF), uint8(0x30))
	f.Add(uint64(1), uint8(0x72))
}

func FuzzFloat64To64(f *testing.F) {
	floatToFuzzSeeds(f)
	f.Fuzz(func(t *testing.T, b uint64, sel uint8) { floatToFuzz(t, dec64, b, sel, false) })
}

func FuzzFloat64To128(f *testing.F) {
	floatToFuzzSeeds(f)
	f.Fuzz(func(t *testing.T, b uint64, sel uint8) { floatToFuzz(t, dec128, b, sel, false) })
}

func FuzzFloat32To64(f *testing.F) {
	floatToFuzzSeeds(f)
	f.Fuzz(func(t *testing.T, b uint64, sel uint8) { floatToFuzz(t, dec64, b, sel, true) })
}

func FuzzFloat32To128(f *testing.F) {
	floatToFuzzSeeds(f)
	f.Fuzz(func(t *testing.T, b uint64, sel uint8) { floatToFuzz(t, dec128, b, sel, true) })
}

// BenchmarkFloatTo measures the paths at both widths: an integer that fits,
// an exact fraction, the typical inexact fraction, an integer divided down,
// and the two ends of the float64 range, which are the worst cases of the
// two multiword computations.
func BenchmarkFloatTo(b *testing.B) {
	cases := []struct {
		name string
		v    float64
	}{
		{"integer", 123456789},
		{"exact-fraction", 0.5},
		{"typical", 0.1},
		{"typical-large", 123.456},
		{"1e23", 1e23},
		{"2^500", math.Ldexp(1, 500)},
		{"max", math.MaxFloat64},
		{"2^-500", math.Ldexp(1, -500)},
		{"min-subnormal", 5e-324},
	}
	var sink64 uint64
	var sink128 bid.Uint128
	for _, c := range cases {
		b.Run("64/"+c.name, func(b *testing.B) {
			for b.Loop() {
				sink64 ^= bid.Float64To64(c.v, bid.ToNearestEven)
			}
		})
		b.Run("128/"+c.name, func(b *testing.B) {
			for b.Loop() {
				x := bid.Float64To128(c.v, bid.ToNearestEven)
				sink128.Lo ^= x.Lo
			}
		})
	}
	b.Run("32to64/typical", func(b *testing.B) {
		for b.Loop() {
			sink64 ^= bid.Float32To64(0.1, bid.ToNearestEven)
		}
	})
	_, _ = sink64, sink128
}
