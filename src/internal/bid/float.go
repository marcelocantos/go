// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !compiler_bootstrap

package bid

import (
	"internal/strconv"
	"unsafe"
)

// Conversions between the decimal formats and the binary formats float32
// and float64 (IEEE 754-2008 clause 5.4.2, convertFormat between radices).
// Each is correctly rounded from the exact value of its operand (language
// proposal design decision 5); the exponent and special-value rules are in
// "Binary floating-point conversions" in the package documentation.
//
// Decimal to binary: the value of an unpacked operand is exactly an integer
// coefficient times a power of ten, which is what strconv.ParseFloat reduces a
// string to before it does anything else, so strconv.Float64FromCoeff and
// strconv.Float32FromCoeff are the same code fed directly. In the default mode:
//
//   - A coefficient of up to 19 digits, which is every decimal64 and any
//     short decimal128, goes straight to strconv's fast unrounded scaling
//     (parseFloat64 and parseFloat32), which is correctly rounded.
//   - A longer decimal128 is cut to a 19-digit prefix d and a sticky. If
//     scaling rounds d and d+1 to the same binary value then the exact value,
//     which lies between them, rounds to it too. With some 60 bits in d
//     against float64's 53 only a small share of random 34-digit coefficients
//     stays undecided, and float32 almost none.
//   - The rest, and everything beyond the exponent bounds of atof64 and
//     atof32, takes the exact path: the
//     value is written into strconv's multiprecision decimal and shifted until
//     the significand is an integer, and the fraction left over is the exact
//     remainder. Values plainly out of range are decided first from the size
//     of the coefficient, so decimal128's exponents far outside float64's cost
//     nothing. The 800-digit decimal lives on the stack and is enough for any
//     128-bit coefficient: see coeffFloatBits in internal/strconv. Nothing
//     allocates.
//
// The other four modes always take the exact path. Scaling returns only the
// nearest value, not which side of the exact value it fell on, so it cannot
// decide a directed rounding or a tie without the same exact comparison.
// Those modes are unreachable from the language operators, which use
// ToNearestEven, so this costs them nothing.
//
// float32 rounds once on both paths: scaling and the decimal each extract 24
// bits from the exact value directly.
//
// None of the decimal-to-binary direction is in the code-generation template
// (see "Width parameterisation" in the package documentation). The core
// mentions no constant of a decimal format and lives in internal/strconv; a
// decimal64 passes a zero high word, which costs it one test. What depends
// on the width is the call to Unpack, in each wrapper below.
//
// Binary to decimal: the exact value m × 2^e is scaled to an integer of
// PrecisionW+1 or more digits with an exact sticky bit by bounded-stack
// integer arithmetic, and handed to the rounding routine with preferred
// exponent 0. The arithmetic and the argument for its exactness are in
// fromfloat.go; the width-dependent driver is fromFloatW in
// fromfloat_tmpl.go, generated into zfromfloat64.go and zfromfloat128.go.
// The wrappers below only take the float apart. That direction needs
// nothing from internal/strconv and its generated files are compiled in
// the bootstrap build like every other generated file; only the exported
// functions below are excluded with this file.
//
// This file is excluded from the compiler bootstrap build. The compiler
// never converts between binary and decimal at run time: constants are
// exact and are encoded through Round64 and Round128. Keeping these
// declarations out of the bootstrap build leaves the implementation free
// to use internal/strconv's exact decimal-to-binary machinery, which the
// minimum bootstrap toolchain does not have. cmd/dist rejects an import of
// an internal package outside its bootstrapDirs in any file it copies,
// regardless of build constraints, so internal/strconv is listed there
// (it depends only on math/bits and internal/goarch). Nothing in the
// bootstrap build imports it, so it is copied and not compiled.

// Float64To64 returns f converted to decimal64, rounded in mode from the
// exact binary value of f, with preferred exponent 0.
func Float64To64(f float64, mode Mode) uint64 {
	kind, neg, m, e := float64Parts(f)
	return fromFloat64(kind, neg, m, e, mode)
}

// Float64To128 is Float64To64 with a decimal128 result.
func Float64To128(f float64, mode Mode) Uint128 {
	kind, neg, m, e := float64Parts(f)
	return fromFloat128(kind, neg, m, e, mode)
}

// Float32To64 is Float64To64 for a float32 operand. Because float64(f) is
// exact, Float32To64(f, mode) equals Float64To64(float64(f), mode); the
// function exists so that callers holding a float32 need not widen it. The
// exact decimal expansion of a float32 can have far more than Precision64
// significant digits (float32(0.1) has 27, and the smallest subnormal more
// than a hundred), so the conversion is rounded in general.
func Float32To64(f float32, mode Mode) uint64 {
	kind, neg, m, e := float32Parts(f)
	return fromFloat64(kind, neg, m, e, mode)
}

// Float32To128 is Float32To64 with a decimal128 result.
func Float32To128(f float32, mode Mode) Uint128 {
	kind, neg, m, e := float32Parts(f)
	return fromFloat128(kind, neg, m, e, mode)
}

