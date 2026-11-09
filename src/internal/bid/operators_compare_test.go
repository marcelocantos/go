// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package bid_test

import (
	"internal/bid"
	"math/big"
	"math/rand/v2"
	"testing"
)

// TestOperatorCompareReference judges all six compiled comparisons with the
// independent big.Int model, including arbitrary encodings and dense word
// coefficients. The kernel is not the oracle for compiler fast paths.
func TestOperatorCompareReference(t *testing.T) {
	for _, f := range both {
		t.Run(f.name, func(t *testing.T) {
			check := func(x, y bits) {
				t.Helper()
				c, ordered := f.compare(x, y)
				want := [6]bool{ordered && c == 0, !ordered || c != 0, ordered && c < 0,
					ordered && c <= 0, ordered && c > 0, ordered && c >= 0}
				var got [6]bool
				if f == dec64 {
					got = opCmp64(d64(x), d64(y))
				} else {
					got = opCmp128(d128(x), d128(y))
				}
				if got != want {
					t.Fatalf("compiled compare(%x, %x): got %v, want %v", x, y, got, want)
				}
			}
			for _, c := range strawmanCases {
				if len(c.x.c) > f.prec || len(c.y.c) > f.prec {
					continue // Some shared fixtures require decimal128 precision.
				}
				check(c.x.bits(f), c.y.bits(f))
			}
			for prefix := uint64(0); prefix < 1<<16; prefix++ {
				for _, lo := range []uint64{0, 1, 1<<48 - 1} {
					x, y := bits{Hi: prefix << 48, Lo: lo}, bits{Hi: prefix << 48, Lo: 1}
					if f == dec64 {
						x, y = bits{Lo: prefix<<48 | lo}, bits{Lo: prefix<<48 | 1}
					}
					check(x, y)
					check(x, f.withSign(y, !f.signbit(y)))
				}
			}
			word := func(c uint64, e int, neg bool) bits {
				return f.encode(value{kind: finite, coeff: new(big.Int).SetUint64(c), exp: e, neg: neg})
			}
			coeffs := []uint64{0, 1, 9, 10, 1<<53 - 1, 1 << 53, bid.MaxCoeff64}
			if f == dec128 {
				coeffs = append(coeffs, 1<<63, 10000000000000000000, ^uint64(0))
			}
			for _, gap := range []int{0, 1, 18, 19, 20, 21} {
				for _, e := range []int{f.minExp, -gap, f.maxExp - gap} {
					for _, a := range coeffs {
						for _, b := range coeffs {
							for signs := range 4 {
								x, y := word(a, e, signs&1 != 0), word(b, e+gap, signs&2 != 0)
								check(x, y)
								check(y, x)
							}
						}
					}
				}
			}
			r := rand.New(rand.NewPCG(*seedFlag, streamSeed("compiled-compare-"+f.name)))
			for i := range numCases() {
				x, y := bits{Hi: r.Uint64(), Lo: r.Uint64()}, bits{Hi: r.Uint64(), Lo: r.Uint64()}
				if f == dec64 {
					x.Hi, y.Hi = 0, 0
				}
				check(x, y)
				a, b := r.Uint64(), r.Uint64()
				if f == dec64 {
					a, b = a%(bid.MaxCoeff64+1), b%(bid.MaxCoeff64+1)
				}
				gap := r.IntN(f.maxExp - f.minExp + 1)
				if i&1 == 0 {
					gap = r.IntN(23)
				}
				e := f.minExp + r.IntN(f.maxExp-f.minExp+1-gap)
				x, y = word(a, e, r.IntN(2) != 0), word(b, e+gap, r.IntN(2) != 0)
				check(x, y)
				check(y, x)
			}
		})
	}
}

func TestOperatorCompareEvaluation(t *testing.T) {
	for _, tc := range []struct {
		x, y string
	}{
		{"1.00", "1.001"},
		{"9999999999999999999999999999999999", "1"},
		{"NaN", "1"},
		{"-0.00", "0"},
	} {
		x, y := addOperand(t, dec128, tc.x), addOperand(t, dec128, tc.y)
		var order []int
		take := func(i int) decimal128 {
			order = append(order, i)
			return d128([]bits{x, y}[i])
		}
		got := take(0) < take(1)
		c, ordered := dec128.compare(x, y)
		if got != (ordered && c < 0) || len(order) != 2 || order[0] != 0 || order[1] != 1 {
			t.Fatalf("compare %s, %s: result %v, evaluation %v", tc.x, tc.y, got, order)
		}
	}
}

