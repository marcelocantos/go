// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Software decimal64 arithmetic using BID (Binary Integer Decimal) encoding.
// IEEE 754-2008 compliant decimal64 operations for softdecimal targets.

package runtime

// dneg64 negates a decimal64 value by flipping the sign bit.
//
//go:nosplit
func dneg64(x decimal64) decimal64 {
	return decimal64frombits(decimal64bits(x) ^ bid64SignBit)
}

// dadd64 computes x + y for BID-encoded decimal64 values.
func dadd64(x, y decimal64) decimal64 {
	xb := decimal64bits(x)
	yb := decimal64bits(y)

	// Handle NaN
	if bid64IsNaN(xb) || bid64IsNaN(yb) {
		return decimal64frombits(bid64NaN)
	}

	// Handle infinity
	xInf := bid64IsInf(xb)
	yInf := bid64IsInf(yb)
	if xInf && yInf {
		// Inf + Inf with same sign = Inf; different signs = NaN
		if bid64Sign(xb) == bid64Sign(yb) {
			return x
		}
		return decimal64frombits(bid64NaN)
	}
	if xInf {
		return x
	}
	if yInf {
		return y
	}

	xs, xe, xc := bid64Unpack(xb)
	ys, ye, yc := bid64Unpack(yb)

	// Handle zeros
	if xc == 0 && yc == 0 {
		// Both zero: -0 + -0 = -0, otherwise +0
		if xs != 0 && ys != 0 {
			return decimal64frombits(bid64Pack(bid64SignBit, min(xe, ye), 0))
		}
		return decimal64frombits(bid64Pack(0, min(xe, ye), 0))
	}
	if xc == 0 {
		// Pad y with trailing zeros to adopt x's more precise quantum.
		for ye > xe && yc <= bid64MaxCoeff/10 {
			yc *= 10
			ye--
		}
		return decimal64frombits(bid64Pack(ys, ye, yc))
	}
	if yc == 0 {
		// Pad x with trailing zeros to adopt y's more precise quantum.
		for xe > ye && xc <= bid64MaxCoeff/10 {
			xc *= 10
			xe--
		}
		return decimal64frombits(bid64Pack(xs, xe, xc))
	}

	// Align exponents: shift the coefficient of the number with
	// the larger exponent so both have the same (smaller) exponent.
	if xe > ye {
		// Shift xc up (multiply by 10^(xe-ye)) to align with ye
		diff := xe - ye
		if diff > 17 {
			// x's coefficient would become huge; y is negligible
			// But we still need to round properly.
			return decimal64frombits(bid64Normalize(xs, xe, xc))
		}
		for i := 0; i < diff; i++ {
			if xc > bid64MaxCoeff/10+1 {
				// Would overflow; shift y down instead
				for j := 0; j < (diff - i); j++ {
					rem := yc % 10
					yc /= 10
					ye++
					if rem > 5 || (rem == 5 && yc%2 != 0) {
						yc++
					}
				}
				break
			}
			xc *= 10
		}
		xe = ye
	} else if ye > xe {
		diff := ye - xe
		if diff > 17 {
			return decimal64frombits(bid64Normalize(ys, ye, yc))
		}
		for i := 0; i < diff; i++ {
			if yc > bid64MaxCoeff/10+1 {
				for j := 0; j < (diff - i); j++ {
					rem := xc % 10
					xc /= 10
					xe++
					if rem > 5 || (rem == 5 && xc%2 != 0) {
						xc++
					}
				}
				break
			}
			yc *= 10
		}
		ye = xe
	}

	// Now xe == ye. Add or subtract coefficients based on sign.
	var rs uint64
	var rc uint64
	re := xe

	if xs == ys {
		// Same sign: add magnitudes
		rs = xs
		rc = xc + yc
	} else {
		// Different signs: subtract magnitudes
		if xc >= yc {
			rs = xs
			rc = xc - yc
		} else {
			rs = ys
			rc = yc - xc
		}
	}

	if rc == 0 {
		// Result is zero. IEEE 754: +0 unless rounding mode is toward -Inf
		return decimal64frombits(bid64Pack(0, re, 0))
	}

	return decimal64frombits(bid64Normalize(rs, re, rc))
}

