// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// The benchmarks of the kernel and the ratchet that locks them (target
// T1.6.4).
//
// BenchmarkKernel has one sub-benchmark for each operation and conversion at
// each width, named Op/Width/Class: Op is the exported function without its
// width suffix, Width is 64 or 128 for the decimal kernel and float64 for the
// reference row, and Class is the kind of operand the row runs over. A
// float64 row exists wherever the language has a counterpart, so a reader can
// see what a decimal operation costs beside the binary one. The strawman of
// the proposal is not a row: its kernel is gone from this tree, and its
// figures are cited in the package documentation ("Width parameterisation")
// and in the proposal's README, not re-measured.
//
// Rows over a "mixed" class run over a pool of operands of random
// coefficient length and exponents within four places of each other, the mix
// of the spike that fixed the cost model of this package. Rows over a named
// class run over the single operand pair the class names, taken from the
// worst or the typical case of the path.
//
// Every row is a plain loop over a slice of operands with the call written
// into it, so that the per-call cost of the row is the call and the loop; the
// generic loop helpers below are inlined into each row. The loop floor is
// about a nanosecond and the same in every row.
//
// TestBenchCoversAPI fails for an exported function with no row. The ratchet
// is TestBenchRatchet: it measures the rows in short runs and compares them
// with testdata/bench_baseline_<GOOS>_<GOARCH>.txt, which holds a benchstat
// file and the noise band the comparison allows. It runs only under
// -bid.ratchet. The baseline moves only by a deliberate commit that replaces
// the file; the header of the file says how it was taken and how to retake
// it.

import (
	"bufio"
	"cmp"
	"flag"
	"fmt"
	"internal/bid"
	"math"
	"math/big"
	"math/rand/v2"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	ratchetFlag  = flag.Bool("bid.ratchet", false, "run TestBenchRatchet: compare short benchmark runs with the baseline in testdata")
	ratchetTime  = flag.Duration("bid.ratchet.time", 50*time.Millisecond, "benchmark time of each measurement of TestBenchRatchet")
	ratchetCount = flag.Int("bid.ratchet.count", 3, "measurements of each row of TestBenchRatchet before it retries; the row's time is the smallest")
	ratchetRetry = flag.Int("bid.ratchet.retry", 5, "further measurements of a row over the band before TestBenchRatchet fails it")
)

const (
	poolSize = 1024 // a power of two: the loops index with i&(len-1)
	rne      = bid.ToNearestEven
)

var benchSink any

// The loops. Each stores the last result in benchSink so that no call is
// removed, and indexes the pools with a mask, so that a pool of one operand
// pair is a pool of length one.

func loop1[A, R any](b *testing.B, xs []A, f func(x A) R) {
	var s R
	m := len(xs) - 1
	for i := 0; i < b.N; i++ {
		s = f(xs[i&m])
	}
	benchSink = s
}

func loop2[A, R any](b *testing.B, xs, ys []A, f func(x, y A) R) {
	var s R
	m := len(xs) - 1
	for i := 0; i < b.N; i++ {
		s = f(xs[i&m], ys[i&m])
	}
	benchSink = s
}

func loop3[A, R any](b *testing.B, xs, ys, zs []A, f func(x, y, z A) R) {
	var s R
	m := len(xs) - 1
	for i := 0; i < b.N; i++ {
		s = f(xs[i&m], ys[i&m], zs[i&m])
	}
	benchSink = s
}

// opnds is one class of operands at every width: the decimal64 encodings,
// the decimal128 encodings and the float64 counterparts.
type opnds struct {
	x64, y64, z64    []uint64
	x128, y128, z128 []bits
	xf, yf, zf       []float64
}

func lows(bs []bits) []uint64 {
	r := make([]uint64, len(bs))
	for i, b := range bs {
		r[i] = b.Lo
	}
	return r
}

// fixedOpnds returns the class of one operand triple, from decimal strings
// for each width and a float for the reference row.
func fixedOpnds(x, y, z [3]string, xf, yf, zf float64) *opnds {
	one := func(f *format, s string) []bits {
		if s == "" {
			s = "0"
		}
		v, _ := f.parse(s, rne)
		return []bits{v}
	}
	o := &opnds{
		x128: one(dec128, x[1]), y128: one(dec128, y[1]), z128: one(dec128, z[1]),
		xf: []float64{xf}, yf: []float64{yf}, zf: []float64{zf},
	}
	o.x64, o.y64, o.z64 = lows(one(dec64, x[0])), lows(one(dec64, y[0])), lows(one(dec64, z[0]))
	return o
}

// operandCoeff returns a random coefficient of 1 to prec digits.
func operandCoeff(r *rand.Rand, prec int) *big.Int {
	digits := 1 + r.IntN(prec)
	c := new(big.Int).Lsh(new(big.Int).SetUint64(r.Uint64()), 64)
	c.Or(c, new(big.Int).SetUint64(r.Uint64()))
	return c.Mod(c, pow10(digits))
}

