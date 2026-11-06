// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zpayload64.go and zpayload128.go: the
// payload operations of IEEE 754-2019 clause 9.7, getPayload, setPayload and
// setPayloadSignaling ("NaN results" in the package documentation). It is
// written in the vocabulary of "Width parameterisation" (see arith_tmpl.go
// for the rules). A payload is an integer value, as everywhere in this
// package: the integer in a NaN's trailing significand field, canonical when
// it is below 10^(PrecisionW-1). Every finite result is exact and goes
// through roundW, which encodes it at the exponent it is given.
package bid

// payloadW returns the payload of the NaN x as a finite value: the positive
// integer the payload is, with exponent 0, or +0 when the payload is not
// canonical and so reads as zero. The signaling bit and the sign of x do not
// matter. For an x that is not a NaN it returns -1, with exponent 0.
func payloadW(x coefW) coefW {
	kind, _, _, _ := UnpackW(x)
	if kind != KindNaN {
		r, _ := roundW(true, wideW{Lo: cPow10W(0)}, 0, classZero, ToNearestEven, MinExpW)
		return r
	}
	r, _ := roundW(false, wideW{Lo: cPayloadW(x)}, 0, classZero, ToNearestEven, MinExpW)
	return r
}

// nanWithPayloadW returns the positive NaN, signaling when signaling is set
// and quiet otherwise, whose payload is the value of p, when p is a canonical
// payload: an integer in [0, 10^(PrecisionW-1)), whatever its exponent, so
// that 5E+1, 50 and 50.0 all give payload 50, and a zero of either sign gives
// payload 0. For any other p, a non-integer, a negative non-zero value, an
// integer of 10^(PrecisionW-1) or more, an infinity or a NaN, it returns +0
// with exponent 0. A non-canonical finite encoding reads as a zero and gives
// payload 0.
func nanWithPayloadW(p coefW, signaling bool) coefW {
	kind, neg, c, exp := UnpackW(p)
	var zero coefW
	plusZero, _ := roundW(false, wideW{Lo: zero}, 0, classZero, ToNearestEven, MinExpW)
	if kind != KindFinite {
		return plusZero
	}
	if cIsZeroW(c) {
		return cNaNPayloadW(zero, signaling)
	}
	if neg {
		return plusZero
	}
	switch {
	case exp > 0:
		// c × 10^exp is below 10^(PrecisionW-1) exactly when it has at most
		// PrecisionW-1 digits.
		if cDigitsW(c)+exp > PrecisionW-1 {
			return plusZero
		}
		c = cMulPow10W(c, exp)
	case exp < 0:
		// The value is an integer exactly when the -exp low digits of c are
		// zeros. A non-zero c has at most PrecisionW digits and is not a
		// multiple of 10^PrecisionW.
		if -exp >= PrecisionW {
			return plusZero
		}
		q, r := cDivPow10W(c, -exp)
		if !cIsZeroW(r) {
			return plusZero
		}
		c = q
	}
	if cCmpW(c, cPow10W(PrecisionW-1)) >= 0 {
		return plusZero
	}
	return cNaNPayloadW(c, signaling)
}
