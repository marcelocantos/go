// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// Directed tests of Rem and Mod at exponent gaps the oracle's generator
// reaches only by chance: every gap of each format, ties at gaps beyond one
// stride of the modular reduction, where only the parity of a quotient far
// wider than any word decides the result, and divisors whose scaled value
// occupies the top bit of the word.

import (
	"fmt"
	"internal/bid"
	"math/big"
	"math/rand/v2"
	"testing"
)

// remFin encodes a finite operand.
func remFin(f *format, neg bool, c *big.Int, exp int) bits {
	return f.encode(value{kind: finite, neg: neg, coeff: c, exp: exp})
}

// checkRemMod compares both operations on x and y with the reference.
func checkRemMod(t *testing.T, f *format, x, y bits) {
	t.Helper()
	var ref reference
	var ker kernel
	if got, want := ker.rem(f, x, y), ref.rem(f, x, y); got != want {
		t.Errorf("Rem%d(%s, %s) = %s, reference %s", f.width, display(f, x), display(f, y), display(f, got), display(f, want))
	}
	if got, want := ker.mod(f, x, y), ref.mod(f, x, y); got != want {
		t.Errorf("Mod%d(%s, %s) = %s, reference %s", f.width, display(f, x), display(f, y), display(f, got), display(f, want))
	}
}

// TestRemModLiteral checks results written out by hand, without the
// reference model. Each is exact arithmetic on a small divisor: 10^k mod 7
// has period 6, 10^k mod 3 is 1, and the exponent is that of the divisor.
func TestRemModLiteral(t *testing.T) {
	const maxExp64, maxExp128 = "1E+369", "1E+6111"
	tests := []struct {
		width    int
		x, y     string
		rem, mod string
	}{
		// The cases of the acceptance criteria, in both widths.
		{64, "1E+17", "3", "1", "1"},
		{64, "1E+20", "3", "1", "1"},
		{64, "1E+30", "7", "1", "1"},
		{128, "1E+17", "3", "1", "1"},
		{128, "1E+20", "3", "1", "1"},
		{128, "1E+30", "7", "1", "1"},
		// The largest gap. 767 = 6×127 + 5 and 12287 = 6×2047 + 5, so
		// 10^gap mod 7 is 10^5 mod 7 = 5, and 2×5 > 7 rounds to -2.
		{64, maxExp64, "7E-398", "-2E-398", "5E-398"},
		{64, maxExp64, "3E-398", "1E-398", "1E-398"},
		{64, "-" + maxExp64, "7E-398", "2E-398", "-5E-398"},
		{128, maxExp128, "7E-6176", "-2E-6176", "5E-6176"},
		{128, maxExp128, "3E-6176", "1E-6176", "1E-6176"},
		{128, "-" + maxExp128, "-7E-6176", "2E-6176", "-5E-6176"},
		// A zero remainder at the largest gap has the sign of x.
		{64, "-" + maxExp64, "1E-398", "-0E-398", "-0E-398"},
		{64, maxExp64, "-1E-398", "0E-398", "0E-398"},
		{128, "-" + maxExp128, "2E-6176", "-0E-6176", "-0E-6176"},
		{128, maxExp128, "-5E-6176", "0E-6176", "0E-6176"},
		// A divisor far above x: n = 0, whatever the gap.
		{64, "1E-398", maxExp64, "1E-398", "1E-398"},
		{64, "-1E-398", maxExp64, "-1E-398", "-1E-398"},
		{128, "1E-6176", maxExp128, "1E-6176", "1E-6176"},
		{128, "-1E-6176", "-" + maxExp128, "-1E-6176", "-1E-6176"},
		// Ties beyond one stride: 3E+k / 2^(k+1) = 3 × 5^k / 2, whose
		// truncated quotient is odd because 3 × 5^k is 3 mod 4, and 1E+k /
		// 2^(k+1), whose truncated quotient is even.
		{64, "1E+20", "2097152", "1048576", "1048576"},
		{64, "3E+20", "2097152", "-1048576", "1048576"},
		{128, "1E+40", "2199023255552", "1099511627776", "1099511627776"},
		{128, "3E+40", "2199023255552", "-1099511627776", "1099511627776"},
	}
	var ref reference
	var ker kernel
	for _, tt := range tests {
		f := dec64
		if tt.width == 128 {
			f = dec128
		}
		x, _ := ref.parse(f, tt.x, bid.ToNearestEven)
		y, _ := ref.parse(f, tt.y, bid.ToNearestEven)
		if got := display(f, ker.rem(f, x, y)); got != tt.rem {
			t.Errorf("Rem%d(%s, %s) = %s, want %s", tt.width, tt.x, tt.y, got, tt.rem)
		}
		if got := display(f, ker.mod(f, x, y)); got != tt.mod {
			t.Errorf("Mod%d(%s, %s) = %s, want %s", tt.width, tt.x, tt.y, got, tt.mod)
		}
	}
}

