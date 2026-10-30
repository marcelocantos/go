// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zrem64.go and zrem128.go: the remainder
// family, Rem and Mod. See arith_tmpl.go for what the template form means;
// gen_arith.go generates both files from it.
package bid

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
func remainderW(x, y coefW, nearest bool) coefW {
	xk, neg, xc, xe := UnpackW(x)
	yk, _, yc, ye := UnpackW(y)
	if xk != KindFinite || yk == KindNaN || (yk == KindFinite && cIsZeroW(yc)) {
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
	if k > 0 || cCmpW(xc, Y) >= 0 {
		m, _ := cAddW(Y, Y)
		r = remScaledW(xc, k, m)
		if cCmpW(r, Y) >= 0 {
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
	r := cModFullW(cMulFullW(c, cPow10W(k%remStepW)), m)
	b := cPow10W(remStepW)
	for k /= remStepW; k > 0; k >>= 1 {
		if k&1 != 0 {
			r = cModFullW(cMulFullW(r, b), m)
		}
		b = cModFullW(cMulFullW(b, b), m)
	}
	return r
}

// remExactW encodes an exact result, which remainderW has shown fits.
func remExactW(neg bool, c coefW, exp int) coefW {
	z, _ := roundW(neg, cMulFullW(c, cPow10W(0)), exp, classZero, ToNearestEven, MinExpW)
	return z
}
