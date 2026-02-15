// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package math

import "math/bits"

// Internal BID (Binary Integer Decimal) helpers for decimal math functions.
// These mirror the runtime's BID helpers but are kept in the math package
// to avoid linkname coupling.

// dUint128 is an unsigned 128-bit integer for decimal128 coefficient manipulation.
type dUint128 struct{ hi, lo uint64 }

var dU128Zero = dUint128{0, 0}

func dU128From64(v uint64) dUint128 { return dUint128{0, v} }

func dU128IsZero(a dUint128) bool { return a.hi == 0 && a.lo == 0 }

func dU128Cmp(a, b dUint128) int {
	if a.hi != b.hi {
		if a.hi < b.hi {
			return -1
		}
		return 1
	}
	if a.lo != b.lo {
		if a.lo < b.lo {
			return -1
		}
		return 1
	}
	return 0
}

func dU128Add(a, b dUint128) dUint128 {
	lo := a.lo + b.lo
	carry := uint64(0)
	if lo < a.lo {
		carry = 1
	}
	return dUint128{a.hi + b.hi + carry, lo}
}

func dU128Sub(a, b dUint128) dUint128 {
	lo := a.lo - b.lo
	borrow := uint64(0)
	if a.lo < b.lo {
		borrow = 1
	}
	return dUint128{a.hi - b.hi - borrow, lo}
}

func dU128Mul64(a dUint128, b uint64) dUint128 {
	hi, lo := bits.Mul64(a.lo, b)
	hi += a.hi * b
	return dUint128{hi, lo}
}

func dU128Div64(a dUint128, b uint64) (q dUint128, rem uint64) {
	if a.hi == 0 {
		return dUint128{0, a.lo / b}, a.lo % b
	}
	qhi := a.hi / b
	rhi := a.hi % b
	qlo, r := bits.Div64(rhi, a.lo, b)
	return dUint128{qhi, qlo}, r
}

// --- decimal64 BID constants ---

const (
	d64Bias     = 398
	d64MaxCoeff = 9999999999999999 // 10^16 - 1
	d64SignBit  = uint64(1) << 63
	d64NaN      = uint64(0x7C00000000000000)
	d64Inf      = uint64(0x7800000000000000)
	d64NaNMask  = uint64(0x7C00000000000000) // also matches NaN
)

func d64IsNaN(bits uint64) bool { return bits&d64NaNMask == d64NaN }
func d64IsInf(bits uint64) bool { return bits&d64NaNMask == d64Inf }

func d64IsSpecial(bits uint64) bool {
	return bits&d64Inf == d64Inf
}

// d64Unpack extracts the sign, unbiased exponent, and coefficient from
// a decimal64 BID encoding. For NaN/Inf, coeff is 0 and exp is 0.
func d64Unpack(bits uint64) (sign uint64, exp int, coeff uint64) {
	sign = bits & d64SignBit

	if d64IsSpecial(bits) {
		return sign, 0, 0
	}

	if bits&(3<<61) == (3 << 61) {
		// Form 2: bits[62:61] == 11
		exp = int((bits>>51)&0x3FF) - d64Bias
		coeff = (1 << 53) | (bits & ((1 << 51) - 1))
		if coeff > d64MaxCoeff {
			coeff = 0
		}
	} else {
		// Form 1: bits[62:61] != 11
		exp = int((bits>>53)&0x3FF) - d64Bias
		coeff = bits & ((1 << 53) - 1)
	}
	return
}

// d64Pack encodes sign, exponent, and coefficient into a decimal64.
func d64Pack(sign uint64, exp int, coeff uint64) decimal64 {
	if coeff == 0 {
		biasedExp := exp + d64Bias
		if biasedExp < 0 {
			biasedExp = 0
		}
		if biasedExp > 767 {
			biasedExp = 0
		}
		return Decimal64frombits(sign | uint64(biasedExp)<<53)
	}
	if coeff > d64MaxCoeff {
		return Decimal64NaN()
	}
	biasedExp := exp + d64Bias
	if biasedExp < 0 || biasedExp > 767 {
		return Decimal64NaN()
	}
	if coeff < (1 << 53) {
		return Decimal64frombits(sign | uint64(biasedExp)<<53 | coeff)
	}
	return Decimal64frombits(sign | (3 << 61) | uint64(biasedExp)<<51 | (coeff & ((1 << 51) - 1)))
}

// --- decimal128 BID constants ---

