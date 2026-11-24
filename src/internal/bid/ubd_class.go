// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// The non-computational predicates of IEEE 754-2019 clause 5.7.2 on UBD.
// None of them is exceptional, even for a signaling NaN. Each reads the
// high word's tag, exponent and coefficient bits; see the layout at UBD.

// UBDIsFinite128 reports whether x is neither an infinity nor a NaN
// (isFinite).
func UBDIsFinite128(x UBD128) bool { return x.Hi>>60 != UBDSpecialTag }

// UBDIsZero128 reports whether x is a zero of either sign (isZero). Shifting
// out the exponent and the sign leaves the coefficient, and every special
// value has the 0x1F prefix there, so a zero coefficient is a finite zero.
func UBDIsZero128(x UBD128) bool { return x.Hi<<15|x.Lo == 0 }

// UBDIsSubnormal128 reports whether x is finite, non-zero and below the
// smallest normal magnitude 10^Emin128 (isSubnormal): its exponent is below
// Emin128 and its coefficient below 10^(Emin128-exp), so that its most
// significant digit is below Emin128. A special value's exponent field,
// 7168 or more, is never below Emin128.
func UBDIsSubnormal128(x UBD128) bool {
	e := int64(x.Hi) >> UBDExpShift
	if e >= Emin128 || UBDIsZero128(x) {
		return false
	}
	// MinExp128 <= e < Emin128, so the power is 10^1 to 10^33.
	return u128Lt(Uint128{Hi: x.Hi & UBDCoeffMaskHi, Lo: x.Lo}, pow10tab128[Emin128-e])
}

// UBDIsNormal128 reports whether x is finite, non-zero and not subnormal
// (isNormal).
func UBDIsNormal128(x UBD128) bool {
	return UBDIsFinite128(x) && !UBDIsZero128(x) && !UBDIsSubnormal128(x)
}

// UBDIsCanonical128 reports whether x is canonical (isCanonical). It is
// always true: the conversion from BID stores the value IEEE 754-2019
// clause 3.5.2 gives a non-canonical encoding, so UBD has no non-canonical
// patterns (see UBDWellFormed128).
func UBDIsCanonical128(x UBD128) bool { return true }

// The classes of IEEE 754-2019 clause 5.7.2, in the order of the standard's
// list, which is also Intel's class functions' numbering.
const (
	ClassSignalingNaN = iota
	ClassQuietNaN
	ClassNegativeInfinity
	ClassNegativeNormal
	ClassNegativeSubnormal
	ClassNegativeZero
	ClassPositiveZero
	ClassPositiveSubnormal
	ClassPositiveNormal
	ClassPositiveInfinity
)

// UBDClass128 returns the class of x (class), one of the Class constants.
func UBDClass128(x UBD128) int {
	neg := x.Hi&UBDSignHi != 0
	switch {
	case x.Hi>>58 == UBDSNaNTag:
		return ClassSignalingNaN
	case x.Hi>>59 == UBDNaNTag:
		return ClassQuietNaN
	case x.Hi>>59 == UBDInfTag:
		return signedClass(neg, ClassPositiveInfinity, ClassNegativeInfinity)
	case UBDIsZero128(x):
		return signedClass(neg, ClassPositiveZero, ClassNegativeZero)
	case UBDIsSubnormal128(x):
		return signedClass(neg, ClassPositiveSubnormal, ClassNegativeSubnormal)
	}
	return signedClass(neg, ClassPositiveNormal, ClassNegativeNormal)
}

func signedClass(neg bool, pos, negClass int) int {
	if neg {
		return negClass
	}
	return pos
}
