// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

// Package decimal provides functions for the decimal floating-point types
// decimal64 and decimal128, the counterparts of package math's functions for
// float64, and the operations IEEE 754 defines for decimal arithmetic that
// the operators don't provide: quantizing, reducing, rounding with an
// explicit mode, the quantum of a value, and reading and writing the payload
// of a NaN.
//
// For fixed-scale arithmetic, such as money in cents, it adds what IEEE 754
// leaves out: the AwayFromZero rounding mode, division rounded once to a
// quantum ([QuoQuantize]), and taking a value apart into its coefficient and
// exponent and putting it back together ([Decompose] and [Compose]).
//
// It also provides the square root, the powers, the exponential and the
// logarithms. Those are correctly rounded: each returns the value of its
// type nearest to the exact result, with ties to even. They are slower
// than the arithmetic, hundreds of nanoseconds to a few microseconds a
// call, and they do not allocate, except for the rare result so close to
// a rounding boundary that it takes many times the usual precision to
// tell which side it is on.
//
// Each function is generic over [Decimal] and computes in the format of its
// type argument, so a decimal64 result is rounded once, as a decimal64, not
// computed in decimal128 and narrowed. The arithmetic is internal/bid's,
// which also does the arithmetic of the operators.
package decimal

import impl "internal/decimal"

// Decimal is the set of decimal floating-point types.
type Decimal interface {
	~decimal64 | ~decimal128
}

// Decimal limit values, as [math.MaxFloat64] and [math.SmallestNonzeroFloat64]
// are for the floats. Max is the largest finite value of the type and
// SmallestNonzero the least positive one, a subnormal.
const (
	MaxDecimal64             = 9.999999999999999e384
	SmallestNonzeroDecimal64 = 1e-398

	MaxDecimal128             = 9.999999999999999999999999999999999e6144
	SmallestNonzeroDecimal128 = 1e-6176
)

// A RoundingMode determines how a result that is not exactly representable
// is rounded. The operators always round to nearest, ties to even. The
// modes, their names and their values are those of math/big's
// RoundingMode.
type RoundingMode byte

// The rounding modes: the five of IEEE 754-2019, and AwayFromZero, which
// rounds every inexact magnitude up ("round up to the next cent").
const (
	ToNearestEven RoundingMode = iota // to nearest, ties to even
	ToNearestAway                     // to nearest, ties away from zero
	ToZero                            // toward zero
	AwayFromZero                      // away from zero
	ToNegativeInf                     // toward -Inf
	ToPositiveInf                     // toward +Inf
)

// String returns the name of the mode, as math/big's RoundingMode does.
func (m RoundingMode) String() string {
	switch m {
	case ToNearestEven:
		return "ToNearestEven"
	case ToNearestAway:
		return "ToNearestAway"
	case ToZero:
		return "ToZero"
	case AwayFromZero:
		return "AwayFromZero"
	case ToNegativeInf:
		return "ToNegativeInf"
	case ToPositiveInf:
		return "ToPositiveInf"
	}
	return "RoundingMode(" + itoa(int(m)) + ")"
}

// itoa formats a small non-negative integer; the package does not import strconv.
func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// NaN returns an IEEE 754 "not-a-number" value of type D.
func NaN[D Decimal]() D { return impl.NaN[D]() }

// Inf returns positive infinity if sign >= 0, negative infinity if sign < 0.
func Inf[D Decimal](sign int) D { return impl.Inf[D](sign) }

// IsNaN reports whether x is an IEEE 754 "not-a-number" value.
func IsNaN[D Decimal](x D) bool { return impl.IsNaN[D](x) }

// IsInf reports whether x is an infinity, according to sign.
// If sign > 0, IsInf reports whether x is positive infinity.
// If sign < 0, IsInf reports whether x is negative infinity.
// If sign == 0, IsInf reports whether x is either infinity.
func IsInf[D Decimal](x D, sign int) bool { return impl.IsInf[D](x, sign) }

