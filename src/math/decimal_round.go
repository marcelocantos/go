// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package math

// Decimal64Trunc returns the integer value of d.
//
// Special cases are:
//
//	Decimal64Trunc(±0) = ±0
//	Decimal64Trunc(±Inf) = ±Inf
//	Decimal64Trunc(NaN) = NaN
func Decimal64Trunc(d decimal64) decimal64 {
	bits := Decimal64bits(d)
	if d64IsSpecial(bits) {
		return d
	}
	sign, exp, coeff := d64Unpack(bits)
	if coeff == 0 || exp >= 0 {
		return d
	}
	nfrac := -exp
	if nfrac >= d64NumDigits(coeff) {
		return d64Pack(sign, 0, 0)
	}
	coeff = coeff / dpow10[nfrac]
	return d64Pack(sign, 0, coeff)
}

// Decimal64Floor returns the greatest integer value less than or equal to d.
//
// Special cases are:
//
//	Decimal64Floor(±0) = ±0
//	Decimal64Floor(±Inf) = ±Inf
//	Decimal64Floor(NaN) = NaN
func Decimal64Floor(d decimal64) decimal64 {
	bits := Decimal64bits(d)
	if d64IsSpecial(bits) {
		return d
	}
	t := Decimal64Trunc(d)
	if d == t || !Decimal64Signbit(d) {
		return t
	}
	return t - 1
}

// Decimal64Ceil returns the least integer value greater than or equal to d.
//
// Special cases are:
//
//	Decimal64Ceil(±0) = ±0
//	Decimal64Ceil(±Inf) = ±Inf
//	Decimal64Ceil(NaN) = NaN
func Decimal64Ceil(d decimal64) decimal64 {
	return -Decimal64Floor(-d)
}

// Decimal64Round returns the nearest integer, rounding half away from zero.
//
// Special cases are:
//
//	Decimal64Round(±0) = ±0
//	Decimal64Round(±Inf) = ±Inf
//	Decimal64Round(NaN) = NaN
func Decimal64Round(d decimal64) decimal64 {
	bits := Decimal64bits(d)
	if d64IsSpecial(bits) {
		return d
	}
	sign, exp, coeff := d64Unpack(bits)
	if coeff == 0 || exp >= 0 {
		return d
	}
	nfrac := -exp
	ndigits := d64NumDigits(coeff)
	if nfrac > ndigits {
		return d64Pack(sign, 0, 0)
	}
	if nfrac == ndigits {
		// All digits fractional; compare leading digit to 5
		lead := coeff / dpow10[ndigits-1]
		if lead >= 5 {
			return d64Pack(sign, 0, 1)
		}
		return d64Pack(sign, 0, 0)
	}
	div := dpow10[nfrac]
	rem := coeff % div
	coeff = coeff / div
	half := div / 2
	if rem > half || (rem == half) {
		coeff++
	}
	return d64Pack(sign, 0, coeff)
}

// Decimal64RoundToEven returns the nearest integer, rounding ties to even.
//
// Special cases are:
//
//	Decimal64RoundToEven(±0) = ±0
//	Decimal64RoundToEven(±Inf) = ±Inf
//	Decimal64RoundToEven(NaN) = NaN
func Decimal64RoundToEven(d decimal64) decimal64 {
	bits := Decimal64bits(d)
	if d64IsSpecial(bits) {
		return d
	}
	sign, exp, coeff := d64Unpack(bits)
	if coeff == 0 || exp >= 0 {
		return d
	}
	nfrac := -exp
	ndigits := d64NumDigits(coeff)
	if nfrac > ndigits {
		return d64Pack(sign, 0, 0)
	}
	if nfrac == ndigits {
		lead := coeff / dpow10[ndigits-1]
		rem := coeff % dpow10[ndigits-1]
		if lead > 5 || (lead == 5 && rem > 0) {
			return d64Pack(sign, 0, 1)
		}
		// lead < 5, or lead == 5 and rem == 0 (tie): round to even (0 is even)
		return d64Pack(sign, 0, 0)
	}
	div := dpow10[nfrac]
	rem := coeff % div
	coeff = coeff / div
	half := div / 2
	if rem > half || (rem == half && coeff%2 != 0) {
		coeff++
	}
	return d64Pack(sign, 0, coeff)
}

// --- decimal128 versions ---

// d128DivPow10 divides a dUint128 coefficient by 10^n (where n <= 34).
// Returns (quotient, remainder).
func d128DivPow10(coeff dUint128, n int) (dUint128, dUint128) {
	if n <= 19 {
		q, r := dU128Div64(coeff, dpow10[n])
		return q, dU128From64(r)
	}
	// n > 19: divide in two steps
	q, r1 := dU128Div64(coeff, dpow10[19])
	q, r2 := dU128Div64(q, dpow10[n-19])
	// Remainder = r2 * 10^19 + r1
	rem := dU128Add(dU128Mul64(dU128From64(r2), dpow10[19]), dU128From64(r1))
	return q, rem
}

