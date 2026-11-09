// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !amd64 && !arm64

// The decimal128 comparisons on targets where the compiler calls the runtime
// for every operand (or, on linux/ppc64le, uses the hardware): the kernel's
// inline front decides word operands of one quantum here. decimal128_inline.go
// has the targets whose compiler decides those itself.

package runtime

import "internal/bid"

func deq128(xhi, xlo, yhi, ylo uint64) bool {
	return bid.Equal128(from128(xhi, xlo), from128(yhi, ylo))
}

func dlt128(xhi, xlo, yhi, ylo uint64) bool {
	return bid.Less128(from128(xhi, xlo), from128(yhi, ylo))
}

func dle128(xhi, xlo, yhi, ylo uint64) bool {
	return bid.LessEqual128(from128(xhi, xlo), from128(yhi, ylo))
}
