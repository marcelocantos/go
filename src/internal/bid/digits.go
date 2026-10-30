// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// Decimal-digit primitives. Removing digits from a value is the one step at
// which a decimal operation can lose information, so it is done in one
// place and in one step: a shift by any power of ten returns the exact
// quotient and a digitClass that says where the whole discarded part lies
// relative to half a unit of the quotient's last place. A rounding function
// that takes those two has nothing else to look at, and because no
// intermediate quotient is ever rounded, there is no second rounding to get
// wrong. Removing digits one at a time, or remembering only the last digit
// removed, cannot be written in terms of these primitives.
//
// The primitives are in the coefficient vocabulary, in coef64.go and
// coef128.go, so that the template can reach them:
//
//	cShiftW(c coefW, k int) (q coefW, class digitClass)
//	cShiftFullW(p wideW, k int) (q wideW, class digitClass)
//	cReduceClassW(p wideW, k int) (q coefW, class digitClass)
//	cScaleW(c coefW, k int) (r coefW, overflow bool)
//	cScaleFullW(p wideW, k int) (r wideW, overflow bool)
//
// cShift64, cShift128 and cShiftFull128 are the 64-, 128- and 256-bit forms
// of the shift (cShiftFull64 is cShift128), and likewise for cScale. The
// digit counts are cDigits64, cDigits128 and cLenFull128 (cLenFull64 is
// cDigits128): a bit length gives the estimate and one comparison with the
// table settles it, with no loop.
//
// What a caller may assume of a shift of v by k digits:
//
//   - k must be non-negative; a negative k panics with an index error. There
//     is no upper limit: any k, including one far beyond the digits of v
//     and beyond the table, is correct.
//   - v = q × 10^k + r with 0 <= r < 10^k, and class says how r compares with
//     10^k / 2. The division is exact; nothing is estimated, so nothing needs
//     correcting by the caller.
//   - k = 0 gives q = v and classZero.
//   - k at or above the number of digits d of v gives q = 0 and r = v. At
//     k = d, v may lie on either side of half of 10^k, and may equal it. Above
//     that, v < 10^d <= 10^k / 2, so the class is classZero or classBelowHalf.
//   - k beyond the table of the width, where 10^k does not fit that width
//     (k >= 20, 39, 78 for 64, 128 and 256 bits), needs no arithmetic: every
//     value of the width is below half of the smallest such 10^k, because
//     2^64 < 5×10^19, 2^128 < 5×10^38 and 2^256 < 5×10^77. The class is
//     classZero for zero and classBelowHalf otherwise, never classHalf.
//
// The table of 256-bit powers ends at 10^77, past the 10^68 that bounds the
// product of two decimal128 coefficients, so no exact value needs a larger
// power. An aligned addition does not need one either, however large the
// exponent gap: it scales the operand with the larger exponent as far as the
// width allows and shifts the other right by the rest of the gap. If that
// exceeds the digits of the other operand, its quotient is zero and its class
// acts as a sticky bit.
//
// cScaleW multiplies by 10^k and reports overflow instead of wrapping. On
// overflow r is zero, so no wrapped value can be mistaken for a result. Zero
// scales to zero for every k >= 0; any other value overflows for every k
// beyond the table.

// A digitClass classifies the part of a value discarded by a shift: the
// remainder r of a division by 10^k, compared with 10^k / 2.
//
// The encoding is two independent facts, so that combining a class with a
// sticky bit is a single OR: bit 1 is r >= 10^k / 2, and bit 0 is that r is
// neither zero nor exactly half.
type digitClass uint8

const (
	classZero      digitClass = 0b00 // r = 0
	classBelowHalf digitClass = 0b01 // 0 < r < 10^k / 2
	classHalf      digitClass = 0b10 // r = 10^k / 2
	classAboveHalf digitClass = 0b11 // r > 10^k / 2
)

// withSticky returns c adjusted for an incoming sticky bit: the value that
// was shifted is not exactly v but v + f, for some unknown 0 < f < 1. This
// is the only place that combines the two.
//
// For k >= 1 the result is exact, whatever f is, because the half point
// 5×10^(k-1) is an integer. If r is below it then r + 1 is at most it, so
// r + f is still below it; if r is at or above it then r + f is above it. So
// classZero becomes classBelowHalf, classHalf becomes classAboveHalf, and
// the other two are unchanged.
//
// The same reasoning composes two exact shifts. Shifting by a and then by b
// >= 1 is shifting by a + b, with the class of the second step
// withSticky(the class of the first != classZero). The first step's class
// matters only as far as being non-zero, because only the second step's
// digits reach the half point.
//
// For k = 0 the half point 1/2 falls between the integers, and whether f
// exceeds it is unknown. The combination still answers classBelowHalf,
// which is the package's rule for that case: rounding (coeff + ε) for an
// arbitrarily small ε (see "Rounding from exact parts" in the package
// documentation). A caller whose sticky bit stands for more than that must
// keep a round digit in the coefficient, so that k >= 1.
func (c digitClass) withSticky(sticky bool) digitClass {
	if sticky {
		return c | classBelowHalf
	}
	return c
}

// complement returns the class of 1 - f, for a fraction f of class c: what a
// subtraction of v + f has left once it has borrowed the unit. Half is its
// own complement, and a zero fraction borrows nothing.
func (c digitClass) complement() digitClass {
	return c ^ c<<1&classHalf
}

// class64 classifies the remainder r of a division by 10^k, where half is
// 10^k >> 1. The test for zero comes first because at k = 0 half is also
// zero, and an empty tail is not a tie.
func class64(r, half uint64) digitClass {
	switch {
	case r == 0:
		return classZero
	case r < half:
		return classBelowHalf
	case r == half:
		return classHalf
	}
	return classAboveHalf
}

// class128 is class64 for a 128-bit remainder.
func class128(r, half Uint128) digitClass {
	if r == (Uint128{}) {
		return classZero
	}
	switch c := u128Cmp(r, half); {
	case c < 0:
		return classBelowHalf
	case c == 0:
		return classHalf
	}
	return classAboveHalf
}

// reduceClass64 is cReduceClass64, out of line because a 128-by-64 division
// and its classification exceed the inlining budget of a coefficient
// helper. A k within the 64-bit table is the common case and is one word
// division: the operation that the decimal64 multiply is timed with.
func reduceClass64(p Uint128, k int) (q uint64, class digitClass) {
	if k >= len(pow10tab64) {
		// 10^k >= 10^20 and p < 2^128, so the quotient is below 2^64.
		wq, class := cShift128(p, k)
		return wq.Lo, class
	}
	d := pow10tab64[k]
	if p.Hi == 0 {
		// A sum never needs the high word, and a word division is much the
		// cheaper: bits.Div64 is software on some ports and slow on others.
		q = p.Lo / d
		return q, class64(p.Lo-q*d, d>>1)
	}
	if p.Hi >= d {
		panic(errQuotientOverflow)
	}
	q, r := bits.Div64(p.Hi, p.Lo, d)
	return q, class64(r, d>>1)
}

// errQuotientOverflow is the panic value of a reduction whose quotient does
// not fit the coefficient type, which is a bug in the caller.
const errQuotientOverflow = "internal/bid: quotient overflow"
