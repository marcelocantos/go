// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// Comparisons compare values, not encodings: 1.0 and 1.00 are equal, +0
// and -0 are equal, and every member of a cohort compares the same. See
// "Comparisons and ordering" in the package documentation.

// Equal64 reports whether x == y (IEEE 754-2019 clause 5.6.1 and 5.11,
// compareQuietEqual). It is false if either operand is a NaN. It is the
// == operator of the language and the equality the runtime uses for map
// keys.
func Equal64(x, y uint64) bool {
	return equal64(x, y)
}

// Equal128 is Equal64 for decimal128.
func Equal128(x, y Uint128) bool {
	return equal128(x, y)
}

// Less64 reports whether x < y (IEEE 754-2019 clause 5.6.1 and 5.11,
// compareQuietLess). It is false if either operand is a NaN.
func Less64(x, y uint64) bool {
	return less64(x, y)
}

// Less128 is Less64 for decimal128.
func Less128(x, y Uint128) bool {
	return less128(x, y)
}

// LessEqual64 reports whether x <= y (IEEE 754-2019 clause 5.6.1 and
// 5.11, compareQuietLessEqual). It is false if either operand is a NaN.
func LessEqual64(x, y uint64) bool {
	return lessEqual64(x, y)
}

// LessEqual128 is LessEqual64 for decimal128.
func LessEqual128(x, y Uint128) bool {
	return lessEqual128(x, y)
}

// EqualSlow128, LessSlow128 and LessEqualSlow128 are Equal128, Less128 and
// LessEqual128 without the inline front of each: the general comparison,
// which the compiler calls where it has already compared the operands that
// sameQuantumKeys128 accepts (word operands of one sign and one exponent);
// see AddSlow128.
func EqualSlow128(x, y Uint128) bool {
	c, ok := quietCmpSlow128(x, y)
	return ok && c == 0
}

func LessSlow128(x, y Uint128) bool {
	c, ok := quietCmpSlow128(x, y)
	return ok && c < 0
}

func LessEqualSlow128(x, y Uint128) bool {
	c, ok := quietCmpSlow128(x, y)
	return ok && c <= 0
}

// Compare64 returns the ordering of x and y as -1, 0 or +1 with ok = true,
// or 0 with ok = false when they are unordered because at least one is a
// NaN (IEEE 754-2019 clause 5.11). Compare64 is the quiet comparison; the
// signaling variants of the standard differ only in the exception they
// raise, which this package does not expose.
func Compare64(x, y uint64) (c int, ok bool) {
	return quietCmpSlow64(x, y)
}

// Compare128 is Compare64 for decimal128.
func Compare128(x, y Uint128) (c int, ok bool) {
	return quietCmpSlow128(x, y)
}

// TotalOrder64 reports whether x precedes or equals y in the total order of
// IEEE 754-2019 clause 5.10. Negative NaNs precede all numbers, positive
// NaNs follow them. Among NaNs of one sign, signaling precedes quiet and
// payloads increase for positive NaNs; both orders reverse for negative NaNs.
// Non-canonical NaN payloads read as zero. Finite cohorts are ordered by
// exponent (smaller first for positive values, larger first for negative).
func TotalOrder64(x, y uint64) bool {
	if IsNaN64(x) && IsNaN64(y) && x&SignMask64 == y&SignMask64 {
		// Order the magnitudes as signaling, then quiet, then by the
		// canonical payload. Reverse that order for negative NaNs.
		xs, ys := x&SignalingBit64 != 0, y&SignalingBit64 != 0
		if xs != ys {
			return xs != (x&SignMask64 != 0)
		}
		px, py := x&nanPayloadMask64, y&nanPayloadMask64
		if px >= maxNaNPayload64 {
			px = 0
		}
		if py >= maxNaNPayload64 {
			py = 0
		}
		if x&SignMask64 != 0 {
			return px >= py
		}
		return px <= py
	}
	return totalOrder64(x, y)
}

// TotalOrder128 is TotalOrder64 for decimal128, except that its legacy BID
// NaN order remains by sign alone. UBDTotalOrder128 supplies the full NaN
// order for decimal128.
func TotalOrder128(x, y Uint128) bool {
	return totalOrder128(x, y)
}

// Key is the canonical hash key of a decimal value. Two values have equal
// keys exactly when they are equal under Equal64 or Equal128, or are both
// NaN: every member of a cohort gives the same key, +0 and -0 give the
// same key, and the keys of a decimal64 value and of the same value widened
// to decimal128 are identical. Non-zero finite values and infinities carry
// their sign; zeros and NaN do not.
//
// The key is laid out for hashing as raw memory: it is 24 bytes with no
// padding, every byte is determined by the value, and the blank field is
// always zero. A caller hashing map keys checks Kind for KindNaN first,
// since a NaN is never equal to anything and must not hash consistently.
type Key struct {
	Coeff Uint128 // coefficient with trailing zeros removed; zero for zero, infinities and NaN
	Exp   int32   // exponent of the reduced coefficient; zero for zero, infinities and NaN
	Kind  Kind
	Neg   bool // false for zero and NaN
	_     [2]byte
}

// Key64 returns the hash key of x.
func Key64(x uint64) Key {
	kind, neg, c, exp := key64(x)
	return Key{Coeff: Uint128{Lo: c}, Exp: int32(exp), Kind: kind, Neg: neg}
}

// Key128 returns the hash key of x. Key128(Widen(x)) == Key64(x) for every
// decimal64 bit pattern x.
func Key128(x Uint128) Key {
	kind, neg, c, exp := key128(x)
	return Key{Coeff: c, Exp: int32(exp), Kind: kind, Neg: neg}
}