// Payload returns the payload of the NaN x (the getPayload operation of
// IEEE 754-2019 clause 9.7): the non-negative integer it carries, as a value
// of type D with exponent 0. The sign of x and whether it is signaling do
// not matter. A NaN's payload survives the arithmetic that propagates the
// NaN, and widening or narrowing keeps its value when the narrower format
// can hold it; otherwise it becomes 0.
//
// Payload returns -1 if x is not a NaN, which no payload is.
func Payload[D Decimal](x D) D { return impl.Payload[D](x) }

// NaNWithPayload returns a quiet NaN of type D whose payload is payload (the
// setPayload operation of IEEE 754-2019 clause 9.7). The payload must be an
// integer from 0 to 10^15-1 for decimal64, or to 10^33-1 for decimal128; its
// exponent does not matter, so 42, 42.0 and 4.2E+1 all give payload 42.
// Payload(NaNWithPayload(p)) == p for every such p.
//
// NaNWithPayload returns +0 if payload is not such an integer: a fraction,
// a negative number, an integer out of range, an infinity or a NaN.
func NaNWithPayload[D Decimal](payload D) D { return impl.NaNWithPayload[D](payload) }

// SignalingNaNWithPayload is NaNWithPayload, but returns a signaling NaN (the
// setPayloadSignaling operation of IEEE 754-2019 clause 9.7). Arithmetic on
// a signaling NaN returns it made quiet, with the same payload.
func SignalingNaNWithPayload[D Decimal](payload D) D { return impl.SignalingNaNWithPayload[D](payload) }

// Signbit reports whether x is negative or negative zero.
func Signbit[D Decimal](x D) bool { return impl.Signbit[D](x) }

// TotalOrder reports whether x precedes or is y in the total order of IEEE
// 754-2019 clause 5.10 (the totalOrder operation). The order is defined on
// every pair of values, NaNs included, so it can sort them:
//
//   - numerically, x < y puts x first;
//   - -0 precedes +0;
//   - members of one cohort, equal values with different exponents, are
//     ordered by exponent, the smaller first for a positive value and the
//     larger first for a negative one, so -1.0 precedes -1.00, which
//     precedes 1.00, which precedes 1.0;
//   - a NaN with a negative sign precedes every number and one with a
//     positive sign follows every number;
//   - between two NaNs, a negative sign comes first, then, for a positive
//     sign, a signaling NaN precedes a quiet one, and the reverse for a
//     negative sign; NaNs of one sign and kind are ordered by payload,
//     ascending for a positive sign and descending for a negative one.
//
// Where decimal128 is stored as UBD, on every target but ppc64le,
// TotalOrder of two different decimal128 values is never true both ways.
// For decimal64, and for decimal128 on ppc64le, NaNs are so far ordered by
// sign alone, so two NaNs of one sign precede each other.
func TotalOrder[D Decimal](x, y D) bool { return impl.TotalOrder[D](x, y) }

// TotalCompare is [TotalOrder] as a three-way comparison, for
// [slices.SortFunc] and the like: -1, 0 or +1 as x precedes, is, or follows
// y. Where decimal128 is stored as UBD, it is 0 for two decimal128 values
// only when they are the same value with the same exponent, sign and, for
// a NaN, kind and payload.
func TotalCompare[D Decimal](x, y D) int { return impl.TotalCompare[D](x, y) }

// TotalOrderMag reports whether the magnitude of x precedes or is the
// magnitude of y in the total order: TotalOrder(Abs(x), Abs(y)) (the
// totalOrderMag operation of IEEE 754-2019 clause 5.7.2).
func TotalOrderMag[D Decimal](x, y D) bool { return impl.TotalOrderMag[D](x, y) }

// Abs returns the absolute value of x, keeping its quantum.
//
// Special cases are:
//
//	Abs(±Inf) = +Inf
//	Abs(NaN) = NaN
func Abs[D Decimal](x D) D { return impl.Abs[D](x) }