// mixedFloat is the float64 counterpart of an operand of the mixed class.
func mixedFloat(r *rand.Rand) float64 {
	v := (1 + 9*r.Float64()) * math.Pow(10, float64(r.IntN(9)-4))
	if r.IntN(2) == 0 {
		v = -v
	}
	return v
}

// benchOperands builds the pools. Seeds are fixed, so every run times the
// same operands.
type benchOperands struct {
	mixed, addMix, fmaMix              *opnds
	far, farFMA                        *opnds
	divInexact, divExact, divWidest    *opnds
	divByZero                          *opnds
	remSmall, remMax                   *opnds
	fracSigned, fracPositive, integers *opnds
	ints                               []int64
	uints                              []uint64
}

func buildBenchOperands() *benchOperands {
	r := rand.New(rand.NewPCG(1, 2))
	bo := &benchOperands{}

	// mixed: coefficients of every length, exponents within ±4.
	pool := func(f *format, gen func() bits) []bits {
		bs := make([]bits, poolSize)
		for i := range bs {
			bs[i] = gen()
		}
		return bs
	}
	mix := func(f *format) bits {
		return f.encode(value{kind: finite, neg: r.IntN(2) == 0, coeff: operandCoeff(r, f.prec), exp: r.IntN(9) - 4})
	}
	bo.mixed = &opnds{}
	bo.mixed.x128, bo.mixed.y128, bo.mixed.z128 = pool(dec128, func() bits { return mix(dec128) }), pool(dec128, func() bits { return mix(dec128) }), pool(dec128, func() bits { return mix(dec128) })
	bo.mixed.x64, bo.mixed.y64, bo.mixed.z64 = lows(pool(dec64, func() bits { return mix(dec64) })), lows(pool(dec64, func() bits { return mix(dec64) })), lows(pool(dec64, func() bits { return mix(dec64) }))
	for range poolSize {
		bo.mixed.xf = append(bo.mixed.xf, mixedFloat(r))
		bo.mixed.yf = append(bo.mixed.yf, mixedFloat(r))
		bo.mixed.zf = append(bo.mixed.zf, mixedFloat(r))
	}

	// addMix: 70% with exponents within 4 of each other, 20% spread over
	// ±(Precision+10) and 10% equal (the width spike's mix for Add).
	addMix := func(f *format) (xs, ys []bits) {
		for range poolSize {
			cx := value{kind: finite, neg: r.IntN(2) == 0, coeff: operandCoeff(r, f.prec), exp: r.IntN(21) - 10}
			cy := value{kind: finite, neg: r.IntN(2) == 0, coeff: operandCoeff(r, f.prec), exp: r.IntN(21) - 10}
			switch n := r.IntN(10); {
			case n < 7:
				cy.exp = cx.exp + r.IntN(9) - 4
			case n < 9:
				cy.exp = cx.exp + r.IntN(2*(f.prec+10)+1) - (f.prec + 10)
			default:
				cy = cx
			}
			xs, ys = append(xs, f.encode(cx)), append(ys, f.encode(cy))
		}
		return xs, ys
	}
	bo.addMix = &opnds{xf: bo.mixed.xf, yf: bo.mixed.yf}
	bo.addMix.x64, bo.addMix.y64 = func() ([]uint64, []uint64) { x, y := addMix(dec64); return lows(x), lows(y) }()
	bo.addMix.x128, bo.addMix.y128 = addMix(dec128)

	// fmaMix: 60% with z within 4 places of the product's exponent, 20%
	// spread over ±(2×Precision+10), and 20% near cancellation.
	fmaMix := func(f *format) (xs, ys, zs []bits) {
		for range poolSize {
			x := value{kind: finite, neg: r.IntN(2) == 0, coeff: operandCoeff(r, f.prec), exp: r.IntN(21) - 10}
			y := value{kind: finite, neg: r.IntN(2) == 0, coeff: operandCoeff(r, f.prec), exp: r.IntN(21) - 10}
			z := value{kind: finite, neg: r.IntN(2) == 0, coeff: operandCoeff(r, f.prec), exp: r.IntN(21) - 10}
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
				p := f.mul(xb, yb, rne)
				zb = f.withSign(p, !f.signbit(p))
			}
			xs, ys, zs = append(xs, xb), append(ys, yb), append(zs, zb)
		}
		return xs, ys, zs
	}
	bo.fmaMix = &opnds{xf: bo.mixed.xf, yf: bo.mixed.yf, zf: bo.mixed.zf}
	bo.fmaMix.x64, bo.fmaMix.y64, bo.fmaMix.z64 = func() (x, y, z []uint64) {
		a, b, c := fmaMix(dec64)
		return lows(a), lows(b), lows(c)
	}()
	bo.fmaMix.x128, bo.fmaMix.y128, bo.fmaMix.z128 = fmaMix(dec128)

	// The gap classes: the sticky-bit alignment of Add and FMA, and the
	// longest exponentiation of Rem and Mod.
	bo.far = fixedOpnds(
		[3]string{"1E+300", "1E+3000"}, [3]string{"-1.5E-300", "-1.5E-3000"}, [3]string{},
		1e300, -1.5e-300, 0)
	bo.farFMA = fixedOpnds(
		[3]string{"1E+200", "1E+3000"}, [3]string{"1E+100", "1E+100"}, [3]string{"-1.5E-300", "-1.5E-3000"},
		1e200, 1e100, -1.5e-300)
	bo.remSmall = fixedOpnds(
		[3]string{"1234567890123456", "1234567890123456789012345678901234"}, [3]string{"9876543", "98765432109876543210"}, [3]string{},
		1234567890123456, 9876543, 0)
	bo.remMax = fixedOpnds(
		[3]string{"9999999999999999E+369", "9999999999999999999999999999999999E+6111"},
		[3]string{"9999999999999997E-398", "9999999999999999999999999999999997E-6176"}, [3]string{},
		math.MaxFloat64, 5e-324, 0)

	// Division: an inexact quotient of two Precision-digit coefficients, an
	// exact quotient with trailing zeros to strip, the widest scaling, and
	// the special-value exit.
	bo.divInexact = fixedOpnds(
		[3]string{"1234567890123456", "1234567890123456789012345678901234"},
		[3]string{"7777777777777777", "7777777777777777777777777777777777"}, [3]string{},
		1234567890123456, 7777777777777777, 0)
	bo.divExact = fixedOpnds([3]string{"8.00", "8.00"}, [3]string{"2", "2"}, [3]string{}, 8, 2, 0)
	bo.divWidest = fixedOpnds(
		[3]string{"1", "1"}, [3]string{"9999999999999999", "9999999999999999999999999999999999"}, [3]string{},
		1, 9999999999999999, 0)
	bo.divByZero = fixedOpnds([3]string{"1", "1"}, [3]string{"0", "0"}, [3]string{}, 1, 0, 0)

	// Integers and the values that convert to them: small coefficients with
	// up to three places, so that the value fits an int64 and is mostly
	// inexact.
	frac := func(f *format, signed bool, digits int) []bits {
		bs := make([]bits, poolSize)
		for i := range bs {
			c := new(big.Int).Lsh(new(big.Int).SetUint64(r.Uint64()), 64)
			c.Or(c, new(big.Int).SetUint64(r.Uint64())).Mod(c, pow10(1+r.IntN(digits)))
			bs[i] = f.encode(value{kind: finite, neg: signed && r.IntN(2) == 0, coeff: c, exp: -r.IntN(4)})
		}
		return bs
	}
	fracOpnds := func(signed bool) *opnds {
		o := &opnds{x64: lows(frac(dec64, signed, 15)), x128: frac(dec128, signed, 18)}
		for i := range poolSize {
			f := float64(o.x64[i&(poolSize-1)] & (1<<50 - 1))
			if signed && i%2 == 0 {
				f = -f
			}
			o.xf = append(o.xf, f/10)
		}
		return o
	}
	bo.fracSigned, bo.fracPositive = fracOpnds(true), fracOpnds(false)
	for range poolSize {
		bo.ints = append(bo.ints, int64(r.Uint64())>>r.IntN(64))
		bo.uints = append(bo.uints, r.Uint64()>>r.IntN(64))
	}
	return bo
}

