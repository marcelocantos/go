// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import "internal/bid"

// ubdKernel is the kernel with the decimal128 operations the compiler calls
// replaced by its UBD entries: each operand converted to UBD, the
// TiesToEven entry called for ToNearestEven and the Rnd entry for every
// other rounding direction, and the result converted back. -bid.vectors=ubd
// runs the published vectors against it, so that they exercise the UBD
// kernel and not only the BID functions. Every other operation is the
// kernel's.
type ubdKernel struct{ kernel }

func ubdIn(x bits) bits  { return bid.UBDFromBID128(x) }
func ubdOut(x bits) bits { return bid.BIDFromUBDAny128(x) }

func ubdBinary(tte func(x, y bits) bits, rnd func(x, y bits, m bid.Mode) bits, x, y bits, m bid.Mode) bits {
	if m == bid.ToNearestEven {
		return ubdOut(tte(ubdIn(x), ubdIn(y)))
	}
	return ubdOut(rnd(ubdIn(x), ubdIn(y), m))
}

func (k ubdKernel) add(f *format, x, y bits, m bid.Mode) bits {
	if w64(f) {
		return k.kernel.add(f, x, y, m)
	}
	return ubdBinary(bid.UBDAddTTE128, bid.UBDAddRnd128, x, y, m)
}
func (k ubdKernel) sub(f *format, x, y bits, m bid.Mode) bits {
	if w64(f) {
		return k.kernel.sub(f, x, y, m)
	}
	return ubdBinary(bid.UBDSubTTE128, bid.UBDSubRnd128, x, y, m)
}
func (k ubdKernel) mul(f *format, x, y bits, m bid.Mode) bits {
	if w64(f) {
		return k.kernel.mul(f, x, y, m)
	}
	return ubdBinary(bid.UBDMulTTE128, bid.UBDMulRnd128, x, y, m)
}
func (k ubdKernel) div(f *format, x, y bits, m bid.Mode) bits {
	if w64(f) {
		return k.kernel.div(f, x, y, m)
	}
	return ubdBinary(bid.UBDDivTTE128, bid.UBDDivRnd128, x, y, m)
}

// fma is UBDFMA128, the entry math/decimal's FMA calls, which takes the
// rounding direction itself rather than coming in a TTE and Rnd pair.
func (k ubdKernel) fma(f *format, x, y, z bits, m bid.Mode) bits {
	if w64(f) {
		return k.kernel.fma(f, x, y, z, m)
	}
	r := bid.UBDFMA128(ubdWord(x), ubdWord(y), ubdWord(z), m)
	return ubdOut(bits{Hi: r.Hi, Lo: r.Lo})
}

// ubdWord is ubdIn for an entry that takes bid.UBD128.
func ubdWord(x bits) bid.UBD128 {
	u := ubdIn(x)
	return bid.UBD128{Hi: u.Hi, Lo: u.Lo}
}
func (k ubdKernel) quantize(f *format, x, y bits, m bid.Mode) bits {
	if w64(f) {
		return k.kernel.quantize(f, x, y, m)
	}
	return ubdBinary(bid.UBDQuantizeTTE128, bid.UBDQuantizeRnd128, x, y, m)
}
func (k ubdKernel) roundToIntegral(f *format, x bits, m bid.Mode) bits {
	if w64(f) {
		return k.kernel.roundToIntegral(f, x, m)
	}
	if m == bid.ToNearestEven {
		return ubdOut(bid.UBDRoundToIntegralTTE128(ubdIn(x)))
	}
	return ubdOut(bid.UBDRoundToIntegralRnd128(ubdIn(x), m))
}
func (k ubdKernel) min(f *format, x, y bits) bits {
	if w64(f) {
		return k.kernel.min(f, x, y)
	}
	return ubdOut(ubdWords(bid.UBDMin128(ubdValue(ubdIn(x)), ubdValue(ubdIn(y)))))
}
func (k ubdKernel) max(f *format, x, y bits) bits {
	if w64(f) {
		return k.kernel.max(f, x, y)
	}
	return ubdOut(ubdWords(bid.UBDMax128(ubdValue(ubdIn(x)), ubdValue(ubdIn(y)))))
}
func (k ubdKernel) equal(f *format, x, y bits) bool {
	if w64(f) {
		return k.kernel.equal(f, x, y)
	}
	return bid.UBDPredicates128(ubdIn(x), ubdIn(y))[0]
}
func (k ubdKernel) less(f *format, x, y bits) bool {
	if w64(f) {
		return k.kernel.less(f, x, y)
	}
	return bid.UBDPredicates128(ubdIn(x), ubdIn(y))[1]
}
func (k ubdKernel) lessEqual(f *format, x, y bits) bool {
	if w64(f) {
		return k.kernel.lessEqual(f, x, y)
	}
	return bid.UBDPredicates128(ubdIn(x), ubdIn(y))[2]
}
func (k ubdKernel) compare(f *format, x, y bits) (int, bool) {
	if w64(f) {
		return k.kernel.compare(f, x, y)
	}
	return bid.UBDCompare128(ubdIn(x), ubdIn(y))
}

// totalOrder is UBDTotalOrder128 on the UBD patterns, which orders NaNs as
// IEEE 754-2019 clause 5.10 d) does (go-decimal-proposal#39), where the
// kernel's TotalOrder128 orders them by sign alone.
func (k ubdKernel) totalOrder(f *format, x, y bits) bool {
	if w64(f) {
		return k.kernel.totalOrder(f, x, y)
	}
	xu, yu := ubdIn(x), ubdIn(y)
	return bid.UBDTotalOrder128(bid.UBD128{Lo: xu.Lo, Hi: xu.Hi}, bid.UBD128{Lo: yu.Lo, Hi: yu.Hi})
}

// ubdTotal reports whether im orders NaNs as clause 5.10 d) does for format
// f, so that the runner's skips for the kernel's NaN order do not apply.
func ubdTotal(im impl, f *format) bool {
	_, ok := im.(ubdKernel)
	return ok && !w64(f)
}

// The predicates of IEEE 754-2019 clause 5.7.2, on the UBD pattern. isNormal
// and class are composed from these by the runner. isCanonical stays the
// kernel's: UBD has no non-canonical patterns, so UBDIsCanonical128 is
// true where the vectors test a BID encoding's canonicality.
func (k ubdKernel) isFinite(f *format, x bits) bool {
	if w64(f) {
		return k.kernel.isFinite(f, x)
	}
	return bid.UBDIsFinite128(ubdWord(x))
}
func (k ubdKernel) isZero(f *format, x bits) bool {
	if w64(f) {
		return k.kernel.isZero(f, x)
	}
	return bid.UBDIsZero128(ubdWord(x))
}
func (k ubdKernel) isSubnormal(f *format, x bits) bool {
	if w64(f) {
		return k.kernel.isSubnormal(f, x)
	}
	return bid.UBDIsSubnormal128(ubdWord(x))
}

// ubdValue and ubdWords convert between UBD words held in a Uint128 and
// the kernel's UBD128, for the exported entries that take UBD128.
func ubdValue(x bits) bid.UBD128 { return bid.UBD128{Lo: x.Lo, Hi: x.Hi} }
func ubdWords(r bid.UBD128) bits { return bits{Hi: r.Hi, Lo: r.Lo} }
