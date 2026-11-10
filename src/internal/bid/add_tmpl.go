// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zadd64.go and zadd128.go: addition and, by
// negating the second operand, subtraction. See arith_tmpl.go for what a
// template is and "Width parameterisation" in the package documentation for
// its rules.
package bid

import "math/bits"

// addDigitsW is the number of digits addW scales a coefficient up to: the
// most the coefficient type holds for every value, 19 digits in 64 bits and
// 38 in 128, the exponent of the last power of ten in pow10tabW. Two bounds
// allow it. The sum of two operands aligned this way must fit the
// coefficient type, so no addition needs the wide type: it is below
// 10^19 + 10^16 < 2^64 and 10^38 + 10^34 < 2^128. And an operand scaled this
// far must keep more than PrecisionW digits when the other, shifted, is
// taken from it, which is what roundW requires of a value with a discarded
// tail: PrecisionW plus two digits would do, because the shifted operand is
// at most 10^PrecisionW, a borrow included, and addDigitsW is PrecisionW+3
// for decimal64 and PrecisionW+4 for decimal128. The larger it is, the
// wider the gap that an exact sum spans: two decimal128 operands of
// PrecisionW digits up to four places apart are added exactly and rounded
// once, with no shift of the smaller.
const addDigitsW = len(pow10tabW) - 1

// addW returns x + y rounded in mode. Word operands of one sign and one
// exponent are added inline by addSameQuantumW, and every other operand
// takes addSlowW; see "Inline fast paths" in the package documentation.
func addW(x, y coefW, mode Mode) coefW {
	if r, ok := addSameQuantumW(x, y); ok {
		return r
	}
	return addSlowW(x, y, mode)
}

// subW returns x - y rounded in mode. Word operands of one sign and one
// exponent are subtracted inline by subSameQuantumW, and every other operand
// takes subSlowW.
func subW(x, y coefW, mode Mode) coefW {
	if r, ok := subSameQuantumW(x, y); ok {
		return r
	}
	return subSlowW(x, y, mode)
}

// subSlowW returns x - y rounded in mode: addSlowW(x, -y, mode), except that
// a NaN y is not negated, so that the result for it is y made quiet with its
// own sign (see "NaN results" in the package documentation).
func subSlowW(x, y coefW, mode Mode) coefW {
	if !IsNaNW(y) {
		y = NegW(y)
	}
	return addSlowW(x, y, mode)
}

