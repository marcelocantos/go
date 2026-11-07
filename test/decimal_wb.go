// run
//go:build goexperiment.decimal

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Test that storing a value that holds a decimal and a pointer through a
// pointer compiles and keeps both fields. Such a store needs a write
// barrier, and the compiler stores the scalar fields separately from the
// pointer fields, so a decimal field has to be treated as a scalar there.

package main

type item64 struct {
	V decimal64
	P *int
}

type item128 struct {
	P *int
	V decimal128
	W [1]decimal64
}

//go:noinline
func store64(p *item64, x decimal64, q *int) {
	*p = item64{V: x, P: q}
}

//go:noinline
func store128(p *item128, x decimal128, y decimal64, q *int) {
	*p = item128{P: q, V: x, W: [1]decimal64{y}}
}

func main() {
	n := 1

	var a item64
	store64(&a, 1.50, &n)
	if a.V != 1.50 || a.P != &n {
		panic("decimal64 field lost in write-barrier store")
	}

	b := new(item128)
	store128(b, 2.25, 3.75, &n)
	if b.V != 2.25 || b.W[0] != 3.75 || b.P != &n {
		panic("decimal128 field lost in write-barrier store")
	}
}
