// run

// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Test decimal64 and decimal128 types: literals, arithmetic,
// comparisons, conversions, map keys, and NaN/zero semantics.

package main

import "math"

var bad bool

func fail(msg string) {
	if !bad {
		println("BUG")
		bad = true
	}
	println(msg)
}

// --- Literal parsing ---

func testLiterals() {
	var a decimal64 = 3.14
	var b decimal64 = 1
	var c decimal64 = 0.01
	var d decimal64 = 100
	var e decimal64 = 0

	if a != decimal64(3.14) {
		fail("decimal64 literal 3.14 mismatch")
	}
	if b != decimal64(1) {
		fail("decimal64 literal 1 mismatch")
	}
	if c != decimal64(0.01) {
		fail("decimal64 literal 0.01 mismatch")
	}
	if d != decimal64(100) {
		fail("decimal64 literal 100 mismatch")
	}
	if e != decimal64(0) {
		fail("decimal64 literal 0 mismatch")
	}

	// decimal128 literals
	var a128 decimal128 = 3.14
	var b128 decimal128 = 1
	if a128 != decimal128(3.14) {
		fail("decimal128 literal 3.14 mismatch")
	}
	if b128 != decimal128(1) {
		fail("decimal128 literal 1 mismatch")
	}
}

// --- Basic arithmetic ---

func testArithmetic() {
	var a decimal64 = 10
	var b decimal64 = 3

	// Addition
	sum := a + b
	if sum != decimal64(13) {
		fail("decimal64 10+3 != 13")
	}

	// Subtraction
	diff := a - b
	if diff != decimal64(7) {
		fail("decimal64 10-3 != 7")
	}

	// Multiplication
	prod := a * b
	if prod != decimal64(30) {
		fail("decimal64 10*3 != 30")
	}

	// Division
	quot := a / b
	// 10/3 in decimal64 gives a long repeating value; check approximate
	expected := decimal64(3.333333333333333)
	if quot < expected-decimal64(0.000000000000001) || quot > expected+decimal64(0.000000000000001) {
		fail("decimal64 10/3 out of expected range")
	}

	// Exact division
	var x decimal64 = 1
	var y decimal64 = 4
	if x/y != decimal64(0.25) {
		fail("decimal64 1/4 != 0.25")
	}

	// Negation
	neg := -a
	if neg != decimal64(-10) {
		fail("decimal64 negation failed")
	}
	if neg+a != decimal64(0) {
		fail("decimal64 -10 + 10 != 0")
	}

	// decimal128 arithmetic
	var a128 decimal128 = 10
	var b128 decimal128 = 3
	if a128+b128 != decimal128(13) {
		fail("decimal128 10+3 != 13")
	}
	if a128-b128 != decimal128(7) {
		fail("decimal128 10-3 != 7")
	}
	if a128*b128 != decimal128(30) {
		fail("decimal128 10*3 != 30")
	}
}

// --- Exact decimal arithmetic (no binary float rounding) ---

func testExactArithmetic() {
	// This is the key advantage of decimals: 0.1 + 0.2 == 0.3
	var a decimal64 = 0.1
	var b decimal64 = 0.2
	var c decimal64 = 0.3
	if a+b != c {
		fail("decimal64 0.1 + 0.2 != 0.3")
	}

	// Money arithmetic: $19.99 * 3 = $59.97
	var price decimal64 = 19.99
	var qty decimal64 = 3
	var total decimal64 = 59.97
	if price*qty != total {
		fail("decimal64 19.99 * 3 != 59.97")
	}
}

// --- Comparisons ---