// TestRemModEveryGap runs every exponent difference of each format, in both
// directions, against the reference: each stride boundary and each bit of the
// exponentiation is reached by some gap.
func TestRemModEveryGap(t *testing.T) {
	for _, f := range both {
		t.Run(fmt.Sprint(f.width), func(t *testing.T) {
			step := 1
			if testing.Short() {
				step = 37
			}
			g := &gen{f: f, r: rand.New(rand.NewPCG(*seedFlag, streamSeed("remgap"+f.name)))}
			for gap := 0; gap <= f.maxExp-f.minExp; gap += step {
				high := f.maxExp - g.r.IntN(f.maxExp-f.minExp-gap+1)
				low := high - gap
				xc, yc := g.randDigits(f.prec), g.randDigits(1+g.r.IntN(f.prec))
				checkRemMod(t, f, remFin(f, false, xc, high), remFin(f, true, yc, low))
				checkRemMod(t, f, remFin(f, true, xc, low), remFin(f, false, yc, high))
			}
		})
	}
}

// TestRemTieParity builds an exact tie at every gap at which one exists.
// X / Y is an odd multiple of one half only if 2^(k+1) divides Y, so a tie's
// gap k is below the bit length of the largest coefficient, which is beyond
// one stride of the reduction in both widths. The truncated quotient is
// c × 5^k >> 1, and its parity is computed here from that formula, not by the
// reference model.
func TestRemTieParity(t *testing.T) {
	var ker kernel
	for _, f := range both {
		for k := 0; ; k++ {
			Y := new(big.Int).Lsh(bigOne, uint(k+1))
			if Y.Cmp(f.maxCoeff) > 0 {
				break
			}
			half := new(big.Int).Rsh(Y, 1)
			for _, c := range []int64{1, 3, 5, 7, 9} {
				C := big.NewInt(c)
				x := remFin(f, false, C, k)
				y := remFin(f, false, Y, 0)
				q := new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(k)), nil)
				q.Mul(q, C).Rsh(q, 1)
				want := remFin(f, q.Bit(0) == 1, half, 0)
				if got := ker.rem(f, x, y); got != want {
					t.Errorf("Rem%d(%s, %s) = %s, want %s: a tie whose truncated quotient has odd = %d", f.width, display(f, x), display(f, y), display(f, got), display(f, want), q.Bit(0))
				}
				checkRemMod(t, f, x, y)
			}
		}
	}
}

// TestRemModScaledDivisor covers Q(x) < Q(y) with the scaled divisor near the
// top of the word, where twice it no longer fits, and just past the top, where
// the scaling reports overflow. The operands straddle |y| / 2 and |y|.
func TestRemModScaledDivisor(t *testing.T) {
	for _, f := range both {
		for k := 0; k <= f.prec+8; k++ {
			for _, yc := range []int64{1, 2, 5, 9, 17, 18, 19, 34, 99} {
				y := remFin(f, false, big.NewInt(yc), k)
				Y := new(big.Int).Mul(big.NewInt(yc), pow10(k))
				for _, base := range []*big.Int{f.maxCoeff, Y, new(big.Int).Rsh(Y, 1)} {
					for _, d := range []int64{-1, 0, 1} {
						c := new(big.Int).Add(base, big.NewInt(d))
						if c.Sign() < 0 || c.Cmp(f.maxCoeff) > 0 {
							continue
						}
						checkRemMod(t, f, remFin(f, false, c, 0), y)
						checkRemMod(t, f, remFin(f, true, c, 0), y)
					}
				}
			}
		}
	}
}

// TestRemModZeros fixes the sign and exponent of a zero result, which the
// reference and the contract agree on but a mistake in the rounding branch
// of Rem would disturb: r = 0 must never be replaced by Y.
func TestRemModZeros(t *testing.T) {
	var ker kernel
	for _, f := range both {
		for _, neg := range []bool{false, true} {
			x := remFin(f, neg, big.NewInt(6), 0)
			y := remFin(f, !neg, big.NewInt(3), 0)
			want := remFin(f, neg, bigZero, 0)
			if got := ker.rem(f, x, y); got != want {
				t.Errorf("Rem%d(%s, %s) = %s, want %s", f.width, display(f, x), display(f, y), display(f, got), display(f, want))
			}
			if got := ker.mod(f, x, y); got != want {
				t.Errorf("Mod%d(%s, %s) = %s, want %s", f.width, display(f, x), display(f, y), display(f, got), display(f, want))
			}
			// A zero x takes min(Q(x), Q(y)), at the largest gap either way.
			checkRemMod(t, f, remFin(f, neg, bigZero, f.maxExp), remFin(f, false, bigOne, f.minExp))
			checkRemMod(t, f, remFin(f, neg, bigZero, f.minExp), remFin(f, false, bigOne, f.maxExp))
		}
	}
}

