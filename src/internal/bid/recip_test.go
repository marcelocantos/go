// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"fmt"
	"internal/bid"
	"math/big"
	mbits "math/bits"
	"math/rand/v2"
	"testing"
)

// Tests of the reciprocal division of recip.go, against math/big and
// against the long division it replaced, which is kept below as
// oldDivMod256 so that the comparison does not depend on the package's
// current division code.

// TestRecipTables recomputes the reciprocal tables with math/big and
// checks the two facts the exactness argument in recip.go needs of them:
// a < 2^(n-1) for every dividend below the bound, and m < 2^(n-3).
func TestRecipTables(t *testing.T) {
	for _, w := range []struct {
		bits  uint
		count int
		entry func(k int) (*big.Int, uint)
	}{
		{64, len(bid.Pow10tab64), func(k int) (*big.Int, uint) {
			m, s := bid.Recip10tab64(k)
			return new(big.Int).SetUint64(m), s
		}},
		{128, len(bid.Pow10tab128), func(k int) (*big.Int, uint) {
			m, s := bid.Recip10tab128(k)
			return bigOf(m), s
		}},
	} {
		for k := 1; k < w.count; k++ {
			d := bigPow10(k)
			s := uint(max(d.BitLen()-1-6, 0))
			m := new(big.Int).Lsh(bigOne, w.bits+s)
			m.Quo(m, d)
			gotM, gotS := w.entry(k)
			if gotM.Cmp(m) != 0 || gotS != s {
				t.Errorf("%d-bit reciprocal of 10^%d = (%v, %d), want (%v, %d)", w.bits, k, gotM, gotS, m, s)
			}
			if m.BitLen() > int(w.bits)-3 {
				t.Errorf("%d-bit reciprocal of 10^%d has %d bits, want at most %d", w.bits, k, m.BitLen(), w.bits-3)
			}
			// The largest dividend below the bound d × 2^(n-8), shifted.
			a := new(big.Int).Lsh(d, w.bits-8)
			a.Sub(a, bigOne).Rsh(a, s)
			if a.BitLen() > int(w.bits)-1 {
				t.Errorf("%d-bit reciprocal of 10^%d: a has %d bits, want at most %d", w.bits, k, a.BitLen(), w.bits-1)
			}
		}
	}
	// The plain reciprocal of recip128.go, ⌊(2^128 - 1) / 10^k⌋, from k = 0.
	top := new(big.Int).Lsh(bigOne, 128)
	top.Sub(top, bigOne)
	for k := range len(bid.Pow10tab128) {
		want := new(big.Int).Quo(top, bigPow10(k))
		if got := bigOf(bid.RecipTrunc10tab128(k)); got.Cmp(want) != 0 {
			t.Errorf("t of the 128-bit reciprocal of 10^%d = %#x, want %#x", k, got, want)
		}
	}
}

// recipValues returns the dividends that TestRecipDivPow10 tries for a
// divisor of 10^k: j × 10^k - 1, j × 10^k and j × 10^k + 1 for quotients j
// at the edges (small, the word boundaries, the precision of each width,
// the reciprocal's bound 2^(n-8) and past it), the values just below and at
// the bound itself, values near 2^(2n), and random ones of every length.
func recipValues(rng *rand.Rand, bits uint, k, random int) []*big.Int {
	d := bigPow10(k)
	limit := new(big.Int).Lsh(bigOne, 2*bits)
	var js []*big.Int
	for _, j := range []uint64{0, 1, 2, 3, 5, 9, 10, 99, 100, 1<<32 - 1, 1 << 32, 1<<63 - 1, 1 << 63, 1<<64 - 1} {
		js = append(js, new(big.Int).SetUint64(j))
	}
	for _, e := range []uint{bits - 9, bits - 8, bits - 7, bits, bits + 1} {
		p := new(big.Int).Lsh(bigOne, e)
		js = append(js, new(big.Int).Sub(p, bigOne), p, new(big.Int).Add(p, bigOne))
	}
	for _, n := range []int{15, 16, 17, 19, 20, 33, 34, 35, 38, 39} {
		p := bigPow10(n)
		js = append(js, new(big.Int).Sub(p, bigOne), p)
	}
	var vs []*big.Int
	add := func(v *big.Int) {
		if v.Sign() >= 0 && v.Cmp(limit) < 0 {
			vs = append(vs, v)
		}
	}
	for _, j := range js {
		p := new(big.Int).Mul(j, d)
		add(new(big.Int).Sub(p, bigOne))
		add(p)
		add(new(big.Int).Add(p, bigOne))
		add(new(big.Int).Add(p, new(big.Int).Rsh(d, 1))) // a tie
	}
	for _, v := range []*big.Int{
		new(big.Int).Sub(limit, bigOne),
		new(big.Int).Sub(limit, d),
		new(big.Int).Sub(limit, new(big.Int).Lsh(d, 1)),
	} {
		add(v)
	}
	for range random {
		v := new(big.Int)
		for i := range 2 * bits / 64 {
			v.Or(v, new(big.Int).Lsh(new(big.Int).SetUint64(rng.Uint64()), uint(64*i)))
		}
		add(v.Rsh(v, uint(rng.IntN(int(2*bits)))))
	}
	return vs
}

