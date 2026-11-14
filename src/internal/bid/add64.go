// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// addSlow64 returns x + y rounded in mode, for the operands that add64 did
// not decide. Finite canonical operands whose exponents are at most
// maxWordGap apart, in a mode that rounds to nearest, are added here, and
// so are those far enough apart that one is swamped, in any mode; every
// other operand goes to addLong64, which decides it from the start.
//
// The method is addSlow128's, in one word. The operand with the larger
// exponent, a, is chosen by a mask, since which it is depends on the data
// as much as on the program, and is scaled up by the m places that its
// digit count leaves, to 16 digits or to the exponent of the other, b. If
// that reaches the exponent of b the sum is exact, in a word, at the
// preferred exponent. Otherwise b loses its d lowest digits, d being what
// is left of the gap, and the sum has the 16 digits of p, a scaled. b is
// biased by half of 10^d before it is divided: the quotient is then
// b / 10^d rounded half up, p plus or minus it is the sum rounded, and the
// remainder says the rest, being zero exactly on a tie. A tie goes to the
// even neighbour, or away from zero, by an adjustment of one: the sum of a
// tie is the upper of its two neighbours and the difference the lower. A
// sum that carries into a 17th digit, and a difference that may have lost
// its leading digit and needs the digits of b that the division dropped,
// go to addWide64, which takes the sum exactly in two words and rounds it
// once. The swamped addend is addLong64's word path for far operands, here
// in front of the call.
//
// What the digits decide is computed: which operand has the larger
// exponent, whether the signs differ, the sign of a difference, the
// direction of a rounding and the form of the result. What the kind of
// operand decides stays a branch: whether the exponents are far apart,
// whether a has 16 digits, which spares the digit count, and whether the
// sum is exact. So does the form of each operand, which is in its leading
// digits: wordParts64's comment says why.
//
// The compiler calls this function directly, so its parameters and result
// are a contract with it: see the end of "Inline fast paths" in the package
// documentation.
func addSlow64(x, y uint64, mode Mode) uint64 {
	xc, xe := wordParts64(x)
	yc, ye := wordParts64(y)
	if xe|ye < 0 {
		return addLong64(x, y, mode)
	}
	if uint(xe-ye)+maxWordGap > 2*maxWordGap {
		// A swamped addend: addLong64's word path for operands far
		// apart, in every mode (see the comment there). Which operand
		// is the larger is a branch here, as there, since far operands
		// are a kind of data.
		sa, ac, ea, gap, bc, sb := x>>63, xc, xe, uint(xe-ye), yc, y>>63
		if xe < ye {
			sa, ac, ea, gap, bc, sb = sb, yc, ye, uint(ye-xe), xc, sa
		}
		if ac != 0 {
			if m := Precision64 - wordDigits(ac); gap >= uint(m+farWordGap) {
				// a × 10^m has 16 digits, at an exponent above that
				// of b.
				p := ac * pow10word[m&31]
				if mode <= ToNearestAway || bc == 0 {
					return packWordSign64(sa, p, ea-m-Bias64)
				}
				if sa == sb {
					return roundFace64(sa != 0, p, ea-m-Bias64, classBelowHalf, mode)
				}
				if p == pow10word[Precision64-1] {
					p, m = pow10word[Precision64], m+1
				}
				return roundFace64(sa != 0, p-1, ea-m-Bias64, classAboveHalf, mode)
			}
		}
		return addLong64(x, y, mode)
	}
	if mode > ToNearestAway {
		return addLong64(x, y, mode)
	}
	// a is the operand with the larger exponent, x if they are equal.
	sw := uint64(int64(xe-ye) >> 63) // all ones when y has the larger
	t := (xc ^ yc) & sw
	ac, bc := xc^t, yc^t
	gap := uint((uint64(xe-ye) ^ sw) - sw)
	exp := ye + int(uint64(xe-ye)&sw) // the smaller exponent, biased
	sub := (x ^ y) >> 63              // 1 when the signs differ
	ms := -sub                        // all ones when the signs differ
	sa := (x ^ (x^y)&sw) >> 63        // the sign of a
	// An a of 10^15 or more has 16 digits and needs no count, which
	// spares operands of full precision the longest chain of the sum. A
	// zero a is given a scale that reaches every b, since the sum is then
	// b exactly.
	m, p := 0, ac
	if ac < pow10word[Precision64-1] {
		m = Precision64 - wordDigits(ac)
		if ac == 0 {
			m = int(maxWordGap)
		}
		p = ac * pow10word[m&31]
	}
	d := int(gap) - m
	if d <= 0 {
		// Exact, in a word: a reaches the exponent of b, below 10^16 and
		// b added. A difference below zero is negated, with the sign of
		// b, and a zero is negative only if both operands are, to
		// nearest.
		s := ac*pow10word[gap&31] + (bc ^ ms) + sub
		n := uint64(int64(s) >> 63) // all ones when b was the larger
		s = (s ^ n) - n
		sign := sa ^ n&1
		if s == 0 {
			sign = sa &^ sub
		}
		if s >= pow10word[Precision64] {
			return addWide64(x, y, mode, ac, bc, gap, exp, sub, sa)
		}
		return packWordSign64(sign, s, exp-Bias64)
	}
	// b / 10^d rounded half up, and whether that was a tie.
	pk := pow10word[d&31]
	tb := bc + pk>>1
	q := tb / pk
	tie := uint64(0)
	if tb-q*pk == 0 {
		tie = 1
	}
	s := p + (q ^ ms) + sub
	odd := s
	if mode != ToNearestEven {
		odd = sub // away from zero, a tie moves only a difference
	}
	ma := sub - 1 // all ones for a sum, which a tie moves down
	st := s + (odd&1 ^ ma) - ma
	if tie != 0 {
		s = st
	}
	// A sum below 10^16 has 16 digits. A difference above 10^15 has
	// them too, and the exact difference, at most half a unit below it,
	// is above 10^15 as well; at 10^15 it may be below and need a digit
	// that the division dropped. A sum of p, which has 16 digits, is
	// never 10^15 or below but at p = 10^15 and nothing added, which
	// addWide64 takes as well.
	if s >= pow10word[Precision64] || s <= pow10word[Precision64-1] {
		return addWide64(x, y, mode, ac, bc, gap, exp, sub, sa)
	}
	// The exponent of the result, ea - m, is at most that of a and
	// above that of b.
	return packWordSign64(sa, s, exp+d-Bias64)
}

