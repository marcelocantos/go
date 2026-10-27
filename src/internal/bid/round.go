// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// Mode is a rounding-direction attribute (IEEE 754-2008 clause 4.3). The
// five values are the five attributes of the standard; a Mode outside them
// behaves as ToNearestEven. The language operators use ToNearestEven only.
// See "Rounding modes" in the package documentation.
type Mode uint8

const (
	ToNearestEven  Mode = iota // roundTiesToEven (clause 4.3.1)
	ToNearestAway              // roundTiesToAway (clause 4.3.1)
	TowardZero                 // roundTowardZero (clause 4.3.2)
	TowardPositive             // roundTowardPositive (clause 4.3.2)
	TowardNegative             // roundTowardNegative (clause 4.3.2)
)

// Status reports the conditions met by a conversion: from a character
// sequence (Parse64, Parse128), to an integer (Int64From64 and its
// relatives), or from exact parts (Round64, Round128). It is a bit set; the
// zero value Exact means the result is the exact value. Arithmetic
// operations do not return a Status: they return the IEEE default result
// for every exception (clause 7) and nothing else. The conditions and
// their default results are defined in "Conditions" in the package
// documentation.
//
// Callers map a Status to their own error type. For example, a parser
// reports Syntax as a syntax error and Overflow as a range error, as
// strconv.ParseFloat does, and ignores Inexact and Underflow.
type Status uint8

// Exact is the zero Status: the result is the exact value.
const Exact Status = 0

const (
	// Inexact is set when the result differs from the exact value
	// (clause 7.6). It accompanies every Overflow and Underflow.
	Inexact Status = 1 << iota

	// Overflow is set when the rounded value's magnitude is too large for
	// the destination (clause 7.4). A decimal result is then the infinity
	// or the largest finite value that the mode selects; an integer result
	// is the nearest bound of the integer type.
	Overflow

	// Underflow is set when a non-zero exact value is below the smallest
	// normal magnitude, judged before rounding, and the result is inexact
	// (clause 7.5, default handling).
	Underflow

	// Invalid is set when the operation has no numeric result (clause
	// 7.2): a NaN or an infinity converted to an integer. The integer
	// result is zero for a NaN and the bound of the infinity's sign.
	Invalid

	// Syntax is set when a character sequence is not a decimal numeral or
	// a special value in the accepted grammar; the decimal result is the
	// canonical quiet NaN.
	Syntax
)

// Round64 is the rounding routine of the package, exposed for callers that
// hold an exact value in parts: the compiler encoding a constant whose
// exact value it has reduced, and the string parser. Every operation of
// the package reaches an encoding through this routine.
//
// The input describes the exact value v = (-1)^neg × (coeff + f) × 10^exp,
// where f is zero when sticky is false and an unspecified fraction with
// 0 < f < 1 when sticky is true. The coefficient may be any uint64 and the
// exponent any int. A caller holding more digits than fit keeps them in
// coeff as far as it can and folds the rest into sticky; a caller with an
// inexact value must keep at least one digit beyond the destination
// precision (a round digit) in coeff, because sticky records only that f
// is non-zero, not its size.
//
// The result is v rounded to decimal64 in the given mode, with the
// preferred exponent exp, under the rules of the package documentation
// ("Rounding from exact parts" and the sections it refers to), and the
// Status reports Inexact, Overflow and Underflow as they occur.
func Round64(neg bool, coeff uint64, exp int, sticky bool, mode Mode) (uint64, Status) {
	panic(notImplemented)
}

// Round128 is Round64 for decimal128. The coefficient may be any Uint128.
func Round128(neg bool, coeff Uint128, exp int, sticky bool, mode Mode) (Uint128, Status) {
	panic(notImplemented)
}
