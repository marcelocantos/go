// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// Addition of decimal64 and, by negating the second operand, subtraction.

// addDigits64 is the number of digits add64 scales a coefficient up to: the
// most the coefficient type holds for every value, 19 digits, the exponent
// of the last power of ten in pow10tab64. Two bounds allow it. The sum of
// two operands aligned this way must fit the coefficient type, so no
// addition needs the wide type: it is below 10^19 + 10^16 < 2^64. And an
// operand scaled this far must keep more than Precision64 digits when the
// other, shifted, is taken from it, which is what round64 requires of a
// value with a discarded tail: Precision64 plus two digits would do,
// because the shifted operand is at most 10^Precision64, a borrow
// included, and addDigits64 is Precision64+3. The larger it is, the wider
// the gap that an exact sum spans: two operands of Precision64 digits up
// to three places apart are added exactly and rounded once, with no shift
// of the smaller.
const addDigits64 = len(pow10tab64) - 1

// add64 returns x + y rounded in mode. Word operands of one sign and one
// exponent are added inline by addSameQuantum64, and every other operand
// takes addSlow64; see "Inline fast paths" in the package documentation.
func add64(x, y coef64, mode Mode) coef64 {
	if r, ok := addSameQuantum64(x, y); ok {
		return r
	}
	return addSlow64(x, y, mode)
}

// sub64 returns x - y rounded in mode. Word operands of one sign and one
// exponent are subtracted inline by subSameQuantum64, and every other operand
// takes subSlow64.
func sub64(x, y coef64, mode Mode) coef64 {
	if r, ok := subSameQuantum64(x, y); ok {
		return r
	}
	return subSlow64(x, y, mode)
}

// subSlow64 returns x - y rounded in mode: addSlow64(x, -y, mode), except that
// a NaN y is not negated, so that the result for it is y made quiet with its
// own sign (see "NaN results" in the package documentation).
func subSlow64(x, y coef64, mode Mode) coef64 {
	if !IsNaN64(y) {
		y = Neg64(y)
	}
	return addSlow64(x, y, mode)
}