// addSlowW returns x + y rounded in mode.
//
// The operand with the larger exponent is scaled up to the exponent of the
// other if that takes it to addDigitsW digits or fewer, and then the sum is
// exact and its exponent is the preferred one. Otherwise, however large the
// gap between the exponents, the larger operand is scaled to addDigitsW
// digits and the smaller is shifted right by the rest of the gap, leaving a
// fraction f of one unit whose class is known exactly. If the rest of the gap
// exceeds the digits of the smaller operand, the shift leaves nothing but f,
// and a sticky bit is all that remains of that operand.
//
// A difference with such a fraction is hi - (lo + f), and it reaches roundW
// as the integer hi - (lo + 1) and the fraction 1 - f. Dropping f instead
// would leave x unchanged where x - ε must land below x in a directed mode.
//
// Two finite operands whose coefficients fit a word and whose exponents are
// at most maxWordGap apart take a fast path first, and if it refuses them the
// general path starts from the operands as the fast path decoded them, so
// that a refusal costs only the attempt (every decimal64 operand is a word
// one, and its refusal is a sum of more than PrecisionW digits). It
// computes their exact
// sum in two words at the smaller exponent, the preferred one, and
// packWordW encodes it if it fits the format; a sum that does not fit, which
// needs rounding, falls through to the general path. That is the exact
// packing exception roundW's comment allows, and the general path gives the
// same encoding. It scales the operand with the larger exponent by
// k = min(gap, addDigitsW - digits) places and shifts the other only if
// k < gap; then the scaled operand has at least addDigitsW >= PrecisionW+3
// digits and the other is below 10^PrecisionW, so the sum or difference has
// more than PrecisionW digits and does not fit. When k = gap the general sum
// is this one, exact, at the same exponent, and roundW packs it unchanged.
// The sign follows the general path too: that of the operands when they
// agree, that of the larger magnitude when they differ, and for an exact
// cancellation the zero of "Sign of a result", negative only in
// TowardNegative.
//
// The compiler calls the decimal128 instantiation of this function
// directly, so its parameters and results are a contract with it: see the
// end of "Inline fast paths" in the package documentation.
func addSlowW(x, y coefW, mode Mode) coefW {
	var yneg, yok bool
	var yw uint64
	var ye int
	xneg, xw, xe, xok := unpackWordCanonW(x)
	if xok {
		yneg, yw, ye, yok = unpackWordCanonW(y)
	}
	var xc, yc coefW
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
			if r, ok := packWordW(neg, hi, lo, ye-BiasW); ok {
				return r
			}
		}
		xc, xe, yc, ye = cFromU64W(xw), xe-BiasW, cFromU64W(yw), ye-BiasW
	} else {
		xneg, xc, xe, xok = unpackFirstW(x)
		yneg, yc, ye, yok = unpackFirstW(y)
		if !xok || !yok {
			var xk, yk Kind
			xk, xneg, xc, xe = unpackRestW(x)
			yk, yneg, yc, ye = unpackRestW(y)
			if xk != KindFinite || yk != KindFinite {
				switch {
				case xk == KindNaN || yk == KindNaN:
					return nanW(x, y, y)
				case xk != KindInf:
					return cInfW(yneg)
				case yk == KindInf && xneg != yneg:
					return cNaNW() // the infinities cancel
				}
				return cInfW(xneg)
			}
		}
		if xe < ye {
			xneg, xc, xe, yneg, yc, ye = yneg, yc, ye, xneg, xc, xe
		}
	}

	exp, class := ye, classZero
	// A zero x has nothing to scale: the sum is y exactly, at the preferred
	// exponent ye, and shifting y would only lose digits of it.
	if gap := xe - ye; gap > 0 && !cIsZeroW(xc) {
		// A swamped y. Scaled by m places, x has exactly PrecisionW digits
		// X; if the gap exceeds m by more than the digits of a non-zero y,
		// y is a fraction f of one unit of X with 0 < f < 1/10, and the sum
		// is X + f or (X - 1) + (1 - f). Their classes are then known, below
		// a half and above it, and roundFaceW rounds them at face value with
		// a coefficient of PrecisionW digits (see its comment): it drops no
		// digit and divides nothing. X - 1 keeps PrecisionW digits unless X
		// is 10^(PrecisionW-1), whose difference would need padding, which a
		// face-value class does not survive. That X is taken one place
		// further instead, to 10^PrecisionW, if the gap leaves f below 1/10
		// there too, and the difference is 10^PrecisionW - 1 and 1 - 10f;
		// otherwise it takes the general path.
		//
		// The test needs no digit count of y: a non-zero y has fewer than
		// gap - m digits exactly when it is below 10^s with s = gap - m - 1,
		// which holds for no y when s <= 0 and for every y when s >=
		// PrecisionW, since y < 10^PrecisionW.
		dx := cDigitsW(xc)
		m := PrecisionW - dx
		if s := gap - m - 1; s > 0 && !cIsZeroW(yc) && (s >= PrecisionW || cLtW(yc, cPow10W(s))) {
			p, class := cMulPow10W(xc, m), classBelowHalf
			near := p // X: what both classes round to in a nearest mode
			if xneg != yneg {
				class = classAboveHalf
				if cEqW(p, cPow10W(PrecisionW-1)) {
					m, s = m+1, s-1
					p = cPow10W(PrecisionW)
					near = p
				}
				p, _ = cSubW(p, cPow10W(0))
			}
			if s > 0 && (s >= PrecisionW || cLtW(yc, cPow10W(s))) {
				// To nearest there is nothing to decide: X + f rounds
				// down to X and (X - 1) + (1 - f) up to X, whatever the
				// parity, and 10^PrecisionW is packFaceW's carry.
				if mode != TowardZero && mode != TowardPositive && mode != TowardNegative {
					return packFaceW(xneg, near, xe-m, mode)
				}
				return roundFaceW(xneg, p, xe-m, class, mode)
			}
		}
		// An aligned y. With x scaled to its PrecisionW digits X and y
		// shifted down to X's place as Y + f, the sum is X + Y + f and the
		// difference (X - Y - 1) + (1 - f) when f is not zero: one division
		// and one class. Whenever that coefficient still has exactly
		// PrecisionW digits it rounds at face value, as the swamped sum
		// does. A sum that carries to PrecisionW+1 digits goes to roundW
		// with the same class, which is still the class of everything
		// below it. A difference that loses its leading digit needs digits
		// of y this shift discarded, and takes the path below.
		if m := PrecisionW - dx; gap > m {
			p := cMulPow10W(xc, m)
			q, c := cShiftW(yc, gap-m)
			if xneg == yneg {
				p, _ = cAddW(p, q)
				if c != classZero && !cGtMaxW(p) {
					return roundFaceW(xneg, p, xe-m, c, mode)
				}
				r, _ := roundW(xneg, wideW{Lo: p}, xe-m, c, mode, MinExpW)
				return r
			}
			if c != classZero {
				q, c = cIncW(q), c.complement()
			}
			if p, _ = cSubW(p, q); !cLtW(p, cPow10W(PrecisionW-1)) {
				if c != classZero {
					return roundFaceW(xneg, p, xe-m, c, mode)
				}
				r, _ := roundW(xneg, wideW{Lo: p}, xe-m, c, mode, MinExpW)
				return r
			}
		}
		k := min(gap, addDigitsW-dx)
		xc = cMulPow10W(xc, k)
		if k < gap {
			exp = xe - k
			yc, class = cShiftW(yc, gap-k)
		}
	}

	neg := xneg
	var sum coefW
	if xneg == yneg {
		sum, _ = cAddW(xc, yc)
	} else {
		if class != classZero {
			yc, class = cIncW(yc), class.complement()
		}
		var borrow bool
		sum, borrow = cSubW(xc, yc)
		switch {
		case borrow:
			// Only an exact sum can borrow: a shifted operand is the smaller
			// by two places at least.
			var zero coefW
			sum, _ = cSubW(zero, sum)
			neg = !xneg
		case cIsZeroW(sum):
			neg = mode == TowardNegative // an exact cancellation
		}
	}
	r, _ := roundW(neg, wideW{Lo: sum}, exp, class, mode, MinExpW)
	return r
}

