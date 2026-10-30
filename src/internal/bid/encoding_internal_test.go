// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math"
	"math/big"
	"testing"
	"time"
)

func bigOf(c bid.Uint128) *big.Int {
	n := new(big.Int).SetUint64(c.Hi)
	n.Lsh(n, 64)
	return n.Or(n, new(big.Int).SetUint64(c.Lo))
}

// u128GePow10 compares with 10^n for every n, including the n for which
// 10^n does not fit a Uint128: then no Uint128 is at least 10^n.
func TestU128GePow10(t *testing.T) {
	cases := []bid.Uint128{
		{}, {Lo: 1}, {Lo: 9}, {Lo: 10}, {Lo: math.MaxUint64}, {Hi: 1},
		{Hi: 0x1ED09BEAD87C0, Lo: 0x378D8E63FFFFFFFF},
		{Hi: math.MaxUint64 >> 1, Lo: math.MaxUint64}, {Hi: 1 << 63}, {Hi: math.MaxUint64, Lo: math.MaxUint64},
	}
	for n := 0; n <= 45; n++ {
		p := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
		for _, c := range cases {
			if got, want := bid.U128GePow10(c, n), bigOf(c).Cmp(p) >= 0; got != want {
				t.Errorf("u128GePow10(%#x:%#x, %d) = %t, want %t", c.Hi, c.Lo, n, got, want)
			}
		}
	}
}

// digits128 counts the decimal digits of every Uint128, the largest
// included (it has 39 digits). It must also terminate on them.
func TestDigits128(t *testing.T) {
	check := func(c bid.Uint128) {
		t.Helper()
		done := make(chan int, 1)
		go func() { done <- bid.Digits128(c) }()
		select {
		case got := <-done:
			if want := len(bigOf(c).String()); got != want {
				t.Errorf("digits128(%#x:%#x) = %d, want %d", c.Hi, c.Lo, got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("digits128(%#x:%#x) does not terminate", c.Hi, c.Lo)
		}
	}
	for n := 0; n <= 38; n++ {
		p := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
		for _, d := range []int64{-1, 0, 1} {
			v := new(big.Int).Add(p, big.NewInt(d))
			if v.Sign() <= 0 || v.BitLen() > 128 {
				continue
			}
			c := bid.Uint128{
				Hi: new(big.Int).Rsh(v, 64).Uint64(),
				Lo: new(big.Int).And(v, new(big.Int).SetUint64(math.MaxUint64)).Uint64(),
			}
			if c.Hi == 0 && c.Lo == 0 {
				continue
			}
			check(c)
		}
	}
	check(bid.Uint128{Hi: math.MaxUint64, Lo: math.MaxUint64})
}
