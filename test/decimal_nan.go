// run
//go:build goexperiment.decimal

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Test that the decimal operators and the math/decimal functions propagate
// a NaN operand's sign and payload, made quiet, as float64 arithmetic does
// (internal/bid, "NaN results"), and that a NaN created by an invalid
// operation is the canonical one. Where the hardware's float64 result does
// not depend on the order the compiler gives the operands, the decimal
// result is also compared with the float64 one on the architectures whose
// float64 arithmetic propagates payloads.

package main

import (
	"fmt"
	"math"
	"math/decimal"
	"runtime"
)

// nan is a NaN reduced to what the test compares: the sign, whether it is
// signaling, and the payload.
type nan struct {
	neg, signaling bool
	payload        uint64
}

func (n nan) String() string {
	s := "NaN"
	if n.signaling {
		s = "sNaN"
	}
	if n.neg {
		s = "-" + s
	}
	return fmt.Sprintf("%s(%#x)", s, n.payload)
}

// d64 returns the NaN of the decimal64 x, with ok = false if x is not one.
func d64(x decimal64) (n nan, ok bool) {
	b := decimal.BID64bits(x)
	if b>>58&0x1F != 0x1F {
		return nan{}, false
	}
	return nan{neg: b>>63 != 0, signaling: b>>57&1 != 0, payload: b & (1<<50 - 1)}, true
}

// d128 returns the NaN of the decimal128 x; its payload must fit 64 bits.
func d128(x decimal128) (n nan, ok bool) {
	hi, lo := decimal.BID128bits(x)
	if hi>>58&0x1F != 0x1F || hi&(1<<46-1) != 0 {
		return nan{}, false
	}
	return nan{neg: hi>>63 != 0, signaling: hi>>57&1 != 0, payload: lo}, true
}

// f64 returns the NaN of the float64 x.
func f64(x float64) (n nan, ok bool) {
	b := math.Float64bits(x)
	if !math.IsNaN(x) {
		return nan{}, false
	}
	return nan{neg: b>>63 != 0, signaling: b>>51&1 == 0, payload: b & (1<<51 - 1)}, true
}

// The operands, built at run time so that nothing is constant-folded: a
// quiet NaN with payload 0x123, a negative quiet NaN with payload 0x45 and
// a signaling NaN with payload 0x67, in each type.
var (
	qd, nd, sd    decimal64
	qd2, nd2, sd2 decimal128
	qf, nf, sf    float64
	one           = 1.0
	oneD          decimal64
	oneD2         decimal128
)

//go:noinline
func setup() {
	qd = decimal.BID64frombits(0x7c00000000000123)
	nd = decimal.BID64frombits(0xfc00000000000045)
	sd = decimal.BID64frombits(0x7e00000000000067)
	qd2 = decimal.BID128frombits(0x7c00000000000000, 0x123)
	nd2 = decimal.BID128frombits(0xfc00000000000000, 0x45)
	sd2 = decimal.BID128frombits(0x7e00000000000000, 0x67)
	qf = math.Float64frombits(0x7ff8000000000123)
	nf = math.Float64frombits(0xfff8000000000045)
	sf = math.Float64frombits(0x7ff0000000000067)
	oneD, oneD2 = 1, 1
}

var (
	quiet123 = nan{payload: 0x123}
	neg45    = nan{neg: true, payload: 0x45}
	quiet67  = nan{payload: 0x67}
	canon    = nan{}
)

var failed bool

func check(name string, got nan, ok bool, want nan) {
	if !ok || got != want {
		fmt.Printf("%s = %v (NaN %v), want %v\n", name, got, ok, want)
		failed = true
	}
}

// float reports whether float64 arithmetic on this architecture propagates
// the payload of a single NaN operand, made quiet, in every operation the
// test compares; signalingFirst, whether it also picks a signaling NaN over
// a quiet one, whichever operand each is. Elsewhere only the decimal result
// is checked.
var (
	float          = runtime.GOARCH == "arm64" || runtime.GOARCH == "amd64"
	signalingFirst = runtime.GOARCH == "arm64"
	floatMinMax    = runtime.GOARCH == "arm64"
)

