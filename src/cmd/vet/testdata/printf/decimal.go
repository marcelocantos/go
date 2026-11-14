// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

// This file contains tests for the printf checker on decimal types.

package printf

import "fmt"

func DecimalPrintfTests() {
	var d64 decimal64
	var d128 decimal128
	// Valid format verbs for decimal types: a float's, but for %x and %X.
	fmt.Printf("%f %f", d64, d128)
	fmt.Printf("%F %F", d64, d128)
	fmt.Printf("%e %e", d64, d128)
	fmt.Printf("%E %E", d64, d128)
	fmt.Printf("%g %g", d64, d128)
	fmt.Printf("%G %G", d64, d128)
	fmt.Printf("%v %v", d64, d128)
	fmt.Printf("%b %b", d64, d128)
	// Invalid format verbs for decimal types. %x and %X are a float's
	// radix-two notations, which fmt does not write for a decimal.
	fmt.Printf("%x", d64)  // ERROR "Printf format %x has arg d64 of wrong type decimal64"
	fmt.Printf("%X", d128) // ERROR "Printf format %X has arg d128 of wrong type decimal128"
	fmt.Printf("%s", d64)  // ERROR "Printf format %s has arg d64 of wrong type decimal64"
	fmt.Printf("%s", d128) // ERROR "Printf format %s has arg d128 of wrong type decimal128"
	fmt.Printf("%d", d64)  // ERROR "Printf format %d has arg d64 of wrong type decimal64"
	fmt.Printf("%d", d128) // ERROR "Printf format %d has arg d128 of wrong type decimal128"
	fmt.Printf("%c", d64)  // ERROR "Printf format %c has arg d64 of wrong type decimal64"
	fmt.Printf("%t", d64)  // ERROR "Printf format %t has arg d64 of wrong type decimal64"
}
