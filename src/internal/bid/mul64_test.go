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

// TestProdTab64 recomputes the table of mulSlow64's rounding with math/big
// and checks the bounds that prodRound64's comment proves the estimate
// from: for every count of leading zeros z of a product that rounds, the
// digit count is t or t + 1, the reciprocals are those of 10^(t-15) and
// 10^(t-16) at the shift max(n - 62, 0), and the shifted product and the
// reciprocal are below 2^62. It then runs the estimate itself, with
// math/big, on products of n bits at the ends of the range, either side of
// 10^t and at random, and checks that it is the product rounded half up,
// or one less.
func TestProdTab64(t *testing.T) {
	rng := rand.New(rand.NewPCG(*seedFlag, streamSeed("prodtab64")))
	two := func(k uint) *big.Int { return new(big.Int).Lsh(bigOne, k) }
	_, _, _, _, _, _, _, _, size := bid.ProdTab64(0)
	if size != 128-54+1 {
		t.Fatalf("prodTab64 has %d entries, want %d", size, 128-54+1)
	}
	for z := 128 - 107; z <= 128-54; z++ {
		pow, d1, m1, d0, m0, j, s, u, _ := bid.ProdTab64(z)
		n := 128 - z
		td := n * 1233 >> 12
		// Every product of n bits has td or td + 1 digits.
		if bigPow10(td-1).Cmp(two(uint(n-1))) > 0 || two(uint(n)).Cmp(bigPow10(td+1)) > 0 {
			t.Fatalf("z = %d: products of %d bits do not have %d or %d digits", z, n, td, td+1)
		}
		if bigOf(pow).Cmp(bigPow10(td)) != 0 || j != uint(td-15) || s != uint(max(n-62, 0)) || u != (64-s)%64 {
			t.Fatalf("z = %d: pow %v, j %d, s %d, u %d; want 10^%d, %d, %d, %d", z, bigOf(pow), j, s, u, td, td-15, max(n-62, 0), (64-s)%64)
		}
		if s == 0 && n > 64 {
			t.Fatalf("z = %d: no shift for a product with a high word", z)
		}
		for _, c := range []struct {
			k    int
			d, m uint64
		}{{td - 15, d1, m1}, {td - 16, d0, m0}} {
			if c.k < 1 {
				if c.d != 0 || c.m != 0 {
					t.Fatalf("z = %d: entry for 10^%d is (%d, %#x), want zeros", z, c.k, c.d, c.m)
				}
				continue
			}
			d := bigPow10(c.k)
			m := new(big.Int).Quo(two(64+s), d)
			if new(big.Int).SetUint64(c.d).Cmp(d) != 0 || new(big.Int).SetUint64(c.m).Cmp(m) != 0 {
				t.Fatalf("z = %d: entry for 10^%d is (%d, %#x), want (%v, %#x)", z, c.k, c.d, c.m, d, m)
			}
			half := new(big.Int).Rsh(d, 1)
			// a < 2^62 for every product of n bits, and m < 2^62.
			if new(big.Int).Rsh(new(big.Int).Sub(two(uint(n)), bigOne), s).BitLen() > 62 || m.BitLen() > 62 {
				t.Fatalf("z = %d, 10^%d: the shifted product or the reciprocal reaches 2^62", z, c.k)
			}
			// The estimate on products of n bits.
			lo, hi := two(uint(n-1)), new(big.Int).Sub(two(uint(n)), bigOne)
			ps := []*big.Int{lo, new(big.Int).Add(lo, bigOne), hi, new(big.Int).Sub(hi, bigOne)}
			if pw := bigPow10(td); pw.Cmp(lo) > 0 && pw.Cmp(hi) <= 0 {
				ps = append(ps, pw, new(big.Int).Sub(pw, bigOne), new(big.Int).Add(pw, bigOne))
			}
			span := new(big.Int).Sub(hi, lo)
			for range 2000 {
				r := new(big.Int).SetUint64(rng.Uint64())
				r.Lsh(r, 64).Or(r, new(big.Int).SetUint64(rng.Uint64())).Mod(r, span)
				ps = append(ps, r.Add(r, lo))
			}
			for _, p := range ps {
				a := new(big.Int).Rsh(p, s)
				qh := new(big.Int).Rsh(a.Mul(a, m), 64)
				q := new(big.Int).Quo(new(big.Int).Add(p, half), d)
				if diff := new(big.Int).Sub(q, qh); diff.Sign() < 0 || diff.Cmp(bigOne) > 0 {
					t.Fatalf("z = %d, 10^%d: estimate %v of %v / 10^%d, want %v or one less", z, c.k, qh, p, c.k, q)
				}
			}
		}
	}
}

