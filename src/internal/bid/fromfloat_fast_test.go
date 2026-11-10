// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math"
	"math/big"
	mbits "math/bits"
	"math/rand/v2"
	"testing"
)

// Tests of the binary-to-decimal fast path (fromfloat_fast.go) against
// math/big: its tables, every answer it gives, and that it refuses often
// enough at decimal128 that the conversions which fall back to the exact
// path are exercised, and agree with the oracle, too.

// TestPow5Approx recomputes pow5MulTab and pow5DivTab and checks pow5Len
// over both.
func TestPow5Approx(t *testing.T) {
	five := big.NewInt(5)
	for s := range max(len(bid.Pow5MulTab), len(bid.Pow5DivTab)) {
		p := new(big.Int).Exp(five, big.NewInt(int64(s)), nil)
		if got := bid.Pow5Len(s); got != p.BitLen() {
			t.Errorf("pow5Len(%d) = %d, want %d", s, got, p.BitLen())
		}
		if s < len(bid.Pow5MulTab) {
			want := new(big.Int).Set(p)
			if a := p.BitLen() - 128; a > 0 {
				want.Rsh(want, uint(a))
			}
			if got := bigOf(bid.Pow5MulTab[s]); got.Cmp(want) != 0 {
				t.Errorf("pow5MulTab[%d] = %#x, want %#x", s, got, want)
			}
		}
		if s >= 1 && s < len(bid.Pow5DivTab) {
			n := new(big.Int).Lsh(big.NewInt(1), uint(p.BitLen()+127))
			want, r := new(big.Int).QuoRem(n, p, new(big.Int))
			if r.Sign() != 0 {
				want.Add(want, big.NewInt(1))
			}
			if got := bigOf(bid.Pow5DivTab[s]); got.Cmp(want) != 0 {
				t.Errorf("pow5DivTab[%d] = %#x, want %#x", s, got, want)
			}
		}
	}
}

// fromFloatFastCase is one call of the fast path that a conversion makes:
// the quotient of the significand m with the binary exponent e at the
// precision prec, with the arguments fromFloatW passes.
func fromFloatFastCheck(t *testing.T, prec int, m uint64, e int) (refused, called bool) {
	t.Helper()
	tz := mbits.TrailingZeros64(m)
	m >>= tz
	e += tz
	tt := bid.Log10Pow2(mbits.Len64(m) + e - 1)
	s := prec - tt
	bm := new(big.Int).SetUint64(m)
	switch {
	case e >= 0 && mbits.Len64(m)+e <= 128:
		return false, false
	case e >= 0:
		k := -s
		q, sticky, ok := bid.DivPow5Fast(m, e-k, k)
		if !ok {
			return true, true
		}
		n := new(big.Int).Lsh(bm, uint(e-k))
		wq, wr := new(big.Int).QuoRem(n, new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(k)), nil), new(big.Int))
		if bigOf(q).Cmp(wq) != 0 || sticky != (wr.Sign() != 0) {
			t.Fatalf("divPow5Fast(%d, %d, %d) = %#x, %v, want %#x, %v", m, e-k, k, bigOf(q), sticky, wq, wr.Sign() != 0)
		}
		return false, true
	case -e <= s:
		return false, false
	}
	r := -e - s
	q, ok := bid.MulPow5Fast(m, s, r)
	if !ok {
		return true, true
	}
	n := new(big.Int).Mul(bm, new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(s)), nil))
	n.Rsh(n, uint(r))
	if bigOf(q).Cmp(n) != 0 {
		t.Fatalf("mulPow5Fast(%d, %d, %d) = %#x, want %#x", m, s, r, bigOf(q), n)
	}
	return false, true
}

// TestFromFloatFast checks every answer the fast path gives for every
// binary exponent with random significands, and the extreme significands
// 1 and 2^53 - 1, at both precisions; then converts, with the kernel and the
// oracle in every mode, values for which the fast path refused, so that the
// fallback inside the conversion is exercised.
func TestFromFloatFast(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 7))
	perExp := floatToCount(40)
	for _, f := range []*format{dec64, dec128} {
		var calls, refusals int
		var refused []float64
		for e := -1074; e <= 1023-52; e++ {
			ms := []uint64{1, 1<<53 - 1, 1<<52 + 1}
			for range perExp {
				ms = append(ms, rng.Uint64N(1<<53)|1)
			}
			for _, m := range ms {
				r, c := fromFloatFastCheck(t, f.prec, m, e)
				if c {
					calls++
				}
				if r {
					refusals++
					if len(refused) < 2000 {
						refused = append(refused, math.Ldexp(float64(m), e))
					}
				}
			}
		}
		t.Logf("%s: %d fast-path calls, %d refused", f.name, calls, refusals)
		if f == dec128 && refusals == 0 {
			t.Errorf("%s: the fast path never refused, so the exact fallback was not exercised", f.name)
		}
		for _, v := range refused {
			for _, mode := range floatToModes {
				floatToCheck(t, f, v, mode)
				floatToCheck(t, f, -v, mode)
			}
		}
	}
}

// TestFromFloatFastConversions converts, at both widths, in every mode and
// with both signs, a significand of 1, of 2^53 - 1 and random ones at every
// binary exponent, and the ends of the subnormal and normal ranges,
// comparing each result with the math/big oracle. Ties at the destination
// precision, near ties and the specials are TestFloatToTies,
// TestFloatToNearTies and TestFloatToSpecials.
func TestFromFloatFastConversions(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 13))
	vs := []float64{math.SmallestNonzeroFloat64, math.Float64frombits(1<<52 - 1),
		math.Float64frombits(1 << 52), math.MaxFloat64, 1}
	for e := -1074; e <= 1023; e++ {
		vs = append(vs, math.Ldexp(1, e))
		if e <= 1023-52 {
			vs = append(vs, math.Ldexp(float64(1<<53-1), e))
			for range floatToCount(2) {
				vs = append(vs, math.Ldexp(float64(rng.Uint64N(1<<53)|1), e))
			}
		}
	}
	for _, f := range []*format{dec64, dec128} {
		for _, v := range vs {
			for _, mode := range floatToModes {
				floatToCheck(t, f, v, mode)
				floatToCheck(t, f, -v, mode)
			}
		}
	}
}
