// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Software decimal128 arithmetic using BID (Binary Integer Decimal) encoding.
// IEEE 754-2008 compliant decimal128 operations for softdecimal targets.

package runtime

import "unsafe"

// --- uint128 arithmetic helpers ---

// uint128 represents a 128-bit unsigned integer as a pair of uint64.
type uint128 struct {
	hi, lo uint64
}

var uint128Zero = uint128{0, 0}

// uint128From64 creates a uint128 from a single uint64 value.
//
//go:nosplit
func uint128From64(v uint64) uint128 {
	return uint128{0, v}
}

// u128IsZero reports whether u is zero.
//
//go:nosplit
func u128IsZero(u uint128) bool {
	return u.hi == 0 && u.lo == 0
}

// u128Cmp compares two uint128 values: returns -1, 0, or +1.
//
//go:nosplit
func u128Cmp(a, b uint128) int {
	if a.hi < b.hi {
		return -1
	}
	if a.hi > b.hi {
		return 1
	}
	if a.lo < b.lo {
		return -1
	}
	if a.lo > b.lo {
		return 1
	}
	return 0
}

// u128Add returns a + b, with carry.
//
//go:nosplit
func u128Add(a, b uint128) uint128 {
	lo := a.lo + b.lo
	carry := uint64(0)
	if lo < a.lo {
		carry = 1
	}
	hi := a.hi + b.hi + carry
	return uint128{hi, lo}
}

// u128Sub returns a - b (assuming a >= b).
//
//go:nosplit
func u128Sub(a, b uint128) uint128 {
	lo := a.lo - b.lo
	borrow := uint64(0)
	if a.lo < b.lo {
		borrow = 1
	}
	hi := a.hi - b.hi - borrow
	return uint128{hi, lo}
}

// u128Mul64 returns a * b where b is a uint64.
//
//go:nosplit
func u128Mul64(a uint128, b uint64) uint128 {
	// a = a.hi * 2^64 + a.lo
	// a * b = a.hi*b * 2^64 + a.lo*b
	loLo, loHi := mullu(a.lo, b)
	hiLo := a.hi * b // overflow of hi*b is discarded (fits assumption)
	return uint128{loHi + hiLo, loLo}
}

// u128Div64 divides a uint128 by a uint64, returning quotient and remainder.
func u128Div64(a uint128, b uint64) (q uint128, rem uint64) {
	if a.hi == 0 {
		return uint128{0, a.lo / b}, a.lo % b
	}
	// Divide hi part first
	qhi := a.hi / b
	rhi := a.hi % b
	// Now divide (rhi:a.lo) by b
	qlo, r := divlu(rhi, a.lo, b)
	return uint128{qhi, qlo}, r
}

// u128Mul returns a * b as a uint256 (represented as [4]uint64, little-endian).
func u128Mul(a, b uint128) (hi, lo uint128) {
	// School multiplication of two 128-bit numbers:
	// a = a.hi*2^64 + a.lo
	// b = b.hi*2^64 + b.lo
	// a*b = a.hi*b.hi*2^128 + (a.hi*b.lo + a.lo*b.hi)*2^64 + a.lo*b.lo

	// a.lo * b.lo -> 128 bits
	p0lo, p0hi := mullu(a.lo, b.lo)
	// a.lo * b.hi -> 128 bits
	p1lo, p1hi := mullu(a.lo, b.hi)
	// a.hi * b.lo -> 128 bits
	p2lo, p2hi := mullu(a.hi, b.lo)
	// a.hi * b.hi -> 128 bits
	p3lo, p3hi := mullu(a.hi, b.hi)

	// Accumulate: result[0] = p0lo
	r0 := p0lo
	// result[1] = p0hi + p1lo + p2lo, carry into result[2]
	r1 := p0hi + p1lo
	c1 := uint64(0)
	if r1 < p0hi {
		c1 = 1
	}
	r1b := r1 + p2lo
	if r1b < r1 {
		c1++
	}
	r1 = r1b
	// result[2] = p1hi + p2hi + p3lo + carry, carry into result[3]
	r2 := p1hi + p2hi
	c2 := uint64(0)
	if r2 < p1hi {
		c2 = 1
	}
	r2b := r2 + p3lo
	if r2b < r2 {
		c2++
	}
	r2 = r2b
	r2c := r2 + c1
	if r2c < r2 {
		c2++
	}
	r2 = r2c
	// result[3] = p3hi + carry
	r3 := p3hi + c2

	return uint128{r3, r2}, uint128{r1, r0}
}

// u128Shl returns a << n (n must be < 128).
//
//go:nosplit
func u128Shl(a uint128, n uint) uint128 {
	if n >= 64 {
		return uint128{a.lo << (n - 64), 0}
	}
	if n == 0 {
		return a
	}
	return uint128{a.hi<<n | a.lo>>(64-n), a.lo << n}
}

