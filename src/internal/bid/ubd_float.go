// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !compiler_bootstrap

package bid

import (
	"internal/strconv"
	"math/bits"
)

// The conversions between decimal128 and the binary formats float32 and
// float64 on UBD: float.go's, with UBD's decoding and packing. The runtime
// converts with them where decimal128 is stored as UBD. Like float.go this
// file is excluded from the compiler bootstrap build, which has no
// internal/strconv.
//
// Decimal to binary needs only the operand's parts, which UBD gives as
// fields: strconv's Float64FromCoeff and Float32FromCoeff take them as they
// are. Binary to decimal is fromFloat128 with its result rounded and packed
// as UBD; its arithmetic (fromfloat.go, fromfloat_fast.go) is shared.

// UBDFloat64To128 is Float64To128 on UBD: f rounded in mode from its exact
// binary value, with clause 5.4.2's preferred exponent.
func UBDFloat64To128(f float64, mode Mode) UBD128 {
	kind, neg, m, e := float64Parts(f)
	return ubdFromFloat128(kind, neg, m, e, mode)
}

// UBDFloat32To128 is Float32To128 on UBD.
func UBDFloat32To128(f float32, mode Mode) UBD128 {
	kind, neg, m, e := float32Parts(f)
	return ubdFromFloat128(kind, neg, m, e, mode)
}

// UBDFloat64From128 is Float64From128 on UBD: x rounded once in mode from
// its exact decimal value to float64.
func UBDFloat64From128(x UBD128, mode Mode) float64 {
	kind, neg, c, exp := UBDUnpack128(x)
	if kind != KindFinite {
		return float64Special(kind, neg)
	}
	return strconv.Float64FromCoeff(c.Hi, c.Lo, exp, neg, binaryRounding(mode, neg))
}

// UBDFloat32From128 is Float32From128 on UBD.
func UBDFloat32From128(x UBD128, mode Mode) float32 {
	kind, neg, c, exp := UBDUnpack128(x)
	if kind != KindFinite {
		return float32Special(kind, neg)
	}
	return strconv.Float32FromCoeff(c.Hi, c.Lo, exp, neg, binaryRounding(mode, neg))
}

// ubdFromFloat128 is fromFloat128 (fromfloat_tmpl.go) on UBD: the binary
// value of kind kind and, when finite, (-1)^neg × m × 2^e, as decimal128,
// rounded once in mode with clause 5.4.2's preferred exponent. A NaN is the
// canonical quiet NaN and an infinity keeps its sign. The digit count, the
// four cases and their exactness argument are fromFloat128's, unchanged;
// TestUBDFloat checks the two against each other.
func ubdFromFloat128(kind Kind, neg bool, m uint64, e int, mode Mode) UBD128 {
	switch kind {
	case KindNaN:
		return ubdQNaN
	case KindInf:
		return ubdInf(neg)
	}
	if m == 0 {
		// A zero keeps its sign and takes the preferred exponent.
		r, _ := ubdRoundFloor128(neg, wide128{}, 0, classZero, mode, MinExp128)
		return r
	}
	tz := bits.TrailingZeros64(m)
	m >>= tz
	e += tz
	t := log10Pow2(bits.Len64(m) + e - 1)
	s := Precision128 - t
	var q Uint128
	exp := 0
	class := classZero
	switch {
	case e >= 0 && bits.Len64(m)+e <= 128:
		q = u128Shl(Uint128{Lo: m}, uint(e))
	case e >= 0:
		k := -s
		if fq, sticky, ok := divPow5Fast(m, e-k, k); ok {
			q = fq
			if sticky {
				class = classBelowHalf
			}
		} else {
			var x floatNat
			x.setShl(m, e-k)
			if x.divPow5(k) {
				class = classBelowHalf
			}
			q = x.lo128()
		}
		exp = k
	case -e <= s:
		q = mulPow5Exact(m, -e)
		exp = e
	default:
		if fq, ok := mulPow5Fast(m, s, -e-s); ok {
			q = fq
		} else {
			var x floatNat
			x.setMulPow5(m, s)
			q = x.shr128(-e - s)
		}
		exp = -s
		class = classBelowHalf
	}
	if class != classZero && !u128GePow10(q, Precision128) {
		panic(errFromFloatDigits)
	}
	r, _ := ubdRoundFloor128(neg, cFromU128128(q), exp, class, mode, MinExp128)
	return r
}