// addLong64 returns x + y rounded in mode, for every operand: addSlow64,
// which the callers above and the compiler call, takes most sums of word
// coefficients to nearest itself (below), and hands the rest to this
// function.
//
// The operand with the larger exponent is scaled up to the exponent of the
// other if that takes it to addDigits64 digits or fewer, and then the sum is
// exact and its exponent is the preferred one. Otherwise, however large the
// gap between the exponents, the larger operand is scaled to addDigits64
// digits and the smaller is shifted right by the rest of the gap, leaving a
// fraction f of one unit whose class is known exactly. If the rest of the gap
// exceeds the digits of the smaller operand, the shift leaves nothing but f,
// and a sticky bit is all that remains of that operand.
//
// A difference with such a fraction is hi - (lo + f), and it reaches round64
// as the integer hi - (lo + 1) and the fraction 1 - f. Dropping f instead
// would leave x unchanged where x - ε must land below x in a directed mode.
//
// Two finite operands whose coefficients fit a word and whose exponents are
// at most maxWordGap apart take a fast path first, and if it refuses them the
// general path starts from the operands as the fast path decoded them, so
// that a refusal costs only the attempt (every decimal64 operand is a word
// one, and its refusal is a sum of more than Precision64 digits). It
// computes their exact
// sum in two words at the smaller exponent, the preferred one, and
// packWord64 encodes it if it fits the format; a sum that does not fit, which
// needs rounding, falls through to the general path. That is the exact
// packing exception round64's comment allows, and the general path gives the
// same encoding. It scales the operand with the larger exponent by
// k = min(gap, addDigits64 - digits) places and shifts the other only if
// k < gap; then the scaled operand has at least addDigits64 >= Precision64+3
// digits and the other is below 10^Precision64, so the sum or difference has
// more than Precision64 digits and does not fit. When k = gap the general sum
// is this one, exact, at the same exponent, and round64 packs it unchanged.
// The sign follows the general path too: that of the operands when they
// agree, that of the larger magnitude when they differ, and for an exact
// cancellation the zero of "Sign of a result", negative only in
// TowardNegative.
func addLong64(x, y coef64, mode Mode) coef64 {
	var yneg, yok bool
	var yw uint64
	var ye int
	xneg, xw, xe, xok := unpackWordCanon64(x)
	if xok {
		yneg, yw, ye, yok = unpackWordCanon64(y)
	}
	var xc, yc coef64
	if yok {
		if xe < ye {
			xneg, xw, xe, yneg, yw, ye = yneg, yw, ye, xneg, xw, xe
		}
		if gap := uint(xe - ye); gap <= maxWordGap {
			hi, lo := bits.Mul64(xw, pow10tab64[gap])
			neg := xneg
			if xneg == yneg {
				var carry uint64
				lo, carry = bits.Add64(lo, yw, 0)
				hi += carry
			} else if hi == 0 && lo < yw {
				neg, lo = yneg, yw-lo
			} else {
				var borrow uint64
				lo, borrow = bits.Sub64(lo, yw, 0)
				hi -= borrow
				if hi|lo == 0 {
					neg = mode == TowardNegative
				}
			}
			if r, ok := packWord64(neg, hi, lo, ye-Bias64); ok {
				return r
			}
		} else if xw != 0 {
			// A y so far below x that it only says which way the sum
			// lies from x scaled to its Precision64 digits, X: the gap
			// leaves a word y below a twentieth of a unit of X (see
			// farWordGap). To nearest it then changes nothing: X + y
			// rounds down to X, and X - y up to it even when X is a
			// power of ten and the difference has its last digit one
			// place lower; and a zero y leaves X exactly, in any mode.
			// packWord64 encodes X. In a directed mode a non-zero y is
			// the fraction of the general path's swamped addend, below
			// a half above X or above a half above X - 1, and
			// roundFace64 rounds it at face value as it does there. The
			// difference from a power of ten has its Precision64 digits
			// one place lower, all nines, and what lies above them is
			// one less ten times the fraction: above a half still,
			// which is why the bound is a twentieth and not a half.
			if m := Precision64 - wordDigits(xw); gap >= uint(m+farWordGap) {
				p := cMulPow1064(cFromU6464(xw), m)
				if mode <= ToNearestAway || yw == 0 {
					hi, lo := cWords64(p)
					if r, ok := packWord64(xneg, hi, lo, xe-m-Bias64); ok {
						return r
					}
				} else if xneg == yneg {
					return roundFace64(xneg, p, xe-m-Bias64, classBelowHalf, mode)
				} else {
					if cEq64(p, cPow1064(Precision64-1)) {
						p, m = cPow1064(Precision64), m+1
					}
					p, _ = cSub64(p, cPow1064(0))
					return roundFace64(xneg, p, xe-m-Bias64, classAboveHalf, mode)
				}
			}
		}
		xc, xe, yc, ye = cFromU6464(xw), xe-Bias64, cFromU6464(yw), ye-Bias64
	} else {
		xneg, xc, xe, xok = unpackFirst64(x)
		yneg, yc, ye, yok = unpackFirst64(y)
		if !xok || !yok {
			var xk, yk Kind
			xk, xneg, xc, xe = unpackRest64(x)
			yk, yneg, yc, ye = unpackRest64(y)
			if xk != KindFinite || yk != KindFinite {
				switch {
				case xk == KindNaN || yk == KindNaN:
					return nan64(x, y, y)
				case xk != KindInf:
					return cInf64(yneg)
				case yk == KindInf && xneg != yneg:
					return cNaN64() // the infinities cancel
				}
				return cInf64(xneg)
			}
		}
		if xe < ye {
			xneg, xc, xe, yneg, yc, ye = yneg, yc, ye, xneg, xc, xe
		}
	}

	exp, class := ye, classZero
	// A zero x has nothing to scale: the sum is y exactly, at the preferred
	// exponent ye, and shifting y would only lose digits of it.
	if gap := xe - ye; gap > 0 && !cIsZero64(xc) {
		// A swamped y. Scaled by m places, x has exactly Precision64 digits
		// X; if the gap exceeds m by more than the digits of a non-zero y,
		// y is a fraction f of one unit of X with 0 < f < 1/10, and the sum
		// is X + f or (X - 1) + (1 - f). Their classes are then known, below
		// a half and above it, and roundFace64 rounds them at face value with
		// a coefficient of Precision64 digits (see its comment): it drops no
		// digit and divides nothing. X - 1 keeps Precision64 digits unless X
		// is 10^(Precision64-1), whose difference would need padding, which a
		// face-value class does not survive. That X is taken one place
		// further instead, to 10^Precision64, if the gap leaves f below 1/10
		// there too, and the difference is 10^Precision64 - 1 and 1 - 10f;
		// otherwise it takes the general path.
		//
		// The test needs no digit count of y: a non-zero y has fewer than
		// gap - m digits exactly when it is below 10^s with s = gap - m - 1,
		// which holds for no y when s <= 0 and for every y when s >=
		// Precision64, since y < 10^Precision64.
		dx := cDigits64(xc)
		m := Precision64 - dx
		if s := gap - m - 1; s > 0 && !cIsZero64(yc) && (s >= Precision64 || cLt64(yc, cPow1064(s))) {
			p, class := cMulPow1064(xc, m), classBelowHalf
			near := p // X: what both classes round to in a nearest mode
			if xneg != yneg {
				class = classAboveHalf
				if cEq64(p, cPow1064(Precision64-1)) {
					m, s = m+1, s-1
					p = cPow1064(Precision64)
					near = p
				}
				p, _ = cSub64(p, cPow1064(0))
			}
			if s > 0 && (s >= Precision64 || cLt64(yc, cPow1064(s))) {
				// To nearest there is nothing to decide: X + f rounds
				// down to X and (X - 1) + (1 - f) up to X, whatever the
				// parity, and 10^Precision64 is roundFace64's carry.
				if mode <= ToNearestAway || mode > TowardPositive {
					return roundFace64(xneg, near, xe-m, classZero, mode)
				}
				return roundFace64(xneg, p, xe-m, class, mode)
			}
		}
		// An aligned y. With x scaled to its Precision64 digits X and y
		// shifted down to X's place as Y + f, the sum is X + Y + f and the
		// difference (X - Y - 1) + (1 - f) when f is not zero: one division
		// and one class. Whenever that coefficient still has exactly
		// Precision64 digits it rounds at face value, as the swamped sum
		// does. A sum that carries to Precision64+1 digits goes to round64
		// with the same class, which is still the class of everything
		// below it. A difference that loses its leading digit needs digits
		// of y this shift discarded, and takes the path below.
		if m := Precision64 - dx; gap > m {
			p := cMulPow1064(xc, m)
			q, c := cShift64(yc, gap-m)
			if xneg == yneg {
				p, _ = cAdd64(p, q)
				if c != classZero && !cGtMax64(p) {
					return roundFace64(xneg, p, xe-m, c, mode)
				}
				r, _ := round64(xneg, wide64{Lo: p}, xe-m, c, mode, MinExp64)
				return r
			}
			if c != classZero {
				q, c = cInc64(q), c.complement()
			}
			if p, _ = cSub64(p, q); !cLt64(p, cPow1064(Precision64-1)) {
				if c != classZero {
					return roundFace64(xneg, p, xe-m, c, mode)
				}
				r, _ := round64(xneg, wide64{Lo: p}, xe-m, c, mode, MinExp64)
				return r
			}
		}
		k := min(gap, addDigits64-dx)
		xc = cMulPow1064(xc, k)
		if k < gap {
			exp = xe - k
			yc, class = cShift64(yc, gap-k)
		}
	}

	neg := xneg
	var sum coef64
	if xneg == yneg {
		sum, _ = cAdd64(xc, yc)
	} else {
		if class != classZero {
			yc, class = cInc64(yc), class.complement()
		}
		var borrow bool
		sum, borrow = cSub64(xc, yc)
		switch {
		case borrow:
			// Only an exact sum can borrow: a shifted operand is the smaller
			// by two places at least.
			var zero coef64
			sum, _ = cSub64(zero, sum)
			neg = !xneg
		case cIsZero64(sum):
			neg = mode == TowardNegative // an exact cancellation
		}
	}
	r, _ := round64(neg, wide64{Lo: sum}, exp, class, mode, MinExp64)
	return r
}

