// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package strconv

import "math/bits"

// This file is ParseFloat without the parsing: it converts a value already
// held as an integer coefficient and a power of ten, for internal/bid, whose
// decimal floating-point formats hold exactly that. It adds the two things
// ParseFloat has no use for: a coefficient of up to 128 bits and a choice of
// rounding direction.

// A Rounding says how Float64FromCoeff and Float32FromCoeff round a value to
// the binary format. The directions are of the magnitude, so a caller holding
// a rounding toward positive or negative infinity chooses between
// RoundTowardZero and RoundAwayFromZero by the sign of the value.
type Rounding uint8

const (
	RoundNearestEven  Rounding = iota // nearest, ties to the even significand (what ParseFloat does)
	RoundNearestAway                  // nearest, ties away from zero
	RoundTowardZero                   // the neighbor of smaller magnitude
	RoundAwayFromZero                 // the neighbor of larger magnitude
)

// Float64FromCoeff returns (-1)^neg × (hi×2^64 + lo) × 10^exp10 rounded once,
// in direction r, to float64. The result has float64's gradual underflow and
// signed zeros. A value too large for the format gives the infinity of its
// sign, or the largest finite magnitude under RoundTowardZero; the sign of a
// zero coefficient is kept. Float64FromCoeff does not allocate.
//
// Any 128-bit coefficient is accepted. Precondition: exp10 is small enough
// that adding 39 to its magnitude does not overflow an int.
func Float64FromCoeff(hi, lo uint64, exp10 int, neg bool, r Rounding) float64 {
	if optimize && r == RoundNearestEven && hi>>63 == 0 {
		man, k, sticky := coeffPrefix(hi, lo)
		f, ok := eiselLemire64(man, exp10+k, neg)
		if ok {
			if !sticky {
				return f
			}
			// The exact value lies strictly between man and man+1 in the
			// last place kept. Rounding is monotonic, so if both round to
			// the same float64 then so does the value. As in atof64, man+1
			// wrapping to zero gives a different f and takes the slow path.
			fUp, ok := eiselLemire64(man+1, exp10+k, neg)
			if ok && f == fUp {
				return f
			}
		}
	}
	return float64frombits(coeffFloatBits(hi, lo, exp10, neg, r, &float64info))
}

// Float32FromCoeff is Float64FromCoeff with a float32 result. It rounds once,
// from the exact value directly to float32, and never by way of float64.
func Float32FromCoeff(hi, lo uint64, exp10 int, neg bool, r Rounding) float32 {
	if optimize && r == RoundNearestEven && hi>>63 == 0 {
		man, k, sticky := coeffPrefix(hi, lo)
		f, ok := eiselLemire32(man, exp10+k, neg)
		if ok {
			if !sticky {
				return f
			}
			// See Float64FromCoeff.
			fUp, ok := eiselLemire32(man+1, exp10+k, neg)
			if ok && f == fUp {
				return f
			}
		}
	}
	return float32frombits(uint32(coeffFloatBits(hi, lo, exp10, neg, r, &float32info)))
}

// coeffPrefix returns man = ⌊c / 10^k⌋ for the coefficient c = hi×2^64 + lo,
// where k is small enough that man fits 64 bits, and whether the digits
// dropped were non-zero. Precondition: hi>>63 == 0, which keeps k at most 19
// and so 10^k within a uint64.
func coeffPrefix(hi, lo uint64) (man uint64, k int, sticky bool) {
	if hi == 0 {
		return lo, 0, false
	}
	// c < 2^(64+n) ≤ 2^64 × 10^k, so the quotient fits and hi < 10^k.
	n := bits.Len64(hi)
	k = log10Pow2(n) + 1
	man, rem := bits.Div64(hi, lo, uint64pow10[k])
	return man, k, rem != 0
}

// coeffFloatBits is the exact path of Float64FromCoeff and Float32FromCoeff,
// returning the bits of the result in format flt. It decides the values
// plainly outside the format's range from the size of the coefficient alone.
// The rest it converts through the decimal type, as ParseFloat's slow path
// does.
//
// The decimal type is exact here. It holds 800 digits, and Shift is its only
// operation that adds any. Each stage of floatBits holds c × 10^q × 2^m, with
// m the net binary shift so far. Where m ≥ 0 that has at most the digits of
// c × 2^m, and m stays near 1133 at most for a value not decided first (the
// 1075 of the smallest subnormal and the 53 of the significand, and a few
// bits of overshoot): about 39 + 341 digits. Where m < 0 it has at most the
// digits of c × 5^-m, and m stays above about -1030: 39 + 720 digits. So
// nothing is truncated, and the fraction left once the significand is
// extracted is the exact remainder, which is what the directed roundings
// need. The decimal lives on the stack: it does not escape, and 800 bytes
// cost little beside the shifts.
func coeffFloatBits(hi, lo uint64, exp10 int, neg bool, r Rounding, flt *floatInfo) uint64 {
	var sign uint64
	if neg {
		sign = 1 << flt.mantbits << flt.expbits
	}
	if hi|lo == 0 {
		return sign
	}

	// With n the bit length of c, 10^log10Pow2(n-1) ≤ c < 10^(log10Pow2(n)+1).
	n := bits.Len64(lo)
	if hi != 0 {
		n = 64 + bits.Len64(hi)
	}

	// The value is at least the power of ten above 2^(2^(expbits-1)), which
	// is the first magnitude beyond every finite value of the format.
	if log10Pow2(n-1)+exp10 > log10Pow2(1<<(flt.expbits-1)) {
		if r == RoundTowardZero {
			return sign | (1<<flt.expbits-2)<<flt.mantbits | (1<<flt.mantbits - 1)
		}
		return sign | (1<<flt.expbits-1)<<flt.mantbits
	}
	// The value is below the power of ten under 2^(bias-mantbits), which is
	// half the smallest subnormal.
	if log10Pow2(n)+1+exp10 <= log10Pow2(flt.bias-int(flt.mantbits)) {
		if r == RoundAwayFromZero {
			return sign | 1
		}
		return sign
	}

	var d decimal
	d.assignCoeff(hi, lo, exp10, neg)
	b, _ := d.floatBits(flt, r)
	return b
}

// assignCoeff sets d to (-1)^neg × (hi×2^64 + lo) × 10^exp10.
func (d *decimal) assignCoeff(hi, lo uint64, exp10 int, neg bool) {
	var buf [39]byte // 2^128 has 39 decimal digits
	i := len(buf)
	for hi != 0 || lo != 0 {
		rem := hi % 10
		hi /= 10
		lo, rem = bits.Div64(rem, lo, 10)
		i--
		buf[i] = byte(rem) + '0'
	}
	d.nd = copy(d.d[:], buf[i:])
	d.dp = d.nd + exp10
	d.neg = neg
	trim(d)
}
