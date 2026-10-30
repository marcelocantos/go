// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// Conversions between the two decimal widths and between decimals and
// integers (IEEE 754-2008 clause 5.4.1, convertFromInt and
// convertToInteger; clause 5.4.2, convertFormat). The names put the other
// type first and the decimal width last: Int64To64 converts an int64 to
// decimal64 and Int64From64 converts a decimal64 to int64. Conversions to
// and from binary floating point are in float.go. The exponent and Status
// rules are in the package documentation ("Format conversions" and
// "Integer conversions").

// Widen returns x converted to decimal128 (IEEE 754-2008 clause 5.4.2,
// convertFormat). The conversion is exact for every decimal64 value and
// preserves the exponent, so it takes no mode.
func Widen(x uint64) Uint128 {
	kind, neg, coeff, exp := Unpack64(x)
	switch kind {
	case KindNaN:
		return Uint128{Hi: NaN128Hi}
	case KindInf:
		return cInf128(neg)
	}
	// Exact fast path (named in roundW's comment and in the allowlist of
	// TestRoundIsTheOnlyPacker): the coefficient and exponent came from
	// Unpack64, and every decimal64 pair is a decimal128 pair, so nothing
	// is rounded and Pack128 cannot refuse. Going through round128 for a
	// 256-bit wide value costs two to three times as much (measured, 2026-10-30,
	// under load) for a result that is never anything but this one.
	z, _ := Pack128(neg, Uint128{Lo: coeff}, exp)
	return z
}

// Narrow returns x converted to decimal64, rounded in mode (IEEE 754-2008
// clause 5.4.2, convertFormat), with preferred exponent Q(x).
func Narrow(x Uint128, mode Mode) uint64 {
	kind, neg, coeff, exp := Unpack128(x)
	switch kind {
	case KindNaN:
		return NaN64
	case KindInf:
		return cInf64(neg)
	}
	// The coefficient has at most 34 digits and is passed whole with an
	// empty tail: the rounding routine drops the excess digits itself,
	// with the exact class of everything it discards, so there is no
	// intermediate truncation to lose a sticky bit.
	z, _ := round64(neg, coeff, exp, classZero, mode, MinExp64)
	return z
}

// Int64To64 returns x converted to decimal64, rounded in mode
// (IEEE 754-2008 clause 5.4.1, convertFromInt), with preferred exponent 0.
func Int64To64(x int64, mode Mode) uint64 {
	panic(notImplemented)
}

// Int64To128 returns x converted to decimal128 with exponent 0. Every int64
// is exactly representable, so it takes no mode.
func Int64To128(x int64) Uint128 {
	panic(notImplemented)
}

// Uint64To64 returns x converted to decimal64, rounded in mode, with
// preferred exponent 0.
func Uint64To64(x uint64, mode Mode) uint64 {
	panic(notImplemented)
}

// Uint64To128 returns x converted to decimal128 with exponent 0. Every
// uint64 is exactly representable, so it takes no mode.
func Uint64To128(x uint64) Uint128 {
	panic(notImplemented)
}

// Int64From64 returns x rounded to an integer in mode and converted to
// int64 (IEEE 754-2008 clause 5.4.1 and 5.8, convertToInteger), with the
// Status of "Integer conversions" in the package documentation: Exact,
// Inexact, Inexact|Overflow with a saturated result, or Invalid for a NaN
// or an infinity. The language conversion uses TowardZero.
func Int64From64(x uint64, mode Mode) (int64, Status) {
	panic(notImplemented)
}

// Int64From128 is Int64From64 for decimal128.
func Int64From128(x Uint128, mode Mode) (int64, Status) {
	panic(notImplemented)
}

// Uint64From64 is Int64From64 with a uint64 result, whose range is
// [0, math.MaxUint64]: a negative value that rounds to a non-zero integer
// is below the range and saturates to 0 with Inexact|Overflow, while a
// negative value that rounds to zero, such as -0.5 toward zero, is in
// range and gives 0 with Inexact.
func Uint64From64(x uint64, mode Mode) (uint64, Status) {
	panic(notImplemented)
}

// Uint64From128 is Uint64From64 for decimal128.
func Uint64From128(x Uint128, mode Mode) (uint64, Status) {
	panic(notImplemented)
}