// benchRow is one sub-benchmark.
type benchRow struct {
	name   string // Op/Width/Class
	covers string // the exported function the row measures; empty for a reference row
	fn     func(b *testing.B)
}

type benchRows []benchRow

func (rs *benchRows) add(op, width, class, covers string, fn func(b *testing.B)) {
	*rs = append(*rs, benchRow{name: op + "/" + width + "/" + class, covers: covers, fn: fn})
}

// apiName is the exported function that a row of op at width measures.
func apiName(op, width string) string {
	switch op {
	case "Widen", "Narrow":
		return op
	}
	return op + width
}

// The row builders. Each registers the decimal64 row, the decimal128 row and
// the float64 reference row of an operation, over one class of operands. A
// nil function means no row at that width.

func reg1[R64, R128, RF any](rs *benchRows, op, class string, o *opnds, f64 func(x uint64) R64, f128 func(x bits) R128, ff func(x float64) RF) {
	if f64 != nil {
		rs.add(op, "64", class, apiName(op, "64"), func(b *testing.B) { loop1(b, o.x64, f64) })
	}
	if f128 != nil {
		rs.add(op, "128", class, apiName(op, "128"), func(b *testing.B) { loop1(b, o.x128, f128) })
	}
	if ff != nil {
		rs.add(op, "float64", class, "", func(b *testing.B) { loop1(b, o.xf, ff) })
	}
}

func reg2[R64, R128, RF any](rs *benchRows, op, class string, o *opnds, f64 func(x, y uint64) R64, f128 func(x, y bits) R128, ff func(x, y float64) RF) {
	if f64 != nil {
		rs.add(op, "64", class, apiName(op, "64"), func(b *testing.B) { loop2(b, o.x64, o.y64, f64) })
	}
	if f128 != nil {
		rs.add(op, "128", class, apiName(op, "128"), func(b *testing.B) { loop2(b, o.x128, o.y128, f128) })
	}
	if ff != nil {
		rs.add(op, "float64", class, "", func(b *testing.B) { loop2(b, o.xf, o.yf, ff) })
	}
}

