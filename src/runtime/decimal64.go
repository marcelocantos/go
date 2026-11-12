// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Soft-decimal entry points for decimal64, the counterpart of
// softfloat64.go. The compiler lowers the decimal64 arithmetic operators to
// calls to these functions, and the comparisons too except on arm64 and
// amd64, where it compares inline and calls here only for the operands its
// fast path refuses (see softDecimalOps and bidCompare in
// cmd/compile/internal/ssagen). The functions
// pass a decimal64 as its native bit pattern, having removed the zero mask of
// its memory form. The native encoding is BID except on linux/ppc64le, where
// it is DPD and the compiler calls only the functions that the hardware
// cannot replace (see internal/bid's native.go); FromNative64 and ToNative64
// convert, and are the identity where the native encoding is BID. All of the
// arithmetic is in internal/bid. The language rounds to nearest, ties to
// even.

package runtime

import (
	"internal/bid"
	"unsafe"
)

func dadd64(x, y uint64) uint64 { return to64(bid.Add64(from64(x), from64(y), bid.ToNearestEven)) }
func dsub64(x, y uint64) uint64 { return to64(bid.Sub64(from64(x), from64(y), bid.ToNearestEven)) }
func dmul64(x, y uint64) uint64 { return to64(bid.Mul64(from64(x), from64(y), bid.ToNearestEven)) }
func ddiv64(x, y uint64) uint64 { return to64(bid.Div64(from64(x), from64(y), bid.ToNearestEven)) }

func deq64(x, y uint64) bool { return bid.Equal64(from64(x), from64(y)) }
func dlt64(x, y uint64) bool { return bid.Less64(from64(x), from64(y)) }
func dle64(x, y uint64) bool { return bid.LessEqual64(from64(x), from64(y)) }

func dmin64(x, y uint64) uint64 { return to64(bid.Min64(from64(x), from64(y))) }
func dmax64(x, y uint64) uint64 { return to64(bid.Max64(from64(x), from64(y))) }

// from64 and to64 convert between the native and BID encodings.
func from64(x uint64) uint64 { return bid.FromNative64(x) }
func to64(x uint64) uint64   { return bid.ToNative64(x) }

// The native encodings of 0E+0, which the memory form of a decimal is XORed
// with. They are not used here: they are for debuggers, which read them
// from the binary's DWARF (see the decimal printer in runtime-gdb.py) both
// to undo the XOR and to tell the BID encoding from the DPD one.
const (
	_DecimalZero64    uint64 = bid.NativeZero64
	_DecimalZero128Hi uint64 = bid.NativeZero128Hi
)

// d64load returns the BID bit pattern of the decimal64 at p. In memory a
// decimal64 is its native encoding XORed with the native encoding of 0E+0,
// so that all bits clear is the zero value.
func d64load(p unsafe.Pointer) uint64 {
	return from64(*(*uint64)(p) ^ bid.NativeZero64)
}

// printdecimal64 prints x for the builtin print and println, in the form
// of internal/bid's Append64: the one %v and strconv write, which keeps the
// quantum.
func printdecimal64(x decimal64) {
	var buf [bid.MaxLen64]byte
	gwrite(bid.Append64(buf[:0], d64load(unsafe.Pointer(&x))))
}
