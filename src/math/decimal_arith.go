// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package math

// Decimal64Mod returns the floating-point remainder of x/y.
// The magnitude of the result is less than y and its sign agrees with that of x.
//
// Special cases are:
//
//	Decimal64Mod(±Inf, y) = NaN
//	Decimal64Mod(NaN, y) = NaN
//	Decimal64Mod(x, 0) = NaN
//	Decimal64Mod(x, ±Inf) = x
//	Decimal64Mod(x, NaN) = NaN
func Decimal64Mod(x, y decimal64) decimal64 {
	xb, yb := Decimal64bits(x), Decimal64bits(y)
	if d64IsNaN(xb) || d64IsNaN(yb) || d64IsInf(xb) || y == 0 {
		return Decimal64NaN()
	}
	if d64IsInf(yb) {
		return x
	}
	return x - Decimal64Trunc(x/y)*y
}

// Decimal64Remainder returns the IEEE 754 floating-point remainder of x/y.
//
// Special cases are:
//
//	Decimal64Remainder(±Inf, y) = NaN
//	Decimal64Remainder(NaN, y) = NaN
//	Decimal64Remainder(x, 0) = NaN
//	Decimal64Remainder(x, ±Inf) = x
//	Decimal64Remainder(x, NaN) = NaN
func Decimal64Remainder(x, y decimal64) decimal64 {
	xb, yb := Decimal64bits(x), Decimal64bits(y)
	if d64IsNaN(xb) || d64IsNaN(yb) || d64IsInf(xb) || y == 0 {
		return Decimal64NaN()
	}
	if d64IsInf(yb) {
		return x
	}
	return x - Decimal64RoundToEven(x/y)*y
}

// Decimal64FMA returns x * y + z, computed with only one rounding.
// (That is, Decimal64FMA returns the fused multiply-add of x, y, and z.)
func Decimal64FMA(x, y, z decimal64) decimal64 {
	xb, yb, zb := Decimal64bits(x), Decimal64bits(y), Decimal64bits(z)

	// Inf or NaN or zero involved. At most one rounding will occur.
	if x == 0 || y == 0 || d64IsSpecial(xb) || d64IsSpecial(yb) {
		return x*y + z
	}
	if z == 0 {
		return x * y
	}
	if d64IsSpecial(zb) {
		return z
	}

	// Widen to decimal128 for exact intermediate computation.
	// decimal64 has 16-digit coefficients; product has at most 32 digits,
	// which fits in decimal128's 34-digit precision.
	return decimal64(decimal128(x)*decimal128(y) + decimal128(z))
}

// Decimal64Quantize returns x rounded to the quantum (exponent) of y.
// The result has the same exponent as y and the value is the closest
// to x with that exponent, using round-to-nearest-even for ties.
//
// Special cases are:
//
//	Decimal64Quantize(NaN, y) = NaN
//	Decimal64Quantize(x, NaN) = NaN
//	Decimal64Quantize(±Inf, ±Inf) = ±Inf (preserving x's sign)
//	Decimal64Quantize(±Inf, y) = NaN (if y is finite)
//	Decimal64Quantize(x, ±Inf) = NaN (if x is finite)
func Decimal64Quantize(x, y decimal64) decimal64 {
	xb, yb := Decimal64bits(x), Decimal64bits(y)

	if d64IsNaN(xb) || d64IsNaN(yb) {
		return Decimal64NaN()
	}
	xInf := d64IsInf(xb)
	yInf := d64IsInf(yb)
	if xInf && yInf {
		return x
	}
	if xInf || yInf {
		return Decimal64NaN()
	}

	xSign, xExp, xCoeff := d64Unpack(xb)
	_, yExp, _ := d64Unpack(yb)

	if xCoeff == 0 {
		return d64Pack(xSign, yExp, 0)
	}
	if xExp == yExp {
		return x
	}

	if xExp < yExp {
		// Remove fractional digits: divide by 10^(yExp-xExp)
		diff := yExp - xExp
		if diff > 16 {
			return d64Pack(xSign, yExp, 0)
		}
		div := dpow10[diff]
		rem := xCoeff % div
		xCoeff /= div
		half := div / 2
		if rem > half || (rem == half && xCoeff%2 != 0) {
			xCoeff++
		}
	} else {
		// Add trailing zeros: multiply by 10^(xExp-yExp)
		diff := xExp - yExp
		if diff > 16 {
			return Decimal64NaN() // overflow
		}
		xCoeff *= dpow10[diff]
		if xCoeff > d64MaxCoeff {
			return Decimal64NaN() // overflow
		}
	}
	return d64Pack(xSign, yExp, xCoeff)
}

