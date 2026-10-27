// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// Conversions between the two decimal widths and between decimals and
// integers (IEEE 754-2008 clause 5.4.1, convertFromInt and
// convertToInteger; clause 5.4.2, convertFormat). The names put the other
// type first and the decimal width last: Int64To64 converts an int64 to
// decimal64 and Int64From64 converts a decimal64 to int64. Conversions to
// and from binary floating point are in float.go.

// Widen returns x converted to decimal128 (IEEE 754-2008 clause 5.4.2,
// convertFormat). The conversion is exact for every decimal64 value, so it
// takes no mode. The exponent and sign are preserved; a NaN becomes the
// canonical quiet NaN.
func Widen(x uint64) Uint128 {
	panic(notImplemented)
}

// Narrow returns x converted to decimal64, rounded in mode (IEEE 754-2008
// clause 5.4.2, convertFormat). An exactly representable value keeps its
// exponent when that exponent is in range for decimal64.
func Narrow(x Uint128, mode Mode) uint64 {
	panic(notImplemented)
}

// Int64To64 returns x converted to decimal64, rounded in mode
// (IEEE 754-2008 clause 5.4.1, convertFromInt). The result has exponent 0
// when exact; an int64 with more than Precision64 digits rounds.
func Int64To64(x int64, mode Mode) uint64 {
	panic(notImplemented)
}

// Int64To128 returns x converted to decimal128 with exponent 0. Every int64
// is exactly representable, so it takes no mode.
func Int64To128(x int64) Uint128 {
	panic(notImplemented)
}

// Uint64To64 returns x converted to decimal64, rounded in mode.
func Uint64To64(x uint64, mode Mode) uint64 {
	panic(notImplemented)
}

// Uint64To128 returns x converted to decimal128 with exponent 0. Every
// uint64 is exactly representable, so it takes no mode.
func Uint64To128(x uint64) Uint128 {
	panic(notImplemented)
}

// Int64From64 returns x rounded to an integer in mode and converted to
// int64 (IEEE 754-2008 clause 5.4.1 and 5.8, convertToInteger). The
// language conversion uses TowardZero. The Status is Inexact when a
// fraction was discarded, Overflow when the rounded value is outside the
// int64 range (the result is then the nearest bound), and Invalid for a
// NaN (result 0) or an infinity (result the bound of its sign).
func Int64From64(x uint64, mode Mode) (int64, Status) {
	panic(notImplemented)
}

// Int64From128 is Int64From64 for decimal128.
func Int64From128(x Uint128, mode Mode) (int64, Status) {
	panic(notImplemented)
}

// Uint64From64 is Int64From64 with a uint64 result. A negative value that
// rounds to a non-zero integer is Overflow with result 0; a negative value
// that rounds to zero, such as -0.5 toward zero, is Inexact with result 0.
func Uint64From64(x uint64, mode Mode) (uint64, Status) {
	panic(notImplemented)
}

// Uint64From128 is Uint64From64 for decimal128.
func Uint64From128(x Uint128, mode Mode) (uint64, Status) {
	panic(notImplemented)
}