// d128HalfPow10 returns 10^n / 2 as a dUint128.
func d128HalfPow10(n int) dUint128 {
	if n <= 19 {
		return dU128From64(dpow10[n] / 2)
	}
	// 10^n / 2 = 5 * 10^(n-1)
	return dU128Mul64(dUpow10[n-1], 5)
}

// Decimal128Trunc returns the integer value of d.
//
// Special cases are:
//
//	Decimal128Trunc(±0) = ±0
//	Decimal128Trunc(±Inf) = ±Inf
//	Decimal128Trunc(NaN) = NaN
func Decimal128Trunc(d decimal128) decimal128 {
	hi, lo := Decimal128bits(d)
	if d128IsSpecial(hi) {
		return d
	}
	sign, exp, coeff := d128Unpack(hi, lo)
	if dU128IsZero(coeff) || exp >= 0 {
		return d
	}
	nfrac := -exp
	if nfrac >= dU128NumDigits(coeff) {
		return d128Pack(sign, 0, dU128Zero)
	}
	q, _ := d128DivPow10(coeff, nfrac)
	return d128Pack(sign, 0, q)
}

// Decimal128Floor returns the greatest integer value less than or equal to d.
//
// Special cases are:
//
//	Decimal128Floor(±0) = ±0
//	Decimal128Floor(±Inf) = ±Inf
//	Decimal128Floor(NaN) = NaN
func Decimal128Floor(d decimal128) decimal128 {
	hi, _ := Decimal128bits(d)
	if d128IsSpecial(hi) {
		return d
	}
	t := Decimal128Trunc(d)
	if d == t || !Decimal128Signbit(d) {
		return t
	}
	return t - 1
}

// Decimal128Ceil returns the least integer value greater than or equal to d.
//
// Special cases are:
//
//	Decimal128Ceil(±0) = ±0
//	Decimal128Ceil(±Inf) = ±Inf
//	Decimal128Ceil(NaN) = NaN
func Decimal128Ceil(d decimal128) decimal128 {
	return -Decimal128Floor(-d)
}

// Decimal128Round returns the nearest integer, rounding half away from zero.
//
// Special cases are:
//
//	Decimal128Round(±0) = ±0
//	Decimal128Round(±Inf) = ±Inf
//	Decimal128Round(NaN) = NaN
func Decimal128Round(d decimal128) decimal128 {
	hi, lo := Decimal128bits(d)
	if d128IsSpecial(hi) {
		return d
	}
	sign, exp, coeff := d128Unpack(hi, lo)
	if dU128IsZero(coeff) || exp >= 0 {
		return d
	}
	nfrac := -exp
	ndigits := dU128NumDigits(coeff)
	if nfrac > ndigits {
		return d128Pack(sign, 0, dU128Zero)
	}
	if nfrac == ndigits {
		// All digits fractional
		lead, _ := d128DivPow10(coeff, ndigits-1)
		if lead.lo >= 5 && lead.hi == 0 {
			return d128Pack(sign, 0, dU128From64(1))
		}
		return d128Pack(sign, 0, dU128Zero)
	}
	q, rem := d128DivPow10(coeff, nfrac)
	half := d128HalfPow10(nfrac)
	if dU128Cmp(rem, half) >= 0 {
		q = dU128Add(q, dU128From64(1))
	}
	return d128Pack(sign, 0, q)
}

// Decimal128RoundToEven returns the nearest integer, rounding ties to even.
//
// Special cases are:
//
//	Decimal128RoundToEven(±0) = ±0
//	Decimal128RoundToEven(±Inf) = ±Inf
//	Decimal128RoundToEven(NaN) = NaN
func Decimal128RoundToEven(d decimal128) decimal128 {
	hi, lo := Decimal128bits(d)
	if d128IsSpecial(hi) {
		return d
	}
	sign, exp, coeff := d128Unpack(hi, lo)
	if dU128IsZero(coeff) || exp >= 0 {
		return d
	}
	nfrac := -exp
	ndigits := dU128NumDigits(coeff)
	if nfrac > ndigits {
		return d128Pack(sign, 0, dU128Zero)
	}
	if nfrac == ndigits {
		lead, rem := d128DivPow10(coeff, ndigits-1)
		if lead.hi == 0 && lead.lo > 5 {
			return d128Pack(sign, 0, dU128From64(1))
		}
		if lead.hi == 0 && lead.lo == 5 && !dU128IsZero(rem) {
			return d128Pack(sign, 0, dU128From64(1))
		}
		return d128Pack(sign, 0, dU128Zero)
	}
	q, rem := d128DivPow10(coeff, nfrac)
	half := d128HalfPow10(nfrac)
	cmp := dU128Cmp(rem, half)
	if cmp > 0 || (cmp == 0 && q.lo%2 != 0) {
		q = dU128Add(q, dU128From64(1))
	}
	return d128Pack(sign, 0, q)
}
