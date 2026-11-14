// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// Directed tests of div64's own path (div64.go): its scale table, the bound
// of its floating-point estimate, and its results against the reference
// model at every bound it decides.

import (
	"internal/bid"
	"math/big"
	"math/rand/v2"
	"testing"
)

// TestDiv64Scale checks div64Scale: the power and the places of each entry,
// and that every coefficient with that many leading zeros, scaled by it, has
// 15 or 16 digits, 16 exactly when its digit count is 16 less the places.
func TestDiv64Scale(t *testing.T) {
	for n := range 64 {
		p, places := bid.Div64ScaleEntry(n)
		est := min((64-n)*1233>>12, 15)
		if places != 15-est || p != pow10word(places) {
			t.Errorf("div64Scale[%d] = {%d, %d}, want {10^%d, %d}", n, p, places, 15-est, 15-est)
		}
		if n < 10 {
			continue // no coefficient has fewer than 10 leading zeros
		}
		lo, hi := uint64(1)<<(63-n), min(uint64(1)<<(64-n)-1, bid.MaxCoeff64)
		for _, a := range []uint64{lo, lo + 1, hi - 1, hi, pow10word(est), pow10word(est) - 1} {
			if a < lo || a > hi {
				continue
			}
			s := new(big.Int).Mul(new(big.Int).SetUint64(a), new(big.Int).SetUint64(p))
			d := digits(s)
			if d != 15 && d != 16 {
				t.Errorf("div64Scale[%d]: %d × 10^%d has %d digits", n, a, places, d)
			}
			if da := digits(new(big.Int).SetUint64(a)); (d == 16) != (da == 16-places) {
				t.Errorf("div64Scale[%d]: %d × 10^%d has %d digits and %d has %d", n, a, places, d, a, da)
			}
		}
	}
}

func pow10word(k int) uint64 { return pow10(k).Uint64() }

// div64Scaled returns the places k by which div64 scales the dividend a
// over the divisor b, so that ⌊a × 10^k / b⌋ has 16 digits, and that
// quotient biased by half the divisor, with its remainder.
func div64Scaled(a, b uint64) (k int, q, r *big.Int) {
	ab, bb := new(big.Int).SetUint64(a), new(big.Int).SetUint64(b)
	da, db := digits(ab), digits(bb)
	k = 16 - da + db
	if new(big.Int).Mul(ab, pow10(db)).Cmp(new(big.Int).Mul(bb, pow10(da))) >= 0 {
		k-- // the leading ratio reaches one
	}
	n := new(big.Int).Mul(ab, pow10(k))
	n.Add(n, new(big.Int).SetUint64(b/2))
	q, r = new(big.Int).QuoRem(n, bb, new(big.Int))
	return k, q, r
}

// TestDiv64Estimate checks the bound that div64's correction allows for:
// the estimate of a quotient is within 7 of it. It draws coefficients of
// every length, and those the estimate rounds most: at and above 2^53,
// where the coefficients are not exact in float64, with scales beyond
// 10^22, where the power is not, and quotients next to 10^15 and 10^16.
func TestDiv64Estimate(t *testing.T) {
	rng := rand.New(rand.NewPCG(*seedFlag, streamSeed("div64estimate")))
	worst := int64(0)
	check := func(a, b uint64) {
		if a == 0 || b == 0 || a > bid.MaxCoeff64 || b > bid.MaxCoeff64 {
			return
		}
		k, q, _ := div64Scaled(a, b)
		if digits(q) > 16 {
			t.Fatalf("%d / %d: quotient %v has more than 16 digits", a, b, q)
		}
		d := int64(bid.Div64Estimate(a, b, k)) - q.Int64()
		if d < 0 {
			d = -d
		}
		worst = max(worst, d)
		if d > 7 {
			t.Errorf("Div64Estimate(%d, %d, %d) is %d from the quotient %v", a, b, k, d, q)
		}
	}
	n := 200000
	if testing.Short() {
		n = 20000
	}
	for range n {
		check(randDigits(rng, 1+rng.IntN(16)).Uint64(), randDigits(rng, 1+rng.IntN(16)).Uint64())
		// Both coefficients inexact in float64.
		check(1<<53+rng.Uint64N(bid.MaxCoeff64-1<<53), 1<<53+rng.Uint64N(bid.MaxCoeff64-1<<53))
		// A short dividend over a long divisor: the largest scales.
		check(1+rng.Uint64N(99), randDigits(rng, 14+rng.IntN(3)).Uint64())
		// A leading ratio within one unit of one, either way.
		b := randDigits(rng, 10+rng.IntN(7)).Uint64()
		for _, a := range []uint64{b - 1, b, b + 1} {
			check(a, b)
			for p := uint64(10); a*p/p == a && a*p <= bid.MaxCoeff64; p *= 10 {
				check(a*p, b)
				check(b, a*p)
			}
		}
	}
	for _, a := range []uint64{1, 2, 9, 1 << 53, 1<<53 - 1, 1<<53 + 1, bid.MaxCoeff64, bid.MaxCoeff64 - 1} {
		for _, b := range []uint64{1, 3, 7, 1e9 - 1, 1e9 + 1, 1 << 53, 1<<53 - 1, 1<<53 + 1, bid.MaxCoeff64, bid.MaxCoeff64 - 1} {
			check(a, b)
		}
	}
	t.Logf("largest distance from the quotient: %d", worst)
}

