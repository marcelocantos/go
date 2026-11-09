// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build amd64 || arm64

// The decimal128 operators that the compiler lowers to integer fast paths on
// these targets (bidAddSub128, bidMul128 and bidCompare in
// cmd/compile/internal/ssagen). The compiler calls these functions only for
// operands its fast paths refuse, and those accept every operand that
// internal/bid's inline fronts accept, so these call the kernel's general
// paths directly instead of testing the operands a second time. The general
// paths take every operand, so the result does not depend on that: a target
// listed here whose compiler decided nothing would still compute correctly,
// only more slowly. decimal128_call.go has the other targets.

package runtime

import "internal/bid"

func dadd128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	return to128(bid.AddSlow128(from128(xhi, xlo), from128(yhi, ylo), bid.ToNearestEven))
}

func dsub128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	return to128(bid.SubSlow128(from128(xhi, xlo), from128(yhi, ylo), bid.ToNearestEven))
}

func dmul128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	return to128(bid.MulSlow128(from128(xhi, xlo), from128(yhi, ylo), bid.ToNearestEven))
}

func deq128(xhi, xlo, yhi, ylo uint64) bool {
	return bid.EqualSlow128(from128(xhi, xlo), from128(yhi, ylo))
}

func dlt128(xhi, xlo, yhi, ylo uint64) bool {
	return bid.LessSlow128(from128(xhi, xlo), from128(yhi, ylo))
}

func dle128(xhi, xlo, yhi, ylo uint64) bool {
	return bid.LessEqualSlow128(from128(xhi, xlo), from128(yhi, ylo))
}
