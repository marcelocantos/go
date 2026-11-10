// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// addSlow64 is the sum of decimal64 operands that add64 did not decide:
// addLong64 is all of it.
func addSlow64(x, y uint64, mode Mode) uint64 {
	return addLong64(x, y, mode)
}

// addSlow128 returns x + y rounded in mode. Finite operands of which one
// at least has a coefficient of two words, in a mode that rounds to
// nearest (what a division or a long product left, added to an amount or
// to another such value), are added here, and every other operand, and a
// result that this path does not encode, goes to addLong128, which decides
// it from the start. Operands of one word each are among those: the word
// paths in front of addLong128 are theirs.
//
// The method is addLong128's (see addLongW), with one call. The operand
// with the larger exponent, a, is chosen by a mask, since which it is
// depends on the data as much as on the program. It is scaled up by the m
// places that its digit count leaves, at most to the exponent of the
// other, b.
//
// If that reaches the exponent of b the sum is exact: p ± b at that
// exponent, with the sign of the larger.
//
// Otherwise b loses its d lowest digits, d being what is left of the gap,
// and the sum has the 34 digits of p. As in division, b is biased by half
// of 10^d before it is divided: the quotient is then b / 10^d rounded half
// up, p plus or minus it is the sum rounded, and the remainder says the
// rest, being zero exactly on a tie. A tie goes to the even neighbour, or
// away from zero, by an adjustment of one: the sum of a tie is the upper
// of its two neighbours and the difference the lower.
//
// A sum that carries into a 35th digit loses one more place, by the last
// digit of the unrounded sum and what lay below it (addCarry128). A
// rounded difference at or below 10^33 has lost its leading digit and
// needs the digits of b that the shift dropped: with at most four of them
// the exact difference fits two words and is rounded to its own length,
// and addLong128 takes the rest.
//
// Two variants were measured and set aside. Dividing always, by one when
// nothing is dropped, so that the exact and the rounded sum are one path,
// and adding and subtracting as one sum under a mask, each make the
// function indifferent to operands that mix those cases and slower by a
// third or more on operands that do not, which are the usual ones.
//
// The compiler calls this function directly, so its parameters and result
// are a contract with it: see the end of "Inline fast paths" in the package
// documentation.
func addSlow128(x, y Uint128, mode Mode) Uint128 {
	// Coefficients of one word each go on at once, before anything is
	// decoded: the compiler sends those to addLong128 itself, and add128
	// cannot afford the test (it would no longer inline).
	if wordPair128(x, y) {
		return addLong128(x, y, mode)
	}
	xhi, yhi, xe, ye, ok := addPair128(x, y)
	if !ok || mode > ToNearestAway {
		return addLong128(x, y, mode)
	}
	// a is the operand with the larger exponent, x if they are equal.
	sw := uint64(int64(xe-ye) >> 63) // all ones when y has the larger
	th, tl := (xhi^yhi)&sw, (x.Lo^y.Lo)&sw
	a := Uint128{Hi: xhi ^ th, Lo: x.Lo ^ tl}
	b := Uint128{Hi: yhi ^ th, Lo: y.Lo ^ tl}
	gap := int((uint64(xe-ye) ^ sw) - sw)
	exp := ye + int(uint64(xe-ye)&sw) // the smaller exponent, biased
	sub := (x.Hi ^ y.Hi) >> 63        // 1 when the signs differ
	neg := (x.Hi^(x.Hi^y.Hi)&sw)>>63 != 0
	da := cDigits128(a)
	// A zero a has no digits and a coefficient that is not canonical has
	// 35; b is tested for itself.
	if uint(da-1) >= Precision128 || u128GtMaxCoeff(b) {
		return addLong128(x, y, mode)
	}
	m := Precision128 - da
	d := gap - m
	var s Uint128
	if d <= 0 {
		// Exact: a reaches the exponent of b.
		p := u128MulLow(a, pow10tab128[gap])
		if sub == 0 {
			s, _ = u128Add(p, b)
			if u128GtMaxCoeff(s) {
				s, exp = addCarry128(s, false, mode), exp+1
			}
		} else {
			var borrow uint64
			s, borrow = u128Sub(p, b)
			if borrow != 0 {
				s, _ = u128Sub(Uint128{}, s)
				neg = !neg
			}
			if s == (Uint128{}) {
				neg = false // an exact cancellation, to nearest
			}
		}
	} else {
		p := u128MulLow(a, pow10tab128[m])
		exp += d
		// b / 10^d rounded half up, and its remainder, which is zero
		// exactly on a tie. A b of fewer than d digits rounds to nothing
		// and does not tie.
		var q, rb, half Uint128
		rb.Lo = 1
		if d <= Precision128 {
			// divPow10Recip128's method (see recip128.go), written
			// out so that the usual sum calls nothing and keeps its
			// operands in registers: the estimate is the high half
			// of t times the reciprocal, at most one short, and the
			// remainder against it says which.
			pd := &pow10tab128[d]
			half = u128Shr(*pd, 1)
			t, _ := u128Add(b, half)
			rm := &recip10tab128[d].t
			h00, _ := bits.Mul64(t.Lo, rm.Lo)
			h01, l01 := bits.Mul64(t.Lo, rm.Hi)
			h10, l10 := bits.Mul64(t.Hi, rm.Lo)
			h11, l11 := bits.Mul64(t.Hi, rm.Hi)
			mid, c1 := bits.Add64(h00, l01, 0)
			_, c2 := bits.Add64(mid, l10, 0)
			q.Lo, c1 = bits.Add64(l11, h01, c1)
			q.Hi = h11 + c1
			q.Lo, c2 = bits.Add64(q.Lo, h10, c2)
			q.Hi += c2
			ph, pl := bits.Mul64(q.Lo, pd.Lo)
			ph += q.Lo*pd.Hi + q.Hi*pd.Lo
			var bw uint64
			rb.Lo, bw = bits.Sub64(t.Lo, pl, 0)
			rb.Hi, _ = bits.Sub64(t.Hi, ph, bw)
			if !u128Lt(rb, *pd) {
				q, _ = u128Add(q, Uint128{Lo: 1})
				rb, _ = u128Sub(rb, *pd)
			}
		}
		tie := uint64(0)
		if rb == (Uint128{}) {
			tie = 1
		}
		if sub == 0 {
			s, _ = u128Add(p, q)
			if u128GtMaxCoeff(s) {
				// The sum before rounding is p plus the quotient
				// rounded down, which is q less one when the
				// remainder reached the half; something lay below it
				// unless the division was exact.
				t := s
				if d <= Precision128 && u128Lt(rb, half) {
					t, _ = u128Sub(s, Uint128{Lo: 1})
				}
				s, exp = addCarry128(t, d > Precision128 || rb != half, mode), exp+1
			} else if mode == ToNearestEven {
				s.Lo &^= tie
			}
		} else {
			s, _ = u128Sub(p, q)
			up := tie
			if mode == ToNearestEven {
				up &= s.Lo
			}
			s, _ = u128Add(s, Uint128{Lo: up})
			if !u128Lt(pow10tab128[Precision128-1], s) {
				// The leading digit is gone, and the digits of b that
				// the shift dropped are wanted back. With at most
				// four of them the exact difference a × 10^gap - b
				// fits two words: it is taken whole and rounded to
				// its own length. More than four cannot cancel this
				// far unless p is 10^33 and a little: addLong128's.
				const spare = len(pow10tab128) - 1 - Precision128
				if d > spare {
					return addLong128(x, y, mode)
				}
				s, _ = u128Sub(u128MulLow(a, pow10tab128[gap]), b)
				exp -= d
				if j := cDigits128(s) - Precision128; j > 0 {
					t, _ := u128Add(s, u128Shr(pow10tab128[j], 1))
					var r Uint128
					s, r = divPow10Recip128(t, j)
					if r == (Uint128{}) && mode == ToNearestEven {
						s.Lo &^= 1
					}
					exp += j
				}
			}
		}
	}
	if r, ok := packWord128(neg, s.Hi, s.Lo, exp-Bias128); ok {
		return r
	}
	return addLong128(x, y, mode)
}

