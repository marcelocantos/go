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
// As in divLong64 the dividend is biased by half the divisor, so that the
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

// divLong64 is Div64 for every operand (div64 above takes most operands
// by itself first): x / y rounded once in mode, with the special cases of
// "NaN and infinity operands" and the zero and infinity results of
// "Exponent of a result" and "Sign of a result" in the package
// documentation.
//
// A divisor coefficient 10^k needs no long division: the exact quotient is
// xc at exponent xe-ye-k. Strip at most k coefficient zeros to approach the
// preferred exponent xe-ye, then let round64 handle exponent bounds. This
// includes a coefficient of one without discarding any of x's quantum.
//
// The general finite case is long division in one step. With dx and dy the
// digit counts of the coefficients, the dividend is scaled by
//
//	k = Precision64 - dx + dy - lead
//
// places, where lead is 1 when the leading ratio xc × 10^(dy-dx) reaches
// yc and 0 otherwise, so that the integer quotient q = ⌊xc × 10^k / yc⌋
// has exactly Precision64 digits: xc × 10^k / yc lies in
// [10^(dx+k-dy-1), 10^(dx+k-dy+1)) and reaches 10^(dx+k-dy) exactly when
// the leading ratio does. The remainder is exact, and so is the class of
// the discarded fraction rem / yc: below a half, a half or above it as rem
// compares with yc - rem. The digit counts bound the widths: dx is at most
// Precision64 and dy at least 1, so k is between 0 and 2×Precision64-1, the
// scaled dividend has at most 2×Precision64 digits, 32, below 2^128, and
// the quotient fits a coefficient. cDivFull64 is the 128-by-64 division
// whose quotient is known to fit; the leading comparison
// multiplies a coefficient up to the other's digit count, at most
// Precision64 digits, which fits too.
//
// An inexact quotient inside the exponent range of roundFace64's proof
// (MinExp64 < exp <= MaxExp64, adjusted exponent at least Emin64) is rounded
// at face value: the one row of the rounding table, with the carry of the
// largest coefficient one place up. This is the second use of the
// face-value exception, after the swamped addend of addLong64 ("Face-value
// rounding" in the package documentation): it saves the reduction of a
// longer quotient by a reciprocal division and its digit count, which were
// a third of a division. In the two nearest modes the row is not even
// looked up: the dividend is biased by half the divisor and the quotient
// comes out rounded, with the exact quotient and the tie told apart by the
// biased remainder (the code below, and the same section). A directed mode
// takes the class of the remainder, rem against yc - rem. Nearer the exponent limits the
// result may be subnormal, clamped or overflowing, which only round64
// decides, so one more division step supplies a round digit and a sticky
// bit, and round64 rounds q × 10 + d at exponent xe - ye - k - 1 as it
// rounds every other inexact result with a discarded digit.
//
// The exponent of q × 10^-k is xe - ye - k, and for an inexact result that
// is what is rounded at: the result then takes the least exponent, as
// every inexact result does. An exact result (the remainder is zero) must
// instead take the exponent closest to the preferred one, xe - ye, among
// those at which it is representable, so the trailing zeros of q, which
// the scaling introduced, are stripped: at most k of them, so the exponent
// never passes xe - ye, and only when the remainder is zero, so that no
// non-zero fraction is lost. round64 then raises the exponent further only
// if the stripped q still has more than Precision64 digits, which is the
// cohort rule. The strip first tries the likely count of zeros by one exact
// division through the modular inverse of a power of ten (cDivExactPow1064,
// which tests divisibility without a remainder): all k of them for the
// long division's quotient, which has them whenever yc divides xc (8.00 /
// 2), and one for a dividend over a power of ten, whose quotient is x's
// own coefficient and as a rule has none. Only when that attempt leaves
// the count open does the binary search on the number of zeros run, as in
// key64, by the same exact divisions: q has at most Precision64+1 digits
// (Precision64, or one more after the round-digit step), so at most
// Precision64 trailing zeros, and the attempt and the steps stay within the
// smaller of k and Precision64+1, which keeps every power of ten within the
// coefficient table.
func divLong64(x, y coef64, mode Mode) coef64 {
	xneg, xc, xe, xok := unpackFirst64(x)
	xk := KindFinite
	if !xok {
		xk, xneg, xc, xe = unpackRest64(x)
	}
	yneg, yc, ye, yok := unpackFirst64(y)
	yk := KindFinite
	if !yok {
		yk, yneg, yc, ye = unpackRest64(y)
	}
	neg := xneg != yneg
	switch {
	case xk == KindNaN || yk == KindNaN:
		return nan64(x, y, y)
	case xk == KindInf:
		if yk == KindInf {
			// ∞ / ∞ is invalid (IEEE 754-2019 clause 7.2 item e).
			return cNaN64()
		}
		return cInf64(neg)
	case yk == KindInf:
		// x / ∞ is a zero at MinExp64 ("Exponent of a result").
		r, _ := round64(neg, wide64{}, MinExp64, classZero, mode, MinExp64)
		return r
	case cIsZero64(yc):
		if cIsZero64(xc) {
			// 0 / 0 is invalid (clause 7.2 item e).
			return cNaN64()
		}
		// Division by zero (clause 7.3).
		return cInf64(neg)
	case cIsZero64(xc):
		// A zero quotient takes the preferred exponent, clamped by round64.
		r, _ := round64(neg, wide64{}, xe-ye, classZero, mode, MinExp64)
		return r
	}

	dy := cDigits64(yc)
	k, q, class := dy-1, xc, classZero
	first := 1 // the likely count of zeros to strip from an exact quotient
	if !cEq64(yc, cPow1064(k)) {
		// Scale so that q has exactly Precision64 digits: xc × 10^k / yc
		// lies in [10^(dx+k-dy-1), 10^(dx+k-dy+1)) and reaches 10^(dx+k-dy)
		// exactly when the leading ratio does, xc × 10^(dy-dx) >= yc, a
		// comparison of two values of dy or dx digits, which fit.
		//
		// Which coefficient is the longer is a property of the operands'
		// kind and stays a branch; whether the ratio reaches one is in
		// their leading digits, as good as random, and is a borrow
		// (cLt64), not a branch.
		dx := cDigits64(xc)
		a, b := xc, yc
		if dx <= dy {
			a = cMulPow1064(xc, dy-dx)
		} else {
			b = cMulPow1064(yc, dx-dy)
		}
		lead := 1
		if cLt64(a, b) {
			lead = 0
		}
		k = Precision64 - dx + dy - lead
		n := cMulPow10Full64(xc, k)
		exp := xe - ye - k
		face := MinExp64 < exp && exp <= MaxExp64 && exp+Precision64-1 >= Emin64
		var rem coef64
		if face && (mode <= ToNearestAway || mode > TowardPositive) {
			// To nearest, inside the exponent range of roundFace64's proof:
			// the dividend is biased by half the divisor, and the quotient
			// of n + ⌊yc/2⌋ by yc is the quotient of n by yc rounded to
			// nearest with ties up, because the bias carries into the next
			// unit exactly when the fraction is a half or more. Its
			// remainder says the rest: it is ⌊yc/2⌋ exactly when yc divides
			// n, the exact quotient, which goes on to the preferred
			// exponent below; and it is zero with an even yc exactly on a
			// tie, where ties to even takes the even neighbour, the one
			// below. Nothing here waits for a comparison of the remainder
			// with the half or branches on the direction, which is as good
			// as random: the two tests are rare, and roundFace64, given no
			// class, only encodes.
			half := cHalf64(yc)
			nb, _ := cAddFull64(n, wide64{Lo: half})
			var rb coef64
			q, rb = cDivFull64(nb, yc)
			if !cEq64(rb, half) {
				if cIsZero64(rb) && !cOdd64(yc) && mode != ToNearestAway {
					q = cEven64(q)
				}
				return roundFace64(neg, q, exp, classZero, mode)
			}
		} else {
			q, rem = cDivFull64(n, yc)
		}
		if !cIsZero64(rem) {
			// An inexact quotient in a directed mode or near the exponent
			// limits: the remainder places the true value between q and
			// q + 1 at exponent xe - ye - k, and its class is exact: rem
			// against yc - rem. Within the exponent range that roundFace64's
			// proof names, that is the whole rounding decision, taken at
			// face value; nearer the limits round64 takes the usual round
			// digit and sticky bit, found by one more division step.
			other, _ := cSub64(yc, rem)
			class = cClass64(rem, other)
			if face {
				return roundFace64(neg, q, exp, class, mode)
			}
			d, rem2 := cDivFull64(wide64{Lo: cMulPow1064(rem, 1)}, yc)
			q, _ = cAdd64(cMulPow1064(q, 1), d)
			k++
			class = classZero
			if !cIsZero64(rem2) {
				class = classBelowHalf
			}
		} else if k < Precision64 {
			// The long division's exact quotient has all k zeros whenever
			// yc divides xc, the usual exact division (8.00 / 2): try them
			// at once. q has Precision64 digits, so k >= Precision64 cannot
			// strip whole; the search takes those.
			first = k
		} else {
			first = 0
		}
	}
	if class == classZero && k > 0 {
		// strip64's steps, written here and not called: as a call they
		// cost a decimal64 division with an exact quotient a sixth more,
		// and one that is not exact a twentieth, for what the call does
		// to this function's registers. The straight path in front of this
		// function, div64, which reaches an exact quotient more rarely,
		// calls strip64.
		search := true
		if first > 0 {
			if sq, ok := cDivExactPow1064(q, first); ok {
				q, k = sq, k-first
				search = first == 1
			} else {
				search = first > 1
			}
		}
		if search && k > 0 {
			step := 1
			for step*2 <= min(k, Precision64+1) {
				step *= 2
			}
			for ; step > 0; step /= 2 {
				if step > k {
					continue
				}
				if sq, ok := cDivExactPow1064(q, step); ok {
					q, k = sq, k-step
				}
			}
		}
	}
	r, _ := round64(neg, wide64{Lo: q}, xe-ye-k, class, mode, MinExp64)
	return r
}

// strip64 removes trailing zeros from the exact quotient q, which is k
// places below the preferred exponent, up to k of them, and returns the
// coefficient and the places that remain. k is positive.
//
// The first attempt is the likely count of zeros, first, by one exact
// division through the modular inverse of 10^first (cDivExactPow1064, which
// tests divisibility without a remainder and refuses by the low bits
// alone), and the binary search runs only when the attempt leaves the count
// open. A dividend over a power of ten keeps its own coefficient, which as
// a rule has no trailing zero: one place is tried, and a failure ends the
// strip. The long division's all-at-once attempt ends the strip on success.
func strip64(q coef64, k, first int) (coef64, int) {
	search := true
	if first > 0 {
		if sq, ok := cDivExactPow1064(q, first); ok {
			q, k = sq, k-first
			search = first == 1
		} else {
			search = first > 1
		}
	}
	if search && k > 0 {
		step := 1
		for step*2 <= min(k, Precision64+1) {
			step *= 2
		}
		for ; step > 0; step /= 2 {
			if step > k {
				continue
			}
			if sq, ok := cDivExactPow1064(q, step); ok {
				q, k = sq, k-step
			}
		}
	}
	return q, k
}
