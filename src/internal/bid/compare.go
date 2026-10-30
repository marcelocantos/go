// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// Comparisons compare values, not encodings: 1.0 and 1.00 are equal, +0
// and -0 are equal, and every member of a cohort compares the same. See
// "Comparisons and ordering" in the package documentation.

// Equal64 reports whether x == y (IEEE 754-2008 clause 5.6.1 and 5.11,
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

// Less64 reports whether x < y (IEEE 754-2008 clause 5.6.1 and 5.11,
// compareQuietLess). It is false if either operand is a NaN.
func Less64(x, y uint64) bool {
	return less64(x, y)
}

// Less128 is Less64 for decimal128.
func Less128(x, y Uint128) bool {
	return less128(x, y)
}

// LessEqual64 reports whether x <= y (IEEE 754-2008 clause 5.6.1 and
// 5.11, compareQuietLessEqual). It is false if either operand is a NaN.
func LessEqual64(x, y uint64) bool {
	return lessEqual64(x, y)
}

// LessEqual128 is LessEqual64 for decimal128.
func LessEqual128(x, y Uint128) bool {
	return lessEqual128(x, y)
}

// Compare64 returns the ordering of x and y as -1, 0 or +1 with ok = true,
// or 0 with ok = false when they are unordered because at least one is a
// NaN (IEEE 754-2008 clause 5.11). Compare64 is the quiet comparison; the
// signaling variants of the standard differ only in the exception they
// raise, which this package does not expose.
func Compare64(x, y uint64) (c int, ok bool) {
	return quietCmp64(x, y)
}

// Compare128 is Compare64 for decimal128.
func Compare128(x, y Uint128) (c int, ok bool) {
	return quietCmp128(x, y)
}

// TotalOrder64 reports whether x precedes or equals y in the total order of
// IEEE 754-2008 clause 5.10 (totalOrder): -NaN < -∞ < negative finite
// values < -0 < +0 < positive finite values < +∞ < +NaN, with members of a
// cohort ordered by exponent (the smaller exponent first for positive
// values and last for negative ones). A NaN is ordered by its sign bit
// alone: its payload and whether it is signaling are ignored, so all NaNs
// of one sign are equal in the order (a divergence from clause 5.10,
// which orders them by payload; see "Comparisons and ordering" in the
// package documentation). Unlike the other comparisons it is defined on
// every pair of operands, so it can sort a slice of bit patterns.
func TotalOrder64(x, y uint64) bool {
	return totalOrder64(x, y)
}

// TotalOrder128 is TotalOrder64 for decimal128.
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
