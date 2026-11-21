// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// Kernel entries for the language's min and max and for its conversions
// between decimal128 and the 64-bit integers, the rounding behind
// math/decimal's Compose, and the runtime's map hash and equality, on UBD. Each is this package's BID function of
// the same name without the ubd prefix, with UBD's decoding; none converts
// to BID. A conversion from an integer needs no entry: the integer is the
// coefficient at exponent 0, which the compiler builds inline.

// ubdMin128 is min128 on UBD: the IEEE 754-2019 minimum, which is Go's min.
// A NaN operand gives a NaN, made quiet, as ubdNaN2 chooses it; otherwise
// the lesser operand, -0 below +0, and of two members of one cohort the one
// totalOrder puts first. A well-formed operand is canonical, so the result
// is the operand itself, where min128 repacks it.
func ubdMin128(x, y UBD128) UBD128 {
	c, ok := ubdCompare128(x, y)
	if !ok {
		return ubdNaN2(x, y)
	}
	xs, ys := x.Hi&UBDSignHi != 0, y.Hi&UBDSignHi != 0
	// Equal values of unlike sign can only be zeros, and -0 is the lesser.
	takeY := b2u(c > 0) | b2u(c == 0)&b2u(ys)&^b2u(xs)
	if c == 0 && x != y && xs == ys && ubdTotalBefore128(y, x) {
		takeY = 1
	}
	return ubdSel128(-takeY, x, y)
}

// ubdMax128 is max128 on UBD: ubdMin128 with the order reversed.
func ubdMax128(x, y UBD128) UBD128 {
	c, ok := ubdCompare128(x, y)
	if !ok {
		return ubdNaN2(x, y)
	}
	xs, ys := x.Hi&UBDSignHi != 0, y.Hi&UBDSignHi != 0
	takeY := b2u(c < 0) | b2u(c == 0)&b2u(xs)&^b2u(ys)
	if c == 0 && x != y && xs == ys && ubdTotalBefore128(x, y) {
		takeY = 1
	}
	return ubdSel128(-takeY, x, y)
}

// ubdTotalBefore128 is totalBefore128 on UBD, for two equal values of one
// sign that are not the same pattern: members of one cohort, which are
// finite (equal infinities of one sign are one pattern). totalOrder puts
// the smaller exponent first for a positive value, the larger for a
// negative one.
func ubdTotalBefore128(a, b UBD128) bool {
	ea, eb := int64(a.Hi)>>UBDExpShift, int64(b.Hi)>>UBDExpShift
	if a.Hi&UBDSignHi != 0 {
		return ea > eb
	}
	return ea < eb
}

// ubdSel128 is cSel128 on UBD: b where the mask m is all ones, else a.
func ubdSel128(m uint64, a, b UBD128) UBD128 {
	return UBD128{Lo: a.Lo ^ (a.Lo^b.Lo)&m, Hi: a.Hi ^ (a.Hi^b.Hi)&m}
}

// ubdIntParts128 is intParts128 on UBD with TowardZero, the language's
// rounding for a conversion to an integer, which never increments: x
// truncated to an integer, as a class, a sign and a magnitude, with big for
// a magnitude beyond a uint64 and inexact for a truncated one.
func ubdIntParts128(x UBD128) (kind Kind, neg bool, m uint64, big, inexact bool) {
	neg = x.Hi&UBDSignHi != 0
	if x.Hi>>60 == UBDSpecialTag {
		if x.Hi>>59 == UBDNaNTag {
			return KindNaN, neg, 0, false, false
		}
		return KindInf, neg, 0, false, false
	}
	kind = KindFinite
	c, exp := coef128{Hi: x.Hi & UBDCoeffMaskHi, Lo: x.Lo}, int(int64(x.Hi)>>UBDExpShift)
	if exp >= 0 {
		scaled, over := cScale128(c, exp)
		m, fits := cToU64128(scaled)
		if over || !fits {
			return kind, neg, 0, true, false
		}
		return kind, neg, m, false, false
	}
	q, class := cShift128(c, -exp)
	inexact = class != classZero
	m, fits := cToU64128(q)
	if !fits {
		return kind, neg, 0, true, inexact
	}
	return kind, neg, m, false, inexact
}

// ubdInt64From128 is the language's conversion of a decimal128 to int64:
// Int64From128 with TowardZero, without the status. A NaN gives 0, an
// infinity and an overflow the int64 of its sign's end. An integer
// coefficient at exponent 0 is the common operand (a value rounded to an
// integer, a count) and returns at once.
func ubdInt64From128(x UBD128) int64 {
	if x.Hi&^UBDSignHi == 0 && x.Lo <= maxInt64 {
		if x.Hi != 0 {
			return -int64(x.Lo)
		}
		return int64(x.Lo)
	}
	r, _ := int64Result(ubdIntParts128(x))
	return r
}

