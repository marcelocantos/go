// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Soft-decimal entry points for decimal128; see decimal64.go. A decimal128
// passes as the two uint64 halves of its BID bit pattern, high first. The
// operators that the compiler also lowers to integer fast paths, + - * and
// the comparisons, are in decimal128_inline.go and decimal128_call.go.

package runtime

import (
	"internal/bid"
	"unsafe"
)

func ddiv128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	z := bid.Div128(from128(xhi, xlo), from128(yhi, ylo), bid.ToNearestEven)
	return to128(z)
}

func dmin128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	z := bid.Min128(from128(xhi, xlo), from128(yhi, ylo))
	return to128(z)
}

func dmax128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	z := bid.Max128(from128(xhi, xlo), from128(yhi, ylo))
	return to128(z)
}

// d128load returns the BID bit pattern of the decimal128 at p. A decimal128 is
// stored as its low half followed by its high half, on every target (see
// dec.rules in cmd/compile/internal/ssa/_gen), and its high half is XORed with
// the high word of the encoding of 0E+0, so that all bits clear is the zero
// value.
func d128load(p unsafe.Pointer) bid.Uint128 {
	x := (*[2]uint64)(p)
	return from128(x[1]^bid.NativeZero128Hi, x[0])
}

// from128 and to128 convert between the native encoding, as its halves, and
// the BID encoding.
func from128(hi, lo uint64) bid.Uint128 { return bid.FromNative128(bid.Uint128{Hi: hi, Lo: lo}) }

func to128(x bid.Uint128) (hi, lo uint64) {
	x = bid.ToNative128(x)
	return x.Hi, x.Lo
}

// printdecimal128 prints x for the builtin print and println, in the
// character form of internal/bid, which preserves the quantum.
func printdecimal128(x decimal128) {
	var buf [bid.MaxLen128]byte
	gwrite(bid.Append128(buf[:0], d128load(unsafe.Pointer(&x))))
}
