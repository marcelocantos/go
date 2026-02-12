// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// BID (Binary Integer Decimal) pack/unpack helpers for decimal64.
// IEEE 754-2008 decimal64 format with BID encoding.

package runtime

import "unsafe"

// decimal64bits returns the BID encoding of d as a uint64.
//
//go:nosplit
func decimal64bits(d decimal64) uint64 {
	return *(*uint64)(unsafe.Pointer(&d))
}

// decimal64frombits returns the decimal64 value corresponding to the
// BID encoding b.
//
//go:nosplit
func decimal64frombits(b uint64) decimal64 {
	return *(*decimal64)(unsafe.Pointer(&b))
}

const (
	// decimal64 BID encoding constants
	bid64Bias     = 398               // exponent bias
	bid64MaxCoeff = 9999999999999999  // 10^16 - 1
	bid64MaxExp   = 369               // max unbiased exponent
	bid64MinExp   = -398              // min unbiased exponent
	bid64SignBit  = 1 << 63           // sign bit

	// Special value patterns
	bid64Inf uint64 = 0x7800000000000000 // +Infinity
	bid64NaN uint64 = 0x7C00000000000000 // quiet NaN

	// Masks
	bid64SpecialMask = 0x7C00000000000000 // bits[62:58] for special detection
	bid64InfMask     = 0x7800000000000000 // infinity pattern
	bid64NaNMask     = 0x7C00000000000000 // NaN pattern

	// powers of 10 (up to 10^19 fits in uint64)
	pow10_0  = 1
	pow10_1  = 10
	pow10_2  = 100
	pow10_3  = 1000
	pow10_4  = 10000
	pow10_5  = 100000
	pow10_6  = 1000000
	pow10_7  = 10000000
	pow10_8  = 100000000
	pow10_9  = 1000000000
	pow10_10 = 10000000000
	pow10_11 = 100000000000
	pow10_12 = 1000000000000
	pow10_13 = 10000000000000
	pow10_14 = 100000000000000
	pow10_15 = 1000000000000000
	pow10_16 = 10000000000000000
	pow10_17 = 100000000000000000
	pow10_18 = 1000000000000000000
)

// pow10tab contains powers of 10 indexed by exponent.
var pow10tab = [20]uint64{
	pow10_0, pow10_1, pow10_2, pow10_3, pow10_4,
	pow10_5, pow10_6, pow10_7, pow10_8, pow10_9,
	pow10_10, pow10_11, pow10_12, pow10_13, pow10_14,
	pow10_15, pow10_16, pow10_17, pow10_18,
	10000000000000000000, // 10^19
}

// bid64IsNaN reports whether x is a NaN.
//
//go:nosplit
func bid64IsNaN(x uint64) bool {
	return x&bid64SpecialMask == bid64NaNMask
}

// bid64IsInf reports whether x is an infinity.
//
//go:nosplit
func bid64IsInf(x uint64) bool {
	// Infinity: bits[62:58] == 11110, NaN: bits[62:58] == 11111
	// So infinity is when the special bits match inf but NOT nan.
	return x&bid64NaNMask == bid64InfMask
}

// bid64IsSpecial reports whether x is NaN or infinity.
//
//go:nosplit
func bid64IsSpecial(x uint64) bool {
	return x&bid64InfMask == bid64InfMask
}

// bid64Sign returns the sign bit of x (0 or bid64SignBit).
//
//go:nosplit
func bid64Sign(x uint64) uint64 {
	return x & bid64SignBit
}

// bid64Unpack extracts the sign, exponent, and coefficient from a BID64 value.
// For special values (NaN, Inf), the returned exp and coeff are undefined.
// The caller should check bid64IsNaN / bid64IsInf first.
//
//go:nosplit
func bid64Unpack(x uint64) (sign uint64, exp int, coeff uint64) {
	sign = x & bid64SignBit

	// Check for special values
	if x&bid64InfMask == bid64InfMask {
		// Infinity or NaN - return zero coeff, zero exp
		return sign, 0, 0
	}

	// Two encoding forms based on bits [62:61]
	if x&(3<<61) == (3 << 61) {
		// Form 2: bits[62:61] == 11 (but not special since we checked above)
		// Exponent in bits[60:51] (10 bits)
		exp = int((x>>51)&0x3FF) - bid64Bias
		// Coefficient: implicit (1<<53) | bits[50:0]
		coeff = (1 << 53) | (x & ((1 << 51) - 1))
		// If coefficient >= 10^16, the value is treated as zero
		if coeff > bid64MaxCoeff {
			coeff = 0
		}
	} else {
		// Form 1: bits[62:61] != 11
		// Exponent in bits[62:53] (10 bits)
		exp = int((x>>53)&0x3FF) - bid64Bias
		// Coefficient in bits[52:0] (53 bits)
		coeff = x & ((1 << 53) - 1)
	}
	return
}

