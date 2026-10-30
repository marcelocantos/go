// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Soft-decimal conversions; see decimal64.go and decimal128.go. A conversion
// to an integer truncates, as the language requires of a conversion from a
// floating-point type; every other inexact conversion rounds to nearest, ties
// to even. A conversion to an integer whose result is out of range has an
// implementation-dependent result, here the saturated value of internal/bid.

package runtime

import "internal/bid"

func di64tod64(x int64) uint64  { return bid.Int64To64(x, bid.ToNearestEven) }
func du64tod64(x uint64) uint64 { return bid.Uint64To64(x, bid.ToNearestEven) }

func dd64toi64(x uint64) int64 {
	z, _ := bid.Int64From64(x, bid.TowardZero)
	return z
}

func dd64tou64(x uint64) uint64 {
	z, _ := bid.Uint64From64(x, bid.TowardZero)
	return z
}

func df32tod64(x float32) uint64 { return bid.Float32To64(x, bid.ToNearestEven) }
func df64tod64(x float64) uint64 { return bid.Float64To64(x, bid.ToNearestEven) }
func dd64tof32(x uint64) float32 { return bid.Float32From64(x, bid.ToNearestEven) }
func dd64tof64(x uint64) float64 { return bid.Float64From64(x, bid.ToNearestEven) }

func di64tod128(x int64) (hi, lo uint64) {
	z := bid.Int64To128(x)
	return z.Hi, z.Lo
}

func du64tod128(x uint64) (hi, lo uint64) {
	z := bid.Uint64To128(x)
	return z.Hi, z.Lo
}

func dd128toi64(hi, lo uint64) int64 {
	z, _ := bid.Int64From128(bid.Uint128{Hi: hi, Lo: lo}, bid.TowardZero)
	return z
}

func dd128tou64(hi, lo uint64) uint64 {
	z, _ := bid.Uint64From128(bid.Uint128{Hi: hi, Lo: lo}, bid.TowardZero)
	return z
}

func df32tod128(x float32) (hi, lo uint64) {
	z := bid.Float32To128(x, bid.ToNearestEven)
	return z.Hi, z.Lo
}

func df64tod128(x float64) (hi, lo uint64) {
	z := bid.Float64To128(x, bid.ToNearestEven)
	return z.Hi, z.Lo
}

func dd128tof32(hi, lo uint64) float32 {
	return bid.Float32From128(bid.Uint128{Hi: hi, Lo: lo}, bid.ToNearestEven)
}

func dd128tof64(hi, lo uint64) float64 {
	return bid.Float64From128(bid.Uint128{Hi: hi, Lo: lo}, bid.ToNearestEven)
}

func dd64tod128(x uint64) (hi, lo uint64) {
	z := bid.Widen(x)
	return z.Hi, z.Lo
}

func dd128tod64(hi, lo uint64) uint64 {
	return bid.Narrow(bid.Uint128{Hi: hi, Lo: lo}, bid.ToNearestEven)
}
