// asmcheck -gcflags=-d=decimalinline=1
//go:build goexperiment.decimal

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codegen

import "math/decimal"

// The decimal64 operators at the none setting of -d=decimalinline: a kernel
// call each, with nothing inline.

func Add64(x, y decimal64) decimal64 {
	// amd64:"CALL internal/bid.addSlow64" -"BTQ" -"CALL runtime.dadd64"
	// arm64:"CALL internal/bid.addSlow64" -"TBNZ" -"CALL runtime.dadd64"
	return x + y
}

func Less64(x, y decimal64) bool {
	// arm64:"CALL internal/bid.quietCmpSlow64" -"UMULH" -"CALL runtime.dlt64"
	// amd64:"CALL internal/bid.quietCmpSlow64" -"MULX?Q" -"CALL runtime.dlt64"
	return x < y
}

func Round64(x decimal64) decimal64 {
	// amd64:"CALL internal/decimal" -"MULX?Q"
	// arm64:"CALL internal/decimal" -"UMULH"
	return decimal.Round(x)
}
