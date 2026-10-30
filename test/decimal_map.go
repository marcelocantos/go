// run

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Test decimal64 and decimal128 inside composite map keys, whose hash and
// equality functions the compiler generates from the runtime's d64hash,
// d128hash, d64equal and d128equal. Hash and equality must agree: +0 and -0
// are one key, the members of a cohort are one key, and a NaN never finds
// anything, as for float64.

package main

import (
	"fmt"
	"math"
)

type key64 struct {
	A decimal64
	B int
}

func check(ok bool, format string, args ...any) {
	if !ok {
		panic(fmt.Sprintf(format, args...))
	}
}

func testStruct64() {
	negZero := math.Decimal64frombits(1 << 63)
	if negZero != 0 || math.Decimal64bits(negZero) == math.Decimal64bits(decimal64(0)) {
		panic("negZero is not a distinct encoding of zero")
	}

	m := map[key64]int{}

	// Insert and look up, with a second field in the key.
	m[key64{1.5, 1}] = 10
	m[key64{1.5, 2}] = 20
	check(len(m) == 2, "two keys differing in B: len = %d", len(m))
	check(m[key64{1.5, 1}] == 10, "lookup {1.5, 1}")
	check(m[key64{1.5, 2}] == 20, "lookup {1.5, 2}")
	_, ok := m[key64{1.5, 3}]
	check(!ok, "lookup of absent {1.5, 3} succeeded")

	// Members of a cohort are one key: 1.5, 1.50 and 1.500 are equal.
	var a, b, c decimal64 = 1.5, 1.50, 1.500
	check(a == b && b == c, "cohort members compare unequal")
	check(math.Decimal64bits(a) != math.Decimal64bits(b), "cohort members share an encoding")
	m[key64{b, 1}]++
	m[key64{c, 1}]++
	check(m[key64{a, 1}] == 12, "cohort lookup: got %d", m[key64{a, 1}])
	check(len(m) == 2, "cohort members made new keys: len = %d", len(m))

	// +0 and -0 are one key.
	m = map[key64]int{}
	m[key64{0, 7}] = 1
	m[key64{negZero, 7}] = 2
	check(len(m) == 1, "+0 and -0 are distinct keys: len = %d", len(m))
	check(m[key64{0, 7}] == 2, "+0 lookup: got %d", m[key64{0, 7}])
	check(m[key64{negZero, 7}] == 2, "-0 lookup: got %d", m[key64{negZero, 7}])

	// Zeros of different exponents are one key as well.
	m[key64{0.00, 7}] = 3
	check(len(m) == 1 && m[key64{0, 7}] == 3, "0.00 and 0 are distinct keys")

	// Infinities are keys, and their signs differ.
	inf, ninf := math.Decimal64Inf(1), math.Decimal64Inf(-1)
	m = map[key64]int{}
	m[key64{inf, 0}] = 1
	m[key64{ninf, 0}] = 2
	check(len(m) == 2 && m[key64{inf, 0}] == 1 && m[key64{ninf, 0}] == 2, "infinity keys")

	// A NaN is never found, and every insert makes a new key. Compare the
	// behaviour of float64, which is the rule.
	nan := math.Decimal64NaN()
	m = map[key64]int{}
	for i := range 100 {
		m[key64{nan, 0}] = i
	}
	check(len(m) == 100, "NaN inserts: len = %d", len(m))
	_, ok = m[key64{nan, 0}]
	check(!ok, "NaN lookup succeeded")
	n := 0
	for k := range m {
		check(k.A != k.A, "NaN key came back as a number")
		n++
	}
	check(n == 100, "NaN iteration: saw %d", n)
	delete(m, key64{nan, 0})
	check(len(m) == 100, "NaN delete removed a key")
	clear(m)
	check(len(m) == 0, "clear left %d NaN keys", len(m))
}