func reg3[R64, R128, RF any](rs *benchRows, op, class string, o *opnds, f64 func(x, y, z uint64) R64, f128 func(x, y, z bits) R128, ff func(x, y, z float64) RF) {
	if f64 != nil {
		rs.add(op, "64", class, apiName(op, "64"), func(b *testing.B) { loop3(b, o.x64, o.y64, o.z64, f64) })
	}
	if f128 != nil {
		rs.add(op, "128", class, apiName(op, "128"), func(b *testing.B) { loop3(b, o.x128, o.y128, o.z128, f128) })
	}
	if ff != nil {
		rs.add(op, "float64", class, "", func(b *testing.B) { loop3(b, o.xf, o.yf, o.zf, ff) })
	}
}

// benchRowList returns every row. It is the single table: BenchmarkKernel,
// TestBenchCoversAPI and TestBenchRatchet all read it.
func benchRowList() benchRows {
	var rs benchRows
	bo := buildBenchOperands()
	mixed := bo.mixed

	// Arithmetic.
	reg2(&rs, "Add", "mixed", bo.addMix,
		func(x, y uint64) uint64 { return bid.Add64(x, y, rne) },
		func(x, y bits) bits { return bid.Add128(x, y, rne) },
		func(x, y float64) float64 { return x + y })
	reg2(&rs, "Add", "far", bo.far,
		func(x, y uint64) uint64 { return bid.Add64(x, y, bid.TowardZero) },
		func(x, y bits) bits { return bid.Add128(x, y, bid.TowardZero) },
		func(x, y float64) float64 { return x + y })
	reg2(&rs, "Sub", "mixed", bo.addMix,
		func(x, y uint64) uint64 { return bid.Sub64(x, y, rne) },
		func(x, y bits) bits { return bid.Sub128(x, y, rne) },
		func(x, y float64) float64 { return x - y })
	reg2(&rs, "Mul", "mixed", mixed,
		func(x, y uint64) uint64 { return bid.Mul64(x, y, rne) },
		func(x, y bits) bits { return bid.Mul128(x, y, rne) },
		func(x, y float64) float64 { return x * y })
	for _, c := range []struct {
		class string
		o     *opnds
	}{{"inexact", bo.divInexact}, {"exact", bo.divExact}, {"widest", bo.divWidest}, {"byzero", bo.divByZero}} {
		reg2(&rs, "Div", c.class, c.o,
			func(x, y uint64) uint64 { return bid.Div64(x, y, rne) },
			func(x, y bits) bits { return bid.Div128(x, y, rne) },
			func(x, y float64) float64 { return x / y })
	}
	reg3(&rs, "FMA", "mixed", bo.fmaMix,
		func(x, y, z uint64) uint64 { return bid.FMA64(x, y, z, rne) },
		func(x, y, z bits) bits { return bid.FMA128(x, y, z, rne) },
		func(x, y, z float64) float64 { return math.FMA(x, y, z) })
	reg3(&rs, "FMA", "far", bo.farFMA,
		func(x, y, z uint64) uint64 { return bid.FMA64(x, y, z, bid.TowardZero) },
		func(x, y, z bits) bits { return bid.FMA128(x, y, z, bid.TowardZero) },
		func(x, y, z float64) float64 { return math.FMA(x, y, z) })
	for _, c := range []struct {
		class string
		o     *opnds
	}{{"smallgap", bo.remSmall}, {"maxgap", bo.remMax}} {
		reg2(&rs, "Rem", c.class, c.o, bid.Rem64, bid.Rem128, math.Remainder)
		reg2(&rs, "Mod", c.class, c.o, bid.Mod64, bid.Mod128, math.Mod)
	}

	// Rounding, quantum, ordering of operands.
	reg1(&rs, "RoundToIntegral", "mixed", mixed,
		func(x uint64) uint64 { return bid.RoundToIntegral64(x, rne) },
		func(x bits) bits { return bid.RoundToIntegral128(x, rne) },
		math.RoundToEven)
	reg2(&rs, "Quantize", "mixed", mixed,
		func(x, y uint64) uint64 { return bid.Quantize64(x, y, rne) },
		func(x, y bits) bits { return bid.Quantize128(x, y, rne) },
		(func(x, y float64) float64)(nil))
	reg1(&rs, "Reduce", "mixed", mixed, bid.Reduce64, bid.Reduce128, (func(float64) float64)(nil))
	reg2(&rs, "SameQuantum", "mixed", mixed, bid.SameQuantum64, bid.SameQuantum128, (func(x, y float64) bool)(nil))
	reg2(&rs, "Min", "mixed", mixed, bid.Min64, bid.Min128, math.Min)
	reg2(&rs, "Max", "mixed", mixed, bid.Max64, bid.Max128, math.Max)

	// Comparison.
	reg2(&rs, "Equal", "mixed", mixed, bid.Equal64, bid.Equal128, func(x, y float64) bool { return x == y })
	reg2(&rs, "Less", "mixed", mixed, bid.Less64, bid.Less128, func(x, y float64) bool { return x < y })
	reg2(&rs, "LessEqual", "mixed", mixed, bid.LessEqual64, bid.LessEqual128, func(x, y float64) bool { return x <= y })
	reg2(&rs, "Compare", "mixed", mixed,
		func(x, y uint64) int { c, _ := bid.Compare64(x, y); return c },
		func(x, y bits) int { c, _ := bid.Compare128(x, y); return c },
		func(x, y float64) int { return cmp.Compare(x, y) })
	reg2(&rs, "TotalOrder", "mixed", mixed, bid.TotalOrder64, bid.TotalOrder128, (func(x, y float64) bool)(nil))
	reg1(&rs, "Key", "mixed", mixed, bid.Key64, bid.Key128, (func(float64) uint64)(nil))

	// Encoding.
	reg1(&rs, "Unpack", "mixed", mixed,
		func(x uint64) uint64 { _, _, c, _ := bid.Unpack64(x); return c },
		func(x bits) bits { _, _, c, _ := bid.Unpack128(x); return c },
		(func(float64) uint64)(nil))
	packs := func(f *format, xs []bits) (neg []bool, c []bits, e []int) {
		for _, x := range xs {
			_, n, co, ex := f.unpack(x)
			neg, c, e = append(neg, n), append(c, toUint128(co)), append(e, ex)
		}
		return neg, c, e
	}
	neg64, c64, e64 := packs(dec64, func() (r []bits) {
		for _, x := range mixed.x64 {
			r = append(r, bits{Lo: x})
		}
		return r
	}())
	neg128, c128, e128 := packs(dec128, mixed.x128)
	rs.add("Pack", "64", "mixed", "Pack64", func(b *testing.B) {
		var s uint64
		for i := 0; i < b.N; i++ {
			j := i & (poolSize - 1)
			s, _ = bid.Pack64(neg64[j], c64[j].Lo, e64[j])
		}
		benchSink = s
	})
	rs.add("Pack", "128", "mixed", "Pack128", func(b *testing.B) {
		var s bits
		for i := 0; i < b.N; i++ {
			j := i & (poolSize - 1)
			s, _ = bid.Pack128(neg128[j], c128[j], e128[j])
		}
		benchSink = s
	})
	reg1(&rs, "IsNaN", "mixed", mixed, bid.IsNaN64, bid.IsNaN128, math.IsNaN)
	reg1(&rs, "IsSignaling", "mixed", mixed, bid.IsSignaling64, bid.IsSignaling128, (func(float64) bool)(nil))
	reg1(&rs, "IsInf", "mixed", mixed, bid.IsInf64, bid.IsInf128, func(x float64) bool { return math.IsInf(x, 0) })
	reg1(&rs, "IsFinite", "mixed", mixed, bid.IsFinite64, bid.IsFinite128, (func(float64) bool)(nil))
	reg1(&rs, "IsZero", "mixed", mixed, bid.IsZero64, bid.IsZero128, func(x float64) bool { return x == 0 })
	reg1(&rs, "IsSubnormal", "mixed", mixed, bid.IsSubnormal64, bid.IsSubnormal128, (func(float64) bool)(nil))
	reg1(&rs, "IsCanonical", "mixed", mixed, bid.IsCanonical64, bid.IsCanonical128, (func(float64) bool)(nil))
	reg1(&rs, "Signbit", "mixed", mixed, bid.Signbit64, bid.Signbit128, math.Signbit)
	reg1(&rs, "Neg", "mixed", mixed, bid.Neg64, bid.Neg128, func(x float64) float64 { return -x })
	reg1(&rs, "Abs", "mixed", mixed, bid.Abs64, bid.Abs128, math.Abs)
	reg2(&rs, "CopySign", "mixed", mixed, bid.CopySign64, bid.CopySign128, math.Copysign)

	// Round: the packing routine the operations end in. The classes are
	// exact at the precision, one digit over it, a wide coefficient with a
	// sticky bit, and a result in the subnormal range.
	roundCases := []struct {
		class  string
		c64    uint64
		c128   string
		e64    int
		e128   int
		sticky bool
	}{
		{"exact", 1234567890123456, "1234567890123456789012345678901234", -5, -5, false},
		{"inexact", 12345678901234567, "12345678901234567890123456789012345", -5, -5, false},
		{"wide", 1234567890123456789, "340282366920938463463374607431768211455", -5, -5, true},
		{"subnormal", 12345678901234567, "12345678901234567890123456789012345", -410, -6190, false},
	}
	for _, c := range roundCases {
		coeff := toUint128(mustBig(c.c128))
		rs.add("Round", "64", c.class, "Round64", func(b *testing.B) {
			var s uint64
			for i := 0; i < b.N; i++ {
				s, _ = bid.Round64(i&1 != 0, c.c64, c.e64, c.sticky, bid.Mode(i%5))
			}
			benchSink = s
		})
		rs.add("Round", "128", c.class, "Round128", func(b *testing.B) {
			var s bits
			for i := 0; i < b.N; i++ {
				s, _ = bid.Round128(i&1 != 0, coeff, c.e128, c.sticky, bid.Mode(i%5))
			}
			benchSink = s
		})
	}

	// Conversions between the decimal widths.
	rs.add("Widen", "64", "mixed", "Widen", func(b *testing.B) { loop1(b, mixed.x64, bid.Widen) })
	rs.add("Widen", "float64", "mixed", "", func(b *testing.B) {
		fs := make([]float32, len(mixed.xf))
		for i, f := range mixed.xf {
			fs[i] = float32(f)
		}
		loop1(b, fs, func(x float32) float64 { return float64(x) })
	})
	rs.add("Narrow", "128", "mixed", "Narrow", func(b *testing.B) {
		loop1(b, mixed.x128, func(x bits) uint64 { return bid.Narrow(x, rne) })
	})
	rs.add("Narrow", "float64", "mixed", "", func(b *testing.B) {
		loop1(b, mixed.xf, func(x float64) float32 { return float32(x) })
	})

	// Integers.
	intTo := func(op string, ints []int64, f64 func(int64) uint64, f128 func(int64) bits, ff func(int64) float64) {
		rs.add(op, "64", "mixed", op+"64", func(b *testing.B) { loop1(b, ints, f64) })
		rs.add(op, "128", "mixed", op+"128", func(b *testing.B) { loop1(b, ints, f128) })
		rs.add(op, "float64", "mixed", "", func(b *testing.B) { loop1(b, ints, ff) })
	}
	intTo("Int64To", bo.ints,
		func(v int64) uint64 { return bid.Int64To64(v, rne) },
		bid.Int64To128,
		func(v int64) float64 { return float64(v) })
	uints := bo.uints
	rs.add("Uint64To", "64", "mixed", "Uint64To64", func(b *testing.B) {
		loop1(b, uints, func(v uint64) uint64 { return bid.Uint64To64(v, rne) })
	})
	rs.add("Uint64To", "128", "mixed", "Uint64To128", func(b *testing.B) { loop1(b, uints, bid.Uint64To128) })
	rs.add("Uint64To", "float64", "mixed", "", func(b *testing.B) {
		loop1(b, uints, func(v uint64) float64 { return float64(v) })
	})
	reg1(&rs, "Int64From", "fraction", bo.fracSigned,
		func(x uint64) int64 { v, _ := bid.Int64From64(x, rne); return v },
		func(x bits) int64 { v, _ := bid.Int64From128(x, rne); return v },
		func(x float64) int64 { return int64(x) })
	reg1(&rs, "Uint64From", "fraction", bo.fracPositive,
		func(x uint64) uint64 { v, _ := bid.Uint64From64(x, rne); return v },
		func(x bits) uint64 { v, _ := bid.Uint64From128(x, rne); return v },
		func(x float64) uint64 { return uint64(x) })

	// Binary floating point to decimal. The classes are the paths: an
	// integer that fits, an exact fraction, the typical inexact fraction, an
	// integer divided down, and the two ends of the range, which are the
	// worst cases of the two multiword computations.
	floatToCases := []struct {
		class string
		v     float64
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
	for _, c := range floatToCases {
		rs.add("Float64To", "64", c.class, "Float64To64", func(b *testing.B) {
			loop1(b, []float64{c.v}, func(v float64) uint64 { return bid.Float64To64(v, rne) })
		})
		rs.add("Float64To", "128", c.class, "Float64To128", func(b *testing.B) {
			loop1(b, []float64{c.v}, func(v float64) bits { return bid.Float64To128(v, rne) })
		})
	}
	float32ToCases := []struct {
		class string
		v     float32
	}{
		{"integer", 123456789},
		{"exact-fraction", 0.5},
		{"typical", 0.1},
		{"typical-large", 123.456},
		{"1e23", 1e23},
		{"max", math.MaxFloat32},
		{"min-subnormal", 1e-45},
	}
	for _, c := range float32ToCases {
		rs.add("Float32To", "64", c.class, "Float32To64", func(b *testing.B) {
			loop1(b, []float32{c.v}, func(v float32) uint64 { return bid.Float32To64(v, rne) })
		})
		rs.add("Float32To", "128", c.class, "Float32To128", func(b *testing.B) {
			loop1(b, []float32{c.v}, func(v float32) bits { return bid.Float32To128(v, rne) })
		})
	}

	// Decimal to binary floating point. The classes: a short coefficient
	// and a long one, which take the fast scaling path; a subnormal and a
	// directed mode, which take the exact path; and an exponent decided by
	// size.
	floatFromCases := []struct {
		class string
		c64   string
		e64   int
		c128  string
		e128  int
		mode  bid.Mode
	}{
		{"short", "12345", -2, "12345", -2, rne},
		{"long", "1234567890123456", -10, "1234567890123456789012345678901234", -30, rne},
		{"subnormal", "1234567890123456", -340, "1234567890123456789012345678901234", -350, rne},
		{"directed", "1234567890123456", -10, "1234567890123456789012345678901234", -30, bid.TowardZero},
		{"directed-extreme", "1234567890123456", 293, "1234567890123456789012345678901234", 270, bid.TowardZero},
		{"far", "1234567890123456", 369, "1234567890123456789012345678901234", 5000, rne},
	}
	for _, c := range floatFromCases {
		x64 := []uint64{decimalOf(dec64, false, c.c64, c.e64).Lo}
		x128 := []bits{decimalOf(dec128, false, c.c128, c.e128)}
		rs.add("Float64From", "64", c.class, "Float64From64", func(b *testing.B) {
			loop1(b, x64, func(x uint64) float64 { return bid.Float64From64(x, c.mode) })
		})
		rs.add("Float64From", "128", c.class, "Float64From128", func(b *testing.B) {
			loop1(b, x128, func(x bits) float64 { return bid.Float64From128(x, c.mode) })
		})
		rs.add("Float32From", "64", c.class, "Float32From64", func(b *testing.B) {
			loop1(b, x64, func(x uint64) float32 { return bid.Float32From64(x, c.mode) })
		})
		rs.add("Float32From", "128", c.class, "Float32From128", func(b *testing.B) {
			loop1(b, x128, func(x bits) float32 { return bid.Float32From128(x, c.mode) })
		})
	}

	// Text. The reference rows are strconv.
	parseCases := []struct{ class, s string }{
		{"short", "123.456"},
		{"long", "1234567890123456789012345678901234.5678"},
		{"exponent", "-1.5E+300"},
		{"nan", "NaN"},
		{"digits", "0." + strings.Repeat("0", 3000) + "123456789012345678901234567890123456789E+99999999999999999999"},
	}
	for _, c := range parseCases {
		rs.add("Parse", "64", c.class, "Parse64", func(b *testing.B) {
			loop1(b, []string{c.s}, func(s string) uint64 { x, _ := bid.Parse64(s, rne); return x })
		})
		rs.add("Parse", "128", c.class, "Parse128", func(b *testing.B) {
			loop1(b, []string{c.s}, func(s string) bits { x, _ := bid.Parse128(s, rne); return x })
		})
		if c.class != "digits" {
			rs.add("Parse", "float64", c.class, "", func(b *testing.B) {
				loop1(b, []string{c.s}, func(s string) float64 { x, _ := strconv.ParseFloat(s, 64); return x })
			})
		}
	}
	buf := make([]byte, 0, bid.MaxLen128)
	rs.add("Append", "64", "mixed", "Append64", func(b *testing.B) {
		loop1(b, mixed.x64, func(x uint64) int { return len(bid.Append64(buf[:0], x)) })
	})
	rs.add("Append", "128", "mixed", "Append128", func(b *testing.B) {
		loop1(b, mixed.x128, func(x bits) int { return len(bid.Append128(buf[:0], x)) })
	})
	rs.add("Append", "float64", "mixed", "", func(b *testing.B) {
		loop1(b, mixed.xf, func(x float64) int { return len(strconv.AppendFloat(buf[:0], x, 'E', -1, 64)) })
	})

	return rs
}

func BenchmarkKernel(b *testing.B) {
	for _, row := range benchRowList() {
		b.Run(row.name, row.fn)
	}
}

// TestBenchCoversAPI fails for an exported function that no row measures and
// for a row that names a function that does not exist, and for a duplicate
// row name.
func TestBenchCoversAPI(t *testing.T) {
	rows := benchRowList()
	covered := make(map[string]bool)
	names := make(map[string]bool)
	api := make(map[string]bool)
	for _, name := range exportedAPI(t) {
		api[name] = true
	}
	for _, row := range rows {
		if names[row.name] {
			t.Errorf("duplicate benchmark row %s", row.name)
		}
		names[row.name] = true
		if row.covers != "" {
			covered[row.covers] = true
			if !api[row.covers] {
				t.Errorf("row %s measures %s, which is not an exported function", row.name, row.covers)
			}
		}
	}
	for name := range api {
		if !covered[name] {
			t.Errorf("exported function %s has no benchmark row", name)
		}
	}
	t.Logf("%d rows over %d exported functions", len(rows), len(api))
}

// The baseline.

// baselineSample is one line of a benchstat file.
type baselineSample struct {
	ns     float64
	allocs float64
}

// benchLine matches "BenchmarkKernel/Op/Width/Class-16   N   X ns/op ... Y allocs/op".
var benchLine = regexp.MustCompile(`^BenchmarkKernel/(\S+?)(?:-\d+)?\s+\d+\s+([0-9.]+) ns/op(?:.*?\s([0-9.]+) allocs/op)?`)

// readBaseline returns the samples of each row in the baseline file of the
// current platform, and the noise band and the absolute slack the file states,
// or ok false when no baseline is recorded for the platform.
func readBaseline(t testing.TB) (samples map[string][]baselineSample, band, slack float64, path string, ok bool) {
	t.Helper()
	path = fmt.Sprintf("testdata/bench_baseline_%s_%s.txt", runtime.GOOS, runtime.GOARCH)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, 0, 0, path, false
	}
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	samples = make(map[string][]baselineSample)
	band, slack = -1, -1
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if v, found := strings.CutPrefix(line, "# ratchet-band:"); found {
			band, err = strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				t.Fatalf("%s: bad ratchet-band: %v", path, err)
			}
			continue
		}
		if v, found := strings.CutPrefix(line, "# ratchet-slack-ns:"); found {
			slack, err = strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				t.Fatalf("%s: bad ratchet-slack-ns: %v", path, err)
			}
			continue
		}
		m := benchLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ns, _ := strconv.ParseFloat(m[2], 64)
		var allocs float64
		if m[3] != "" {
			allocs, _ = strconv.ParseFloat(m[3], 64)
		}
		samples[m[1]] = append(samples[m[1]], baselineSample{ns, allocs})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if band < 0 || slack < 0 {
		t.Fatalf("%s states no '# ratchet-band:' or no '# ratchet-slack-ns:'", path)
	}
	return samples, band, slack, path, true
}

