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