// TestDiv64Words drives div64's own path against the reference in every
// mode, and one beyond them: every pair of digit counts, which puts the
// divisor on both sides of the bound between two word divisions and the
// estimate and the leading ratio on both sides of one; exact quotients and
// divisors that are powers of ten, whose zeros are stripped; ties and their
// neighbours under even divisors, and odd divisors whose biased remainder is
// zero, on both sides of the bound; the quotients nearest below 10^16; the
// coefficients of the second form and those that are not canonical;
// exponents that put the quotient at and one beyond both ends of the range;
// and zeros, infinities and NaNs beside words.
func TestDiv64Words(t *testing.T) {
	var k kernel
	f := dec64
	rng := rand.New(rand.NewPCG(*seedFlag, streamSeed("div64words")))
	modes := append(append([]bid.Mode{}, allModes...), 5)
	n := 0
	checkBits := func(x, y bits) {
		for _, m := range modes {
			n++
			if got, want := k.div(f, x, y, m), f.div(x, y, m); got != want {
				t.Fatalf("div(%s, %s, %s) = %s, want %s", display(f, x), display(f, y), modeName(m), display(f, got), display(f, want))
			}
		}
	}
	check := func(xc, yc *big.Int, xe, ye int) {
		if xc.Cmp(f.maxCoeff) > 0 || yc.Cmp(f.maxCoeff) > 0 || xe < f.minExp || xe > f.maxExp || ye < f.minExp || ye > f.maxExp {
			return
		}
		for _, neg := range []bool{false, true} {
			x := f.encode(value{kind: finite, neg: neg, coeff: xc, exp: xe})
			y := f.encode(value{kind: finite, coeff: yc, exp: ye})
			checkBits(x, y)
		}
	}
	exps := func() (int, int) {
		switch rng.IntN(8) {
		case 0: // the quotient near the smallest exponent
			return f.minExp + rng.IntN(40), rng.IntN(20)
		case 1: // and near the largest
			return f.maxExp - rng.IntN(20), -rng.IntN(40)
		}
		return rng.IntN(41) - 20, rng.IntN(41) - 20
	}
	rounds := 200
	if *longTest {
		rounds = 5000
	} else if testing.Short() {
		rounds = 20
	}
	for dx := 1; dx <= f.prec; dx++ {
		for dy := 1; dy <= f.prec; dy++ {
			for range rounds {
				xc, yc := randDigits(rng, dx), randDigits(rng, dy)
				xe, ye := exps()
				check(xc, yc, xe, ye)
				// An exact quotient: the dividend is a multiple.
				if m := new(big.Int).Mul(yc, randDigits(rng, 1+rng.IntN(dx))); m.Cmp(f.maxCoeff) <= 0 {
					check(m, yc, xe, ye)
				}
				// A power of ten and the words either side of it.
				p10 := pow10(dy - 1)
				check(xc, p10, xe, ye)
				check(xc, new(big.Int).Add(p10, bigOne), xe, ye)
				if dy > 1 {
					check(xc, new(big.Int).Sub(p10, bigOne), xe, ye)
				}
			}
		}
	}

	// The bound between two word divisions and the estimate, from both
	// sides, under dividends of every length, and divisors near 2^53.
	for _, y := range []uint64{1e9 - 1, 1e9 + 1, 999999937, 1000000007, 1e9 - 2, 1e9 + 2, 1<<53 - 1, 1<<53 + 1, 1 << 53} {
		yc := new(big.Int).SetUint64(y)
		for dx := 1; dx <= f.prec; dx++ {
			for range max(rounds/10, 4) {
				check(randDigits(rng, dx), yc, 0, 0)
				check(yc, randDigits(rng, dx), 0, 0)
			}
		}
	}

	// Ties and the fractions next to them: odd dividends over powers of
	// two, and over powers of two times powers of five, both sides of
	// the bound; and the same divisors under small dividends.
	for a := 1; a <= 53; a++ {
		for _, fives := range []int{0, 1, 3, 7} {
			yc := new(big.Int).Lsh(new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(fives)), nil), uint(a))
			if yc.Cmp(f.maxCoeff) > 0 {
				continue
			}
			for range max(rounds/10, 4) {
				xc := randDigits(rng, 1+rng.IntN(f.prec))
				check(xc.SetBit(xc, 0, 1), yc, 0, 0)
				check(big.NewInt(int64(1+2*rng.IntN(500))), yc, 0, 0)
			}
		}
	}
	for y := int64(2); y <= 300; y++ {
		for x := int64(1); x <= 40; x++ {
			check(big.NewInt(x), big.NewInt(y), 0, 0)
			check(big.NewInt(x), big.NewInt(y), -2, -2)
		}
	}

	// An odd divisor whose biased remainder is zero: a fraction just above
	// the half, which rounds up and is no tie (see TestDivWords for the
	// construction), on both sides of the bound.
	found := 0
	for dy := 1; dy <= f.prec; dy++ {
		for range max(rounds/2, 40) {
			yc := randDigits(rng, dy)
			yc.SetBit(yc, 0, 1)
			if new(big.Int).Mod(yc, big.NewInt(5)).Sign() == 0 {
				continue
			}
			k := max(dy-1, 1) + rng.IntN(3)
			m := pow10(k)
			m2 := new(big.Int).Lsh(m, 1)
			inv := new(big.Int).ModInverse(yc, m2)
			q := new(big.Int).Sub(m2, inv)
			q.Rsh(q, 1)
			top := randDigits(rng, f.prec)
			top.Sub(top, new(big.Int).Mod(top, m)).Add(top, q)
			if digits(top) != f.prec {
				continue
			}
			xc := new(big.Int).Mul(top, yc)
			xc.Add(xc, new(big.Int).Rsh(new(big.Int).Add(yc, bigOne), 1))
			if new(big.Int).Mod(xc, m).Sign() != 0 {
				t.Fatalf("construction: %v not a multiple of 10^%d", xc, k)
			}
			xc.Quo(xc, m)
			if xc.Cmp(f.maxCoeff) <= 0 {
				found++
				check(xc, yc, 0, 0)
			}
		}
	}
	if found < 50 {
		t.Fatalf("only %d odd divisors with a biased remainder of zero", found)
	}

	// The quotients nearest below 10^16, which round to it in no case
	// (TestDivNearest), and the largest coefficients over each other.
	for _, d := range []int64{0, 1, 2, 3, 7} {
		xc := new(big.Int).Sub(f.maxCoeff, big.NewInt(d))
		for _, e := range []int64{0, 1, 2, 5} {
			check(xc, new(big.Int).Sub(f.maxCoeff, big.NewInt(e)), 0, 0)
			check(new(big.Int).Sub(f.maxCoeff, big.NewInt(e)), xc, 0, 0)
		}
		for dy := 1; dy <= f.prec; dy++ {
			nines := new(big.Int).Sub(pow10(dy), bigOne)
			check(xc, nines, 0, 0)
			check(nines, xc, 0, 0)
		}
	}

	// The exponent of the result at and one beyond each end of the range,
	// for inexact quotients and for exact ones, whose exponent moves with
	// the zeros stripped.
	edges := 0
	for range max(rounds*4, 200) {
		a := randDigits(rng, 1+rng.IntN(f.prec)).Uint64()
		b := randDigits(rng, 1+rng.IntN(f.prec)).Uint64()
		if rng.IntN(3) == 0 {
			if p := a * b; a <= f.maxCoeff.Uint64() && b != 0 && p/b == a && p <= f.maxCoeff.Uint64() {
				a = p // exact
			}
		}
		k, _, _ := div64Scaled(a, b)
		ac, bc := new(big.Int).SetUint64(a), new(big.Int).SetUint64(b)
		for _, d := range []int{-1, 0, 1} {
			// exp - k = minExp + d, and exp - k = maxExp + d.
			check(ac, bc, f.minExp+d+k, 0)
			check(ac, bc, f.maxExp, -k-d)
			edges++
		}
		// The preferred exponent itself at each end.
		for _, d := range []int{-1, 0, 1} {
			check(ac, bc, f.minExp+d, 0)
			check(ac, bc, f.maxExp, -d)
		}
	}

	// Second-form coefficients, and encodings of that form whose
	// coefficient is above the largest, which read as zero.
	nonCanonical := func(c uint64, exp int) bits {
		return bits{Lo: bid.Form2Mask64 | uint64(exp+f.bias)<<bid.Form2ExpShift64 | c&(1<<bid.Form2CoeffBits64-1)}
	}
	for range max(rounds, 40) {
		big2 := new(big.Int).SetUint64(1<<53 + rng.Uint64N(f.maxCoeff.Uint64()-1<<53+1))
		w := randDigits(rng, 1+rng.IntN(f.prec))
		check(big2, w, 0, 0)
		check(w, big2, 0, 0)
		check(big2, big2, 3, -3)
		bad := nonCanonical(f.maxCoeff.Uint64()+1+rng.Uint64N(1<<54-f.maxCoeff.Uint64()-1), rng.IntN(41)-20)
		wb := f.encode(value{kind: finite, coeff: w, exp: rng.IntN(41) - 20})
		checkBits(bad, wb)
		checkBits(wb, bad)
		checkBits(bad, bad)
		checkBits(f.encode(value{kind: finite, coeff: new(big.Int), exp: 0}), bad)
	}

	// Zeros, infinities and NaNs beside words of every sort.
	specials := []bits{
		f.encode(value{kind: infinite}), f.encode(value{kind: infinite, neg: true}),
		f.encode(value{kind: notANumber}), {Lo: bid.NaN64 | 1<<57 | 7},
	}
	for _, v := range []uint64{1, 2, 10, 7890, 1e9, 1<<53 + 5, bid.MaxCoeff64} {
		vc := new(big.Int).SetUint64(v)
		for _, e := range []int{f.minExp, -30, 0, 30, f.maxExp} {
			check(new(big.Int), vc, e, 0)
			check(new(big.Int), vc, e, f.maxExp)
			check(new(big.Int), vc, e, f.minExp)
			check(vc, new(big.Int), e, 0)
			w := f.encode(value{kind: finite, coeff: vc, exp: e})
			for _, s := range specials {
				checkBits(s, w)
				checkBits(w, s)
			}
		}
	}
	check(new(big.Int), new(big.Int), 0, 0)
	if n == 0 || edges == 0 {
		t.Fatal("no division checked")
	}
}