// remFuzzOperands draws two operands from fuzz input, biased towards what
// the arithmetic finds hard rather than what a byte mutator finds easily.
// The shapes are the raw encodings as given, exponents over the whole range,
// the largest gaps with small divisors, exact ties, and exponents within a
// precision of one another.
func remFuzzOperands(f *format, xw, yw bits, a, b uint16, shape uint8) (x, y bits) {
	span := f.maxExp - f.minExp + 1
	limit := new(big.Int).Add(f.maxCoeff, bigOne)
	xc := new(big.Int).Mod(fromUint128(xw), limit)
	yc := new(big.Int).Mod(fromUint128(yw), limit)
	xneg, yneg := a&1 != 0, b&1 != 0
	switch shape % 5 {
	case 0:
		return xw, yw
	case 1:
		return remFin(f, xneg, xc, f.minExp+int(a)%span), remFin(f, yneg, yc, f.minExp+int(b)%span)
	case 2:
		yd := pow10(1 + int(b>>8)%f.prec)
		return remFin(f, xneg, xc, f.maxExp-int(a)%64), remFin(f, yneg, yc.Mod(yc, yd), f.minExp+int(b)%64)
	case 3:
		// X = m × c × 10^k and Y = m × 2^(k+1), with c odd, so that X / Y =
		// c × 5^k / 2 is an exact tie at gap k.
		k := int(a) % (limit.BitLen() - 1)
		two := new(big.Int).Lsh(bigOne, uint(k+1))
		m := new(big.Int).Quo(limit, two)
		m.Mod(new(big.Int).Set(yc), m).SetBit(m, 0, 1)
		Y := new(big.Int).Mul(two, m)
		if Y.Cmp(f.maxCoeff) > 0 {
			m.SetInt64(1)
			Y.Set(two)
		}
		c := new(big.Int).Quo(limit, m)
		c.Mod(xc, c).SetBit(c, 0, 1)
		X := new(big.Int).Mul(c, m)
		if X.Cmp(f.maxCoeff) > 0 {
			X.Set(m)
		}
		ye := f.minExp + int(b)%(span-k)
		return remFin(f, xneg, X, ye+k), remFin(f, yneg, Y, ye)
	}
	gap := int(a)%(2*f.prec+5) - f.prec - 2
	xe := f.minExp + f.prec + 2 + int(b)%(span-2*f.prec-4)
	return remFin(f, xneg, xc, xe), remFin(f, yneg, yc, xe-gap)
}

func FuzzRem64(f *testing.F) {
	f.Add(uint64(1), uint64(3), uint16(0), uint16(0), uint8(0))
	f.Add(uint64(9999999999999999), uint64(3), uint16(3), uint16(5), uint8(2))
	f.Add(uint64(3), uint64(1), uint16(20), uint16(40), uint8(3))
	f.Add(uint64(15), uint64(1), uint16(10), uint16(100), uint8(4))
	f.Fuzz(func(t *testing.T, xw, yw uint64, a, b uint16, shape uint8) {
		x, y := remFuzzOperands(dec64, lo(xw), lo(yw), a, b, shape)
		checkRemMod(t, dec64, x, y)
	})
}

func FuzzRem128(f *testing.F) {
	f.Add(uint64(0), uint64(1), uint64(0), uint64(3), uint16(0), uint16(0), uint8(0))
	f.Add(uint64(0x1ed09bead87c0), uint64(0x378d8e63ffffffff), uint64(0), uint64(7), uint16(3), uint16(5), uint8(2))
	f.Add(uint64(0), uint64(3), uint64(0), uint64(1), uint16(40), uint16(400), uint8(3))
	f.Add(uint64(0), uint64(15), uint64(0), uint64(1), uint16(10), uint16(100), uint8(4))
	f.Fuzz(func(t *testing.T, xh, xl, yh, yl uint64, a, b uint16, shape uint8) {
		x, y := remFuzzOperands(dec128, bits{Hi: xh, Lo: xl}, bits{Hi: yh, Lo: yl}, a, b, shape)
		checkRemMod(t, dec128, x, y)
	})
}
