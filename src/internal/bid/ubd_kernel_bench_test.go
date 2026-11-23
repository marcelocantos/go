// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	. "internal/bid"
	mbits "math/bits"
	"math/rand/v2"
	"testing"
)

// BenchmarkUBDKernel times the UBD kernel entries the compiler calls at
// -d=ubdlevel=1 against the BID kernel entries the XOR-BID compiler calls
// for the same operation, each on its own encoding of the same operands, so
// neither side converts. Both sides are called through a function value, as
// a call site calls the kernel: the compiler's call is never inlined, and a
// direct call here would let the UBD entries' small wrappers inline into the
// loop. Each row runs over 1024 operand pairs in a fixed pseudo-random order.
//
//	go test internal/bid -run '^$' -bench 'UBDKernel' -count 10
//
// The rows are named Op/class/side, side ubd or bid, for benchstat's
// -col /side.

const ubdBenchN = 1024

type ubdBenchPair struct {
	ux, uy UBD128
	bx, by Uint128
}

// ubdBenchVal returns the BID and UBD encodings of (-1)^neg × c × 10^exp.
func ubdBenchVal(neg bool, c Uint128, exp int) (Uint128, UBD128) {
	b, ok := Pack128(neg, c, exp)
	if !ok {
		panic("ubdBenchVal: not representable")
	}
	u := UBDFromBID128(b)
	return b, UBD128{Lo: u.Lo, Hi: u.Hi}
}

// ubdBenchDigits returns a random coefficient of lo to hi digits.
func ubdBenchDigits(r *rand.Rand, lo, hi int) Uint128 {
	n := lo + r.IntN(hi-lo+1)
	var c Uint128
	for i := 0; i < n; i++ {
		d := uint64(r.IntN(10))
		if i == 0 && d == 0 {
			d = 1
		}
		hi, lo := mbits.Mul64(c.Lo, 10)
		var carry uint64
		c.Lo, carry = mbits.Add64(lo, d, 0)
		c.Hi = c.Hi*10 + hi + carry
	}
	return c
}

// ubdBenchClass describes one operand distribution.
type ubdBenchClass struct {
	name           string
	xlo, xhi, xexp int // x: digits lo..hi at exponent xexp
	ylo, yhi, yexp int
	negPct         int // percent of operands negative
	yexpAlt        int // if nonzero, half of the y operands have this exponent instead
}

var ubdBenchClasses = []ubdBenchClass{
	{"money", 3, 10, -2, 3, 10, -2, 0, 0},       // same quantum, positive
	{"cents10neg", 3, 10, -2, 3, 10, -2, 10, 0}, // same quantum, 10% negative
	{"gap2", 3, 10, -2, 1, 5, -4, 0, 0},         // amount and rate
	{"gap2neg", 3, 10, -2, 1, 5, -4, 10, 0},
	{"int", 1, 6, 0, 1, 6, 0, 0, 0},
	{"w19", 17, 19, -6, 17, 19, -9, 0, 0},  // just inside one word
	{"w25", 22, 25, -6, 22, 25, -9, 10, 0}, // two words, exact
	{"d34", 34, 34, -20, 34, 34, -24, 10, 0},
	{"q34", 30, 34, -28, 1, 6, -2, 10, 0},   // a quotient and an amount: quantize rounds 26 places
	{"mixexp", 3, 10, -2, 1, 8, -2, 10, -4}, // half one exponent, half two apart, at random
}

func ubdBenchPairs(cl ubdBenchClass) []ubdBenchPair {
	r := rand.New(rand.NewPCG(1, uint64(len(cl.name))))
	ps := make([]ubdBenchPair, ubdBenchN)
	for i := range ps {
		bx, ux := ubdBenchVal(r.IntN(100) < cl.negPct, ubdBenchDigits(r, cl.xlo, cl.xhi), cl.xexp)
		yexp := cl.yexp
		if cl.yexpAlt != 0 && r.IntN(2) == 0 {
			yexp = cl.yexpAlt
		}
		by, uy := ubdBenchVal(r.IntN(100) < cl.negPct, ubdBenchDigits(r, cl.ylo, cl.yhi), yexp)
		ps[i] = ubdBenchPair{ux, uy, bx, by}
	}
	return ps
}

var (
	ubdSinkU UBD128
	ubdSinkB Uint128
	ubdSinkI int
)

