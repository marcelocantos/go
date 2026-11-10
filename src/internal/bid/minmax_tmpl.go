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
	// mask and not by a branch.
	takeY := b2u(c > 0) | b2u(c == 0)&b2u(SignbitW(y))&^b2u(SignbitW(x))
	return resultW(cSelW(-takeY, x, y))
}

// maxW is MaxW: see "Minimum and maximum" in the package documentation.
func maxW(x, y coefW) coefW {
	c, ok := quietCmpSlowW(x, y)
	if !ok {
		return nanW(x, y, y)
	}
	takeY := b2u(c < 0) | b2u(c == 0)&b2u(SignbitW(x))&^b2u(SignbitW(y))
	return resultW(cSelW(-takeY, x, y))
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