// bigClass is the digit class of the remainder r of a division by d.
func bigClass(r, d *big.Int) bid.DigitClass {
	if r.Sign() == 0 {
		return bid.ClassZero
	}
	switch new(big.Int).Lsh(r, 1).Cmp(d) {
	case -1:
		return bid.ClassBelowHalf
	case 0:
		return bid.ClassHalf
	}
	return bid.ClassAboveHalf
}

// TestRecipDivPow10 compares the reciprocal divisions with math/big and
// with the long division, for every power of ten of each table: the
// division itself, its bound (ok exactly when the quotient is below
// 2^(n-8)), and the shifts and reductions that use it.
func TestRecipDivPow10(t *testing.T) {
	random := 1_000_000 / (len(bid.Pow10tab64) + len(bid.Pow10tab128))
	if testing.Short() {
		random /= 10
	}
	rng := rand.New(rand.NewPCG(13, 0x5ec1))
	q, r := new(big.Int), new(big.Int)

	for k := range len(bid.Pow10tab128) {
		d := bigPow10(k)
		bound := new(big.Int).Lsh(d, 120)
		for _, v := range recipValues(rng, 128, k, random) {
			q.QuoRem(v, d, r)
			p := from256(v)
			what := fmt.Sprintf("10^%d into %v", k, v)
			gq, gr, ok := bid.U256DivPow10(p, k)
			if want := v.Cmp(bound) < 0; ok != want {
				t.Fatalf("U256DivPow10(%s): ok = %t, want %t", what, ok, want)
			}
			if ok {
				eq128(t, "U256DivPow10("+what+") q", gq, q)
				eq128(t, "U256DivPow10("+what+") r", gr, r)
			}
			oq, or := oldDivMod256(p, bid.Pow10tab128[k])
			eq256(t, "old division "+what+" q", oq, q)
			eq128(t, "old division "+what+" r", or, r)

			class := bigClass(r, d)
			sq, sc := bid.CShiftFull128(p, k)
			eq256(t, "CShiftFull128("+what+") q", sq, q)
			if sc != class {
				t.Errorf("CShiftFull128(%s) class = %d, want %d", what, sc, class)
			}
			if q.BitLen() <= 128 {
				rq, rc := bid.CReduceClass128(p, k)
				eq128(t, "CReduceClass128("+what+") q", rq, q)
				if rc != class {
					t.Errorf("CReduceClass128(%s) class = %d, want %d", what, rc, class)
				}
			}
			if v.BitLen() <= 128 {
				cq, cc := bid.CShift128(p.Lo, k)
				eq128(t, "CShift128("+what+") q", cq, q)
				if cc != class {
					t.Errorf("CShift128(%s) class = %d, want %d", what, cc, class)
				}
				dq, dr := bid.CDivPow10128(p.Lo, k)
				eq128(t, "CDivPow10128("+what+") q", dq, q)
				eq128(t, "CDivPow10128("+what+") r", dr, r)
			}
		}
	}

	for k := range len(bid.Pow10tab64) {
		d := bigPow10(k)
		bound := new(big.Int).Lsh(d, 56)
		for _, v := range recipValues(rng, 64, k, random) {
			q.QuoRem(v, d, r)
			p := from128(v)
			what := fmt.Sprintf("10^%d into %v", k, v)
			gq, gr, ok := bid.DivPow10Recip64(p, k)
			if want := v.Cmp(bound) < 0; ok != want {
				t.Fatalf("DivPow10Recip64(%s): ok = %t, want %t", what, ok, want)
			}
			if ok && (new(big.Int).SetUint64(gq).Cmp(q) != 0 || new(big.Int).SetUint64(gr).Cmp(r) != 0) {
				t.Errorf("DivPow10Recip64(%s) = %d, %d, want %v, %v", what, gq, gr, q, r)
			}
			if p.Hi < bid.Pow10tab64[k] {
				oq, or := mbits.Div64(p.Hi, p.Lo, bid.Pow10tab64[k])
				if new(big.Int).SetUint64(oq).Cmp(q) != 0 || new(big.Int).SetUint64(or).Cmp(r) != 0 {
					t.Errorf("bits.Div64(%s) = %d, %d, want %v, %v", what, oq, or, q, r)
				}
				rq, rc := bid.CReduceClass64(p, k)
				if new(big.Int).SetUint64(rq).Cmp(q) != 0 || rc != bigClass(r, d) {
					t.Errorf("CReduceClass64(%s) = %d, %d, want %v, %d", what, rq, rc, q, bigClass(r, d))
				}
			}
		}
	}
}

