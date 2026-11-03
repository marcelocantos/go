// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"./a"
	"fmt"
)

func check(name string, got any, want string) {
	if s := fmt.Sprint(got); s != want {
		panic(fmt.Sprintf("%s = %s, want %s", name, s, want))
	}
}

// Typed decimal constant expressions are constants, rounded at each
// operation as at run time. The expected strings are written by hand.
const (
	one        decimal64 = 1
	roundTrip            = one / 3 * 3
	exact      decimal64 = 1.0 / 3 * 3
	product              = decimal64(1.50) * 2
	nearOne    decimal64 = 1.00000000000000001
	quarter              = decimal64(1.00) / 4
	widened              = decimal128(product) + 0.001
	difference           = decimal64(5.00) - 5
	arrayLen             = len([int(decimal64(2.50) * 2)]bool{})
)

func main() {
	var p decimal64 = a.Base
	var q decimal64 = a.Twice
	var n decimal64 = -a.Base
	var r decimal64 = a.Third
	var o decimal64 = a.Other
	var m decimal64 = a.Neg
	check("a.Base", p, "1.50")
	check("a.Twice", q, "3.00")
	check("-a.Base", n, "-1.50")
	check("a.Third", r, "0.3333333333333333")
	check("a.Other", o, "2.00")
	check("a.Neg", m, "-1.50")
	check("a.Price", a.Price, "19.99")
	check("a.Tax", a.Tax, "1.649175")
	check("a.Total", a.Total, "3.00")
	check("decimal128(a.Rate) * 100", decimal128(a.Rate)*100, "8.2500")

	check("one / 3 * 3", roundTrip, "0.9999999999999999")
	check("1.0 / 3 * 3", exact, "1")
	check("decimal64(1.50) * 2", product, "3.00")
	check("1.00000000000000001", nearOne, "1.000000000000000")
	check("decimal64(1.00) / 4", quarter, "0.25")
	check("decimal128(product) + 0.001", widened, "3.001")
	check("decimal64(5.00) - 5", difference, "0.00")
	check("array length", arrayLen, "5")
	if nearOne != 1 {
		panic("1.00000000000000001 != 1")
	}

	// The same expressions on variables give the same results.
	v1, v3 := one, decimal64(3)
	check("variables", v1/v3*v3, "0.9999999999999999")
	v150 := decimal64(1.50)
	check("variables", v150*2, "3.00")
}