// u128Shr returns a >> n (n must be < 128).
//
//go:nosplit
func u128Shr(a uint128, n uint) uint128 {
	if n >= 64 {
		return uint128{0, a.hi >> (n - 64)}
	}
	if n == 0 {
		return a
	}
	return uint128{a.hi >> n, a.lo>>n | a.hi<<(64-n)}
}

// u128Div divides a by b, returning quotient and remainder.
// b must not be zero.
func u128Div(a, b uint128) (q, rem uint128) {
	if b.hi == 0 {
		// Divisor fits in 64 bits - use simpler division.
		qq, r := u128Div64(a, b.lo)
		return qq, uint128{0, r}
	}
	// Both hi parts non-zero. Use shift-subtract division.
	// Since b.hi != 0, quotient fits in 64 bits.
	if u128Cmp(a, b) < 0 {
		return uint128Zero, a
	}
	// Find the number of leading zeros difference
	n := u128LeadingZeros(b) - u128LeadingZeros(a)
	// Shift b left by n bits
	bshifted := u128Shl(b, uint(n))
	var result uint128
	for i := 0; i <= n; i++ {
		result = u128Shl(result, 1)
		if u128Cmp(a, bshifted) >= 0 {
			a = u128Sub(a, bshifted)
			result.lo |= 1
		}
		bshifted = u128Shr(bshifted, 1)
	}
	return result, a
}

// u128LeadingZeros returns the number of leading zero bits in a.
//
//go:nosplit
func u128LeadingZeros(a uint128) int {
	if a.hi != 0 {
		return leadingZeros64(a.hi)
	}
	return 64 + leadingZeros64(a.lo)
}

// leadingZeros64 returns the number of leading zero bits in x.
//
//go:nosplit
func leadingZeros64(x uint64) int {
	if x == 0 {
		return 64
	}
	n := 0
	if x&0xFFFFFFFF00000000 == 0 {
		n += 32
		x <<= 32
	}
	if x&0xFFFF000000000000 == 0 {
		n += 16
		x <<= 16
	}
	if x&0xFF00000000000000 == 0 {
		n += 8
		x <<= 8
	}
	if x&0xF000000000000000 == 0 {
		n += 4
		x <<= 4
	}
	if x&0xC000000000000000 == 0 {
		n += 2
		x <<= 2
	}
	if x&0x8000000000000000 == 0 {
		n++
	}
	return n
}

// u128NumDigits returns the number of decimal digits in a (a must be > 0).
func u128NumDigits(a uint128) int {
	if a.hi == 0 {
		return bid64NumDigits(a.lo)
	}
	// a >= 2^64 ≈ 1.8e19, so at least 20 digits.
	// decimal128 max coefficient is 10^34-1 (34 digits).
	n := 20
	// Divide down to count
	v := a
	for {
		v, _ = u128Div64(v, pow10_18)
		if v.hi == 0 && v.lo == 0 {
			break
		}
		n += 18
		if n > 40 {
			return n // safety
		}
	}
	// That approach over-counts. Use a simpler method:
	// Compare against known powers of 10.
	n = 1
	cmp := uint128From64(10)
	for u128Cmp(a, cmp) >= 0 {
		n++
		if n >= 39 {
			break // safety: max 39 digits for uint128
		}
		cmp = u128Mul64(cmp, 10)
		// If cmp overflows, a has n+1 digits
		if cmp.hi == 0 && cmp.lo == 0 {
			break
		}
	}
	return n
}

// --- BID128 encoding ---

const (
	bid128Bias     = 6176
	bid128MaxExp   = 6111  // max unbiased exponent
	bid128MinExp   = -6176 // min unbiased exponent
	bid128SignBit  = uint64(1) << 63

	// Special value patterns (in the high 64 bits)
	bid128Inf uint64 = 0x7800000000000000
	bid128NaN uint64 = 0x7C00000000000000

	// Masks for the high 64 bits
	bid128InfMask = uint64(0x7800000000000000)
	bid128NaNMask = uint64(0x7C00000000000000)
)

// bid128MaxCoeff is 10^34 - 1 = 9999999999999999999999999999999999
var bid128MaxCoeff = uint128{0x0001ED09BEAD87C0, 0x378D8E63FFFFFFFF}

// pow10_34 is 10^34
var pow10_34 = uint128{0x0001ED09BEAD87C0, 0x378D8E6400000000}

// decimal128bits returns the BID encoding of d as two uint64s (hi, lo).
//
//go:nosplit
func decimal128bits(d decimal128) (uint64, uint64) {
	p := (*[2]uint64)(unsafe.Pointer(&d))
	// decimal128 is stored as [lo, hi] in little-endian memory
	return p[1], p[0]
}

// decimal128frombits creates a decimal128 from two uint64s (hi, lo).
//
//go:nosplit
func decimal128frombits(hi, lo uint64) decimal128 {
	var d decimal128
	p := (*[2]uint64)(unsafe.Pointer(&d))
	p[0] = lo
	p[1] = hi
	return d
}

// bid128IsNaN reports whether the high word indicates NaN.
//
//go:nosplit
func bid128IsNaN(hi uint64) bool {
	return hi&bid128NaNMask == bid128NaNMask
}