// oldDivMod256 is the long division that u256DivMod128 performed before
// the reciprocal divisions, copied here as a second oracle: Knuth's
// Algorithm D in base 2^64 with a normalised two-word divisor, each
// quotient word estimated by bits.Div64.
func oldDivMod256(n bid.Uint256, d bid.Uint128) (q bid.Uint256, r bid.Uint128) {
	var rh bid.Uint128
	q.Hi, rh = oldDiv128(bid.Uint256{Lo: n.Hi}, d)
	q.Lo, r = oldDiv128(bid.Uint256{Hi: rh, Lo: n.Lo}, d)
	return q, r
}

// oldDiv128 divides n by d, given n.Hi < d.
func oldDiv128(n bid.Uint256, d bid.Uint128) (q, r bid.Uint128) {
	if d.Hi == 0 {
		var r64 uint64
		q.Hi, r64 = mbits.Div64(n.Hi.Lo, n.Lo.Hi, d.Lo)
		q.Lo, r64 = mbits.Div64(r64, n.Lo.Lo, d.Lo)
		return q, bid.Uint128{Lo: r64}
	}
	s := uint(mbits.LeadingZeros64(d.Hi))
	v1 := d.Hi<<s | d.Lo>>(64-s)
	v0 := d.Lo << s
	w3 := n.Hi.Hi<<s | n.Hi.Lo>>(64-s)
	w2 := n.Hi.Lo<<s | n.Lo.Hi>>(64-s)
	w1 := n.Lo.Hi<<s | n.Lo.Lo>>(64-s)
	w0 := n.Lo.Lo << s
	var r1, r0 uint64
	q.Hi, r1, r0 = oldDiv3by2(w3, w2, w1, v1, v0)
	q.Lo, r1, r0 = oldDiv3by2(r1, r0, w0, v1, v0)
	return q, bid.Uint128{Hi: r1 >> s, Lo: r0>>s | r1<<(64-s)}
}

func oldDiv3by2(u2, u1, u0, v1, v0 uint64) (q, r1, r0 uint64) {
	if u2 >= v1 {
		q = 1<<64 - 1
	} else {
		q, _ = mbits.Div64(u2, u1, v1)
	}
	h0, p0 := mbits.Mul64(q, v0)
	h1, p1 := mbits.Mul64(q, v1)
	var c, b uint64
	p1, c = mbits.Add64(p1, h0, 0)
	p2 := h1 + c
	r0, b = mbits.Sub64(u0, p0, 0)
	r1, b = mbits.Sub64(u1, p1, b)
	r2, b := mbits.Sub64(u2, p2, b)
	for b != 0 {
		q--
		r0, c = mbits.Add64(r0, v0, 0)
		r1, c = mbits.Add64(r1, v1, c)
		r2, c = mbits.Add64(r2, 0, c)
		b ^= c
	}
	_ = r2
	return q, r1, r0
}