// Copysign returns a value with the magnitude and quantum of f and the sign
// of sign.
func Copysign[D Decimal](f, sign D) D { return impl.Copysign[D](f, sign) }

// Dim returns the maximum of x-y or 0.
//
// Special cases are:
//
//	Dim(+Inf, +Inf) = NaN
//	Dim(-Inf, -Inf) = NaN
//	Dim(x, NaN) = Dim(NaN, x) = NaN
func Dim[D Decimal](x, y D) D { return impl.Dim[D](x, y) }

// Trunc returns the integer value of x, rounding toward zero.
func Trunc[D Decimal](x D) D { return impl.Trunc[D](x) }

// Floor returns the greatest integer value less than or equal to x.
func Floor[D Decimal](x D) D { return impl.Floor[D](x) }

// Ceil returns the least integer value greater than or equal to x.
func Ceil[D Decimal](x D) D { return impl.Ceil[D](x) }

// Round returns the nearest integer, rounding half away from zero.
func Round[D Decimal](x D) D { return impl.Round[D](x) }

// RoundToEven returns the nearest integer, rounding ties to even.
func RoundToEven[D Decimal](x D) D { return impl.RoundToEven[D](x) }

// Mod returns the floating-point remainder of x/y. The magnitude of the
// result is less than y and its sign agrees with that of x. The result is
// exact whatever the magnitude of the quotient.
//
// Special cases are:
//
//	Mod(±Inf, y) = NaN
//	Mod(NaN, y) = NaN
//	Mod(x, 0) = NaN
//	Mod(x, ±Inf) = x
//	Mod(x, NaN) = NaN
func Mod[D Decimal](x, y D) D { return impl.Mod[D](x, y) }

// Remainder returns the IEEE 754 floating-point remainder of x/y. The result
// is exact whatever the magnitude of the quotient.
//
// Special cases are:
//
//	Remainder(±Inf, y) = NaN
//	Remainder(NaN, y) = NaN
//	Remainder(x, 0) = NaN
//	Remainder(x, ±Inf) = x
//	Remainder(x, NaN) = NaN
func Remainder[D Decimal](x, y D) D { return impl.Remainder[D](x, y) }

// FMA returns x * y + z, computed with only one rounding.
func FMA[D Decimal](x, y, z D) D { return impl.FMA[D](x, y, z) }

// Quantize returns x rounded to the quantum of y, to nearest with ties to
// even: the result has the exponent of y and the value closest to x with that
// exponent. Quantize(x, 0.01) rounds x to the cent.
//
// Special cases are:
//
//	Quantize(NaN, y) = NaN
//	Quantize(x, NaN) = NaN
//	Quantize(±Inf, ±Inf) = ±Inf, with the sign of x
//	Quantize(±Inf, y) = NaN, for finite y
//	Quantize(x, ±Inf) = NaN, for finite x
//	Quantize(x, y) = NaN, if the result does not fit the format
func Quantize[D Decimal](x, y D) D { return impl.Quantize[D](x, y) }

// QuantizeMode is Quantize with the rounding mode given by mode: for example,
// QuantizeMode(x, 0.01, ToNearestAway) rounds half a cent up in magnitude.
func QuantizeMode[D Decimal](x, y D, mode RoundingMode) D {
	return impl.QuantizeMode[D](x, y, impl.RoundingMode(mode))
}