// bid128IsInf reports whether the high word indicates infinity.
//
//go:nosplit
func bid128IsInf(hi uint64) bool {
	return hi&bid128NaNMask == bid128InfMask
}

// bid128Sign returns the sign bit from the high word.
//
//go:nosplit
func bid128Sign(hi uint64) uint64 {
	return hi & bid128SignBit
}

// bid128Unpack extracts sign, exponent, and coefficient from BID128.
//
//go:nosplit
func bid128Unpack(hi, lo uint64) (sign uint64, exp int, coeff uint128) {
	sign = hi & bid128SignBit

	if hi&bid128InfMask == bid128InfMask {
		return sign, 0, uint128Zero
	}

	// Two encoding forms based on bits [62:61] of hi word
	if hi&(3<<61) == (3 << 61) {
		// Form 2: bits[62:61] == 11 (but not special)
		// Exponent in bits[60:47] of hi (14 bits)
		exp = int((hi>>47)&0x3FFF) - bid128Bias
		// Coefficient: implicit (1<<49) prefix | bits[46:0] of hi : lo
		coeffHi := (uint64(1) << 49) | (hi & ((1 << 47) - 1))
		coeff = uint128{coeffHi, lo}
		if u128Cmp(coeff, bid128MaxCoeff) > 0 {
			coeff = uint128Zero
		}
	} else {
		// Form 1: bits[62:61] != 11
		// Exponent in bits[62:49] of hi (14 bits)
		exp = int((hi>>49)&0x3FFF) - bid128Bias
		// Coefficient in bits[48:0] of hi : lo (49+64 = 113 bits)
		coeffHi := hi & ((1 << 49) - 1)
		coeff = uint128{coeffHi, lo}
	}
	return
}

// bid128Pack packs sign, exponent, and coefficient into BID128 (hi, lo).
//
//go:nosplit
func bid128Pack(sign uint64, exp int, coeff uint128) (uint64, uint64) {
	if u128IsZero(coeff) {
		biasedExp := exp + bid128Bias
		if biasedExp < 0 {
			biasedExp = 0
		}
		if biasedExp > 0x3FFF {
			biasedExp = 0
		}
		return sign | uint64(biasedExp)<<49, 0
	}

	if u128Cmp(coeff, bid128MaxCoeff) > 0 {
		return sign | bid128NaN, 0
	}

	biasedExp := exp + bid128Bias
	if biasedExp < 0 || biasedExp > 0x3FFF {
		return sign | bid128NaN, 0
	}

	if coeff.hi < (1 << 49) {
		// Form 1: coefficient fits in 113 bits (hi < 2^49)
		return sign | uint64(biasedExp)<<49 | coeff.hi, coeff.lo
	}
	// Form 2: coefficient.hi >= 2^49 (but < 10^34)
	return sign | (3 << 61) | uint64(biasedExp)<<47 | (coeff.hi & ((1 << 47) - 1)), coeff.lo
}

// bid128Normalize adjusts coeff and exp so that coeff <= bid128MaxCoeff.
func bid128Normalize(sign uint64, exp int, coeff uint128) (uint64, uint64) {
	if u128IsZero(coeff) {
		return bid128Pack(sign, exp, uint128Zero)
	}

	// If coefficient is too large, divide by 10 and round.
	for u128Cmp(coeff, bid128MaxCoeff) > 0 {
		if exp > bid128MaxExp {
			return sign | bid128Inf, 0
		}
		var rem uint64
		coeff, rem = u128Div64(coeff, 10)
		exp++
		if rem > 5 || (rem == 5 && coeff.lo%2 != 0) {
			coeff = u128Add(coeff, uint128From64(1))
		}
	}

	// If exponent too large, try increasing coefficient
	for exp > bid128MaxExp && !u128IsZero(coeff) {
		test := u128Mul64(coeff, 10)
		if u128Cmp(test, bid128MaxCoeff) > 0 {
			return sign | bid128Inf, 0
		}
		coeff = test
		exp--
	}
	if exp > bid128MaxExp {
		return sign | bid128Inf, 0
	}

	// If exponent too small, decrease coefficient
	for exp < bid128MinExp && !u128IsZero(coeff) {
		var rem uint64
		coeff, rem = u128Div64(coeff, 10)
		exp++
		if rem > 5 || (rem == 5 && coeff.lo%2 != 0) {
			coeff = u128Add(coeff, uint128From64(1))
		}
	}
	if u128IsZero(coeff) {
		return bid128Pack(sign, bid128MinExp, uint128Zero)
	}

	return bid128Pack(sign, exp, coeff)
}

// --- Arithmetic ---

// dneg128 negates a decimal128 value by flipping the sign bit.
//
//go:nosplit
func dneg128(x decimal128) decimal128 {
	hi, lo := decimal128bits(x)
	return decimal128frombits(hi^bid128SignBit, lo)
}

