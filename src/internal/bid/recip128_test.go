// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math/big"
	"math/rand"
	"testing"
)

// Tests of the reciprocal division by powers of ten (recip128.go) against
// math/big. The oracle's powers of ten and reciprocals come from big.Int,
// not from the package's tables.

// TestRecip128Table recomputes the reciprocal tables.
func TestRecip128Table(t *testing.T) {
	top := new(big.Int).Lsh(big.NewInt(1), 128)
	top.Sub(top, big.NewInt(1))
	if len(bid.RecipTrunc10tab128) != len(bid.Pow10tab128) {
		t.Fatalf("len(bid.RecipTrunc10tab128) = %d, want %d", len(bid.RecipTrunc10tab128), len(bid.Pow10tab128))
	}
	for k := range bid.RecipTrunc10tab128 {
		want := new(big.Int).Quo(top, bigPow10(k))
		if got := bigOf(bid.RecipTrunc10tab128[k]); got.Cmp(want) != 0 {
			t.Errorf("bid.RecipTrunc10tab128[%d] = %#x, want %#x", k, got, want)
		}
	}
}

// recip128Inputs returns, for the power 10^k, the values the reciprocal
// division is most likely to get wrong: each side of multiples j·10^k for
// small, random and extreme j, the ends of the type and of the coefficient
// range, and random values.
func recip128Inputs(k int, rng *rand.Rand, nrand int) []bid.Uint128 {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	p := bigPow10(k)
	jmax := new(big.Int).Quo(new(big.Int).Sub(limit, big.NewInt(1)), p)
	var out []bid.Uint128
	add := func(b *big.Int) {
		if b.Sign() >= 0 && b.Cmp(limit) < 0 {
			out = append(out, from128(b))
		}
	}
	around := func(b *big.Int) {
		for d := int64(-2); d <= 2; d++ {
			add(new(big.Int).Add(b, big.NewInt(d)))
		}
	}
	js := []*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(9), big.NewInt(10),
		jmax, new(big.Int).Sub(jmax, big.NewInt(1)), new(big.Int).Rsh(jmax, 1)}
	for range 64 {
		js = append(js, new(big.Int).Rand(rng, new(big.Int).Add(jmax, big.NewInt(1))))
	}
	for _, j := range js {
		m := new(big.Int).Mul(j, p)
		around(m)
		// The half point of each multiple, where the class changes.
		around(new(big.Int).Add(m, new(big.Int).Rsh(p, 1)))
	}
	around(new(big.Int).Sub(limit, big.NewInt(1)))
	around(new(big.Int).Lsh(big.NewInt(1), 64))
	around(bigPow10(34))
	around(bigPow10(38))
	for range nrand {
		add(new(big.Int).Rand(rng, limit))
	}
	return out
}

// TestRecip128 checks divPow10Recip128, cDivPow10128 and cShift128 at every
// k against math/big, including the class, which class128 computes from the
// remainder.
func TestRecip128(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	nrand := 1_000_000 / len(bid.Pow10tab128)
	if testing.Short() {
		nrand = 1000
	}
	for k := range bid.Pow10tab128 {
		p := bigPow10(k)
		half := new(big.Int).Rsh(p, 1)
		for _, c := range recip128Inputs(k, rng, nrand) {
			bq, br := new(big.Int).QuoRem(bigOf(c), p, new(big.Int))
			q, r := bid.DivPow10Recip128(c, k)
			if bigOf(q).Cmp(bq) != 0 || bigOf(r).Cmp(br) != 0 {
				t.Fatalf("bid.DivPow10Recip128(%#x, %d) = %#x, %#x, want %#x, %#x", bigOf(c), k, bigOf(q), bigOf(r), bq, br)
			}
			q2, r2 := bid.CDivPow10128(c, k)
			if q2 != q || r2 != r {
				t.Fatalf("bid.CDivPow10128(%#x, %d) = %#x, %#x, want %#x, %#x", bigOf(c), k, bigOf(q2), bigOf(r2), bq, br)
			}
			want := bid.ClassZero
			if br.Sign() != 0 {
				switch br.Cmp(half) {
				case -1:
					want = bid.ClassBelowHalf
				case 0:
					want = bid.ClassHalf
				default:
					want = bid.ClassAboveHalf
				}
			}
			sq, class := bid.CShift128(c, k)
			if sq != q || class != want {
				t.Fatalf("bid.CShift128(%#x, %d) = %#x, %v, want %#x, %v", bigOf(c), k, bigOf(sq), class, bq, want)
			}
		}
	}
}