// roundFace64 returns (-1)^neg × (q + f) × 10^exp rounded in mode, for the
// swamped and the aligned addend of addLong64, for decimal64 the swamped
// addend of addSlow64 too (addLong64's word path for far operands, in
// front of it), and the inexact quotient of divLong64, its only callers: q
// has exactly Precision64 digits, 10^(Precision64-1)
// <= q <= cMax64(), and f is a fraction, not zero, whose class is known at
// face value (classBelowHalf or classAboveHalf for a swamped addend; an
// aligned addend's discarded digits and a quotient's remainder can also be
// exactly a half). It is the face-value exception to the rule that every
// rounded result passes through round64; see "Face-value rounding" in the
// package documentation.
//
// Why it gives round64's bits. The callers establish MinExp64 < exp <=
// MaxExp64 and an adjusted exponent exp + Precision64 - 1 of at least Emin64
// (addLong64 from its operands' exponents: exp = xe - m, or xe - m - 1 for
// the padded power of ten, with xe <= MaxExp64 and exp > ye >= MinExp64,
// since the gap xe - ye exceeds m, by s + 1 with s > 0 for a swamped
// addend; divLong64 by testing them before the call), so exp is at or above the
// floor MinExp64 and the value is not tiny: nothing underflows.
// q has Precision64 digits, so round64's place of the result is exp itself:
// it pads nothing, discards no digit of q, and its class is the caller's.
// Its decision is then roundInc(class, mode, neg, odd(q)), the one row of
// the six-mode table this function takes, ties included: a swamped
// addend's class is never classHalf, the others' may be, and roundInc
// decides a tie as round64 would. The class is never classZero: roundInc
// takes an inexact value, and an exact one goes to round64. An increment of
// cMax64() carries to 10^Precision64, which
// round64 encodes as 10^(Precision64-1) one place up, and that place above
// MaxExp64 overflows to overflow64's result. Every other step of round64 (the
// clamp, the digit count, the shift) leaves such a value unchanged.
//
// A caller that has rounded q itself, to nearest, passes classZero in a
// nearest mode: roundInc adds nothing for it there (in a directed mode it
// would), and what is left is the encoding, with the carry of a q that the
// caller's rounding took to 10^Precision64. divLong64 rounds its quotient that
// way, by biasing the dividend, and the swamped sum of addLong64 is x
// itself in a nearest mode.
// TestRoundFaceValue drives every bound here in every mode against the
// reference, TestSwampedAdd the classes around the swamping gap,
// TestAlignedAdd the aligned addend's classes, carries and cancellations,
// and the oracle's far and general regimes reach it in every mode.
func roundFace64(neg bool, q coef64, exp int, class digitClass, mode Mode) coef64 {
	// The increment is added, not branched on (roundInc); the carry out
	// of the largest coefficient is the branch, and it is rare.
	q = cAddU6464(q, roundInc(class, mode, neg, cOdd64(q)))
	if cGtMax64(q) {
		q, exp = cPow1064(Precision64-1), exp+1
		if exp > MaxExp64 {
			r, _ := overflow64(neg, mode)
			return r
		}
	}
	return pack64(neg, q, exp)
}

// addSlow64 returns x + y rounded in mode, for the operands that add64 did
// not decide. Finite canonical operands whose exponents are at most
// maxWordGap apart, in a mode that rounds to nearest, are added here, and
// so are those far enough apart that one is swamped, in any mode; every
// other operand goes to addLong64, which decides it from the start.
//
// The method is ubdAddTTE128's, in one word. The operand with the larger
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
