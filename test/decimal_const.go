// run

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Test the encodings the compiler gives decimal constants, on both of its
// paths: package-level variables (static data) and values in function bodies
// (SSA). Every expected encoding is written here as a sign, a coefficient and
// an exponent, derived by hand from the exact value of the constant.
// decimal_const_overflow.go covers the constants the type checker rejects.

package main

import (
	"math/big"
	"unsafe"
)

var bad bool

func fail(name, path string, got, want any) {
	if !bad {
		println("BUG")
		bad = true
	}
	switch got := got.(type) {
	case uint64:
		println(name, path, "got", got, "want", want.(uint64))
	case [2]uint64:
		w := want.([2]uint64)
		println(name, path, "got lo", got[0], "hi", got[1], "want lo", w[0], "hi", w[1])
	}
}

// want64 is the BID encoding of (-1)^neg × coeff × 10^exp.
func want64(neg bool, coeff uint64, exp int) uint64 {
	const bias = 398
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

// want128 is the BID encoding, as low and high words, of
// (-1)^neg × coeff × 10^exp, with coeff written in decimal. Every canonical
// decimal128 coefficient is below 2^113, which the first encoding form holds.
func want128(neg bool, coeff string, exp int) [2]uint64 {
	const bias = 6176
	c, ok := new(big.Int).SetString(coeff, 10)
	if !ok {
		panic("bad coefficient " + coeff)
	}
	if c.BitLen() > 113 {
		panic("coefficient too large " + coeff)
	}
	lo := new(big.Int).And(c, new(big.Int).SetUint64(1<<64-1)).Uint64()
	hi := new(big.Int).Rsh(c, 64).Uint64()
	hi |= uint64(exp+bias) << 49
	if neg {
		hi |= 1 << 63
	}
	return [2]uint64{lo, hi}
}

//go:noinline
func bits64(d decimal64) uint64 { return *(*uint64)(unsafe.Pointer(&d)) }

//go:noinline
func bits128(d decimal128) [2]uint64 { return *(*[2]uint64)(unsafe.Pointer(&d)) }

// check64 compares the variable g, from static data, and the argument l,
// materialized by SSA at the call site, with want.
//
//go:noinline
func check64(name string, g, l decimal64, want uint64) {
	if got := bits64(g); got != want {
		fail(name, "static", got, want)
	}
	if got := bits64(l); got != want {
		fail(name, "ssa", got, want)
	}
}

//go:noinline
func check128(name string, g, l decimal128, want [2]uint64) {
	if got := bits128(g); got != want {
		fail(name, "static", got, want)
	}
	if got := bits128(l); got != want {
		fail(name, "ssa", got, want)
	}
}

// Computed constants have no source literal.
const (
	third      = 1.0 / 3.0
	twoThirds  = 2.0 / 3.0
	sum        = 1.5 + 1.5
	gone       = 1.5 - 1.5
	tieTail64  = 1.0000000000000005 + 1.0/3.0*1e-30 // just above a tie at 16 digits
	tieTail128 = 1.0000000000000000000000000000000005 + 1.0/3.0*1e-40
)

// Named constants carry the literal they are declared with.
const (
	named               = 3.14
	typed64  decimal64  = 1.50
	typed128 decimal128 = 1.50
	f32Named float32    = 0.1
	f64Named float64    = 0.1
	digits34            = 1.234567890123456789012345678901234
)

var (
	rune64        decimal64  = 'a'
	rune128       decimal128 = 'a'
	nl64          decimal64  = '\n'
	nl128         decimal128 = '\n'
	hexFloat64    decimal64  = 0x1p-2
	hexFloat128   decimal128 = 0x1p-2
	hexFrac64     decimal64  = 0x1.8p1
	hexFrac128    decimal128 = 0x1.8p1
	hexBig64      decimal64  = 0x1p100
	hexBig128     decimal128 = 0x1p100
	hexInt64      decimal64  = 0x10
	hexInt128     decimal128 = 0x10
	octInt64      decimal64  = 0o17
	binInt64      decimal64  = 0b101
	legacyOct64   decimal64  = 0755
	underscore64  decimal64  = 1_000.50
	underscore128 decimal128 = 1_000.50

	long64      decimal64  = 1.00000000000000000000000001
	long128     decimal128 = 1.00000000000000000000000001
	longInt64   decimal64  = 12345678901234567890123
	longInt128  decimal128 = 12345678901234567890123
	tie64       decimal64  = 1.0000000000000005
	tieOdd64    decimal64  = 1.0000000000000015
	tieSticky64 decimal64  = 1.00000000000000050000000000000001
	padded64    decimal64  = 1.0000000000000000000000
	padded128   decimal128 = 1.0000000000000000000000000000000000000000
	big26       decimal64  = 100000000000000000000000000

	full128      decimal128 = 1.234567890123456789012345678901234
	named128     decimal128 = digits34
	named64      decimal64  = digits34
	tie128       decimal128 = 1.0000000000000000000000000000000005
	tieSticky128 decimal128 = 1.00000000000000000000000000000000050000001
	far128       decimal128 = 1e+2000
	farNeg128    decimal128 = 1e-2000
	farTie128    decimal128 = 1.0000000000000000000000000000000005e+2000

	third64      decimal64  = third
	third128     decimal128 = third
	twoThirds64  decimal64  = twoThirds
	twoThirds128 decimal128 = twoThirds
	direct64     decimal64  = 1.0 / 3.0
	direct128    decimal128 = 1.0 / 3.0
	tieTail64v   decimal64  = tieTail64
	tieTail64w   decimal128 = tieTail64
	tieTail128v  decimal64  = tieTail128
	tieTail128w  decimal128 = tieTail128

	sum64   decimal64  = sum
	sum128  decimal128 = sum
	gone64  decimal64  = gone
	gone128 decimal128 = gone

	lit64        decimal64  = 1.50
	lit128       decimal128 = 1.50
	exp64        decimal64  = 1.5e3
	exp128       decimal128 = 1.5e3
	neg64        decimal64  = -1.50
	neg128       decimal128 = -1.50
	zero64       decimal64  = 0.00
	zero128      decimal128 = 0.00
	minusZero64  decimal64  = -0.0
	minusZero128 decimal128 = -0.0
	namedVar64   decimal64  = named
	namedVar128  decimal128 = -named
	typedVar64              = typed64
	typedVar128             = typed128
	conv64                  = decimal64(1.50)
	conv128                 = decimal128(-1.50)
	f32Var64                = decimal64(f32Named)
	f32Var128               = decimal128(f32Named)
	f64Var64                = decimal64(f64Named)
	f64Var128               = decimal128(f64Named)

	max64       decimal64  = 9.999999999999999e+384
	max128      decimal128 = 9.999999999999999999999999999999999e+6144
	top64       decimal64  = 1e+384
	top128      decimal128 = 1e+6144
	clamp64     decimal64  = 1e+370
	clamp128    decimal128 = 1e+6112
	zeroHigh64  decimal64  = 0e+400
	zeroHigh128 decimal128 = 0e+7000

	minNormal64   decimal64  = 1e-383
	minNormal128  decimal128 = 1e-6143
	sub64         decimal64  = 1.23456789e-390
	sub128        decimal128 = 1.23e-6170
	tiny64        decimal64  = 1e-398
	tiny128       decimal128 = 1e-6176
	subUp64       decimal64  = 1.5e-398
	subUp128      decimal128 = 1.5e-6176
	subEven64     decimal64  = 2.5e-398
	subEven128    decimal128 = 2.5e-6176
	subTie64      decimal64  = 5e-399
	subTie128     decimal128 = 5e-6177
	subRoundUp64  decimal64  = 6e-399
	subRoundUp128 decimal128 = 6e-6177
	under64       decimal64  = 1e-399
	under128      decimal128 = 1e-6177
	negUnder64    decimal64  = -1e-399
	negUnder128   decimal128 = -1e-6177
	vanish64      decimal64  = 1e-100000
	vanish128     decimal128 = 1e-100000
	computedSub64 decimal64  = 1e-200 * 1e-198
)

// A conversion to a type parameter is not a constant, but the compiler encodes
// it, from the constant operand. It has no literal to take a quantum from.
func convert64[T ~decimal64]() (a, b, c, d T) {
	return T(1.50), T(7), T('a'), T(float32(0.1))
}

func convert128[T ~decimal128]() (a, b, c, d T) {
	return T(0.1), T(-1.50), T(1.234567890123456789012345678901234), T(float32(0.1))
}

func main() {
	// Rune literals.
	check64("rune", rune64, 'a', want64(false, 97, 0))
	check128("rune", rune128, 'a', want128(false, "97", 0))
	check64("newline", nl64, '\n', want64(false, 10, 0))
	check128("newline", nl128, '\n', want128(false, "10", 0))

	// Hexadecimal floating-point and other non-decimal literals: no decimal
	// exponent to keep, so the exact value at the exponent nearest 0.
	check64("hexfloat", hexFloat64, 0x1p-2, want64(false, 25, -2))
	check128("hexfloat", hexFloat128, 0x1p-2, want128(false, "25", -2))
	check64("hexfrac", hexFrac64, 0x1.8p1, want64(false, 3, 0))
	check128("hexfrac", hexFrac128, 0x1.8p1, want128(false, "3", 0))
	check64("hexbig", hexBig64, 0x1p100, want64(false, 1267650600228229, 15))
	check128("hexbig", hexBig128, 0x1p100, want128(false, "1267650600228229401496703205376", 0))
	check64("hexint", hexInt64, 0x10, want64(false, 16, 0))
	check128("hexint", hexInt128, 0x10, want128(false, "16", 0))
	check64("octint", octInt64, 0o17, want64(false, 15, 0))
	check64("binint", binInt64, 0b101, want64(false, 5, 0))
	check64("legacy octal", legacyOct64, 0755, want64(false, 493, 0))
	check64("underscore", underscore64, 1_000.50, want64(false, 100050, -2))
	check128("underscore", underscore128, 1_000.50, want128(false, "100050", -2))

	// Literals with more than 19 significant digits.
	check64("long", long64, 1.00000000000000000000000001, want64(false, 1000000000000000, -15))
	check128("long", long128, 1.00000000000000000000000001, want128(false, "100000000000000000000000001", -26))
	check64("longint", longInt64, 12345678901234567890123, want64(false, 1234567890123457, 7))
	check128("longint", longInt128, 12345678901234567890123, want128(false, "12345678901234567890123", 0))
	check64("tie", tie64, 1.0000000000000005, want64(false, 1000000000000000, -15))
	check64("tie odd", tieOdd64, 1.0000000000000015, want64(false, 1000000000000002, -15))
	check64("tie sticky", tieSticky64, 1.00000000000000050000000000000001, want64(false, 1000000000000001, -15))
	check64("padded", padded64, 1.0000000000000000000000, want64(false, 1000000000000000, -15))
	check128("padded", padded128, 1.0000000000000000000000000000000000000000, want128(false, "1000000000000000000000000000000000", -33))
	check64("big26", big26, 100000000000000000000000000, want64(false, 1000000000000000, 11))

	// 34-digit decimal128 constants.
	check128("full", full128, 1.234567890123456789012345678901234, want128(false, "1234567890123456789012345678901234", -33))
	check128("named34", named128, digits34, want128(false, "1234567890123456789012345678901234", -33))
	check64("named34", named64, digits34, want64(false, 1234567890123457, -15))
	check128("tie", tie128, 1.0000000000000000000000000000000005, want128(false, "1000000000000000000000000000000000", -33))
	check128("tie sticky", tieSticky128, 1.00000000000000000000000000000000050000001, want128(false, "1000000000000000000000000000000001", -33))
	check128("far", far128, 1e+2000, want128(false, "1", 2000))
	check128("far negative", farNeg128, 1e-2000, want128(false, "1", -2000))
	check128("far tie", farTie128, 1.0000000000000000000000000000000005e+2000, want128(false, "1000000000000000000000000000000000", 1967))

	// Computed constants: no exact expansion, so a full coefficient.
	check64("third", third64, third, want64(false, 3333333333333333, -16))
	check128("third", third128, third, want128(false, "3333333333333333333333333333333333", -34))
	check64("two thirds", twoThirds64, twoThirds, want64(false, 6666666666666667, -16))
	check128("two thirds", twoThirds128, twoThirds, want128(false, "6666666666666666666666666666666667", -34))
	check64("direct", direct64, 1.0/3.0, want64(false, 3333333333333333, -16))
	check128("direct", direct128, 1.0/3.0, want128(false, "3333333333333333333333333333333333", -34))
	check64("tie tail", tieTail64v, tieTail64, want64(false, 1000000000000001, -15))
	check128("tie tail", tieTail64w, tieTail64, want128(false, "1000000000000000500000000000000333", -33))
	check64("tie tail 128", tieTail128v, tieTail128, want64(false, 1000000000000000, -15))
	check128("tie tail 128", tieTail128w, tieTail128, want128(false, "1000000000000000000000000000000001", -33))

	// Computed constants with an exact value: a 16-digit coefficient at both
	// widths, and the zero value for a zero.
	check64("sum", sum64, sum, want64(false, 3000000000000000, -15))
	check128("sum", sum128, sum, want128(false, "3000000000000000", -15))
	check64("gone", gone64, gone, 0)
	check128("gone", gone128, gone, [2]uint64{})

	// The quantum of the literal.
	check64("literal", lit64, 1.50, want64(false, 150, -2))
	check128("literal", lit128, 1.50, want128(false, "150", -2))
	check64("exponent", exp64, 1.5e3, want64(false, 15, 2))
	check128("exponent", exp128, 1.5e3, want128(false, "15", 2))
	check64("negative", neg64, -1.50, want64(true, 150, -2))
	check128("negative", neg128, -1.50, want128(true, "150", -2))
	check64("zero", zero64, 0.00, want64(false, 0, -2))
	check128("zero", zero128, 0.00, want128(false, "0", -2))
	check64("minus zero", minusZero64, -0.0, want64(false, 0, -1))
	check128("minus zero", minusZero128, -0.0, want128(false, "0", -1))
	check64("named", namedVar64, named, want64(false, 314, -2))
	check128("named negated", namedVar128, -named, want128(true, "3140000000000000", -15))
	check64("typed", typedVar64, typed64, want64(false, 150, -2))
	check128("typed", typedVar128, typed128, want128(false, "150", -2))
	check64("conversion", conv64, decimal64(1.50), want64(false, 150, -2))
	check128("conversion", conv128, decimal128(-1.50), want128(true, "150", -2))

	// A typed binary constant converts from its exact binary value.
	check64("float32", f32Var64, decimal64(f32Named), want64(false, 1000000014901161, -16))
	check128("float32", f32Var128, decimal128(f32Named), want128(false, "100000001490116119384765625", -27))
	check64("float64", f64Var64, decimal64(f64Named), want64(false, 1000000000000000, -16))
	check128("float64", f64Var128, decimal128(f64Named), want128(false, "1000000000000000055511151231257827", -34))

	// The top of the range. Constants above it do not compile.
	check64("max", max64, 9.999999999999999e+384, want64(false, 9999999999999999, 369))
	check128("max", max128, 9.999999999999999999999999999999999e+6144, want128(false, "9999999999999999999999999999999999", 6111))
	check64("top", top64, 1e+384, want64(false, 1000000000000000, 369))
	check128("top", top128, 1e+6144, want128(false, "1000000000000000000000000000000000", 6111))
	check64("clamp", clamp64, 1e+370, want64(false, 10, 369))
	check128("clamp", clamp128, 1e+6112, want128(false, "10", 6111))
	check64("zero high", zeroHigh64, 0e+400, want64(false, 0, 369))
	check128("zero high", zeroHigh128, 0e+7000, want128(false, "0", 6111))

	// Subnormal constants and underflow, which round to nearest, ties to even.
	check64("min normal", minNormal64, 1e-383, want64(false, 1, -383))
	check128("min normal", minNormal128, 1e-6143, want128(false, "1", -6143))
	check64("subnormal", sub64, 1.23456789e-390, want64(false, 123456789, -398))
	check128("subnormal", sub128, 1.23e-6170, want128(false, "123", -6172))
	check64("tiny", tiny64, 1e-398, want64(false, 1, -398))
	check128("tiny", tiny128, 1e-6176, want128(false, "1", -6176))
	check64("sub up", subUp64, 1.5e-398, want64(false, 2, -398))
	check128("sub up", subUp128, 1.5e-6176, want128(false, "2", -6176))
	check64("sub even", subEven64, 2.5e-398, want64(false, 2, -398))
	check128("sub even", subEven128, 2.5e-6176, want128(false, "2", -6176))
	check64("sub tie", subTie64, 5e-399, want64(false, 0, -398))
	check128("sub tie", subTie128, 5e-6177, want128(false, "0", -6176))
	check64("sub round up", subRoundUp64, 6e-399, want64(false, 1, -398))
	check128("sub round up", subRoundUp128, 6e-6177, want128(false, "1", -6176))
	check64("under", under64, 1e-399, want64(false, 0, -398))
	check128("under", under128, 1e-6177, want128(false, "0", -6176))
	check64("negative under", negUnder64, -1e-399, want64(true, 0, -398))
	check128("negative under", negUnder128, -1e-6177, want128(true, "0", -6176))
	check64("vanish", vanish64, 1e-100000, want64(false, 0, -398))
	check128("vanish", vanish128, 1e-100000, want128(false, "0", -6176))
	check64("computed subnormal", computedSub64, 1e-200*1e-198, want64(false, 1, -398))

	// Conversions to a type parameter.
	a64, b64, c64, d64 := convert64[decimal64]()
	check64("T(1.50)", a64, a64, want64(false, 1500000000000000, -15))
	check64("T(7)", b64, b64, want64(false, 7000000000000000, -15))
	check64("T('a')", c64, c64, want64(false, 9700000000000000, -14))
	check64("T(float32(0.1))", d64, d64, want64(false, 1000000014901161, -16))
	a128, b128, c128, d128 := convert128[decimal128]()
	check128("T(0.1)", a128, a128, want128(false, "1000000000000000", -16))
	check128("T(-1.50)", b128, b128, want128(true, "1500000000000000", -15))
	check128("T(34 digits)", c128, c128, want128(false, "1234567890123456789012345678901234", -33))
	check128("T(float32(0.1))", d128, d128, want128(false, "100000001490116119384765625", -27))

	// A constant assigned in a function body.
	local64 := decimal64(1.0 / 3.0)
	if got, want := bits64(local64), want64(false, 3333333333333333, -16); got != want {
		fail("local third", "ssa", got, want)
	}
	local128 := decimal128(1.234567890123456789012345678901234)
	if got, want := bits128(local128), want128(false, "1234567890123456789012345678901234", -33); got != want {
		fail("local full", "ssa", got, want)
	}

	if bad {
		panic("decimal_const failed")
	}
}