func testArray128() {
	negZero := math.Decimal128frombits(1<<63, 0)
	if negZero != 0 {
		panic("negZero is not zero")
	}

	m := map[[2]decimal128]int{}

	// Insert and look up, with both elements significant.
	m[[2]decimal128{1, 2}] = 12
	m[[2]decimal128{2, 1}] = 21
	check(len(m) == 2, "reordered elements: len = %d", len(m))
	check(m[[2]decimal128{1, 2}] == 12, "lookup {1, 2}")
	check(m[[2]decimal128{2, 1}] == 21, "lookup {2, 1}")
	_, ok := m[[2]decimal128{1, 3}]
	check(!ok, "lookup of absent {1, 3} succeeded")

	// Members of a cohort are one key, in either element.
	var a, b decimal128 = 1.5, 1.50
	hiA, loA := math.Decimal128bits(a)
	hiB, loB := math.Decimal128bits(b)
	check(a == b, "cohort members compare unequal")
	check(hiA != hiB || loA != loB, "cohort members share an encoding")
	m[[2]decimal128{a, 1}] = 1
	m[[2]decimal128{b, 1.0}]++
	check(m[[2]decimal128{1.500, 1.00}] == 2, "cohort lookup: got %d", m[[2]decimal128{1.500, 1.00}])

	// The two halves of the encoding both reach the hash: these differ only
	// in the low word.
	large1 := decimal128(1)
	large2 := decimal128(2)
	large1 += 1e20
	large2 += 1e20
	m = map[[2]decimal128]int{}
	m[[2]decimal128{large1, 0}] = 1
	m[[2]decimal128{large2, 0}] = 2
	check(len(m) == 2 && m[[2]decimal128{large1, 0}] == 1 && m[[2]decimal128{large2, 0}] == 2, "low-word keys")

	// +0 and -0 are one key, in either element.
	m = map[[2]decimal128]int{}
	m[[2]decimal128{0, 0}] = 1
	m[[2]decimal128{negZero, 0}] = 2
	m[[2]decimal128{0, negZero}] = 3
	m[[2]decimal128{negZero, negZero}] = 4
	check(len(m) == 1, "+0 and -0 are distinct keys: len = %d", len(m))
	check(m[[2]decimal128{0, 0}] == 4, "zero lookup: got %d", m[[2]decimal128{0, 0}])

	// A NaN in either element is never found.
	nan := math.Decimal128NaN()
	m = map[[2]decimal128]int{}
	for i := range 100 {
		m[[2]decimal128{nan, 1}] = i
		m[[2]decimal128{1, nan}] = i
	}
	check(len(m) == 200, "NaN inserts: len = %d", len(m))
	_, ok = m[[2]decimal128{nan, 1}]
	check(!ok, "NaN lookup succeeded")
	delete(m, [2]decimal128{1, nan})
	check(len(m) == 200, "NaN delete removed a key")
	clear(m)
	check(len(m) == 0, "clear left %d NaN keys", len(m))
}

// testPlain covers the bare key types, whose functions are the runtime's own
// rather than generated ones, and a key reached through an interface, which
// goes through the runtime's typehash.
func testPlain() {
	negZero64 := math.Decimal64frombits(1 << 63)
	m64 := map[decimal64]int{1.5: 1}
	m64[1.50] = 2
	m64[0] = 3
	m64[negZero64] = 4
	check(len(m64) == 2 && m64[1.500] == 2 && m64[0] == 4, "map[decimal64]int")

	negZero128 := math.Decimal128frombits(1<<63, 0)
	m128 := map[decimal128]int{1.5: 1}
	m128[1.50] = 2
	m128[0] = 3
	m128[negZero128] = 4
	check(len(m128) == 2 && m128[1.500] == 2 && m128[0] == 4, "map[decimal128]int")

	mi := map[any]int{}
	mi[decimal64(1.5)] = 1
	mi[decimal64(1.50)] = 2
	mi[decimal128(1.5)] = 3
	mi[key64{1.50, 1}] = 4
	mi[key64{1.500, 1}] = 5
	mi[[2]decimal128{0, 1.0}] = 6
	mi[[2]decimal128{negZero128, 1.00}] = 7
	check(len(mi) == 4, "map[any]int: len = %d", len(mi))
	check(mi[decimal64(1.500)] == 2, "map[any]int decimal64")
	check(mi[decimal128(1.500)] == 3, "map[any]int decimal128")
	check(mi[key64{1.5, 1}] == 5, "map[any]int struct")
	check(mi[[2]decimal128{0, 1}] == 7, "map[any]int array")

	nan := math.Decimal64NaN()
	mi = map[any]int{}
	mi[nan] = 1
	mi[nan] = 2
	check(len(mi) == 2, "map[any]int NaN: len = %d", len(mi))
}

// testCompositeEquality checks the generated equality functions directly.
func testCompositeEquality() {
	check(key64{1.5, 1} == key64{1.50, 1}, "struct cohort equality")
	check(key64{1.5, 1} != key64{1.5, 2}, "struct inequality")
	check(key64{0, 1} == key64{math.Decimal64frombits(1 << 63), 1}, "struct zero equality")
	nan := math.Decimal64NaN()
	check(key64{nan, 1} != key64{nan, 1}, "struct NaN equality")
	check([2]decimal128{1.5, 0} == [2]decimal128{1.50, 0.0}, "array cohort equality")
	nan128 := math.Decimal128NaN()
	check([2]decimal128{nan128, 0} != [2]decimal128{nan128, 0}, "array NaN equality")
	var x, y any = key64{1.5, 1}, key64{1.50, 1}
	check(x == y, "interface struct equality")
}

func main() {
	testStruct64()
	testArray128()
	testPlain()
	testCompositeEquality()
}