// dadd128 computes x + y for BID-encoded decimal128 values.
func dadd128(x, y decimal128) decimal128 {
	xhi, xlo := decimal128bits(x)
	yhi, ylo := decimal128bits(y)

	if bid128IsNaN(xhi) || bid128IsNaN(yhi) {
		return decimal128frombits(bid128NaN, 0)
	}

	xInf := bid128IsInf(xhi)
	yInf := bid128IsInf(yhi)
	if xInf && yInf {
		if bid128Sign(xhi) == bid128Sign(yhi) {
			return x
		}
		return decimal128frombits(bid128NaN, 0)
	}
	if xInf {
		return x
	}
	if yInf {
		return y
	}

	xs, xe, xc := bid128Unpack(xhi, xlo)
	ys, ye, yc := bid128Unpack(yhi, ylo)

	if u128IsZero(xc) && u128IsZero(yc) {
		if xs != 0 && ys != 0 {
			e := xe
			if ye < e {
				e = ye
			}
			return decimal128frombits(bid128Pack(bid128SignBit, e, uint128Zero))
		}
		e := xe
		if ye < e {
			e = ye
		}
		return decimal128frombits(bid128Pack(0, e, uint128Zero))
	}
	if u128IsZero(xc) {
		return y
	}
	if u128IsZero(yc) {
		return x
	}

	// Align exponents
	if xe > ye {
		diff := xe - ye
		if diff > 40 {
			return decimal128frombits(bid128Normalize(xs, xe, xc))
		}
		for i := 0; i < diff; i++ {
			test := u128Mul64(xc, 10)
			if u128Cmp(test, uint128{0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF}) > 0 {
				// Would overflow; shift y down instead
				for j := 0; j < (diff - i); j++ {
					var rem uint64
					yc, rem = u128Div64(yc, 10)
					ye++
					if rem > 5 || (rem == 5 && yc.lo%2 != 0) {
						yc = u128Add(yc, uint128From64(1))
					}
				}
				break
			}
			xc = test
		}
		xe = ye
	} else if ye > xe {
		diff := ye - xe
		if diff > 40 {
			return decimal128frombits(bid128Normalize(ys, ye, yc))
		}
		for i := 0; i < diff; i++ {
			test := u128Mul64(yc, 10)
			if u128Cmp(test, uint128{0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF}) > 0 {
				for j := 0; j < (diff - i); j++ {
					var rem uint64
					xc, rem = u128Div64(xc, 10)
					xe++
					if rem > 5 || (rem == 5 && xc.lo%2 != 0) {
						xc = u128Add(xc, uint128From64(1))
					}
				}
				break
			}
			yc = test
		}
		ye = xe
	}

	var rs uint64
	var rc uint128
	re := xe

	if xs == ys {
		rs = xs
		rc = u128Add(xc, yc)
	} else {
		cmp := u128Cmp(xc, yc)
		if cmp >= 0 {
			rs = xs
			rc = u128Sub(xc, yc)
		} else {
			rs = ys
			rc = u128Sub(yc, xc)
		}
	}

	if u128IsZero(rc) {
		return decimal128frombits(bid128Pack(0, re, uint128Zero))
	}

	return decimal128frombits(bid128Normalize(rs, re, rc))
}

// dsub128 computes x - y.
//
//go:nosplit
func dsub128(x, y decimal128) decimal128 {
	return dadd128(x, dneg128(y))
}

// dmul128 computes x * y for BID-encoded decimal128 values.
func dmul128(x, y decimal128) decimal128 {
	xhi, xlo := decimal128bits(x)
	yhi, ylo := decimal128bits(y)

	if bid128IsNaN(xhi) || bid128IsNaN(yhi) {
		return decimal128frombits(bid128NaN, 0)
	}

	xs := bid128Sign(xhi)
	ys := bid128Sign(yhi)
	rs := xs ^ ys

	xInf := bid128IsInf(xhi)
	yInf := bid128IsInf(yhi)
	if xInf || yInf {
		if xInf {
			_, _, yc := bid128Unpack(yhi, ylo)
			if !yInf && u128IsZero(yc) {
				return decimal128frombits(bid128NaN, 0)
			}
		}
		if yInf {
			_, _, xc := bid128Unpack(xhi, xlo)
			if !xInf && u128IsZero(xc) {
				return decimal128frombits(bid128NaN, 0)
			}
		}
		return decimal128frombits(rs|bid128Inf, 0)
	}

	_, xe, xc := bid128Unpack(xhi, xlo)
	_, ye, yc := bid128Unpack(yhi, ylo)

	if u128IsZero(xc) || u128IsZero(yc) {
		re := xe + ye
		if re < bid128MinExp {
			re = bid128MinExp
		}
		if re > bid128MaxExp {
			re = bid128MaxExp
		}
		return decimal128frombits(bid128Pack(rs, re, uint128Zero))
	}

	re := xe + ye
	prodHi, prodLo := u128Mul(xc, yc)

	if u128IsZero(prodHi) {
		// Product fits in 128 bits
		return decimal128frombits(bid128Normalize(rs, re, prodLo))
	}

	// Product needs more than 128 bits. Reduce to 34 digits.
	rc := u256To34Digits(prodHi, prodLo, &re)
	return decimal128frombits(bid128Normalize(rs, re, rc))
}

