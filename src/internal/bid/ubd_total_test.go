// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math/big"
	"math/rand/v2"
	"slices"
	"testing"
)

// ubdTotalPool returns UBD patterns rich in the cases of totalOrder: zeros
// and cohorts at several exponents, values near one another, infinities,
// and quiet and signaling NaNs of both signs with several payloads, each
// with its BID encoding.
func ubdTotalPool(r *rand.Rand) (ubd []bid.UBD128, bids []bid.Uint128) {
	add := func(b bid.Uint128) {
		w := bid.UBDFromBID128(b)
		ubd = append(ubd, bid.UBD128{Lo: w.Lo, Hi: w.Hi})
		bids = append(bids, b)
	}
	pack := func(neg bool, c uint64, exp int) {
		if b, ok := bid.Pack128(neg, bid.Uint128{Lo: c}, exp); ok {
			add(b)
		}
	}
	for _, neg := range []bool{false, true} {
		for _, e := range []int{bid.MinExp128, -3, -2, -1, 0, 1, bid.MaxExp128} {
			pack(neg, 0, e)
		}
		// Cohorts of 1 and 1.5, and their neighbors.
		for _, e := range []int{-4, -3, -2, -1, 0} {
			pack(neg, pow10u(-e), e)
			if e < 0 {
				pack(neg, 15*pow10u(-e-1), e)
			}
		}
		pack(neg, 9, -1)
		pack(neg, 11, -1)
		pack(neg, 1, bid.MinExp128)
		pack(neg, 1, bid.MaxExp128)
		sign := uint64(0)
		if neg {
			sign = bid.SignMask64
		}
		add(bid.Uint128{Hi: sign | bid.Inf128Hi})
		for _, p := range []uint64{0, 1, 2, 42, 1 << 40} {
			add(bid.Uint128{Hi: sign | bid.NaN128Hi, Lo: p})
			add(bid.Uint128{Hi: sign | bid.NaN128Hi | bid.SignalingBit64, Lo: p})
		}
		// A payload with high-word bits: the top of a 10^33 - 1 payload.
		add(bid.Uint128{Hi: sign | bid.NaN128Hi | 0x0000314dc6448d93, Lo: 0x38c15b09ffffffff})
		add(bid.Uint128{Hi: sign | bid.NaN128Hi | bid.SignalingBit64 | 1, Lo: 0})
	}
	for range 300 {
		pack(r.IntN(2) == 0, r.Uint64()>>uint(r.IntN(60)), -10+r.IntN(20))
	}
	return ubd, bids
}

func pow10u(n int) uint64 {
	p := uint64(1)
	for range n {
		p *= 10
	}
	return p
}

// TestUBDTotalOrder checks UBDTotalCompare128, UBDTotalOrder128 and
// UBDTotalOrderMag128 (against UBDTotalOrder128 of the magnitudes) on every
// pair of a pool:
//
//   - against the BID TotalOrder128 for every pair without a NaN, where its
//     order is the clause's (its NaN order is go-decimal-proposal#39);
//   - against the clause's rules for every pair with a NaN, from the
//     sign, the signaling bit and the payload of the BID encoding;
//   - for being a total order: 0 exactly for the same pattern,
//     antisymmetric, and transitive, which sorting the pool and checking
//     every pair of the sorted pool against its positions shows.
func TestUBDTotalOrder(t *testing.T) {
	r := rand.New(rand.NewPCG(38, 39))
	ubd, bids := ubdTotalPool(r)
	for i, x := range ubd {
		for j, y := range ubd {
			c := bid.UBDTotalCompare128(x, y)
			if got := bid.UBDTotalOrder128(x, y); got != (c <= 0) {
				t.Fatalf("UBDTotalOrder128(%#x, %#x) = %v, but the compare is %d", x, y, got, c)
			}
			if (c == 0) != (x == y) {
				t.Errorf("UBDTotalCompare128(%#x, %#x) = 0 is %v, but the patterns are the same is %v", x, y, c == 0, x == y)
			}
			ax, ay := bid.UBD128{Lo: x.Lo, Hi: x.Hi &^ bid.UBDSignHi}, bid.UBD128{Lo: y.Lo, Hi: y.Hi &^ bid.UBDSignHi}
			if got, want := bid.UBDTotalOrderMag128(x, y), bid.UBDTotalOrder128(ax, ay); got != want {
				t.Errorf("UBDTotalOrderMag128(%#x, %#x) = %v, but totalOrder of the magnitudes is %v", x, y, got, want)
			}
			if d := bid.UBDTotalCompare128(y, x); d != -c {
				t.Errorf("UBDTotalCompare128 not antisymmetric on %#x, %#x: %d and %d", x, y, c, d)
			}
			bx, by := bids[i], bids[j]
			var want int
			if bid.IsNaN128(bx) || bid.IsNaN128(by) {
				want = nanTotalCompare(bx, by)
			} else {
				want = -1
				if !bid.TotalOrder128(bx, by) {
					want = 1
				} else if bid.TotalOrder128(by, bx) {
					want = 0
				}
			}
			if c != want {
				t.Errorf("UBDTotalCompare128(%#x, %#x) = %d, want %d (BID %#x, %#x)", x, y, c, want, bx, by)
			}
		}
	}

	sorted := slices.Clone(ubd)
	slices.SortFunc(sorted, bid.UBDTotalCompare128)
	for i := range sorted {
		for j := range sorted {
			if got, want := bid.UBDTotalCompare128(sorted[i], sorted[j]), cmpPos(i, j); got != want && sorted[i] != sorted[j] {
				t.Fatalf("sorted pool: compare of positions %d and %d is %d, want %d", i, j, got, want)
			}
		}
	}
}

func cmpPos(i, j int) int {
	switch {
	case i < j:
		return -1
	case i > j:
		return 1
	}
	return 0
}

// nanTotalCompare is the order IEEE 754-2019 clause 5.10 d) gives a pair of
// BID encodings with a NaN among them, the payload order being this
// package's choice for d)5)iii): ascending for +NaN, descending for -NaN.
func nanTotalCompare(x, y bid.Uint128) int {
	key := func(b bid.Uint128) (neg bool, rank int, payload *big.Int) {
		neg = b.Hi&bid.SignMask64 != 0
		switch {
		case !bid.IsNaN128(b):
			return neg, 0, nil // a number: below every NaN of its sign's side
		case bid.IsSignaling128(b):
			rank = 1
		default:
			rank = 2
		}
		p := new(big.Int).SetUint64(b.Hi & (1<<46 - 1))
		p.Lsh(p, 64).Or(p, new(big.Int).SetUint64(b.Lo))
		return neg, rank, p
	}
	xn, xr, xp := key(x)
	yn, yr, yp := key(y)
	if xn != yn {
		if xn {
			return -1
		}
		return 1
	}
	var c int
	switch {
	case xr != yr:
		c = cmpPos(xr, yr)
	default:
		c = xp.Cmp(yp)
	}
	if xn {
		return -c
	}
	return c
}
