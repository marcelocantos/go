// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package decimal_test

import (
	"fmt"
	"math/decimal"
)

func ExampleQuoQuantize() {
	// A month's interest on a balance at an annual rate, rounded once to
	// the cent, half up.
	balance, rate := decimal64(294016.80), decimal64(0.0625)
	interest := decimal.QuoQuantize(balance*rate, 12, 0.01, decimal.ToNearestAway)
	fmt.Println(interest)

	// Rounding the quotient to the precision first, and then to the cent,
	// can round twice: 1.0049999999999995 becomes 1.005000000000000, which
	// rounds half up to 1.01.
	x := decimal64(2.009999999999999)
	fmt.Println(decimal.QuantizeMode(x/2, 0.01, decimal.ToNearestAway))
	fmt.Println(decimal.QuoQuantize(x, 2, 0.01, decimal.ToNearestAway))

	// Integer division.
	fmt.Println(decimal.QuoQuantize(decimal64(17), 5, 1, decimal.ToZero))
	// Output:
	// 1531.34
	// 1.01
	// 1.00
	// 3
}

func ExampleAwayFromZero() {
	fee := decimal64(2.341)
	fmt.Println(decimal.QuantizeMode(fee, 0.01, decimal.AwayFromZero))
	fmt.Println(decimal.QuantizeMode(-fee, 0.01, decimal.AwayFromZero))
	// Output:
	// 2.35
	// -2.35
}

func ExampleDecompose() {
	neg, hi, lo, exp, ok := decimal.Decompose(decimal128(-1234.50))
	fmt.Println(neg, hi, lo, exp, ok)

	// A NUMERIC(10, 2) value of 1234.50 arrives as the coefficient 123450
	// and the scale 2.
	fmt.Println(decimal.Compose[decimal128](false, 0, 123450, -2))
	// Output:
	// true 0 123450 -2 true
	// 1234.50
}
