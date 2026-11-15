// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zminmax64.go and zminmax128.go; see the
// comment in arith_tmpl.go and "Width parameterisation" in the package
// documentation.
package bid

// minW is MinW: see "Minimum and maximum" in the package documentation.
func minW(x, y coefW) coefW {
	c, ok := quietCmpSlowW(x, y)
	if !ok {
		return nanW(x, y, y)
	}
	// y if it is the lesser, or equal with the negative sign: operands of
	// equal value and unlike sign can only be zeros, and -0 is the lesser.
	// Which operand that is lies in their values, so it is chosen by a
	// mask and not by a branch; members of one cohort, rarer, by one.
	takeY := b2u(c > 0) | b2u(c == 0)&b2u(SignbitW(y))&^b2u(SignbitW(x))
	// Of two members of one cohort, the one totalOrder puts first.
	if c == 0 && SignbitW(x) == SignbitW(y) && totalBeforeW(y, x) {
		takeY = 1
	}
	return resultW(cSelW(-takeY, x, y))
}

// maxW is MaxW: see "Minimum and maximum" in the package documentation.
func maxW(x, y coefW) coefW {
	c, ok := quietCmpSlowW(x, y)
	if !ok {
		return nanW(x, y, y)
	}
	takeY := b2u(c < 0) | b2u(c == 0)&b2u(SignbitW(x))&^b2u(SignbitW(y))
	// Of two members of one cohort, the one totalOrder puts last.
	if c == 0 && SignbitW(x) == SignbitW(y) && totalBeforeW(x, y) {
		takeY = 1
	}
	return resultW(cSelW(-takeY, x, y))
}

// totalBeforeW reports, for operands a and b of equal value and sign, whether
// IEEE 754-2019's totalOrder (clause 5.10) puts a strictly before b: for
// positive values the one with the smaller exponent, for negative values the
// one with the larger. Two infinities of one sign are not ordered.
func totalBeforeW(a, b coefW) bool {
	ea, oka := exponentOfW(a)
	eb, okb := exponentOfW(b)
	if !oka || !okb {
		return false
	}
	if SignbitW(a) {
		return ea > eb
	}
	return ea < eb
}

// exponentOfW returns the exponent of the finite x, a non-canonical
// encoding reading as the zero Unpack gives, and false for an infinity.
func exponentOfW(x coefW) (int, bool) {
	if _, _, e, ok := unpackFirstW(x); ok {
		return e, true
	}
	k, _, _, e := unpackRestW(x)
	return e, k == KindFinite
}

// resultW returns the canonical encoding of the non-NaN value x, which
// keeps its coefficient and exponent, a non-canonical encoding reading as
// the zero Unpack gives. Rounding an exactly representable value changes
// nothing, so the mode is immaterial.
func resultW(x coefW) coefW {
	neg, c, e, xok := unpackFirstW(x)
	k := KindFinite
	if !xok {
		k, neg, c, e = unpackRestW(x)
	}
	if k == KindInf {
		return cInfW(neg)
	}
	r, _ := roundW(neg, wideW{Lo: c}, e, classZero, ToNearestEven, MinExpW)
	return r
}
