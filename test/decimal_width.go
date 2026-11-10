// run
//go:build goexperiment.decimal

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Test that a function large enough for the sparse phi builder may use the
// compiler's own paths for both decimal widths. Those paths merge through a
// temporary, and that builder gives a temporary the type of the first read
// it meets: one temporary shared by the two widths gave such a function a
// decimal64 phi over decimal128 values, and the compiler stopped with half
// of each value never split.

package main

import "math/decimal"

//go:noinline
func mixed(a, p decimal64, b, q decimal128) (decimal64, decimal128) {
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	a, b = decimal.Quantize(a+p, p), decimal.Quantize(b+q, q)
	a, b = a*p+a, b*q+b
	a, b = max(a, p)-p, min(b, q)+q
	return a, b
}

func main() {
	a, b := mixed(1.5, 0.25, 2.5, 0.125)
	// The same steps, a width at a time.
	x, p := decimal64(1.5), decimal64(0.25)
	y, q := decimal128(2.5), decimal128(0.125)
	for range 64 {
		x = decimal.Quantize(x+p, p)
		x = x*p + x
		x = max(x, p) - p
	}
	for range 64 {
		y = decimal.Quantize(y+q, q)
		y = y*q + y
		y = min(y, q) + q
	}
	if a != x || b != y {
		println("mixed:", a, b, "want", x, y)
		panic("decimal widths")
	}
}