func testComparisons() {
	var a decimal64 = 1
	var b decimal64 = 2
	var c decimal64 = 1

	if !(a == c) {
		fail("decimal64 1 == 1 failed")
	}
	if a == b {
		fail("decimal64 1 == 2 should be false")
	}
	if !(a != b) {
		fail("decimal64 1 != 2 failed")
	}
	if !(a < b) {
		fail("decimal64 1 < 2 failed")
	}
	if a > b {
		fail("decimal64 1 > 2 should be false")
	}
	if !(a <= c) {
		fail("decimal64 1 <= 1 failed")
	}
	if !(a <= b) {
		fail("decimal64 1 <= 2 failed")
	}
	if !(b >= a) {
		fail("decimal64 2 >= 1 failed")
	}
	if !(a >= c) {
		fail("decimal64 1 >= 1 failed")
	}

	// Negative values
	var neg decimal64 = -5
	if !(neg < a) {
		fail("decimal64 -5 < 1 failed")
	}

	// decimal128 comparisons
	var a128 decimal128 = 1
	var b128 decimal128 = 2
	if !(a128 < b128) {
		fail("decimal128 1 < 2 failed")
	}
	if !(a128 == decimal128(1)) {
		fail("decimal128 1 == 1 failed")
	}
}

// --- NaN behavior ---

func testNaN() {
	nan := math.Decimal64NaN()

	// NaN != NaN
	if nan == nan {
		fail("decimal64 NaN == NaN should be false")
	}
	if !(nan != nan) {
		fail("decimal64 NaN != NaN should be true")
	}

	// NaN comparisons are all false
	var one decimal64 = 1
	if nan < one {
		fail("decimal64 NaN < 1 should be false")
	}
	if nan > one {
		fail("decimal64 NaN > 1 should be false")
	}
	if nan <= one {
		fail("decimal64 NaN <= 1 should be false")
	}
	if nan >= one {
		fail("decimal64 NaN >= 1 should be false")
	}
	if nan == one {
		fail("decimal64 NaN == 1 should be false")
	}

	// NaN propagation in arithmetic
	result := nan + one
	if !math.IsDecimal64NaN(result) {
		fail("decimal64 NaN + 1 should be NaN")
	}
	result = nan * one
	if !math.IsDecimal64NaN(result) {
		fail("decimal64 NaN * 1 should be NaN")
	}
}

// --- Negative zero ---

func testNegativeZero() {
	var zero decimal64 = 0
	negZero := -zero

	// Negative zero equals positive zero
	if negZero != zero {
		fail("decimal64 -0 != 0 should be false")
	}
	if !(negZero == zero) {
		fail("decimal64 -0 == 0 should be true")
	}

	// But they have different bit representations
	if math.Decimal64bits(negZero) == math.Decimal64bits(zero) {
		fail("decimal64 -0 and +0 should have different bits")
	}
}

// --- Conversions: int <-> decimal64 ---

func testIntConversions() {
	// int -> decimal64
	var i int = 42
	var d decimal64 = decimal64(i)
	if d != decimal64(42) {
		fail("int->decimal64 conversion failed")
	}

	// decimal64 -> int
	var d2 decimal64 = 42
	var i2 int = int(d2)
	if i2 != 42 {
		fail("decimal64->int conversion failed")
	}

	// Truncation toward zero
	var d3 decimal64 = 3.7
	var i3 int = int(d3)
	if i3 != 3 {
		fail("decimal64->int truncation failed: 3.7 should become 3")
	}

	var d4 decimal64 = -3.7
	var i4 int = int(d4)
	if i4 != -3 {
		fail("decimal64->int truncation failed: -3.7 should become -3")
	}

	// int64 -> decimal64 -> int64
	var big int64 = 1000000
	var dbig decimal64 = decimal64(big)
	var back int64 = int64(dbig)
	if back != big {
		fail("int64->decimal64->int64 roundtrip failed")
	}
}

// --- Conversions: float64 <-> decimal64 ---