// dsub64 computes x - y for BID-encoded decimal64 values.
//
//go:nosplit
func dsub64(x, y decimal64) decimal64 {
	return dadd64(x, dneg64(y))
}

// dmul64 computes x * y for BID-encoded decimal64 values.
func dmul64(x, y decimal64) decimal64 {
	xb := decimal64bits(x)
	yb := decimal64bits(y)

	// Handle NaN
	if bid64IsNaN(xb) || bid64IsNaN(yb) {
		return decimal64frombits(bid64NaN)
	}

	xs := bid64Sign(xb)
	ys := bid64Sign(yb)
	rs := xs ^ ys // result sign

	// Handle infinity
	xInf := bid64IsInf(xb)
	yInf := bid64IsInf(yb)
	if xInf || yInf {
		// Check for 0 * Inf = NaN
		if xInf {
			_, _, yc := bid64Unpack(yb)
			if !yInf && yc == 0 {
				return decimal64frombits(bid64NaN)
			}
		}
		if yInf {
			_, _, xc := bid64Unpack(xb)
			if !xInf && xc == 0 {
				return decimal64frombits(bid64NaN)
			}
		}
		return decimal64frombits(rs | bid64Inf)
	}

	_, xe, xc := bid64Unpack(xb)
	_, ye, yc := bid64Unpack(yb)

	// Handle zeros
	if xc == 0 || yc == 0 {
		re := xe + ye
		if re < bid64MinExp {
			re = bid64MinExp
		}
		if re > bid64MaxExp {
			re = bid64MaxExp
		}
		return decimal64frombits(bid64Pack(rs, re, 0))
	}

	// Multiply coefficients.
	// Max coefficient is 9999999999999999 (~10^16), which is about 54 bits.
	// Product can be up to ~108 bits. We need 128-bit multiplication.
	re := xe + ye
	lo, hi := mullu(xc, yc)

	if hi == 0 {
		// Product fits in 64 bits
		return decimal64frombits(bid64Normalize(rs, re, lo))
	}

	// Product needs more than 64 bits.
	// We need to reduce it to at most 16 decimal digits.
	// Divide by powers of 10 until it fits.
	// Use repeated division by 10 with the 128-bit value.
	rc := mul128to16digits(hi, lo, &re)

	return decimal64frombits(bid64Normalize(rs, re, rc))
}

// mul128to16digits reduces a 128-bit number (hi:lo) to at most 16 decimal digits
// by dividing by 10 repeatedly, adjusting *exp. Uses round-to-nearest-even.
func mul128to16digits(hi, lo uint64, exp *int) uint64 {
	// First, estimate how many digits to remove.
	// The max product is (10^16-1)^2 < 10^32, which is about 106 bits.
	// We need to get down to 16 digits (about 54 bits).
	// That means removing about 16 digits.

	// We'll divide the 128-bit number by 10 repeatedly.
	// Track the last remainder for rounding.
	var lastRem uint64

	for hi > 0 || lo > bid64MaxCoeff {
		var newHi, newLo uint64
		if hi > 0 {
			newHi = hi / 10
			r := hi % 10
			newLo, lastRem = divlu(r, lo, 10)
			hi = newHi
			lo = newLo
		} else {
			lastRem = lo % 10
			lo = lo / 10
		}
		*exp++
	}

	// Round-to-nearest-even
	if lastRem > 5 || (lastRem == 5 && lo%2 != 0) {
		lo++
	}

	return lo
}

