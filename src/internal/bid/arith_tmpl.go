// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zarith64.go and zarith128.go, written once
// in the vocabulary of "Width parameterisation" in the package
// documentation: every identifier that depends on the width ends in a
// capital W, and the generator (gen_arith.go) replaces that W with 64 or
// 128. Nothing else is substituted. This file is not compiled; the build
// ignores it, and only the generated files are compiled. Everything from
// the build constraint to the package clause is commentary on the template
// and is dropped from the generated files, which nobody edits.
//
// Nothing below the package clause, in code or in a comment, may end an
// identifier with that capital letter unless it is meant to be replaced.
package bid

// mulReduceW multiplies two coefficients and reduces the exact product to
// at most PrecisionW+1 digits, the first step of a multiplication. The
// returned q is ⌊a × b / 10^k⌋, k is the number of digits dropped (the
// amount by which the exponent of the product rises), and sticky reports
// whether any dropped digit was non-zero. One digit beyond the precision is
// kept so that a single later rounding sees the digit that decides it and,
// in sticky, whether anything lies below. Nothing is rounded here.
//
// Precondition: a and b are at most MaxCoeffW (a decimal coefficient).
func mulReduceW(a, b coefW) (q coefW, k int, sticky bool) {
	p := cMulFullW(a, b)
	k = max(cLenFullW(p)-(PrecisionW+1), 0)
	q, sticky = cReduceW(p, k)
	return q, k, sticky
}

// magCmpW compares the magnitudes of two non-zero finite values, returning
// -1, 0 or +1. The values are decided by adjusted exponent first: a value
// whose leading digit lies in a higher decimal place is the larger, whatever
// the coefficients, so values whose exponents differ by thousands of places
// are compared without scaling anything. Only when the adjusted exponents
// are equal are the coefficients aligned, and then the coefficient with the
// larger exponent has fewer digits than the other by exactly the difference
// of the exponents; scaling it up gives it the digit count of the other,
// which is at most PrecisionW, so the product cannot overflow and the
// difference of the exponents is below the table of powers of ten.
//
// Precondition: xc and yc are non-zero and at most MaxCoeffW.
func magCmpW(xc coefW, xe int, yc coefW, ye int) int {
	if xe == ye {
		return cCmpW(xc, yc)
	}
	// xa and ya are one more than the adjusted exponents.
	xa, ya := cDigitsW(xc)+xe, cDigitsW(yc)+ye
	switch {
	case xa < ya:
		return -1
	case xa > ya:
		return 1
	case xe > ye:
		return cCmpW(cMulPow10W(xc, xe-ye), yc)
	}
	return cCmpW(xc, cMulPow10W(yc, ye-xe))
}

// finiteCmpW compares two finite values, returning -1, 0 or +1: a zero of
// either sign is below every positive value and above every negative one,
// and equal to every zero.
func finiteCmpW(xneg bool, xc coefW, xe int, yneg bool, yc coefW, ye int) int {
	xz, yz := cIsZeroW(xc), cIsZeroW(yc)
	switch {
	case xz && yz:
		return 0
	case xz:
		if yneg {
			return 1
		}
		return -1
	case yz:
		if xneg {
			return -1
		}
		return 1
	case xneg != yneg:
		if xneg {
			return -1
		}
		return 1
	case xneg:
		return -magCmpW(xc, xe, yc, ye)
	}
	return magCmpW(xc, xe, yc, ye)
}

// quietCmpW returns the ordering of x and y as -1, 0 or +1 with ok = true,
// or 0 and ok = false when either is a NaN.
func quietCmpW(x, y coefW) (c int, ok bool) {
	xk, xneg, xc, xe := UnpackW(x)
	yk, yneg, yc, ye := UnpackW(y)
	switch {
	case xk == KindNaN || yk == KindNaN:
		return 0, false
	case xk == KindFinite && yk == KindFinite:
		return finiteCmpW(xneg, xc, xe, yneg, yc, ye), true
	case xk == KindInf && yk == KindInf:
		// Two infinities: equal if of one sign, else the negative is the lesser.
		if xneg == yneg {
			return 0, true
		}
		if xneg {
			return -1, true
		}
		return 1, true
	case xk == KindInf:
		// An infinity against a finite value, finite zero included.
		if xneg {
			return -1, true
		}
		return 1, true
	}
	if yneg {
		return 1, true
	}
	return -1, true
}

// equalW reports whether x == y by value; it is false if either is a NaN.
func equalW(x, y coefW) bool {
	c, ok := quietCmpW(x, y)
	return ok && c == 0
}

// lessW reports whether x < y; it is false if either is a NaN.
func lessW(x, y coefW) bool {
	c, ok := quietCmpW(x, y)
	return ok && c < 0
}

// lessEqualW reports whether x <= y; it is false if either is a NaN.
func lessEqualW(x, y coefW) bool {
	c, ok := quietCmpW(x, y)
	return ok && c <= 0
}

