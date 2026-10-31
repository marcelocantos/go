// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math/big"
	"testing"
)

// The audit reproducers: products whose correct rounding depends on every
// dropped digit, and on an exponent far outside the format.
func TestMulAuditReproducers(t *testing.T) {
	// The exact product is 6145114010015392352999999999999938|548859899846076470
	// × 10^-9: the 34 digits before the bar end …938 and the 18 below it are
	// just above half, so to nearest it rounds up to …939. A rounding that
	// keeps only the last dropped digit as the sticky bit (audit R7) sees a
	// zero there and calls the tail an exact tie or less, giving …938.
	x, _ := bid.Pack128(false, toUint128(mustBig("99999999999999999999999999999999")), 1)
	y, _ := bid.Pack128(true, toUint128(mustBig("61451140100153923530")), -10)
	lo, _ := bid.Pack128(true, toUint128(mustBig("6145114010015392352999999999999938")), 9)
	hi, _ := bid.Pack128(true, toUint128(mustBig("6145114010015392352999999999999939")), 9)
	for _, tc := range []struct {
		mode bid.Mode
		want bid.Uint128
	}{
		{bid.ToNearestEven, hi},
		{bid.ToNearestAway, hi},
		{bid.TowardZero, lo},
		{bid.TowardPositive, lo},
		{bid.TowardNegative, hi},
	} {
		if got := bid.Mul128(x, y, tc.mode); got != tc.want {
			t.Errorf("Mul128(99999999999999999999999999999999E1, -61451140100153923530E-10, %v) = %s, want %s",
				tc.mode, display(dec128, got), display(dec128, tc.want))
		}
	}

	// 1E+6144 × 10 overflows decimal128: the product has coefficient 10^34
	// at exponent 6111, one place above the largest finite value.
	a, _ := bid.Pack128(false, toUint128(mustBig("1000000000000000000000000000000000")), 6111)
	ten, _ := bid.Pack128(false, bid.Uint128{Lo: 10}, 0)
	for _, tc := range []struct {
		mode bid.Mode
		inf  bool
	}{
		{bid.ToNearestEven, true}, {bid.ToNearestAway, true}, {bid.TowardPositive, true},
		{bid.TowardZero, false}, {bid.TowardNegative, false},
	} {
		got := bid.Mul128(a, ten, tc.mode)
		if tc.inf && got != (bid.Uint128{Hi: bid.Inf128Hi}) {
			t.Errorf("Mul128(1E+6144, 10, %v) = %s, want Infinity", tc.mode, display(dec128, got))
		}
		if !tc.inf && !bid.IsFinite128(got) {
			t.Errorf("Mul128(1E+6144, 10, %v) = %s, want the largest finite value", tc.mode, display(dec128, got))
		}
	}
}

func mustBig(s string) *big.Int {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic(s)
	}
	return v
}

// FuzzMul checks Mul64 and Mul128 against the reference model in every
// mode: on operands built from a coefficient and an exponent over the whole
// range of the format, so that the products reach the overflow, fold-down
// and subnormal edges, and on arbitrary bit patterns, which reach the
// specials and the non-canonical encodings.
func FuzzMul(f *testing.F) {
	for _, wide := range []bool{false, true} {
		for mode := range uint8(5) {
			f.Add(wide, mode, uint8(0), false, true, uint64(0), uint64(1)<<62, uint64(0), uint64(7), 0, 0)
			f.Add(wide, mode, uint8(0), true, true, uint64(1)<<30, uint64(1)<<60, uint64(0), uint64(3)<<62, -398, 369)
			f.Add(wide, mode, uint8(0), false, false, uint64(0x1ED09BEAD87C0), uint64(0x378D8E63FFFFFFFF), uint64(0), uint64(10), 6111, 6144)
			f.Add(wide, mode, uint8(1), false, false, uint64(0x7800)<<48, uint64(0), uint64(0), uint64(0), 0, 0)
			f.Add(wide, mode, uint8(1), false, false, uint64(0x7C00)<<48, uint64(0), uint64(0x7800)<<48, uint64(0), 0, 0)
		}
	}
	f.Fuzz(func(t *testing.T, wide bool, mode, raw uint8, xneg, yneg bool, xhi, xlo, yhi, ylo uint64, xe, ye int) {
		fm := fuzzFormat(wide)
		m := bid.Mode(mode % 5)
		var x, y bits
		if raw&1 != 0 {
			x, y = rawBits(fm, xhi, xlo), rawBits(fm, yhi, ylo)
		} else {
			x = fuzzOperand(fm, xneg, xhi, xlo, xe)
			y = fuzzOperand(fm, yneg, yhi, ylo, ye)
		}
		want := reference{}.mul(fm, x, y, m)
		got := kernel{}.mul(fm, x, y, m)
		if got != want {
			t.Fatalf("%s Mul(%s [%016x%016x], %s [%016x%016x], %v) = %s, want %s", fm.name,
				display(fm, x), x.Hi, x.Lo, display(fm, y), y.Hi, y.Lo, m, display(fm, got), display(fm, want))
		}
	})
}

// fuzzOperand encodes the finite value with a coefficient reduced into the
// format and an exponent clamped into it.
func fuzzOperand(fm *format, neg bool, hi, lo uint64, exp int) bits {
	c := new(big.Int).Lsh(new(big.Int).SetUint64(hi), 64)
	c.Or(c, new(big.Int).SetUint64(lo))
	c.Mod(c, new(big.Int).Add(fm.maxCoeff, bigOne))
	return fm.encode(value{kind: finite, neg: neg, coeff: c, exp: clamp(exp, fm.minExp, fm.maxExp)})
}