// Decimal64SameQuantum reports whether x and y have the same quantum
// (exponent). Both NaN values are considered to have the same quantum.
// Both infinities are considered to have the same quantum.
func Decimal64SameQuantum(x, y decimal64) bool {
	xb, yb := Decimal64bits(x), Decimal64bits(y)
	xNaN, yNaN := d64IsNaN(xb), d64IsNaN(yb)
	if xNaN && yNaN {
		return true
	}
	if xNaN || yNaN {
		return false
	}
	xInf, yInf := d64IsInf(xb), d64IsInf(yb)
	if xInf && yInf {
		return true
	}
	if xInf || yInf {
		return false
	}
	_, xExp, _ := d64Unpack(xb)
	_, yExp, _ := d64Unpack(yb)
	return xExp == yExp
}

// --- decimal128 versions ---

// Decimal128Mod returns the floating-point remainder of x/y.
//
// Special cases are:
//
//	Decimal128Mod(±Inf, y) = NaN
//	Decimal128Mod(NaN, y) = NaN
//	Decimal128Mod(x, 0) = NaN
//	Decimal128Mod(x, ±Inf) = x
//	Decimal128Mod(x, NaN) = NaN
func Decimal128Mod(x, y decimal128) decimal128 {
	xhi, _ := Decimal128bits(x)
	yhi, _ := Decimal128bits(y)
	if d128IsNaN(xhi) || d128IsNaN(yhi) || d128IsInf(xhi) || y == 0 {
		return Decimal128frombits(d128NaN, 0)
	}
	if d128IsInf(yhi) {
		return x
	}
	return x - Decimal128Trunc(x/y)*y
}

// Decimal128Remainder returns the IEEE 754 floating-point remainder of x/y.
//
// Special cases are:
//
//	Decimal128Remainder(±Inf, y) = NaN
//	Decimal128Remainder(NaN, y) = NaN
//	Decimal128Remainder(x, 0) = NaN
//	Decimal128Remainder(x, ±Inf) = x
//	Decimal128Remainder(x, NaN) = NaN
func Decimal128Remainder(x, y decimal128) decimal128 {
	xhi, _ := Decimal128bits(x)
	yhi, _ := Decimal128bits(y)
	if d128IsNaN(xhi) || d128IsNaN(yhi) || d128IsInf(xhi) || y == 0 {
		return Decimal128frombits(d128NaN, 0)
	}
	if d128IsInf(yhi) {
		return x
	}
	return x - Decimal128RoundToEven(x/y)*y
}

// Decimal128FMA returns x * y + z, computed with only one rounding.
// (That is, Decimal128FMA returns the fused multiply-add of x, y, and z.)
//
// Note: this implementation is not truly fused for decimal128; it may
// involve two roundings. A full fused implementation requires 68-digit
// intermediate arithmetic.
func Decimal128FMA(x, y, z decimal128) decimal128 {
	xhi, yhi, zhi := func() (uint64, uint64, uint64) {
		xh, _ := Decimal128bits(x)
		yh, _ := Decimal128bits(y)
		zh, _ := Decimal128bits(z)
		return xh, yh, zh
	}()

	if x == 0 || y == 0 || d128IsSpecial(xhi) || d128IsSpecial(yhi) {
		return x*y + z
	}
	if z == 0 {
		return x * y
	}
	if d128IsSpecial(zhi) {
		return z
	}
	return x*y + z
}

