// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zquant64.go and zquant128.go: the
// operations that fix or inspect the exponent of a result rather than
// compute a new value, namely RoundToIntegral, Quantize, Reduce,
// SameQuantum and Quantum. It is written in the vocabulary of "Width parameterisation"
// in the package documentation (see arith_tmpl.go for the rules). Nothing
// here rounds by itself: the first two hand their bounded coefficient to roundFiniteW,
// with its floor parameter set to the fixed exponent of the result, which is
// why they need no second rounding routine.
package bid

import "math/bits"

// roundToIntegralW rounds the decimal x to an integral value in mode. The
// result exponent is max(Q(x), 0) whether or not a fraction was discarded
// (see "Exponent of a result"), which is exactly what roundFiniteW produces with
// floor 0: for Q(x) < 0 the value has fewer than PrecisionW integer digits,
// so the least exponent at which the coefficient fits is the floor, and the
// digits below it are dropped and rounded; for Q(x) >= 0 nothing is
// dropped and the exact value is re-encoded at its own exponent, which also
// canonicalizes a non-canonical finite encoding. A zero keeps its sign and
// takes exponent max(Q(x), 0). A NaN gives that NaN made quiet and an
// infinity is returned unchanged.
func roundToIntegralW(x coefW, mode Mode) coefW {
	neg, c, exp, xok := unpackFirstW(x)
	kind := KindFinite
	if !xok {
		kind, neg, c, exp = unpackRestW(x)
	}
	switch kind {
	case KindNaN:
		return cQuietW(x)
	case KindInf:
		return cInfW(neg)
	}
	r, _ := roundFiniteW(neg, c, exp, mode, 0)
	return r
}

// quantizeW returns x with the exponent of y, rounded in mode. A word x
// with the exponent of y is x itself, decided inline by quantizeSameW, and
// every other operand takes quantizeSlowW; see "Inline fast paths" in the
// package documentation.
func quantizeW(x, y coefW, mode Mode) coefW {
	if quantizeSameW(x, y) {
		return x
	}
	return quantizeSlowW(x, y, mode)
}

// quantizeSlowW returns x with the exponent of y, rounded in mode, or the
// canonical NaN when the coefficient at that exponent has more than
// PrecisionW digits. The exponent of y is in range, so a finite result
// never needs its exponent adjusted.
//
// When x has the larger exponent its coefficient is scaled up by the
// difference; roundFiniteW has nothing to round then (a coefficient padded to
// PrecisionW digits or fewer at the exponent of y is already the result)
// and the shared rounding helper encodes the result. When x has the smaller
// exponent, roundFiniteW drops the digits below the exponent of y, with floor
// that exponent. Dropping at least one digit leaves at most PrecisionW-1
// digits, so rounding up cannot carry out of the coefficient, and
// an x so small that every digit is dropped rounds to zero or to one unit,
// whatever the distance. An exact zero takes the exponent of y.
//
// A finite x whose coefficient fits a word, with an exponent at or above
// that of a finite y by less than the table of powers, takes a fast path
// first: the coefficient scaled to the exponent of y, exactly, by one word
// multiply across a gap of at most maxWordGap places (the scaled value
// always fits the two words) and by cScaleTabW beyond that, and packWordW
// encodes it if it fits the format, which is the exact packing exception
// roundW's comment allows. The general path computes the same scaled
// coefficient and roundFiniteW, with nothing to drop, encodes it unchanged,
// a zero with the sign of x included. A coefficient that does not fit is
// the NaN of the general path, which the fast path returns itself: ye is
// the exponent of a finite operand, so packWordW refuses nothing else, and
// a word multiply that overflows two words cannot happen. A word x below
// the exponent of y, which needs rounding, or beyond the table above it,
// takes the general path from x as the fast path decoded it.
func quantizeSlowW(x, y coefW, mode Mode) coefW {
	xneg, xw, xe, xok := unpackWordCanonW(x)
	yk, ye := kindExpW(y)
	var xc coefW
	xk := KindFinite
	if xok {
		xe -= BiasW
		if gap := xe - ye; gap >= 0 && gap < len(pow10tabW) && yk == KindFinite {
			if uint(gap) <= maxWordGap {
				hi, lo := bits.Mul64(xw, pow10tab64[gap])
				if r, ok := packWordW(xneg, hi, lo, ye); ok {
					return r
				}
			} else if s, overflow := cScaleTabW(cFromU64W(xw), gap); !overflow {
				hi, lo := cWordsW(s)
				if r, ok := packWordW(xneg, hi, lo, ye); ok {
					return r
				}
			}
			return cNaNW()
		}
		xc = cFromU64W(xw)
	} else {
		xneg, xc, xe, xok = unpackFirstW(x)
		if !xok {
			xk, xneg, xc, xe = unpackRestW(x)
		}
	}
	switch {
	case xk == KindNaN || yk == KindNaN:
		return nanW(x, y, y)
	case xk == KindInf && yk == KindInf:
		return cInfW(xneg)
	case xk == KindInf || yk == KindInf:
		return cNaNW()
	}
	if gap := xe - ye; gap > 0 {
		// cScaleW's bound, tested here so that the scale inlines (see
		// cScaleTabW): beyond the table every non-zero coefficient
		// overflows, and a zero stays a zero.
		var overflow bool
		if gap < len(pow10tabW) {
			xc, overflow = cScaleTabW(xc, gap)
		} else {
			overflow = !cIsZeroW(xc)
		}
		if overflow || cGtMaxW(xc) {
			return cNaNW()
		}
		xe = ye
	}
	r, _ := roundFiniteW(xneg, xc, xe, mode, ye)
	return r
}