// u256To34Digits reduces a 256-bit number (hi:lo as two uint128) to 34 digits.
func u256To34Digits(hi, lo uint128, exp *int) uint128 {
	// We need to divide the 256-bit value by 10 until it fits in 34 decimal digits.
	// Work with hi:lo as a 256-bit value.
	var lastRem uint64

	for !u128IsZero(hi) || u128Cmp(lo, bid128MaxCoeff) > 0 {
		// Divide the 256-bit value (hi:lo) by 10.
		// First divide hi by 10.
		qhi, rhi := u128Div64(hi, 10)
		// Now divide (rhi:lo.hi:lo.lo) by 10, where rhi < 10.
		var q2, q3 uint64
		r := rhi
		q2, r = divlu(r, lo.hi, 10)
		q3, lastRem = divlu(r, lo.lo, 10)

		hi = qhi
		lo = uint128{q2, q3}
		*exp++
	}

	// Round-to-nearest-even
	if lastRem > 5 || (lastRem == 5 && lo.lo%2 != 0) {
		lo = u128Add(lo, uint128From64(1))
	}

	return lo
}

// ddiv128 computes x / y for BID-encoded decimal128 values.
func ddiv128(x, y decimal128) decimal128 {
	xhi, xlo := decimal128bits(x)
	yhi, ylo := decimal128bits(y)

	if bid128IsNaN(xhi) || bid128IsNaN(yhi) {
		return decimal128frombits(bid128NaN, 0)
	}

	xs := bid128Sign(xhi)
	ys := bid128Sign(yhi)
	rs := xs ^ ys

	xInf := bid128IsInf(xhi)
	yInf := bid128IsInf(yhi)

	if xInf && yInf {
		return decimal128frombits(bid128NaN, 0)
	}
	if xInf {
		return decimal128frombits(rs|bid128Inf, 0)
	}
	if yInf {
		return decimal128frombits(bid128Pack(rs, 0, uint128Zero))
	}

	_, xe, xc := bid128Unpack(xhi, xlo)
	_, ye, yc := bid128Unpack(yhi, ylo)

	if u128IsZero(yc) {
		if u128IsZero(xc) {
			return decimal128frombits(bid128NaN, 0)
		}
		return decimal128frombits(rs|bid128Inf, 0)
	}

	if u128IsZero(xc) {
		re := xe - ye
		if re < bid128MinExp {
			re = bid128MinExp
		}
		if re > bid128MaxExp {
			re = bid128MaxExp
		}
		return decimal128frombits(bid128Pack(rs, re, uint128Zero))
	}

	re := xe - ye

	// Scale xc up by 10^34 for precision, then divide by yc.
	// xc * 10^34 can be up to ~34+34 = 68 digits (~226 bits).
	// We use u128Mul for this.
	scaledHi, scaledLo := u128Mul(xc, pow10_34)
	re -= 34

	// Divide the 256-bit (scaledHi:scaledLo) by yc (128-bit).
	q, rem := u256Div128(scaledHi, scaledLo, yc)

	// Round-to-nearest-even
	doubleRem := u128Shl(rem, 1)
	if u128Cmp(doubleRem, yc) > 0 || (u128Cmp(doubleRem, yc) == 0 && q.lo%2 != 0) {
		q = u128Add(q, uint128From64(1))
	}

	return decimal128frombits(bid128Normalize(rs, re, q))
}

// u256Div128 divides a 256-bit number (hi:lo, two uint128s) by a 128-bit divisor.
// Returns quotient and remainder (both uint128).
func u256Div128(hi, lo, divisor uint128) (q, rem uint128) {
	if u128IsZero(hi) {
		return u128Div(lo, divisor)
	}

	// Divide hi by divisor first
	qhi, rhi := u128Div(hi, divisor)
	_ = qhi // quotient high part (should be small for our use case)

	// Now divide (rhi * 2^128 + lo) by divisor
	// This is where it gets tricky with large numbers.
	// We use iterative shift-subtract division.

	// If rhi is zero, just divide lo
	if u128IsZero(rhi) {
		qlo, r := u128Div(lo, divisor)
		return qlo, r
	}

	// General case: shift-subtract division of (rhi:lo) / divisor
	// The quotient fits in 128 bits because rhi < divisor.
	// We process 128 bits of lo, one at a time, with rhi as initial remainder.
	result := uint128Zero
	remainder := rhi
	for i := 127; i >= 0; i-- {
		// Shift remainder left by 1
		remainder = u128Shl(remainder, 1)
		// Bring in bit i of lo
		bit := uint64(0)
		if uint(i) >= 64 {
			bit = (lo.hi >> uint(i-64)) & 1
		} else {
			bit = (lo.lo >> uint(i)) & 1
		}
		remainder.lo |= bit

		if u128Cmp(remainder, divisor) >= 0 {
			remainder = u128Sub(remainder, divisor)
			if uint(i) >= 64 {
				result.hi |= 1 << uint(i-64)
			} else {
				result.lo |= 1 << uint(i)
			}
		}
	}
	return result, remainder
}