// TestDiv64Paths counts, over operands drawn as TestDiv64Words' first loop
// draws them, how often div64's own path takes each of its ways to the
// quotient, so that a change that left one of them untested would show.
func TestDiv64Paths(t *testing.T) {
	rng := rand.New(rand.NewPCG(*seedFlag, streamSeed("div64paths")))
	counts := map[string]int{}
	for dx := 1; dx <= 16; dx++ {
		for dy := 1; dy <= 16; dy++ {
			for i := range 40 {
				a, b := randDigits(rng, dx).Uint64(), randDigits(rng, dy).Uint64()
				if m := randDigits(rng, 1+rng.IntN(dx)).Uint64(); i%2 == 1 && m <= bid.MaxCoeff64/b {
					a = b * m // an exact quotient
				}
				_, q, r := div64Scaled(a, b)
				way := "estimate"
				if b < 1e9 {
					way = "two divisions"
				}
				switch {
				case b == pow10word(dy-1):
					way = "power of ten"
				case r.Uint64() == b/2:
					way += ", exact"
				case r.Sign() == 0 && b%2 == 0:
					way += ", tie"
				}
				if q.Uint64() >= 1<<53 {
					counts["second form"]++
				}
				counts[way]++
			}
		}
	}
	for _, way := range []string{"estimate", "two divisions", "power of ten", "estimate, exact", "two divisions, exact", "second form"} {
		if counts[way] == 0 {
			t.Errorf("no quotient takes the way %q", way)
		}
	}
	t.Logf("%v", counts)
}
