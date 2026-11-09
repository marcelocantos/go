// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zstring64.go and zstring128.go, written in
// the vocabulary of "Width parameterisation" in the package documentation:
// every identifier that depends on the width ends in a capital W, which the
// generator (gen_arith.go) replaces with 64 or 128. This file is not
// compiled. Everything from the build constraint to the package clause is
// commentary on the template and is dropped from the generated files, which
// nobody edits.
//
// It holds what the conversions between decimal values and character
// sequences need of a coefficient or an encoding; the character form
// itself, its grammar and its notation, is width-independent and lives in
// string.go.
package bid

// UnpackDigitsW decodes x and writes its coefficient digits to the end of buf,
// without a sign, decimal point or exponent. It returns the same kind, sign and exponent as
// UnpackW, and the number of bytes written. Zero and non-finite values write
// no digits and return n = 0. Nonzero finite values have no leading zeros.
// The buffer must be non-nil; bytes before the last n are unchanged.
// It does not allocate.
func UnpackDigitsW(buf *[PrecisionW]byte, x coefW) (kind Kind, neg bool, n, exp int) {
	// The common word coefficient needs neither a general decoder call nor
	// a 128-bit split. The word guard proves a finite canonical encoding;
	// the base-10 word bound is exactly the digit writer's precondition.
	if sign, word, biased, ok := unpackWordW(x); ok && word < pow10tab64[u64Digits] {
		if word != 0 {
			n = writeWordDigits(buf[:], word)
		}
		return KindFinite, sign, n, biased - BiasW
	}
	// A word-guard miss may be non-finite. Decode it directly rather than
	// paying both the guard and a general decoder call for a digitless value.
	if !IsFiniteW(x) {
		kind = KindInf
		if IsNaNW(x) {
			kind = KindNaN
		}
		return kind, SignbitW(x), 0, 0
	}
	var c coefW
	kind, neg, c, exp = UnpackW(x)
	// The non-finite cases returned above; only a zero coefficient is empty.
	if cIsZeroW(c) {
		return
	}
	hi, lo := cSplitW(c)
	if hi == 0 {
		n = writeWordDigits(buf[:], lo)
	} else {
		n = writeDigits(buf[:], hi, lo)
	}
	return
}

// appendW is Append64 and Append128: it appends the to-scientific-string
// form of x to dst. A NaN of any kind is "NaN" and an infinity "Infinity"
// with its sign; a finite value is its coefficient digits, rendered into a
// local buffer, and its exponent, which appendDecimal places. UnpackW reads
// a non-canonical encoding as the zero it denotes, so every bit pattern has
// a form.
func appendW(dst []byte, x coefW) []byte {
	kind, neg, c, exp := UnpackW(x)
	switch kind {
	case KindNaN:
		return append(dst, "NaN"...)
	case KindInf:
		if neg {
			dst = append(dst, '-')
		}
		return append(dst, "Infinity"...)
	}
	var digs [digitBufLen]byte
	hi, lo := cSplitW(c)
	var n int
	if hi == 0 {
		n = writeWordDigits(digs[:], lo)
	} else {
		n = writeDigits(digs[:], hi, lo)
	}
	return appendDecimal(dst, neg, digs[len(digs)-n:], exp)
}

// parseW is Parse64 and Parse128. The string is scanned once, keeping the
// first PrecisionW+1 significant digits; the digits beyond them become a
// sticky bit and a larger exponent, so that scanDecimal's description of
// the numeral is exact. One digit beyond the precision is kept so that
// roundW, which discards at least that one digit of the coefficient, sees
// the digit that decides the rounding and the sticky bit stands only for
// what lies below it (the caller contract in roundW's comment). The result
// is rounded once, by roundW, from that exact description.
func parseW(s string, mode Mode) (coefW, Status) {
	n := scanDecimal(s, PrecisionW+1)
	switch n.kind {
	case scanSyntax:
		return cNaNW(), Syntax
	case scanNaN:
		return cNaNW(), Exact
	case scanInf:
		return cInfW(n.neg), Exact
	}
	class := classZero
	if n.sticky {
		class = classBelowHalf
	}
	return roundW(n.neg, cWideW(n.mant), n.exp, class, mode, MinExpW)
}
