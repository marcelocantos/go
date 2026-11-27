// asmcheck -gcflags=-d=decimalinline=3
//go:build goexperiment.decimal

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codegen

// The decimal64 operators at the full setting of -d=decimalinline, where
// they differ from the compact default that decimal.go checks.

func Less64(x, y decimal64) bool {
	// arm64:"UMULH" -"CALL runtime.dlt64"
	// amd64:"MULX?Q" -"CALL runtime.dlt64"
	return x < y
}

func Sub64(x, y decimal64) decimal64 {
	// amd64:"CALL internal/bid.addSlow64" -"CALL runtime.dsub64"
	// arm64:"CALL internal/bid.addSlow64" -"CALL runtime.dsub64"
	return x - y
}