// ddiv64 computes x / y for BID-encoded decimal64 values.
func ddiv64(x, y decimal64) decimal64 {
	xb := decimal64bits(x)
	yb := decimal64bits(y)

	// Handle NaN
	if bid64IsNaN(xb) || bid64IsNaN(yb) {
		return decimal64frombits(bid64NaN)
	}

	xs := bid64Sign(xb)
	ys := bid64Sign(yb)
	rs := xs ^ ys

	xInf := bid64IsInf(xb)
	yInf := bid64IsInf(yb)

	// Inf / Inf = NaN
	if xInf && yInf {
		return decimal64frombits(bid64NaN)
	}
	// Inf / y = Inf (with sign)
	if xInf {
		return decimal64frombits(rs | bid64Inf)
	}
	// x / Inf = 0 (with sign)
	if yInf {
		return decimal64frombits(bid64Pack(rs, 0, 0))
	}

	_, xe, xc := bid64Unpack(xb)
	_, ye, yc := bid64Unpack(yb)

	// Division by zero
	if yc == 0 {
		if xc == 0 {
			return decimal64frombits(bid64NaN) // 0/0
		}
		return decimal64frombits(rs | bid64Inf) // x/0
	}

	// 0 / y = 0
	if xc == 0 {
		re := xe - ye
		if re < bid64MinExp {
			re = bid64MinExp
		}
		if re > bid64MaxExp {
			re = bid64MaxExp
		}
		return decimal64frombits(bid64Pack(rs, re, 0))
	}

	// Compute quotient with 16 significant digits of precision.
	// Scale xc up so that the quotient xc_scaled / yc has ~16 digits.
	// We want to find the largest power of 10 we can multiply xc by
	// while keeping the 128-bit result's hi part < yc (requirement of divlu).
	re := xe - ye
	preferredExp := xe + ye // preferred quantum for the quotient

	// First try: scale xc by 10^16 and reduce if needed.
	// We need scaledHi < yc for divlu.
	scale := 16
	scaledLo, scaledHi := mullu(xc, pow10_16)

	for scaledHi >= yc && scale > 0 {
		// Reduce scale: divide scaled value by 10
		// This is equivalent to using a smaller power of 10
		scale--
		scaledLo, scaledHi = mullu(xc, pow10tab[scale])
	}
	re -= scale

	if scaledHi >= yc {
		// yc is very small (e.g., 1) and even xc alone has hi >= yc.
		// Use a different approach: divide directly, then scale result.
		q := xc / yc
		rem := xc % yc

		// We have q with potentially fewer than 16 digits. Scale up.
		for bid64NumDigits(q) < 16 && rem > 0 {
			q *= 10
			rem *= 10
			re--
			q += rem / yc
			rem = rem % yc
		}

		// Round-to-nearest-even
		doubleRem := rem * 2
		if doubleRem > yc || (doubleRem == yc && q%2 != 0) {
			q++
		}

		// Strip trailing zeros to preferred quantum.
		for re < preferredExp && q > 0 && q%10 == 0 {
			q /= 10
			re++
		}
		return decimal64frombits(bid64Normalize(rs, re, q))
	}

	q, rem := divlu(scaledHi, scaledLo, yc)

	// Round-to-nearest-even based on remainder.
	doubleRem := rem * 2
	if doubleRem > yc || (doubleRem == yc && q%2 != 0) {
		q++
	}

	// Strip trailing zeros to preferred quantum.
	for re < preferredExp && q > 0 && q%10 == 0 {
		q /= 10
		re++
	}

	return decimal64frombits(bid64Normalize(rs, re, q))
}

// deq64 reports whether x == y for BID-encoded decimal64 values.
// NaN is not equal to anything, including itself.
//
//go:nosplit
func deq64(x, y decimal64) bool {
	xb := decimal64bits(x)
	yb := decimal64bits(y)

	// NaN != anything
	if bid64IsNaN(xb) || bid64IsNaN(yb) {
		return false
	}

	// Inf == Inf only if same sign
	xInf := bid64IsInf(xb)
	yInf := bid64IsInf(yb)
	if xInf && yInf {
		return bid64Sign(xb) == bid64Sign(yb)
	}
	if xInf || yInf {
		return false
	}

	xs, xe, xc := bid64Unpack(xb)
	ys, ye, yc := bid64Unpack(yb)

	// +0 == -0
	if xc == 0 && yc == 0 {
		return true
	}

	// Different signs
	if xs != ys {
		return false
	}

	// Same sign. If same exponent, compare coefficients directly.
	if xe == ye {
		return xc == yc
	}

	// Different exponents: normalize to compare.
	// Scale the one with smaller exponent up.
	return bid64CompareEqual(xe, xc, ye, yc)
}

// bid64CompareEqual compares two positive decimal values with
// different exponents for equality.
// xc * 10^xe == yc * 10^ye  iff  xc == yc * 10^(ye-xe).
func bid64CompareEqual(xe int, xc uint64, ye int, yc uint64) bool {
	// Make xe <= ye so diff >= 0.
	if xe > ye {
		xe, xc, ye, yc = ye, yc, xe, xc
	}
	diff := ye - xe
	if diff > 17 {
		return false
	}

	// Scale yc up by 10^diff (yc has the larger exponent,
	// so its coefficient is typically smaller).
	for i := 0; i < diff; i++ {
		yc *= 10
		if yc > xc {
			return false
		}
	}
	return xc == yc
}

