// run
//go:build goexperiment.decimal

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"math"
	"runtime"
)

var calls int

//go:noinline
func operand(x decimal128, order int) decimal128 {
	if calls != order {
		panic("decimal operand order")
	}
	calls++
	return x
}

// A composite result and live integer values exercise result and memory
// joins when arithmetic sometimes takes a compiler path and sometimes calls.
type result struct {
	a, b, c decimal128
	n       int
}

//go:noinline
func chain(x, y, z decimal128, n int) result {
	return result{x + y, x + z, (x + y) + z, n + 1}
}

//go:noinline
func difference(x, y decimal128) decimal128 { return x - y }

type named decimal128

// Keep decimal values and pointers alive across calls, including a call in
// the slow arithmetic branch. The array arguments/results also exceed the
// argument registers and exercise stack slots and the composite ABI.
//
//go:noinline
func pressure(a, b [5]named, live *[5]int) ([5]named, *[5]int) {
	x0, x1, x2 := a[0]+b[0], a[1]*b[1], a[2]*b[2]
	runtime.GC()
	x3, x4 := a[3]-b[3], a[4]+b[4]
	live[0]++
	return [5]named{x0, x1, x2, x3, x4}, live
}

func check(x, y decimal128) {
	xh, xl := math.Decimal128bits(x)
	yh, yl := math.Decimal128bits(y)
	if xh != yh || xl != yl {
		panic("decimal result bits")
	}
}

func main() {
	if operand(0.00, 0) != 0 || calls != 1 {
		panic("decimal zero evaluation")
	}
	if 0.00 != operand(0, 1) || calls != 2 {
		panic("decimal reverse zero evaluation")
	}
	check(operand(1.25, 2)+operand(2.50, 3), 3.75)
	if calls != 4 {
		panic("decimal add evaluation")
	}
	check(operand(1.25, 4)*operand(2.50, 5), 3.1250)
	if calls != 6 {
		panic("decimal multiply evaluation")
	}
	check(operand(1.25, 6)-operand(0.001, 7), 1.249)
	if calls != 8 {
		panic("decimal subtract evaluation")
	}
	if got := [4]bool{operand(-1, 8) < 0, 0 < operand(1, 9), operand(0.00, 10) >= 0, 0.00 >= operand(0, 11)}; got != [4]bool{true, true, true, true} || calls != 12 {
		panic("decimal ordered zero evaluation")
	}
	for _, v := range []struct{ x, y, z decimal128 }{
		{1.25, 2.50, 0.001},
		{-1.25, -2.50, 0.001},
		{18446744073709551615.00, 0.01, 0.001},
	} {
		r := chain(v.x, v.y, v.z, 42)
		check(r.a, v.x+v.y)
		check(r.b, v.x+v.z)
		check(r.c, (v.x+v.y)+v.z)
		if r.n != 43 {
			panic("decimal composite integer result")
		}
	}
	var sum decimal128 = 0.00
	for range 1000 {
		sum += 0.01
	}
	check(sum, 10.00)
	for range 1000 {
		sum = difference(sum, 0.001)
	}
	check(sum, 9.000)
	check(difference(1.0, 1.00), 0.00)
	check(difference(-1.0, -1.00), 0.00)
	a := [5]named{1.25, 2.50, 18446744073709551615, -1.25, 2.50}
	b := [5]named{2.50, 0.001, 18446744073709551615, -2.50, -2.50}
	// The third coefficient product exceeds 34 digits and must round through
	// the runtime; the other results are exact at their preferred exponents.
	want := [5]named{3.75, 0.00250, 3402823669209384634264811192843491e5, 1.25, 0.00}
	live := new([5]int)
	for range 3 {
		got, p := pressure(a, b, live)
		if p != live {
			panic("decimal composite pointer result")
		}
		for i := range got {
			check(decimal128(got[i]), decimal128(want[i]))
		}
	}
	if live[0] != 3 {
		panic("decimal memory join")
	}
}
