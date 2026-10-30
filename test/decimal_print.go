// run

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Test the print and println of decimal values, which use the character form
// of internal/bid and so preserve the quantum.

package main

import "math"

type myDecimal decimal64

func main() {
	var one, d decimal64 = 1, 70000000000000
	println(one / d)
	println(decimal64(1.50), decimal64(1.500), decimal128(1.50))
	println(decimal64(100), decimal64(1e3), decimal128(1e-20))
	println(-decimal64(2.5), myDecimal(2.5))

	var a, b decimal128 = 1e20, 1e-20
	println(a + b)

	big := math.Decimal128frombits(0x5ffe314dc6448d93, 0x38c15b0a00000000) // 1E+6144
	var ten decimal128 = 10
	println(big, big*ten, -big*ten)

	println(math.Decimal64NaN(), math.Decimal128NaN())
	println(math.Decimal64Inf(1), math.Decimal64Inf(-1))
	println(math.Decimal128Inf(1), math.Decimal128Inf(-1))
	println(math.Decimal64frombits(1<<63), decimal64(0), decimal64(0.00))

	var zeroValue decimal64
	println(zeroValue)
}
