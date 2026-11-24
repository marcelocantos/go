// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// mulSlow64 returns x × y rounded in mode. Finite canonical operands of
// either encoding form, in a mode that rounds to nearest, are multiplied
// here, and every other operand, and a result that this path does not
// encode, goes to mulLong64, which decides it from the start.
//
// The product of the coefficients is exact in two words and below 10^32.
// With 16 or fewer digits it is the result, at the sum of the exponents,
// as in mulLong64's word path. With more, the j digits beyond 16 go, as
// in mulSlow128: the product is biased by half of 10^j, so that the
// quotient is the product rounded half up, and a zero remainder is the
// tie, which takes the even neighbour by clearing the low bit. The
// quotient has 16 digits, or is 10^16 after a carry, and its exponent is
// the sum of the exponents and j, where round64 puts it too when that is
// within the format.
//
// One entry of prodTab64, chosen by the leading zeros of the product,
// holds everything the rounding needs: the power of ten that decides
// between the two digit counts the product's bit length allows, and for
// each of them 10^j and its reciprocal at a shift that the bit length
// fixes. The power and both reciprocals are read at once and the borrow
// of the product less the power chooses between them, so that the
// division does not wait for the digit count to finish before its operands
// are read. The whole path calls nothing, and branches on the kind of its
// operands, on the rare tie and carry, and in packSign64 on the encoding
// form of the result, which is the second for one product of random
// operands in thirty.
//
// What packSign64 refuses is handed on: a quotient that the rounding took
// to 10^16, and an exponent outside the range, where the result is
// subnormal, clamped or an overflow.
//
// The compiler calls this function directly, so its parameters and result
// are a contract with it: see the end of "Inline fast paths" in the package
// documentation.
func mulSlow64(x, y uint64, mode Mode) uint64 {
	xc, xe := fields64(x)
	yc, ye := fields64(y)
	// Operands that are not finite and canonical, and the directed modes.
	// These are branches on the kind of the operands, and they measured
	// faster than one test of the bits of all five (5 percent).
	if xc > MaxCoeff64 || yc > MaxCoeff64 || xe >= specialExp64 || ye >= specialExp64 || mode > ToNearestAway {
		return mulLong64(x, y, mode)
	}
	sign := (x ^ y) & SignMask64
	exp := xe + ye - 2*Bias64
	hi, lo := bits.Mul64(xc, yc)
	if hi == 0 && lo <= MaxCoeff64 {
		if r, ok := packSign64(sign, lo, exp); ok {
			return r
		}
		return mulLong64(x, y, mode)
	}

	// The leading zeros of the product, from 21 to 74: those of the low
	// word count only when the high word is all zeros.
	z := uint(bits.LeadingZeros64(hi))
	z += uint(bits.LeadingZeros64(lo)) & -(z >> 6)
	e := &prodTab64[z]
	_, bw := bits.Sub64(lo, e.pow.Lo, 0)
	_, bw = bits.Sub64(hi, e.pow.Hi, bw)
	d, m, d0, m0 := e.d1, e.m1, e.d0, e.m0
	if bw != 0 {
		d, m = d0, m0 // the product has t digits, not t + 1
	}
	// The estimate (see prodRound64), and the remainder of the biased
	// product against it, which is below 2 × 10^j and exact in the low
	// word; one comparison corrects an estimate that is one short.
	q, _ := bits.Mul64(lo>>(e.s&63)|hi<<(e.u&63), m)
	r := lo + d>>1 - q*d
	if r >= d {
		q, r = q+1, r-d
	}
	if r == 0 && mode == ToNearestEven {
		q &^= 1 // a tie: the even neighbour is the one below
	}
	if r, ok := packSign64(sign, q, exp+int(e.j)-int(bw)); ok {
		return r
	}
	return mulLong64(x, y, mode)
}

// prodRound64 is the rounding of a decimal64 product of n bits, from 54 to
// 107, to 16 digits: prodTab64 holds one for each count of leading zeros,
// 128 - n. A product of n bits has t or t + 1 digits, t = ⌊n × 1233 /
// 4096⌋, as cLenFull64 counts them, and pow is 10^t, which it reaches
// exactly when it has t + 1. With t + 1 digits it loses j digits, d1 =
// 10^j, and with t it loses j - 1, d0 = 10^(j-1) (zero when j - 1 is not
// positive: such a product has 16 digits and does not round). m1 and m0
// are their reciprocals at the shift s = max(n - 62, 0), ⌊2^(64+s) / d⌋.
// With s = 0 the product has no high word, and the high word's shift u,
// 64 - s modulo 64, is zero too.
//
// The quotient wanted is q = ⌊P / d⌋ for the biased product P = p + d/2,
// the product rounded half up. Its estimate is q̂ = ⌊a × m / 2^64⌋ with a
// = ⌊p / 2^s⌋, the method of recip.go on the product without its bias,
// with a shift that the bit length fixes rather than the divisor, so that
// the product is shifted before the digit count is known. q̂ is q or q - 1.
// It is at most ⌊p / d⌋ ≤ q, because a ≤ p / 2^s and m ≤ 2^(64+s) / d.
// And p / d - a × m / 2^64 is below (a + m + 1) / 2^64, by recip.go's
// argument, which is below a half, because a < 2^62 and m < 2^62
// (TestProdTab64 checks both, and the estimate itself, for every entry):
// so a × m / 2^64 > p / d - 1/2 and q̂ ≥ ⌊p / d - 1/2⌋ = q - 1. The
// remainder P - q̂ × d is then below 2d < 2^55.
type prodRound64 struct {
	pow    Uint128
	d1, m1 uint64
	d0, m0 uint64
	j, s   uint8
	u      uint8     // (64 - s) mod 64, the shift of the high word
	_      [13]uint8 // to 64 bytes, so that an index is one shift
}

