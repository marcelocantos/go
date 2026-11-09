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
	// amd64:"CALL runtime.dadd64"
	// arm64:"CALL runtime.dadd64"
	return x + y
}

func MulAdd64(x, y, z decimal64) decimal64 {
	// ppc64le:"DMUL" "DADD" -"CALL"
	return x*y + z
}

func Sub64(x, y decimal64) decimal64 {
	// ppc64le:"DSUB" -"CALL"
	return x - y
}

func Div64(x, y decimal64) decimal64 {
	// ppc64le:"DDIV" -"CALL"
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
	// amd64:"CALL runtime.dadd128"
	// arm64:"CALL runtime.dadd128" "ADDS" "ADC"
	return x + y
}

func Sub128(x, y decimal128) decimal128 {
	// ppc64le:"DSUBQ" -"CALL"
	// arm64:"CALL runtime.dsub128" "SUBS"
	// amd64:"CALL runtime.dsub128"
	return x - y
}

func Mul128(x, y decimal128) decimal128 {
	// ppc64le:"DMULQ" -"CALL"
	// arm64:"CALL runtime.dmul128" "MUL" "UMULH"
	return x * y
}

func Eq128(x, y decimal128) bool {
	// ppc64le:"DCMPUQ" -"CALL"
	// arm64:"UMULH" "CALL runtime.deq128"
	// amd64:"MULX?Q" "CALL runtime.deq128"
	return x == y
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
	// arm64:-"CALL"
	// amd64:-"CALL"
	return 0 == x
}

func Nonzero128(x decimal128) bool {
	// arm64:-"CALL"
	// amd64:-"CALL"
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
	// arm64:-"CALL"
	// amd64:-"CALL"
	return 0.00 > x
}

func LessEqualZero128(x decimal128) bool {
	// arm64:-"CALL"
	// amd64:-"CALL"
	return 0.00 >= x
}

func GreaterZero128(x decimal128) bool {
	// arm64:-"CALL"
	// amd64:-"CALL"
	return x > 0
}

func GreaterEqualZero128(x decimal128) bool {
	// arm64:-"CALL"
	// amd64:-"CALL"
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
	// arm64:-"CALL"
	// amd64:-"CALL"
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
	// arm64:"UDIV" "CALL internal/decimal.quantize64" -"CALL .*Quantize"
	// amd64:"DIVQ" "CALL internal/decimal.quantize64" -"CALL .*Quantize"
	return decimal.Quantize(x, decimal64(0.01))
}
func QuantizeCent128(x decimal128) decimal128 {
	// arm64:"UDIV" "CALL internal/decimal.quantize128" -"CALL .*Quantize"
	// amd64:"DIVQ" "CALL internal/decimal.quantize128" -"CALL .*Quantize"
	return decimal.Quantize(x, decimal128(0.01))
}
func IntegralEven128(x decimal128) decimal128 {
	// arm64:"UDIV" "CALL internal/decimal.integral128" -"CALL .*RoundToEven"
	// amd64:"DIVQ" "CALL internal/decimal.integral128" -"CALL .*RoundToEven"
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
	// arm64:"UDIV" "CALL internal/decimal.integral128" -"CALL .*integralDispatch128"
	// amd64:"DIVQ" "CALL internal/decimal.integral128" -"CALL .*integralDispatch128"
	return integralDispatch128(x, decimal.ToZero)
}