// addWide64 is addSlow64 for the sums its word arithmetic does not round:
// a sum of 17 digits or more (35 at most, since gap <= maxWordGap), and a
// difference that may have lost its leading digit, taken exactly in two
// words. a is ac at the larger exponent and b is bc at exp, the smaller,
// biased, gap places below it; sub is 1 when the signs differ and sa is the
// sign of a, which is the sign of the result: a difference comes here only
// when a scaled to the exponent of b has 17 digits or more, and is the
// larger.
//
// A difference that has fewer than 17 digits is exact, at the preferred
// exponent. Otherwise s has D digits and loses k = D - 16 of them, biased
// by half of 10^k so that the quotient by the reciprocal of recip.go is
// s / 10^k rounded half up, with the remainder zero exactly on a tie. A
// quotient that reaches 10^16 is 10^15 one place up, and that place may
// be beyond MaxExp64, which addLong64 takes.
func addWide64(x, y uint64, mode Mode, ac, bc uint64, gap uint, exp int, sub, sa uint64) uint64 {
	// s = a × 10^gap ± b, exactly.
	hi, lo := bits.Mul64(ac, pow10tab64[gap])
	ms := -sub
	var c uint64
	lo, c = bits.Add64(lo, bc^ms, sub)
	hi, _ = bits.Add64(hi, ms, c)
	if hi == 0 && lo <= MaxCoeff64 {
		return packWordSign64(sa, lo, exp-Bias64)
	}
	k := cDigits128(Uint128{Hi: hi, Lo: lo}) - Precision64
	// q = (s + 10^k / 2) / 10^k by divPow10Recip64's method: s is below
	// 10^(k+16), so the biased s is below 10^k × 2^56, and the estimate
	// is q or q - 1.
	pk := pow10tab64[k]
	lo, c = bits.Add64(lo, pk>>1, 0)
	hi += c
	e := &recip10tab64[k]
	sh := uint(e.s)
	q, _ := bits.Mul64(lo>>sh|hi<<(64-sh), e.m)
	ph, pl := bits.Mul64(q, pk)
	r, c := bits.Sub64(lo, pl, 0)
	rh := hi - ph - c
	_, c = bits.Sub64(r, pk, 0) // 1 when r < 10^k
	up := rh | (c ^ 1)
	q += up
	r -= pk & -up
	if mode == ToNearestEven && r == 0 {
		q &^= 1
	}
	// A quotient of 10^16 is 10^15 one place up.
	cy := (MaxCoeff64 - q) >> 63
	q -= (pow10tab64[Precision64] - pow10tab64[Precision64-1]) & -cy
	exp += k + int(cy)
	if exp > MaxExp64+Bias64 {
		return addLong64(x, y, mode) // overflow
	}
	return packWordSign64(sa, q, exp-Bias64)
}
