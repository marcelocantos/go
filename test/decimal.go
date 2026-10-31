// run

// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Test decimal64 and decimal128 types: literals, arithmetic,
// comparisons, conversions, map keys, and NaN/zero semantics.

package main

import (
	"math"
	"math/big"
	"strconv"
)

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

	// Division: 10/3 is 3.333333333333333 (16 digits, rounded down), so the
	// result is the encoding of 3333333333333333E-15 exactly.
	quot := a / b
	if got := math.Decimal64bits(quot); got != enc64(false, 3333333333333333, -15) {
		failBits64("decimal64 10/3 != 3.333333333333333", got, enc64(false, 3333333333333333, -15))
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
	if got, want := bits128(a128/b128), enc128(false, "3333333333333333333333333333333333", -33); got != want {
		failBits128("decimal128 10/3 != 3.333333333333333333333333333333333", got, want)
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

// --- IEEE 754-2008 results, checked by bit pattern ---
//
// The tables below hold, for each operation, the encoding of the result that
// IEEE 754-2008 requires: the exactly rounded value (ties to even) at the
// exponent of clauses 5.2 and 7. Each expectation is written as a sign, a
// coefficient and an exponent, derived by hand from the exact value of the
// operation, and cross-checked with Python's decimal module (the General
// Decimal Arithmetic specification, with the exponent range clamped to the
// IEEE one). None comes from the implementation under test. Operands are
// read from table fields, so the operators run in the runtime rather than
// folding in the compiler, and results are compared as encodings so that the
// quantum (1.50 against 1.5) is checked too, not just the value.

// d128bits is the encoding of a decimal128 as math.Decimal128bits returns it.
type d128bits struct{ hi, lo uint64 }

// enc64 is the BID encoding of (-1)^neg × coeff × 10^exp.
func enc64(neg bool, coeff uint64, exp int) uint64 {
	const bias = 398
	if coeff > 9999999999999999 || exp < -398 || exp > 369 {
		panic("enc64: out of range")
	}
	var bits uint64
	if coeff < 1<<53 {
		bits = uint64(exp+bias)<<53 | coeff
	} else {
		bits = 3<<61 | uint64(exp+bias)<<51 | coeff&(1<<51-1)
	}
	if neg {
		bits |= 1 << 63
	}
	return bits
}

// enc128 is the BID encoding of (-1)^neg × coeff × 10^exp, with coeff written
// in decimal. Every decimal128 coefficient is below 2^113, which the first
// encoding form holds.
func enc128(neg bool, coeff string, exp int) d128bits {
	const bias = 6176
	c, ok := new(big.Int).SetString(coeff, 10)
	if !ok || len(coeff) > 34 || exp < -6176 || exp > 6111 {
		panic("enc128: out of range " + coeff)
	}
	lo := new(big.Int).And(c, new(big.Int).SetUint64(1<<64-1)).Uint64()
	hi := new(big.Int).Rsh(c, 64).Uint64() | uint64(exp+bias)<<49
	if neg {
		hi |= 1 << 63
	}
	return d128bits{hi, lo}
}

const (
	pinf64bits uint64 = 0x7800000000000000
	ninf64bits uint64 = 0xf800000000000000
	nan64bits  uint64 = 0x7c00000000000000
)

var (
	pinf128bits = d128bits{0x7800000000000000, 0}
	ninf128bits = d128bits{0xf800000000000000, 0}
	nan128bits  = d128bits{0x7c00000000000000, 0}

	pinf64  = math.Decimal64frombits(pinf64bits)
	ninf64  = math.Decimal64frombits(ninf64bits)
	nan64   = math.Decimal64frombits(nan64bits)
	nzero64 = math.Decimal64frombits(enc64(true, 0, 0))

	// Go constants have no negative zero, so -0.0 and -0.00 are built here.
	nzeroE1_64 = math.Decimal64frombits(enc64(true, 0, -1))
	nzeroE2_64 = math.Decimal64frombits(enc64(true, 0, -2))

	pinf128  = math.Decimal128frombits(pinf128bits.hi, pinf128bits.lo)
	ninf128  = math.Decimal128frombits(ninf128bits.hi, ninf128bits.lo)
	nan128   = math.Decimal128frombits(nan128bits.hi, nan128bits.lo)
	nzero128 = frombits128(enc128(true, "0", 0))

	// Zeros whose exponents lie outside decimal64's range.
	zeroE1000  = frombits128(enc128(false, "0", 1000))
	zeroEm1000 = frombits128(enc128(false, "0", -1000))
)

func frombits128(b d128bits) decimal128 { return math.Decimal128frombits(b.hi, b.lo) }

func bits128(d decimal128) d128bits {
	hi, lo := math.Decimal128bits(d)
	return d128bits{hi, lo}
}

func hex(x uint64) string { return "0x" + strconv.FormatUint(x, 16) }

func failBits64(name string, got, want uint64) {
	fail(name + ": got " + hex(got) + " want " + hex(want))
}

func failBits128(name string, got, want d128bits) {
	fail(name + ": got hi " + hex(got.hi) + " lo " + hex(got.lo) + " want hi " + hex(want.hi) + " lo " + hex(want.lo))
}

// testEncoders checks enc64 and enc128 against encodings computed
// independently (by Python), so that a mistake in an encoder cannot hide a
// matching mistake in the implementation.
func testEncoders() {
	for _, c := range []struct {
		name string
		got  uint64
		want uint64
	}{
		{"enc64 1", enc64(false, 1, 0), 0x31c0000000000001},
		{"enc64 -max", enc64(true, 9999999999999999, 369), 0xf7fb86f26fc0ffff},
		{"enc64 1.000000000000001E+17", enc64(false, 1000000000000001, 2), 0x32038d7ea4c68001},
		{"enc64 -0E-398", enc64(true, 0, -398), 0x8000000000000000},
	} {
		if c.got != c.want {
			failBits64(c.name, c.got, c.want)
		}
	}
	for _, c := range []struct {
		name      string
		got, want d128bits
	}{
		{"enc128 1", enc128(false, "1", 0), d128bits{0x3040000000000000, 1}},
		{"enc128 -max", enc128(true, "9999999999999999999999999999999999", 6111), d128bits{0xdfffed09bead87c0, 0x378d8e63ffffffff}},
		{"enc128 12345678901234567890123456789.01234", enc128(false, "1234567890123456789012345678901234", -5), d128bits{0x30363cde6fff9732, 0xde825cd07e96aff2}},
	} {
		if c.got != c.want {
			failBits128(c.name, c.got, c.want)
		}
	}
}

type arithCase64 struct {
	name string
	a    decimal64
	op   byte
	b    decimal64
	want uint64
}

type arithCase128 struct {
	name string
	a    decimal128
	op   byte
	b    decimal128
	want d128bits
}

//go:noinline
func apply64(a decimal64, op byte, b decimal64) decimal64 {
	switch op {
	case '+':
		return a + b
	case '-':
		return a - b
	case '*':
		return a * b
	case '/':
		return a / b
	}
	panic("bad op")
}

//go:noinline
func apply128(a decimal128, op byte, b decimal128) decimal128 {
	switch op {
	case '+':
		return a + b
	case '-':
		return a - b
	case '*':
		return a * b
	case '/':
		return a / b
	}
	panic("bad op")
}

func testIEEEArithmetic() {
	for _, c := range arith64 {
		if got := math.Decimal64bits(apply64(c.a, c.op, c.b)); got != c.want {
			failBits64("decimal64 "+c.name, got, c.want)
		}
	}
	for _, c := range arith128 {
		if got := bits128(apply128(c.a, c.op, c.b)); got != c.want {
			failBits128("decimal128 "+c.name, got, c.want)
		}
	}
}

// The compound assignment forms take the same paths as the binary operators
// but are lowered separately.
func testIEEECompound() {
	x := decimal64(1234567890123455)
	x += 0.5
	if got := math.Decimal64bits(x); got != enc64(false, 1234567890123456, 0) {
		failBits64("decimal64 x += 0.5 tie", got, enc64(false, 1234567890123456, 0))
	}
	y := decimal64(1)
	y /= 3
	if got := math.Decimal64bits(y); got != enc64(false, 3333333333333333, -16) {
		failBits64("decimal64 y /= 3", got, enc64(false, 3333333333333333, -16))
	}
	z := decimal128(2)
	z *= 0.25
	if got := bits128(z); got != enc128(false, "50", -2) {
		failBits128("decimal128 z *= 0.25", got, enc128(false, "50", -2))
	}
}

var arith64 = []arithCase64{
	{"tie at 17th digit rounds down to even", 1234567890123456, '+', 0.5, enc64(false, 1234567890123456, 0)},
	{"tie at 17th digit rounds up to even", 1234567890123455, '+', 0.5, enc64(false, 1234567890123456, 0)},
	{"below a tie", 1234567890123456, '+', 0.4, enc64(false, 1234567890123456, 0)},
	{"above a tie", 1234567890123456, '+', 0.6, enc64(false, 1234567890123457, 0)},
	{"just above a tie (sticky digit)", 1234567890123456, '+', 0.5000000000000001, enc64(false, 1234567890123457, 0)},
	{"just below a tie", 1234567890123456, '+', 0.4999999999999999, enc64(false, 1234567890123456, 0)},
	{"negative tie, odd to even", -1234567890123455, '-', 0.5, enc64(true, 1234567890123456, 0)},
	{"negative tie, even stays", -1234567890123456, '-', 0.5, enc64(true, 1234567890123456, 0)},
	{"carry into a new digit", 9999999999999999, '+', 0.5, enc64(false, 1000000000000000, 1)},
	{"tie at the top, even stays", 9999999999999998, '+', 0.5, enc64(false, 9999999999999998, 0)},
	{"below the carry", 9999999999999999, '+', 0.4, enc64(false, 9999999999999999, 0)},
	{"negative carry", -9999999999999999, '-', 0.5, enc64(true, 1000000000000000, 1)},
	{"product tie rounds down to even", 2500000000000001, '*', 0.5, enc64(false, 1250000000000000, 0)},
	{"product tie rounds up to even", 2500000000000003, '*', 0.5, enc64(false, 1250000000000002, 0)},
	{"quotient tie rounds down to even", 2500000000000001, '/', 2, enc64(false, 1250000000000000, 0)},
	{"quotient tie rounds up to even", 2500000000000003, '/', 2, enc64(false, 1250000000000002, 0)},
	{"10/3", 10, '/', 3, enc64(false, 3333333333333333, -15)},
	{"2/3 rounds up", 2, '/', 3, enc64(false, 6666666666666667, -16)},
	{"1/3000", 1, '/', 3000, enc64(false, 3333333333333333, -19)},
	{"7/9999999999999999", 7, '/', 9999999999999999, enc64(false, 7000000000000001, -31)},
	{"1E300/3", 1e300, '/', 3, enc64(false, 3333333333333333, 284)},
	{"1.00/3 is inexact, 16 digits", 1.00, '/', 3, enc64(false, 3333333333333333, -16)},
	{"inexact product, 16 digits", 1.100000000000001, '*', 1.100000000000001, enc64(false, 1210000000000002, -15)},
	{"inexact sum 1+1e-20", 1, '+', 1e-20, enc64(false, 1000000000000000, -15)},
	{"inexact sum 100.00+1e-30", 100.00, '+', 1e-30, enc64(false, 1000000000000000, -13)},
	{"2.50*1.1 exact at Q(x)+Q(y)", 2.50, '*', 1.1, enc64(false, 2750, -3)},
	{"1.00/1.0 exact at Q(x)-Q(y)", 1.00, '/', 1.0, enc64(false, 10, -1)},
	{"1.50+1.5 exact at min(Q)", 1.50, '+', 1.5, enc64(false, 300, -2)},
	{"29.90*3 exact", 29.90, '*', 3, enc64(false, 8970, -2)},
	{"1.50-0.5 exact", 1.50, '-', 0.5, enc64(false, 100, -2)},
	{"0.10*10 exact", 0.10, '*', 10, enc64(false, 100, -2)},
	{"100/8 exact, preferred exponent 0 unreachable", 100, '/', 8, enc64(false, 125, -1)},
	{"1.00/4 exact", 1.00, '/', 4, enc64(false, 25, -2)},
	{"max + half ulp is +Inf (tie to even)", 9999999999999999e369, '+', 5e368, pinf64bits},
	{"max + just under half ulp stays max", 9999999999999999e369, '+', 4999999999999999e353, enc64(false, 9999999999999999, 369)},
	{"max + 5e353 stays max", 9999999999999999e369, '+', 5e353, enc64(false, 9999999999999999, 369)},
	{"max + 6e368 is +Inf", 9999999999999999e369, '+', 6e368, pinf64bits},
	{"-max - half ulp is -Inf", -9999999999999999e369, '-', 5e368, ninf64bits},
	{"-max - just under half ulp stays -max", -9999999999999999e369, '-', 4999999999999999e353, enc64(true, 9999999999999999, 369)},
	{"max-1ulp + half ulp ties to even (down)", 9999999999999998e369, '+', 5e368, enc64(false, 9999999999999998, 369)},
	{"max-1ulp + 6e368 rounds up", 9999999999999998e369, '+', 6e368, enc64(false, 9999999999999999, 369)},
	{"max + max", 9999999999999999e369, '+', 9999999999999999e369, pinf64bits},
	{"max * 1", 9999999999999999e369, '*', 1, enc64(false, 9999999999999999, 369)},
	{"max * 10", 9999999999999999e369, '*', 10, pinf64bits},
	{"5e384 * 2 is 1e385", 5000000000000000e369, '*', 2, pinf64bits},
	{"9e384 / 0.5", 9e384, '/', 0.5, pinf64bits},
	{"-max * 10", -9999999999999999e369, '*', 10, ninf64bits},
	{"exponent clamp: 1e384 + 0 is exact", 1e384, '+', 0, enc64(false, 1000000000000000, 369)},
	{"smallest subnormal * 1", 1e-398, '*', 1, enc64(false, 1, -398)},
	{"smallest subnormal doubled", 1e-398, '+', 1e-398, enc64(false, 2, -398)},
	{"1.5 ulp tie rounds up to even", 3e-398, '*', 0.5, enc64(false, 2, -398)},
	{"2.5 ulp tie rounds down to even", 5e-398, '*', 0.5, enc64(false, 2, -398)},
	{"3.5 ulp tie rounds up to even", 7e-398, '*', 0.5, enc64(false, 4, -398)},
	{"inexact subnormal 1.2 ulp", 3e-398, '*', 0.4, enc64(false, 1, -398)},
	{"gradual underflow to zero", 1e-398, '*', 0.4, enc64(false, 0, -398)},
	{"half the smallest subnormal ties to 0", 1e-398, '*', 0.5, enc64(false, 0, -398)},
	{"0.6 of the smallest subnormal", 1e-398, '*', 0.6, enc64(false, 1, -398)},
	{"underflow to -0 (negative operand)", -1e-398, '*', 0.4, enc64(true, 0, -398)},
	{"underflow to -0 (negative factor)", 1e-398, '*', -0.4, enc64(true, 0, -398)},
	{"subnormal tie rounds up to the smallest normal", 9999999999999995e-398, '*', 0.1, enc64(false, 1000000000000000, -398)},
	{"subnormal above a tie rounds up to the smallest normal", 9999999999999996e-398, '*', 0.1, enc64(false, 1000000000000000, -398)},
	{"subnormal below a tie stays subnormal", 9999999999999994e-398, '*', 0.1, enc64(false, 999999999999999, -398)},
	{"largest subnormal + smallest", 999999999999999e-398, '+', 1e-398, enc64(false, 1000000000000000, -398)},
	{"smallest normal - smallest subnormal", 1000000000000000e-398, '-', 1e-398, enc64(false, 999999999999999, -398)},
	{"exact subnormal product keeps Q(x)+Q(y)", 1e-383, '*', 0.1, enc64(false, 1, -384)},
	{"exact product below MinExp is shortened", 100e-398, '*', 0.1, enc64(false, 10, -398)},
	{"subnormal quotient rounds up", 5e-390, '/', 3e7, enc64(false, 17, -398)},
	{"subnormal quotient rounds down", 1e-390, '/', 3e7, enc64(false, 3, -398)},
	{"quotient underflows to zero", 1e-390, '/', 1e10, enc64(false, 0, -398)},
	{"negative quotient underflows to -0", -1e-390, '/', 1e10, enc64(true, 0, -398)},
	{"1e20 + 1e-20", 1e20, '+', 1e-20, enc64(false, 1000000000000000, 5)},
	{"1e-300 + 1 (small operand first)", 1e-300, '+', 1, enc64(false, 1000000000000000, -15)},
	{"1 + 1e-300", 1, '+', 1e-300, enc64(false, 1000000000000000, -15)},
	{"1e300 + 1e-300", 1e300, '+', 1e-300, enc64(false, 1000000000000000, 285)},
	{"1 - 1e-40 rounds up to 1", 1, '-', 1e-40, enc64(false, 1000000000000000, -15)},
	{"1 - 1e-16 is exact", 1, '-', 1e-16, enc64(false, 9999999999999999, -16)},
	{"1 - 1e-17 rounds up to 1", 1, '-', 1e-17, enc64(false, 1000000000000000, -15)},
	{"1 + 5e-16 tie, even stays", 1, '+', 5e-16, enc64(false, 1000000000000000, -15)},
	{"1 + 1.5e-15 tie, odd rounds up", 1, '+', 1.5e-15, enc64(false, 1000000000000002, -15)},
	{"-1 - 1e-20", -1, '-', 1e-20, enc64(true, 1000000000000000, -15)},
	{"-1 + 1e-40 rounds to -1", -1, '+', 1e-40, enc64(true, 1000000000000000, -15)},
	{"0/0 is NaN", 0, '/', 0, nan64bits},
	{"1/0 is +Inf", 1, '/', 0, pinf64bits},
	{"-1/0 is -Inf", -1, '/', 0, ninf64bits},
	{"1/-0 is -Inf", 1, '/', nzero64, ninf64bits},
	{"-1/-0 is +Inf", -1, '/', nzero64, pinf64bits},
	{"NaN/0 is NaN", nan64, '/', 0, nan64bits},
	{"0/5 keeps Q(x)-Q(y)", 0, '/', 5, enc64(false, 0, 0)},
	{"0/-5 is -0", 0, '/', -5, enc64(true, 0, 0)},
	{"0.00/5 is 0E-2", 0.00, '/', 5, enc64(false, 0, -2)},
	{"5/Inf is 0 at MinExp", 5, '/', pinf64, enc64(false, 0, -398)},
	{"-5/Inf is -0 at MinExp", -5, '/', pinf64, enc64(true, 0, -398)},
	{"5/-Inf is -0 at MinExp", 5, '/', ninf64, enc64(true, 0, -398)},
	{"Inf/5", pinf64, '/', 5, pinf64bits},
	{"Inf/-5", pinf64, '/', -5, ninf64bits},
	{"Inf/Inf is NaN", pinf64, '/', pinf64, nan64bits},
	{"Inf-Inf is NaN", pinf64, '-', pinf64, nan64bits},
	{"Inf+-Inf is NaN", pinf64, '+', ninf64, nan64bits},
	{"Inf+Inf", pinf64, '+', pinf64, pinf64bits},
	{"-Inf-Inf", ninf64, '-', pinf64, ninf64bits},
	{"Inf-(-Inf)", pinf64, '-', ninf64, pinf64bits},
	{"0*Inf is NaN", 0, '*', pinf64, nan64bits},
	{"Inf*0 is NaN", pinf64, '*', 0, nan64bits},
	{"-0*Inf is NaN", nzero64, '*', pinf64, nan64bits},
	{"Inf*-2", pinf64, '*', -2, ninf64bits},
	{"Inf*Inf", pinf64, '*', pinf64, pinf64bits},
	{"-Inf*Inf", ninf64, '*', pinf64, ninf64bits},
	{"1+NaN", 1, '+', nan64, nan64bits},
	{"NaN*1", nan64, '*', 1, nan64bits},
	{"Inf+1 is exact", pinf64, '+', 1, pinf64bits},
	{"x-x is +0 at the smaller exponent (1.5-1.5)", 1.5, '-', 1.5, enc64(false, 0, -1)},
	{"x-x is +0 at the smaller exponent (1.50-1.5)", 1.50, '-', 1.5, enc64(false, 0, -2)},
	{"x-x is +0 at the smaller exponent (1.5-1.50)", 1.5, '-', 1.50, enc64(false, 0, -2)},
	{"-x+x is +0", -1.5, '+', 1.5, enc64(false, 0, -1)},
	{"(-x)-(-x) is +0", -1.5, '-', -1.5, enc64(false, 0, -1)},
	{"-0 + -0 is -0", nzero64, '+', nzero64, enc64(true, 0, 0)},
	{"0 + -0 is +0", 0, '+', nzero64, enc64(false, 0, 0)},
	{"-0 + 0 is +0", nzero64, '+', 0, enc64(false, 0, 0)},
	{"-0 - 0 is -0", nzero64, '-', 0, enc64(true, 0, 0)},
	{"0 - 0 is +0", 0, '-', 0, enc64(false, 0, 0)},
	{"0 - -0 is +0", 0, '-', nzero64, enc64(false, 0, 0)},
	{"-0 - -0 is +0", nzero64, '-', nzero64, enc64(false, 0, 0)},
	{"0.0 + 0.00 is 0E-2", 0.0, '+', 0.00, enc64(false, 0, -2)},
	{"-0.0 + -0.00 is -0E-2", nzeroE1_64, '+', nzeroE2_64, enc64(true, 0, -2)},
	{"-0 * 5 is -0", nzero64, '*', 5, enc64(true, 0, 0)},
	{"-0 * -5 is +0", nzero64, '*', -5, enc64(false, 0, 0)},
	{"0 * -5 is -0", 0, '*', -5, enc64(true, 0, 0)},
	{"0 * 0.01 is 0E-2", 0, '*', 0.01, enc64(false, 0, -2)},
}

var arith128 = []arithCase128{
	{"tie at 35th digit rounds down to even", 1234567890123456789012345678901234, '+', 0.5, enc128(false, "1234567890123456789012345678901234", 0)},
	{"tie at 35th digit rounds up to even", 1234567890123456789012345678901233, '+', 0.5, enc128(false, "1234567890123456789012345678901234", 0)},
	{"below a tie", 1234567890123456789012345678901234, '+', 0.4, enc128(false, "1234567890123456789012345678901234", 0)},
	{"above a tie", 1234567890123456789012345678901234, '+', 0.6, enc128(false, "1234567890123456789012345678901235", 0)},
	{"just above a tie (sticky digit)", 1234567890123456789012345678901234, '+', 0.5000000000000000000000000000000001, enc128(false, "1234567890123456789012345678901235", 0)},
	{"just below a tie", 1234567890123456789012345678901234, '+', 0.4999999999999999999999999999999999, enc128(false, "1234567890123456789012345678901234", 0)},
	{"negative tie, odd to even", -1234567890123456789012345678901233, '-', 0.5, enc128(true, "1234567890123456789012345678901234", 0)},
	{"carry into a new digit", 9999999999999999999999999999999999, '+', 0.5, enc128(false, "1000000000000000000000000000000000", 1)},
	{"negative carry", -9999999999999999999999999999999999, '-', 0.5, enc128(true, "1000000000000000000000000000000000", 1)},
	{"product tie rounds down to even", 2500000000000000000000000000000001, '*', 0.5, enc128(false, "1250000000000000000000000000000000", 0)},
	{"product tie rounds up to even", 2500000000000000000000000000000003, '*', 0.5, enc128(false, "1250000000000000000000000000000002", 0)},
	{"quotient tie rounds down to even", 2500000000000000000000000000000001, '/', 2, enc128(false, "1250000000000000000000000000000000", 0)},
	{"quotient tie rounds up to even", 2500000000000000000000000000000003, '/', 2, enc128(false, "1250000000000000000000000000000002", 0)},
	{"1/3", 1, '/', 3, enc128(false, "3333333333333333333333333333333333", -34)},
	{"2/3 rounds up", 2, '/', 3, enc128(false, "6666666666666666666666666666666667", -34)},
	{"10/3", 10, '/', 3, enc128(false, "3333333333333333333333333333333333", -33)},
	{"1.00/3 is inexact, 34 digits", 1.00, '/', 3, enc128(false, "3333333333333333333333333333333333", -34)},
	{"inexact sum 1e20+1e-20", 1e20, '+', 1e-20, enc128(false, "1000000000000000000000000000000000", -13)},
	{"inexact sum 100.00+1e-50", 100.00, '+', 1e-50, enc128(false, "1000000000000000000000000000000000", -31)},
	{"2.50*1.1 exact", 2.50, '*', 1.1, enc128(false, "2750", -3)},
	{"1.00/1.0 exact", 1.00, '/', 1.0, enc128(false, "10", -1)},
	{"1.50+1.5 exact", 1.50, '+', 1.5, enc128(false, "300", -2)},
	{"29.90*3 exact", 29.90, '*', 3, enc128(false, "8970", -2)},
	{"exact sum filling 34 digits", 1, '+', 1e-33, enc128(false, "1000000000000000000000000000000001", -33)},
	{"1 + 1e-34 rounds to 1", 1, '+', 1e-34, enc128(false, "1000000000000000000000000000000000", -33)},
	{"1 + 5e-34 tie, even stays", 1, '+', 5e-34, enc128(false, "1000000000000000000000000000000000", -33)},
	{"1 + 15e-34 tie, odd rounds up", 1, '+', 15e-34, enc128(false, "1000000000000000000000000000000002", -33)},
	{"1 - 1e-40 rounds up to 1", 1, '-', 1e-40, enc128(false, "1000000000000000000000000000000000", -33)},
	{"1 - 1e-33 exact", 1, '-', 1e-33, enc128(false, "999999999999999999999999999999999", -33)},
	{"1 - 1e-35 rounds up to 1", 1, '-', 1e-35, enc128(false, "1000000000000000000000000000000000", -33)},
	{"1e19 + 1e-19", 1e19, '+', 1e-19, enc128(false, "1000000000000000000000000000000000", -14)},
	{"1e300 + 1e-300", 1e300, '+', 1e-300, enc128(false, "1000000000000000000000000000000000", 267)},
	{"sum from the audit", 999999999999999999999999999e8, '+', 6118384908704389844375805015e-14, enc128(false, "1000000000000000000000611837490870", 2)},
	{"product from the audit", 79370027447169958363511204e14, '*', -99999999999999999999999999e9, enc128(true, "7937002744716995836351120320629973", 41)},
	{"quotient from the audit", 234978520688775e-17, '/', 9047123382365178798256528742811e-4801, enc128(false, "2597273307301185964404701320706016", 4734)},
	{"max + half ulp is +Inf", 9999999999999999999999999999999999e6111, '+', 5e6110, pinf128bits},
	{"max + just under half ulp stays max", 9999999999999999999999999999999999e6111, '+', 4999999999999999999999999999999999e6077, enc128(false, "9999999999999999999999999999999999", 6111)},
	{"-max - half ulp is -Inf", -9999999999999999999999999999999999e6111, '-', 5e6110, ninf128bits},
	{"max-1ulp + half ulp ties to even", 9999999999999999999999999999999998e6111, '+', 5e6110, enc128(false, "9999999999999999999999999999999998", 6111)},
	{"max + max", 9999999999999999999999999999999999e6111, '+', 9999999999999999999999999999999999e6111, pinf128bits},
	{"max * 10", 9999999999999999999999999999999999e6111, '*', 10, pinf128bits},
	{"1e6144 * 10 is +Inf", 1e6144, '*', 10, pinf128bits},
	{"1e6144 * 1 is finite (clamped)", 1e6144, '*', 1, enc128(false, "1000000000000000000000000000000000", 6111)},
	{"1e6144 + 1e6144 is finite", 1e6144, '+', 1e6144, enc128(false, "2000000000000000000000000000000000", 6111)},
	{"9e6144 / 2 is finite", 9e6144, '/', 2, enc128(false, "4500000000000000000000000000000000", 6111)},
	{"9e6144 / 0.5 is +Inf", 9e6144, '/', 0.5, pinf128bits},
	{"1e6138 / 1e-9 is +Inf", 1e6138, '/', 1e-9, pinf128bits},
	{"smallest subnormal * 1", 1e-6176, '*', 1, enc128(false, "1", -6176)},
	{"1.5 ulp tie rounds up to even", 3e-6176, '*', 0.5, enc128(false, "2", -6176)},
	{"2.5 ulp tie rounds down to even", 5e-6176, '*', 0.5, enc128(false, "2", -6176)},
	{"3.5 ulp tie rounds up to even", 7e-6176, '*', 0.5, enc128(false, "4", -6176)},
	{"gradual underflow to zero", 1e-6176, '*', 0.4, enc128(false, "0", -6176)},
	{"underflow to -0", -1e-6176, '*', 0.4, enc128(true, "0", -6176)},
	{"0.6 of the smallest subnormal", 1e-6176, '*', 0.6, enc128(false, "1", -6176)},
	{"subnormal tie rounds up to the smallest normal", 9999999999999999999999999999999995e-6176, '*', 0.1, enc128(false, "1000000000000000000000000000000000", -6176)},
	{"subnormal below a tie stays subnormal", 9999999999999999999999999999999994e-6176, '*', 0.1, enc128(false, "999999999999999999999999999999999", -6176)},
	{"quotient underflows to zero", 1e-6170, '/', 1e10, enc128(false, "0", -6176)},
	{"0/0 is NaN", 0, '/', 0, nan128bits},
	{"1/0 is +Inf", 1, '/', 0, pinf128bits},
	{"-1/0 is -Inf", -1, '/', 0, ninf128bits},
	{"1/-0 is -Inf", 1, '/', nzero128, ninf128bits},
	{"5/Inf is 0 at MinExp", 5, '/', pinf128, enc128(false, "0", -6176)},
	{"-5/Inf is -0 at MinExp", -5, '/', pinf128, enc128(true, "0", -6176)},
	{"Inf-Inf is NaN", pinf128, '-', pinf128, nan128bits},
	{"0*Inf is NaN", 0, '*', pinf128, nan128bits},
	{"Inf*-2", pinf128, '*', -2, ninf128bits},
	{"Inf/Inf is NaN", pinf128, '/', pinf128, nan128bits},
	{"x-x is +0 at the smaller exponent", 1.50, '-', 1.5, enc128(false, "0", -2)},
	{"-0 + -0 is -0", nzero128, '+', nzero128, enc128(true, "0", 0)},
	{"0 + -0 is +0", 0, '+', nzero128, enc128(false, "0", 0)},
	{"-0 - 0 is -0", nzero128, '-', 0, enc128(true, "0", 0)},
	{"-0 * 5 is -0", nzero128, '*', 5, enc128(true, "0", 0)},
}

var int64ToDec64 = []intCase64[int64]{
	{"zero", 0, enc64(false, 0, 0)},
	{"100", 100, enc64(false, 100, 0)},
	{"-42", -42, enc64(true, 42, 0)},
	{"16 digits", 9999999999999999, enc64(false, 9999999999999999, 0)},
	{"1e16 needs exponent 1", 10000000000000000, enc64(false, 1000000000000000, 1)},
	{"audit: 100000000000000149 rounds down", 100000000000000149, enc64(false, 1000000000000001, 2)},
	{"tie rounds up to even", 100000000000000150, enc64(false, 1000000000000002, 2)},
	{"tie rounds down to even", 100000000000000250, enc64(false, 1000000000000002, 2)},
	{"above a tie", 100000000000000151, enc64(false, 1000000000000002, 2)},
	{"negative below a tie", -100000000000000149, enc64(true, 1000000000000001, 2)},
	{"17 digits tie, even stays", 12345678901234565, enc64(false, 1234567890123456, 1)},
	{"17 digits tie, odd rounds up", 12345678901234575, enc64(false, 1234567890123458, 1)},
	{"MaxInt64", math.MaxInt64, enc64(false, 9223372036854776, 3)},
	{"MinInt64", math.MinInt64, enc64(true, 9223372036854776, 3)},
}

var uint64ToDec64 = []intCase64[uint64]{
	{"zero", 0, enc64(false, 0, 0)},
	{"MaxUint64", math.MaxUint64, enc64(false, 1844674407370955, 4)},
	{"2^63", 1 << 63, enc64(false, 9223372036854776, 3)},
	{"1e19+5000 is a tie", 10000000000000005000, enc64(false, 1000000000000000, 4)},
	{"1e19+5001 is above a tie", 10000000000000005001, enc64(false, 1000000000000001, 4)},
}

var int64ToDec128 = []intCase128[int64]{
	{"MaxInt64", math.MaxInt64, enc128(false, "9223372036854775807", 0)},
	{"MinInt64", math.MinInt64, enc128(true, "9223372036854775808", 0)},
	{"zero", 0, enc128(false, "0", 0)},
	{"-1", -1, enc128(true, "1", 0)},
}

var uint64ToDec128 = []intCase128[uint64]{
	{"MaxUint64", math.MaxUint64, enc128(false, "18446744073709551615", 0)},
	{"zero", 0, enc128(false, "0", 0)},
}

var float64ToDec64 = []floatCase64[float64]{
	{"0.1 gives 0.1000000000000000", 0.1, enc64(false, 1000000000000000, -16)},
	{"0.5", 0.5, enc64(false, 5, -1)},
	{"2", 2.0, enc64(false, 2, 0)},
	{"3.14", 3.14, enc64(false, 3140000000000000, -15)},
	{"1e23 is 99999999999999991611392", 1e23, enc64(false, 9999999999999999, 7)},
	{"1e20", 1e20, enc64(false, 1000000000000000, 5)},
	{"-0", math.Float64frombits(1 << 63), enc64(true, 0, 0)},
	{"MaxFloat64", math.MaxFloat64, enc64(false, 1797693134862316, 293)},
}

var float64ToDec128 = []floatCase128[float64]{
	{"0.1 gives 34 digits", 0.1, enc128(false, "1000000000000000055511151231257827", -34)},
	{"0.5", 0.5, enc128(false, "5", -1)},
	{"1e20 is exact", 1e20, enc128(false, "100000000000000000000", 0)},
	{"1e23 is exact", 1e23, enc128(false, "99999999999999991611392", 0)},
	{"-0", math.Float64frombits(1 << 63), enc128(true, "0", 0)},
	{"MaxFloat64", math.MaxFloat64, enc128(false, "1797693134862315708145274237317044", 275)},
}

var float32ToDec64 = []floatCase64[float32]{
	{"float32(0.1)", 0.1, enc64(false, 1000000014901161, -16)},
	{"float32(0.5)", 0.5, enc64(false, 5, -1)},
	{"float32(16777217) is 16777216", 16777217, enc64(false, 16777216, 0)},
}

var float32ToDec128 = []floatCase128[float32]{
	{"float32(0.1) is exact", 0.1, enc128(false, "100000001490116119384765625", -27)},
	{"float32(0.5)", 0.5, enc128(false, "5", -1)},
}

var dec64ToFloat64 = []toFloatCase64[uint64]{
	{"1e36 (audit)", 1e36, 0x476812f9cf7920e3},
	{"1e23 is a tie, rounds to even", 1e23, 0x44b52d02c7e14af6},
	{"0.1", 0.1, 0x3fb999999999999a},
	{"2^53+1 is a tie, rounds down to even", 9007199254740993, 0x4340000000000000},
	{"2^53+3 is a tie, rounds up to even", 9007199254740995, 0x4340000000000002},
	{"smallest subnormal", 5e-324, 0x1},
	{"just below half the smallest subnormal", 2.470328229206232e-324, 0x0},
	{"just above half the smallest subnormal", 2.470328229206233e-324, 0x1},
	{"underflow to +0", 1e-398, 0x0},
	{"underflow to -0", -1e-398, 0x8000000000000000},
	{"overflow to +Inf", 1e310, 0x7ff0000000000000},
	{"overflow to -Inf", -1e310, 0xfff0000000000000},
	{"past MaxFloat64 plus half an ulp", 1.797693134862316e308, 0x7ff0000000000000},
}

var dec128ToFloat64 = []toFloatCase128[uint64]{
	{"just below 1+2^-53, rounds to 1", 1.000000000000000111022302462515654, 0x3ff0000000000000},
	{"just above 1+2^-53, rounds up", 1.000000000000000111022302462515655, 0x3ff0000000000001},
	{"1e23 is a tie, rounds to even", 1e23, 0x44b52d02c7e14af6},
	{"0.1", 0.1, 0x3fb999999999999a},
}

var dec64ToFloat32 = []toFloatCase64[uint32]{
	{"16777217 is a tie, rounds to even", 16777217, 0x4b800000},
	{"16777219 is a tie, rounds to even", 16777219, 0x4b800002},
	{"0.1", 0.1, 0x3dcccccd},
	{"just below half the smallest subnormal", 7.006492321624085e-46, 0x0},
	{"just above half the smallest subnormal", 7.006492321624086e-46, 0x1},
	{"overflow to +Inf", 1e39, 0x7f800000},
	{"overflow to -Inf", -1e39, 0xff800000},
}

var dec128ToFloat32 = []toFloatCase128[uint32]{
	{"tie rounds to even", 16777217, 0x4b800000},
	{"a hair above a tie rounds up (double rounding through float64 would not)", 16777217.0000000000000000001, 0x4b800001},
	{"a hair below a tie rounds down", 16777216.9999999999999999999, 0x4b800000},
}

var narrowCases = []narrowCase{
	{"audit: 1769609755703054503235303742165E167", 1769609755703054503235303742165e167, enc64(false, 1769609755703055, 182)},
	{"17-digit tie, even stays", 12345678901234565, enc64(false, 1234567890123456, 1)},
	{"17-digit tie, odd rounds up", 12345678901234575, enc64(false, 1234567890123458, 1)},
	{"a hair above a tie", 1234567890123456500000000000000001, enc64(false, 1234567890123457, 18)},
	{"a hair below a tie", 1234567890123456499999999999999999, enc64(false, 1234567890123456, 18)},
	{"exact keeps its exponent", 1.50, enc64(false, 150, -2)},
	{"exponent above MaxExp pads the coefficient", 1e384, enc64(false, 1000000000000000, 369)},
	{"overflow", 1e400, pinf64bits},
	{"negative overflow", -1e400, ninf64bits},
	{"tie at the overflow threshold", 99999999999999995e368, pinf64bits},
	{"just below the overflow threshold", 99999999999999994999e365, enc64(false, 9999999999999999, 369)},
	{"underflow to +0", 1e-400, enc64(false, 0, -398)},
	{"underflow to -0", -1e-400, enc64(true, 0, -398)},
	{"subnormal tie rounds up to even", 15e-399, enc64(false, 2, -398)},
	{"subnormal tie rounds down to even", 25e-399, enc64(false, 2, -398)},
	{"smallest subnormal", 1e-398, enc64(false, 1, -398)},
	{"zero at exponent 1000 clamps to 369", zeroE1000, enc64(false, 0, 369)},
	{"zero at exponent -1000 clamps to -398", zeroEm1000, enc64(false, 0, -398)},
	{"+Inf", pinf128, pinf64bits},
	{"NaN", nan128, nan64bits},
}

var widenCases = []widenCase{
	{"1.50 keeps its exponent", 1.50, enc128(false, "150", -2)},
	{"0.00 keeps its exponent", 0.00, enc128(false, "0", -2)},
	{"max", 9999999999999999e369, enc128(false, "9999999999999999", 369)},
	{"smallest subnormal", 1e-398, enc128(false, "1", -398)},
	{"-0", nzero64, enc128(true, "0", 0)},
	{"-Inf", ninf64, ninf128bits},
	{"NaN", nan64, nan128bits},
}

// --- Conversions ---

type intCase64[T int64 | uint64] struct {
	name string
	v    T
	want uint64
}

type intCase128[T int64 | uint64] struct {
	name string
	v    T
	want d128bits
}

type floatCase64[T float32 | float64] struct {
	name string
	v    T
	want uint64
}

type floatCase128[T float32 | float64] struct {
	name string
	v    T
	want d128bits
}

type toFloatCase64[T uint32 | uint64] struct {
	name string
	d    decimal64
	want T
}

type toFloatCase128[T uint32 | uint64] struct {
	name string
	d    decimal128
	want T
}

type narrowCase struct {
	name string
	d    decimal128
	want uint64
}

type widenCase struct {
	name string
	d    decimal64
	want d128bits
}

//go:noinline
func i64to64(v int64) decimal64 { return decimal64(v) }

//go:noinline
func u64to64(v uint64) decimal64 { return decimal64(v) }

//go:noinline
func i64to128(v int64) decimal128 { return decimal128(v) }

//go:noinline
func u64to128(v uint64) decimal128 { return decimal128(v) }

//go:noinline
func f64to64(v float64) decimal64 { return decimal64(v) }

//go:noinline
func f64to128(v float64) decimal128 { return decimal128(v) }

//go:noinline
func f32to64(v float32) decimal64 { return decimal64(v) }

//go:noinline
func f32to128(v float32) decimal128 { return decimal128(v) }

//go:noinline
func d64tof64(d decimal64) float64 { return float64(d) }

//go:noinline
func d128tof64(d decimal128) float64 { return float64(d) }

//go:noinline
func d64tof32(d decimal64) float32 { return float32(d) }

//go:noinline
func d128tof32(d decimal128) float32 { return float32(d) }

//go:noinline
func narrow(d decimal128) decimal64 { return decimal64(d) }

//go:noinline
func widen(d decimal64) decimal128 { return decimal128(d) }

func testIEEEConversions() {
	for _, c := range int64ToDec64 {
		if got := math.Decimal64bits(i64to64(c.v)); got != c.want {
			failBits64("decimal64(int64) "+c.name, got, c.want)
		}
	}
	for _, c := range uint64ToDec64 {
		if got := math.Decimal64bits(u64to64(c.v)); got != c.want {
			failBits64("decimal64(uint64) "+c.name, got, c.want)
		}
	}
	for _, c := range int64ToDec128 {
		if got := bits128(i64to128(c.v)); got != c.want {
			failBits128("decimal128(int64) "+c.name, got, c.want)
		}
	}
	for _, c := range uint64ToDec128 {
		if got := bits128(u64to128(c.v)); got != c.want {
			failBits128("decimal128(uint64) "+c.name, got, c.want)
		}
	}
	for _, c := range float64ToDec64 {
		if got := math.Decimal64bits(f64to64(c.v)); got != c.want {
			failBits64("decimal64(float64) "+c.name, got, c.want)
		}
	}
	for _, c := range float64ToDec128 {
		if got := bits128(f64to128(c.v)); got != c.want {
			failBits128("decimal128(float64) "+c.name, got, c.want)
		}
	}
	for _, c := range float32ToDec64 {
		if got := math.Decimal64bits(f32to64(c.v)); got != c.want {
			failBits64("decimal64(float32) "+c.name, got, c.want)
		}
	}
	for _, c := range float32ToDec128 {
		if got := bits128(f32to128(c.v)); got != c.want {
			failBits128("decimal128(float32) "+c.name, got, c.want)
		}
	}
	for _, c := range dec64ToFloat64 {
		if got := math.Float64bits(d64tof64(c.d)); got != c.want {
			failBits64("float64(decimal64) "+c.name, got, c.want)
		}
	}
	for _, c := range dec128ToFloat64 {
		if got := math.Float64bits(d128tof64(c.d)); got != c.want {
			failBits64("float64(decimal128) "+c.name, got, c.want)
		}
	}
	for _, c := range dec64ToFloat32 {
		if got := math.Float32bits(d64tof32(c.d)); got != c.want {
			failBits64("float32(decimal64) "+c.name, uint64(got), uint64(c.want))
		}
	}
	for _, c := range dec128ToFloat32 {
		if got := math.Float32bits(d128tof32(c.d)); got != c.want {
			failBits64("float32(decimal128) "+c.name, uint64(got), uint64(c.want))
		}
	}
	for _, c := range narrowCases {
		if got := math.Decimal64bits(narrow(c.d)); got != c.want {
			failBits64("decimal64(decimal128) "+c.name, got, c.want)
		}
	}
	for _, c := range widenCases {
		if got := bits128(widen(c.d)); got != c.want {
			failBits128("decimal128(decimal64) "+c.name, got, c.want)
		}
	}
}

// --- Comparisons of special values ---

type cmpResult struct{ eq, ne, lt, le, gt, ge bool }

type cmpCase64 struct {
	name string
	a, b decimal64
	want cmpResult
}

type cmpCase128 struct {
	name string
	a, b decimal128
	want cmpResult
}

//go:noinline
func cmp64(a, b decimal64) cmpResult {
	return cmpResult{a == b, a != b, a < b, a <= b, a > b, a >= b}
}

//go:noinline
func cmp128(a, b decimal128) cmpResult {
	return cmpResult{a == b, a != b, a < b, a <= b, a > b, a >= b}
}

var (
	unordered = cmpResult{ne: true}
	equal     = cmpResult{eq: true, le: true, ge: true}
	less      = cmpResult{ne: true, lt: true, le: true}
	greater   = cmpResult{ne: true, gt: true, ge: true}
)

var cmps64 = []cmpCase64{
	{"NaN, NaN", nan64, nan64, unordered},
	{"NaN, 1", nan64, 1, unordered},
	{"1, NaN", 1, nan64, unordered},
	{"NaN, +Inf", nan64, pinf64, unordered},
	{"-Inf, NaN", ninf64, nan64, unordered},
	{"NaN, 0", nan64, 0, unordered},
	{"-0, 0", nzero64, 0, equal},
	{"0, -0", 0, nzero64, equal},
	{"-0, -0", nzero64, nzero64, equal},
	{"1.0, 1.00", 1.0, 1.00, equal},
	{"1.00, 1", 1.00, 1, equal},
	{"-1.0, -1.00", -1.0, -1.00, equal},
	{"0.00, 0", 0.00, 0, equal},
	{"0.00, -0", 0.00, nzero64, equal},
	{"1e300, 1000000000000000e285", 1e300, 1000000000000000e285, equal},
	{"+Inf, +Inf", pinf64, pinf64, equal},
	{"-Inf, -Inf", ninf64, ninf64, equal},
	{"-Inf, +Inf", ninf64, pinf64, less},
	{"+Inf, -Inf", pinf64, ninf64, greater},
	{"-Inf, -max", ninf64, -9999999999999999e369, less},
	{"max, +Inf", 9999999999999999e369, pinf64, less},
	{"1, 2", 1, 2, less},
	{"2, 1", 2, 1, greater},
	{"-2, -1", -2, -1, less},
	{"smallest subnormal, 0", 1e-398, 0, greater},
	{"-smallest subnormal, -0", -1e-398, nzero64, less},
	{"0.9999999999999999, 1", 0.9999999999999999, 1, less},
	{"1.000000000000001, 1.000000000000002", 1.000000000000001, 1.000000000000002, less},
}

var cmps128 = []cmpCase128{
	{"NaN, NaN", nan128, nan128, unordered},
	{"NaN, 1", nan128, 1, unordered},
	{"1, NaN", 1, nan128, unordered},
	{"NaN, -Inf", nan128, ninf128, unordered},
	{"-0, 0", nzero128, 0, equal},
	{"0, -0", 0, nzero128, equal},
	{"1.0, 1.00", 1.0, 1.00, equal},
	{"-1.0, -1.00", -1.0, -1.00, equal},
	{"1e6144, 10e6143", 1e6144, 10e6143, equal},
	{"+Inf, +Inf", pinf128, pinf128, equal},
	{"-Inf, +Inf", ninf128, pinf128, less},
	{"+Inf, -Inf", pinf128, ninf128, greater},
	{"smallest subnormal, 0", 1e-6176, 0, greater},
	{"-smallest subnormal, -0", -1e-6176, nzero128, less},
	{"1, 1.000000000000000000000000000000001", 1, 1.000000000000000000000000000000001, less},
}

func testSpecialComparisons() {
	for _, c := range cmps64 {
		if got := cmp64(c.a, c.b); got != c.want {
			fail("decimal64 comparison " + c.name)
		}
	}
	for _, c := range cmps128 {
		if got := cmp128(c.a, c.b); got != c.want {
			fail("decimal128 comparison " + c.name)
		}
	}

	// Members of a cohort are equal but not identical: the quantum is
	// carried in the encoding.
	one, oneZero := decimal64(1), decimal64(1.00)
	if one != oneZero || math.Decimal64bits(one) == math.Decimal64bits(oneZero) {
		fail("decimal64 1 and 1.00 should be equal with different encodings")
	}
	one128, oneZero128 := decimal128(1), decimal128(1.00)
	if one128 != oneZero128 || bits128(one128) == bits128(oneZero128) {
		fail("decimal128 1 and 1.00 should be equal with different encodings")
	}

	// A NaN is not equal to itself even when it is the same variable.
	n := nan64
	if n == n || !(n != n) {
		fail("decimal64 NaN == NaN")
	}
	n128 := nan128
	if n128 == n128 || !(n128 != n128) {
		fail("decimal128 NaN == NaN")
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
	testEncoders()
	testIEEEArithmetic()
	testIEEECompound()
	testIEEEConversions()
	testSpecialComparisons()
	testNamedTypes()
	testStructs()
	testSliceArray()
	testInterface()

	if bad {
		panic("decimal tests failed")
	}
}
