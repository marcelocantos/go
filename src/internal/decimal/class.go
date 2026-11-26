// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package decimal

import "internal/bid"

// The non-computational operations of IEEE 754-2019 clause 5.7.2 that the
// functions above do not already give. A decimal128 stored as UBD reads its
// own predicates; decimal64, and decimal128 on a target that stores BID,
// read the BID predicates.

// IsFinite reports whether x is neither an infinity nor a NaN.
func IsFinite[D Decimal](x D) bool {
	if is64(x) {
		return bid.IsFinite64(BID64bits(decimal64(x)))
	}
	if bid.NativeUBD128 {
		return bid.UBDIsFinite128(ubdWords(decimal128(x)))
	}
	return bid.IsFinite128(b128(decimal128(x)))
}

// IsZero reports whether x is a zero of either sign.
func IsZero[D Decimal](x D) bool {
	if is64(x) {
		return bid.IsZero64(BID64bits(decimal64(x)))
	}
	if bid.NativeUBD128 {
		return bid.UBDIsZero128(ubdWords(decimal128(x)))
	}
	return bid.IsZero128(b128(decimal128(x)))
}

// IsSubnormal reports whether x is finite, non-zero and smaller in magnitude
// than the smallest normal value of its type.
func IsSubnormal[D Decimal](x D) bool {
	if is64(x) {
		return bid.IsSubnormal64(BID64bits(decimal64(x)))
	}
	if bid.NativeUBD128 {
		return bid.UBDIsSubnormal128(ubdWords(decimal128(x)))
	}
	return bid.IsSubnormal128(b128(decimal128(x)))
}

// IsNormal reports whether x is finite, non-zero and not subnormal.
func IsNormal[D Decimal](x D) bool {
	if !is64(x) && bid.NativeUBD128 {
		return bid.UBDIsNormal128(ubdWords(decimal128(x)))
	}
	return IsFinite(x) && !IsZero(x) && !IsSubnormal(x)
}

// IsCanonical reports whether x is stored in its canonical encoding.
func IsCanonical[D Decimal](x D) bool {
	if is64(x) {
		return bid.IsCanonical64(BID64bits(decimal64(x)))
	}
	if bid.NativeUBD128 {
		return bid.UBDIsCanonical128(ubdWords(decimal128(x)))
	}
	return bid.IsCanonical128(b128(decimal128(x)))
}

// Class returns the class of x, one of internal/bid's Class constants.
func Class[D Decimal](x D) int {
	if !is64(x) && bid.NativeUBD128 {
		return bid.UBDClass128(ubdWords(decimal128(x)))
	}
	var signaling bool
	if is64(x) {
		signaling = bid.IsSignaling64(BID64bits(decimal64(x)))
	} else {
		signaling = bid.IsSignaling128(b128(decimal128(x)))
	}
	neg := Signbit(x)
	switch {
	case signaling:
		return bid.ClassSignalingNaN
	case IsNaN(x):
		return bid.ClassQuietNaN
	case IsInf(x, 0):
		return signedClass(neg, bid.ClassPositiveInfinity, bid.ClassNegativeInfinity)
	case IsZero(x):
		return signedClass(neg, bid.ClassPositiveZero, bid.ClassNegativeZero)
	case IsSubnormal(x):
		return signedClass(neg, bid.ClassPositiveSubnormal, bid.ClassNegativeSubnormal)
	}
	return signedClass(neg, bid.ClassPositiveNormal, bid.ClassNegativeNormal)
}

func signedClass(neg bool, pos, negClass int) int {
	if neg {
		return negClass
	}
	return pos
}
