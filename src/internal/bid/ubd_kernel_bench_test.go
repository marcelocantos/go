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
	{"w16", 14, 16, -4, 14, 16, -6, 10, 0}, // words whose product passes 2^96 but is exact
	{"w19", 17, 19, -6, 17, 19, -9, 0, 0},  // just inside one word
	{"w25", 22, 25, -6, 22, 25, -9, 10, 0}, // two words, exact
	{"d34", 34, 34, -20, 34, 34, -24, 10, 0},
	{"w2q", 20, 34, -6, 20, 34, -6, 50, 0},  // two words at one exponent, half negative: half the sums differences
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

// BenchmarkUBDKernelCompound times compounding, a chain of a multiply by
// a rate near one and a quantize to cents, each step waiting for the one
// before: acc = quantize(acc × rate, 0.01), restarted every 64 steps.
func BenchmarkUBDKernelCompound(b *testing.B) {
	r := rand.New(rand.NewPCG(29, 31))
	type rate struct {
		u UBD128
		b Uint128
	}
	rates := make([]rate, ubdBenchN)
	for i := range rates {
		rb, ru := ubdBenchVal(false, Uint128{Lo: 10001 + uint64(r.IntN(500))}, -4)
		rates[i] = rate{ru, rb}
	}
	b0, u0 := ubdBenchVal(false, Uint128{Lo: 12345678}, -2)
	bq, uq := ubdBenchVal(false, Uint128{Lo: 1}, -2)
	b.Run("ubd", func(b *testing.B) {
		acc := u0
		for i := 0; i < b.N; i++ {
			acc = UBDKernelQnt(UBDKernelMul(acc, rates[i&(ubdBenchN-1)].u), uq)
			if i&63 == 63 {
				acc = u0
			}
		}
		ubdSinkU = acc
	})
	b.Run("bid", func(b *testing.B) {
		acc := b0
		for i := 0; i < b.N; i++ {
			acc = BIDKernelQnt(BIDKernelMul(acc, rates[i&(ubdBenchN-1)].b, ToNearestEven), bq, ToNearestEven)
			if i&63 == 63 {
				acc = b0
			}
		}
		ubdSinkB = acc
	})
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
		// A running total: each sum waits for the one before, as a loop
		// that accumulates amounts does, so these rows time the latency of
		// a call where the others time its throughput.
		b.Run("AddChain/"+cl.name+"/ubd", func(b *testing.B) {
			acc := ps[0].ux
			for i := 0; i < b.N; i++ {
				acc = UBDKernelAdd(acc, ps[i&(ubdBenchN-1)].uy)
				if i&(ubdBenchN-1) == ubdBenchN-1 {
					acc = ps[0].ux // keep the total from growing past a word
				}
			}
			ubdSinkU = acc
		})
		b.Run("AddChain/"+cl.name+"/bid", func(b *testing.B) {
			acc := ps[0].bx
			for i := 0; i < b.N; i++ {
				acc = BIDKernelAdd(acc, ps[i&(ubdBenchN-1)].by, ToNearestEven)
				if i&(ubdBenchN-1) == ubdBenchN-1 {
					acc = ps[0].bx
				}
			}
			ubdSinkB = acc
		})
		// A running balance: each difference waits for the one before.
		b.Run("SubChain/"+cl.name+"/ubd", func(b *testing.B) {
			acc := ps[0].ux
			for i := 0; i < b.N; i++ {
				acc = UBDKernelSub(acc, ps[i&(ubdBenchN-1)].uy)
				if i&(ubdBenchN-1) == ubdBenchN-1 {
					acc = ps[0].ux
				}
			}
			ubdSinkU = acc
		})
		b.Run("SubChain/"+cl.name+"/bid", func(b *testing.B) {
			acc := ps[0].bx
			for i := 0; i < b.N; i++ {
				acc = BIDKernelSub(acc, ps[i&(ubdBenchN-1)].by, ToNearestEven)
				if i&(ubdBenchN-1) == ubdBenchN-1 {
					acc = ps[0].bx
				}
			}
			ubdSinkB = acc
		})
		// The Rnd entry, which the compiler calls for a mode other than
		// ties to even: telco truncates its taxes.
		b.Run("QuantizeTZ/"+cl.name+"/ubd", func(b *testing.B) {
			ubdBench2(b, ps, func(x, y UBD128) UBD128 { return UBDKernelQntR(x, y, TowardZero) })
		})
		b.Run("QuantizeTZ/"+cl.name+"/bid", func(b *testing.B) {
			bidBench2(b, ps, func(x, y Uint128, _ Mode) Uint128 { return BIDKernelQnt(x, y, TowardZero) })
		})
		b.Run("Min/"+cl.name+"/ubd", func(b *testing.B) { ubdBench2(b, ps, UBDKernelMin) })
		b.Run("Min/"+cl.name+"/bid", func(b *testing.B) {
			bidBench2(b, ps, func(x, y Uint128, _ Mode) Uint128 { return BIDKernelMin(x, y) })
		})
		b.Run("RoundToIntegral/"+cl.name+"/ubd", func(b *testing.B) {
			ubdBench2(b, ps, func(x, _ UBD128) UBD128 { return UBDKernelRTI(x) })
		})
		b.Run("RoundToIntegral/"+cl.name+"/bid", func(b *testing.B) {
			bidBench2(b, ps, func(x, _ Uint128, m Mode) Uint128 { return BIDKernelRTI(x, m) })
		})
		b.Run("Int64/"+cl.name+"/ubd", func(b *testing.B) {
			n := int64(0)
			for i := 0; i < b.N; i++ {
				n += UBDKernelInt64(ps[i&(ubdBenchN-1)].ux)
			}
			ubdSinkI = int(n)
		})
		b.Run("Int64/"+cl.name+"/bid", func(b *testing.B) {
			n := int64(0)
			for i := 0; i < b.N; i++ {
				v, _ := BIDKernelInt64(ps[i&(ubdBenchN-1)].bx, TowardZero)
				n += v
			}
			ubdSinkI = int(n)
		})
		b.Run("Uint64/"+cl.name+"/ubd", func(b *testing.B) {
			n := uint64(0)
			for i := 0; i < b.N; i++ {
				n += UBDKernelUint64(ps[i&(ubdBenchN-1)].ux)
			}
			ubdSinkI = int(n)
		})
		b.Run("Uint64/"+cl.name+"/bid", func(b *testing.B) {
			n := uint64(0)
			for i := 0; i < b.N; i++ {
				v, _ := BIDKernelUint64(ps[i&(ubdBenchN-1)].bx, TowardZero)
				n += v
			}
			ubdSinkI = int(n)
		})
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
			check("Min", UBDKernelMin(p.ux, p.uy), BIDKernelMin(p.bx, p.by))
			check("Max", UBDKernelMax(p.ux, p.uy), BIDKernelMax(p.bx, p.by))
			check("RoundToIntegral", UBDKernelRTI(p.ux), BIDKernelRTI(p.bx, ToNearestEven))
			if got, _ := BIDKernelInt64(p.bx, TowardZero); UBDKernelInt64(p.ux) != got {
				t.Fatalf("Int64/%s pair %d: ubd %d, bid %d", cl.name, i, UBDKernelInt64(p.ux), got)
			}
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

// TestUBDAddWideAgrees checks the ties-to-even add and subtract entries on
// finite operands at one exponent with coefficients of up to two words
// against the BID kernel: both signs, so that half are differences, equal
// and near-equal magnitudes (cancellations, borrows out of the low word),
// zeros, the largest coefficient, and both ends of the exponent range.
func TestUBDAddWideAgrees(t *testing.T) {
	r := rand.New(rand.NewPCG(13, 17))
	coeff := func() Uint128 {
		switch r.IntN(8) {
		case 0:
			return Uint128{}
		case 1:
			return Uint128{Hi: MaxCoeff128Hi, Lo: MaxCoeff128Lo - uint64(r.IntN(3))}
		case 2:
			return Uint128{Hi: uint64(r.IntN(3)), Lo: ^uint64(0) - uint64(r.IntN(3))}
		case 3:
			return Uint128{Lo: r.Uint64()}
		default:
			return ubdBenchDigits(r, 20, 34)
		}
	}
	gtMax := func(c Uint128) bool {
		return c.Hi > MaxCoeff128Hi || c.Hi == MaxCoeff128Hi && c.Lo > MaxCoeff128Lo
	}
	exps := []int{MinExp128, MinExp128 + 1, -6, 0, 3, MaxExp128 - 1, MaxExp128}
	for i := 0; i < 2000000; i++ {
		e := exps[r.IntN(len(exps))]
		xc := coeff()
		yc := xc
		switch r.IntN(4) {
		case 0: // equal magnitudes
		case 1: // magnitudes one apart, either way
			var c uint64
			if r.IntN(2) == 0 {
				yc.Lo, c = mbits.Add64(xc.Lo, 1, 0)
				yc.Hi += c
				if gtMax(yc) {
					yc = xc
				}
			} else if xc != (Uint128{}) {
				yc.Lo, c = mbits.Sub64(xc.Lo, 1, 0)
				yc.Hi -= c
			}
		case 2: // the same high word
			yc.Lo = r.Uint64()
			if gtMax(yc) {
				yc = xc
			}
		default:
			yc = coeff()
		}
		bx, ux := ubdBenchVal(r.IntN(2) == 0, xc, e)
		by, uy := ubdBenchVal(r.IntN(2) == 0, yc, e)
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

// TestUBDInt64WordAgrees checks the int64 and uint64 conversion entries on
// word operands, with fractions of up to 19 digits and integers of either
// sign, against Int64From128 and Uint64From128 with TowardZero.
func TestUBDInt64WordAgrees(t *testing.T) {
	r := rand.New(rand.NewPCG(19, 23))
	for i := 0; i < 2000000; i++ {
		c := r.Uint64() >> uint(r.IntN(64))
		if r.IntN(8) == 0 {
			c = ^uint64(0) - uint64(r.IntN(3))
		}
		e := r.IntN(26) - 22
		bx, ux := ubdBenchVal(r.IntN(2) == 0, Uint128{Lo: c}, e)
		want, _ := BIDKernelInt64(bx, TowardZero)
		if got := UBDKernelInt64(ux); got != want {
			t.Fatalf("int64(%#x): got %d, want %d", bx, got, want)
		}
		uwant, _ := BIDKernelUint64(bx, TowardZero)
		if got := UBDKernelUint64(ux); got != uwant {
			t.Fatalf("uint64(%#x): got %d, want %d", bx, got, uwant)
		}
	}
}

// TestUBDQuantizeWideAgrees checks the ties-to-even quantize entry on
// operands wider than a word, which ubdQuantizeWideTTE decides, against
// the BID kernel: coefficients of 20 to 34 digits, quantized to exponents
// from 40 below to 40 above their own, so that the result rounds, scales,
// is the same, is zero, or is invalid.
func TestUBDQuantizeWideAgrees(t *testing.T) {
	r := rand.New(rand.NewPCG(37, 41))
	for i := 0; i < 1000000; i++ {
		xe := r.IntN(200) - 100
		ye := xe + r.IntN(81) - 40
		bx, ux := ubdBenchVal(r.IntN(2) == 0, ubdBenchDigits(r, 20, 34), xe)
		by, uy := ubdBenchVal(r.IntN(2) == 0, ubdBenchDigits(r, 1, 5), ye)
		got := UBDKernelQnt(ux, uy)
		want := BIDKernelQnt(bx, by, ToNearestEven)
		if g := BIDFromUBDAny128(Uint128{Hi: got.Hi, Lo: got.Lo}); g != want {
			t.Fatalf("quantize(%#x, %#x): got %#x, want %#x", bx, by, g, want)
		}
	}
}

// TestUBDMulAgrees checks the multiply entries, ties to even and the Rnd
// entry in every mode, against the BID kernel on coefficients of 1 to 34
// digits, so that products are exact, round, or pass the format.
func TestUBDMulAgrees(t *testing.T) {
	r := rand.New(rand.NewPCG(43, 47))
	modes := []Mode{ToNearestEven, ToNearestAway, TowardZero, AwayFromZero, TowardNegative, TowardPositive}
	for i := 0; i < 1000000; i++ {
		bx, ux := ubdBenchVal(r.IntN(2) == 0, ubdBenchDigits(r, 1, 34), r.IntN(400)-200)
		by, uy := ubdBenchVal(r.IntN(2) == 0, ubdBenchDigits(r, 1, 34), r.IntN(400)-200)
		got := UBDKernelMul(ux, uy)
		if g, want := BIDFromUBDAny128(Uint128{Hi: got.Hi, Lo: got.Lo}), BIDKernelMul(bx, by, ToNearestEven); g != want {
			t.Fatalf("mul(%#x, %#x): got %#x, want %#x", bx, by, g, want)
		}
		m := modes[i%len(modes)]
		g := UBDMulRnd128(Uint128{Hi: ux.Hi, Lo: ux.Lo}, Uint128{Hi: uy.Hi, Lo: uy.Lo}, m)
		if g, want := BIDFromUBDAny128(g), Mul128(bx, by, m); g != want {
			t.Fatalf("mul(%#x, %#x, %v): got %#x, want %#x", bx, by, m, g, want)
		}
	}
}

// TestUBDDivAgrees checks the divide entries, ties to even and the Rnd
// entry in every mode, against the BID kernel on coefficients of 1 to 34
// digits, zeros among them.
func TestUBDDivAgrees(t *testing.T) {
	r := rand.New(rand.NewPCG(53, 59))
	modes := []Mode{ToNearestEven, ToNearestAway, TowardZero, AwayFromZero, TowardNegative, TowardPositive}
	coeff := func() Uint128 {
		if r.IntN(16) == 0 {
			return Uint128{}
		}
		return ubdBenchDigits(r, 1, 34)
	}
	for i := 0; i < 500000; i++ {
		bx, ux := ubdBenchVal(r.IntN(2) == 0, coeff(), r.IntN(400)-200)
		by, uy := ubdBenchVal(r.IntN(2) == 0, coeff(), r.IntN(400)-200)
		got := UBDKernelDiv(ux, uy)
		if g, want := BIDFromUBDAny128(Uint128{Hi: got.Hi, Lo: got.Lo}), BIDKernelDiv(bx, by, ToNearestEven); g != want {
			t.Fatalf("div(%#x, %#x): got %#x, want %#x", bx, by, g, want)
		}
		m := modes[i%len(modes)]
		g := UBDDivRnd128(Uint128{Hi: ux.Hi, Lo: ux.Lo}, Uint128{Hi: uy.Hi, Lo: uy.Lo}, m)
		if g, want := BIDFromUBDAny128(g), Div128(bx, by, m); g != want {
			t.Fatalf("div(%#x, %#x, %v): got %#x, want %#x", bx, by, m, g, want)
		}
	}
}

// TestUBDDivPow10Agrees checks the ties-to-even divide entry with divisors
// that are powers of ten, which it decides from the divisor's trailing zero
// bits, and with divisors that look like them (10^k ± 1, 2^k, 5^k, and
// 10^k times 2^32, whose trailing zeros index past the table), against the
// BID kernel: dividends of every width with trailing zeros, so that the
// quotient strips, at exponents at both ends of the range.
func TestUBDDivPow10Agrees(t *testing.T) {
	r := rand.New(rand.NewPCG(61, 67))
	var divisors []uint64
	p := uint64(1)
	for k := 0; k <= 19; k++ {
		divisors = append(divisors, p, p+1, p<<32)
		if p > 1 {
			divisors = append(divisors, p-1)
		}
		p *= 10 // wraps after 10^19, when the loop ends
	}
	for k := 0; k < 64; k++ {
		divisors = append(divisors, 1<<k)
	}
	for f := uint64(5); f < 1<<62; f *= 5 {
		divisors = append(divisors, f)
	}
	exps := []int{MinExp128, MinExp128 + 20, -6, -2, 0, 4, MaxExp128 - 20, MaxExp128}
	for i := 0; i < 1000000; i++ {
		xc := ubdBenchDigits(r, 1, 34)
		if r.IntN(2) == 0 {
			// Append zeros, up to 34 digits in all.
			for z := r.IntN(20); z > 0; z-- {
				hi, lo := mbits.Mul64(xc.Lo, 10)
				n := Uint128{Hi: xc.Hi*10 + hi, Lo: lo}
				if n.Hi > MaxCoeff128Hi || n.Hi == MaxCoeff128Hi && n.Lo > MaxCoeff128Lo {
					break
				}
				xc = n
			}
		}
		bx, ux := ubdBenchVal(r.IntN(2) == 0, xc, exps[r.IntN(len(exps))])
		by, uy := ubdBenchVal(r.IntN(2) == 0, Uint128{Lo: divisors[r.IntN(len(divisors))]}, exps[r.IntN(len(exps))])
		got := UBDKernelDiv(ux, uy)
		if g, want := BIDFromUBDAny128(Uint128{Hi: got.Hi, Lo: got.Lo}), BIDKernelDiv(bx, by, ToNearestEven); g != want {
			t.Fatalf("div(%#x, %#x): got %#x, want %#x", bx, by, g, want)
		}
	}
}

// TestUBDRoundToIntegralWideAgrees checks the ties-to-even round-to-
// integral entry on operands wider than a word against the BID kernel:
// coefficients of 20 to 34 digits at exponents from -60 to 10, so that the
// result rounds, is the operand, or is zero.
func TestUBDRoundToIntegralWideAgrees(t *testing.T) {
	r := rand.New(rand.NewPCG(61, 67))
	for i := 0; i < 1000000; i++ {
		bx, ux := ubdBenchVal(r.IntN(2) == 0, ubdBenchDigits(r, 20, 34), r.IntN(71)-60)
		got := UBDKernelRTI(ux)
		if g, want := BIDFromUBDAny128(Uint128{Hi: got.Hi, Lo: got.Lo}), BIDKernelRTI(bx, ToNearestEven); g != want {
			t.Fatalf("roundToIntegral(%#x): got %#x, want %#x", bx, g, want)
		}
	}
}
