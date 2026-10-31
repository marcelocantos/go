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
// Every exported function that works at one width is width-suffixed:
// Add64 operates on decimal64 bit patterns and Add128 on decimal128 bit
// patterns. [Widen] and [Narrow], which go between the widths, are the
// exceptions. How the two widths share an implementation is an internal
// matter that this API does not decide.
//
// This documentation is the contract of the package. It fixes, for every
// operation, the result on every input, including the exceptional cases of
// IEEE 754-2008 clause 7, before any operation is implemented; the test
// oracle and the implementation are both written against it. Where the
// standard fixes an answer the clause is cited. Where the standard leaves
// latitude or is silent the choice is stated and, when it follows the
// General Decimal Arithmetic specification (Cowlishaw,
// http://speleotrove.com/decimal/decarith.html, called GDA below, the
// source of the decTest vectors), that is said. Every deliberate departure
// from either document is marked "Divergence". Per-function comments
// describe what a function computes and point here; a rule lives in one
// place only.
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
//     [ToNearestEven].
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
//     Every operation that computes a value returns a canonical encoding.
//     The only functions that return a non-canonical encoding are the
//     sign operations [Neg64], [Abs64] and [CopySign64] (and the
//     decimal128 forms), which copy the bits of their operand; see "Sign
//     operations".
//   - Signaling NaNs are NaNs. A signaling NaN operand is treated exactly
//     like a quiet NaN, and every NaN that an operation produces is the
//     canonical positive quiet NaN: [NaN64], or the decimal128 whose high
//     word is [NaN128Hi] and whose low word is zero. NaN payloads and NaN
//     signs are not propagated. IEEE 754-2008 clause 6.2.3 only
//     recommends payload propagation; the language proposal requires that
//     it not happen. The sign operations, which copy bits, are again the
//     only exception. Infinities keep their sign. Divergence: IEEE
//     754-2008 clause 7.2 item a and GDA both signal invalid operation
//     for a signaling NaN operand; this package treats it as quiet and
//     reports nothing (see "Conditions").
//
// # Rounding modes
//
// Every operation that can be inexact takes a [Mode] parameter, one of the
// five rounding-direction attributes of IEEE 754-2008 clause 4.3:
//
//	Mode            IEEE 754-2008 attribute   Clause   Rounds to
//	ToNearestEven   roundTiesToEven           4.3.1    nearest; ties to the even coefficient
//	ToNearestAway   roundTiesToAway           4.3.1    nearest; ties away from zero
//	TowardZero      roundTowardZero           4.3.2    the representable value nearest to and no greater in magnitude
//	TowardPositive  roundTowardPositive       4.3.2    the representable value nearest to and no less than the exact value
//	TowardNegative  roundTowardNegative       4.3.2    the representable value nearest to and no greater than the exact value
//
// A Mode outside these five behaves as ToNearestEven; it is not an error.
// The language operators round to nearest, ties to even, and nothing else
// (language proposal design decision 4); the runtime passes the constant
// [ToNearestEven]. The mode is a parameter rather than a global attribute
// because Go has no dynamic rounding state, because directed rounding is
// how the standard library exposes the round-half-away and directed
// functions that financial code asks for, and because the oracle that
// proves this package tests every operation under every mode. Operations
// that are always exact (remainder, modulo, reduce, sign operations,
// comparisons, widening, conversion of an integer to decimal128) take no
// mode.
//
// # Conditions
//
// IEEE 754-2008 clause 7 defines five exceptions. This package delivers
// the clause 7 default result for each. Divergence: clause 7.1 requires
// that each exception also raise a status flag the program can test and
// clear; this package has no flags, as Go's binary floating point has
// none. Arithmetic,
// Widen, Narrow, the conversions from integers and the Float conversions
// return a value and nothing else; the caller cannot learn whether it was
// exact. The three families of conversion where the caller must act on the
// outcome, Parse64 and Parse128, the integer conversions Int64From64 and
// its relatives, and the rounding routine Round64 and Round128, return a
// [Status] alongside the result. Status is a bit set over these flags:
//
//	Status     IEEE 754-2008 exception   Clause   Default result delivered
//	Invalid    invalid operation         7.2      the canonical quiet NaN; for an integer result, see "Integer conversions"
//	(none)     division by zero          7.3      the infinity with the exclusive-or of the operand signs
//	Overflow   overflow                  7.4      an infinity or the largest finite magnitude, by mode; see "Overflow"
//	Underflow  underflow                 7.5      the rounded subnormal or zero; see "Underflow and subnormal results"
//	Inexact    inexact                   7.6      the rounded result
//	Syntax     (none)                    -        the canonical quiet NaN; the string is not in the accepted grammar
//
// Overflow and Underflow are always accompanied by Inexact (clauses 7.4
// and 7.5; a result that is tiny but exact reports nothing). Invalid and
// Syntax are always reported alone. Exact is the zero Status.
//
// No function reports Invalid for a NaN operand of an arithmetic
// operation, because arithmetic reports nothing, and no function reports
// Invalid because an operand is signaling rather than quiet. The only
// source of Invalid is the conversion of a NaN or an infinity to an
// integer. Divergence: GDA's Clamped, Rounded, Subnormal, Division
// impossible and Division undefined conditions have no counterpart;
// Division undefined is a NaN result with no flag, and the operations
// that GDA refuses with Division impossible succeed here (see
// "Remainder and modulo").
//
// # Values, cohorts and the character form
//
// A finite decimal value is (-1)^sign × coefficient × 10^exponent with the
// coefficient an integer of at most Precision digits and the exponent in
// [MinExp, MaxExp] (IEEE 754-2008 clause 3.3, parameters in Table 3.6;
// the constants are in encoding.go). Q(x) denotes the exponent of a finite
// x. The members of a cohort (clause 2.1) are the encodings of one
// non-zero value with different exponents, such as 1.5, 1.50 and 1.500;
// they are equal under every comparison and differ under TotalOrder,
// SameQuantum, Append and the exponent rules below. Every zero with a given
// sign is one cohort too, with exponents over the whole range.
//
// The character form used throughout this documentation and produced by
// Append64 and Append128 is the to-scientific-string form of GDA. IEEE
// 754-2008 clause 5.12 requires a conversion to character form that is
// exact and preserves the quantum, and GDA's form meets it; the standard
// fixes no particular notation. For a finite value, the coefficient is
// written with all its digits and the adjusted exponent a = Q(x) +
// (digits - 1) is computed. If Q(x) <= 0 and a >= -6 the value is written
// in plain notation: the coefficient digits with a decimal point inserted
// -Q(x) digits from the right, padded on the left with "0." and zeros as
// needed, and no point at all when Q(x) = 0. Otherwise it is written in
// exponential notation: the first coefficient digit, a point and the
// remaining digits (no point when there is one digit), then "E", the sign
// of a (always written) and the decimal digits of |a|. A zero coefficient
// is written as the single digit 0. A negative sign is written first.
// Infinities are "Infinity" and "-Infinity". Every NaN is written "NaN"
// (divergence from GDA and from the recommendation of clause 5.12.1, which
// allow a sign, "sNaN" and a payload: this package never produces a NaN
// with any of those). The form preserves the exponent, so Parse reads it
// back to the same bit pattern:
//
//	Coefficient  Exponent  Written       Coefficient  Exponent  Written
//	150          -2        1.50          0            0         0
//	15           2         1.5E+3        0            -2        0.00
//	123          0         123           0            1         0E+1
//	123          1         1.23E+3       0            -398      0E-398
//	1            -6        0.000001      1            -7        1E-7
//	10           369       1.0E+370      12345        -7        0.0012345
//
// # Exponent of a result
//
// IEEE 754-2008 clause 5.2 fixes the exponent of a decimal result through
// the preferred exponent of each operation. If the result is exact and
// the cohort of the exact value contains a member with the preferred
// exponent, that member is the result; if it does not, the member whose
// exponent is closest to the preferred exponent is the result. If the
// result is inexact, the result is the cohort member of the rounded value
// with the least possible exponent, so that its coefficient has Precision
// digits (fewer only when the result is subnormal, whose exponent is
// MinExp). Two operations have a fixed rather than a preferred exponent:
// Quantize, whose result has the exponent of its second operand (clause
// 5.3.2), and RoundToIntegral, whose result has exponent max(Q(x), 0)
// whether or not a fraction was discarded (clause 5.2 lists this value;
// GDA defines round-to-integral-exact as quantize by 1E+0 when Q(x) < 0).
// The preferred exponents are:
//
//	Operation                  Preferred exponent              Source
//	Add, Sub                   min(Q(x), Q(y))                 clause 5.2
//	Mul                        Q(x) + Q(y)                     clause 5.2
//	Div                        Q(x) - Q(y)                     clause 5.2
//	FMA                        min(Q(x) + Q(y), Q(z))          clause 5.2
//	Rem                        min(Q(x), Q(y)); always exact   clause 5.2
//	Mod                        min(Q(x), Q(y)); always exact   GDA remainder (not an IEEE operation)
//	RoundToIntegral            max(Q(x), 0), fixed             clause 5.2
//	Quantize                   Q(y), fixed                     clause 5.3.2
//	Reduce                     the largest exponent at most    GDA reduce (not an IEEE operation)
//	                           MaxExp at which the value is
//	                           representable; 0 for a zero
//	Widen, Narrow              Q(x)                            clause 5.2, convertFormat
//	Int64To*, Uint64To*        0                               clause 5.2, convertFromInt
//	Float64To*, Float32To*     0                               this package; see "Binary floating-point conversions"
//	Parse                      the exponent the numeral        clause 5.12.2 with clause 5.2
//	                           denotes: its exponent part
//	                           less its fraction digits
//	Min, Max                   Q of the operand returned       clause 5.2, minNum and maxNum
//	Round                      the exp argument                this package
//
// Exponent clamping. An exact result whose exponent under the rule above
// would exceed MaxExp is encoded with exponent MaxExp and a coefficient
// padded with trailing zeros whenever the padded coefficient fits in
// Precision digits; it is exact, it does not overflow and it reports
// nothing (GDA calls this fold-down and signals Clamped; IEEE 754-2008
// clause 7.4 defines overflow by magnitude, so a value no larger than the
// largest finite value never overflows). An exact result whose exponent
// would be below MinExp is likewise encoded at MinExp with the coefficient
// divided by the corresponding power of ten when that division is exact;
// otherwise the result is inexact and rounds at MinExp (see "Underflow
// and subnormal results"). A zero result takes the exponent the rule
// gives, clamped into [MinExp, MaxExp]: 0E+400 is encoded as 0E+369 in
// decimal64, and 0E-500 as 0E-398, exactly and with no report (GDA
// signals Clamped). In other words, every exact result is the unique
// encoding of its value whose exponent is closest to the preferred
// exponent among the encodings the format has.
//
// Division of a finite x by an infinity has no preferred exponent in
// clause 5.2, since Q(∞) is undefined. The result is a zero with exponent
// MinExp, the sign being the exclusive-or of the operand signs. This is
// the limit of Q(x) - Q(y) as the divisor grows without bound, after
// clamping, and it is what GDA's reference implementation and its
// ddDivide test vectors produce (1 / Infinity is 0E-398 in decimal64); the
// GDA text itself does not state it.
//
// Remainder of a finite x by an infinity is x unchanged, with x's
// exponent (clause 5.3.1).
//
// # Sign of a result
//
// IEEE 754-2008 clause 6.3 fixes the sign of every result that is not a
// NaN; a NaN result has no sign here (it is the canonical positive NaN).
//
//   - A non-zero sum or difference has the sign of its value.
//   - An exact zero sum or difference of operands with opposite signs
//     (for Add) or like signs (for Sub) is +0 under ToNearestEven,
//     ToNearestAway, TowardZero and TowardPositive, and -0 under
//     TowardNegative. x + x and x - (-x) keep the sign of x even when x is
//     a zero, under every mode: Add64(-0, -0, m) is -0 and Add64(0, 0,
//     TowardNegative) is 0.
//   - A product or quotient, including a zero or infinite one, has the
//     exclusive-or of the operand signs: Mul64(-0, 5, m) is -0 and
//     Div64(0, -2, m) is -0.
//   - FMA64(x, y, z) takes the sign that adding the exact product x × y to
//     z would give under the rules above, with the exact product's sign
//     being the exclusive-or of the signs of x and y. FMA64(-0, 5, -0, m)
//     is -0 (a sum of like signs) and FMA64(2, 3, -6, m) is +0 except
//     under TowardNegative.
//   - Rem, Mod, RoundToIntegral, Quantize, Reduce, Widen, Narrow, Parse,
//     the integer-to-decimal and binary-to-decimal conversions and Round
//     give their result the sign of the (first or only) operand, including
//     a zero result: Rem64(-6, 3) is -0 and RoundToIntegral64(-0.4, m) is
//     -0 in every mode. Clause 5.3.1 fixes the sign of a zero remainder as
//     that of x; GDA's remainder agrees.
//   - An inexact result that rounds to zero keeps the sign of the exact
//     result: Mul64(-1E-200, 1E-199, ToNearestEven) is -0E-398, and
//     Parse64("-1E-399", ToNearestEven) is -0E-398.
//   - Min and Max return one operand with its sign; -0 is less than +0
//     for them (see "Minimum and maximum").
//   - An infinity produced by overflow or by division by zero has the sign
//     the rules above give the exact or intermediate result.
//
// # Overflow
//
// A result overflows when the value it would round to under the mode
// with an unbounded exponent range exceeds the largest finite magnitude
// of the format, 9.999999999999999E+384 for decimal64 and
// 9.999999999999999999999999999999999E+6144 for decimal128 (IEEE 754-2008
// clause 7.4). Overflow is thus decided after rounding: under TowardZero,
// 99999999999999995E+368 rounds to 9999999999999999E+369 and does not
// overflow, while under ToNearestEven it rounds to 1E+385 and does. A
// value with an exponent above MaxExp that is no larger than the largest
// finite value does not overflow; it is padded (see "Exponent of a
// result"). The default result is fixed by clause 7.4 per mode and per
// the sign of the intermediate result:
//
//	Mode            Positive overflow                  Negative overflow
//	ToNearestEven   +Infinity                          -Infinity
//	ToNearestAway   +Infinity                          -Infinity
//	TowardZero      largest finite value               negative of the largest finite value
//	TowardPositive  +Infinity                          negative of the largest finite value
//	TowardNegative  largest finite value               -Infinity
//
// Status-returning functions report Inexact|Overflow. Arithmetic returns
// the result and reports nothing. GDA's overflow table (its Overflow
// condition, under round-half-even, round-half-up, round-down,
// round-ceiling and round-floor) is the same.
//
// # Underflow and subnormal results
//
// A non-zero finite value whose magnitude is below 10^Emin (1E-383 for
// decimal64, 1E-6143 for decimal128) is subnormal; its coefficient has
// fewer than Precision digits at exponent MinExp, and like any value it
// may be encoded at any exponent of its cohort (IEEE 754-2008 clause 3.3):
// 1E-390 is a subnormal decimal64 with coefficient 1 and exponent -390.
// An inexact subnormal result is encoded at MinExp, as the inexact-result
// rule gives. A result is tiny when its exact value, before any rounding, is
// non-zero and below 10^Emin in magnitude. Clause 7.5 fixes tininess
// detection before rounding for decimal formats (it leaves the choice to
// the implementer for binary formats only), and GDA's Subnormal
// condition is defined the same way, so the decTest vectors apply. A
// tiny result is rounded at exponent MinExp in the given mode, and may
// round to a subnormal, to zero or up to 10^Emin itself. Under clause 7.5
// default handling the Status-returning functions report Inexact|Underflow
// when the result is tiny and the rounding was inexact, and nothing when
// a tiny result is exact: 1E-398 is an exact subnormal and Parse64 reports
// Exact for it. A tiny result that rounds up to the normal 10^Emin still
// reports Inexact|Underflow, because tininess is judged before rounding.
// Arithmetic returns the rounded result and reports nothing.
//
// A result that rounds to zero from a non-zero exact value has exponent
// MinExp (the least possible exponent, as for every inexact result) and
// the sign of the exact value: 0E-398 or -0E-398 in decimal64. Under
// TowardPositive a positive tiny value never rounds to zero but to 1E-398
// at least, and symmetrically for TowardNegative and negative values.
//
// # NaN and infinity operands
//
// Any NaN operand, quiet or signaling, makes the result of an arithmetic
// operation, a rounding, a quantize, a reduce, a min or max, or a format
// conversion the canonical quiet NaN (clause 6.2), and makes an integer
// conversion report Invalid. Comparisons treat a NaN as unordered (see
// "Comparisons and ordering"). Infinities follow clause 6.1: an operation
// with an infinite operand whose exact result is an infinity returns that
// infinity, exactly. The invalid operations, which return the canonical
// quiet NaN, are those of clause 7.2 (items b to h) that the package's
// operations can meet:
//
//	Operation           Invalid when                                        Clause
//	Add, Sub            the infinities cancel: ∞ + (-∞), ∞ - ∞                7.2 d
//	Mul                 0 × ∞ or ∞ × 0                                      7.2 b
//	Div                 0 / 0 or ∞ / ∞                                      7.2 e
//	FMA                 x × y is 0 × ∞, or the exact product is an           7.2 c, d
//	                    infinity and z the opposite infinity
//	Rem, Mod            x is an infinity, or y is a zero                    7.2 f; GDA remainder, Division undefined
//	Quantize            exactly one operand is an infinity, or the result   7.2 h; clause 5.3.2
//	                    coefficient would exceed Precision digits
//	Int64From*, ...     x is a NaN or an infinity (Status Invalid)          clause 5.8
//
// Everything else with infinities is defined: ∞ + x, ∞ × x (x non-zero),
// ∞ / x and x / 0 (x finite non-zero, clause 7.3) give a signed infinity;
// x / ∞ gives a signed zero (see "Exponent of a result"); Rem64(x, ∞) and
// Mod64(x, ∞) give x for finite x (clause 5.3.1); Quantize64(±∞, ±∞) gives
// the infinity x (clause 5.3.2; GDA quantize); RoundToIntegral, Reduce,
// Widen and Narrow return an infinity unchanged. Mod64 is not an IEEE
// operation; its special cases are those of Rem64, which agrees with GDA's
// remainder. Clause 7.2 item c makes it implementation-defined whether
// FMA64(0, ∞, qNaN) signals invalid; here the result is NaN either way
// and nothing is signaled.
//
// # Remainder and modulo
//
// Rem64(x, y) is the IEEE remainder of clause 5.3.1: x - y × n with n the
// integer nearest x / y, and even when x / y is halfway between two
// integers. Mod64(x, y) is x - y × trunc(x / y), the decimal analogue of
// Go's % operator and the remainder operation of GDA. Both are exact for
// every pair of finite operands: the result is an integer multiple of
// 10^min(Q(x), Q(y)) with magnitude at most |y| / 2 (Rem) or below |y|
// (Mod), so its coefficient at that exponent has no more digits than the
// larger operand's, whatever the magnitude of the quotient. Neither takes
// a Mode. A zero result has the sign of x. Divergence from GDA: GDA's
// remainder and remainder-near refuse with Division impossible when the
// integer quotient would have more than Precision digits (GDA notes this
// as its own deviation from IEEE 754); this package computes the exact
// remainder for any quotient, as clause 5.3.1 requires. decTest vectors
// that expect Division impossible are out of scope.
//
// # Sign operations
//
// Neg64, Abs64 and CopySign64 are the negate, abs and copySign operations
// of IEEE 754-2008 clause 5.5.1. They operate on the sign bit alone and on
// every encoding alike: they do not canonicalize, they do not quiet a
// signaling NaN, they do not clear a payload, and they never signal. They
// are the one exception to "every result is canonical" and "every NaN
// result is the canonical quiet NaN"; a caller that needs a canonical
// result passes the output through an arithmetic operation. Because
// Append writes every NaN as "NaN", the character form does not show
// the sign a sign operation gives a NaN; TotalOrder and Signbit do.
//
// # Comparisons and ordering
//
// Equal64, Less64, LessEqual64 and Compare64 are the quiet comparison
// predicates of IEEE 754-2008 clauses 5.6.1 and 5.11. They compare values,
// not encodings: every member of a cohort is equal to every other, +0
// equals -0, and a non-canonical encoding compares as the value it reads
// as. A NaN operand, quiet or signaling, is unordered: Equal, Less and
// LessEqual return false and Compare returns (0, false). No exception is
// exposed; the signaling predicates of the standard differ only in what
// they signal. Infinities compare as the clause 6.1 values: -∞ is below
// every finite value and +∞ above.
//
// TotalOrder64 is the totalOrder predicate of clause 5.10 on values and
// encodings: -NaN precedes -∞, which precedes the negative finite values
// in decreasing magnitude, then -0, then +0, the positive finite values,
// +∞ and +NaN. Two members of one cohort are ordered by exponent: for
// positive values the smaller exponent precedes (1.0 precedes 1), for
// negative values the larger exponent precedes (-1 precedes -1.0). A
// non-canonical encoding orders as the canonical value it reads as. A NaN
// operand is ordered by its sign bit alone (TotalOrder reads a sign; it
// does not produce a NaN) and all NaNs of one sign are equal in the
// order, so TotalOrder64(x, y) and TotalOrder64(y, x) are both true for
// two NaNs of the same sign. Divergence: clause 5.10 orders a signaling
// NaN before a quiet one and NaNs of the same kind by payload; this
// package ignores the signaling bit and the payload in TotalOrder, as it
// does everywhere else.
//
// SameQuantum64 (clause 5.7.3) is true when both operands are finite with
// the same exponent, both are infinities, or both are NaNs, and false
// otherwise. Key64 is the hash key described at [Key].
//
// # Minimum and maximum
//
// Min64 and Max64 implement the Go built-ins min and max for decimal
// values, not the minNum and maxNum of IEEE 754-2008 clause 5.3.1 nor
// GDA's min and max. Divergence: if either operand is a NaN the result is
// NaN (minNum, maxNum and GDA return the number). Otherwise the result is
// the lesser (greater) operand by value with -0 less than +0, and when the
// operands are equal in value and sign, including two members of one
// cohort with different exponents, the result is x, canonicalized (GDA
// chooses by exponent, so Max64(1.0, 1) is 1.0 here and 1 in GDA). The
// preferred exponent of the result is that of the operand returned, so
// the exponent is preserved.
//
// # Format conversions
//
// Widen is convertFormat of clause 5.4.2 from decimal64 to decimal128. It
// is exact for every decimal64 value and preserves the exponent, so it
// takes no Mode; a NaN of either kind becomes the canonical NaN128.
// Narrow is convertFormat from decimal128 to decimal64 with the preferred
// exponent Q(x): a value representable in decimal64 keeps its exponent
// when that exponent is in [MinExp64, MaxExp64] and is otherwise encoded
// at the nearer bound with a padded or shortened coefficient (an exact
// zero at exponent 1000 becomes 0E+369); a value that is not
// representable is rounded once in the mode, with overflow and underflow
// as above.
//
// # Integer conversions
//
// Int64To64, Uint64To64, Int64To128 and Uint64To128 are convertFromInt
// (clause 5.4.1) with preferred exponent 0. Every int64 and uint64 fits
// the 34 digits of decimal128, so the decimal128 forms are exact and take
// no Mode. An int64 or uint64 with more than 16 significant digits rounds
// to decimal64 in the given mode, and the inexact result then has 16
// digits and a positive exponent as the inexact-result rule gives.
//
// Int64From64, Uint64From64 and the decimal128 forms are the
// convertToIntegerExact{...} operations of clauses 5.4.1 and 5.8, which
// signal inexact (the plain convertToInteger{...} operations do not): x is
// rounded to an integer in the given mode and converted. The language
// conversion uses TowardZero. The Status is
// Exact when x is an integer in range, Inexact when a non-zero fraction
// was discarded and the integer is in range, Inexact|Overflow when the
// rounded integer is outside the range of the result type (the result is
// then the bound of the type nearest to it: math.MaxInt64 or
// math.MinInt64 for int64, 0 or math.MaxUint64 for uint64), and Invalid
// when x is a NaN (result 0) or an infinity (result the bound of the
// infinity's sign). A negative value that rounds to zero converts to the
// uint64 0 with Status Exact or Inexact as a fraction was discarded, not
// Overflow. Divergence: clause 5.8 signals invalid operation for a finite
// value outside the range of the integer format and leaves the result to
// the implementation; this package reports Overflow with a saturated
// result so that a caller can tell a range error from a non-number. The
// language-level conversion is the runtime's concern, not this package's.
//
// # Binary floating-point conversions
//
// Float64To64, Float64To128, Float32To64 and Float32To128 are
// convertFormat between radices (clause 5.4.2), correctly rounded in the
// given mode from the exact value of the binary operand, never from its
// shortest decimal representation (language proposal design decision 5):
// the float64 nearest 0.1 converts to 0.1000000000000000 in decimal64 and
// to 0.1000000000000000055511151231257827 in decimal128, and the float64
// 1e23, whose exact value is 99999999999999991611392, converts to
// 9.999999999999999E+22 in decimal64. Clause 5.2 gives no preferred
// exponent for a conversion from a binary format (Q(x) is undefined for
// binary x, and GDA has no such operation); this package chooses 0, as
// for convertFromInt, so an exact result has the exponent closest to 0:
// float64 0.5 is 0.5, 2.0 is 2, -0 is -0 with exponent 0, and 1e20 is
// 100000000000000000000 in decimal128. Infinities and NaNs convert to the
// corresponding decimal special values.
//
// Float64From64, Float32From64 and the decimal128 forms round once in
// the given mode from the exact decimal value to the binary format, with
// the binary format's overflow (an infinity or the largest finite binary
// value by the mode, the same table as above), gradual underflow and
// signed zeros. A NaN converts to the binary quiet NaN with the sign bit
// clear and the payload zero; infinities keep their sign.
//
// # Character conversions
//
// Parse64 and Parse128 are convertFromDecimalCharacter (clauses 5.4.2 and
// 5.12.2): the numeral's exact value is rounded once in the given mode,
// with the preferred exponent of the numeral itself, so an exact result
// has the numeral's exponent ("0.00" is 0E-2, "1.50" is 150E-2, "1.5E+3"
// is 15E+2) unless that exponent is out of range, in which case the
// clamping rule applies ("0E+400" is 0E+369, "1E+370" is 10E+369). The
// Status reports Inexact, Overflow and Underflow as for Round, and
// Syntax, with result NaN, for a string outside the grammar documented at
// Parse64. The special words give Infinity, -Infinity and NaN with Status
// Exact. Divergence: the word "snan" gives the canonical quiet NaN with
// Status Exact; clause 5.12.1 and GDA convert it to a signaling NaN. A
// numeral's exponent part may have any number of digits; "1E+99999999999"
// overflows and "0E+99999999999" is 0E+369.
//
// Append64 and Append128 are convertToDecimalCharacter (clauses 5.4.2 and
// 5.12): they write the character form defined under "Values, cohorts and
// the character form", which Parse reads back to the canonical encoding
// of the operand. They are exact and take no Mode.
//
// # Rounding from exact parts
//
// Round64 and Round128 are the rounding routine every operation uses,
// exposed for the compiler and the parser. The input is an exact value in
// parts, (-1)^neg × (coeff + f) × 10^exp with f = 0 when sticky is false
// and 0 < f < 1 when it is true; the preferred exponent is exp. The result
// is that value rounded to the format in the given mode under every rule
// above: precision rounding, the exponent rule and clamping, overflow,
// underflow and the sign of zero (an exact zero, coeff = 0 and sticky
// false, keeps the sign neg). Sticky records that f is non-zero and
// nothing about its size, so it decides the rounding exactly when at least
// one digit of coeff is discarded. When sticky is set and no digit is
// discarded, the result is that of rounding (coeff + ε) × 10^exp for an
// arbitrarily small ε > 0: the directed modes move away from coeff where
// the direction demands (TowardPositive of 1 with sticky set is
// 1.000000000000001), the nearest modes do not, and the Status is
// Inexact. The result is inexact, so it has Precision digits like every
// inexact result: ToNearestEven of 1 with sticky set is 1.000000000000000,
// not 1. A coeff of zero with sticky set is an arbitrarily small
// non-zero value of sign neg: it rounds to the zero of that sign at MinExp
// under the nearest modes, TowardZero and the directed mode away from its
// sign, to the smallest subnormal of its sign under the directed mode
// toward its sign, and reports Inexact|Underflow in every mode.
//
// # Worked examples
//
// The examples below are derived by hand from the rules above and from
// the text of IEEE 754-2008 and GDA, not from any implementation, so that
// an implementation checked against them is checked independently. One
// example per line, in the form
//
//	Function(operands, Mode) = result
//	Function(operands, Mode) = result, Status
//
// Decimal operands and results are in the character form of Append. Two
// forms that Append never writes are used for operands and for the
// results of the sign operations: -NaN is a NaN with the sign bit set and
// sNaN a signaling NaN (-sNaN both). Binary operands and results of the
// Float functions are Go float64 or float32 literals, the shortest that
// denote the value, with +Inf, -Inf and NaN for the specials. Round64
// takes its arguments as Go values in declaration order. Status values
// are the constant names joined with "|".
//
// Exponent of an exact result (preferred exponent and cohort rule):
//
//	Add64(1.0, 2.00, ToNearestEven) = 3.00
//	Add64(12, 7.00, ToNearestEven) = 19.00
//	Add64(1E+2, 1E+4, ToNearestEven) = 1.01E+4
//	Add64(1E+5, 1, ToNearestEven) = 100001
//	Sub64(1.3, 1.07, ToNearestEven) = 0.23
//	Sub64(1.30, 1.3, ToNearestEven) = 0.00
//	Mul64(1.20, 3, ToNearestEven) = 3.60
//	Mul64(7, 3, ToNearestEven) = 21
//	Mul64(654321, 654321, ToNearestEven) = 428135971041
//	Mul64(1234567890123456, 10, ToNearestEven) = 1.234567890123456E+16
//	Div64(1, 4, ToNearestEven) = 0.25
//	Div64(8.00, 2, ToNearestEven) = 4.00
//	Div64(2.400, 2.0, ToNearestEven) = 1.20
//	Div64(1000, 100, ToNearestEven) = 10
//	Div64(1000, 1, ToNearestEven) = 1000
//	Div64(2.40E+6, 2, ToNearestEven) = 1.20E+6
//	Div64(12, 12, ToNearestEven) = 1
//	FMA64(2, 3, 4, ToNearestEven) = 10
//	FMA64(1.5, 2, 0.25, ToNearestEven) = 3.25
//	FMA64(2.0, 3, 4.00, ToNearestEven) = 10.00
//	FMA64(1.000000000000001, 1.000000000000001, -1, ToNearestEven) = 2.000000000000001E-15
//	Rem64(10, 3) = 1
//	Rem64(10.0, 3) = 1.0
//	Rem64(10, 3.00) = 1.00
//	Mod64(10, 3) = 1
//	Mod64(10.0, 3) = 1.0
//	RoundToIntegral64(1E+3, ToNearestEven) = 1E+3
//	RoundToIntegral64(100.0, ToNearestEven) = 100
//	RoundToIntegral64(1.0E+6, ToNearestEven) = 1.0E+6
//	RoundToIntegral64(7.89E+77, ToNearestEven) = 7.89E+77
//	RoundToIntegral64(0.00, ToNearestEven) = 0
//	RoundToIntegral64(0E+2, ToNearestEven) = 0E+2
//	Quantize64(2.17, 0.001, ToNearestEven) = 2.170
//	Quantize64(2.17, 0.01, ToNearestEven) = 2.17
//	Quantize64(217, 0.1, ToNearestEven) = 217.0
//	Quantize64(217, 1, ToNearestEven) = 217
//	Quantize64(2.17, 0.00, ToNearestEven) = 2.17
//	Quantize64(1E+15, 1, ToNearestEven) = 1000000000000000
//	Reduce64(2.1) = 2.1
//	Reduce64(-2.0) = -2
//	Reduce64(1.200) = 1.2
//	Reduce64(-120) = -1.2E+2
//	Reduce64(120.00) = 1.2E+2
//	Reduce64(0.00) = 0
//	Reduce64(-0E+5) = -0
//	Reduce64(1.00E+371) = 1.00E+371
//	Reduce64(1.000E+371) = 1.00E+371
//	Widen(1.50) = 1.50
//	Widen(0E-398) = 0E-398
//	Widen(1.0E+370) = 1.0E+370
//	Narrow(1.50, ToNearestEven) = 1.50
//	Narrow(1E+370, ToNearestEven) = 1.0E+370
//	Narrow(1E+384, ToNearestEven) = 1.000000000000000E+384
//	Narrow(0E+1000, ToNearestEven) = 0E+369
//	Narrow(-0E-1000, ToNearestEven) = -0E-398
//	Narrow(1E-398, ToNearestEven) = 1E-398
//	Narrow(1.23456789E-390, ToNearestEven) = 1.23456789E-390
//	Int64To64(123, ToNearestEven) = 123
//	Int64To64(-1000, ToNearestEven) = -1000
//	Int64To64(0, ToNearestEven) = 0
//	Int64To128(-9223372036854775808) = -9223372036854775808
//	Uint64To128(18446744073709551615) = 18446744073709551615
//	Min64(1.0, 1) = 1.0
//	Min64(1, 1.0) = 1
//	Max64(1.0, 1) = 1.0
//	Parse64("1.50", ToNearestEven) = 1.50, Exact
//	Parse64("1.5E+3", ToNearestEven) = 1.5E+3, Exact
//	Parse64("1.5e3", ToNearestEven) = 1.5E+3, Exact
//	Parse64("123E+1", ToNearestEven) = 1.23E+3, Exact
//	Parse64("1.00E5", ToNearestEven) = 1.00E+5, Exact
//	Parse64("+1", ToNearestEven) = 1, Exact
//	Parse64(".5", ToNearestEven) = 0.5, Exact
//	Parse64("5.", ToNearestEven) = 5, Exact
//	Parse64("0.000001", ToNearestEven) = 0.000001, Exact
//	Parse64("0.0000001", ToNearestEven) = 1E-7, Exact
//	Parse64("10E-7", ToNearestEven) = 0.0000010, Exact
//	Parse64("12345678901234560", ToNearestEven) = 1.234567890123456E+16, Exact
//	Parse64("0.1000000000000000000", ToNearestEven) = 0.1000000000000000, Exact
//	Parse64("100", ToNearestEven) = 100, Exact
//	Parse64("1E+2", ToNearestEven) = 1E+2, Exact
//	Parse64("1.0E+2", ToNearestEven) = 1.0E+2, Exact
//	Round64(false, 1, 0, false, ToNearestEven) = 1, Exact
//	Round64(false, 10000000000000000, 0, false, ToNearestEven) = 1.000000000000000E+16, Exact
//	Round64(false, 100, -400, false, ToNearestEven) = 1E-398, Exact
//
// Exponent of an inexact result (least possible exponent, Precision
// digits) and the five modes on a tie and on a direction:
//
//	Div64(1, 3, ToNearestEven) = 0.3333333333333333
//	Div64(2, 3, ToNearestEven) = 0.6666666666666667
//	Div64(1, 3, TowardPositive) = 0.3333333333333334
//	Div64(1, 3, TowardZero) = 0.3333333333333333
//	Div64(-1, 3, TowardNegative) = -0.3333333333333334
//	Div64(-1, 3, TowardPositive) = -0.3333333333333333
//	Div64(1E+300, 3, ToNearestEven) = 3.333333333333333E+299
//	Add64(1E+16, 1, ToNearestEven) = 1.000000000000000E+16
//	Add64(9999999999999999, 1, ToNearestEven) = 1.000000000000000E+16
//	Add64(1E+16, 5, ToNearestEven) = 1.000000000000000E+16
//	Add64(1E+16, 15, ToNearestEven) = 1.000000000000002E+16
//	Add64(1E+16, 5, ToNearestAway) = 1.000000000000001E+16
//	Add64(1E+16, 5, TowardZero) = 1.000000000000000E+16
//	Add64(1E+16, 5, TowardPositive) = 1.000000000000001E+16
//	Add64(1E+16, 5, TowardNegative) = 1.000000000000000E+16
//	Add64(-1E+16, -5, ToNearestEven) = -1.000000000000000E+16
//	Add64(-1E+16, -5, ToNearestAway) = -1.000000000000001E+16
//	Add64(-1E+16, -5, TowardZero) = -1.000000000000000E+16
//	Add64(-1E+16, -5, TowardPositive) = -1.000000000000000E+16
//	Add64(-1E+16, -5, TowardNegative) = -1.000000000000001E+16
//	Add64(1.0E+370, 1, ToNearestEven) = 1.000000000000000E+370
//	Mul64(1234567890123456, 11, ToNearestEven) = 1.358024679135802E+16
//	Int64To64(9223372036854775807, ToNearestEven) = 9.223372036854776E+18
//	Int64To64(-9223372036854775808, ToNearestEven) = -9.223372036854776E+18
//	Int64To64(-9223372036854775808, TowardZero) = -9.223372036854775E+18
//	Int64To64(-9223372036854775808, TowardPositive) = -9.223372036854775E+18
//	Uint64To64(18446744073709551615, ToNearestEven) = 1.844674407370955E+19
//	Uint64To64(18446744073709551615, TowardPositive) = 1.844674407370956E+19
//	Narrow(0.3333333333333333333333333333333333, ToNearestEven) = 0.3333333333333333
//	Narrow(0.6666666666666666666666666666666667, ToNearestEven) = 0.6666666666666667
//	Narrow(1.0000000000000005, ToNearestEven) = 1.000000000000000
//	Narrow(1.0000000000000015, ToNearestEven) = 1.000000000000002
//	Narrow(1.0000000000000005, ToNearestAway) = 1.000000000000001
//	Narrow(-1.0000000000000005, TowardNegative) = -1.000000000000001
//	Narrow(-1.0000000000000005, TowardZero) = -1.000000000000000
//	Parse64("12345678901234567", ToNearestEven) = 1.234567890123457E+16, Inexact
//	Parse64("12345678901234565", ToNearestEven) = 1.234567890123456E+16, Inexact
//	Parse64("12345678901234565", ToNearestAway) = 1.234567890123457E+16, Inexact
//	Parse64("0.33333333333333333", ToNearestEven) = 0.3333333333333333, Inexact
//	Round64(false, 12345678901234567, 0, false, ToNearestEven) = 1.234567890123457E+16, Inexact
//	Round64(false, 12345678901234565, 0, false, ToNearestEven) = 1.234567890123456E+16, Inexact
//	Round64(false, 12345678901234565, 0, true, ToNearestEven) = 1.234567890123457E+16, Inexact
//	Round64(false, 1, 0, true, ToNearestEven) = 1.000000000000000, Inexact
//	Round64(false, 1, 0, true, TowardPositive) = 1.000000000000001, Inexact
//	Round64(true, 1, 0, true, TowardPositive) = -1.000000000000000, Inexact
//	Round64(true, 1, 0, true, TowardNegative) = -1.000000000000001, Inexact
//	Round64(false, 18446744073709551615, 0, false, ToNearestEven) = 1.844674407370955E+19, Inexact
//
// Exponent clamping of exact results and of zeros:
//
//	Mul64(1E+200, 1E+170, ToNearestEven) = 1.0E+370
//	Mul64(5E+369, 2, ToNearestEven) = 1.0E+370
//	Mul64(1E+200, 1E+184, ToNearestEven) = 1.000000000000000E+384
//	Add64(1.0E+370, 1.0E+370, ToNearestEven) = 2.0E+370
//	Mul64(1E-200, 1E-198, ToNearestEven) = 1E-398
//	Mul64(1.23456789E-200, 1E-190, ToNearestEven) = 1.23456789E-390
//	Mul64(0E+300, 1E+300, ToNearestEven) = 0E+369
//	Mul64(0E-300, 1E-300, ToNearestEven) = 0E-398
//	Div64(0, 1E+300, ToNearestEven) = 0E-300
//	Div64(0, 1E-300, ToNearestEven) = 0E+300
//	Div64(0, 1E-398, ToNearestEven) = 0E+369
//	Div64(0E-398, 1E+300, ToNearestEven) = 0E-398
//	Add64(0E+5, 0E+3, ToNearestEven) = 0E+3
//	Parse64("1E+370", ToNearestEven) = 1.0E+370, Exact
//	Parse64("1E+384", ToNearestEven) = 1.000000000000000E+384, Exact
//	Parse64("0E+400", ToNearestEven) = 0E+369, Exact
//	Parse64("-0E+400", ToNearestEven) = -0E+369, Exact
//	Parse64("0E-500", ToNearestEven) = 0E-398, Exact
//	Parse64("0E+99999999999", ToNearestEven) = 0E+369, Exact
//	Parse64("100E-400", ToNearestEven) = 1E-398, Exact
//	Round64(false, 1, 370, false, ToNearestEven) = 1.0E+370, Exact
//	Round64(false, 0, 400, false, ToNearestEven) = 0E+369, Exact
//	Round64(true, 0, -500, false, ToNearestEven) = -0E-398, Exact
//	Parse128("1E+6111", ToNearestEven) = 1E+6111, Exact
//	Parse128("1E+6112", ToNearestEven) = 1.0E+6112, Exact
//	Parse128("1E+6144", ToNearestEven) = 1.000000000000000000000000000000000E+6144, Exact
//	Parse128("0E-7000", ToNearestEven) = 0E-6176, Exact
//	Mul128(1E+3000, 1E+3112, ToNearestEven) = 1.0E+6112
//	Reduce128(1.00E+6113) = 1.00E+6113
//
// Sign of an exact zero sum or difference under each mode, and the x + x
// rule:
//
//	Add64(1, -1, ToNearestEven) = 0
//	Add64(1, -1, ToNearestAway) = 0
//	Add64(1, -1, TowardZero) = 0
//	Add64(1, -1, TowardPositive) = 0
//	Add64(1, -1, TowardNegative) = -0
//	Add64(-1, 1, TowardNegative) = -0
//	Sub64(1, 1, ToNearestEven) = 0
//	Sub64(1, 1, ToNearestAway) = 0
//	Sub64(1, 1, TowardZero) = 0
//	Sub64(1, 1, TowardPositive) = 0
//	Sub64(1, 1, TowardNegative) = -0
//	Sub64(-1, -1, TowardNegative) = -0
//	Add64(1.00, -1, ToNearestEven) = 0.00
//	Add64(1E+3, -1E+3, TowardNegative) = -0E+3
//	Add64(0, -0, ToNearestEven) = 0
//	Add64(0, -0, TowardNegative) = -0
//	Add64(-0, 0, ToNearestEven) = 0
//	Add64(-0, 0, TowardNegative) = -0
//	Sub64(0, 0, ToNearestEven) = 0
//	Sub64(0, 0, TowardNegative) = -0
//	Sub64(-0, -0, ToNearestEven) = 0
//	Sub64(-0, -0, TowardNegative) = -0
//	Add64(0, 0, ToNearestEven) = 0
//	Add64(0, 0, TowardNegative) = 0
//	Add64(-0, -0, ToNearestEven) = -0
//	Add64(-0, -0, TowardNegative) = -0
//	Sub64(0, -0, ToNearestEven) = 0
//	Sub64(0, -0, TowardNegative) = 0
//	Sub64(-0, 0, ToNearestEven) = -0
//	Sub64(-0, 0, TowardPositive) = -0
//	Add64(1E-398, -1E-398, ToNearestEven) = 0E-398
//	Add64(1E-398, -1E-398, TowardNegative) = -0E-398
//
// Sign of a zero product, quotient and fused result:
//
//	Mul64(-0, 5, TowardZero) = -0
//	Mul64(0, -3.0, ToNearestEven) = -0.0
//	Mul64(0.9, -0, ToNearestEven) = -0.0
//	Mul64(-0, -1E+5, ToNearestEven) = 0E+5
//	Mul64(-0, 5, TowardNegative) = -0
//	Div64(0, -2, ToNearestEven) = -0
//	Div64(-0, 2, ToNearestEven) = -0
//	Div64(-0, -2, ToNearestEven) = 0
//	Div64(0.00, 5, ToNearestEven) = 0.00
//	Div64(0, 5, TowardNegative) = 0
//	FMA64(2, 3, -6, ToNearestEven) = 0
//	FMA64(2, 3, -6, ToNearestAway) = 0
//	FMA64(2, 3, -6, TowardZero) = 0
//	FMA64(2, 3, -6, TowardPositive) = 0
//	FMA64(2, 3, -6, TowardNegative) = -0
//	FMA64(-2, 3, 6, ToNearestEven) = 0
//	FMA64(-2, 3, 6, TowardNegative) = -0
//	FMA64(0, 5, -0, ToNearestEven) = 0
//	FMA64(0, 5, -0, TowardNegative) = -0
//	FMA64(-0, 5, 0, ToNearestEven) = 0
//	FMA64(-0, 5, -0, ToNearestEven) = -0
//	FMA64(-0, 5, -0, TowardNegative) = -0
//	FMA64(0, 5, 0, TowardNegative) = 0
//	FMA64(2.0, 3, -6.00, ToNearestEven) = 0.00
//	FMA64(2, 3, -6.0, ToNearestEven) = 0.0
//	FMA64(-1, 0, 0, ToNearestEven) = 0
//	FMA64(-1, 0, 0, TowardNegative) = -0
//
// Sign of a zero from the single-operand and remainder operations and
// from inexact rounding to zero:
//
//	Rem64(6, 3) = 0
//	Rem64(-6, 3) = -0
//	Rem64(6.0, 3) = 0.0
//	Rem64(-0, 1) = -0
//	Rem64(0.00, 3) = 0.00
//	Mod64(-6, 3) = -0
//	Mod64(6, -3) = 0
//	RoundToIntegral64(-0.4, ToNearestEven) = -0
//	RoundToIntegral64(-0.5, ToNearestEven) = -0
//	RoundToIntegral64(-0.1, TowardZero) = -0
//	RoundToIntegral64(-0.1, TowardPositive) = -0
//	RoundToIntegral64(0.1, TowardNegative) = 0
//	RoundToIntegral64(-0.00, ToNearestEven) = -0
//	RoundToIntegral64(-1E-398, TowardPositive) = -0
//	Quantize64(-0.1, 1, ToNearestEven) = -0
//	Quantize64(-0, 1E+5, ToNearestEven) = -0E+5
//	Quantize64(0, 1E-17, ToNearestEven) = 0E-17
//	Quantize64(-0, 1E-20, TowardZero) = -0E-20
//	Quantize64(0E+369, 1E-398, ToNearestEven) = 0E-398
//	Quantize64(-0.001, 0.01, TowardPositive) = -0.00
//	Quantize64(0.001, 0.01, ToNearestEven) = 0.00
//	Reduce64(-0.00) = -0
//	Mul64(-1E-200, 1E-199, ToNearestEven) = -0E-398
//	Div64(-1E-398, 3, ToNearestEven) = -0E-398
//	Parse64("-1E-399", ToNearestEven) = -0E-398, Inexact|Underflow
//	Parse64("-0", ToNearestEven) = -0, Exact
//	Parse64("-0.00", ToNearestEven) = -0.00, Exact
//	Parse64("-0E-500", ToNearestEven) = -0E-398, Exact
//	Float64To64(-0, ToNearestEven) = -0
//
// Overflow under every mode, both signs, decided after rounding:
//
//	Mul64(1E+200, 1E+200, ToNearestEven) = Infinity
//	Mul64(1E+200, 1E+200, ToNearestAway) = Infinity
//	Mul64(1E+200, 1E+200, TowardZero) = 9.999999999999999E+384
//	Mul64(1E+200, 1E+200, TowardPositive) = Infinity
//	Mul64(1E+200, 1E+200, TowardNegative) = 9.999999999999999E+384
//	Mul64(-1E+200, 1E+200, ToNearestEven) = -Infinity
//	Mul64(-1E+200, 1E+200, ToNearestAway) = -Infinity
//	Mul64(-1E+200, 1E+200, TowardZero) = -9.999999999999999E+384
//	Mul64(-1E+200, 1E+200, TowardPositive) = -9.999999999999999E+384
//	Mul64(-1E+200, 1E+200, TowardNegative) = -Infinity
//	Mul64(1E+200, 1E+185, ToNearestEven) = Infinity
//	Mul64(1E+200, 1E+185, TowardNegative) = 9.999999999999999E+384
//	Add64(9.999999999999999E+384, 1E+369, ToNearestEven) = Infinity
//	Add64(9.999999999999999E+384, 1E+369, TowardZero) = 9.999999999999999E+384
//	Add64(9.999999999999999E+384, 1E+368, ToNearestEven) = 9.999999999999999E+384
//	Add64(9.999999999999999E+384, 5E+368, ToNearestEven) = Infinity
//	Add64(9.999999999999999E+384, 5E+368, TowardPositive) = Infinity
//	Add64(-9.999999999999999E+384, -5E+368, TowardPositive) = -9.999999999999999E+384
//	Div64(1E+300, 1E-300, ToNearestEven) = Infinity
//	FMA64(1E+200, 1E+200, -1E+300, ToNearestEven) = Infinity
//	Parse64("1E+385", ToNearestEven) = Infinity, Inexact|Overflow
//	Parse64("1E+385", TowardZero) = 9.999999999999999E+384, Inexact|Overflow
//	Parse64("-1E+385", TowardPositive) = -9.999999999999999E+384, Inexact|Overflow
//	Parse64("-1E+385", TowardNegative) = -Infinity, Inexact|Overflow
//	Parse64("99999999999999995E+368", ToNearestEven) = Infinity, Inexact|Overflow
//	Parse64("99999999999999995E+368", TowardZero) = 9.999999999999999E+384, Inexact
//	Parse64("9999999999999999E+369", ToNearestEven) = 9.999999999999999E+384, Exact
//	Parse64("1E+99999999999", ToNearestEven) = Infinity, Inexact|Overflow
//	Round64(false, 1, 400, false, ToNearestEven) = Infinity, Inexact|Overflow
//	Round64(false, 1, 400, false, TowardZero) = 9.999999999999999E+384, Inexact|Overflow
//	Round64(true, 1, 400, false, TowardPositive) = -9.999999999999999E+384, Inexact|Overflow
//	Round64(true, 1, 400, false, TowardNegative) = -Infinity, Inexact|Overflow
//	Narrow(1E+500, ToNearestEven) = Infinity
//	Narrow(1E+500, TowardZero) = 9.999999999999999E+384
//	Narrow(-1E+500, TowardPositive) = -9.999999999999999E+384
//	Int64To64(9223372036854775807, TowardPositive) = 9.223372036854776E+18
//	Mul128(1E+3000, 1E+3145, ToNearestEven) = Infinity
//	Mul128(1E+3000, 1E+3145, TowardZero) = 9.999999999999999999999999999999999E+6144
//	Mul128(-1E+3000, 1E+3145, TowardPositive) = -9.999999999999999999999999999999999E+6144
//	Mul128(-1E+3000, 1E+3145, TowardNegative) = -Infinity
//	Parse128("1E+6145", ToNearestEven) = Infinity, Inexact|Overflow
//
// Underflow: exact subnormals, inexact subnormals, rounding to zero and
// rounding up to the smallest normal:
//
//	Parse64("1E-398", ToNearestEven) = 1E-398, Exact
//	Parse64("1.23456789E-390", ToNearestEven) = 1.23456789E-390, Exact
//	Parse64("1.234567891E-390", ToNearestEven) = 1.23456789E-390, Inexact|Underflow
//	Parse64("1.234567891E-390", TowardPositive) = 1.23456790E-390, Inexact|Underflow
//	Parse64("1E-399", ToNearestEven) = 0E-398, Inexact|Underflow
//	Parse64("1E-399", TowardPositive) = 1E-398, Inexact|Underflow
//	Parse64("1E-399", TowardNegative) = 0E-398, Inexact|Underflow
//	Parse64("-1E-399", TowardPositive) = -0E-398, Inexact|Underflow
//	Parse64("-1E-399", TowardNegative) = -1E-398, Inexact|Underflow
//	Parse64("5E-399", ToNearestEven) = 0E-398, Inexact|Underflow
//	Parse64("5E-399", ToNearestAway) = 1E-398, Inexact|Underflow
//	Parse64("15E-399", ToNearestEven) = 2E-398, Inexact|Underflow
//	Parse64("25E-399", ToNearestEven) = 2E-398, Inexact|Underflow
//	Parse64("25E-399", ToNearestAway) = 3E-398, Inexact|Underflow
//	Parse64("9.999999999999999E-384", ToNearestEven) = 1.000000000000000E-383, Inexact|Underflow
//	Parse64("9.999999999999999E-384", TowardZero) = 9.99999999999999E-384, Inexact|Underflow
//	Parse64("1E-99999999999", ToNearestEven) = 0E-398, Inexact|Underflow
//	Round64(false, 15, -399, false, ToNearestEven) = 2E-398, Inexact|Underflow
//	Round64(false, 1, -399, false, TowardPositive) = 1E-398, Inexact|Underflow
//	Round64(false, 1, -399, false, ToNearestEven) = 0E-398, Inexact|Underflow
//	Round64(false, 0, 0, true, ToNearestEven) = 0E-398, Inexact|Underflow
//	Round64(false, 0, 0, true, TowardPositive) = 1E-398, Inexact|Underflow
//	Round64(true, 0, 0, true, TowardPositive) = -0E-398, Inexact|Underflow
//	Round64(true, 0, 0, true, TowardNegative) = -1E-398, Inexact|Underflow
//	Div64(1E-398, 3, ToNearestEven) = 0E-398
//	Div64(2E-398, 3, ToNearestEven) = 1E-398
//	Div64(1E-398, 3, TowardPositive) = 1E-398
//	Div64(-1E-398, 3, TowardNegative) = -1E-398
//	Div64(1.234567891E-380, 1E+10, ToNearestEven) = 1.23456789E-390
//	Mul64(1E-200, 1E-199, ToNearestEven) = 0E-398
//	Mul64(5E-200, 1E-199, ToNearestEven) = 0E-398
//	Mul64(5E-200, 1E-199, ToNearestAway) = 1E-398
//	Mul64(1.5E-200, 1E-198, ToNearestEven) = 2E-398
//	Mul64(2.5E-200, 1E-198, ToNearestEven) = 2E-398
//	Narrow(1E-400, ToNearestEven) = 0E-398
//	Narrow(1E-400, TowardPositive) = 1E-398
//	Narrow(1.5E-398, ToNearestEven) = 2E-398
//	Parse128("1E-6176", ToNearestEven) = 1E-6176, Exact
//	Parse128("1E-6177", ToNearestEven) = 0E-6176, Inexact|Underflow
//	Parse128("1E-6177", TowardPositive) = 1E-6176, Inexact|Underflow
//	Mul128(1E-3000, 1E-3176, ToNearestEven) = 1E-6176
//
// Invalid operations and the defined infinity cases:
//
//	Add64(Infinity, -Infinity, ToNearestEven) = NaN
//	Add64(-Infinity, Infinity, ToNearestEven) = NaN
//	Sub64(Infinity, Infinity, ToNearestEven) = NaN
//	Sub64(-Infinity, -Infinity, ToNearestEven) = NaN
//	Add64(Infinity, Infinity, ToNearestEven) = Infinity
//	Add64(-Infinity, -Infinity, ToNearestEven) = -Infinity
//	Sub64(-Infinity, Infinity, ToNearestEven) = -Infinity
//	Add64(Infinity, -9.999999999999999E+384, ToNearestEven) = Infinity
//	Sub64(1, Infinity, ToNearestEven) = -Infinity
//	Mul64(0, Infinity, ToNearestEven) = NaN
//	Mul64(-Infinity, 0, ToNearestEven) = NaN
//	Mul64(0.00, -Infinity, ToNearestEven) = NaN
//	Mul64(Infinity, -2, ToNearestEven) = -Infinity
//	Mul64(-1, Infinity, ToNearestEven) = -Infinity
//	Mul64(Infinity, Infinity, ToNearestEven) = Infinity
//	Mul64(-Infinity, -Infinity, ToNearestEven) = Infinity
//	Div64(0, 0, ToNearestEven) = NaN
//	Div64(-0, 0, ToNearestEven) = NaN
//	Div64(Infinity, Infinity, ToNearestEven) = NaN
//	Div64(-Infinity, Infinity, ToNearestEven) = NaN
//	Div64(1, 0, ToNearestEven) = Infinity
//	Div64(1, -0, ToNearestEven) = -Infinity
//	Div64(-1, 0, ToNearestEven) = -Infinity
//	Div64(-1, -0, ToNearestEven) = Infinity
//	Div64(1E-398, 0, TowardZero) = Infinity
//	Div64(Infinity, 0, ToNearestEven) = Infinity
//	Div64(Infinity, -0, ToNearestEven) = -Infinity
//	Div64(Infinity, 5, ToNearestEven) = Infinity
//	Div64(Infinity, -5, ToNearestEven) = -Infinity
//	Div64(1, Infinity, ToNearestEven) = 0E-398
//	Div64(-1, Infinity, ToNearestEven) = -0E-398
//	Div64(1, -Infinity, ToNearestEven) = -0E-398
//	Div64(-1000, -Infinity, TowardNegative) = 0E-398
//	Div64(0, Infinity, ToNearestEven) = 0E-398
//	Div64(-0, Infinity, ToNearestEven) = -0E-398
//	FMA64(0, Infinity, 1, ToNearestEven) = NaN
//	FMA64(Infinity, 0, 1, ToNearestEven) = NaN
//	FMA64(Infinity, 0, NaN, ToNearestEven) = NaN
//	FMA64(Infinity, 1, -Infinity, ToNearestEven) = NaN
//	FMA64(-Infinity, 1, Infinity, ToNearestEven) = NaN
//	FMA64(Infinity, 1, Infinity, ToNearestEven) = Infinity
//	FMA64(Infinity, -1, -Infinity, ToNearestEven) = -Infinity
//	FMA64(1E+200, 1E+200, -Infinity, ToNearestEven) = -Infinity
//	FMA64(1, 1, Infinity, ToNearestEven) = Infinity
//	FMA64(1, 1, NaN, ToNearestEven) = NaN
//	Rem64(Infinity, 1) = NaN
//	Rem64(-Infinity, Infinity) = NaN
//	Rem64(1, 0) = NaN
//	Rem64(0, 0) = NaN
//	Rem64(1, -0) = NaN
//	Rem64(1, Infinity) = 1
//	Rem64(-1000, Infinity) = -1000
//	Rem64(-0, Infinity) = -0
//	Rem64(1.50, -Infinity) = 1.50
//	Mod64(Infinity, 1) = NaN
//	Mod64(1, 0) = NaN
//	Mod64(0, 0) = NaN
//	Mod64(1, Infinity) = 1
//	Mod64(-0, Infinity) = -0
//	RoundToIntegral64(Infinity, ToNearestEven) = Infinity
//	RoundToIntegral64(-Infinity, TowardZero) = -Infinity
//	RoundToIntegral64(NaN, ToNearestEven) = NaN
//	Quantize64(Infinity, Infinity, ToNearestEven) = Infinity
//	Quantize64(-Infinity, Infinity, ToNearestEven) = -Infinity
//	Quantize64(Infinity, -Infinity, ToNearestEven) = Infinity
//	Quantize64(Infinity, 1, ToNearestEven) = NaN
//	Quantize64(2, Infinity, ToNearestEven) = NaN
//	Quantize64(1E+16, 1, ToNearestEven) = NaN
//	Quantize64(1234567890123456, 0.1, ToNearestEven) = NaN
//	Quantize64(-123456789012345.6, 0.01, ToNearestEven) = NaN
//	Quantize64(1, 1E-398, ToNearestEven) = NaN
//	Quantize64(9.999999999999999E+384, 1E+368, ToNearestEven) = NaN
//	Quantize64(NaN, 1, ToNearestEven) = NaN
//	Quantize64(1, NaN, ToNearestEven) = NaN
//	Quantize128(1E+34, 1, ToNearestEven) = NaN
//	Quantize128(1E+33, 1, ToNearestEven) = 1000000000000000000000000000000000
//	Reduce64(Infinity) = Infinity
//	Reduce64(-Infinity) = -Infinity
//	Reduce64(NaN) = NaN
//	Widen(NaN) = NaN
//	Widen(sNaN) = NaN
//	Widen(-Infinity) = -Infinity
//	Narrow(NaN, ToNearestEven) = NaN
//	Narrow(-Infinity, ToNearestEven) = -Infinity
//	Add64(1, NaN, ToNearestEven) = NaN
//	Add64(NaN, Infinity, ToNearestEven) = NaN
//	Sub64(1, NaN, ToNearestEven) = NaN
//	Add64(sNaN, 1, ToNearestEven) = NaN
//	Add64(-NaN, 1, ToNearestEven) = NaN
//	Mul64(NaN, 0, ToNearestEven) = NaN
//	Div64(NaN, 0, ToNearestEven) = NaN
//	Rem64(1, NaN) = NaN
//	Min64(NaN, 1) = NaN
//	Max64(1, NaN) = NaN
//	Max64(1, sNaN) = NaN
//
// Remainder and modulo, including quotients GDA refuses:
//
//	Rem64(11, 3) = -1
//	Rem64(-10, 3) = -1
//	Rem64(10, -3) = 1
//	Rem64(-10, -3) = -1
//	Rem64(10, 6) = -2
//	Rem64(10.0, 6) = -2.0
//	Rem64(2.1, 3) = -0.9
//	Rem64(10.2, 1) = 0.2
//	Rem64(10, 0.3) = 0.1
//	Rem64(3.6, 1.3) = -0.3
//	Rem64(5, 2) = 1
//	Rem64(7, 2) = -1
//	Rem64(-7, 2) = 1
//	Rem64(2.5, 1) = 0.5
//	Rem64(3.5, 1) = -0.5
//	Rem64(7.5, 5) = -2.5
//	Rem64(1.5, 3) = 1.5
//	Rem64(4.5, 3) = -1.5
//	Rem64(1, 3) = 1
//	Rem64(1E+2, 3) = 1
//	Rem64(1E+16, 3) = 1
//	Rem64(1E+30, 3) = 1
//	Rem64(1E+30, 6) = -2
//	Rem64(1E+30, 7) = 1
//	Rem64(1.000000000000000E+384, 1) = 0
//	Rem64(1.000000000000000E+384, 3) = 1
//	Rem64(9.999999999999999E+384, 2) = 0
//	Rem64(1234567890123456, 0.1) = 0.0
//	Rem64(1, 1E-398) = 0E-398
//	Mod64(11, 3) = 2
//	Mod64(-10, 3) = -1
//	Mod64(10, -3) = 1
//	Mod64(-10, -3) = -1
//	Mod64(10, 6) = 4
//	Mod64(10.0, 6) = 4.0
//	Mod64(10, 6.0) = 4.0
//	Mod64(2.1, 3) = 2.1
//	Mod64(10.2, 1) = 0.2
//	Mod64(10, 0.3) = 0.1
//	Mod64(3.6, 1.3) = 1.0
//	Mod64(7, 2) = 1
//	Mod64(-7, 2) = -1
//	Mod64(7.5, 2) = 1.5
//	Mod64(1E+2, 3) = 1
//	Mod64(1E+30, 6) = 4
//	Mod64(1E+30, 7) = 1
//	Mod64(1.000000000000000E+384, 7) = 1
//	Mod64(1234567890123456, 0.1) = 0.0
//	Rem128(1E+6000, 7) = 1
//	Rem128(1E+6000, 6) = -2
//	Mod128(1E+6000, 6) = 4
//
// Rounding to an integral value and quantizing under the modes:
//
//	RoundToIntegral64(2.1, ToNearestEven) = 2
//	RoundToIntegral64(1.5, ToNearestEven) = 2
//	RoundToIntegral64(2.5, ToNearestEven) = 2
//	RoundToIntegral64(2.5, ToNearestAway) = 3
//	RoundToIntegral64(-1.5, ToNearestEven) = -2
//	RoundToIntegral64(-1.5, ToNearestAway) = -2
//	RoundToIntegral64(-1.5, TowardZero) = -1
//	RoundToIntegral64(-1.5, TowardPositive) = -1
//	RoundToIntegral64(-1.5, TowardNegative) = -2
//	RoundToIntegral64(101.5, ToNearestEven) = 102
//	RoundToIntegral64(-101.5, ToNearestEven) = -102
//	RoundToIntegral64(0.5, ToNearestEven) = 0
//	RoundToIntegral64(0.5, ToNearestAway) = 1
//	RoundToIntegral64(-0.5, ToNearestAway) = -1
//	RoundToIntegral64(999999999999999.9, ToNearestEven) = 1000000000000000
//	RoundToIntegral64(9999999999999999, ToNearestEven) = 9999999999999999
//	RoundToIntegral64(1E-398, ToNearestEven) = 0
//	RoundToIntegral64(-1E-398, TowardNegative) = -1
//	Quantize64(2.17, 0.1, ToNearestEven) = 2.2
//	Quantize64(2.17, 1, ToNearestEven) = 2
//	Quantize64(2.17, 1E+1, ToNearestEven) = 0E+1
//	Quantize64(2.17, 0E+1, ToNearestEven) = 0E+1
//	Quantize64(2.5, 1, ToNearestEven) = 2
//	Quantize64(2.5, 1, ToNearestAway) = 3
//	Quantize64(-2.5, 1, ToNearestEven) = -2
//	Quantize64(-2.5, 1, TowardNegative) = -3
//	Quantize64(150, 1E+2, ToNearestEven) = 2E+2
//	Quantize64(250, 1E+2, ToNearestEven) = 2E+2
//	Quantize64(250, 1E+2, ToNearestAway) = 3E+2
//	Quantize64(-250, 1E+2, TowardZero) = -2E+2
//	Quantize64(0.001, 0.01, TowardPositive) = 0.01
//	Quantize64(0.1, 1E-10, ToNearestEven) = 0.1000000000
//	Quantize64(1, 1E+2, ToNearestEven) = 0E+2
//	Quantize64(1E-398, 1E-397, ToNearestEven) = 0E-397
//	Quantize64(1E-398, 1E-397, TowardPositive) = 1E-397
//
// Comparisons, total order, same quantum and min/max:
//
//	Equal64(1.0, 1.00) = true
//	Equal64(0, -0) = true
//	Equal64(0E+369, -0E-398) = true
//	Equal64(NaN, NaN) = false
//	Equal64(sNaN, sNaN) = false
//	Equal64(Infinity, Infinity) = true
//	Equal64(1, 1.000000000000001) = false
//	Less64(-0, 0) = false
//	Less64(-Infinity, -9.999999999999999E+384) = true
//	Less64(9.999999999999999E+384, Infinity) = true
//	Less64(1E-398, 2E-398) = true
//	Less64(1, NaN) = false
//	Less64(NaN, 1) = false
//	LessEqual64(1.0, 1) = true
//	LessEqual64(1, NaN) = false
//	LessEqual64(-0, 0) = true
//	Compare64(1, 2) = -1, true
//	Compare64(2, 1) = 1, true
//	Compare64(1.0, 1) = 0, true
//	Compare64(-0, 0) = 0, true
//	Compare64(1, NaN) = 0, false
//	Compare64(NaN, NaN) = 0, false
//	Compare64(sNaN, 1) = 0, false
//	TotalOrder64(-0, 0) = true
//	TotalOrder64(0, -0) = false
//	TotalOrder64(1.0, 1) = true
//	TotalOrder64(1, 1.0) = false
//	TotalOrder64(-1, -1.0) = true
//	TotalOrder64(-1.0, -1) = false
//	TotalOrder64(1.0, 1.0) = true
//	TotalOrder64(0E+369, 0E-398) = false
//	TotalOrder64(0E-398, 0E+369) = true
//	TotalOrder64(-Infinity, -9.999999999999999E+384) = true
//	TotalOrder64(9.999999999999999E+384, Infinity) = true
//	TotalOrder64(Infinity, NaN) = true
//	TotalOrder64(NaN, Infinity) = false
//	TotalOrder64(-NaN, -Infinity) = true
//	TotalOrder64(-Infinity, -NaN) = false
//	TotalOrder64(-NaN, NaN) = true
//	TotalOrder64(NaN, -NaN) = false
//	TotalOrder64(NaN, NaN) = true
//	TotalOrder64(sNaN, NaN) = true
//	TotalOrder64(NaN, sNaN) = true
//	TotalOrder64(-sNaN, -NaN) = true
//	SameQuantum64(1.0, 2.0) = true
//	SameQuantum64(1.0, 1) = false
//	SameQuantum64(Infinity, -Infinity) = true
//	SameQuantum64(NaN, sNaN) = true
//	SameQuantum64(Infinity, NaN) = false
//	SameQuantum64(1, Infinity) = false
//	Min64(3, 2) = 2
//	Min64(-10, 3) = -10
//	Max64(3, 2) = 3
//	Max64(-10, 3) = 3
//	Min64(0, -0) = -0
//	Min64(-0, 0) = -0
//	Max64(-0, 0) = 0
//	Max64(0, -0) = 0
//	Max64(1, 1.0) = 1
//	Min64(-1.0, -1) = -1.0
//	Min64(-Infinity, 5) = -Infinity
//	Max64(Infinity, 5) = Infinity
//
// Sign operations (bit operations on any encoding):
//
//	Neg64(1.50) = -1.50
//	Neg64(-0) = 0
//	Neg64(Infinity) = -Infinity
//	Neg64(NaN) = -NaN
//	Neg64(sNaN) = -sNaN
//	Abs64(-1.50) = 1.50
//	Abs64(-0E+5) = 0E+5
//	Abs64(-NaN) = NaN
//	Abs64(-sNaN) = sNaN
//	CopySign64(1.50, -0) = -1.50
//	CopySign64(-1.50, 2) = 1.50
//	CopySign64(Infinity, -NaN) = -Infinity
//	CopySign64(-sNaN, 1) = sNaN
//
// Integer conversions:
//
//	Int64From64(1E+2, TowardZero) = 100, Exact
//	Int64From64(-0, TowardZero) = 0, Exact
//	Int64From64(1.5, TowardZero) = 1, Inexact
//	Int64From64(-1.5, TowardZero) = -1, Inexact
//	Int64From64(1.5, ToNearestEven) = 2, Inexact
//	Int64From64(2.5, ToNearestEven) = 2, Inexact
//	Int64From64(2.5, ToNearestAway) = 3, Inexact
//	Int64From64(-1.5, TowardNegative) = -2, Inexact
//	Int64From64(-1.5, TowardPositive) = -1, Inexact
//	Int64From64(0.001, TowardZero) = 0, Inexact
//	Int64From64(-0.001, TowardZero) = 0, Inexact
//	Int64From64(1E-398, TowardPositive) = 1, Inexact
//	Int64From64(9.223372036854775E+18, TowardZero) = 9223372036854775000, Exact
//	Int64From64(9.223372036854776E+18, TowardZero) = 9223372036854775807, Inexact|Overflow
//	Int64From64(1E+19, TowardZero) = 9223372036854775807, Inexact|Overflow
//	Int64From64(-1E+19, TowardZero) = -9223372036854775808, Inexact|Overflow
//	Int64From64(Infinity, TowardZero) = 9223372036854775807, Invalid
//	Int64From64(-Infinity, TowardZero) = -9223372036854775808, Invalid
//	Int64From64(NaN, TowardZero) = 0, Invalid
//	Int64From64(sNaN, TowardZero) = 0, Invalid
//	Int64From128(9223372036854775807, TowardZero) = 9223372036854775807, Exact
//	Int64From128(-9223372036854775808, TowardZero) = -9223372036854775808, Exact
//	Int64From128(9223372036854775807.5, TowardZero) = 9223372036854775807, Inexact
//	Int64From128(9223372036854775807.5, ToNearestEven) = 9223372036854775807, Inexact|Overflow
//	Int64From128(9223372036854775808, TowardZero) = 9223372036854775807, Inexact|Overflow
//	Int64From128(-9223372036854775808.5, TowardZero) = -9223372036854775808, Inexact
//	Uint64From64(1.844674407370955E+19, TowardZero) = 18446744073709550000, Exact
//	Uint64From64(2E+19, TowardZero) = 18446744073709551615, Inexact|Overflow
//	Uint64From64(-1, TowardZero) = 0, Inexact|Overflow
//	Uint64From64(-1.5, TowardZero) = 0, Inexact|Overflow
//	Uint64From64(-0.5, TowardZero) = 0, Inexact
//	Uint64From64(-0.5, ToNearestEven) = 0, Inexact
//	Uint64From64(-0.5, ToNearestAway) = 0, Inexact|Overflow
//	Uint64From64(-0, TowardZero) = 0, Exact
//	Uint64From64(-Infinity, TowardZero) = 0, Invalid
//	Uint64From64(Infinity, TowardZero) = 18446744073709551615, Invalid
//	Uint64From64(NaN, TowardZero) = 0, Invalid
//	Uint64From128(18446744073709551615, TowardZero) = 18446744073709551615, Exact
//	Uint64From128(18446744073709551616, TowardZero) = 18446744073709551615, Inexact|Overflow
//
// Binary floating-point conversions (binary values as Go literals):
//
//	Float64To64(0.5, ToNearestEven) = 0.5
//	Float64To64(2, ToNearestEven) = 2
//	Float64To64(0.125, ToNearestEven) = 0.125
//	Float64To64(1e20, ToNearestEven) = 1.000000000000000E+20
//	Float64To128(1e20, ToNearestEven) = 100000000000000000000
//	Float64To64(0.1, ToNearestEven) = 0.1000000000000000
//	Float64To64(0.1, TowardPositive) = 0.1000000000000001
//	Float64To64(0.1, TowardZero) = 0.1000000000000000
//	Float64To128(0.1, ToNearestEven) = 0.1000000000000000055511151231257827
//	Float64To128(0.1, TowardPositive) = 0.1000000000000000055511151231257828
//	Float64To64(0.3, ToNearestEven) = 0.3000000000000000
//	Float64To64(1e23, ToNearestEven) = 9.999999999999999E+22
//	Float64To128(1e23, ToNearestEven) = 99999999999999991611392
//	Float64To64(1.7976931348623157e308, ToNearestEven) = 1.797693134862316E+308
//	Float64To64(1.7976931348623157e308, TowardZero) = 1.797693134862315E+308
//	Float64To64(5e-324, ToNearestEven) = 4.940656458412465E-324
//	Float64To64(5e-324, TowardPositive) = 4.940656458412466E-324
//	Float64To64(+Inf, ToNearestEven) = Infinity
//	Float64To64(-Inf, ToNearestEven) = -Infinity
//	Float64To64(NaN, ToNearestEven) = NaN
//	Float32To64(0.1, ToNearestEven) = 0.1000000014901161
//	Float32To128(0.1, ToNearestEven) = 0.100000001490116119384765625
//	Float32To64(16777216, ToNearestEven) = 16777216
//	Float64From64(0.5, ToNearestEven) = 0.5
//	Float64From64(0.1, ToNearestEven) = 0.1
//	Float64From64(123.456, ToNearestEven) = 123.456
//	Float64From64(-0, ToNearestEven) = -0
//	Float64From64(0E+369, ToNearestEven) = 0
//	Float64From64(9007199254740993, ToNearestEven) = 9007199254740992
//	Float64From64(9007199254740995, ToNearestEven) = 9007199254740996
//	Float64From64(9007199254740995, TowardZero) = 9007199254740994
//	Float64From64(1E+309, ToNearestEven) = +Inf
//	Float64From64(1E+309, TowardZero) = 1.7976931348623157e308
//	Float64From64(-1E+309, TowardPositive) = -1.7976931348623157e308
//	Float64From64(-1E+309, TowardNegative) = -Inf
//	Float64From128(1E-400, ToNearestEven) = 0
//	Float64From128(-1E-400, ToNearestEven) = -0
//	Float64From128(-1E-400, TowardNegative) = -5e-324
//	Float64From64(1E-398, TowardPositive) = 5e-324
//	Float64From64(1E-398, ToNearestEven) = 0
//	Float64From64(Infinity, ToNearestEven) = +Inf
//	Float64From64(-Infinity, ToNearestEven) = -Inf
//	Float64From64(NaN, ToNearestEven) = NaN
//	Float32From64(0.1, ToNearestEven) = 0.1
//	Float32From64(1E+39, ToNearestEven) = +Inf
//	Float32From64(1E+39, TowardZero) = 3.4028235e38
//	Float32From64(16777217, ToNearestEven) = 16777216
//	Float32From64(16777219, ToNearestEven) = 16777220
//
// Character conversions, including the grammar's edges:
//
//	Parse64("0.00", ToNearestEven) = 0.00, Exact
//	Parse64("0", ToNearestEven) = 0, Exact
//	Parse64("0E+1", ToNearestEven) = 0E+1, Exact
//	Parse64("1E-7", ToNearestEven) = 1E-7, Exact
//	Parse64("1e5", ToNearestEven) = 1E+5, Exact
//	Parse64("inf", ToNearestEven) = Infinity, Exact
//	Parse64("-Infinity", ToNearestEven) = -Infinity, Exact
//	Parse64("+INF", ToNearestEven) = Infinity, Exact
//	Parse64("nan", ToNearestEven) = NaN, Exact
//	Parse64("-NaN", ToNearestEven) = NaN, Exact
//	Parse64("snan", ToNearestEven) = NaN, Exact
//	Parse64("sNaN", ToNearestEven) = NaN, Exact
//	Parse64("", ToNearestEven) = NaN, Syntax
//	Parse64("-", ToNearestEven) = NaN, Syntax
//	Parse64(".", ToNearestEven) = NaN, Syntax
//	Parse64("1e", ToNearestEven) = NaN, Syntax
//	Parse64("1e+", ToNearestEven) = NaN, Syntax
//	Parse64("e5", ToNearestEven) = NaN, Syntax
//	Parse64(" 1", ToNearestEven) = NaN, Syntax
//	Parse64("1 ", ToNearestEven) = NaN, Syntax
//	Parse64("1_000", ToNearestEven) = NaN, Syntax
//	Parse64("0x10", ToNearestEven) = NaN, Syntax
//	Parse64("1.2.3", ToNearestEven) = NaN, Syntax
//	Parse64("nan123", ToNearestEven) = NaN, Syntax
//	Parse64("infinit", ToNearestEven) = NaN, Syntax
//	Parse64("Infinity1", ToNearestEven) = NaN, Syntax
//	Parse128("0.1000000000000000055511151231257827", ToNearestEven) = 0.1000000000000000055511151231257827, Exact
//	Parse128("12345678901234567890123456789012345", ToNearestEven) = 1.234567890123456789012345678901234E+34, Inexact
//	Parse128("12345678901234567890123456789012345", ToNearestAway) = 1.234567890123456789012345678901235E+34, Inexact
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
// # Width parameterisation
//
// The two widths share one implementation of every algorithm through code
// generation: each width-dependent algorithm is written once, in a
// template, and a generator instantiates it for decimal64 and for
// decimal128. The alternatives, generics over a width type and one core
// over a 128-bit intermediate with the format parameters as run-time
// values, were rejected on measurement. This section records the spike
// that decided it (2026-10-30, target T1.1.2) and the rules that the
// shared core must follow. The spike code was discarded; the numbers and
// the decision are what remain.
//
// The spike implemented addition and multiplication for both widths four
// times over one algorithm: unpack; align exponents by scaling the
// coefficient with the larger exponent up to Precision+3 digits and
// truncating the other with a sticky bit; add or subtract; for
// multiplication take the full double-width product and reduce it by a
// power of ten to Precision+1 digits with a sticky bit; round once in the
// given mode with exponent clamping, overflow and subnormal handling;
// pack. The four forms were hand-written per width (the baseline),
// generated from a template, generic over a width type whose arithmetic is
// reached through methods of the type parameter (the format parameters
// passed by pointer, so the dictionary carried only the methods), and a
// single core over Uint128 coefficients and a 256-bit product with the
// format parameters passed by pointer. All four agreed bit for bit with
// each other in all five rounding modes, and with a math/big reference in
// ToNearestEven, on 4096 random operand pairs per width (70% with
// exponents within 4 of each other, 20% spread over ±(Precision+10),
// 10% equal) and on the special values. The strawman is the runtime of
// this tree's decimal64 and decimal128 operators, timed through the
// operators; float64 is the hardware operation.
//
// Timings are benchstat medians of 20 interleaved passes of 300000
// iterations each on one CPU; ± is the spread. Apple M4 Max, macOS 26.6.2,
// this tree at 7807ba793d (go1.26 development), 2026-10-30, with a load
// average near 40 from concurrent toolchain builds. Fixed iteration counts
// and interleaving keep the comparison fair under that load; the absolute
// numbers may be pessimistic.
//
// darwin/arm64, native:
//
//	        hand       generated   generic    wide core   strawman    float64
//	Add64   10.2n ±6%  10.6n ±4%   22.7n ±1%  17.3n ±2%   10.8n ±7%   0.71n ±3%
//	Mul64   10.6n ±4%  11.1n ±4%   23.4n ±2%  15.0n ±3%   44.6n ±1%   0.91n ±1%
//	Add128  15.8n ±4%  15.5n ±4%   27.9n ±1%  18.4n ±5%   29.4n ±2%
//	Mul128  21.5n ±2%  22.5n ±3%   35.3n ±1%  23.8n ±3%    266n ±2%
//
// Relative to hand-written (benchstat, n=20, ~ means no significant
// difference): generated ~ (p=0.08), ~ (p=0.09), ~ (p=0.13), +4.8%
// (p=0.001); generic +122%, +120%, +77%, +64%; wide core +69%, +41%,
// +16%, +11%.
//
// darwin/amd64 under Rosetta 2 on the same machine:
//
//	        hand       generated   generic    wide core   strawman    float64
//	Add64   11.7n ±4%  12.8n ±4%   37.9n ±3%  19.3n ±6%   11.2n ±4%   0.70n ±2%
//	Mul64   43.3n ±1%  43.8n ±1%   64.0n ±3%  43.9n ±1%   48.2n ±0%   0.70n ±2%
//	Add128  37.4n ±1%  37.3n ±3%   60.3n ±4%  39.3n ±1%   33.2n ±3%
//	Mul128   148n ±1%   148n ±1%    165n ±1%   149n ±1%    304n ±1%
//
// Relative to hand-written: generated +9.8% (p=0.000), +1.1%, ~ (p=0.67),
// ~ (p=0.49); generic +225%, +48%, +61%, +11%; wide core +66%, +1.4%,
// +5.1%, +1.2%. Rosetta's translation of the 64-bit divide dominates every
// multiply and flattens the differences there; the Add64 row is the
// informative one.
//
// linux/amd64 is unmeasured: no Linux host was reachable when the spike
// ran. The Rosetta numbers stand in only for the relative ordering of the
// candidates and are not linux/amd64 timings. The 10% gap between
// generated and hand-written Add64 under Rosetta, absent natively, is
// unexplained; the first implementation target should repeat the Add64
// and Mul64 rows on real linux/amd64 hardware before tuning anything for
// it.
//
// Why generation. The generated code is the hand-written code. Every
// coefficient helper of the decimal64 instantiation inlines (the compiler
// reports "inlining call to" for all twenty and "cannot inline" for none),
// the generated add64 is 264 instructions against 256 hand-written and
// mul64 108 against 120, the stack frames are the same size, there are no
// indirect calls, nothing allocates, and the timings are within noise
// natively at both widths. One template is one source of truth for each
// algorithm, which is the proposal's claim that decimal64 is nearly free
// once decimal128 exists.
//
// Why not generics. A method call on a value of type-parameter type
// compiles to a load from the dictionary and an indirect call, in every
// shape instantiation including go.shape.uint64, and the inliner cannot
// see through it: the generic add had 23 indirect calls and the multiply
// 14, with half again as much code. The compiler devirtualizes such calls
// only after inlining the whole generic function into its caller, which a
// 250-instruction arithmetic routine never is. A constraint whose type set
// unites uint64 and Uint128 has no core type, so the operators are
// unavailable on the type parameter and methods are the only way to reach
// the arithmetic; a traits type parameter (a zero-size struct carrying the
// operations) makes the same indirect calls. The runtime can call generic
// code in an imported package (it instantiates
// internal/runtime/atomic.Pointer[T]; the dictionary is a static read-only
// symbol and nothing allocates), so the objection is cost, not
// possibility: 2.2× on decimal64 natively, 3.3× under Rosetta.
//
// Why not one core over a wide intermediate. It is the simplest form and
// needs no generator, and for decimal128 it is the hand-written code with
// the format parameters loaded from memory (+11% to +16% natively). But
// decimal64 then pays two-word arithmetic for every coefficient operation
// and a 128-by-64 division where a 64-by-64 one would do: +69% on addition
// and +41% on multiplication natively, +66% on addition under Rosetta
// (320 instructions in the core add against 256). That fails the
// requirement that decimal64 stay within noise of a width-specific
// implementation.
//
// Runtime and bootstrap constraints checked by the spike:
//
//   - The package is absent from runtimePkgs in
//     cmd/internal/objabi/pkgspecial.go (internal/strconv is present), so
//     it is compiled as an ordinary package: implicit allocation is not
//     diagnosed, and under -race, -msan or -asan it is instrumented although
//     the runtime calls it. Adding it to that list is a prerequisite of the
//     runtime import whatever the width strategy; the generated form needs
//     nothing else from that list.
//   - No function needs //go:nosplit; the strawman's dadd64 is not nosplit
//     either. Generated frames: add64 0x48 bytes of locals, mul64 0x48,
//     add128 0xc8, mul128 0x78, the same as hand-written.
//   - Uint128 is passed and returned in two registers under the register
//     ABI on arm64 and amd64 in every candidate, including the
//     go.shape.struct instantiation; the two-word struct costs nothing over
//     (hi, lo) pairs, as "Representation of decimal128" assumes.
//   - The generated files are ordinary Go that the minimum bootstrap
//     toolchain compiles. The template and the generator carry
//     //go:build ignore, which the bootstrap build honours; no go:generate
//     step runs in any build.
//   - No candidate allocates (0 B/op in every benchmark).
//
// Rules for the shared core. These bind the 128/256-bit arithmetic target
// (T1.3.1) and every operation implemented after it.
//
//  1. Source of truth. Every width-dependent algorithm lives in a template
//     NAME_tmpl.go (//go:build ignore), written once in the template
//     vocabulary below, one template per family of operations. gen_arith.go
//     (//go:build ignore, run by "//go:generate go run gen_arith.go" in
//     arith.go) writes zNAME64.go and zNAME128.go from each template, which
//     are committed, and zpow10.go, the tables of powers of ten that the
//     helpers below index. Nobody edits a z*.go file. A test,
//     TestGenerated, guarded by testenv.MustHaveGoRun, regenerates into a
//     scratch directory and fails on any difference from the committed
//     files.
//  2. The width suffix rule. In the template every width-dependent
//     identifier ends in a capital W and no other identifier does. The
//     generator replaces that final W with 64 or 128 and does nothing
//     else: PrecisionW, MaxExpW, MinExpW, BiasW, UnpackW, digitsW, addW,
//     roundW. The package's existing width-suffixed names (Precision64,
//     Unpack64, digits64, ...) are therefore usable from the template as
//     they are, and the template reads as Go. No text/template syntax, no
//     other substitution.
//  3. Types. coefW is the coefficient type, declared as the aliases
//     "type coef64 = uint64" in coef64.go and "type coef128 = Uint128" in
//     coef128.go, so generated signatures are the API's own types. wideW
//     is the double-width product: "type wide64 = Uint128" and
//     "type wide128 = Uint256", where Uint256 is {Hi, Lo Uint128} and
//     belongs to T1.3.1. Bit patterns and coefficients share coefW, as
//     uint64 and Uint128 already do in the API.
//  4. Helpers. The width-specific arithmetic is a fixed vocabulary of
//     functions over coefW and wideW, defined with identical signatures in
//     coef64.go and coef128.go (one file per width, nothing else in them):
//     cIsZeroW, cCmpW, cAddW, cSubW, cOddW, cDigitsW, cMulPow10W(c, k),
//     cDivPow10W(c, k) (q, r coefW), cGtMaxW,
//     cPow10W(k), cMulFullW(a, b) wideW, cReduceW(p wideW, k) (q coefW,
//     sticky bool) and cLenFullW(p wideW) int, with the decimal-digit
//     primitives cShiftW, cShiftFullW, cReduceClassW, cScaleW and cScaleFullW
//     (digits.go says what a caller may assume of them) and the rounding
//     routine's cLoW(p wideW) coefW, cIncW, cMaxW() and cInfW(neg), the
//     division's cMulPow10FullW(c, k) wideW (a coefficient scaled into the
//     double width), cDivFullW(n wideW, d coefW) (q, r coefW) (the 128-by-64
//     or 256-by-128 division whose quotient fits a coefficient) and cNaNW()
//     (the canonical quiet NaN), the remainder's cModFullW, the character
//     conversions' cSplitW(c) (hi, lo uint64) and cWideW(m Uint128) wideW,
//     and FMA's cAddFullW, cSubFullW and cIncFullW, extended as the other
//     operations need. Every coef64.go helper must inline;
//     check with -gcflags=-m=2 after changing one (no "cannot inline
//     c…64"; plain -m does not print "cannot inline" lines).
//     coef128.go is where T1.3.1's 128- and 256-bit arithmetic goes; it
//     may call unexported Uint128 and Uint256 functions freely.
//  5. The generated functions are unexported (add64, add128, round64,
//     round128, ...). The exported, documented functions in arith.go,
//     round.go and the other API files stay where they are and become
//     one-line wrappers, which the inliner removes; the API files do not
//     change shape as operations are implemented.
//  6. Width constants are compile-time constants in each instantiation
//     (PrecisionW becomes Precision64), never fields of a format value
//     passed at run time, and the package has no generic code. The two
//     rejected forms are not to be reintroduced piecemeal.
//  7. Width-independent code (the rounding-direction decision, the digit
//     estimate from a bit length, the Mode and Status types) is ordinary Go
//     in ordinary files. The template holds only what mentions a
//     coefficient, a product or a format constant.
//  8. The numbers above are the ratchet. A decimal64 operation whose
//     generated form falls more than noise behind the hand-written row on
//     the same machine has a helper that stopped inlining or an algorithm
//     that grew a width-independent cost; fix that, do not special-case the
//     width.
//
// # Dependencies
//
// The files that cmd/dist compiles for the compiler bootstrap import only
// math/bits. Conversion from decimal to binary floating point lives in
// float.go, which is excluded from the bootstrap build, because it shares
// internal/strconv's exact machinery, which the minimum bootstrap toolchain
// does not provide, and because the compiler never needs it: constants are
// exact and are encoded through [Round64] and [Round128]. Conversion from
// binary to decimal (fromfloat.go and its generated files) needs only
// math/bits and is compiled everywhere.
package bid