// TestMulStraight64 drives mulSlow64's own path against the reference in
// every mode (the directed modes take mulLong64, which the same operands
// then test) and with every pair of signs, through Mul64 and through
// mulSlow64 itself:
//
//   - every pair of digit counts, so that the product has from 1 to 32
//     digits in one word or two, is exact or loses from 1 to 16 digits,
//     with random coefficients and with the extremes of each length, whose
//     products sit on both sides of a power of ten, and coefficients either
//     side of 2^53, where the encoding changes form;
//   - ties: products that end in a five and zeros below the 16th digit,
//     with the kept digits even and odd, and the products either side;
//   - products whose rounding carries to 10^16, and those just below;
//   - exponents at both ends of the range, where the path must refuse
//     rather than encode, from both sides;
//   - zeros, coefficients that are not canonical, infinities and NaNs,
//     which it hands on.
func TestMulStraight64(t *testing.T) {
	var k kernel
	f := dec64
	p := f.prec
	rng := rand.New(rand.NewPCG(*seedFlag, streamSeed("mulstraight64")))
	pow := bigPow10
	n := 0
	bitsCheck := func(x, y bits) {
		for _, m := range append(allModes, bid.Mode(7)) {
			for _, pair := range [][2]bits{{x, y}, {y, x}} {
				n++
				want := f.mul(pair[0], pair[1], m)
				if got := k.mul(f, pair[0], pair[1], m); got != want {
					t.Fatalf("mul(%s, %s, %s) = %s, want %s", f.format(pair[0]), f.format(pair[1]), modeName(m), f.format(got), f.format(want))
				}
				if got := lo(bid.MulSlow64(pair[0].Lo, pair[1].Lo, m)); got != want {
					t.Fatalf("mulSlow64(%s, %s, %s) = %s, want %s", f.format(pair[0]), f.format(pair[1]), modeName(m), f.format(got), f.format(want))
				}
			}
		}
	}
	check := func(xc, yc *big.Int, xe, ye int) {
		if xc.Sign() < 0 || yc.Sign() < 0 || xc.Cmp(f.maxCoeff) > 0 || yc.Cmp(f.maxCoeff) > 0 {
			return
		}
		if xe < f.minExp || xe > f.maxExp || ye < f.minExp || ye > f.maxExp {
			return
		}
		for _, signs := range [][2]bool{{false, false}, {false, true}, {true, true}} {
			bitsCheck(f.encode(value{kind: finite, neg: signs[0], coeff: xc, exp: xe}), f.encode(value{kind: finite, neg: signs[1], coeff: yc, exp: ye}))
		}
	}
	coeffs := func(d int) []*big.Int {
		lo, hi := pow(d-1), new(big.Int).Sub(pow(d), bigOne)
		cs := []*big.Int{randDigits(rng, d), randDigits(rng, d), lo, hi}
		if d > 1 {
			cs = append(cs, new(big.Int).Add(lo, bigOne), new(big.Int).Sub(hi, bigOne))
		}
		if d == p {
			// Either side of 2^53, the first coefficient of the second form.
			f2 := new(big.Int).Lsh(bigOne, 53)
			cs = append(cs, f2, new(big.Int).Sub(f2, bigOne), new(big.Int).Add(f2, bigOne))
		}
		return cs
	}
	rounds := 1
	if *longTest {
		rounds = 20
	}
	for range rounds {
		for dx := 1; dx <= p; dx++ {
			for dy := 1; dy <= p; dy++ {
				for _, xc := range coeffs(dx) {
					for _, yc := range coeffs(dy) {
						check(xc, yc, -dx, 3-dy)
					}
				}
				// The ends of the range: the product's exponent at and
				// around the smallest and the largest at which the path
				// encodes, after it drops its digits.
				xc, yc := randDigits(rng, dx), randDigits(rng, dy)
				drop := digits(new(big.Int).Mul(xc, yc)) - p
				for _, e := range []int{f.minExp - drop - 1, f.minExp - drop, f.minExp - drop + 1, f.minExp, f.minExp - 1, f.maxExp - drop - 1, f.maxExp - drop, f.maxExp - drop + 1, f.maxExp, f.maxExp + 1} {
					xe := e / 2
					check(xc, yc, xe, e-xe)
					check(xc, yc, f.minExp, e-f.minExp)
					check(xc, yc, f.maxExp, e-f.maxExp)
				}
			}
		}
	}
	// Ties. An odd x of the full precision times 5 × 10^(a-1) is 5x and
	// a-1 zeros: when 5x has one digit more than the precision, a five and
	// zeros lie below the 16th digit. The digits kept are even and odd as
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
	// The carry to 10^16: a product of 16 + j digits from (10^16 - 1/2) ×
	// 10^j up, which rounds to 10^16, and the products just below it. For
	// each j some y just above 10^j has a multiple x × y in that range with
	// x below 10^16.
	carries := 0
	for j := 1; j <= p; j++ {
		top := pow(p + j)
		low := new(big.Int).Sub(top, new(big.Int).Quo(pow(j), big.NewInt(2)))
		for y := new(big.Int).Add(pow(j), bigOne); y.Cmp(new(big.Int).Add(pow(j), big.NewInt(400))) < 0; y.Add(y, bigOne) {
			x := new(big.Int).Add(low, y)
			x.Sub(x, bigOne).Quo(x, y) // ⌈low / y⌉
			if new(big.Int).Mul(x, y).Cmp(top) >= 0 || x.Cmp(f.maxCoeff) > 0 {
				continue
			}
			carries++
			for _, dx := range []int64{0, -1, -2} {
				check(new(big.Int).Add(x, big.NewInt(dx)), y, -j, 7)
				check(new(big.Int).Add(x, big.NewInt(dx)), y, f.maxExp-j-1, f.maxExp/2)
			}
			break
		}
	}
	if carries < p/2 {
		t.Fatalf("only %d carries to 10^16", carries)
	}
	// Many products of two full coefficients drawn at random, in one mode.
	for range 200000 * rounds {
		x := f.encode(value{kind: finite, neg: rng.IntN(2) == 0, coeff: randDigits(rng, 1+rng.IntN(p)), exp: rng.IntN(40) - 20})
		y := f.encode(value{kind: finite, neg: rng.IntN(2) == 0, coeff: randDigits(rng, 1+rng.IntN(p)), exp: rng.IntN(40) - 20})
		n++
		if got, want := lo(bid.MulSlow64(x.Lo, y.Lo, bid.ToNearestEven)), f.mul(x, y, bid.ToNearestEven); got != want {
			t.Fatalf("mulSlow64(%s, %s) = %s, want %s", f.format(x), f.format(y), f.format(got), f.format(want))
		}
	}
	// Zeros, coefficients that are not canonical, and the special values,
	// beside a full coefficient and beside small ones at the least
	// exponents, whose product with the coefficient field of an infinity or
	// a NaN read as the second form would lie within the format.
	full := f.encode(value{kind: finite, coeff: randDigits(rng, p), exp: -5})
	smalls := []bits{
		f.encode(value{kind: finite, coeff: big.NewInt(1), exp: f.minExp}),
		f.encode(value{kind: finite, coeff: big.NewInt(7), exp: f.minExp + 3}),
		f.encode(value{kind: finite, coeff: randDigits(rng, p), exp: f.minExp}),
	}
	specials := []bits{
		f.encode(value{kind: finite, coeff: new(big.Int), exp: -3}),
		f.encode(value{kind: finite, coeff: new(big.Int), exp: f.maxExp}),
		lo(3<<61 | 400<<51 | (1<<51 - 1)),                   // 2^53 + 2^51 - 1, not canonical
		lo(3<<61 | 400<<51 | (MaxCoeff64Plus1 - 1<<53)),     // 10^16, not canonical
		lo(3<<61 | 767<<51 | (MaxCoeff64Plus1 - 1<<53 - 1)), // the largest canonical, at the largest exponent
		f.encode(value{kind: infinite}),
		f.encode(value{kind: notANumber}),
		f.encode(value{kind: notANumber, signaling: true}),
	}
	for _, s := range specials {
		bitsCheck(s, full)
		bitsCheck(s, k.neg(f, full))
		bitsCheck(s, s)
		for _, y := range smalls {
			bitsCheck(s, y)
		}
	}
	t.Logf("%d products, %d ties, %d carries", n, ties, carries)
}

// MaxCoeff64Plus1 is 10^16.
const MaxCoeff64Plus1 = bid.MaxCoeff64 + 1