// The layout of the binary formats (IEEE 754-2008 clause 3.4) and the bits
// of their specials. This package may not import math, which the runtime
// cannot; strconv's own NaN has a non-zero payload.
const (
	float64MantBits        = 52
	float64ExpBits         = 11
	float64ExpMask  uint64 = 1<<float64ExpBits - 1
	float64Bias            = 1<<(float64ExpBits-1) - 1 // 1023
	float64QNaN     uint64 = 0x7FF8000000000000
	float64Inf      uint64 = 0x7FF0000000000000
	float64Sign     uint64 = 1 << 63

	float32MantBits        = 23
	float32ExpBits         = 8
	float32ExpMask  uint32 = 1<<float32ExpBits - 1
	float32Bias            = 1<<(float32ExpBits-1) - 1 // 127
	float32QNaN     uint32 = 0x7FC00000
	float32Inf      uint32 = 0x7F800000
	float32Sign     uint32 = 1 << 31
)

func float64bits(f float64) uint64     { return *(*uint64)(unsafe.Pointer(&f)) }
func float32bits(f float32) uint32     { return *(*uint32)(unsafe.Pointer(&f)) }
func float64frombits(b uint64) float64 { return *(*float64)(unsafe.Pointer(&b)) }
func float32frombits(b uint32) float32 { return *(*float32)(unsafe.Pointer(&b)) }

// float64Parts takes f apart: its kind, its sign, and for a finite f the
// integer m and exponent e with |f| = m × 2^e exactly. The significand's
// implicit bit is restored for a normal f; a subnormal has none and the
// exponent of the smallest one, so m may have trailing zero bits, which
// fromFloatW moves into e. A zero has m = 0.
func float64Parts(f float64) (kind Kind, neg bool, m uint64, e int) {
	b := float64bits(f)
	neg = b&float64Sign != 0
	biased := b >> float64MantBits & float64ExpMask
	m = b & (1<<float64MantBits - 1)
	switch biased {
	case float64ExpMask:
		if m != 0 {
			return KindNaN, false, 0, 0
		}
		return KindInf, neg, 0, 0
	case 0:
		return KindFinite, neg, m, 1 - float64Bias - float64MantBits
	}
	return KindFinite, neg, m | 1<<float64MantBits, int(biased) - float64Bias - float64MantBits
}

// float32Parts is float64Parts for a float32.
func float32Parts(f float32) (kind Kind, neg bool, m uint64, e int) {
	b := float32bits(f)
	neg = b&float32Sign != 0
	biased := b >> float32MantBits & float32ExpMask
	frac := b & (1<<float32MantBits - 1)
	switch biased {
	case float32ExpMask:
		if frac != 0 {
			return KindNaN, false, 0, 0
		}
		return KindInf, neg, 0, 0
	case 0:
		return KindFinite, neg, uint64(frac), 1 - float32Bias - float32MantBits
	}
	return KindFinite, neg, uint64(frac | 1<<float32MantBits), int(biased) - float32Bias - float32MantBits
}

// binaryRounding returns the direction of the magnitude of a value of sign
// neg that mode selects. A Mode outside the five behaves as ToNearestEven.
func binaryRounding(mode Mode, neg bool) strconv.Rounding {
	switch mode {
	case ToNearestAway:
		return strconv.RoundNearestAway
	case TowardZero:
		return strconv.RoundTowardZero
	case TowardPositive:
		if neg {
			return strconv.RoundTowardZero
		}
		return strconv.RoundAwayFromZero
	case TowardNegative:
		if neg {
			return strconv.RoundAwayFromZero
		}
		return strconv.RoundTowardZero
	}
	return strconv.RoundNearestEven
}

// float64Special returns the float64 of a NaN or an infinity of sign neg.
func float64Special(kind Kind, neg bool) float64 {
	if kind == KindNaN {
		return float64frombits(float64QNaN)
	}
	if neg {
		return float64frombits(float64Inf | float64Sign)
	}
	return float64frombits(float64Inf)
}

// float32Special is float64Special for float32.
func float32Special(kind Kind, neg bool) float32 {
	if kind == KindNaN {
		return float32frombits(float32QNaN)
	}
	if neg {
		return float32frombits(float32Inf | float32Sign)
	}
	return float32frombits(float32Inf)
}

// Float64From64 returns x converted to float64, rounded in mode from the
// exact decimal value of x, with float64's overflow, gradual underflow and
// signed zeros. A NaN result is the quiet float64 NaN with the sign bit
// clear.
func Float64From64(x uint64, mode Mode) float64 {
	kind, neg, c, exp := Unpack64(x)
	if kind != KindFinite {
		return float64Special(kind, neg)
	}
	return strconv.Float64FromCoeff(0, c, exp, neg, binaryRounding(mode, neg))
}

// Float64From128 is Float64From64 for decimal128.
func Float64From128(x Uint128, mode Mode) float64 {
	kind, neg, c, exp := Unpack128(x)
	if kind != KindFinite {
		return float64Special(kind, neg)
	}
	return strconv.Float64FromCoeff(c.Hi, c.Lo, exp, neg, binaryRounding(mode, neg))
}

// Float32From64 is Float64From64 with a float32 result. It rounds once,
// from the exact decimal value to float32; it is not float32(Float64From64(x)),
// which would round twice.
func Float32From64(x uint64, mode Mode) float32 {
	kind, neg, c, exp := Unpack64(x)
	if kind != KindFinite {
		return float32Special(kind, neg)
	}
	return strconv.Float32FromCoeff(0, c, exp, neg, binaryRounding(mode, neg))
}

// Float32From128 is Float32From64 for decimal128.
func Float32From128(x Uint128, mode Mode) float32 {
	kind, neg, c, exp := Unpack128(x)
	if kind != KindFinite {
		return float32Special(kind, neg)
	}
	return strconv.Float32FromCoeff(c.Hi, c.Lo, exp, neg, binaryRounding(mode, neg))
}