// roundFaceW returns (-1)^neg × (q + f) × 10^exp rounded in mode, for the
// swamped and the aligned addend of addSlowW and the inexact quotient of
// divW, its only callers: q has exactly PrecisionW digits, 10^(PrecisionW-1)
// <= q <= cMaxW(), and f is a fraction, not zero, whose class is known at
// face value (classBelowHalf or classAboveHalf for a swamped addend; an
// aligned addend's discarded digits and a quotient's remainder can also be
// exactly a half). It is the face-value exception to the rule that every
// rounded result passes through roundW; see "Face-value rounding" in the
// package documentation.
//
// Why it gives roundW's bits. The callers establish MinExpW < exp <=
// MaxExpW and an adjusted exponent exp + PrecisionW - 1 of at least EminW
// (addSlowW from its operands' exponents: exp = xe - m, or xe - m - 1 for
// the padded power of ten, with xe <= MaxExpW and exp > ye >= MinExpW,
// since the gap xe - ye exceeds m, by s + 1 with s > 0 for a swamped
// addend; divW by testing them before the call), so exp is at or above the
// floor MinExpW and the value is not tiny: nothing underflows.
// q has PrecisionW digits, so roundW's place of the result is exp itself:
// it pads nothing, discards no digit of q, and its class is the caller's.
// Its decision is then roundInc(class, mode, neg, odd(q)), the one row of
// the five-mode table this function takes, ties included: a swamped
// addend's class is never classHalf, the others' may be, and roundInc
// decides a tie as roundW would. The class is never classZero: roundInc
// takes an inexact value, and an exact one goes to roundW. An increment of
// cMaxW() carries to 10^PrecisionW, which
// roundW encodes as 10^(PrecisionW-1) one place up, and that place above
// MaxExpW overflows to overflowW's result. Every other step of roundW (the
// clamp, the digit count, the shift) leaves such a value unchanged.
// TestRoundFaceValue drives every bound here in every mode against the
// reference, TestSwampedAdd the classes around the swamping gap,
// TestAlignedAdd the aligned addend's classes, carries and cancellations,
// and the oracle's far and general regimes reach it in every mode.
func roundFaceW(neg bool, q coefW, exp int, class digitClass, mode Mode) coefW {
	// The increment is added, not branched on (roundInc); the carry out
	// of the largest coefficient is the branch, and it is rare.
	return packFaceW(neg, cAddU64W(q, roundInc(class, mode, neg, cOddW(q))), exp, mode)
}

// packFaceW returns (-1)^neg × q × 10^exp for a q that roundFaceW has
// rounded, or that divW has by biasing its dividend: PrecisionW digits, or
// 10^PrecisionW when the rounding carried out of the largest coefficient,
// which is 10^(PrecisionW-1) one place up, and above MaxExpW overflowW's
// result. The carry is the one branch of a face-value rounding, and rare.
func packFaceW(neg bool, q coefW, exp int, mode Mode) coefW {
	if cGtMaxW(q) {
		q, exp = cPow10W(PrecisionW-1), exp+1
		if exp > MaxExpW {
			r, _ := overflowW(neg, mode)
			return r
		}
	}
	return packW(neg, q, exp)
}
