// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// Conversion of a binary floating-point value to decimal: the exact
// arithmetic behind fromFloatW (fromfloat_tmpl.go), which is Float64To64,
// Float64To128, Float32To64 and Float32To128 (float.go). This file is
// width-independent (rule 7 of "Width parameterisation" in the package
// documentation): it mentions no format constant. The template chooses the
// digit count and calls the rounding routine; the arithmetic that produces
// the digits is here.
//
// # The problem
//
// A finite binary value is v = m × 2^e with m an odd integer below 2^53
// (trailing zero bits of the significand are moved into e) and e in
// [-1074, 1023]. The contract asks for v rounded once, in the mode, from
// its exact value, with preferred exponent 0 ("Binary floating-point
// conversions"). The rounding routine roundW does the rounding, the
// exponent rule and the sign of zero once it is handed the exact value in
// parts: an integer q with more than PrecisionW digits and a sticky bit
// that is set exactly when v is not q × 10^exp, or, for an exact value, q
// and exp themselves with the sticky bit clear. So the conversion is the
// problem of producing q = ⌊v × 10^s⌋ for a chosen s, and knowing whether
// v × 10^s is an integer, both exactly.
//
// # Why exact integer arithmetic, and why it is cheap
//
// The exact decimal expansion of v is long: 2^-1074 has 751 significant
// digits, and 2^1023 has 308. A table of 128-bit approximations to the
// powers of ten, the usual tool of a Ryu-style printer, gives q with an
// error bound and cannot by itself say whether a remainder is zero, so it
// decides the sticky bit and a tie only up to that bound and needs an exact
// path behind it for the cases it cannot decide. The exact path is the
// whole method here, because the structure of v makes it cheap:
//
//   - For e < 0 and s chosen so that v × 10^s has PrecisionW+1 or
//     PrecisionW+2 digits, v × 10^s = m × 5^s / 2^r with r = -e - s. When
//     r <= 0 this is an integer below 10^(PrecisionW+2) < 2^120 and v
//     itself is the integer m × 5^(-e) at exponent e, computed in 128 bits.
//     When r > 0, m × 5^s is odd (m is odd and so is every power of five),
//     so v × 10^s is not an integer: the sticky bit is set by this fact
//     alone, with no arithmetic to get wrong. The quotient ⌊m × 5^s / 2^r⌋
//     is the product m × 5^s, at most 53 + 831 = 884 bits for s <= 358
//     (the 2^-1074 case at decimal128), shifted right by r. The product is
//     exact: a multiword integer multiplied by 5^27, the largest power of
//     five in a word, a word at a time, s/27 passes over at most 14 words.
//     There is no division on this path.
//   - For e >= 0, v is the integer m × 2^e of at most 1024 bits. If it
//     fits a Uint128 it is handed to roundW as it is, with exponent 0 and
//     the sticky bit clear; roundW reduces it exactly. Otherwise
//     v / 10^k = m × 2^(e-k) / 5^k, the same rational, so q = ⌊v / 10^k⌋
//     and whether 10^k divides v are the quotient and the remainder of the
//     multiword integer m × 2^(e-k) divided by 5^k: a division by 5^27, the
//     largest power of five in a word, one word at a time, k/27 passes over
//     at most 12 words, each remainder exact and the sticky bit their OR.
//     Because v is an integer, every digit roundW discards after that is
//     exact too.
//
// Both are bounded-stack computations on a [floatNatWords]uint64 that is
// never heap-allocated. Typical operands (|e| below 64 or so) multiply a
// one- or two-word integer once or twice, which is what a fixed-precision
// table lookup would have cost; the cost grows linearly with |e| and is
// largest at the ends of the binary range. The ns/op for both are in
// BenchmarkFloatTo.
//
// # Why the tie and sticky decisions are exact
//
// Every quantity above is an integer computed in full: no value is
// truncated or approximated before roundW sees it. The only estimate in
// the algorithm is the adjusted exponent of v, from log10Pow2, and that is
// exact too (see its comment); even if it were not, it would change which
// s is used and hence how many digits q has, never the value of q or the
// sticky bit, and roundW accepts any count of PrecisionW+1 or more. The
// template checks the count. The decisions are therefore:
//
//   - A tie at the destination precision is q ending in the digit 5 with
//     the sticky bit clear. The sticky bit is clear only on the paths that
//     deliver v as an exact integer times a power of ten (r <= 0 above, or
//     e >= 0 with a zero remainder), where q is v × 10^s itself.
//   - A near tie is q ending in 4 or 5 with the sticky bit set. The sticky
//     bit is set structurally (an odd numerator over a positive power of
//     two) or by a non-zero exact remainder, never by an error bound.
//   - Every directed mode asks the same two questions: is the value exact,
//     and if not, what is q. Both are answered exactly, so no mode needs
//     an exact fallback: there is nothing to fall back from.

// floatNatWords is the capacity of a floatNat in 64-bit words. The largest
// value a conversion builds is m × 2^e for the largest float64, below 2^1024,
// or m × 5^358 for the smallest, below 2^885.
const floatNatWords = 16

// floatNat is a bounded-precision natural number on the stack, enough for
// the exact scaled significand of any float64. The words are little-endian;
// n is the number in use and every word at or above n is zero, which holds
// for the zero value and is kept by every method. A value that would not
// fit panics by an index out of range, which is a bug in the caller's
// bound, not an input condition.
type floatNat struct {
	w [floatNatWords]uint64
	n int
}