func ubdBench2(b *testing.B, ps []ubdBenchPair, f func(x, y UBD128) UBD128) {
	var acc UBD128
	for i := 0; i < b.N; i++ {
		p := &ps[i&(ubdBenchN-1)]
		r := f(p.ux, p.uy)
		acc.Lo ^= r.Lo
		acc.Hi ^= r.Hi
	}
	ubdSinkU = acc
}

func bidBench2(b *testing.B, ps []ubdBenchPair, f func(x, y Uint128, mode Mode) Uint128) {
	var acc Uint128
	for i := 0; i < b.N; i++ {
		p := &ps[i&(ubdBenchN-1)]
		r := f(p.bx, p.by, ToNearestEven)
		acc.Lo ^= r.Lo
		acc.Hi ^= r.Hi
	}
	ubdSinkB = acc
}

func BenchmarkUBDKernel(b *testing.B) {
	for _, cl := range ubdBenchClasses {
		ps := ubdBenchPairs(cl)
		b.Run("Add/"+cl.name+"/ubd", func(b *testing.B) { ubdBench2(b, ps, UBDKernelAdd) })
		b.Run("Add/"+cl.name+"/bid", func(b *testing.B) { bidBench2(b, ps, BIDKernelAdd) })
		b.Run("Sub/"+cl.name+"/ubd", func(b *testing.B) { ubdBench2(b, ps, UBDKernelSub) })
		b.Run("Sub/"+cl.name+"/bid", func(b *testing.B) { bidBench2(b, ps, BIDKernelSub) })
		b.Run("Mul/"+cl.name+"/ubd", func(b *testing.B) { ubdBench2(b, ps, UBDKernelMul) })
		b.Run("Mul/"+cl.name+"/bid", func(b *testing.B) { bidBench2(b, ps, BIDKernelMul) })
		b.Run("Div/"+cl.name+"/ubd", func(b *testing.B) { ubdBench2(b, ps, UBDKernelDiv) })
		b.Run("Div/"+cl.name+"/bid", func(b *testing.B) { bidBench2(b, ps, BIDKernelDiv) })
		b.Run("Quantize/"+cl.name+"/ubd", func(b *testing.B) { ubdBench2(b, ps, UBDKernelQnt) })
		b.Run("Quantize/"+cl.name+"/bid", func(b *testing.B) { bidBench2(b, ps, BIDKernelQnt) })
		b.Run("Less/"+cl.name+"/ubd", func(b *testing.B) {
			n := 0
			for i := 0; i < b.N; i++ {
				p := &ps[i&(ubdBenchN-1)]
				if UBDKernelLess(p.ux, p.uy) {
					n++
				}
			}
			ubdSinkI = n
		})
		b.Run("Less/"+cl.name+"/bid", func(b *testing.B) {
			n := 0
			for i := 0; i < b.N; i++ {
				p := &ps[i&(ubdBenchN-1)]
				if c, ok := BIDKernelCmp(p.bx, p.by); ok && c < 0 {
					n++
				}
			}
			ubdSinkI = n
		})
		b.Run("Equal/"+cl.name+"/ubd", func(b *testing.B) {
			n := 0
			for i := 0; i < b.N; i++ {
				p := &ps[i&(ubdBenchN-1)]
				if UBDKernelEqual(p.ux, p.uy) {
					n++
				}
			}
			ubdSinkI = n
		})
		b.Run("Compare/"+cl.name+"/ubd", func(b *testing.B) {
			n := 0
			for i := 0; i < b.N; i++ {
				p := &ps[i&(ubdBenchN-1)]
				if c, ok := UBDKernelCmp(p.ux, p.uy); ok {
					n += c
				}
			}
			ubdSinkI = n
		})
	}
}

// TestUBDKernelBenchAgrees checks that every benchmarked UBD entry gives
// the BID entry's result on the benchmark's operands, so that the rows time
// the same work.
func TestUBDKernelBenchAgrees(t *testing.T) {
	for _, cl := range ubdBenchClasses {
		for i, p := range ubdBenchPairs(cl) {
			check := func(op string, u UBD128, bv Uint128) {
				if got := BIDFromUBDAny128(Uint128{Hi: u.Hi, Lo: u.Lo}); got != bv {
					t.Fatalf("%s/%s pair %d: ubd %#x, bid %#x", op, cl.name, i, got, bv)
				}
			}
			check("Add", UBDKernelAdd(p.ux, p.uy), BIDKernelAdd(p.bx, p.by, ToNearestEven))
			check("Sub", UBDKernelSub(p.ux, p.uy), BIDKernelSub(p.bx, p.by, ToNearestEven))
			check("Mul", UBDKernelMul(p.ux, p.uy), BIDKernelMul(p.bx, p.by, ToNearestEven))
			check("Div", UBDKernelDiv(p.ux, p.uy), BIDKernelDiv(p.bx, p.by, ToNearestEven))
			check("Quantize", UBDKernelQnt(p.ux, p.uy), BIDKernelQnt(p.bx, p.by, ToNearestEven))
			uc, uok := UBDKernelCmp(p.ux, p.uy)
			bc, bok := BIDKernelCmp(p.bx, p.by)
			if uc != bc || uok != bok {
				t.Fatalf("Compare/%s pair %d: ubd %d %v, bid %d %v", cl.name, i, uc, uok, bc, bok)
			}
		}
	}
}