func testFloatConversions() {
	// float64 -> decimal64
	var f float64 = 3.14
	var d decimal64 = decimal64(f)
	// float64(3.14) is not exactly 3.14, so we check approximate
	if d < decimal64(3.13) || d > decimal64(3.15) {
		fail("float64->decimal64 conversion out of range")
	}

	// decimal64 -> float64
	var d2 decimal64 = 2.5
	var f2 float64 = float64(d2)
	if f2 != 2.5 {
		fail("decimal64->float64 conversion of 2.5 failed")
	}

	// Exact powers of 10
	var d3 decimal64 = 100
	var f3 float64 = float64(d3)
	if f3 != 100.0 {
		fail("decimal64->float64 conversion of 100 failed")
	}

	// float64 -> decimal64 for exact integers.
	// These are all exactly representable in float64,
	// so the conversion to decimal64 must be exact.
	var f83 float64 = 83
	if decimal64(f83) != decimal64(83) {
		fail("float64->decimal64 conversion of 83 is not exact")
	}
	var f97 float64 = 97
	if decimal64(f97) != decimal64(97) {
		fail("float64->decimal64 conversion of 97 is not exact")
	}
	var f1000 float64 = 1000
	if decimal64(f1000) != decimal64(1000) {
		fail("float64->decimal64 conversion of 1000 is not exact")
	}

	// float32 -> decimal64
	var f32 float32 = 1.5
	var d32 decimal64 = decimal64(f32)
	if d32 != decimal64(1.5) {
		fail("float32->decimal64 conversion of 1.5 failed")
	}
}

// --- Conversions: decimal64 <-> decimal128 ---

func testDecimalWidening() {
	// decimal64 -> decimal128
	var d64 decimal64 = 3.14
	var d128 decimal128 = decimal128(d64)
	if d128 != decimal128(3.14) {
		fail("decimal64->decimal128 widening failed")
	}

	// decimal128 -> decimal64
	var d128b decimal128 = 42
	var d64b decimal64 = decimal64(d128b)
	if d64b != decimal64(42) {
		fail("decimal128->decimal64 narrowing failed")
	}

	// Round-trip
	var orig decimal64 = 1.23
	var wide decimal128 = decimal128(orig)
	var narrow decimal64 = decimal64(wide)
	if narrow != orig {
		fail("decimal64->decimal128->decimal64 roundtrip failed")
	}
}

// --- Map key behavior ---

func testMapKeys() {
	// decimal64 as map key
	m := make(map[decimal64]string)
	m[decimal64(1)] = "one"
	m[decimal64(2)] = "two"
	m[decimal64(3.14)] = "pi"

	if m[decimal64(1)] != "one" {
		fail("map[decimal64] lookup for 1 failed")
	}
	if m[decimal64(2)] != "two" {
		fail("map[decimal64] lookup for 2 failed")
	}
	if m[decimal64(3.14)] != "pi" {
		fail("map[decimal64] lookup for 3.14 failed")
	}

	// Negative zero should map to the same key as positive zero
	m2 := make(map[decimal64]int)
	var zero decimal64 = 0
	negZero := -zero
	m2[zero] = 1
	m2[negZero] = 2 // should overwrite
	if len(m2) != 1 {
		fail("map[decimal64] -0 and +0 should be same key")
	}
	if m2[zero] != 2 {
		fail("map[decimal64] +0 lookup should find negZero's value")
	}

	// NaN keys: each insert creates a new entry (NaN != NaN)
	m3 := make(map[decimal64]int)
	nan := math.Decimal64NaN()
	m3[nan] = 1
	m3[nan] = 2
	if len(m3) != 2 {
		fail("map[decimal64] NaN keys should create separate entries")
	}

	// Cannot look up NaN keys
	v, ok := m3[nan]
	if ok {
		fail("map[decimal64] NaN lookup should return !ok")
	}
	_ = v

	// decimal128 as map key
	m4 := make(map[decimal128]string)
	m4[decimal128(1)] = "one"
	if m4[decimal128(1)] != "one" {
		fail("map[decimal128] lookup failed")
	}
}

// --- Named constants ---