// totalOrderW reports whether x precedes or equals y in the total order of
// IEEE 754-2008 clause 5.10 as the package defines it: by sign first
// (negative operands, NaNs included, precede positive ones), then, within
// one sign, finite values by value, then infinities, then NaNs; two finite
// values of equal value, zeros included, by exponent. For negative operands
// everything within the sign reverses. All NaNs of one sign are equal.
func totalOrderW(x, y coefW) bool {
	xk, xneg, xc, xe := UnpackW(x)
	yk, yneg, yc, ye := UnpackW(y)
	if xneg != yneg {
		return xneg
	}
	// Order the magnitudes: c is -1, 0 or +1 as |x| precedes, equals or
	// follows |y|, which for positive operands is the order of the values.
	c := 0
	switch {
	case xk != yk:
		// Kind orders finite, infinity, NaN.
		c = -1
		if xk > yk {
			c = 1
		}
	case xk != KindFinite:
		// Infinities of one sign are equal, as are NaNs.
	default:
		xz, yz := cIsZeroW(xc), cIsZeroW(yc)
		switch {
		case xz && yz:
		case xz:
			c = -1
		case yz:
			c = 1
		default:
			c = magCmpW(xc, xe, yc, ye)
		}
		if c == 0 {
			// One value, two quanta: the smaller exponent first.
			switch {
			case xe < ye:
				c = -1
			case xe > ye:
				c = 1
			}
		}
	}
	if xneg {
		return c >= 0
	}
	return c <= 0
}

// keyW returns the fields of the hash key of x. See Key.
func keyW(x coefW) (kind Kind, neg bool, c coefW, exp int) {
	kind, neg, c, exp = UnpackW(x)
	if kind == KindNaN {
		return KindNaN, false, c, 0
	}
	if kind == KindInf {
		return KindInf, neg, c, 0
	}
	if cIsZeroW(c) {
		return KindFinite, false, c, 0
	}
	// Strip the trailing zeros of the coefficient by binary search on their
	// number: at most PrecisionW-1 of them, so steps of the powers of two
	// from the largest one not above PrecisionW down to 1 can remove any
	// count. Most coefficients have none, which one division settles.
	q, r := cDivPow10W(c, 1)
	if !cIsZeroW(r) {
		return kind, neg, c, exp
	}
	c, exp = q, exp+1
	step := 1
	for step*2 <= PrecisionW {
		step *= 2
	}
	for ; step > 0; step /= 2 {
		if q, r := cDivPow10W(c, step); cIsZeroW(r) {
			c, exp = q, exp+step
		}
	}
	return kind, neg, c, exp
}

