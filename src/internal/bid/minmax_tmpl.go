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
	c, ok := quietCmpW(x, y)
	switch {
	case !ok:
		return cNaNW()
	case c > 0, c == 0 && SignbitW(y) && !SignbitW(x):
		// Operands of equal value and unlike sign can only be zeros, and
		// -0 is the lesser.
		return canonicalW(y)
	}
	return canonicalW(x)
}

// maxW is MaxW: see "Minimum and maximum" in the package documentation.
func maxW(x, y coefW) coefW {
	c, ok := quietCmpW(x, y)
	switch {
	case !ok:
		return cNaNW()
	case c < 0, c == 0 && SignbitW(x) && !SignbitW(y):
		return canonicalW(y)
	}
	return canonicalW(x)
}

// canonicalW returns the canonical encoding of the non-NaN value x, which
// keeps its coefficient and exponent, a non-canonical encoding reading as
// the zero Unpack gives. Rounding an exactly representable value changes
// nothing, so the mode is immaterial.
func canonicalW(x coefW) coefW {
	k, neg, c, e := UnpackW(x)
	if k == KindInf {
		return cInfW(neg)
	}
	r, _ := roundW(neg, wideW{Lo: c}, e, classZero, ToNearestEven, MinExpW)
	return r
}
