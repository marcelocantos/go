// asmcheck
//go:build goexperiment.decimal

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codegen

import "math/decimal"

// On linux/ppc64le the decimal operators are POWER's decimal
// floating-point instructions, not runtime calls (see
// cmd/compile/internal/ssagen/dpd.go). BID targets use integer fast paths
// where proved exact, with runtime calls into the kernel for general cases.

func Add64(x, y decimal64) decimal64 {
	// ppc64le:"DADD" -"CALL"
	// amd64:"CALL internal/bid.addSlow64" -"CALL runtime.dadd64"
	// arm64:"CALL internal/bid.addSlow64" -"CALL runtime.dadd64"
	return x + y
}

func MulAdd64(x, y, z decimal64) decimal64 {
	// ppc64le:"DMUL" "DADD" -"CALL"
	return x*y + z
}

func Sub64(x, y decimal64) decimal64 {
	// ppc64le:"DSUB" -"CALL"
	// amd64:"CALL internal/bid.addSlow64" -"CALL runtime.dsub64"
	// arm64:"CALL internal/bid.addSlow64" -"CALL runtime.dsub64"
	return x - y
}

func Mul64(x, y decimal64) decimal64 {
	// ppc64le:"DMUL" -"CALL"
	// amd64:"CALL internal/bid.mulSlow64" -"CALL runtime.dmul64"
	// arm64:"CALL internal/bid.mulSlow64" -"CALL runtime.dmul64"
	return x * y
}

func Div64(x, y decimal64) decimal64 {
	// ppc64le:"DDIV" -"CALL"
	// amd64:"CALL internal/bid.div64" -"CALL runtime.ddiv64"
	// arm64:"CALL internal/bid.div64" -"CALL runtime.ddiv64"
	return x / y
}

func Less64(x, y decimal64) bool {
	// ppc64le:"DCMPU" -"CALL"
	// arm64:"UMULH" -"CALL runtime.dlt64"
	// amd64:"MULX?Q" -"CALL runtime.dlt64"
	return x < y
}

func Add128(x, y decimal128) decimal128 {
	// ppc64le:"DADDQ" -"CALL"
	// UBD (the default, ubdLevel 10): decimal128-go's first-level add path
	// narrowed to one-word coefficients, one guard ((xh^yh) | xh<<15 == 0),
	// in front of the kernel's ties-to-even entry; operands of opposite
	// signs go to the kernel.
	// amd64:"SHLQ\t[$]15" "ADCQ\t[$]0" "CALL internal/bid.ubdAddTTE128" -"NEGQ" -"CALL runtime.dadd128"
	// arm64:"ORR\tR[0-9]+<<15" "ADC" "CALL internal/bid.ubdAddTTE128" -"CSNEG" -"CALL runtime.dadd128"
	return x + y
}

func Sub128(x, y decimal128) decimal128 {
	// ppc64le:"DSUBQ" -"CALL"
	// UBD (the default, ubdLevel 10): the second-level add path narrowed to
	// one-word coefficients: one borrow, the difference negated by the
	// borrow's mask, +0 for an exact zero; in front of the kernel's
	// ties-to-even entry.
	// arm64:"ORR\tR[0-9]+<<15" "NGC" "CSEL" -"SBCS" "CALL internal/bid.ubdSubTTE128" -"CALL runtime.dsub128"
	// amd64:"SHLQ\t[$]15" "SBBQ" "CMOVQEQ" "CALL internal/bid.ubdSubTTE128" -"CALL runtime.dsub128"
	return x - y
}

func Mul128(x, y decimal128) decimal128 {
	// ppc64le:"DMULQ" -"CALL"
	// UBD (the default, ubdLevel 5 and up): decimal128-go's first-level
	// multiply path, coefficients below 2^56, in front of the kernel's
	// ties-to-even entry.
	// arm64:"UMULH" "CALL internal/bid.ubdMulTTE128" -"CALL runtime.dmul128"
	// amd64:"MULX?Q" "CALL internal/bid.ubdMulTTE128" -"CALL runtime.dmul128"
	return x * y
}