// TestBenchBaselineShape checks, without running a benchmark, that the
// baseline names exactly the rows of the table, so that a row added or
// removed without retaking the baseline fails by default and not only under
// -bid.ratchet.
func TestBenchBaselineShape(t *testing.T) {
	samples, band, slack, path, ok := readBaseline(t)
	if !ok {
		t.Skipf("no baseline %s for this platform", path)
	}
	rows := make(map[string]bool)
	for _, row := range benchRowList() {
		rows[row.name] = true
		if len(samples[row.name]) == 0 {
			t.Errorf("row %s has no sample in %s", row.name, path)
		}
	}
	for name := range samples {
		if !rows[name] {
			t.Errorf("%s has samples for %s, which is not a row", path, name)
		}
	}
	for name, ss := range samples {
		for _, s := range ss {
			if s.allocs != 0 {
				t.Errorf("%s: row %s allocates in the baseline: %v allocs/op", path, name, s.allocs)
			}
		}
	}
	t.Logf("%s: %d rows, band %.0f%%, slack %.2f ns", path, len(samples), band*100, slack)
}

// TestBenchRatchet compares short runs of the rows with the baseline and
// fails a row that is slower than the baseline by more than the noise band,
// or that allocates. It runs only under -bid.ratchet.
//
// The time of a row is the smallest of its measurements, in the baseline and
// here, since interference from other work on the machine only ever adds
// time. A row over the band is measured again, up to -bid.ratchet.retry more
// times, before it fails. The limit of a row is the baseline time times
// (1 + band) plus the file's absolute slack, which stands for the timer
// resolution on rows that take about a nanosecond. A row faster than the
// baseline by more than the band is logged, not failed: retake the baseline
// by a deliberate commit to lock the improvement.
func TestBenchRatchet(t *testing.T) {
	if !*ratchetFlag {
		t.Skip("run with -bid.ratchet to compare with the benchmark baseline")
	}
	samples, band, slack, path, ok := readBaseline(t)
	if !ok {
		t.Skipf("no baseline %s for this platform; it is recorded by the CI of the platform", path)
	}
	oldTime := flag.Lookup("test.benchtime").Value.String()
	if err := flag.Set("test.benchtime", ratchetTime.String()); err != nil {
		t.Fatal(err)
	}
	defer flag.Set("test.benchtime", oldTime)

	measure := func(row benchRow) (ns, allocs float64) {
		r := testing.Benchmark(func(b *testing.B) {
			b.ReportAllocs()
			row.fn(b)
		})
		return float64(r.T.Nanoseconds()) / float64(r.N), float64(r.AllocsPerOp())
	}
	var failed, faster int
	for _, row := range benchRowList() {
		ss := samples[row.name]
		if len(ss) == 0 {
			t.Errorf("row %s has no sample in %s", row.name, path)
			continue
		}
		base := slices.MinFunc(ss, func(a, b baselineSample) int { return cmp.Compare(a.ns, b.ns) })
		limit := base.ns*(1+band) + slack
		best, allocs := math.Inf(1), 0.0
		for n := 0; n < *ratchetCount+*ratchetRetry; n++ {
			if n >= *ratchetCount && best <= limit {
				break
			}
			ns, a := measure(row)
			best, allocs = min(best, ns), max(allocs, a)
		}
		switch {
		case allocs != 0:
			failed++
			t.Errorf("%s: %v allocs/op", row.name, allocs)
		case best > limit:
			failed++
			t.Errorf("%s: %.2f ns/op, over the limit %.2f ns/op (baseline %.2f ns/op, band %.0f%%, slack %.2f ns)",
				row.name, best, limit, base.ns, band*100, slack)
		case best*(1+band)+slack < base.ns:
			faster++
			t.Logf("%s: %.2f ns/op, faster than the baseline %.2f ns/op by more than the band", row.name, best, base.ns)
		}
	}
	if faster > 0 {
		t.Logf("%d rows are faster than the baseline by more than the band; retake the baseline to lock them", faster)
	}
	if failed == 0 {
		t.Logf("ratchet held on %d rows within %.0f%% of %s", len(samples), band*100, path)
	}
}