// QuoQuantize returns x / y rounded once, in the given mode, to the quantum
// of q: the result has the exponent of q and the value closest to the exact
// quotient with that exponent, in that mode. QuoQuantize(x, 12, 0.01,
// ToNearestAway) divides by twelve and rounds half a cent up in magnitude.
//
// It differs from QuantizeMode(x/y, q, mode) by rounding once. The operator
// rounds the quotient to the precision of D first, and that rounding can
// move it onto the half the second rounding decides: in decimal64,
// 2.009999999999999 / 2 is 1.0049999999999995, which the operator rounds to
// 1.005000000000000 and QuantizeMode then rounds half up to 1.01, while
// QuoQuantize rounds the exact quotient to 1.00. With q = 1 and mode ToZero
// it is integer division: QuoQuantize(17, 5, 1, ToZero) is 3.
//
// The sign and coefficient of q are ignored. Special cases are those of
// dividing and then quantizing:
//
//	QuoQuantize(x, y, q, mode) = NaN, if the result does not fit the format
//	QuoQuantize(±Inf, ±Inf, q, mode) = NaN
//	QuoQuantize(0, 0, q, mode) = NaN
//	QuoQuantize(x, 0, q, mode) = NaN, for finite x != 0 and finite q
//	QuoQuantize(x, 0, ±Inf, mode) = ±Inf, for finite x != 0
//	QuoQuantize(±Inf, y, ±Inf, mode) = ±Inf, for finite y
//	QuoQuantize(±Inf, y, q, mode) = NaN, for finite y and finite q
//	QuoQuantize(x, ±Inf, q, mode) = ±0 with the exponent of q, for finite x and finite q
//	QuoQuantize(x, y, ±Inf, mode) = NaN, for a finite quotient
//	QuoQuantize(x, y, q, mode) = NaN, if x, y or q is a NaN
func QuoQuantize[D Decimal](x, y, q D, mode RoundingMode) D {
	return impl.QuoQuantize[D](x, y, q, impl.RoundingMode(mode))
}

// Decompose returns the sign, coefficient and exponent of a finite x, so
// that x = (-1)^neg × (hi × 2^64 + lo) × 10^exp, and ok true. The
// coefficient has at most 16 digits for a decimal64, whose hi is always
// zero, and at most 34 for a decimal128. The exponent is that of x's last
// digit, so Decompose(1.50) is 150 and -2, the parts of a SQL NUMERIC, an
// Arrow or BSON decimal, or a coefficient and scale in another library. A
// zero keeps its sign and exponent. For an infinity or a NaN, Decompose
// returns zeros and ok false.
func Decompose[D Decimal](x D) (neg bool, hi, lo uint64, exp int, ok bool) {
	return impl.Decompose[D](x)
}

// Compose returns the value of type D nearest to (-1)^neg × (hi × 2^64 + lo)
// × 10^exp, with ties to even, as parsing the same digits does: a
// coefficient with more digits than D holds is rounded once, a value too
// large is an infinity and one too small rounds to a subnormal or zero. An
// exact value keeps exp as its exponent, so Compose of the parts that
// Decompose returns gives back every finite canonical x of type D.
func Compose[D Decimal](neg bool, hi, lo uint64, exp int) D {
	return impl.Compose[D](neg, hi, lo, exp)
}

// SameQuantum reports whether x and y have the same quantum, the exponent of
// their last digit. Two NaNs have the same quantum, and so do two infinities.
func SameQuantum[D Decimal](x, y D) bool { return impl.SameQuantum[D](x, y) }

// Quantum returns the quantum of x, the value of one unit in its last digit
// (the quantum operation of IEEE 754-2019 clause 5.3.2): 1 × 10^e, where e is
// the exponent of x, as a positive value of type D with exponent e.
// Quantum(1.50) is 0.01, Quantum(-5E+3) is 1E+3 and Quantum(0.00) is 0.01.
// Quantize(x, Quantum(y)) is Quantize(x, y) for every finite y.
//
// Special cases are:
//
//	Quantum(±Inf) = +Inf
//	Quantum(NaN) = NaN, with the payload of the NaN operand
func Quantum[D Decimal](x D) D { return impl.Quantum[D](x) }

// Reduce returns x with its trailing zeros removed: the value of x with the
// largest exponent at which it is representable. Reduce(1.50) is 1.5, and
// Reduce of any zero is a zero with exponent 0 and the sign of x.
func Reduce[D Decimal](x D) D { return impl.Reduce[D](x) }