// reduceW returns the member of the cohort of x with the fewest coefficient
// digits: the trailing zeros of the coefficient are removed and the exponent
// raised to match, as far as MaxExpW allows. A zero becomes a zero of its
// sign with exponent 0. A NaN gives that NaN made quiet and an infinity is
// returned unchanged.
func reduceW(x coefW) coefW {
	neg, c, exp, xok := unpackFirstW(x)
	kind := KindFinite
	if !xok {
		kind, neg, c, exp = unpackRestW(x)
	}
	switch kind {
	case KindNaN:
		return cQuietW(x)
	case KindInf:
		return cInfW(neg)
	}
	if cIsZeroW(c) {
		exp = 0
	} else if q, ok := cDivExactPow10W(c, 1); ok && exp < MaxExpW {
		// Strip the trailing zeros by binary search on their number, as keyW
		// does, but never past MaxExpW. The number to strip, n, is at most
		// PrecisionW-1, below twice the first step, and each step takes a
		// power of two that n still contains: it can be removed exactly when
		// the coefficient divides by 10^step and the exponent has room.
		c, exp = q, exp+1
		step := 1
		for step*2 <= PrecisionW {
			step *= 2
		}
		for ; step > 0; step /= 2 {
			if exp+step > MaxExpW {
				continue
			}
			if q, ok := cDivExactPow10W(c, step); ok {
				c, exp = q, exp+step
			}
		}
	}
	r, _ := roundW(neg, wideW{Lo: c}, exp, classZero, ToNearestEven, MinExpW)
	return r
}

// sameQuantumW reports whether x and y are both finite with one exponent,
// both infinities or both NaNs.
func sameQuantumW(x, y coefW) bool {
	xk, xe := kindExpW(x)
	yk, ye := kindExpW(y)
	return xk == yk && (xk != KindFinite || xe == ye)
}

// quantumW returns the quantum of x: for a finite x the positive value
// 1 × 10^Q(x), whose exponent is Q(x) and which is therefore always
// representable; for an infinity of either sign +∞; for a NaN that NaN made
// quiet. A non-canonical finite encoding reads as a zero with its encoded
// exponent, so its quantum is that of the zero.
func quantumW(x coefW) coefW {
	kind, exp := kindExpW(x)
	switch kind {
	case KindNaN:
		return cQuietW(x)
	case KindInf:
		return cInfW(false)
	}
	r, _ := roundW(false, wideW{Lo: cPow10W(0)}, exp, classZero, ToNearestEven, MinExpW)
	return r
}
