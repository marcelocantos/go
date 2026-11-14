// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// The constants of div64.
const (
	// div64Norm is 10^(Precision64-1), the least coefficient of
	// Precision64 digits: a coefficient scaled by div64Scale is at or
	// above it exactly when it has Precision64 digits.
	div64Norm = 1e15

	// div64Short is the bound below which a divisor takes two word
	// divisions; a divisor at or above it takes the estimate.
	div64Short = 1e9

	// div64Slack is the error of the estimate, in units of the quotient,
	// that the correction allows for: more than twice the bound of 7 that
	// div64's comment proves.
	div64Slack = 16
)

// div64 is Div64. A finite dividend over a finite divisor, neither of them
// zero, in a mode that rounds to nearest, is divided here, and every other
// operand, and a result outside the range that packQuotient64 encodes, goes
// to divLong64, which decides it from the start.
//
// The quotient is divLong64's: the coefficient a scaled by 10^k, plus half
// the divisor b, over b, with k = Precision64 - da + db - lead so that it has
// exactly Precision64 digits (da and db are the digit counts and lead whether
// the leading ratio reaches one). Only how it is found differs.
//
// The digit counts and the leading ratio come from one table load and one
// product for each coefficient. div64Scale, indexed by the leading zeros,
// gives 10^s for s = 15 - t, where t is the digit count that the bit length
// suggests, da or da - 1; a × 10^s has 15 or 16 digits, and the borrow of it
// against 10^15 says which, and so da. Brought to 16 digits, the two
// coefficients compare as the leading ratio does. A divisor that is a power
// of ten comes to exactly 10^15 and divides by moving the point.
//
// The division is two words by one, and how it is made depends on the
// target and the divisor.
//
// Where the processor divides two words by one (amd64), or a word division
// is itself a routine (32-bit targets), it is bits.Div64 of a × 10^k plus
// the bias, once.
//
// Otherwise a divisor below 10^9, which the divisors of money are, takes
// two word divisions in decimal. The first divides a × 10^s, below 10^16,
// by b; its remainder r, below b, times 10^(k-s), plus the bias, is the
// second dividend, and q1 × 10^(k-s) + q2 is the quotient, with the second
// remainder its own. k - s is db - 1 + (a < 10^t) + (1 - lead), at most
// db + 1, so the second dividend is below 10^(2 db + 1), within a word for
// db up to 9. The first division needs neither digit count nor the leading
// ratio, which are found beside it: it starts after one table load and one
// product, where bits.Div64 is a routine of two divisions and loops of
// corrections that waits for all of them.
//
// A larger divisor takes an estimate in float64: (a / b) × 10^k + 0.5
// carries at most six roundings of 2^-53 each (a, b, the quotient, 10^k
// beyond 10^22, the product and the sum, the last two one when fused), on
// a value below 10^16, and 0.5 differs from ⌊b/2⌋ / b by at most 1/(2b),
// so truncated it is within 7 of the quotient. The remainder of the estimate, a × 10^k
// + b/2 - qe × b, is then exact in the low word alone, and with
// div64Slack divisors added it is positive and below 2^64; one word
// division of it by b gives the correction and the remainder. Nothing but
// the size of the correction depends on the floating point: a worse
// estimate would fail the bound, not round differently.
//
// As in div128 the dividend is biased by half the divisor, so that the
// quotient is rounded half up and the remainder says the rest: equal to
// the half exactly when the division is exact, when div64Exact strips the
// zeros that the scale put in, and zero under an even divisor exactly on a
// tie. No quotient of two coefficients rounds up to 10^16 (TestDivNearest),
// and at an exponent in range a quotient of 16 digits is normal, so one
// rounding is the whole result.
//
// The compiler calls this function directly, so its parameters and result
// are a contract with it: see the end of "Inline fast paths" in the package
// documentation.
func div64(x, y uint64, mode Mode) uint64 {
	a, b, exp, sign, first := divPair64(x, y)
	if !first || mode > ToNearestAway {
		// The second form, the special values and the directed modes. A
		// coefficient of the second form is one of 16 digits from 2^53
		// up, and is decoded again here; the rest are divLong64's.
		_, ca, xe, okx := unpackWordCanon64(x)
		_, cb, ye, oky := unpackWordCanon64(y)
		if !okx || !oky || mode > ToNearestAway {
			return divLong64(x, y, mode)
		}
		a, b, exp = ca, cb, xe-ye
	}
	if min(a, b) == 0 {
		// A zero dividend over a divisor that is not zero is the zero at
		// the preferred exponent, and a dividend over zero the infinity,
		// both with the sign of the quotient; 0 / 0 and a zero whose
		// exponent is outside the format are divLong64's.
		if b != 0 {
			if r, ok := packWord64(false, 0, 0, exp); ok {
				return r | sign
			}
		} else if a != 0 {
			return Inf64 | sign
		}
		return divLong64(x, y, mode)
	}

	sa, sb := &div64Scale[bits.LeadingZeros64(a)&63], &div64Scale[bits.LeadingZeros64(b)&63]
	as, bs := a*sa.p, b*sb.p
	_, lta := bits.Sub64(as, div64Norm, 0)
	_, ltb := bits.Sub64(bs, div64Norm, 0)
	b16 := bs + bs*9&-ltb // b × 10^(16-db)
	if b16 == div64Norm {
		return div64Exact(x, y, mode, a, Precision64-1-sb.s-int(ltb), 1, exp, sign)
	}
	_, lt := bits.Sub64(as+as*9&-lta, b16, 0) // 1 - lead
	k := Precision64 - 1 + sa.s - sb.s + int(lta) - int(ltb) + int(lt)

	half := b >> 1
	var q, rem uint64
	switch {
	case div64Native || bits.UintSize == 32:
		t := &pow10tab128[k&31]
		h, n0 := bits.Mul64(a, t.Lo)
		n0, c := bits.Add64(n0, half, 0)
		q, rem = bits.Div64(h+a*t.Hi+c, n0, b)
	case b < div64Short:
		q1 := as / b
		p := pow10word[(k-sa.s)&31]
		n := (as-q1*b)*p + half
		q2 := n / b
		q, rem = q1*p+q2, n-q2*b
	default:
		qe := div64Estimate(a, b, k)
		r := a*pow10tab128[k&31].Lo + half - qe*b + div64Slack*b
		d := r / b
		q, rem = qe+d-div64Slack, r-d*b
	}
	if rem == half {
		first := 0
		if k < Precision64 {
			first = k
		}
		return div64Exact(x, y, mode, q, k, first, exp, sign)
	}
	if rem == 0 && b&1 == 0 && mode != ToNearestAway {
		q &^= 1 // a tie: the even neighbour is the one below
	}
	if r, ok := packQuotient64(q, exp-k, sign); ok {
		return r
	}
	return divLong64(x, y, mode)
}

