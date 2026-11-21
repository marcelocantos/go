// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package d128

// The entry points internal/bid calls: each takes and returns decimal128
// values as their UBD words, low first.

// Rounding directions, decimal128-go's round values.
const (
	TiesToEven     = int(const_ROUND_TIES_TO_EVEN)
	TiesToAway     = int(const_ROUND_TIES_TO_AWAY)
	TowardZero     = int(const_ROUND_TOWARD_ZERO)
	TowardPositive = int(const_ROUND_TOWARD_POSITIVE)
	TowardNegative = int(const_ROUND_TOWARD_NEGATIVE)
)

// Add returns x + y rounded in direction r.
func Add(xLo, xHi, yLo, yHi uint64, r int) (lo, hi uint64) {
	z := d128_add_rnd_ctxnull(d128{int64(xLo), int64(xHi)}, d128{int64(yLo), int64(yHi)}, round(r), nil)
	return uint64(z.ubdLo64), uint64(z.ubdHi64)
}

// Mul returns x * y rounded in direction r.
func Mul(xLo, xHi, yLo, yHi uint64, r int) (lo, hi uint64) {
	z := d128_multiply_rnd_ctxnull(d128{int64(xLo), int64(xHi)}, d128{int64(yLo), int64(yHi)}, round(r), nil)
	return uint64(z.ubdLo64), uint64(z.ubdHi64)
}

// Sub returns x - y rounded in direction r.
func Sub(xLo, xHi, yLo, yHi uint64, r int) (lo, hi uint64) {
	z := d128_subtract_rnd_ctxnull(d128{int64(xLo), int64(xHi)}, d128{int64(yLo), int64(yHi)}, round(r), nil)
	return uint64(z.ubdLo64), uint64(z.ubdHi64)
}