// setShl sets x to m × 2^e. Precondition: x is zero, m != 0 and e >= 0.
func (x *floatNat) setShl(m uint64, e int) {
	wi, bi := e/64, uint(e%64)
	x.w[wi] = m << bi
	x.n = wi + 1
	if bi != 0 {
		if hi := m >> (64 - bi); hi != 0 {
			x.w[wi+1] = hi
			x.n = wi + 2
		}
	}
}

// mulWord sets x to x × f. Precondition: f != 0.
func (x *floatNat) mulWord(f uint64) {
	var carry uint64
	for i := 0; i < x.n; i++ {
		hi, lo := bits.Mul64(x.w[i], f)
		lo, c := bits.Add64(lo, carry, 0)
		x.w[i] = lo
		carry = hi + c
	}
	if carry != 0 {
		x.w[x.n] = carry
		x.n++
	}
}

// pow5Table is the largest k for which 5^k is read from the 64-bit table of
// powers of ten: 10^k = 5^k × 2^k, so 5^k is pow10tab64[k] >> k. pow5Step
// is the largest k for which 5^k fits a word: 5^27 < 2^64 < 5^28. The
// multiword operations take their factor or divisor pow5Step digits at a
// time.
const (
	pow5Table = len(pow10tab64) - 1
	pow5Step  = 27
)

// pow5 returns 5^k. Precondition: 0 <= k <= pow5Step.
func pow5(k int) uint64 {
	if k <= pow5Table {
		return pow10tab64[k] >> uint(k)
	}
	return pow10tab64[pow5Table] >> uint(pow5Table) * (pow10tab64[k-pow5Table] >> uint(k-pow5Table))
}

// setMulPow5 sets x to m × 5^s. Precondition: x is zero, m != 0 and s >= 0.
func (x *floatNat) setMulPow5(m uint64, s int) {
	x.w[0] = m
	x.n = 1
	for ; s > pow5Step; s -= pow5Step {
		x.mulWord(pow5(pow5Step))
	}
	if s > 0 {
		x.mulWord(pow5(s))
	}
}

// shr128 returns ⌊x / 2^r⌋, which must fit 128 bits; a quotient that does
// not panics rather than truncates. Precondition: r >= 0.
func (x *floatNat) shr128(r int) Uint128 {
	wi, bi := r/64, uint(r%64)
	var w [3]uint64
	for i := range w {
		if wi+i < x.n {
			w[i] = x.w[wi+i]
		}
	}
	if bi != 0 {
		w[0] = w[0]>>bi | w[1]<<(64-bi)
		w[1] = w[1]>>bi | w[2]<<(64-bi)
		w[2] >>= bi
	}
	if w[2] != 0 || x.n > wi+3 {
		panic(errQuotientOverflow)
	}
	return Uint128{Hi: w[1], Lo: w[0]}
}

// divPow5 sets x to ⌊x / 5^k⌋ and reports whether the remainder was
// non-zero: the sticky bit of the discarded part. The divisor is taken
// pow5Step at a time, the most that fit one word, and each pass is an exact
// long division of x by that word. Precondition: k >= 0.
func (x *floatNat) divPow5(k int) (sticky bool) {
	for k > 0 {
		step := min(k, pow5Step)
		d := pow5(step)
		var r uint64
		for i := x.n - 1; i >= 0; i-- {
			x.w[i], r = bits.Div64(r, x.w[i], d)
		}
		sticky = sticky || r != 0
		for x.n > 0 && x.w[x.n-1] == 0 {
			x.n--
		}
		k -= step
	}
	return sticky
}

// lo128 returns x, which must fit 128 bits; a value that does not panics
// rather than truncates.
func (x *floatNat) lo128() Uint128 {
	if x.n > 2 {
		panic(errQuotientOverflow)
	}
	return Uint128{Hi: x.w[1], Lo: x.w[0]}
}

// log10Of2Fixed is ⌊2^32 × log10 2⌋; log10Of2Fixed / 2^32 is below log10 2
// by less than 2^-32.
const log10Of2Fixed = 1292913986

// log10Pow2 returns ⌊n × log10 2⌋, the adjusted exponent of 2^n: the t for
// which 10^t <= 2^n < 10^(t+1). It is exact for |n| <= 2135, which covers
// every binary exponent of a float64 significand with room to spare.
//
// The fixed-point product n × log10Of2Fixed / 2^32 is within |n| × 2^-32 <
// 5 × 10^-7 of n × log10 2. The floor can differ only if some integer lies
// between the two, that is if n × log10 2 is within 5 × 10^-7 of an
// integer. The continued fraction of log10 2 has the convergents 3/10,
// 28/93, 59/196, 146/485 and 643/2136, and a convergent's denominator is
// the best approximation below the next denominator, so for 0 < |n| < 2136
// the distance from n × log10 2 to the nearest integer is at least
// |485 × log10 2 - 146| > 4.5 × 10^-4. The shift is arithmetic, so the
// result is the floor for negative n as well. TestLog10Pow2 checks every
// n in range against math/big.
func log10Pow2(n int) int {
	return int(int64(n) * log10Of2Fixed >> 32)
}

// errFromFloatDigits is the panic value of a conversion whose inexact
// quotient has too few digits for the rounding routine, which would mean
// that log10Pow2 is not exact: a bug, not an input condition.
const errFromFloatDigits = "internal/bid: binary conversion generated too few digits"
