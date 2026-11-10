// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"math/big"
	"math/rand/v2"
	"testing"
)

// TestMulWide drives mulSlow128's own path, the product of finite operands
// of which one has a coefficient of two words, against the reference in
// every mode (the directed modes take the general path, which the same
// operands then test) and with every pair of signs:
//
//   - every pair of digit counts, so that the product has from 20 to 68
//     digits in two, three and four words and loses from none to 34 of
//     them, with random coefficients and with the extremes of each length,
//     whose products sit on both sides of a power of ten;
//   - ties: products that end in a five and zeros below the 34th digit,
//     with the kept digits even and odd, and the products either side;
//   - exponents at both ends of the range, where the path must refuse
//     rather than encode;
//   - zeros and coefficients that are not canonical, which it hands on.
func TestMulWide(t *testing.T) {
	var k kernel
	f := dec128
	p := f.prec
	rng := rand.New(rand.NewPCG(*seedFlag, streamSeed("mulwide")))
	pow := func(n int) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }
	n := 0
	bitsCheck := func(x, y bits) {
		for _, m := range allModes {
			for _, pair := range [][2]bits{{x, y}, {y, x}} {
				n++
				if got, want := k.mul(f, pair[0], pair[1], m), f.mul(pair[0], pair[1], m); got != want {
					t.Fatalf("mul(%s, %s, %s) = %s, want %s", f.format(pair[0]), f.format(pair[1]), modeName(m), f.format(got), f.format(want))
				}
			}
		}
	}
	check := func(xc, yc *big.Int, xe, ye int) {
		if xc.Sign() < 0 || yc.Sign() < 0 || xc.Cmp(f.maxCoeff) > 0 || yc.Cmp(f.maxCoeff) > 0 {
			return
		}
		for _, signs := range [][2]bool{{false, false}, {false, true}, {true, true}} {
			bitsCheck(f.encode(value{kind: finite, neg: signs[0], coeff: xc, exp: xe}), f.encode(value{kind: finite, neg: signs[1], coeff: yc, exp: ye}))
		}
	}
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
		for dx := 20; dx <= p; dx++ {
			for dy := 1; dy <= p; dy++ {
				for _, xc := range coeffs(dx) {
					for _, yc := range coeffs(dy) {
						check(xc, yc, -dx, -dy)
					}
				}
				// The ends of the range: the product's exponent at
				// and around the smallest and the largest.
				xc, yc := randDigits(rng, dx), randDigits(rng, dy)
				drop := max(dx+dy-p, 0)
				for _, e := range []int{f.minExp - drop - 1, f.minExp - drop, f.minExp - drop + 1, f.minExp, f.maxExp - drop - 1, f.maxExp - drop, f.maxExp - drop + 1} {
					xe := e / 2
					if xe >= f.minExp && xe <= f.maxExp && e-xe >= f.minExp && e-xe <= f.maxExp {
						check(xc, yc, xe, e-xe)
					}
				}
			}
		}
	}
	// Ties. An odd x of the full precision times 5 × 10^(a-1) is 5x and
	// a-1 zeros: when 5x has one digit more than the precision, a five and
	// zeros lie below the 34th digit. The digits kept are even and odd as
	// x is 1 or 3 modulo 4. The products of x ± 1 and of the multipliers
	// either side of that one lie a little off the tie.
	ties := 0
	for a := 1; a <= p; a++ {
		yc := new(big.Int).Mul(big.NewInt(5), pow(a-1))
		for range 20 * rounds {
			xc := randDigits(rng, p)
			xc.SetBit(xc, 0, 1)
			if digits(new(big.Int).Mul(xc, big.NewInt(5))) == p+1 {
				ties++
			}
			for _, dx := range []int64{0, -1, 1} {
				for _, dy := range []int64{0, -1, 1} {
					check(new(big.Int).Add(xc, big.NewInt(dx)), new(big.Int).Add(yc, big.NewInt(dy)), 0, 0)
				}
			}
		}
	}
	if ties < 50 {
		t.Fatalf("only %d ties", ties)
	}
	// The carries of the product's upper words are rare (the high words
	// of the coefficients are short), so many more products of two full
	// coefficients are tried, in one mode: a missed carry misplaces the
	// product by 2^192.
	for range 300000 * rounds {
		x := f.encode(value{kind: finite, coeff: randDigits(rng, p), exp: -p})
		y := f.encode(value{kind: finite, coeff: randDigits(rng, p), exp: -p})
		n++
		if got, want := k.mul(f, x, y, allModes[0]), f.mul(x, y, allModes[0]); got != want {
			t.Fatalf("mul(%s, %s) = %s, want %s", f.format(x), f.format(y), f.format(got), f.format(want))
		}
	}
	// Zeros and coefficients that are not canonical beside two words.
	wide := f.encode(value{kind: finite, coeff: randDigits(rng, p), exp: -5})
	for _, c := range []*big.Int{new(big.Int), pow(p), new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 113), big.NewInt(1))} {
		raw := f.encode(value{kind: finite, coeff: new(big.Int), exp: -3})
		raw.Hi |= new(big.Int).Rsh(c, 64).Uint64()
		raw.Lo = new(big.Int).And(c, new(big.Int).SetUint64(^uint64(0))).Uint64()
		bitsCheck(raw, wide)
		bitsCheck(raw, k.neg(f, wide))
	}
	t.Logf("%d products, %d ties", n, ties)
}
