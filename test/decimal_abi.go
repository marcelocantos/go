// run
//go:build goexperiment.decimal

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Exercise decimal fields across the boundary between SSA-able values and
// larger aggregates returned in registers. The latter must store both words
// of every decimal128 field, rather than treating it as one register value.
package main

import "math"

type D decimal128
type one struct{ A D }
type two struct{ A, B D }
type three struct{ A, B, C D }
type four struct{ A, B, C, D D }
type nested struct{ A, B, C one }
type mixed struct {
	A       D
	B, C, D int
}
type mixed2 struct {
	A, B D
	C, D int
}
type array struct {
	A [2]D
	B D
}
type stack struct{ A, B, C, D, E, F D }
type small struct{ A, B, C decimal64 }

//go:noinline
func relay[T any](x T) T { return x }

//go:noinline
func returnThree(x three) three { return x }

//go:noinline
func forwardThree(x three) three { return returnThree(x) }

//go:noinline
func mixedResults(b []byte, x three) ([]byte, three) { return b, x }

func check(got, want D) {
	gh, gl := math.Decimal128bits(decimal128(got))
	wh, wl := math.Decimal128bits(decimal128(want))
	if gh != wh || gl != wl {
		panic("decimal128 argument or result changed bits")
	}
}

func main() {
	const a, b, c, d, e, f D = 1.50, -2.75, 3e+10, -4e-20, 5.000, 6e+30
	check(relay(a), a)
	check(relay(one{a}).A, a)
	t := relay(two{a, b})
	check(t.A, a)
	check(t.B, b)
	want := three{a, b, c}
	for _, v := range []three{relay(want), forwardThree(want)} {
		check(v.A, a)
		check(v.B, b)
		check(v.C, c)
	}
	u := relay(four{a, b, c, d})
	check(u.A, a)
	check(u.B, b)
	check(u.C, c)
	check(u.D, d)
	n := relay(nested{one{a}, one{b}, one{c}})
	check(n.A.A, a)
	check(n.B.A, b)
	check(n.C.A, c)
	m := relay(mixed{a, 7, 8, 9})
	check(m.A, a)
	if m.B != 7 || m.C != 8 || m.D != 9 {
		panic("mixed integer fields changed")
	}
	m2 := relay(mixed2{a, b, 10, 11})
	check(m2.A, a)
	check(m2.B, b)
	if m2.C != 10 || m2.D != 11 {
		panic("mixed integer fields changed")
	}
	ar := relay(array{[2]D{a, b}, c})
	check(ar.A[0], a)
	check(ar.A[1], b)
	check(ar.B, c)
	s := relay(stack{a, b, c, d, e, f})
	check(s.A, a)
	check(s.B, b)
	check(s.C, c)
	check(s.D, d)
	check(s.E, e)
	check(s.F, f)
	sm := relay(small{1.50, -2.75, 3e+10})
	if sm != (small{1.50, -2.75, 3e+10}) {
		panic("decimal64 fields changed")
	}
	bytes, result := mixedResults([]byte{12, 13}, want)
	if len(bytes) != 2 || bytes[0] != 12 || bytes[1] != 13 {
		panic("mixed slice result changed")
	}
	check(result.A, a)
	check(result.B, b)
	check(result.C, c)
}
