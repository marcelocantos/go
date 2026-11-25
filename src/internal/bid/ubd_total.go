// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// totalOrder of IEEE 754-2019 clause 5.10 on UBD. A UBD pattern is
// canonical, so the order is total on every well-formed pattern, and two
// patterns compare equal only when they are the same bits.
//
// The order, from the clause:
//
//   - a) and b): numerically, x < y orders x first;
//   - c) equal values: -0 before +0, and members of one cohort by exponent,
//     the smaller first for a positive value and the larger for a negative
//     one;
//   - d) NaNs: a negative NaN before every number and a positive NaN after
//     every number; between two NaNs, the negative sign first (i), then
//     signaling before quiet for +NaN and the reverse for -NaN (ii), and
//     otherwise an order the implementation defines (iii). Here that is the
//     order of the payloads, as integers, ascending for +NaN and descending
//     for -NaN, which makes the order total.
//
// So the order is the sign first, then the magnitude, reversed for a
// negative sign. Magnitudes order finite values (by value, then exponent),
// then the infinity, then signaling NaNs, then quiet NaNs (each by payload).

// UBDTotalCompare128 returns -1, 0 or +1 as x precedes, is, or follows y in
// the total order of IEEE 754-2019 clause 5.10. It is 0 only when x and y
// are the same pattern.
func UBDTotalCompare128(x, y UBD128) int {
	xneg, yneg := x.Hi&UBDSignHi != 0, y.Hi&UBDSignHi != 0
	if xneg != yneg {
		if xneg {
			return -1
		}
		return 1
	}
	c := ubdTotalCompareMag128(x, y)
	if xneg {
		return -c
	}
	return c
}

// UBDTotalOrder128 is totalOrder (IEEE 754-2019 clause 5.10): whether x
// precedes or is y in the total order.
func UBDTotalOrder128(x, y UBD128) bool { return UBDTotalCompare128(x, y) <= 0 }

// UBDTotalOrderMag128 is totalOrderMag (IEEE 754-2019 clause 5.7.2):
// totalOrder(abs(x), abs(y)), whether the magnitude of x precedes or is the
// magnitude of y in the total order.
func UBDTotalOrderMag128(x, y UBD128) bool { return ubdTotalCompareMag128(x, y) <= 0 }

// ubdTotalCompareMag128 orders the magnitudes of x and y, their signs
// ignored, as totalOrder orders positive values.
func ubdTotalCompareMag128(x, y UBD128) int {
	xr, yr := ubdTotalRank(x), ubdTotalRank(y)
	if xr != yr {
		return cmpInt(xr, yr)
	}
	switch xr {
	case ubdRankFinite:
		ax := UBD128{Lo: x.Lo, Hi: x.Hi &^ UBDSignHi}
		ay := UBD128{Lo: y.Lo, Hi: y.Hi &^ UBDSignHi}
		if c, _ := ubdQuietCmp128(ax, ay); c != 0 {
			return c
		}
		// One value: members of one cohort, the smaller exponent first.
		return cmpInt(int(int64(x.Hi)>>UBDExpShift), int(int64(y.Hi)>>UBDExpShift))
	case ubdRankInf:
		return 0
	}
	// Two NaNs of one kind: by payload, the low two qExp bits above the
	// low 44 coefficient bits of the high word, then the low word.
	px := x.Hi>>UBDExpShift&3<<44 | x.Hi&(1<<44-1)
	py := y.Hi>>UBDExpShift&3<<44 | y.Hi&(1<<44-1)
	if px != py {
		return cmpUint(px, py)
	}
	return cmpUint(x.Lo, y.Lo)
}

// The magnitude ranks of totalOrder.
const (
	ubdRankFinite = iota
	ubdRankInf
	ubdRankSNaN
	ubdRankQNaN
)

func ubdTotalRank(x UBD128) int {
	switch {
	case x.Hi>>60 != UBDSpecialTag:
		return ubdRankFinite
	case x.Hi>>59 == UBDInfTag:
		return ubdRankInf
	case x.Hi>>58 == UBDSNaNTag:
		return ubdRankSNaN
	}
	return ubdRankQNaN
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
