// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import (
	"math/big"
	"math/bits"
	"math/rand/v2"
	"testing"
)

// TestLog10Pow2 checks the exponent estimate against math/big for every n
// the conversions can produce and beyond: 10^t <= 2^n < 10^(t+1).
func TestLog10Pow2(t *testing.T) {
	for n := -2135; n <= 2135; n++ {
		var want int
		if n >= 0 {
			want = len(new(big.Int).Lsh(big.NewInt(1), uint(n)).String()) - 1
		} else {
			// 2^n = 1 / 2^-n; with d the digits of 2^-n, 10^(d-1) < 2^-n < 10^d
			// (a power of two is not a power of ten), so 10^-d < 2^n < 10^(1-d).
			want = -len(new(big.Int).Lsh(big.NewInt(1), uint(-n)).String())
		}
		if got := log10Pow2(n); got != want {
			t.Errorf("log10Pow2(%d) = %d, want %d", n, got, want)
		}
	}
}

func bigOfNat(x *floatNat) *big.Int {
	n := new(big.Int)
	for i := x.n - 1; i >= 0; i-- {
		n.Lsh(n, 64)
		n.Or(n, new(big.Int).SetUint64(x.w[i]))
	}
	for i := x.n; i < len(x.w); i++ {
		if x.w[i] != 0 {
			panic("floatNat has a non-zero word above n")
		}
	}
	return n
}

// TestFloatNat checks the multiword operations against math/big on random
// significands and the extreme exponents: m × 2^e for e in [0, 971],
// m × 5^s for s in [0, 358], the right shift of the latter into 128 bits,
// and the division of the former by a power of ten with its sticky bit.
func TestFloatNat(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	five := big.NewInt(5)
	for i := 0; i < 20000; i++ {
		m := r.Uint64N(1<<53-1) + 1
		if i%3 == 0 {
			m = 1 << r.UintN(53)
		}
		bm := new(big.Int).SetUint64(m)

		e := r.IntN(972)
		if i < 3 {
			e = []int{0, 971, 1023 - bits.Len64(m) + 1}[i]
		}
		if bits.Len64(m)+e > 1024 {
			e = 1024 - bits.Len64(m)
		}
		var x floatNat
		x.setShl(m, e)
		want := new(big.Int).Lsh(bm, uint(e))
		if got := bigOfNat(&x); got.Cmp(want) != 0 {
			t.Fatalf("setShl(%d, %d) = %v, want %v", m, e, got, want)
		}
		k := r.IntN(310)
		p := new(big.Int).Exp(five, big.NewInt(int64(k)), nil)
		q, rem := new(big.Int).QuoRem(want, p, new(big.Int))
		sticky := x.divPow5(k)
		if got := bigOfNat(&x); got.Cmp(q) != 0 || sticky != (rem.Sign() != 0) {
			t.Fatalf("divPow5(%v, %d) = %v sticky %v, want %v sticky %v", want, k, got, sticky, q, rem.Sign() != 0)
		}

		s := r.IntN(359)
		var y floatNat
		y.setMulPow5(m, s)
		want = new(big.Int).Mul(bm, new(big.Int).Exp(five, big.NewInt(int64(s)), nil))
		if got := bigOfNat(&y); got.Cmp(want) != 0 {
			t.Fatalf("setMulPow5(%d, %d) = %v, want %v", m, s, got, want)
		}
		// A shift that leaves at most 120 bits, as the conversion's does.
		sh := max(want.BitLen()-120+r.IntN(8), 0)
		q = new(big.Int).Rsh(want, uint(sh))
		if q.BitLen() > 128 {
			continue
		}
		if got := bigOf(y.shr128(sh)); got.Cmp(q) != 0 {
			t.Fatalf("shr128(%v, %d) = %v, want %v", want, sh, got, q)
		}
	}
}

// TestPow5 checks 5^k for every k the multiword operations use.
func TestPow5(t *testing.T) {
	for k := 0; k <= pow5Step; k++ {
		want := new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(k)), nil)
		if got := pow5(k); got != want.Uint64() {
			t.Errorf("pow5(%d) = %d, want %v", k, got, want)
		}
	}
	if new(big.Int).Exp(big.NewInt(5), big.NewInt(pow5Step+1), nil).IsUint64() {
		t.Errorf("5^%d fits a word; pow5Step is not the largest", pow5Step+1)
	}
}

// TestFloatNatOverflowPanics checks that a quotient too large for 128 bits
// panics rather than truncating.
func TestFloatNatOverflowPanics(t *testing.T) {
	var x floatNat
	x.setMulPow5(1<<53-1, 100)
	for _, fn := range []func(){
		func() { x.shr128(10) },
		func() { x.lo128() },
	} {
		func() {
			defer func() {
				if recover() != errQuotientOverflow {
					t.Error("a 128-bit overflow did not panic with errQuotientOverflow")
				}
			}()
			fn()
		}()
	}
}