// --- Comparisons ---

// deq128 reports whether x == y.
//
//go:nosplit
func deq128(x, y decimal128) bool {
	xhi, xlo := decimal128bits(x)
	yhi, ylo := decimal128bits(y)

	if bid128IsNaN(xhi) || bid128IsNaN(yhi) {
		return false
	}

	xInf := bid128IsInf(xhi)
	yInf := bid128IsInf(yhi)
	if xInf && yInf {
		return bid128Sign(xhi) == bid128Sign(yhi)
	}
	if xInf || yInf {
		return false
	}

	xs, xe, xc := bid128Unpack(xhi, xlo)
	ys, ye, yc := bid128Unpack(yhi, ylo)

	if u128IsZero(xc) && u128IsZero(yc) {
		return true // +0 == -0
	}
	if xs != ys {
		return false
	}
	if xe == ye {
		return u128Cmp(xc, yc) == 0
	}

	return bid128CompareEqual(xe, xc, ye, yc)
}

// bid128CompareEqual compares two positive decimal128 values with
// different exponents for equality.
func bid128CompareEqual(xe int, xc uint128, ye int, yc uint128) bool {
	if xe > ye {
		xe, xc, ye, yc = ye, yc, xe, xc
	}
	diff := ye - xe
	if diff > 40 {
		return false
	}
	for i := 0; i < diff; i++ {
		xc = u128Mul64(xc, 10)
		if u128Cmp(xc, uint128{0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF}) > 0 {
			return false
		}
	}
	return u128Cmp(xc, yc) == 0
}

// dlt128 reports whether x < y.
func dlt128(x, y decimal128) bool {
	xhi, xlo := decimal128bits(x)
	yhi, ylo := decimal128bits(y)

	if bid128IsNaN(xhi) || bid128IsNaN(yhi) {
		return false
	}

	xInf := bid128IsInf(xhi)
	yInf := bid128IsInf(yhi)
	xNeg := bid128Sign(xhi) != 0
	yNeg := bid128Sign(yhi) != 0

	if xInf && yInf {
		return xNeg && !yNeg
	}
	if xInf {
		return xNeg
	}
	if yInf {
		return !yNeg
	}

	xs, xe, xc := bid128Unpack(xhi, xlo)
	ys, ye, yc := bid128Unpack(yhi, ylo)

	if u128IsZero(xc) && u128IsZero(yc) {
		return false
	}
	if u128IsZero(xc) {
		return ys == 0 && !u128IsZero(yc)
	}
	if u128IsZero(yc) {
		return xs != 0
	}

	if xs != ys {
		return xs != 0
	}

	neg := xs != 0
	cmp := bid128CompareMagnitude(xe, xc, ye, yc)
	if neg {
		return cmp > 0
	}
	return cmp < 0
}

// bid128CompareMagnitude compares |a| vs |b|. Returns -1, 0, or +1.
func bid128CompareMagnitude(ae int, ac uint128, be int, bc uint128) int {
	if ae == be {
		return u128Cmp(ac, bc)
	}

	if ae > be {
		cmp := bid128CompareMagnitude(be, bc, ae, ac)
		return -cmp
	}

	diff := be - ae
	if diff > 40 {
		if u128IsZero(bc) {
			if u128IsZero(ac) {
				return 0
			}
			return 1
		}
		return -1
	}

	for i := 0; i < diff; i++ {
		test := u128Mul64(ac, 10)
		// Check for overflow
		if test.hi < ac.hi || (test.hi == ac.hi && test.lo < ac.lo && ac.lo != 0) {
			// ac overflowed, it's definitely bigger after full scaling
			// Scale bc down instead
			remaining := diff - i
			for j := 0; j < remaining; j++ {
				bc, _ = u128Div64(bc, 10)
			}
			return u128Cmp(ac, bc)
		}
		ac = test
	}

	return u128Cmp(ac, bc)
}

// dle128 reports whether x <= y.
//
//go:nosplit
func dle128(x, y decimal128) bool {
	xhi, _ := decimal128bits(x)
	yhi, _ := decimal128bits(y)
	if bid128IsNaN(xhi) || bid128IsNaN(yhi) {
		return false
	}
	return deq128(x, y) || dlt128(x, y)
}

// --- Conversions ---

// di64tod128 converts an int64 to a decimal128.
func di64tod128(x int64) decimal128 {
	if x == 0 {
		return decimal128frombits(bid128Pack(0, 0, uint128Zero))
	}
	var sign uint64
	var abs uint64
	if x < 0 {
		sign = bid128SignBit
		abs = uint64(-x)
	} else {
		abs = uint64(x)
	}
	return decimal128frombits(bid128Pack(sign, 0, uint128From64(abs)))
}

