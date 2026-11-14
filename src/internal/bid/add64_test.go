// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math/big"
	"math/rand/v2"
	"testing"
)

// TestAddWord64 drives addSlow64's own paths, and addWide64 behind them,
// against the reference in every mode (the directed modes take addLong64,
// which the same operands then test), in both orders, with every pair of
// signs, through Add64 and Sub64 and through addSlow64 itself, and checks
// that addSlow64 and addLong64 agree:
//
//   - every digit count of the operand with the larger exponent, either
//     side of 10^15, where a takes its digit count or skips it, against
//     gaps that leave its scale short of the other exponent by more, by
//     one, by none (the exact sum) and that exceed it by one to six, by
//     most of the other's digits, by all of them and by more (the rounded
//     sum, b losing d digits to a quotient of nothing), up to maxWordGap
//     and one beyond it, and either side of the swamping gap, m +
//     farWordGap;
//   - ties: a b whose dropped digits are a five and zeros, and one either
//     side of that, under an a of 16 digits, even and odd, for every d;
//   - sums whose rounded value reaches 10^16, or stops one short, and
//     differences whose rounded value is 10^15 + 1, 10^15 or one below,
//     the bounds of the word path;
//   - coefficients and results either side of 2^53, where the form of an
//     operand and of the result changes, and MaxCoeff64;
//   - zeros, as either operand, of both signs, at every gap;
//   - exponents at both ends of the range, where a carry overflows;
//   - non-canonical second-form coefficients and the special values,
//     which addSlow64 hands on.
func TestAddWord64(t *testing.T) {
	var k kernel
	f := dec64
	p := f.prec
	rng := rand.New(rand.NewPCG(*seedFlag, streamSeed("addword64")))
	pow := func(n int) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }
	n := 0
	try := func(x, y bits) {
		for _, m := range allModes {
			for _, pair := range [][2]bits{{x, y}, {y, x}} {
				a, b := pair[0], pair[1]
				n++
				want := f.add(a, b, m)
				if got := k.add(f, a, b, m); got != want {
					t.Fatalf("add(%s, %s, %s) = %s, want %s", f.format(a), f.format(b), modeName(m), f.format(got), f.format(want))
				}
				if got := lo(bid.AddSlow64(a.Lo, b.Lo, m)); got != want {
					t.Fatalf("addSlow64(%s, %s, %s) = %s, want %s", f.format(a), f.format(b), modeName(m), f.format(got), f.format(want))
				}
				if got, long := bid.AddSlow64(a.Lo, b.Lo, m), bid.AddLong64(a.Lo, b.Lo, m); got != long {
					t.Fatalf("addSlow64(%s, %s, %s) = %s, addLong64 %s", f.format(a), f.format(b), modeName(m), f.format(lo(got)), f.format(lo(long)))
				}
				if got, want := k.sub(f, a, b, m), f.sub(a, b, m); got != want {
					t.Fatalf("sub(%s, %s, %s) = %s, want %s", f.format(a), f.format(b), modeName(m), f.format(got), f.format(want))
				}
			}
		}
	}
	check := func(xc, yc *big.Int, xe, ye int) {
		if xc.Sign() < 0 || yc.Sign() < 0 || xc.Cmp(f.maxCoeff) > 0 || yc.Cmp(f.maxCoeff) > 0 ||
			xe < f.minExp || xe > f.maxExp || ye < f.minExp || ye > f.maxExp {
			return
		}
		for _, signs := range [][2]bool{{false, false}, {false, true}, {true, false}, {true, true}} {
			try(f.encode(value{kind: finite, neg: signs[0], coeff: xc, exp: xe}),
				f.encode(value{kind: finite, neg: signs[1], coeff: yc, exp: ye}))
		}
	}
	// The coefficients of one length: random, the extremes, and the ones
	// next to the extremes.
	coeffs := func(d int) []*big.Int {
		lo, hi := pow(d-1), new(big.Int).Sub(pow(d), big.NewInt(1))
		cs := []*big.Int{randDigits(rng, d), lo, hi}
		if d > 1 {
			cs = append(cs, new(big.Int).Add(lo, big.NewInt(1)), new(big.Int).Sub(hi, big.NewInt(1)))
		}
		return cs
	}
	rounds := 1
	if *longTest {
		rounds = 10
	}
	for range rounds {
		for da := 1; da <= p; da++ {
			m := p - da
			for _, excess := range []int{-m, -m + 1, -2, -1, 0, 1, 2, 3, 4, 5, 6, 15, 16, 17, 18, 19, 20, 21, 22, 23, 30} {
				gap := m + excess
				if gap < 0 {
					continue
				}
				dbs := []int{1, 8, 16}
				if *longTest {
					dbs = []int{1, 2, 5, 8, 15, 16}
				}
				for _, db := range dbs {
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
	// The gaps either side of maxWordGap and of the swamping gap, for
	// every digit count of a.
	for da := 1; da <= p; da++ {
		m := p - da
		for _, gap := range []int{18, 19, 20, 21, m + 19, m + 20, m + 21, m + 22} {
			for _, bc := range coeffs(p) {
				check(randDigits(rng, da), bc, 0, -gap)
			}
		}
	}
	// Ties and their neighbours: b ends in a five and d-1 zeros, or one
	// either side of that, under an a of 16 digits, even and odd, as a sum
	// and as a difference (the signs of check).
	for d := 1; d <= 19; d++ {
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
				check(new(big.Int).SetBit(new(big.Int).Set(a), 0, 1), b, 0, -d)
				check(new(big.Int).SetBit(new(big.Int).Set(a), 0, 0), b, 0, -d)
			}
		}
	}
	// The bounds of the rounded sum: p + q at 10^16 - 1 and 10^16, and
	// p - q at 10^15 + 1, 10^15 and 10^15 - 1, with a tie or one either
	// side of it in what b drops.
	for d := 1; d <= 4; d++ {
		pd := pow(d)
		half := new(big.Int).Rsh(pd, 1)
		for _, delta := range []int64{-1, 0, 1} {
			for _, q := range []int64{1, 2, 3, 10, 11} {
				// The sum: a = 10^16 - q + j, b = q·10^d ± a half.
				for j := int64(-1); j <= 1; j++ {
					a := new(big.Int).Sub(pow(p), big.NewInt(q-j))
					b := new(big.Int).Mul(big.NewInt(q), pd)
					b.Add(b, new(big.Int).Add(half, big.NewInt(delta)))
					b.Sub(b, pd) // rounds half up to q
					check(a, b, 0, -d)
				}
				// The difference: a = 10^15 + q + j.
				for j := int64(-1); j <= 1; j++ {
					a := new(big.Int).Add(pow(p-1), big.NewInt(q+j))
					b := new(big.Int).Mul(big.NewInt(q), pd)
					b.Add(b, new(big.Int).Add(half, big.NewInt(delta)))
					b.Sub(b, pd)
					check(a, b, 0, -d)
				}
			}
		}
	}
	// Coefficients either side of 2^53, as operands and as results.
	two53 := new(big.Int).Lsh(big.NewInt(1), 53)
	for _, c := range []int64{-2, -1, 0, 1, 2} {
		v := new(big.Int).Add(two53, big.NewInt(c))
		for _, gap := range []int{0, 1, 3, 17, 19} {
			for _, o := range []*big.Int{big.NewInt(1), big.NewInt(5), big.NewInt(9), randDigits(rng, 16), f.maxCoeff} {
				check(v, o, 0, -gap)
				check(o, v, 0, -gap)
			}
		}
		// v + 0.4 and v - 0.4 round to v; v·10 + 5 at one place lower is a tie.
		check(new(big.Int).Sub(v, big.NewInt(1)), big.NewInt(6), 0, -1)
		check(new(big.Int).Add(v, big.NewInt(1)), big.NewInt(6), 0, -1)
		check(v, big.NewInt(4), 0, -1)
		check(v, big.NewInt(5), 0, -1)
	}
	// Zeros, as either operand, at every gap.
	for gap := 0; gap <= 22; gap++ {
		for _, o := range []*big.Int{new(big.Int), big.NewInt(1), randDigits(rng, 9), randDigits(rng, 16), f.maxCoeff} {
			check(new(big.Int), o, 0, -gap)
			check(o, new(big.Int), 0, -gap)
		}
	}
	// A carry at the largest exponent overflows; the smallest exponents
	// are exact.
	for gap := 1; gap <= 19; gap++ {
		check(f.maxCoeff, f.maxCoeff, f.maxExp, f.maxExp-gap)
		check(f.maxCoeff, new(big.Int).Mul(big.NewInt(5), pow(gap-1)), f.maxExp, f.maxExp-gap)
		check(f.maxCoeff, f.maxCoeff, f.minExp+gap, f.minExp)
	}
	// Non-canonical second-form coefficients, which are zeros, and the
	// special values, beside operands that addSlow64 would take.
	specials := []bits{lo(bid.NaN64), lo(bid.SNaN64), lo(bid.Inf64), lo(bid.Inf64 | bid.SignMask64), lo(bid.NaN64 | 12345)}
	for _, c := range []uint64{bid.MaxCoeff64 + 1, 1<<53 | 1<<51 - 1} {
		x := uint64(bid.Form2Mask64) | uint64(bid.Bias64-3)<<bid.Form2ExpShift64 | c&(1<<bid.Form2CoeffBits64-1)
		specials = append(specials, lo(x), lo(x|bid.SignMask64))
	}
	for _, s := range specials {
		for _, gap := range []int{0, 1, 5, 19, 40} {
			for _, oc := range []*big.Int{new(big.Int), randDigits(rng, 16), randDigits(rng, 5)} {
				try(s, f.encode(value{kind: finite, neg: rng.IntN(2) == 0, coeff: oc, exp: -3 + gap}))
			}
		}
	}
	t.Logf("%d sums", n)
}
