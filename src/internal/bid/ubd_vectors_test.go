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
	return ubdOut(bid.UBDMin128(ubdIn(x), ubdIn(y)))
}
func (k ubdKernel) max(f *format, x, y bits) bits {
	if w64(f) {
		return k.kernel.max(f, x, y)
	}
	return ubdOut(bid.UBDMax128(ubdIn(x), ubdIn(y)))
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
