// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package decimal_test

import (
	"math/decimal"
	"testing"
)

// BenchmarkMoney measures money arithmetic as a program writes it: the
// language's operators, which the compiler lowers and the runtime completes,
// and the functions of this package. The operands are those of
// internal/bid's BenchmarkMoney, each paired with the operand three places on;
// "cents" quantizes their products, at exponent -4, to 0.01, and "nearby"
// adds amounts of two and four decimal places.
var (
	money128 = []decimal128{1234.56, 78.90, 19.99, 3.00, 250.75, 0.05, 1000000.00, 7.25}
	money64  = []decimal64{1234.56, 78.90, 19.99, 3.00, 250.75, 0.05, 1000000.00, 7.25}
	rates128 = []decimal128{0.0125, 1.0000, 0.0450, 0.9999, 0.0001, 2.5000, 0.0875, 1.1111}

	moneySink128 decimal128
	moneySink64  decimal64
	moneySinkB   bool
)

func BenchmarkMoney(b *testing.B) {
	const m = 7
	b.Run("Add/128", func(b *testing.B) {
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = money128[i&m] + money128[(i+3)&m]
		}
		moneySink128 = s
	})
	b.Run("Add/64", func(b *testing.B) {
		var s decimal64
		for i := 0; i < b.N; i++ {
			s = money64[i&m] + money64[(i+3)&m]
		}
		moneySink64 = s
	})
	b.Run("Sub/128", func(b *testing.B) {
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = money128[i&m] - money128[(i+3)&m]
		}
		moneySink128 = s
	})
	b.Run("Mul/128", func(b *testing.B) {
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = money128[i&m] * money128[(i+3)&m]
		}
		moneySink128 = s
	})
	b.Run("Nearby/128", func(b *testing.B) {
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = money128[i&m] + rates128[(i+3)&m]
		}
		moneySink128 = s
	})
	b.Run("Less/128", func(b *testing.B) {
		var s bool
		for i := 0; i < b.N; i++ {
			s = money128[i&m] < money128[(i+3)&m]
		}
		moneySinkB = s
	})
	b.Run("Quantize/128/same", func(b *testing.B) {
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = decimal.Quantize(money128[i&m], 0.01)
		}
		moneySink128 = s
	})
	b.Run("Quantize/64/same", func(b *testing.B) {
		var s decimal64
		for i := 0; i < b.N; i++ {
			s = decimal.Quantize(money64[i&m], 0.01)
		}
		moneySink64 = s
	})
	b.Run("Quantize/128/cents", func(b *testing.B) {
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = decimal.Quantize(money128[i&m]*rates128[(i+3)&m], 0.01)
		}
		moneySink128 = s
	})
}
