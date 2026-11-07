// asmcheck
//go:build goexperiment.decimal

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codegen

// On linux/ppc64le the decimal operators are POWER's decimal
// floating-point instructions, not runtime calls (see
// cmd/compile/internal/ssagen/dpd.go). Elsewhere they are calls into the
// kernel through the runtime.

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
	return x < y
}

func Add128(x, y decimal128) decimal128 {
	// ppc64le:"DADDQ" -"CALL"
	// amd64:"CALL runtime.dadd128"
	return x + y
}

func Mul128(x, y decimal128) decimal128 {
	// ppc64le:"DMULQ" -"CALL"
	return x * y
}

func Eq128(x, y decimal128) bool {
	// ppc64le:"DCMPUQ" -"CALL"
	return x == y
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
