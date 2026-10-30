// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package strconv

type Uint128 = uint128

const (
	Pow10Min = pow10Min
	Pow10Max = pow10Max
)

var (
	MulLog10_2       = mulLog10_2
	MulLog2_10       = mulLog2_10
	ParseFloatPrefix = parseFloatPrefix
	Pow10            = pow10
	Umul128          = umul128
	Umul192          = umul192
	Div5Tab          = div5Tab
	DivisiblePow5    = divisiblePow5
	TrimZeros        = trimZeros
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
