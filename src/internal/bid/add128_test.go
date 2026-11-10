// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"math/big"
	"math/rand/v2"
	"testing"
)

// TestAddWide drives addSlow128's own path, the sum of finite operands of
// which one has a coefficient of two words, against the reference in every
// mode (the directed modes take the general path, which the same operands
// then test), in both orders and with every pair of signs:
//
//   - every digit count of the operand with the larger exponent against
//     gaps that leave its scale short of the other exponent by none, by one
//     to six, by most of the other's digits, by all of them and by more:
//     the exact sum, the difference that loses its leading digit and is
//     taken whole, the shifted sum, and the addend that rounds to nothing;
//   - coefficients of nines and of a one and zeros, whose sums carry into
//     a 35th digit and whose differences cancel;
//   - ties: an addend whose dropped digits are a five and zeros, and the
//     digits either side of that;
//   - exponents at both ends of the range, where the path must refuse
//     rather than encode.
func TestAddWide(t *testing.T) {
	var k kernel
	f := dec128
	p := f.prec
	rng := rand.New(rand.NewPCG(*seedFlag, streamSeed("addwide")))
	pow := func(n int) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }
	n := 0
	check := func(xc, yc *big.Int, xe, ye int) {
		if xc.Sign() < 0 || yc.Sign() < 0 || xc.Cmp(f.maxCoeff) > 0 || yc.Cmp(f.maxCoeff) > 0 ||
			xe < f.minExp || xe > f.maxExp || ye < f.minExp || ye > f.maxExp {
			return
		}
		for _, signs := range [][2]bool{{false, false}, {false, true}, {true, false}, {true, true}} {
			x := f.encode(value{kind: finite, neg: signs[0], coeff: xc, exp: xe})
			y := f.encode(value{kind: finite, neg: signs[1], coeff: yc, exp: ye})
			for _, m := range allModes {
				for _, pair := range [][2]bits{{x, y}, {y, x}} {
					n++
					if got, want := k.add(f, pair[0], pair[1], m), f.add(pair[0], pair[1], m); got != want {
						t.Fatalf("add(%s, %s, %s) = %s, want %s", f.format(pair[0]), f.format(pair[1]), modeName(m), f.format(got), f.format(want))
					}
				}
			}
		}
	}
	// The coefficients of one length: random, the extremes, and the ones
	// next to the extremes.
	coeffs := func(d int) []*big.Int {
		lo, hi := pow(d-1), new(big.Int).Sub(pow(d), big.NewInt(1))
		cs := []*big.Int{randDigits(rng, d), randDigits(rng, d), lo, hi}
		if d > 1 {
			cs = append(cs, new(big.Int).Add(lo, big.NewInt(1)), new(big.Int).Sub(hi, big.NewInt(1)))
		}
		return cs
	}
	rounds := 1
	if *longTest {
		rounds = 20
	}
	for range rounds {
		for da := 1; da <= p; da++ {
			m := p - da
			for _, excess := range []int{-m, -1, 0, 1, 2, 3, 4, 5, 6, 17, 20, 33, 34, 35, 38, 39, 60} {
				gap := m + excess
				if gap < 0 {
					continue
				}
				dbs := []int{1, 19, 20, 34}
				if *longTest {
					dbs = []int{1, 2, 10, 19, 20, 21, 33, 34}
				}
				for _, db := range dbs {
					if da < 20 && db < 20 {
						continue // one word each: not this path
					}
					for _, ae := range []int{0, f.maxExp, f.minExp + gap} {
						for _, ac := range coeffs(da) {
							for _, bc := range coeffs(db) {
								check(ac, bc, ae, ae-gap)
							}
						}
					}
				}
			}
		}
	}
	// Ties and their neighbours: b ends in a five and d-1 zeros, or one
	// either side of that, under an a of the full precision, even and odd.
	for d := 1; d <= p; d++ {
		half := new(big.Int).Mul(big.NewInt(5), pow(d-1))
		for range 8 * rounds {
			a := randDigits(rng, p)
			top := new(big.Int)
			if d < p {
				top.Mul(randDigits(rng, 1+rng.IntN(p-d)), pow(d))
			}
			for _, delta := range []int64{-1, 0, 1} {
				b := new(big.Int).Add(top, half)
				b.Add(b, big.NewInt(delta))
				check(a, b, 0, -d)
				check(new(big.Int).SetBit(new(big.Int).Set(a), 0, 1), b, 0, -d)
				check(new(big.Int).SetBit(new(big.Int).Set(a), 0, 0), b, 0, -d)
			}
		}
	}
	// Sums that carry and differences that cancel, at the gaps where the
	// exact difference still fits two words and just beyond them.
	for gap := 0; gap <= 8; gap++ {
		for range 40 * rounds {
			a := randDigits(rng, p-rng.IntN(2))
			b := new(big.Int).Mul(a, pow(gap))
			b.Add(b, big.NewInt(int64(rng.IntN(2001)-1000)))
			if b.Sign() > 0 {
				check(a, b, 0, -gap) // nearly equal: the difference is a few digits
			}
			c := new(big.Int).Sub(pow(p), a)
			c.Add(c, big.NewInt(int64(rng.IntN(21)-10)))
			check(a, new(big.Int).Mul(c, pow(gap)), 0, -gap) // the sum is about 10^p
			check(a, new(big.Int).Quo(new(big.Int).Mul(c, pow(gap)), big.NewInt(3)), 0, -gap)
		}
	}
	// Coefficients that are not canonical, of the first form and above the
	// largest coefficient: zeros, which this path hands on, as either
	// operand beside one of two words.
	for _, c := range []*big.Int{pow(p), new(big.Int).Add(pow(p), big.NewInt(7)), new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 113), big.NewInt(1))} {
		raw := f.encode(value{kind: finite, coeff: new(big.Int), exp: -3})
		raw.Hi |= new(big.Int).Rsh(c, 64).Uint64()
		raw.Lo = new(big.Int).And(c, new(big.Int).SetUint64(^uint64(0))).Uint64()
		for _, gap := range []int{0, 1, 5, 20, 40} {
			for _, oc := range []*big.Int{randDigits(rng, p), randDigits(rng, 20), randDigits(rng, 5)} {
				o := f.encode(value{kind: finite, neg: rng.IntN(2) == 0, coeff: oc, exp: -3 + gap - 20})
				for _, m := range allModes {
					for _, pair := range [][2]bits{{raw, o}, {o, raw}} {
						n++
						if got, want := k.add(f, pair[0], pair[1], m), f.add(pair[0], pair[1], m); got != want {
							t.Fatalf("add(%s, %s, %s) = %s, want %s", f.format(pair[0]), f.format(pair[1]), modeName(m), f.format(got), f.format(want))
						}
					}
				}
			}
		}
	}
	if n == 0 {
		t.Fatal("no sum checked")
	}
	t.Logf("%d sums", n)
}