// Decimal128Quantize returns x rounded to the quantum (exponent) of y.
//
// Special cases are:
//
//	Decimal128Quantize(NaN, y) = NaN
//	Decimal128Quantize(x, NaN) = NaN
//	Decimal128Quantize(±Inf, ±Inf) = ±Inf
//	Decimal128Quantize(±Inf, y) = NaN (if y is finite)
//	Decimal128Quantize(x, ±Inf) = NaN (if x is finite)
func Decimal128Quantize(x, y decimal128) decimal128 {
	xhi, xlo := Decimal128bits(x)
	yhi, ylo := Decimal128bits(y)

	if d128IsNaN(xhi) || d128IsNaN(yhi) {
		return Decimal128frombits(d128NaN, 0)
	}
	xInf := d128IsInf(xhi)
	yInf := d128IsInf(yhi)
	if xInf && yInf {
		return x
	}
	if xInf || yInf {
		return Decimal128frombits(d128NaN, 0)
	}

	xSign, xExp, xCoeff := d128Unpack(xhi, xlo)
	_, yExp, _ := d128Unpack(yhi, ylo)

	if dU128IsZero(xCoeff) {
		return d128Pack(xSign, yExp, dU128Zero)
	}
	if xExp == yExp {
		return x
	}

	if xExp < yExp {
		diff := yExp - xExp
		if diff > 34 {
			return d128Pack(xSign, yExp, dU128Zero)
		}
		// Divide coefficient by 10^diff
		var q, rem dUint128
		if diff <= 19 {
			var r uint64
			q, r = dU128Div64(xCoeff, dpow10[diff])
			rem = dU128From64(r)
		} else {
			var r1 uint64
			q, r1 = dU128Div64(xCoeff, dpow10[19])
			var r2 uint64
			q, r2 = dU128Div64(q, dpow10[diff-19])
			rem = dU128Add(dU128Mul64(dU128From64(r2), dpow10[19]), dU128From64(r1))
		}
		// Round-to-nearest-even
		var half dUint128
		if diff <= 19 {
			half = dU128From64(dpow10[diff] / 2)
		} else {
			half = dU128Mul64(dUpow10[diff-1], 5)
		}
		cmp := dU128Cmp(rem, half)
		if cmp > 0 || (cmp == 0 && q.lo%2 != 0) {
			q = dU128Add(q, dU128From64(1))
		}
		xCoeff = q
	} else {
		diff := xExp - yExp
		if diff > 34 {
			return Decimal128frombits(d128NaN, 0)
		}
		for i := 0; i < diff; i++ {
			xCoeff = dU128Mul64(xCoeff, 10)
		}
		if dU128Cmp(xCoeff, d128MaxCoeff) > 0 {
			return Decimal128frombits(d128NaN, 0)
		}
	}
	return d128Pack(xSign, yExp, xCoeff)
}

// Decimal128SameQuantum reports whether x and y have the same quantum.
func Decimal128SameQuantum(x, y decimal128) bool {
	xhi, xlo := Decimal128bits(x)
	yhi, ylo := Decimal128bits(y)
	xNaN, yNaN := d128IsNaN(xhi), d128IsNaN(yhi)
	if xNaN && yNaN {
		return true
	}
	if xNaN || yNaN {
		return false
	}
	xInf, yInf := d128IsInf(xhi), d128IsInf(yhi)
	if xInf && yInf {
		return true
	}
	if xInf || yInf {
		return false
	}
	_, xExp, _ := d128Unpack(xhi, xlo)
	_, yExp, _ := d128Unpack(yhi, ylo)
	return xExp == yExp
}
