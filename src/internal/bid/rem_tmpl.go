// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zrem64.go and zrem128.go: the remainder
// family, Rem and Mod. See arith_tmpl.go for what the template form means;
// gen_arith.go generates both files from it.
package bid

import "math/bits"

// remStepW is the exponent of the largest power of ten a coefficient word
// holds, the stride of remScaledW. The product of two such words fits wideW.
const remStepW = len(pow10tabW) - 1

// remW is Rem for one width. See "Remainder and modulo" in the package
// documentation.
func remW(x, y coefW) coefW {
	return remainderW(x, y, true)
}

// modW is Mod for one width.
func modW(x, y coefW) coefW {
	return remainderW(x, y, false)
}

// remainderW computes x - y × n exactly, with n the quotient |x| / |y|
// truncated, or, if nearest is set, rounded to the nearest integer with ties
// to the even one.
//
// With e = min(Q(x), Q(y)), write |x| = X × 10^e and |y| = Y × 10^e. Only one
// of X and Y is scaled up, so the arithmetic is on integers:
//
//   - If Q(x) > Q(y), X = xc × 10^k with k up to the whole exponent range of
//     the format, more than 12000 digits for decimal128, and Y = yc. X is
//     never formed; remScaledW reduces it.
//   - Otherwise X = xc and Y = yc × 10^k. A Y that does not fit a word is more
//     than twice X, because 2 × 10^PrecisionW is below the word's range, so
//     n = 0 for both operations and the result is x itself. An infinite y is
//     the limit of the same case.
//
// The quotient's parity decides a tie and cannot be read from X mod Y.
// Reducing modulo 2Y gives it: if X = 2Y × m + R with 0 <= R < 2Y, then
// R < Y means ⌊X / Y⌋ = 2m with remainder R, and R >= Y means ⌊X / Y⌋ = 2m + 1
// with remainder R - Y. The modulus fits a word: whenever it is computed, Y is
// at most MaxCoeffW, either because Y = yc or because Y <= X = xc, and twice
// that is below the word's range. Y itself may exceed MaxCoeffW only when
// X < Y, which needs no reduction and is settled before the modulus is formed.
//
// The remainder r is below Y. Rounding the quotient up instead replaces it by
// Y - r on the other side of zero, which is the nearer one when 2r exceeds Y,
// or the even one when 2r equals Y and the truncated quotient is odd.
//
// Every result fits. Its magnitude M at exponent e is at most Y, since r < Y.
// It is also at most X: r is at most X, and Y - r is used only when 2r >= Y,
// so that Y - r <= r. Since X = xc when e = Q(x) and Y = yc when e = Q(y), M is
// at most a coefficient, and e is within the format, so roundW has nothing to
// round.
//
// The word path. Finite canonical operands whose coefficients fit a word
// each, and whose X and Y both fit one, are the amounts and counts this
// operation is mostly asked about (is 1234.56 a multiple of 0.05), and need
// one division instruction: the truncated quotient q = X / Y, its remainder
// r, and for the nearest quotient the comparison of r with Y - r, which is
// that of 2r with Y without the doubling. packWordW encodes the result,
// which the argument above shows fits, at the exponent e; that is the
// exact packing exception roundW's comment allows, and remExactW gives the
// same encoding. A zero divisor, and an X or a Y that leaves the word, take
// the general path: for decimal128 a Y beyond the word is not yet beyond
// twice X, as it is beyond the coefficient type.
func remainderW(x, y coefW, nearest bool) coefW {
	if xneg, X, xb, ok := unpackWordCanonW(x); ok {
		if _, Y, yb, ok := unpackWordCanonW(y); ok && Y != 0 {
			var hi uint64
			e := yb
			if k := uint(xb - yb); k < uint(len(pow10tab64)) {
				hi, X = bits.Mul64(X, pow10tab64[k])
			} else if k = uint(yb - xb); k < uint(len(pow10tab64)) {
				hi, Y = bits.Mul64(Y, pow10tab64[k])
				e = xb
			} else {
				hi = 1
			}
			if hi == 0 {
				q := X / Y
				r := X - q*Y
				if d := Y - r; nearest && (r > d || r == d && q&1 != 0) {
					r, xneg = d, !xneg
				}
				if z, ok := packWordW(xneg, 0, r, e-BiasW); ok {
					return z
				}
			}
		}
	}
	neg, xc, xe, xok := unpackFirstW(x)
	xk := KindFinite
	if !xok {
		xk, neg, xc, xe = unpackRestW(x)
	}
	_, yc, ye, yok := unpackFirstW(y)
	yk := KindFinite
	if !yok {
		yk, _, yc, ye = unpackRestW(y)
	}
	if xk == KindNaN || yk == KindNaN {
		return nanW(x, y, y)
	}
	if xk != KindFinite || (yk == KindFinite && cIsZeroW(yc)) {
		return cNaNW()
	}
	if yk == KindInf {
		return remExactW(neg, xc, xe)
	}

	k, Y := xe-ye, yc
	if k < 0 {
		scaled, overflow := cScaleW(yc, -k)
		if overflow {
			return remExactW(neg, xc, xe)
		}
		k, Y = 0, scaled
	}

	r, odd := xc, false
	if k > 0 || !cLtW(xc, Y) {
		m, _ := cAddW(Y, Y)
		r = remScaledW(xc, k, m)
		if !cLtW(r, Y) {
			r, _ = cSubW(r, Y)
			odd = true
		}
	}

	if nearest {
		twice, _ := cAddW(r, r)
		if c := cCmpW(twice, Y); c > 0 || (c == 0 && odd) {
			r, _ = cSubW(Y, r)
			neg = !neg
		}
	}
	return remExactW(neg, r, min(xe, ye))
}

// remScaledW returns c × 10^k mod m, by repeated squaring in the base
// 10^remStepW: one multiplication and division for a gap within one stride,
// and at most 2 × log2(k / remStepW) more beyond it, which is under twenty at
// the largest gap of decimal128. No intermediate value exceeds wideW, because
// every factor is a word.
//
// Precondition: m is not zero.
func remScaledW(c coefW, k int, m coefW) coefW {
	if k < remStepW {
		return cModFullW(cMulFullW(c, cPow10W(k)), m)
	}
	// Beyond one stride every reduction is by the one modulus, prepared
	// once: its reciprocals are part of every division. A gap within one
	// stride, the usual one, has the one division above and prepares
	// nothing. The residues are in the prepared modulus's own form from
	// here to the return.
	pm := cModPrepareW(m)
	r := cMulModW(cModEnterW(c, &pm), cModEnterW(cPow10W(k%remStepW), &pm), &pm)
	b := cModEnterW(cPow10W(remStepW), &pm)
	for k /= remStepW; ; {
		if k&1 != 0 {
			r = cMulModW(r, b, &pm)
		}
		// The square after the last bit would be computed and thrown
		// away, and the squares are the chain that sets the time.
		if k >>= 1; k == 0 {
			return cModLeaveW(r, &pm)
		}
		b = cMulModW(b, b, &pm)
	}
}

// remExactW encodes an exact result, which remainderW has shown fits.
func remExactW(neg bool, c coefW, exp int) coefW {
	z, _ := roundW(neg, cMulFullW(c, cPow10W(0)), exp, classZero, ToNearestEven, MinExpW)
	return z
}