// TestUBDAddWordAgrees checks the ties-to-even add and subtract entries on
// word operands, which their word path (ubdAddWordTTE) decides, against the
// BID kernel: exponent gaps of 0 to 20 either way, coefficients of every
// size up to 2^64 - 1, zeros, and both signs.
func TestUBDAddWordAgrees(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 11))
	coeff := func() uint64 {
		switch r.IntN(6) {
		case 0:
			return 0
		case 1:
			return ^uint64(0) - uint64(r.IntN(3))
		case 2:
			return uint64(r.IntN(1000))
		default:
			return r.Uint64() >> uint(r.IntN(64))
		}
	}
	for i := 0; i < 2000000; i++ {
		xe := r.IntN(60) - 30
		ye := xe + r.IntN(41) - 20
		bx, ux := ubdBenchVal(r.IntN(2) == 0, Uint128{Lo: coeff()}, xe)
		by, uy := ubdBenchVal(r.IntN(2) == 0, Uint128{Lo: coeff()}, ye)
		if got, want := UBDKernelAdd(ux, uy), BIDKernelAdd(bx, by, ToNearestEven); BIDFromUBDAny128(Uint128{Hi: got.Hi, Lo: got.Lo}) != want {
			t.Fatalf("add %#x %#x: got %#x, want %#x", bx, by, BIDFromUBDAny128(Uint128{Hi: got.Hi, Lo: got.Lo}), want)
		}
		if got, want := UBDKernelSub(ux, uy), BIDKernelSub(bx, by, ToNearestEven); BIDFromUBDAny128(Uint128{Hi: got.Hi, Lo: got.Lo}) != want {
			t.Fatalf("sub %#x %#x: got %#x, want %#x", bx, by, BIDFromUBDAny128(Uint128{Hi: got.Hi, Lo: got.Lo}), want)
		}
	}
}

// TestUBDCmpWordAgrees checks the compare and predicate entries on word
// operands, which their word path (ubdCmpWord) decides, against the BID
// kernel: exponent gaps of 0 to 25 either way, zeros of both signs, and
// coefficients of every size.
func TestUBDCmpWordAgrees(t *testing.T) {
	r := rand.New(rand.NewPCG(13, 17))
	coeff := func() uint64 {
		switch r.IntN(5) {
		case 0:
			return 0
		case 1:
			return ^uint64(0) - uint64(r.IntN(3))
		default:
			return r.Uint64() >> uint(r.IntN(64))
		}
	}
	for i := 0; i < 2000000; i++ {
		xe := r.IntN(60) - 30
		ye := xe + r.IntN(51) - 25
		xc := coeff()
		yc := coeff()
		if r.IntN(4) == 0 { // equal values at different exponents
			yc, ye = xc, xe
			for k := r.IntN(3); k > 0 && yc <= ^uint64(0)/10; k-- {
				yc, ye = yc*10, ye-1
			}
		}
		bx, ux := ubdBenchVal(r.IntN(2) == 0, Uint128{Lo: xc}, xe)
		by, uy := ubdBenchVal(r.IntN(2) == 0, Uint128{Lo: yc}, ye)
		wc, wok := BIDKernelCmp(bx, by)
		if c, ok := UBDKernelCmp(ux, uy); c != wc || ok != wok {
			t.Fatalf("compare %#x %#x: got %d %v, want %d %v", bx, by, c, ok, wc, wok)
		}
		if got, want := UBDKernelLess(ux, uy), wok && wc < 0; got != want {
			t.Fatalf("less %#x %#x: got %v, want %v", bx, by, got, want)
		}
		if got, want := UBDKernelEqual(ux, uy), wok && wc == 0; got != want {
			t.Fatalf("equal %#x %#x: got %v, want %v", bx, by, got, want)
		}
	}
}