// dlt64 reports whether x < y for BID-encoded decimal64 values.
// Any comparison involving NaN returns false.
func dlt64(x, y decimal64) bool {
	xb := decimal64bits(x)
	yb := decimal64bits(y)

	// NaN: any comparison with NaN is false
	if bid64IsNaN(xb) || bid64IsNaN(yb) {
		return false
	}

	// Handle infinities
	xInf := bid64IsInf(xb)
	yInf := bid64IsInf(yb)
	xNeg := bid64Sign(xb) != 0
	yNeg := bid64Sign(yb) != 0

	if xInf && yInf {
		// -Inf < +Inf; +Inf is not < anything; -Inf is not < -Inf
		return xNeg && !yNeg
	}
	if xInf {
		// -Inf < anything (except -Inf, handled above)
		return xNeg
	}
	if yInf {
		// anything < +Inf (except +Inf, handled above)
		return !yNeg
	}

	xs, xe, xc := bid64Unpack(xb)
	ys, ye, yc := bid64Unpack(yb)

	// Handle zeros (+0 == -0, neither is less)
	if xc == 0 && yc == 0 {
		return false
	}
	if xc == 0 {
		// 0 < y only if y is positive
		return ys == 0 && yc > 0
	}
	if yc == 0 {
		// x < 0 only if x is negative
		return xs != 0
	}

	// Different signs
	if xs != ys {
		return xs != 0 // negative < positive
	}

	// Same sign: compare magnitudes
	neg := xs != 0
	cmp := bid64CompareMagnitude(xe, xc, ye, yc)
	if neg {
		return cmp > 0 // For negatives, larger magnitude means smaller value
	}
	return cmp < 0
}

// bid64CompareMagnitude compares |a| vs |b| where a has exponent ae, coefficient ac.
// Returns -1, 0, or +1.
func bid64CompareMagnitude(ae int, ac uint64, be int, bc uint64) int {
	if ae == be {
		if ac < bc {
			return -1
		}
		if ac > bc {
			return 1
		}
		return 0
	}

	// Different exponents. Try to align.
	// Make ae <= be, scale ac up.
	if ae > be {
		ae, ac, be, bc = be, bc, ae, ac
		// Swap means we need to negate the result
		cmp := bid64CompareMagnitude(ae, ac, be, bc)
		return -cmp
	}

	diff := be - ae

	// Strip trailing zeros from ac to reduce diff.
	for diff > 0 && ac%10 == 0 {
		ac /= 10
		diff--
	}
	if diff == 0 {
		if ac < bc {
			return -1
		}
		if ac > bc {
			return 1
		}
		return 0
	}
	if diff > 17 {
		// bc has much larger exponent, so |b| > |a| unless bc is 0
		if bc == 0 {
			if ac == 0 {
				return 0
			}
			return 1
		}
		return -1
	}

	// Scale bc up by 10^diff to align at exponent ae.
	// (bc has the larger exponent, so bc * 10^diff aligns it to ae.)
	for i := 0; i < diff; i++ {
		if bc > pow10_18/10 {
			// bc would overflow. Scale ac down instead (loses precision
			// but difference is already large enough to determine order).
			remaining := diff - i
			for j := 0; j < remaining; j++ {
				ac /= 10
			}
			if ac > bc {
				return 1
			}
			if ac < bc {
				return -1
			}
			return 0
		}
		bc *= 10
	}

	if ac < bc {
		return -1
	}
	if ac > bc {
		return 1
	}
	return 0
}

// dle64 reports whether x <= y for BID-encoded decimal64 values.
// Any comparison involving NaN returns false.
//
//go:nosplit
func dle64(x, y decimal64) bool {
	if bid64IsNaN(decimal64bits(x)) || bid64IsNaN(decimal64bits(y)) {
		return false
	}
	return deq64(x, y) || dlt64(x, y)
}

