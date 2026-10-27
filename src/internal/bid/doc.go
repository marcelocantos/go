// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bid is the decimal floating-point kernel shared by the runtime,
// the compiler and the standard library.
//
// It implements the IEEE 754-2008 decimal64 and decimal128 interchange
// formats in their binary integer decimal (BID) encoding (IEEE 754-2008
// clause 3.5.2) and the operations of clause 5 that the language and its
// libraries need. The package operates on bit patterns only: a decimal64
// value is a uint64 and a decimal128 value is a [Uint128], exactly as
// runtime/softfloat64.go operates on the bit patterns of float64 and
// float32. No decimal-typed value appears in any signature, so the package
// compiles with a toolchain that does not know the decimal types; that is
// what lets the runtime, cmd/compile (through the cmd/dist bootstrap) and
// the standard library all import one implementation.
//
// Every exported function is width-suffixed: Add64 operates on decimal64
// bit patterns and Add128 on decimal128 bit patterns. How the two widths
// share an implementation is an internal matter that this API does not
// decide.
//
// # Invariants
//
// Every function in this package keeps the following invariants, and a
// change to any of them is a change to this contract.
//
//   - No allocation. No function allocates. Functions that produce text
//     append to a caller-supplied buffer; [MaxLen64] and [MaxLen128]
//     bound what they append.
//   - No panic on any input. Every bit pattern of a uint64 or a Uint128
//     is a valid operand, including non-canonical encodings, signaling
//     NaNs and NaNs with payloads, and every function returns a result for
//     it. A [Mode] value outside the five defined modes behaves as
//     [ToNearestEven]. (Until an operation is implemented its body panics
//     with a fixed message; that is a temporary state, not part of the
//     contract.)
//   - No mutable global state. The package has no variables that are
//     written after initialization. Static tables are permitted.
//   - Exactly one rounding per operation. Every operation computes its
//     result exactly, or with an exact sticky bit, and that intermediate
//     reaches the encoding through one rounding routine. No operation
//     rounds an intermediate and then rounds again, and no two operations
//     carry their own rounding logic. The exported [Round64] and
//     [Round128] are that routine's entry points for callers that already
//     hold an exact value in parts.
//   - Non-canonical encodings read as their canonical value. An encoding
//     whose coefficient field exceeds [MaxCoeff64] or 10^34 - 1
//     ([MaxCoeff128Hi], [MaxCoeff128Lo]) is read as a zero with the
//     encoded sign and exponent; an infinity or NaN with non-zero reserved
//     bits is read as that infinity or NaN (IEEE 754-2008 clause 3.5.2).
//     Results are always canonical.
//   - Signaling NaNs are NaNs. A signaling NaN operand is treated exactly
//     like a quiet NaN, and a NaN result is always the canonical quiet NaN
//     [NaN64] or [NaN128]: NaN payloads and NaN signs are not propagated
//     (IEEE 754-2008 clause 6.2 permits this; the language proposal
//     requires it). Infinities keep their sign.
//
// The per-operation contract (the preferred exponent of each result, the
// sign of an exact zero under each rounding mode, overflow and underflow
// behaviour, and the NaN cases of each operation) is written down in this
// package's documentation by a later target, before any operation is
// implemented. This file states only what every operation has in common.
//
// # Rounding modes
//
// Every operation that can be inexact takes a [Mode] parameter, one of the
// five rounding-direction attributes of IEEE 754-2008 clause 4.3. The
// language operators round to nearest, ties to even, and nothing else
// (language proposal design decision 4); the runtime passes the constant
// [ToNearestEven]. The mode is a parameter rather than a global attribute
// because Go has no dynamic rounding state, because directed rounding is
// how the standard library exposes the round-half-away and directed
// functions that financial code asks for, and because the oracle that
// proves this package tests every operation under every mode. Operations
// that are always exact (remainder, modulo, sign operations, comparisons,
// widening) take no mode.
//
// # Conditions
//
// Arithmetic operations return the IEEE default result for every exception
// and expose no status flags: invalid operations return NaN, division by
// zero returns a signed infinity, overflow and underflow round per the
// mode. The three families of conversion where the caller must act on the
// outcome (character sequences, integers, and rounding from exact parts)
// return a [Status] alongside their result, which the caller maps to its
// own error type. This package sits below errors in the dependency graph
// and defines no error values.
//
// # Representation of decimal128
//
// A decimal128 bit pattern is a [Uint128], a struct of two uint64 words,
// most significant first. A struct rather than separate hi and lo
// parameters because the two conventions (hi, lo) and (lo, hi) both
// occurred in the code this package replaces and a struct makes the
// mistake impossible, and because a two-word struct and two scalar words
// cost the same under the register ABI. The same type carries 128-bit
// coefficients, as uint64 carries both decimal64 bit patterns and 64-bit
// coefficients. A Uint128 is a value, not a memory image: it has no
// defined relationship to the byte order of a decimal128 in memory.
//
// # Dependencies
//
// The files that cmd/dist copies for the compiler bootstrap import only
// math/bits. Conversion to and from binary floating point lives in a file
// excluded from the bootstrap build (see float.go), because a correctly
// rounded conversion between binary and decimal is expected to share
// internal/strconv's exact machinery, which the minimum bootstrap toolchain
// does not provide, and because the compiler never needs it: constants are
// exact and are encoded through [Round64] and [Round128].
package bid
