// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Soft-decimal entry points for decimal128; see decimal64.go. A decimal128
// passes as the two uint64 halves of its BID bit pattern, high first.

package runtime

import (
	"internal/bid"
	"unsafe"
)

func dadd128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	z := bid.Add128(bid.Uint128{Hi: xhi, Lo: xlo}, bid.Uint128{Hi: yhi, Lo: ylo}, bid.ToNearestEven)
	return z.Hi, z.Lo
}

func dsub128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	z := bid.Sub128(bid.Uint128{Hi: xhi, Lo: xlo}, bid.Uint128{Hi: yhi, Lo: ylo}, bid.ToNearestEven)
	return z.Hi, z.Lo
}

func dmul128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	z := bid.Mul128(bid.Uint128{Hi: xhi, Lo: xlo}, bid.Uint128{Hi: yhi, Lo: ylo}, bid.ToNearestEven)
	return z.Hi, z.Lo
}

func ddiv128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	z := bid.Div128(bid.Uint128{Hi: xhi, Lo: xlo}, bid.Uint128{Hi: yhi, Lo: ylo}, bid.ToNearestEven)
	return z.Hi, z.Lo
}

func deq128(xhi, xlo, yhi, ylo uint64) bool {
	return bid.Equal128(bid.Uint128{Hi: xhi, Lo: xlo}, bid.Uint128{Hi: yhi, Lo: ylo})
}

func dlt128(xhi, xlo, yhi, ylo uint64) bool {
	return bid.Less128(bid.Uint128{Hi: xhi, Lo: xlo}, bid.Uint128{Hi: yhi, Lo: ylo})
}

func dle128(xhi, xlo, yhi, ylo uint64) bool {
	return bid.LessEqual128(bid.Uint128{Hi: xhi, Lo: xlo}, bid.Uint128{Hi: yhi, Lo: ylo})
}

func dmin128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	z := bid.Min128(bid.Uint128{Hi: xhi, Lo: xlo}, bid.Uint128{Hi: yhi, Lo: ylo})
	return z.Hi, z.Lo
}

func dmax128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	z := bid.Max128(bid.Uint128{Hi: xhi, Lo: xlo}, bid.Uint128{Hi: yhi, Lo: ylo})
	return z.Hi, z.Lo
}

// d128load returns the bit pattern of the decimal128 at p. A decimal128 is
// stored as its low half followed by its high half, on every target (see
// dec.rules in cmd/compile/internal/ssa/_gen).
func d128load(p unsafe.Pointer) bid.Uint128 {
	x := (*[2]uint64)(p)
	return bid.Uint128{Hi: x[1], Lo: x[0]}
}

// printdecimal128 prints x for the builtin print and println, in the
// character form of internal/bid, which preserves the quantum.
func printdecimal128(x decimal128) {
	var buf [bid.MaxLen128]byte
	gwrite(bid.Append128(buf[:0], d128load(unsafe.Pointer(&x))))
}
