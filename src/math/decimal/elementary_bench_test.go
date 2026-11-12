// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package decimal_test

import (
	"math/decimal"
	"testing"
)

// BenchmarkElementary measures the square root, the powers, the
// exponential and the logarithms on the operands a program has: amounts,
// rates and counts. A row named exact has a result that is found from the
// coefficients; the others go through the multiple-precision arithmetic
// once, and allocate there.
func BenchmarkElementary(b *testing.B) {
	const m = 7
	rates := []decimal128{0.0125, 0.0450, 0.0001, 0.0875, 0.0025, 0.0650, 0.1200, 0.0399}
	amounts := money128
	squares := []decimal128{2.25, 6.25, 1.5625, 0.0625, 144, 1.21, 10000, 0.81}
	b.Run("Sqrt/128", func(b *testing.B) {
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = decimal.Sqrt(amounts[i&m])
		}
		moneySink128 = s
	})
	b.Run("Sqrt/128/exact", func(b *testing.B) {
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = decimal.Sqrt(squares[i&m])
		}
		moneySink128 = s
	})
	b.Run("Sqrt/64", func(b *testing.B) {
		var s decimal64
		for i := 0; i < b.N; i++ {
			s = decimal.Sqrt(money64[i&m])
		}
		moneySink64 = s
	})
	b.Run("Exp/128", func(b *testing.B) {
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = decimal.Exp(rates[i&m])
		}
		moneySink128 = s
	})
	b.Run("Log/128", func(b *testing.B) {
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = decimal.Log(amounts[i&m])
		}
		moneySink128 = s
	})
	b.Run("Log10/128", func(b *testing.B) {
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = decimal.Log10(amounts[i&m])
		}
		moneySink128 = s
	})
	b.Run("Pow/128", func(b *testing.B) {
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = decimal.Pow(amounts[i&m], rates[(i+3)&m])
		}
		moneySink128 = s
	})
	b.Run("Pown/128", func(b *testing.B) { // a growth factor over 30 years of months
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = decimal.Pown(1+rates[i&m], 360)
		}
		moneySink128 = s
	})
	b.Run("Pown/128/exact", func(b *testing.B) {
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = decimal.Pown(squares[i&m], 3)
		}
		moneySink128 = s
	})
	b.Run("Compound/128", func(b *testing.B) {
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = decimal.Compound(rates[i&m], 360)
		}
		moneySink128 = s
	})
	b.Run("Compound/64", func(b *testing.B) {
		var s decimal64
		for i := 0; i < b.N; i++ {
			s = decimal.Compound(decimal64(rates[i&m]), 360)
		}
		moneySink64 = s
	})
	b.Run("Rootn/128", func(b *testing.B) { // a monthly factor from a yearly one
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = decimal.Rootn(1+rates[i&m], 12)
		}
		moneySink128 = s
	})
	b.Run("ScaleB/128", func(b *testing.B) {
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = decimal.ScaleB(amounts[i&m], 2)
		}
		moneySink128 = s
	})
	b.Run("LogB/128", func(b *testing.B) {
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = decimal.LogB(amounts[i&m])
		}
		moneySink128 = s
	})
	b.Run("NextUp/128", func(b *testing.B) {
		var s decimal128
		for i := 0; i < b.N; i++ {
			s = decimal.NextUp(amounts[i&m])
		}
		moneySink128 = s
	})
}