// dd128toi64 converts a decimal128 to int64.
func dd128toi64(x decimal128) int64 {
	hi, lo := decimal128bits(x)
	if bid128IsNaN(hi) || bid128IsInf(hi) {
		return 0
	}

	sign, exp, coeff := bid128Unpack(hi, lo)
	if u128IsZero(coeff) {
		return 0
	}

	var val uint128
	if exp >= 0 {
		val = coeff
		for i := 0; i < exp; i++ {
			val = u128Mul64(val, 10)
			if val.hi != 0 {
				return 0 // overflow
			}
		}
	} else {
		val = coeff
		for i := 0; i < -exp; i++ {
			val, _ = u128Div64(val, 10)
		}
	}

	if val.hi != 0 {
		return 0
	}

	if sign != 0 {
		if val.lo > 1<<63 {
			return 0
		}
		return -int64(val.lo)
	}
	if val.lo > 1<<63-1 {
		return 0
	}
	return int64(val.lo)
}

// du64tod128 converts a uint64 to a decimal128.
func du64tod128(x uint64) decimal128 {
	if x == 0 {
		return decimal128frombits(bid128Pack(0, 0, uint128Zero))
	}
	return decimal128frombits(bid128Pack(0, 0, uint128From64(x)))
}

// dd128tou64 converts a decimal128 to uint64.
func dd128tou64(x decimal128) uint64 {
	hi, lo := decimal128bits(x)
	if bid128IsNaN(hi) || bid128IsInf(hi) {
		return 0
	}

	sign, exp, coeff := bid128Unpack(hi, lo)
	if u128IsZero(coeff) || sign != 0 {
		return 0
	}

	var val uint128
	if exp >= 0 {
		val = coeff
		for i := 0; i < exp; i++ {
			val = u128Mul64(val, 10)
			if val.hi != 0 {
				return 0
			}
		}
	} else {
		val = coeff
		for i := 0; i < -exp; i++ {
			val, _ = u128Div64(val, 10)
		}
	}

	if val.hi != 0 {
		return 0
	}
	return val.lo
}

// df64tod128 converts a float64 to a decimal128.
func df64tod128(x float64) decimal128 {
	sign := float64bits(x) & (1 << 63)
	dsign := uint64(0)
	if sign != 0 {
		dsign = bid128SignBit
	}

	if x != x {
		return decimal128frombits(bid128NaN, 0)
	}
	if x > 0 && x+x == x {
		return decimal128frombits(dsign|bid128Inf, 0)
	}
	if x < 0 && x+x == x {
		return decimal128frombits(dsign|bid128Inf, 0)
	}
	if x == 0 {
		return decimal128frombits(bid128Pack(dsign, 0, uint128Zero))
	}

	f := x
	if f < 0 {
		f = -f
	}

	// Convert to 17 significant decimal digits (float64 precision)
	var coeff uint64
	dexp := 0

	fval := f
	for fval >= 10 {
		dexp++
		fval /= 10
	}
	for fval < 1 {
		dexp--
		fval *= 10
	}

	for i := 0; i < 17; i++ {
		digit := uint64(fval)
		coeff = coeff*10 + digit
		fval -= float64(digit)
		fval *= 10
	}
	dexp -= 16

	if uint64(fval) >= 5 {
		coeff++
	}

	return decimal128frombits(bid128Normalize(dsign, dexp, uint128From64(coeff)))
}

// dd128tof64 converts a decimal128 to float64.
func dd128tof64(x decimal128) float64 {
	hi, lo := decimal128bits(x)
	if bid128IsNaN(hi) {
		return float64frombits(0x7FF8000000000000)
	}
	if bid128IsInf(hi) {
		if bid128Sign(hi) != 0 {
			return float64frombits(0xFFF0000000000000)
		}
		return float64frombits(0x7FF0000000000000)
	}

	sign, exp, coeff := bid128Unpack(hi, lo)
	if u128IsZero(coeff) {
		if sign != 0 {
			return float64frombits(0x8000000000000000)
		}
		return 0
	}

	// Convert coefficient to float64
	// coeff can be up to 34 digits (~113 bits)
	// float64 has 53 bits of mantissa, so we'll lose precision
	result := float64(coeff.hi)*float64(uint64(1)<<32)*float64(uint64(1)<<32) + float64(coeff.lo)

	if exp > 0 {
		for exp > 0 {
			if exp >= 16 {
				result *= 1e16
				exp -= 16
			} else if exp >= 8 {
				result *= 1e8
				exp -= 8
			} else {
				result *= 10
				exp--
			}
		}
	} else if exp < 0 {
		for exp < 0 {
			if exp <= -16 {
				result /= 1e16
				exp += 16
			} else if exp <= -8 {
				result /= 1e8
				exp += 8
			} else {
				result /= 10
				exp++
			}
		}
	}

	if sign != 0 {
		result = -result
	}
	return result
}