func Eq128(x, y decimal128) bool {
	// ppc64le:"DCMPUQ" -"CALL"
	// UBD (the default, ubdLevel 10): operands of one qExp and one sign and
	// one-word coefficients compared inline, with no NaN tag test (a NaN's
	// coefficient prefix is in the high word), in front of the kernel's
	// predicate entry.
	// arm64:"ORR\tR[0-9]+<<15" -"LSR\t[$]59" "CALL internal/bid.ubdEqual128" -"ubdCompare128" -"UMULH" -"CALL runtime.deq128"
	// amd64:"SHLQ\t[$]15" -"SHRQ\t[$]59" "CALL internal/bid.ubdEqual128" -"ubdCompare128" -"MULX?Q" -"CALL runtime.deq128"
	return x == y
}

func Less128(x, y decimal128) bool {
	// UBD (the default, ubdLevel 10): the low words XORed with the sign
	// mask and compared once, no borrow chain, in front of the kernel's
	// predicate entry. (At ubdLevel 9 the two-word borrow is read from the
	// flags by an ARM64.rules rule: CSET LO, not NGC; CMN.)
	// arm64:"ORR\tR[0-9]+<<15" "CSET\tHI" -"SBCS" -"NGC" "CALL internal/bid.ubdLess128" -"ubdCompare128"
	// amd64:"SHLQ\t[$]15" "SETHI" -"SBBQ" "CALL internal/bid.ubdLess128" -"ubdCompare128"
	return x < y
}

func Greater128(x, y decimal128) bool {
	// UBD: its own entry, with the operands where they are.
	// arm64:"ORR\tR[0-9]+<<15" -"SBCS" "CALL internal/bid.ubdGreater128" -"ubdLess128"
	// amd64:"SHLQ\t[$]15" -"SBBQ" "CALL internal/bid.ubdGreater128" -"ubdLess128"
	return x > y
}

func Zero64(x decimal64) bool {
	// arm64:-"CALL"
	// amd64:-"CALL"
	return x == 0
}

func Nonzero64(x decimal64) bool {
	// arm64:-"CALL"
	// amd64:-"CALL"
	return 0.00 != x
}

func Zero128(x decimal128) bool {
	// UBD (the default, ubdLevel 9): the coefficient bits, (hi<<15)|lo,
	// tested inline; a special's coefficient prefix makes it nonzero.
	// arm64:"ORR" -"CALL"
	// amd64:"SHLQ" -"CALL"
	return 0 == x
}

func Nonzero128(x decimal128) bool {
	// UBD (the default, ubdLevel 9): the coefficient bits, (hi<<15)|lo,
	// tested inline; a special's coefficient prefix makes it nonzero.
	// arm64:"ORR" -"CALL"
	// amd64:"SHLQ" -"CALL"
	return x != 0.00
}

func LessZero64(x decimal64) bool {
	// arm64:-"CALL"
	// amd64:-"CALL"
	return x < 0
}

func LessEqualZero64(x decimal64) bool {
	// arm64:-"CALL"
	// amd64:-"CALL"
	return x <= 0
}

func GreaterZero64(x decimal64) bool {
	// arm64:-"CALL"
	// amd64:-"CALL"
	return 0.00 < x
}

func GreaterEqualZero64(x decimal64) bool {
	// arm64:-"CALL"
	// amd64:-"CALL"
	return 0.00 <= x
}

func LessZero128(x decimal128) bool {
	// UBD (the default, ubdLevel 9): inline, with no call.
	// arm64:"ORR" -"CALL"
	// amd64:"SHLQ" -"CALL"
	return 0.00 > x
}

func LessEqualZero128(x decimal128) bool {
	// UBD (the default, ubdLevel 9): inline, with no call.
	// arm64:"ORR" -"CALL"
	// amd64:"SHLQ" -"CALL"
	return 0.00 >= x
}

func GreaterZero128(x decimal128) bool {
	// UBD (the default, ubdLevel 9): inline, with no call.
	// arm64:"ORR" -"CALL"
	// amd64:"SHLQ" -"CALL"
	return x > 0
}