// bid64Pack packs sign, exponent, and coefficient into a BID64 value.
// The coefficient must be <= bid64MaxCoeff. The exponent must be in
// range [bid64MinExp, bid64MaxExp] after any normalization.
//
//go:nosplit
func bid64Pack(sign uint64, exp int, coeff uint64) uint64 {
	if coeff == 0 {
		// Zero with the given sign and preferred exponent
		biasedExp := exp + bid64Bias
		if biasedExp < 0 {
			biasedExp = 0
		}
		if biasedExp > 767 {
			biasedExp = 0
		}
		return sign | uint64(biasedExp)<<53
	}

	if coeff > bid64MaxCoeff {
		// Should not happen after normalization; return NaN
		return sign | bid64NaN
	}

	biasedExp := exp + bid64Bias
	if biasedExp < 0 || biasedExp > 767 {
		// Out of range; caller should have handled overflow/underflow
		return sign | bid64NaN
	}

	if coeff < (1 << 53) {
		// Form 1: coefficient fits in 53 bits
		return sign | uint64(biasedExp)<<53 | coeff
	}
	// Form 2: coefficient >= 2^53 (but < 10^16)
	// bits[62:61] = 11, exponent in bits[60:51], coefficient lower 51 bits
	return sign | (3 << 61) | uint64(biasedExp)<<51 | (coeff & ((1 << 51) - 1))
}

// bid64Zero returns positive zero.
//
//go:nosplit
func bid64Zero() uint64 {
	return bid64Pack(0, 0, 0)
}

// bid64NumDigits returns the number of decimal digits in v (v must be > 0).
//
//go:nosplit
func bid64NumDigits(v uint64) int {
	n := 1
	if v >= pow10_16 {
		return 17 // max for our purposes
	}
	if v >= pow10_8 {
		n += 8
		v /= pow10_8
	}
	if v >= pow10_4 {
		n += 4
		v /= pow10_4
	}
	if v >= pow10_2 {
		n += 2
		v /= pow10_2
	}
	if v >= pow10_1 {
		n++
	}
	return n
}

// bid64Normalize adjusts coeff and exp so that coeff fits in the decimal64
// range (at most 16 digits, i.e., <= 9999999999999999).
// It applies round-to-nearest-even when truncating.
// Returns the final sign, exp, and coeff ready for packing.
func bid64Normalize(sign uint64, exp int, coeff uint64) uint64 {
	if coeff == 0 {
		return bid64Pack(sign, exp, 0)
	}

	// If coefficient is too large, divide by powers of 10 and round.
	for coeff > bid64MaxCoeff {
		if exp > bid64MaxExp {
			// Overflow -> infinity
			return sign | bid64Inf
		}
		// Determine how many digits to remove
		// We remove one digit at a time for correct rounding
		rem := coeff % 10
		coeff /= 10
		exp++

		// Round-to-nearest-even
		if rem > 5 || (rem == 5 && coeff%2 != 0) {
			coeff++
		}
	}

	// If exponent is too large, try to increase coefficient
	for exp > bid64MaxExp && coeff != 0 {
		if coeff > bid64MaxCoeff/10 {
			// Would overflow coefficient; result is infinity
			return sign | bid64Inf
		}
		coeff *= 10
		exp--
	}
	if exp > bid64MaxExp {
		return sign | bid64Inf
	}

	// If exponent is too small, try to decrease coefficient
	for exp < bid64MinExp && coeff != 0 {
		rem := coeff % 10
		coeff /= 10
		exp++
		if rem > 5 || (rem == 5 && coeff%2 != 0) {
			coeff++
		}
	}
	if coeff == 0 {
		return bid64Pack(sign, bid64MinExp, 0)
	}

	return bid64Pack(sign, exp, coeff)
}

// bid64RoundToNDigits rounds coeff to at most n significant digits,
// adjusting exp accordingly. Uses round-to-nearest-even.
func bid64RoundToNDigits(coeff uint64, exp int, n int) (uint64, int) {
	if coeff == 0 {
		return 0, exp
	}
	digits := bid64NumDigits(coeff)
	for digits > n {
		rem := coeff % 10
		coeff /= 10
		exp++
		digits--
		if rem > 5 || (rem == 5 && coeff%2 != 0) {
			coeff++
			// Check if rounding increased digit count
			if bid64NumDigits(coeff) > digits {
				// e.g. 9999 + 1 = 10000
				coeff /= 10
				exp++
				digits--
			}
		}
	}
	return coeff, exp
}