// roundW is the rounding routine of the package: the one function that
// turns an exact result into an encoding, under each of the five modes,
// and the only way any operation produces a rounded result. Every rule of
// "Exponent of a result", "Overflow", "Underflow and subnormal results"
// and "Sign of a result" (for zeros) in the package documentation is
// implemented here and nowhere else. The width-independent direction
// decision is roundUp in round.go; the rest is this function.
//
// The input is the exact value
//
//	v = (-1)^neg × (p + f) × 10^exp
//
// with p a wide integer (the full product of two coefficients fits it) and
// f a fraction, 0 <= f < 1, described only by class: classZero for f = 0,
// otherwise where f lies relative to one half. floor is the least exponent
// the result may take: MinExpW for every operation with a preferred
// exponent (exp is then that preferred exponent), and the fixed exponent
// itself, equal to exp, for Quantize and RoundToIntegral, whose result
// exponent is fixed by the operation rather than by the precision; such a
// caller treats a result whose exponent is not exp as not fitting
// (Quantize returns NaN, RoundToIntegral keeps the carried value).
//
// What a caller must supply for the result to be correctly rounded:
//
//   - p and class together describe v exactly. A caller holding more
//     digits than p can carry (FMA, a quotient generated beyond the
//     precision, a sum whose operands are far apart) reduces them with the
//     digit primitives, cShiftFullW or cReduceClassW, and passes the class
//     they return as class; two reductions compose as digits.go says.
//   - If class is not classZero and stands for a fraction of unknown size,
//     p must have more than PrecisionW digits. roundW then discards at
//     least one digit of p itself, and withSticky combines the two
//     discarded parts exactly. A caller meets this by keeping PrecisionW+1
//     digits or more (mulReduceW keeps exactly that), or by scaling its
//     exact part up to that many digits before truncating the rest.
//   - If class is not classZero and p has PrecisionW digits or fewer,
//     the result is that of rounding p + ε for an arbitrarily small ε > 0
//     ("Rounding from exact parts"): the fraction is taken to be below
//     every rounding boundary, whatever class says beyond being non-zero.
//     Only RoundW and RoundW's callers mean this; an operation never does.
//     A p of zero with a non-zero class is such a value below 10^exp, and
//     a caller meaning "arbitrarily small" passes exp <= MinExpW with it,
//     as roundPartsW does, so that it rounds at MinExpW.
//
// The result exponent x is exp for an exact value, raised when p has more
// than PrecisionW digits, when exp is below floor, or when exp is above
// MaxExpW and the coefficient can be padded with zeros to make up the
// difference (fold-down); if it cannot, the value overflows. An inexact
// value is encoded at the least exponent at or above floor at which its
// rounded coefficient has at most PrecisionW digits, so p is padded or
// shifted to that place before the increment, and a coefficient that
// carries out to 10^PrecisionW becomes 10^(PrecisionW-1) one place up. An
// exact zero takes exp clamped into [floor, MaxExpW] and keeps the sign.
//
// Status: Exact when the result is v, including an exact subnormal, an
// exact fold-down and a zero; Inexact otherwise; Inexact|Underflow when,
// in addition, v is non-zero and below 10^EminW (tininess before
// rounding); Inexact|Overflow when the rounded value is above the largest
// finite one, in which case the result is the infinity or the largest
// finite value of sign neg that overflowsToInf selects. The arithmetic
// wrappers discard the Status; it is meaningful to RoundW, to the parser
// and to the integer conversions, which report it.
//
// Exact fast paths. PackW is the raw encoder; it rounds nothing and
// refuses what does not fit. No function other than roundW may call it,
// unless it re-encodes a coefficient and exponent already known to be
// representable because UnpackW returned them, possibly with another
// sign, and is named both here and in the allowlist of
// TestRoundIsTheOnlyPacker, which parses the package source. At present
// there is no such function.
func roundW(neg bool, p wideW, exp int, class digitClass, mode Mode, floor int) (coefW, Status) {
	if p == (wideW{}) && class == classZero {
		var zero coefW
		return packW(neg, zero, min(max(exp, floor), MaxExpW)), Exact
	}
	// Far outside the format every exponent rounds alike: overflow in every
	// mode above, a value below a tenth of the smallest subnormal below.
	// Clamping keeps the exponent arithmetic below in range and the shift
	// counts within what the digit primitives are specified for.
	exp = min(max(exp, MinExpW-maxWideDigits-2), MaxExpW+PrecisionW+2)

	// a is the adjusted exponent of v, the power of ten of its leading
	// digit, and tiny whether v is below 10^EminW. For p = 0, v is below
	// 10^exp, which is what a = exp - 1 says.
	d := cLenFullW(p)
	a := d - 1 + exp
	tiny := a < EminW

	// x is the exponent of an inexact result: the least, at or above floor,
	// at which PrecisionW digits hold the value. s is the number of places
	// p moves to reach it: left (padding) when positive, right (dropping
	// digits, which can make an exact value inexact) when negative.
	x := max(a-(PrecisionW-1), floor)
	s := exp - x
	var q coefW
	if s < 0 {
		var dropped digitClass
		q, dropped = cReduceClassW(p, -s)
		class = dropped.withSticky(class != classZero)
	} else {
		q = cLoW(p)
	}

	if class == classZero {
		// Exact: the member of the cohort whose exponent is closest to exp.
		// Dropping digits raised it to x; otherwise it is exp, folded down
		// to MaxExpW if the coefficient has room for the zeros.
		if s < 0 {
			exp = x
		}
		if exp > MaxExpW {
			padded, overflow := cScaleW(q, exp-MaxExpW)
			if overflow || cGtMaxW(padded) {
				return overflowW(neg, mode)
			}
			q, exp = padded, MaxExpW
		}
		return packW(neg, q, exp), Exact
	}

	// Inexact: pad to the result's place, then increment or not.
	if s > 0 {
		q = cMulPow10W(q, s)
	}
	if roundUp(class, mode, neg, cOddW(q)) {
		q = cIncW(q)
		if cGtMaxW(q) {
			q, x = cPow10W(PrecisionW-1), x+1
		}
	}
	if x > MaxExpW {
		return overflowW(neg, mode)
	}
	st := Inexact
	if tiny {
		st |= Underflow
	}
	return packW(neg, q, x), st
}

// overflowW returns the default result of an overflow of sign neg in mode
// and its Status (IEEE 754-2008 clause 7.4; "Overflow" in the package
// documentation).
func overflowW(neg bool, mode Mode) (coefW, Status) {
	if overflowsToInf(mode, neg) {
		return cInfW(neg), Inexact | Overflow
	}
	return packW(neg, cMaxW(), MaxExpW), Inexact | Overflow
}

// packW is PackW for a coefficient and exponent that roundW has brought
// within the format. PackW refusing them is a bug in roundW, not an input
// condition, and panics rather than yielding a NaN.
func packW(neg bool, c coefW, exp int) coefW {
	x, ok := PackW(neg, c, exp)
	if !ok {
		panic(errRoundRange)
	}
	return x
}

// roundPartsW is RoundW: the sticky bit stands for an arbitrarily small
// fraction, which is classBelowHalf when p has PrecisionW digits or fewer
// and acts as a sticky bit when roundW drops digits of p (see roundW). A
// zero p with the bit set is a value below every subnormal, so it is
// placed at MinExpW, where it rounds to the zero or the smallest
// subnormal of its sign.
func roundPartsW(neg bool, p wideW, exp int, sticky bool, mode Mode) (coefW, Status) {
	class := classZero
	if sticky {
		class = classBelowHalf
		if p == (wideW{}) {
			exp = min(exp, MinExpW)
		}
	}
	return roundW(neg, p, exp, class, mode, MinExpW)
}
