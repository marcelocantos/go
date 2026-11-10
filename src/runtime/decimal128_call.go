// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !amd64 && !arm64

// The decimal128 operators on targets where the compiler calls the runtime
// for every operand (or, on linux/ppc64le, uses the hardware): the kernel's
// inline fronts decide the money cases here. On amd64 and arm64 the
// compiler decides those itself and calls the kernel directly for the
// rest, so these functions do not exist there.

package runtime

import "internal/bid"

func dadd128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	return to128(bid.Add128(from128(xhi, xlo), from128(yhi, ylo), bid.ToNearestEven))
}

func dsub128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	return to128(bid.Sub128(from128(xhi, xlo), from128(yhi, ylo), bid.ToNearestEven))
}

func dmul128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	return to128(bid.Mul128(from128(xhi, xlo), from128(yhi, ylo), bid.ToNearestEven))
}

func deq128(xhi, xlo, yhi, ylo uint64) bool {
	return bid.Equal128(from128(xhi, xlo), from128(yhi, ylo))
}

func dlt128(xhi, xlo, yhi, ylo uint64) bool {
	return bid.Less128(from128(xhi, xlo), from128(yhi, ylo))
}

func dle128(xhi, xlo, yhi, ylo uint64) bool {
	return bid.LessEqual128(from128(xhi, xlo), from128(yhi, ylo))
}
