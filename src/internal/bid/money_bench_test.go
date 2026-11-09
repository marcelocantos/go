// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"testing"
)

// BenchmarkMoney measures the exported entry points on money operands:
// word coefficients with two decimal places, the case the word fast paths
// and the inline paths of the API are for. Rows are named Op/Width/Case.
// The operands are those of the operator benchmark of the T13 survey:
// 1234.56, 78.90, 19.99, 3.00, 250.75, 0.05, 1000000.00 and 7.25, paired
// with the operand three places on. "same" pairs have one exponent; "cents"
// quantizes the product of two of them, at exponent -4, to 0.01. The rows
// are not ratchet rows; BenchmarkKernel's are.
func BenchmarkMoney(b *testing.B) {
	texts := []string{"1234.56", "78.90", "19.99", "3.00", "250.75", "0.05", "1000000.00", "7.25"}
	var x64, y64, p64, c64 []uint64
	var x128, y128, p128, c128 []bid.Uint128
	cent64, _ := bid.Parse64("0.01", bid.ToNearestEven)
	cent128, _ := bid.Parse128("0.01", bid.ToNearestEven)
	for i := range texts {
		x, _ := bid.Parse64(texts[i], bid.ToNearestEven)
		y, _ := bid.Parse64(texts[(i+3)%len(texts)], bid.ToNearestEven)
		x64, y64 = append(x64, x), append(y64, y)
		p64, c64 = append(p64, bid.Mul64(x, y, bid.ToNearestEven)), append(c64, cent64)
		xw, _ := bid.Parse128(texts[i], bid.ToNearestEven)
		yw, _ := bid.Parse128(texts[(i+3)%len(texts)], bid.ToNearestEven)
		x128, y128 = append(x128, xw), append(y128, yw)
		p128, c128 = append(p128, bid.Mul128(xw, yw, bid.ToNearestEven)), append(c128, cent128)
	}
	const m = bid.ToNearestEven
	b.Run("Add/64/same", func(b *testing.B) {
		loop2(b, x64, y64, func(x, y uint64) uint64 { return bid.Add64(x, y, m) })
	})
	b.Run("Add/128/same", func(b *testing.B) {
		loop2(b, x128, y128, func(x, y bits) bits { return bid.Add128(x, y, m) })
	})
	b.Run("Sub/64/same", func(b *testing.B) {
		loop2(b, x64, y64, func(x, y uint64) uint64 { return bid.Sub64(x, y, m) })
	})
	b.Run("Sub/128/same", func(b *testing.B) {
		loop2(b, x128, y128, func(x, y bits) bits { return bid.Sub128(x, y, m) })
	})
	b.Run("Mul/64/same", func(b *testing.B) {
		loop2(b, x64, y64, func(x, y uint64) uint64 { return bid.Mul64(x, y, m) })
	})
	b.Run("Mul/128/same", func(b *testing.B) {
		loop2(b, x128, y128, func(x, y bits) bits { return bid.Mul128(x, y, m) })
	})
	b.Run("Quantize/64/same", func(b *testing.B) {
		loop2(b, x64, c64, func(x, y uint64) uint64 { return bid.Quantize64(x, y, m) })
	})
	b.Run("Quantize/128/same", func(b *testing.B) {
		loop2(b, x128, c128, func(x, y bits) bits { return bid.Quantize128(x, y, m) })
	})
	b.Run("Quantize/64/cents", func(b *testing.B) {
		loop2(b, p64, c64, func(x, y uint64) uint64 { return bid.Quantize64(x, y, m) })
	})
	b.Run("Quantize/128/cents", func(b *testing.B) {
		loop2(b, p128, c128, func(x, y bits) bits { return bid.Quantize128(x, y, m) })
	})
	b.Run("Less/64/same", func(b *testing.B) {
		loop2(b, x64, y64, func(x, y uint64) bool { return bid.Less64(x, y) })
	})
	b.Run("Less/128/same", func(b *testing.B) {
		loop2(b, x128, y128, func(x, y bits) bool { return bid.Less128(x, y) })
	})
	b.Run("Compare/64/same", func(b *testing.B) {
		loop2(b, x64, y64, func(x, y uint64) int { c, _ := bid.Compare64(x, y); return c })
	})
	b.Run("Compare/128/same", func(b *testing.B) {
		loop2(b, x128, y128, func(x, y bits) int { c, _ := bid.Compare128(x, y); return c })
	})
}