// di64tod64 converts an int64 to a decimal64.
func di64tod64(x int64) decimal64 {
	if x == 0 {
		return decimal64frombits(bid64Pack(0, 0, 0))
	}
	var sign uint64
	var abs uint64
	if x < 0 {
		sign = bid64SignBit
		abs = uint64(-x)
	} else {
		abs = uint64(x)
	}

	// int64 range: -9223372036854775808 to 9223372036854775807
	// Max abs value: 9223372036854775808, which is 19 digits.
	// decimal64 can hold 16 digits. If abs > 10^16-1, we need to round.
	if abs <= bid64MaxCoeff {
		return decimal64frombits(bid64Pack(sign, 0, abs))
	}

	return decimal64frombits(bid64Normalize(sign, 0, abs))
}

// dd64toi64 converts a decimal64 to int64.
// Truncates toward zero. Returns 0 for NaN/Inf or out-of-range values.
func dd64toi64(x decimal64) int64 {
	xb := decimal64bits(x)
	if bid64IsNaN(xb) || bid64IsInf(xb) {
		return 0
	}

	sign, exp, coeff := bid64Unpack(xb)
	if coeff == 0 {
		return 0
	}

	// Apply exponent
	var val uint64
	if exp >= 0 {
		val = coeff
		for i := 0; i < exp; i++ {
			if val > (1<<63)/10 {
				// Overflow
				return 0
			}
			val *= 10
		}
	} else {
		// Negative exponent: divide (truncate toward zero)
		val = coeff
		for i := 0; i < -exp; i++ {
			val /= 10
		}
	}

	if sign != 0 {
		if val > 1<<63 {
			return 0 // Overflow for negative
		}
		return -int64(val)
	}
	if val > 1<<63-1 {
		return 0 // Overflow for positive
	}
	return int64(val)
}

// du64tod64 converts a uint64 to a decimal64.
func du64tod64(x uint64) decimal64 {
	if x == 0 {
		return decimal64frombits(bid64Pack(0, 0, 0))
	}
	if x <= bid64MaxCoeff {
		return decimal64frombits(bid64Pack(0, 0, x))
	}
	return decimal64frombits(bid64Normalize(0, 0, x))
}

// dd64tou64 converts a decimal64 to uint64.
// Truncates toward zero. Returns 0 for negative, NaN, Inf, or out-of-range values.
func dd64tou64(x decimal64) uint64 {
	xb := decimal64bits(x)
	if bid64IsNaN(xb) || bid64IsInf(xb) {
		return 0
	}

	sign, exp, coeff := bid64Unpack(xb)
	if coeff == 0 {
		return 0
	}
	if sign != 0 {
		return 0 // Negative -> 0 for unsigned conversion
	}

	var val uint64
	if exp >= 0 {
		val = coeff
		for i := 0; i < exp; i++ {
			next := val * 10
			if next/10 != val {
				return 0 // Overflow
			}
			val = next
		}
	} else {
		val = coeff
		for i := 0; i < -exp; i++ {
			val /= 10
		}
	}
	return val
}

// df64tod64 converts a float64 to a decimal64.
func df64tod64(x float64) decimal64 {
	// Handle special float64 values
	sign := float64bits(x) & (1 << 63)
	dsign := uint64(0)
	if sign != 0 {
		dsign = bid64SignBit
	}

	// NaN
	if x != x {
		return decimal64frombits(bid64NaN)
	}
	// Infinity
	if x > 0 && x+x == x {
		return decimal64frombits(dsign | bid64Inf)
	}
	if x < 0 && x+x == x {
		return decimal64frombits(dsign | bid64Inf)
	}
	// Zero
	if x == 0 {
		return decimal64frombits(bid64Pack(dsign, 0, 0))
	}

	f := x
	if f < 0 {
		f = -f
	}

	// Convert the float64 value to 16 significant decimal digits.
	var coeff uint64
	dexp := 0

	// Normalize f to [1, 10)
	fval := f
	for fval >= 10 {
		dexp++
		fval /= 10
	}
	for fval < 1 {
		dexp--
		fval *= 10
	}

	// Extract 16 digits
	for i := 0; i < 16; i++ {
		digit := uint64(fval)
		coeff = coeff*10 + digit
		fval -= float64(digit)
		fval *= 10
	}
	dexp -= 15 // We extracted 16 digits, so adjust exponent

	// Round the last digit
	if uint64(fval) >= 5 {
		coeff++
		if coeff >= pow10_16 {
			coeff /= 10
			dexp++
		}
	}

	return decimal64frombits(bid64Normalize(dsign, dexp, coeff))
}