func testConstants() {
	const pi = 3.14
	var x decimal64 = pi
	if x != decimal64(3.14) {
		fail("decimal64 from untyped constant failed")
	}

	const answer = 42
	var y decimal64 = answer
	if y != decimal64(42) {
		fail("decimal64 from untyped int constant failed")
	}

	const half = 0.5
	var z decimal128 = half
	if z != decimal128(0.5) {
		fail("decimal128 from untyped constant failed")
	}

	// Typed decimal64 constant is not allowed (decimal types are not constant types),
	// but untyped float/int constants should work fine as shown above.
}

// --- Infinity ---

func testInfinity() {
	inf := math.Decimal64Inf(1)
	ninf := math.Decimal64Inf(-1)

	// Inf comparisons
	if !(inf > decimal64(999999)) {
		fail("decimal64 +Inf > 999999 failed")
	}
	if !(ninf < decimal64(-999999)) {
		fail("decimal64 -Inf < -999999 failed")
	}
	if inf != inf {
		fail("decimal64 +Inf == +Inf failed")
	}
	if !(ninf < inf) {
		fail("decimal64 -Inf < +Inf failed")
	}

	// Inf arithmetic
	if inf+decimal64(1) != inf {
		fail("decimal64 Inf + 1 != Inf")
	}
	if !math.IsDecimal64NaN(inf + ninf) {
		fail("decimal64 +Inf + -Inf should be NaN")
	}
}

// --- Named types ---

func testNamedTypes() {
	type MyDecimal decimal64

	var a MyDecimal = 3.14
	var b MyDecimal = 2.86
	sum := a + b
	if sum != MyDecimal(6) {
		fail("named decimal64 type arithmetic failed")
	}

	// Conversion between named and underlying type
	var d decimal64 = decimal64(a)
	if d != decimal64(3.14) {
		fail("named decimal64 -> decimal64 conversion failed")
	}
}

// --- Struct with decimal fields ---

func testStructs() {
	type Price struct {
		amount   decimal64
		currency string
	}

	p := Price{amount: 19.99, currency: "USD"}
	if p.amount != decimal64(19.99) {
		fail("struct decimal64 field failed")
	}

	// Struct comparison with decimal fields
	q := Price{amount: 19.99, currency: "USD"}
	if p != q {
		fail("struct with decimal64 field comparison failed")
	}
}

// --- Slice and array ---

func testSliceArray() {
	arr := [3]decimal64{1.1, 2.2, 3.3}
	if arr[0] != decimal64(1.1) {
		fail("decimal64 array element 0 failed")
	}
	if arr[1] != decimal64(2.2) {
		fail("decimal64 array element 1 failed")
	}
	if arr[2] != decimal64(3.3) {
		fail("decimal64 array element 2 failed")
	}

	// Array comparison
	arr2 := [3]decimal64{1.1, 2.2, 3.3}
	if arr != arr2 {
		fail("decimal64 array comparison failed")
	}

	s := []decimal64{10, 20, 30}
	var sum decimal64
	for _, v := range s {
		sum += v
	}
	if sum != decimal64(60) {
		fail("decimal64 slice sum failed")
	}
}

// --- Interface ---

func testInterface() {
	var x interface{} = decimal64(42)
	d, ok := x.(decimal64)
	if !ok {
		fail("decimal64 type assertion failed")
	}
	if d != decimal64(42) {
		fail("decimal64 type assertion value mismatch")
	}

	// Interface equality
	var a interface{} = decimal64(1)
	var b interface{} = decimal64(1)
	if a != b {
		fail("decimal64 interface equality failed")
	}

	var c interface{} = decimal64(2)
	if a == c {
		fail("decimal64 interface inequality failed")
	}

	// decimal64 and decimal128 in interface are different types
	var d64 interface{} = decimal64(1)
	var d128 interface{} = decimal128(1)
	if d64 == d128 {
		fail("decimal64 and decimal128 should not be equal in interface")
	}
}

