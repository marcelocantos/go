// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Soft-decimal entry points for decimal128; see decimal64.go. A decimal128
// passes as the two uint64 halves of its BID bit pattern, high first. The
// operators that the compiler also lowers to integer fast paths, + - * and
// the comparisons, are in decimal128_call.go for the targets without those
// paths. On amd64 and arm64 the compiler calls internal/bid for the
// operands its fast paths refuse, and for division, with no function here
// between them (cmd/compile/internal/ssagen.ubdKernelOp).

package runtime

import (
	"internal/bid"
	"unsafe"
)

func ddiv128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	return fromUBD128(bid.UBDDiv128(ubd128(xhi, xlo), ubd128(yhi, ylo)))
}

func dmin128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	return fromUBD128(bid.UBDMin128(ubd128(xhi, xlo), ubd128(yhi, ylo)))
}

func dmax128(xhi, xlo, yhi, ylo uint64) (hi, lo uint64) {
	return fromUBD128(bid.UBDMax128(ubd128(xhi, xlo), ubd128(yhi, ylo)))
}

// d128words returns the decimal128 at p in UBD. A decimal128 is stored as
// its low half followed by its high half, on every target (see dec.rules in
// cmd/compile/internal/ssa/_gen), and its high half is XORed with the high
// word of the native encoding of 0E+0, so that all bits clear is the zero
// value; UBD's mask is zero.
func d128words(p unsafe.Pointer) bid.UBD128 {
	x := (*[2]uint64)(p)
	return ubd128(x[1]^bid.NativeZero128Hi, x[0])
}

// ubd128 and fromUBD128 convert between the native encoding, as its halves,
// and UBD, the encoding of the kernel's decimal128 arithmetic. Where
// decimal128 is stored as UBD (bid.NativeUBD128) they are the halves
// themselves; on a DPD target they convert through BID. bid.NativeUBD128 is
// a constant, so each is one of its two branches.
func ubd128(hi, lo uint64) bid.UBD128 {
	if !bid.NativeUBD128 {
		u := bid.UBDFromBID128(from128(hi, lo))
		hi, lo = u.Hi, u.Lo
	}
	return bid.UBD128{Lo: lo, Hi: hi}
}

func fromUBD128(x bid.UBD128) (hi, lo uint64) {
	if !bid.NativeUBD128 {
		return to128(bid.BIDFromUBD128(bid.Uint128{Hi: x.Hi, Lo: x.Lo}))
	}
	return x.Hi, x.Lo
}

// from128 and to128 convert between the native encoding, as its halves, and
// the BID encoding.
func from128(hi, lo uint64) bid.Uint128 { return bid.FromNative128(bid.Uint128{Hi: hi, Lo: lo}) }

func to128(x bid.Uint128) (hi, lo uint64) {
	x = bid.ToNative128(x)
	return x.Hi, x.Lo
}

// printdecimal128 prints x for the builtin print and println, in the form
// of internal/bid's Append128: the one %v and strconv's 'g' at precision -1
// write, which keeps the quantum. A float64 prints its %v form too.
func printdecimal128(x decimal128) {
	var buf [bid.MaxLen128]byte
	gwrite(bid.UBDAppend128(buf[:0], d128words(unsafe.Pointer(&x))))
}