// div64Estimate returns (a / b) × 10^k + 0.5 in float64, truncated: for
// coefficients a and b and the k of div64, within 7 of ⌊(a × 10^k + ⌊b/2⌋)
// / b⌋ (see div64; TestDiv64Estimate checks the bound).
func div64Estimate(a, b uint64, k int) uint64 {
	return uint64(int64(float64(int64(a))/float64(int64(b))*pow10f[k&31] + 0.5))
}

// div64Exact is div64 for an exact quotient q, k places below the
// preferred exponent exp: strip64 removes up to k of its zeros, trying
// first of them at once, and the result is encoded, or left to divLong64
// outside the range. It is a function of its own so that div64 calls
// nothing on its way to an inexact result and keeps every value in a
// register.
func div64Exact(x, y uint64, mode Mode, q uint64, k, first, exp int, sign uint64) uint64 {
	if k > 0 {
		q, k = strip64(q, k, first)
	}
	if r, ok := packWord64(false, 0, q, exp-k); ok {
		return r | sign
	}
	return divLong64(x, y, mode)
}

// div64Scale[n] is 10^s and s for a coefficient with n leading zero bits,
// where s = 15 - min(⌊(64 - n) × 1233 / 4096⌋, 15): the bit length's
// estimate of the digit count, less at most one, capped at the 16 digits
// that a coefficient has at most. TestDiv64Scale checks it.
var div64Scale = [64]struct {
	p uint64
	s int
}{
	{1e0, 0}, {1e0, 0}, {1e0, 0}, {1e0, 0},
	{1e0, 0}, {1e0, 0}, {1e0, 0}, {1e0, 0},
	{1e0, 0}, {1e0, 0}, {1e0, 0}, {1e0, 0},
	{1e0, 0}, {1e0, 0}, {1e0, 0}, {1e1, 1},
	{1e1, 1}, {1e1, 1}, {1e2, 2}, {1e2, 2},
	{1e2, 2}, {1e3, 3}, {1e3, 3}, {1e3, 3},
	{1e3, 3}, {1e4, 4}, {1e4, 4}, {1e4, 4},
	{1e5, 5}, {1e5, 5}, {1e5, 5}, {1e6, 6},
	{1e6, 6}, {1e6, 6}, {1e6, 6}, {1e7, 7},
	{1e7, 7}, {1e7, 7}, {1e8, 8}, {1e8, 8},
	{1e8, 8}, {1e9, 9}, {1e9, 9}, {1e9, 9},
	{1e9, 9}, {1e10, 10}, {1e10, 10}, {1e10, 10},
	{1e11, 11}, {1e11, 11}, {1e11, 11}, {1e12, 12},
	{1e12, 12}, {1e12, 12}, {1e12, 12}, {1e13, 13},
	{1e13, 13}, {1e13, 13}, {1e14, 14}, {1e14, 14},
	{1e14, 14}, {1e15, 15}, {1e15, 15}, {1e15, 15},
}

// pow10f[k] is 10^k rounded to float64: exact up to 10^22.
var pow10f = [32]float64{
	1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9,
	1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18, 1e19,
	1e20, 1e21, 1e22, 1e23, 1e24, 1e25, 1e26, 1e27, 1e28, 1e29,
	1e30, 1e31,
}
