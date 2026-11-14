// errorcheck
//go:build goexperiment.decimal

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Test that complex rejects decimal arguments: there is no complex type
// with decimal parts.

package p

var (
	d64  decimal64
	d128 decimal128
	_    = complex(d64, d64)        // ERROR "arguments have type decimal64, expected binary floating-point \(there is no decimal complex type\)"
	_    = complex(d128, 1)         // ERROR "arguments have type decimal128, expected binary floating-point"
	_    = complex(decimal64(1), 2) // ERROR "expected binary floating-point"
	_    = real(d64)                // ERROR "argument has type decimal64, expected complex type|invalid argument"
)