// Sqrt returns the square root of x, correctly rounded. An exact root
// has the exponent nearest to half that of x: Sqrt(2.25) is 1.5 and
// Sqrt(4.00) is 2.0.
//
// Special cases are:
//
//	Sqrt(+Inf) = +Inf
//	Sqrt(±0) = ±0
//	Sqrt(x < 0) = NaN
//	Sqrt(NaN) = NaN
func Sqrt[D Decimal](x D) D { return impl.Sqrt[D](x) }

// Exp returns e**x, the base-e exponential of x, correctly rounded.
//
// Special cases are:
//
//	Exp(+Inf) = +Inf
//	Exp(-Inf) = 0
//	Exp(±0) = 1
//	Exp(NaN) = NaN
//
// Very large values overflow to +Inf and very small values underflow to
// 0.
func Exp[D Decimal](x D) D { return impl.Exp[D](x) }

// Log returns the natural logarithm of x, correctly rounded.
//
// Special cases are:
//
//	Log(+Inf) = +Inf
//	Log(1) = 0
//	Log(±0) = -Inf
//	Log(x < 0) = NaN
//	Log(NaN) = NaN
func Log[D Decimal](x D) D { return impl.Log[D](x) }

// Log10 returns the decimal logarithm of x, correctly rounded. The
// logarithm of a power of ten is exact: Log10(1000) is 3 and Log10(0.01)
// is -2. The special cases are the same as for [Log].
func Log10[D Decimal](x D) D { return impl.Log10[D](x) }

// Pow returns x**y, the base-x exponential of y, correctly rounded. A
// result that is exact has the exponent nearest to y times that of x, as
// repeated multiplication would give it: Pow(1.5, 2) is 2.25 and
// Pow(2.25, 0.5) is 1.5.
//
// Special cases are (in order):
//
//	Pow(x, ±0) = 1 for any x
//	Pow(1, y) = 1 for any y
//	Pow(x, NaN) = NaN
//	Pow(NaN, y) = NaN
//	Pow(±0, y) = ±Inf for y an odd integer < 0
//	Pow(±0, -Inf) = +Inf
//	Pow(±0, +Inf) = +0
//	Pow(±0, y) = +Inf for finite y < 0 and not an odd integer
//	Pow(±0, y) = ±0 for y an odd integer > 0
//	Pow(±0, y) = +0 for finite y > 0 and not an odd integer
//	Pow(-1, ±Inf) = 1
//	Pow(x, +Inf) = +Inf for |x| > 1
//	Pow(x, -Inf) = +0 for |x| > 1
//	Pow(x, +Inf) = +0 for |x| < 1
//	Pow(x, -Inf) = +Inf for |x| < 1
//	Pow(+Inf, y) = +Inf for y > 0
//	Pow(+Inf, y) = +0 for y < 0
//	Pow(-Inf, y) = Pow(-0, -y)
//	Pow(x, y) = NaN for finite x < 0 and finite non-integer y
func Pow[D Decimal](x, y D) D { return impl.Pow[D](x, y) }

// Pown returns x**n for an integer n, correctly rounded (the pown
// operation of IEEE 754-2019 clause 9.2). The exponent of an exact result
// is the nearest to n times that of x: Pown(1.5, 2) is 2.25 and
// Pown(2, -2) is 0.25.
//
// Special cases are:
//
//	Pown(x, 0) = 1 for any x, even a NaN
//	Pown(±0, n) = ±Inf for odd n < 0
//	Pown(±0, n) = +Inf for even n < 0
//	Pown(±0, n) = ±0 for odd n > 0
//	Pown(±0, n) = +0 for even n > 0
//	Pown(±Inf, n) = Pown(±0, -n)
//	Pown(NaN, n) = NaN
func Pown[D Decimal](x D, n int) D { return impl.Pown[D](x, n) }

