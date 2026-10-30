// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package math

import "internal/bid"

// Decimal64Trunc returns the integer value of d, rounding toward zero.
// The result has exponent max(Q(d), 0), where Q(d) is the exponent of d.
//
// Special cases are:
//
//	Decimal64Trunc(±0) = ±0
//	Decimal64Trunc(±Inf) = ±Inf
//	Decimal64Trunc(NaN) = NaN
func Decimal64Trunc(d decimal64) decimal64 {
	return Decimal64frombits(bid.RoundToIntegral64(Decimal64bits(d), bid.TowardZero))
}

// Decimal64Floor returns the greatest integer value less than or equal to d.
// The result has exponent max(Q(d), 0), where Q(d) is the exponent of d.
//
// Special cases are:
//
//	Decimal64Floor(±0) = ±0
//	Decimal64Floor(±Inf) = ±Inf
//	Decimal64Floor(NaN) = NaN
func Decimal64Floor(d decimal64) decimal64 {
	return Decimal64frombits(bid.RoundToIntegral64(Decimal64bits(d), bid.TowardNegative))
}

// Decimal64Ceil returns the least integer value greater than or equal to d.
// The result has exponent max(Q(d), 0), where Q(d) is the exponent of d.
//
// Special cases are:
//
//	Decimal64Ceil(±0) = ±0
//	Decimal64Ceil(±Inf) = ±Inf
//	Decimal64Ceil(NaN) = NaN
func Decimal64Ceil(d decimal64) decimal64 {
	return Decimal64frombits(bid.RoundToIntegral64(Decimal64bits(d), bid.TowardPositive))
}

// Decimal64Round returns the nearest integer, rounding half away from zero.
// The result has exponent max(Q(d), 0), where Q(d) is the exponent of d.
//
// Special cases are:
//
//	Decimal64Round(±0) = ±0
//	Decimal64Round(±Inf) = ±Inf
//	Decimal64Round(NaN) = NaN
func Decimal64Round(d decimal64) decimal64 {
	return Decimal64frombits(bid.RoundToIntegral64(Decimal64bits(d), bid.ToNearestAway))
}

// Decimal64RoundToEven returns the nearest integer, rounding ties to even.
// The result has exponent max(Q(d), 0), where Q(d) is the exponent of d.
//
// Special cases are:
//
//	Decimal64RoundToEven(±0) = ±0
//	Decimal64RoundToEven(±Inf) = ±Inf
//	Decimal64RoundToEven(NaN) = NaN
func Decimal64RoundToEven(d decimal64) decimal64 {
	return Decimal64frombits(bid.RoundToIntegral64(Decimal64bits(d), bid.ToNearestEven))
}

// --- decimal128 versions ---

// Decimal128Trunc returns the integer value of d, rounding toward zero.
// The result has exponent max(Q(d), 0), where Q(d) is the exponent of d.
//
// Special cases are:
//
//	Decimal128Trunc(±0) = ±0
//	Decimal128Trunc(±Inf) = ±Inf
//	Decimal128Trunc(NaN) = NaN
func Decimal128Trunc(d decimal128) decimal128 {
	return fromD128(bid.RoundToIntegral128(d128(d), bid.TowardZero))
}

// Decimal128Floor returns the greatest integer value less than or equal to d.
// The result has exponent max(Q(d), 0), where Q(d) is the exponent of d.
//
// Special cases are:
//
//	Decimal128Floor(±0) = ±0
//	Decimal128Floor(±Inf) = ±Inf
//	Decimal128Floor(NaN) = NaN
func Decimal128Floor(d decimal128) decimal128 {
	return fromD128(bid.RoundToIntegral128(d128(d), bid.TowardNegative))
}

// Decimal128Ceil returns the least integer value greater than or equal to d.
// The result has exponent max(Q(d), 0), where Q(d) is the exponent of d.
//
// Special cases are:
//
//	Decimal128Ceil(±0) = ±0
//	Decimal128Ceil(±Inf) = ±Inf
//	Decimal128Ceil(NaN) = NaN
func Decimal128Ceil(d decimal128) decimal128 {
	return fromD128(bid.RoundToIntegral128(d128(d), bid.TowardPositive))
}

// Decimal128Round returns the nearest integer, rounding half away from zero.
// The result has exponent max(Q(d), 0), where Q(d) is the exponent of d.
//
// Special cases are:
//
//	Decimal128Round(±0) = ±0
//	Decimal128Round(±Inf) = ±Inf
//	Decimal128Round(NaN) = NaN
func Decimal128Round(d decimal128) decimal128 {
	return fromD128(bid.RoundToIntegral128(d128(d), bid.ToNearestAway))
}

// Decimal128RoundToEven returns the nearest integer, rounding ties to even.
// The result has exponent max(Q(d), 0), where Q(d) is the exponent of d.
//
// Special cases are:
//
//	Decimal128RoundToEven(±0) = ±0
//	Decimal128RoundToEven(±Inf) = ±Inf
//	Decimal128RoundToEven(NaN) = NaN
func Decimal128RoundToEven(d decimal128) decimal128 {
	return fromD128(bid.RoundToIntegral128(d128(d), bid.ToNearestEven))
}