// dd64tod128 widens a decimal64 to decimal128.
func dd64tod128(x decimal64) decimal128 {
	xb := decimal64bits(x)
	if bid64IsNaN(xb) {
		return decimal128frombits(bid128NaN, 0)
	}
	if bid64IsInf(xb) {
		return decimal128frombits(bid64Sign(xb)|bid128Inf, 0)
	}
	sign, exp, coeff := bid64Unpack(xb)
	// Map sign bit: decimal64 sign bit is also bit 63
	dsign := uint64(0)
	if sign != 0 {
		dsign = bid128SignBit
	}
	return decimal128frombits(bid128Pack(dsign, exp, uint128From64(coeff)))
}

// dd128tod64 narrows a decimal128 to decimal64.
func dd128tod64(x decimal128) decimal64 {
	hi, lo := decimal128bits(x)
	if bid128IsNaN(hi) {
		return decimal64frombits(bid64NaN)
	}
	if bid128IsInf(hi) {
		dsign := uint64(0)
		if bid128Sign(hi) != 0 {
			dsign = bid64SignBit
		}
		return decimal64frombits(dsign | bid64Inf)
	}
	sign, exp, coeff := bid128Unpack(hi, lo)
	dsign := uint64(0)
	if sign != 0 {
		dsign = bid64SignBit
	}

	// Reduce coefficient to 16 digits (decimal64 max)
	for u128Cmp(coeff, uint128From64(bid64MaxCoeff)) > 0 {
		var rem uint64
		coeff, rem = u128Div64(coeff, 10)
		exp++
		if rem > 5 || (rem == 5 && coeff.lo%2 != 0) {
			coeff = u128Add(coeff, uint128From64(1))
		}
	}

	return decimal64frombits(bid64Normalize(dsign, exp, coeff.lo))
}

// --- Print ---

// printdecimal128 prints a BID-encoded decimal128 value for the builtin println.
func printdecimal128(x decimal128) {
	hi, lo := decimal128bits(x)
	if bid128IsNaN(hi) {
		printstring("NaN")
		return
	}
	if bid128IsInf(hi) {
		if bid128Sign(hi) != 0 {
			printstring("-Inf")
		} else {
			printstring("+Inf")
		}
		return
	}

	sign, exp, coeff := bid128Unpack(hi, lo)

	if sign != 0 {
		printstring("-")
	}

	if u128IsZero(coeff) {
		printstring("0")
		return
	}

	// Strip trailing zeros
	for {
		c, rem := u128Div64(coeff, 10)
		if rem != 0 {
			break
		}
		coeff = c
		exp++
	}

	// Extract digits (most-significant first)
	var digits [40]byte
	ndigits := 0
	c := coeff
	for !u128IsZero(c) {
		var rem uint64
		c, rem = u128Div64(c, 10)
		digits[ndigits] = byte(rem) + '0'
		ndigits++
	}
	for i, j := 0, ndigits-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}

	e := exp + ndigits - 1

	if e < -4 || e >= ndigits {
		// Scientific notation
		var buf [50]byte
		w := 0
		buf[w] = digits[0]
		w++
		if ndigits > 1 {
			buf[w] = '.'
			w++
			for i := 1; i < ndigits; i++ {
				buf[w] = digits[i]
				w++
			}
		}
		buf[w] = 'e'
		w++
		if e < 0 {
			buf[w] = '-'
			e = -e
		} else {
			buf[w] = '+'
		}
		w++
		if e >= 1000 {
			buf[w] = byte(e/1000) + '0'
			w++
		}
		if e >= 100 {
			buf[w] = byte(e/100%10) + '0'
			w++
		}
		if e >= 10 {
			buf[w] = byte(e/10%10) + '0'
			w++
		}
		buf[w] = byte(e%10) + '0'
		w++
		gwrite(buf[:w])
	} else if exp >= 0 {
		gwrite(digits[:ndigits])
		for i := 0; i < exp; i++ {
			printstring("0")
		}
	} else {
		intPart := ndigits + exp
		if intPart > 0 {
			gwrite(digits[:intPart])
			printstring(".")
			gwrite(digits[intPart:ndigits])
		} else {
			printstring("0.")
			for i := 0; i < -intPart; i++ {
				printstring("0")
			}
			gwrite(digits[:ndigits])
		}
	}
}

// --- Hash and equal ---

func d128hash(p unsafe.Pointer, h uintptr) uintptr {
	x := (*[2]uint64)(p)
	hi := x[1]
	lo := x[0]
	if bid128IsNaN(hi) {
		return c1 * (c0 ^ h ^ uintptr(rand()))
	}
	_, _, coeff := bid128Unpack(hi, lo)
	if u128IsZero(coeff) {
		return c1 * (c0 ^ h) // +0 == -0
	}
	return memhash(p, h, 16)
}

func d128equal(p, q unsafe.Pointer) bool {
	pp := (*[2]uint64)(p)
	qq := (*[2]uint64)(q)
	return deq128(decimal128frombits(pp[1], pp[0]), decimal128frombits(qq[1], qq[0]))
}
