// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build amd64 || arm64

// The decimal128 comparisons on the targets where the compiler compares word
// operands itself (bidCompare in cmd/compile/internal/ssagen) and calls
// these only for the operands it refuses: wider coefficients and the special
// values. Its fast path accepts every pair that internal/bid's inline front
// for Equal128, Less128 and LessEqual128 accepts (word operands of one sign
// and one exponent), so these call Compare128, the general comparison, which
// has no front, instead of testing the operands a second time. The result
// does not depend on that: Compare128 takes every operand. decimal128_call.go
// has the other targets.

package runtime

import "internal/bid"

func deq128(xhi, xlo, yhi, ylo uint64) bool {
	c, ok := bid.Compare128(from128(xhi, xlo), from128(yhi, ylo))
	return ok && c == 0
}

func dlt128(xhi, xlo, yhi, ylo uint64) bool {
	c, ok := bid.Compare128(from128(xhi, xlo), from128(yhi, ylo))
	return ok && c < 0
}

func dle128(xhi, xlo, yhi, ylo uint64) bool {
	c, ok := bid.Compare128(from128(xhi, xlo), from128(yhi, ylo))
	return ok && c <= 0
}