// TestReciprocal3by2 compares the two-word reciprocal with math/big.
func TestReciprocal3by2(t *testing.T) {
	two192 := new(big.Int).Lsh(bigOne, 192)
	two192.Sub(two192, bigOne)
	check := func(d1, d0 uint64) {
		d := bigOf(bid.Uint128{Hi: d1, Lo: d0})
		want := new(big.Int).Quo(two192, d)
		want.Sub(want, bigTwo64)
		if got := bid.Reciprocal3by2(d1, d0); new(big.Int).SetUint64(got).Cmp(want) != 0 {
			t.Fatalf("Reciprocal3by2(%#x, %#x) = %#x, want %v", d1, d0, got, want)
		}
	}
	edges := []uint64{0, 1, 2, 1<<63 - 1, 1 << 63, 1<<64 - 2, 1<<64 - 1}
	for _, d1 := range []uint64{1 << 63, 1<<63 + 1, 1<<64 - 2, 1<<64 - 1} {
		for _, d0 := range edges {
			check(d1, d0)
		}
	}
	rng := rand.New(rand.NewPCG(2011, 0x3233))
	for range 200_000 {
		check(rng.Uint64()|1<<63, rng.Uint64())
	}
}

// TestRecipDivide compares u256Div128, the division by an arbitrary divisor
// whose quotient fits 128 bits, with math/big and with the old long
// division, for divisors at the word boundary (2^64 ± 1), at the precision
// of decimal128 (10^33, 10^34 - 1, 10^34), at the edges of normalisation,
// and at random, with dividends at the bound n.Hi < d, near multiples of the
// divisor, and at random.
func TestRecipDivide(t *testing.T) {
	rng := rand.New(rand.NewPCG(2011, 0x3334))
	var ds []*big.Int
	for _, d := range []*big.Int{
		new(big.Int).Sub(bigTwo64, bigOne), bigTwo64, new(big.Int).Add(bigTwo64, bigOne),
		bigPow10(33), new(big.Int).Sub(bigPow10(34), bigOne), bigPow10(34), bigPow10(19), bigPow10(38),
		big.NewInt(1), big.NewInt(3), big.NewInt(10), new(big.Int).Lsh(bigOne, 63), new(big.Int).Sub(bigTwo128, bigOne),
		new(big.Int).Lsh(bigOne, 127), new(big.Int).Add(new(big.Int).Lsh(bigOne, 127), bigOne),
		new(big.Int).Sub(new(big.Int).Lsh(bigOne, 127), bigOne),
	} {
		ds = append(ds, d)
	}
	random := 2000
	if testing.Short() {
		random /= 10
	}
	for range random {
		d := new(big.Int).SetUint64(rng.Uint64())
		d.Lsh(d, 64).Or(d, new(big.Int).SetUint64(rng.Uint64()))
		ds = append(ds, d.Rsh(d, uint(rng.IntN(128))))
	}
	q, r := new(big.Int), new(big.Int)
	for _, d := range ds {
		if d.Sign() == 0 {
			continue
		}
		top := new(big.Int).Lsh(d, 128) // n < d × 2^128
		ns := []*big.Int{
			big.NewInt(0), big.NewInt(1), new(big.Int).Sub(d, bigOne), d,
			new(big.Int).Sub(top, bigOne), new(big.Int).Sub(top, d), new(big.Int).Sub(top, new(big.Int).Add(d, bigOne)),
		}
		for _, j := range []*big.Int{bigTwo64, new(big.Int).Sub(bigTwo64, bigOne), bigPow10(34), new(big.Int).Sub(bigTwo128, bigOne)} {
			p := new(big.Int).Mul(j, d)
			ns = append(ns, new(big.Int).Sub(p, bigOne), p, new(big.Int).Add(p, bigOne))
		}
		for range 50 {
			n := new(big.Int)
			for i := range 4 {
				n.Or(n, new(big.Int).Lsh(new(big.Int).SetUint64(rng.Uint64()), uint(64*i)))
			}
			ns = append(ns, n.Mod(n.Rsh(n, uint(rng.IntN(256))), top))
		}
		dd := from128(d)
		for _, n := range ns {
			if n.Sign() < 0 || n.Cmp(top) >= 0 {
				continue
			}
			q.QuoRem(n, d, r)
			nn := from256(n)
			what := fmt.Sprintf("%v / %v", n, d)
			gq, gr, ok := bid.U256Div128(nn, dd)
			if !ok {
				t.Fatalf("U256Div128(%s): not ok", what)
			}
			eq128(t, "U256Div128("+what+") q", gq, q)
			eq128(t, "U256Div128("+what+") r", gr, r)
			oq, or := oldDiv128(nn, dd)
			eq128(t, "old division "+what+" q", oq, q)
			eq128(t, "old division "+what+" r", or, r)
		}
	}
}
