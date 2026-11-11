// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package strconv

var (
	Log10Pow2        = log10Pow2
	Log2Pow10        = log2Pow10
	ParseFloatPrefix = parseFloatPrefix
)

func NewDecimal(i uint64) *decimal {
	d := new(decimal)
	d.Assign(i)
	return d
}

func SetOptimize(b bool) bool {
	old := optimize
	optimize = b
	return old
}

// CoeffDecimalExact reports whether the decimal of the exact path holds
// (hi×2^64 + lo) × 10^exp10 without truncation through floatBits.
func CoeffDecimalExact(hi, lo uint64, exp10 int, bitSize int) bool {
	flt := &float64info
	if bitSize == 32 {
		flt = &float32info
	}
	var d decimal
	d.assignCoeff(hi, lo, exp10, false)
	d.floatBits(flt, RoundTowardZero)
	return !d.trunc
}

// CoeffScale is coeffScale, and CoeffExact the exact path it stands in
// front of, both for a positive value rounded to nearest, ties to even.
func CoeffScale(hi, lo uint64, exp10 int, bitSize int) (uint64, bool) {
	if bitSize == 32 {
		return coeffScale(hi, lo, exp10, &float32info)
	}
	return coeffScale(hi, lo, exp10, &float64info)
}

func CoeffExact(hi, lo uint64, exp10 int, bitSize int) uint64 {
	if bitSize == 32 {
		return coeffFloatBits(hi, lo, exp10, false, RoundNearestEven, &float32info)
	}
	return coeffFloatBits(hi, lo, exp10, false, RoundNearestEven, &float64info)
}

const CoeffExactPow10 = coeffExactPow10
