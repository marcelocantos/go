// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"testing"
)

// BenchmarkFull measures addition and subtraction of two coefficients of
// full precision whose exponents differ by 4, so that the aligned sum has
// Precision+4 digits and rounds: the "full" set of the T13 cross-library
// comparison (34-digit 1234567890123456789012345678901234 and
// 9876543210987654321098765432109876E-4 for decimal128, their first 16
// digits for decimal64), in four variants of the last digit so that nothing
// folds. The rows are not ratchet rows; BenchmarkKernel's are.
func BenchmarkFull(b *testing.B) {
	xs := []string{"1234567890123456789012345678901234", "1234567890123456789012345678901233", "1234567890123456789012345678901232", "1234567890123456789012345678901231"}
	ys := []string{"9876543210987654321098765432109876E-4", "9876543210987654321098765432109877E-4", "9876543210987654321098765432109878E-4", "9876543210987654321098765432109879E-4"}
	var x64, y64 []uint64
	var x128, y128 []bits
	for i := range xs {
		x, _ := bid.Parse64(xs[i][:15]+xs[i][33:], bid.ToNearestEven)
		y, _ := bid.Parse64(ys[i][:15]+ys[i][33:], bid.ToNearestEven)
		x64, y64 = append(x64, x), append(y64, y)
		xw, _ := bid.Parse128(xs[i], bid.ToNearestEven)
		yw, _ := bid.Parse128(ys[i], bid.ToNearestEven)
		x128, y128 = append(x128, xw), append(y128, yw)
	}
	const m = bid.ToNearestEven
	b.Run("Add/64/full", func(b *testing.B) {
		loop2(b, x64, y64, func(x, y uint64) uint64 { return bid.Add64(x, y, m) })
	})
	b.Run("Add/128/full", func(b *testing.B) {
		loop2(b, x128, y128, func(x, y bits) bits { return bid.Add128(x, y, m) })
	})
	b.Run("Sub/64/full", func(b *testing.B) {
		loop2(b, x64, y64, func(x, y uint64) uint64 { return bid.Sub64(x, y, m) })
	})
	b.Run("Sub/128/full", func(b *testing.B) {
		loop2(b, x128, y128, func(x, y bits) bits { return bid.Sub128(x, y, m) })
	})
}
