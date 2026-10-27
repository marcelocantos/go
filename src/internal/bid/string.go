// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// Conversion between decimal values and character sequences
// (IEEE 754-2008 clause 5.4.2 and 5.12, convertFromDecimalCharacter and
// convertToDecimalCharacter). The character form is the to-scientific-string
// form of the General Decimal Arithmetic specification, which the standard
// adopts: it preserves the quantum, so a string converts back to the same
// bit pattern.

// MaxLen64 and MaxLen128 bound the number of bytes Append64 and Append128
// write. The longest forms are the plain notation of a full-precision
// coefficient with the smallest adjusted exponent that avoids exponential
// notation, "-0.00000ddd…d" (a sign, "0.", five zeros and Precision
// digits), and the exponential notation "-d.ddd…dE-dddd" (a sign,
// Precision digits, a point, "E", a sign and the exponent digits), which
// is never longer.
const (
	MaxLen64  = 1 + len("0.") + 5 + Precision64
	MaxLen128 = 1 + len("0.") + 5 + Precision128
)

// Append64 appends the character form of x to dst and returns the extended
// buffer (IEEE 754-2008 clause 5.12, convertToDecimalCharacter). It does
// not allocate when dst has MaxLen64 bytes of spare capacity.
//
// A finite value is written with all its coefficient digits and its
// exponent preserved, in plain notation when the exponent is at most zero
// and the adjusted exponent is at least -6, and otherwise in exponential
// notation with an explicitly signed exponent: 1.50, 1.5E+3, 0.00, -0,
// 1E-7. Infinities are "Infinity" and "-Infinity"; every NaN is "NaN",
// without sign or payload. The output is exactly what Parse64 reads back
// to the canonical encoding of x.
func Append64(dst []byte, x uint64) []byte {
	panic(notImplemented)
}

// Append128 is Append64 for decimal128; it needs MaxLen128 bytes of spare
// capacity to avoid allocating.
func Append128(dst []byte, x Uint128) []byte {
	panic(notImplemented)
}

// Parse64 converts the character sequence s to decimal64, rounded once in
// mode from the exact value s denotes (IEEE 754-2008 clause 5.4.2 and
// 5.12, convertFromDecimalCharacter).
//
// The accepted grammar is that of clause 5.12: an optional sign, then
// either a decimal numeral (digits with an optional point among them, at
// least one digit, followed by an optional exponent part "e" or "E", an
// optional sign and at least one digit) or one of the words "inf",
// "infinity", "nan" and "snan", all case-insensitive. Nothing else is
// accepted: no surrounding space, no underscores, no hexadecimal forms, no
// NaN payload, and no characters after a special word. A signaling NaN
// word gives the canonical quiet NaN. Callers that accept Go literal
// syntax normalize it before calling.
//
// The numeral may have any number of digits and any exponent; the value is
// rounded once. Status reports Syntax (the result is then NaN64), and
// otherwise Inexact, Overflow and Underflow as Round64 reports them. The
// exponent of an exact result is that of the numeral: "0.00" gives a zero
// with exponent -2, "1.50" has exponent -2, "1.5E+3" has exponent 2.
func Parse64(s string, mode Mode) (uint64, Status) {
	panic(notImplemented)
}

// Parse128 is Parse64 for decimal128.
func Parse128(s string, mode Mode) (Uint128, Status) {
	panic(notImplemented)
}
