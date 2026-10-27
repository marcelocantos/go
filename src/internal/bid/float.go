// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !compiler_bootstrap

package bid

// Conversions between the decimal formats and the binary formats float32
// and float64 (IEEE 754-2008 clause 5.4.2, convertFormat between radices).
// Each is correctly rounded from the exact value of its operand (language
// proposal design decision 5): a float64 holding the nearest binary value to
// 0.1 converts to the decimal128 0.1000000000000000055511151231257827, and
// a decimal converts to the binary value nearest its exact decimal value.
//
// This file is excluded from the compiler bootstrap build. The compiler
// never converts between binary and decimal at run time: constants are
// exact and are encoded through Round64 and Round128. Keeping these
// declarations out of the bootstrap build leaves the implementation free
// to use internal/strconv's exact decimal-to-binary machinery, which the
// minimum bootstrap toolchain does not have. Note that cmd/dist rejects an
// import of an internal package outside its bootstrapDirs in any file it
// copies, regardless of build constraints; an implementation that imports
// internal/strconv here must also list internal/strconv in bootstrapDirs
// (it depends only on math/bits) or teach the copy step to honour the
// compiler_bootstrap constraint.

// Float64To64 returns f converted to decimal64, rounded in mode from the
// exact binary value of f. Infinities and NaN convert to the corresponding
// decimal special values; -0 converts to -0 with exponent 0.
func Float64To64(f float64, mode Mode) uint64 {
	panic(notImplemented)
}

// Float64To128 is Float64To64 with a decimal128 result.
func Float64To128(f float64, mode Mode) Uint128 {
	panic(notImplemented)
}

// Float32To64 is Float64To64 for a float32 operand. Because float64(f) is
// exact, Float32To64(f, mode) equals Float64To64(float64(f), mode); the
// function exists so that callers holding a float32 need not widen it. The
// exact decimal expansion of a float32 can have far more than Precision64
// significant digits (float32(0.1) has 27, and the smallest subnormal more
// than a hundred), so the conversion is rounded in general.
func Float32To64(f float32, mode Mode) uint64 {
	panic(notImplemented)
}

// Float32To128 is Float32To64 with a decimal128 result.
func Float32To128(f float32, mode Mode) Uint128 {
	panic(notImplemented)
}

// Float64From64 returns x converted to float64, rounded in mode from the
// exact decimal value of x. Infinities, zeros and NaN convert to the
// corresponding binary special values; a NaN result is the canonical quiet
// float64 NaN with the sign bit clear. Values too large for float64
// become an infinity or the largest finite float64 as the mode selects;
// values too small become zero or a subnormal.
func Float64From64(x uint64, mode Mode) float64 {
	panic(notImplemented)
}

// Float64From128 is Float64From64 for decimal128.
func Float64From128(x Uint128, mode Mode) float64 {
	panic(notImplemented)
}

// Float32From64 is Float64From64 with a float32 result. It rounds once,
// from the exact decimal value to float32; it is not float32(Float64From64(x)),
// which would round twice.
func Float32From64(x uint64, mode Mode) float32 {
	panic(notImplemented)
}

// Float32From128 is Float32From64 for decimal128.
func Float32From128(x Uint128, mode Mode) float32 {
	panic(notImplemented)
}