// dd64tof64 converts a decimal64 to float64.
func dd64tof64(x decimal64) float64 {
	xb := decimal64bits(x)
	if bid64IsNaN(xb) {
		return float64frombits(0x7FF8000000000000) // quiet NaN
	}
	if bid64IsInf(xb) {
		if bid64Sign(xb) != 0 {
			return float64frombits(0xFFF0000000000000) // -Inf
		}
		return float64frombits(0x7FF0000000000000) // +Inf
	}

	sign, exp, coeff := bid64Unpack(xb)
	if coeff == 0 {
		if sign != 0 {
			return float64frombits(0x8000000000000000) // -0
		}
		return 0 // +0
	}

	// Compute coeff * 10^exp as a float64
	// Build the result carefully to avoid importing math.
	result := float64(coeff)

	if exp > 0 {
		// Multiply by 10^exp
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
		// Divide by 10^(-exp)
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

// printdecimal64 prints a BID-encoded decimal64 value for the builtin println.
// printdecimal64 formats a decimal64 value using the same 'g' format
// as printfloat on Go tip: shortest representation, switching between
// fixed and scientific notation. Since decimal64 is already base-10,
// the shortest representation is just the coefficient with trailing
// zeros stripped.
func printdecimal64(x decimal64) {
	xb := decimal64bits(x)
	if bid64IsNaN(xb) {
		printstring("NaN")
		return
	}
	if bid64IsInf(xb) {
		if bid64Sign(xb) != 0 {
			printstring("-Inf")
		} else {
			printstring("+Inf")
		}
		return
	}

	sign, exp, coeff := bid64Unpack(xb)

	if sign != 0 {
		printstring("-")
	}

	if coeff == 0 {
		printstring("0")
		return
	}

	// Strip trailing zeros from the coefficient.
	for coeff%10 == 0 {
		coeff /= 10
		exp++
	}

	// Extract digits (most-significant first).
	var digits [16]byte
	ndigits := 0
	c := coeff
	for c > 0 {
		digits[ndigits] = byte(c%10) + '0'
		c /= 10
		ndigits++
	}
	for i, j := 0, ndigits-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}

	// The value is digits * 10^exp.  In scientific notation:
	//   digits[0].digits[1:] × 10^(exp+ndigits-1)
	// Use 'g' rules: scientific if e < -4 or e >= ndigits.
	e := exp + ndigits - 1

	if e < -4 || e >= ndigits {
		// Scientific notation: d.dddde±dd
		var buf [24]byte
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
		if e >= 100 {
			buf[w] = byte(e/100) + '0'
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
		// Integer: digits followed by trailing zeros.
		gwrite(digits[:ndigits])
		for i := 0; i < exp; i++ {
			printstring("0")
		}
	} else {
		// Fixed: decimal point within or before digits.
		// intPart = ndigits + exp  (number of digits before the '.')
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

// dmin64 returns the minimum of two decimal64 values.
// IEEE 754-2019 minimum semantics: NaN propagates, min(-0,+0) = -0.
func dmin64(x, y decimal64) decimal64 {
	xb := decimal64bits(x)
	yb := decimal64bits(y)

	if bid64IsNaN(xb) || bid64IsNaN(yb) {
		return decimal64frombits(bid64NaN)
	}

	if dlt64(y, x) {
		return y
	}
	if dlt64(x, y) {
		return x
	}
	// x == y; prefer the one with sign bit set (i.e. -0 < +0)
	if bid64Sign(xb) != 0 {
		return x
	}
	return y
}

// dmax64 returns the maximum of two decimal64 values.
// IEEE 754-2019 maximum semantics: NaN propagates, max(-0,+0) = +0.
func dmax64(x, y decimal64) decimal64 {
	xb := decimal64bits(x)
	yb := decimal64bits(y)

	if bid64IsNaN(xb) || bid64IsNaN(yb) {
		return decimal64frombits(bid64NaN)
	}

	if dlt64(x, y) {
		return y
	}
	if dlt64(y, x) {
		return x
	}
	// x == y; prefer the one without sign bit (i.e. +0 > -0)
	if bid64Sign(xb) == 0 {
		return x
	}
	return y
}