// both checks a decimal64 result against want and, if cmp, against the
// float64 result of the same expression.
func both(name string, d decimal64, f float64, want nan, cmp bool) {
	g, ok := d64(d)
	check(name, g, ok, want)
	if cmp {
		fg, fok := f64(f)
		check(name+" (float64)", fg, fok, want)
	}
}

func main() {
	setup()

	// One NaN operand: that NaN, made quiet, with its sign and payload.
	both("x + 1", qd+oneD, qf+one, quiet123, float)
	both("1 + x", oneD+qd, one+qf, quiet123, float)
	both("x - 1", qd-oneD, qf-one, quiet123, float)
	both("1 - n", oneD-nd, one-nf, neg45, float) // a NaN y is not negated
	both("x * 2.5", qd*2.5, qf*2.5, quiet123, float)
	both("0 * n", 0*nd, 0*nf, neg45, float)
	both("x / 3", qd/3, qf/3, quiet123, float)
	both("3 / n", 3/nd, 3/nf, neg45, float)
	both("s + 1", sd+oneD, sf+one, quiet67, float)
	both("-x", -qd, -qf, nan{neg: true, payload: 0x123}, float)
	both("FMA(x, 1, 1)", decimal.FMA(qd, 1, 1), math.FMA(qf, 1, 1), quiet123, float)
	both("FMA(1, 1, n)", decimal.FMA(1, 1, nd), math.FMA(1, 1, nf), neg45, float)
	both("min(x, 1)", min(qd, oneD), min(qf, one), quiet123, floatMinMax)
	both("max(1, n)", max(oneD, nd), max(one, nf), neg45, floatMinMax)

	// A signaling NaN wins over a quiet one, whichever operand it is.
	both("x + s", qd+sd, qf+sf, quiet67, signalingFirst)
	both("s * x", sd*qd, sf*qf, quiet67, signalingFirst)
	both("min(x, s)", min(qd, sd), min(qf, sf), quiet67, signalingFirst && floatMinMax)

	// Two quiet NaNs: the first.
	both("x + n", qd+nd, qf+nf, quiet123, false)
	both("n / x", nd/qd, nf/qf, neg45, false)

	// The math/decimal functions with no float64 counterpart that
	// propagates (math.Mod and math.Remainder return the canonical NaN).
	both("Mod(n, 2)", decimal.Mod(nd, 2), 0, neg45, false)
	both("Remainder(1, s)", decimal.Remainder(1, sd), 0, quiet67, false)
	both("Quantize(x, 0.01)", decimal.Quantize(qd, 0.01), 0, quiet123, false)
	both("Reduce(s)", decimal.Reduce(sd), 0, quiet67, false)
	both("Trunc(n)", decimal.Trunc(nd), 0, neg45, false)
	both("Round(s)", decimal.Round(sd), 0, quiet67, false)
	both("Dim(x, 1)", decimal.Dim(qd, 1), 0, quiet123, false)

	// A NaN that an invalid operation creates is the canonical one.
	inf := decimal.Inf[decimal64](1)
	both("Inf - Inf", inf-inf, 0, canon, false)
	both("0 / 0", (oneD-oneD)/(oneD-oneD), 0, canon, false)

	// decimal128, and the conversions between the widths.
	check128 := func(name string, d decimal128, want nan) {
		g, ok := d128(d)
		check(name, g, ok, want)
	}
	check128("x + 1 (decimal128)", qd2+oneD2, quiet123)
	check128("1 - n (decimal128)", oneD2-nd2, neg45)
	check128("x * s (decimal128)", qd2*sd2, quiet67)
	check128("FMA(1, 1, n) (decimal128)", decimal.FMA(1, 1, nd2), neg45)
	check128("decimal128(n)", decimal128(nd), neg45)
	check128("decimal128(s)", decimal128(sd), quiet67)
	g, ok := d64(decimal64(nd2))
	check("decimal64(n)", g, ok, neg45)
	// A decimal128 payload of 10^15 or more does not fit decimal64.
	big := decimal.BID128frombits(0x7c00000000000000, 1_000_000_000_000_000)
	g, ok = d64(decimal64(big))
	check("decimal64(NaN 10^15)", g, ok, canon)

	if failed {
		panic("decimal NaN payloads are not propagated as float64 propagates them")
	}
}