const (
	d128Bias    = 6176
	d128SignBit = uint64(1) << 63
	d128NaN     = uint64(0x7C00000000000000)
	d128Inf     = uint64(0x7800000000000000)
	d128NaNMask = uint64(0x7C00000000000000)
)

var d128MaxCoeff = dUint128{0x0001ED09BEAD87C0, 0x378D8E63FFFFFFFF} // 10^34 - 1

func d128IsNaN(hi uint64) bool { return hi&d128NaNMask == d128NaN }
func d128IsInf(hi uint64) bool { return hi&d128NaNMask == d128Inf }

func d128IsSpecial(hi uint64) bool {
	return hi&d128Inf == d128Inf
}

// d128Unpack extracts the sign, unbiased exponent, and coefficient from
// a decimal128 BID encoding.
func d128Unpack(hi, lo uint64) (sign uint64, exp int, coeff dUint128) {
	sign = hi & d128SignBit

	if d128IsSpecial(hi) {
		return sign, 0, dU128Zero
	}

	if hi&(3<<61) == (3 << 61) {
		// Form 2
		exp = int((hi>>47)&0x3FFF) - d128Bias
		coeffHi := (uint64(1) << 49) | (hi & ((1 << 47) - 1))
		coeff = dUint128{coeffHi, lo}
		if dU128Cmp(coeff, d128MaxCoeff) > 0 {
			coeff = dU128Zero
		}
	} else {
		// Form 1
		exp = int((hi>>49)&0x3FFF) - d128Bias
		coeffHi := hi & ((1 << 49) - 1)
		coeff = dUint128{coeffHi, lo}
	}
	return
}

// d128Pack encodes sign, exponent, and coefficient into a decimal128.
func d128Pack(sign uint64, exp int, coeff dUint128) decimal128 {
	if dU128IsZero(coeff) {
		biasedExp := exp + d128Bias
		if biasedExp < 0 {
			biasedExp = 0
		}
		if biasedExp > 0x3FFF {
			biasedExp = 0
		}
		return Decimal128frombits(sign|uint64(biasedExp)<<49, 0)
	}
	if dU128Cmp(coeff, d128MaxCoeff) > 0 {
		return Decimal128frombits(sign|d128NaN, 0)
	}
	biasedExp := exp + d128Bias
	if biasedExp < 0 || biasedExp > 0x3FFF {
		return Decimal128frombits(sign|d128NaN, 0)
	}
	if coeff.hi < (1 << 49) {
		return Decimal128frombits(sign|uint64(biasedExp)<<49|coeff.hi, coeff.lo)
	}
	return Decimal128frombits(sign|(3<<61)|uint64(biasedExp)<<47|(coeff.hi&((1<<47)-1)), coeff.lo)
}

// --- digit counting ---

func d64NumDigits(v uint64) int {
	if v == 0 {
		return 1
	}
	n := 1
	if v >= dpow10[16] {
		return 17
	}
	if v >= dpow10[8] {
		n += 8
		v /= dpow10[8]
	}
	if v >= dpow10[4] {
		n += 4
		v /= dpow10[4]
	}
	if v >= dpow10[2] {
		n += 2
		v /= dpow10[2]
	}
	if v >= 10 {
		n++
	}
	return n
}

func dU128NumDigits(a dUint128) int {
	if a.hi == 0 {
		return d64NumDigits(a.lo)
	}
	// a >= 2^64, so at least 20 digits.
	n := 1
	cmp := dU128From64(10)
	for dU128Cmp(a, cmp) >= 0 {
		n++
		if n >= 39 {
			break
		}
		cmp = dU128Mul64(cmp, 10)
	}
	return n
}

// --- powers of 10 tables ---

var dpow10 = [20]uint64{
	1,
	10,
	100,
	1000,
	10000,
	100000,
	1000000,
	10000000,
	100000000,
	1000000000,
	10000000000,
	100000000000,
	1000000000000,
	10000000000000,
	100000000000000,
	1000000000000000,
	10000000000000000,
	100000000000000000,
	1000000000000000000,
	10000000000000000000, // 10^19
}

// dUpow10 contains 10^0 through 10^34 as dUint128, for decimal128 operations.
var dUpow10 [35]dUint128

func init() {
	dUpow10[0] = dU128From64(1)
	for i := 1; i < 35; i++ {
		dUpow10[i] = dU128Mul64(dUpow10[i-1], 10)
	}
}