// --- Overflow / underflow ---

func testOverflowUnderflow() {
	// decimal64 overflow: max coefficient is 9999999999999999 * 10^369
	// Multiplying large values should produce +Inf.
	big64 := decimal64(9e384)
	result64 := big64 * decimal64(2)
	if !math.IsDecimal64Inf(result64, 1) {
		fail("decimal64 overflow: 9e384 * 2 should be +Inf")
	}

	// decimal64 negative overflow
	negbig64 := decimal64(-9e384)
	result64neg := negbig64 * decimal64(2)
	if !math.IsDecimal64Inf(result64neg, -1) {
		fail("decimal64 overflow: -9e384 * 2 should be -Inf")
	}

	// decimal64 underflow: very small values multiplied should produce zero
	// decimal64 min subnormal is ~1e-398; multiplying two tiny values below that range gives zero.
	tiny64 := decimal64(1e-398)
	result64tiny := tiny64 * tiny64
	if result64tiny != decimal64(0) {
		fail("decimal64 underflow: 1e-398 * 1e-398 should be 0")
	}

	// decimal128 overflow: max exponent is ~6144
	big128 := decimal128(9e6144)
	result128 := big128 * decimal128(2)
	if !math.IsDecimal128Inf(result128, 1) {
		fail("decimal128 overflow: 9e6144 * 2 should be +Inf")
	}

	// decimal128 negative overflow
	negbig128 := decimal128(-9e6144)
	result128neg := negbig128 * decimal128(2)
	if !math.IsDecimal128Inf(result128neg, -1) {
		fail("decimal128 overflow: -9e6144 * 2 should be -Inf")
	}
}

// --- Decimal128 Infinity ---

func testInfinity128() {
	inf := math.Decimal128Inf(1)
	ninf := math.Decimal128Inf(-1)

	// Construction checks
	if !math.IsDecimal128Inf(inf, 1) {
		fail("decimal128 Decimal128Inf(1) not detected as +Inf")
	}
	if !math.IsDecimal128Inf(ninf, -1) {
		fail("decimal128 Decimal128Inf(-1) not detected as -Inf")
	}
	if !math.IsDecimal128Inf(inf, 0) {
		fail("decimal128 Decimal128Inf(1) not detected as Inf (either sign)")
	}

	// Inf comparisons
	if !(inf > decimal128(999999)) {
		fail("decimal128 +Inf > 999999 failed")
	}
	if !(ninf < decimal128(-999999)) {
		fail("decimal128 -Inf < -999999 failed")
	}
	if inf != inf {
		fail("decimal128 +Inf == +Inf failed")
	}
	if !(ninf < inf) {
		fail("decimal128 -Inf < +Inf failed")
	}

	// Inf arithmetic
	if inf+decimal128(1) != inf {
		fail("decimal128 Inf + 1 != Inf")
	}
	if inf*decimal128(2) != inf {
		fail("decimal128 Inf * 2 != Inf")
	}
	if !math.IsDecimal128NaN(inf + ninf) {
		fail("decimal128 +Inf + -Inf should be NaN")
	}

	// NaN propagation with infinity
	nan128 := math.Decimal128NaN()
	if !math.IsDecimal128NaN(inf + nan128) {
		fail("decimal128 Inf + NaN should be NaN")
	}
	if !math.IsDecimal128NaN(nan128 * inf) {
		fail("decimal128 NaN * Inf should be NaN")
	}
}

func main() {
	testLiterals()
	testArithmetic()
	testExactArithmetic()
	testComparisons()
	testNaN()
	testNegativeZero()
	testIntConversions()
	testFloatConversions()
	testDecimalWidening()
	testMapKeys()
	testConstants()
	testInfinity()
	testOverflowUnderflow()
	testInfinity128()
	testNamedTypes()
	testStructs()
	testSliceArray()
	testInterface()

	if bad {
		panic("decimal tests failed")
	}
}