func GreaterEqualZero128(x decimal128) bool {
	// UBD (the default, ubdLevel 9): inline, with no call.
	// arm64:"ORR" -"CALL"
	// amd64:"SHLQ" -"CALL"
	return x >= 0
}

func LocalZero64(x decimal64) bool {
	var zero decimal64
	// arm64:-"CALL"
	// amd64:-"CALL"
	return x < zero
}

func LocalZero128(x decimal128) bool {
	zero := decimal128(0.00)
	// arm64:"ORR" -"CALL"
	// amd64:"SHLQ" -"CALL"
	return zero <= x
}

func Widen(x decimal64) decimal128 {
	// ppc64le:"DCTQPQ" -"CALL"
	return decimal128(x)
}

func Narrow(x decimal128) decimal64 {
	// ppc64le:"DRDPQ" -"CALL"
	return decimal64(x)
}

func FromInt(x int) decimal64 {
	// ppc64le:"DCFFIX" -"CALL"
	return decimal64(x)
}

func ToInt(x decimal64) int64 {
	// ppc64le:"DRINTN" "DCTFIX" -"CALL"
	return int64(x)
}

func Min64(x, y decimal64) decimal64 {
	// ppc64le:"DCMPU" -"CALL"
	return min(x, y)
}

func Neg64(x decimal64) decimal64 {
	// ppc64le:"XOR" -"CALL" -"DADD"
	return -x
}

func ToFloat(x decimal64) float64 {
	// No instruction converts between the radices: a runtime call.
	// ppc64le:"CALL runtime.dd64tof64"
	return float64(x)
}

// A constant quantum and mode must reach the typed intrinsic through both
// generic API layers. Only the guarded fallback may call the portable helper.
func QuantizeCent64(x decimal64) decimal64 {
	// arm64:"UDIV" "CALL internal/decimal.quantize64" -"CALL .*[.]Quantize"
	// amd64:"DIVQ" "CALL internal/decimal.quantize64" -"CALL .*[.]Quantize"
	return decimal.Quantize(x, decimal64(0.01))
}
func QuantizeCent128(x decimal128) decimal128 {
	// arm64:"CALL internal/bid.ubdQuantizeTTE128" -"UDIV" -"CALL .*[.]Quantize"
	// amd64:"CALL internal/bid.ubdQuantizeTTE128" -"DIVQ" -"CALL .*[.]Quantize"
	return decimal.Quantize(x, decimal128(0.01))
}
func IntegralEven128(x decimal128) decimal128 {
	// arm64:"CALL internal/bid.ubdRoundToIntegralTTE128" -"UDIV" -"CALL .*RoundToEven"
	// amd64:"CALL internal/bid.ubdRoundToIntegralTTE128" -"DIVQ" -"CALL .*RoundToEven"
	return decimal.RoundToEven(x)
}
func IntegralTrunc64(x decimal64) decimal64 {
	// arm64:"UDIV" "CALL internal/decimal.integral64" -"CALL .*Trunc"
	// amd64:"DIVQ" "CALL internal/decimal.integral64" -"CALL .*Trunc"
	return decimal.Trunc(x)
}

func integralDispatch128(x decimal128, mode decimal.RoundingMode) decimal128 {
	switch mode {
	case decimal.ToNearestEven:
		return decimal.RoundToEven(x)
	case decimal.ToNearestAway:
		return decimal.Round(x)
	case decimal.ToZero:
		return decimal.Trunc(x)
	case decimal.ToPositiveInf:
		return decimal.Ceil(x)
	case decimal.ToNegativeInf:
		return decimal.Floor(x)
	default:
		panic("invalid rounding mode")
	}
}

func TruncThroughDispatch128(x decimal128) decimal128 {
	// arm64:"CALL internal/bid.ubdRoundToIntegralRnd128" -"UDIV" -"CALL .*integralDispatch128"
	// amd64:"CALL internal/bid.ubdRoundToIntegralRnd128" -"DIVQ" -"CALL .*integralDispatch128"
	return integralDispatch128(x, decimal.ToZero)
}