// Compound returns (1+x)**n, correctly rounded (the compound operation
// of IEEE 754-2019 clause 9.2): the growth of one unit at the rate x
// over n periods. It is exact about the sum 1+x, which Pown(1+x, n) is
// not when x is a small rate whose digits the sum would lose.
//
// Special cases are:
//
//	Compound(x, 0) = 1 for x >= -1 or a NaN
//	Compound(-1, n) = +Inf for n < 0
//	Compound(-1, n) = +0 for n > 0
//	Compound(±0, n) = 1
//	Compound(+Inf, n) = +Inf for n > 0
//	Compound(+Inf, n) = +0 for n < 0
//	Compound(x, n) = NaN for x < -1
//	Compound(NaN, n) = NaN for n != 0
func Compound[D Decimal](x D, n int) D { return impl.Compound[D](x, n) }

// Rootn returns the nth root of x, correctly rounded (the rootn
// operation of IEEE 754-2019 clause 9.2): Rootn(x, 12) is the monthly
// growth factor of a yearly one. The root of a negative x is negative
// for an odd n.
//
// Special cases are:
//
//	Rootn(x, 0) = NaN
//	Rootn(±0, n) = ±Inf for odd n < 0
//	Rootn(±0, n) = +Inf for even n < 0
//	Rootn(±0, n) = ±0 for odd n > 0
//	Rootn(±0, n) = +0 for even n > 0
//	Rootn(+Inf, n) = +Inf for n > 0
//	Rootn(+Inf, n) = +0 for n < 0
//	Rootn(-Inf, n) = -Inf for odd n > 0
//	Rootn(-Inf, n) = -0 for odd n < 0
//	Rootn(x, n) = NaN for x < 0 and even n
//	Rootn(NaN, n) = NaN
func Rootn[D Decimal](x D, n int) D { return impl.Rootn[D](x, n) }

// Ldexp returns frac × 10**exp, the inverse of [Logb] as [math.Ldexp] is
// of math.Logb, with the radix of the format, ten (the scaleB operation of
// IEEE 754-2019 clause 5.3.3). It moves the decimal point and changes no
// digit, unless the result is beyond the format's range: Ldexp(1.50, 2) is
// 150 and Ldexp(150, -2) is 1.50.
//
// Special cases are:
//
//	Ldexp(±0, exp) = ±0, with its exponent moved
//	Ldexp(±Inf, exp) = ±Inf
//	Ldexp(NaN, exp) = NaN
func Ldexp[D Decimal](frac D, exp int) D { return impl.Ldexp[D](frac, exp) }

// Logb returns the decimal exponent of x as an integer value of type D, as
// [math.Logb] returns the binary exponent of a float64 (the logB operation
// of IEEE 754-2019 clause 5.3.3): the exponent of x written with one digit
// before the point, so Logb(250) is 2 and Logb(0.025) is -2.
//
// Special cases are:
//
//	Logb(±Inf) = +Inf
//	Logb(0) = -Inf
//	Logb(NaN) = NaN
func Logb[D Decimal](x D) D { return impl.Logb[D](x) }

// Ilogb returns the decimal exponent of x as an integer, as [math.Ilogb]
// returns the binary exponent of a float64.
//
// Special cases are:
//
//	Ilogb(±Inf) = MaxInt32
//	Ilogb(0) = MinInt32
//	Ilogb(NaN) = MaxInt32
func Ilogb[D Decimal](x D) int { return impl.Ilogb[D](x) }

// Nextafter returns the next representable value of type D after x towards
// y, as [math.Nextafter] does for float64. IEEE 754-2019's nextUp(x)
// (clause 5.3.1) is Nextafter(x, Inf[D](1)), and its nextDown(x) is
// Nextafter(x, Inf[D](-1)). The next value after ±0 is the least subnormal
// of the sign of the direction, and a result has the least exponent that
// holds it.
//
// Special cases are:
//
//	Nextafter(x, x) = x, also for x and y of one cohort, such as 1.0 and 1.00
//	Nextafter(NaN, y) = NaN
//	Nextafter(x, NaN) = NaN
func Nextafter[D Decimal](x, y D) D { return impl.Nextafter[D](x, y) }
