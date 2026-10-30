// errorcheck

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Test that a decimal constant above the largest finite value of its type
// does not compile, however large it is.

package p

const third = 1.0 / 3.0

var (
	a decimal64  = 1e385                        // ERROR "overflows|truncated"
	b decimal128 = 1e6145                       // ERROR "overflows|truncated"
	c decimal64  = 1.0000000000000001e385       // ERROR "overflows|truncated"
	d decimal128 = 1e100000                     // ERROR "overflows|truncated"
	e            = decimal64(1e1000)            // ERROR "overflows|truncated"
	f            = decimal128(-1e7000)          // ERROR "overflows|truncated"
	g decimal64  = 1e386 * third                // ERROR "overflows|truncated"
	h            = decimal64(decimal128(1e385)) // ERROR "overflows|truncated"
)

const i decimal64 = 1e400 // ERROR "overflows|truncated"