// The source operators stay inside the rotating loop. Word-sized coefficients
// cover equal/nearby quanta, signs and zero; far rows exercise exponent-only
// ordering, while wide/special rows keep encoding and fallback costs visible.
func BenchmarkOperatorCompare(b *testing.B) {
	for _, f := range both {
		wide := "9999999999999999"
		if f == dec128 {
			wide = "9999999999999999999999999999999999"
		}
		for _, c := range []struct {
			name string
			x, y [4]opnd
		}{
			{"sameQuantum", [4]opnd{fin("12345", -2), nfin("42", -2), fin("0", 0), nfin("0", 0)},
				[4]opnd{fin("42", -2), nfin("12345", -2), fin("1", 0), nfin("0", 0)}},
			{"nearby", [4]opnd{fin("12345", -2), nfin("42", -2), fin("1", 0), fin("9", 19)},
				[4]opnd{fin("42", -3), nfin("12345", -4), fin("999999", -5), fin("1", 0)}},
			{"signs", [4]opnd{nfin("12345", -2), fin("42", -2), nfin("0", 0), fin("0", -2)},
				[4]opnd{fin("42", -3), nfin("12345", -4), fin("0", -2), nfin("0", 0)}},
			{"far", [4]opnd{fin("1", 20), fin("1", f.maxExp), nfin("1", 20), fin("0", f.maxExp)},
				[4]opnd{fin("9", 0), fin("1", f.minExp), nfin("9", 0), fin("1", 0)}},
			{"wide", [4]opnd{fin(wide, -2), nfin(wide, -2), fin(wide, 0), fin(wide, -3)},
				[4]opnd{fin(wide, -3), fin(wide, -2), fin("1", 0), fin(wide, -3)}},
			{"special", [4]opnd{cNaN, cInf, cSNaN, cNegInf},
				[4]opnd{fin("1", 0), cInf, cNaN, fin("1", 0)}},
		} {
			var xs64, ys64 [4]decimal64
			var xs128, ys128 [4]decimal128
			for i := range c.x {
				if f == dec64 {
					xs64[i], ys64[i] = d64(c.x[i].bits(f)), d64(c.y[i].bits(f))
				} else {
					xs128[i], ys128[i] = d128(c.x[i].bits(f)), d128(c.y[i].bits(f))
				}
			}
			b.Run(f.name+"/"+c.name, func(b *testing.B) {
				if f == dec64 {
					b.Run("eq", func(b *testing.B) { loop2(b, xs64[:], ys64[:], func(x, y decimal64) bool { return x == y }) })
					b.Run("lt", func(b *testing.B) { loop2(b, xs64[:], ys64[:], func(x, y decimal64) bool { return x < y }) })
				} else {
					b.Run("eq", func(b *testing.B) { loop2(b, xs128[:], ys128[:], func(x, y decimal128) bool { return x == y }) })
					b.Run("lt", func(b *testing.B) { loop2(b, xs128[:], ys128[:], func(x, y decimal128) bool { return x < y }) })
				}
			})
		}
	}
}

// Uniform raw encodings complement the financial operands above. In
// decimal128 most coefficients and NaN payloads fail the word guard, so this
// keeps the broad fallback cost visible rather than measuring only fast hits.
func BenchmarkOperatorCompareRaw(b *testing.B) {
	r := rand.New(rand.NewPCG(*seedFlag, streamSeed("operator-compare-raw")))
	xs64, ys64 := make([]decimal64, poolSize), make([]decimal64, poolSize)
	xs128, ys128 := make([]decimal128, poolSize), make([]decimal128, poolSize)
	for i := range xs64 {
		xs64[i], ys64[i] = d64(bits{Lo: r.Uint64()}), d64(bits{Lo: r.Uint64()})
		xs128[i], ys128[i] = d128(bits{Hi: r.Uint64(), Lo: r.Uint64()}), d128(bits{Hi: r.Uint64(), Lo: r.Uint64()})
	}
	b.Run("decimal64/eq", func(b *testing.B) { loop2(b, xs64, ys64, func(x, y decimal64) bool { return x == y }) })
	b.Run("decimal64/lt", func(b *testing.B) { loop2(b, xs64, ys64, func(x, y decimal64) bool { return x < y }) })
	b.Run("decimal128/eq", func(b *testing.B) { loop2(b, xs128, ys128, func(x, y decimal128) bool { return x == y }) })
	b.Run("decimal128/lt", func(b *testing.B) { loop2(b, xs128, ys128, func(x, y decimal128) bool { return x < y }) })
}