// ubdUint64From128 is ubdInt64From128 for uint64: Uint64From128 with
// TowardZero, without the status.
func ubdUint64From128(x UBD128) uint64 {
	if x.Hi == 0 {
		return x.Lo
	}
	r, _ := uint64Result(ubdIntParts128(x))
	return r
}

// UBDRound128 is Round128 on UBD: (-1)^neg × coeff × 10^exp, with sticky
// for a non-zero remainder below coeff's last digit, rounded once in mode,
// and the status. coeff may be any Uint128. math/decimal's Compose rounds
// with it.
func UBDRound128(neg bool, coeff Uint128, exp int, sticky bool, mode Mode) (UBD128, Status) {
	class := classZero
	if sticky {
		class = classBelowHalf
		if coeff == (Uint128{}) {
			exp = min(exp, MinExp128)
		}
	}
	return ubdRoundStatus128(neg, wide128{Lo: coeff}, exp, class, mode)
}

// UBDKey128 is Key128 on UBD: the canonical hash key of x, on which all
// the values equal to it agree. A non-zero finite coefficient loses its
// trailing zeros and the exponent rises by their count, so every member of
// a cohort gives one key; a zero gives the key of +0E+0, an infinity its
// sign alone and a NaN its kind alone. The runtime hashes map keys with it
// where decimal128 is stored as UBD.
func UBDKey128(x UBD128) Key {
	if x.Hi>>60 == UBDSpecialTag {
		if x.Hi>>59 == UBDNaNTag {
			return Key{Kind: KindNaN}
		}
		return Key{Kind: KindInf, Neg: x.Hi&UBDSignHi != 0}
	}
	neg, exp := x.Hi&UBDSignHi != 0, int(int64(x.Hi)>>UBDExpShift)
	var c coef128
	if hi := x.Hi & UBDCoeffMaskHi; hi == 0 {
		if x.Lo == 0 {
			return Key{Kind: KindFinite}
		}
		// Most coefficients end in a digit other than 0: they are their
		// own key, with no call.
		if x.Lo%10 != 0 {
			return Key{Coeff: Uint128{Lo: x.Lo}, Exp: int32(exp), Kind: KindFinite, Neg: neg}
		}
		c.Lo, exp = ubdStripWord(x.Lo, exp)
	} else {
		c, exp = ubdStrip128(coef128{Hi: hi, Lo: x.Lo}, exp)
	}
	return Key{Coeff: c, Exp: int32(exp), Kind: KindFinite, Neg: neg}
}

// ubdStripWord removes the trailing zeros of a one-word coefficient c, a
// non-zero multiple of 10, raising exp by their count. A word holds at
// most 19 trailing zeros, so after the first the rest number at most 18,
// and the steps 16, 8, 4, 2 and 1, each taken when it divides exactly,
// remove any such count. Every divisor is a constant, so each test is a
// multiplication.
func ubdStripWord(c uint64, exp int) (uint64, int) {
	c, exp = c/10, exp+1
	if c%1e16 == 0 {
		c, exp = c/1e16, exp+16
	}
	if c%1e8 == 0 {
		c, exp = c/1e8, exp+8
	}
	if c%1e4 == 0 {
		c, exp = c/1e4, exp+4
	}
	if c%100 == 0 {
		c, exp = c/100, exp+2
	}
	if c%10 == 0 {
		c, exp = c/10, exp+1
	}
	return c, exp
}

// ubdStrip128 is ubdStripWord for a non-zero coefficient of two words:
// key128's binary search on the count of trailing zeros, by exact division
// (cDivExactPow10128).
func ubdStrip128(c coef128, exp int) (coef128, int) {
	q, ok := cDivExactPow10128(c, 1)
	if !ok {
		return c, exp
	}
	c, exp = q, exp+1
	step := 1
	for step*2 <= Precision128 {
		step *= 2
	}
	for ; step > 0; step /= 2 {
		if q, ok := cDivExactPow10128(c, step); ok {
			c, exp = q, exp+step
		}
	}
	return c, exp
}

// UBDEqual128 is Equal128 on UBD, the language's ==: the kernel entry
// ubdEqual128, which settles the pair of one encoding, the common pair of
// map keys, without a call. The runtime compares map keys with it where
// decimal128 is stored as UBD.
func UBDEqual128(x, y UBD128) bool { return ubdEqual128(x, y) }

// UBDNaN128 is the NaN result of an operation on x and y, at least one of
// them a NaN, on UBD: the first signaling NaN, else the first NaN, made
// quiet, as nan128(x, y, y) chooses it ("NaN results" in the package
// documentation). For one operand pass it twice. math/decimal's
// elementary functions propagate NaNs with it.
func UBDNaN128(x, y UBD128) UBD128 { return ubdNaN2(x, y) }