// addCarry128 rounds to nearest a sum t of 35 digits, with below saying
// whether anything non-zero lay under it, to the 34 digits that are left
// when its last digit u goes: up when u is above five, or is five with
// something below it, and on a tie (five, nothing below) to the even
// neighbour or away from zero as mode says.
func addCarry128(t Uint128, below bool, mode Mode) Uint128 {
	q, u := divPow10Recip128(t, 1)
	// The digit that goes is as good as random, so the decision is
	// arithmetic: u + 3 reaches 8 when u is five or more and 9 when it
	// is six or more, and a five counts as a six when the tie goes up.
	tieUp := q.Lo & 1
	if below || mode == ToNearestAway {
		tieUp = 1
	}
	_, five := bits.Sub64(4, u.Lo, 0) // 1 when u >= 5
	_, six := bits.Sub64(5, u.Lo, 0)  // 1 when u >= 6
	q, _ = u128Add(q, Uint128{Lo: six | five&tieUp})
	return q
}

// u128MulLow returns the low 128 bits of a × b: the product, when the
// caller knows that it fits.
func u128MulLow(a, b Uint128) Uint128 {
	hi, lo := bits.Mul64(a.Lo, b.Lo)
	return Uint128{Hi: hi + a.Lo*b.Hi + a.Hi*b.Lo, Lo: lo}
}
