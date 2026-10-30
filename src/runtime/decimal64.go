// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Soft-decimal entry points for decimal64, the counterpart of
// softfloat64.go. The compiler lowers the decimal64 operators to calls to
// these functions (see softDecimalOps in cmd/compile/internal/ssagen), which
// pass a decimal64 as its BID bit pattern. All of the arithmetic is in
// internal/bid. The language rounds to nearest, ties to even.

package runtime

import (
	"internal/bid"
	"unsafe"
)

func dadd64(x, y uint64) uint64 { return bid.Add64(x, y, bid.ToNearestEven) }
func dsub64(x, y uint64) uint64 { return bid.Sub64(x, y, bid.ToNearestEven) }
func dmul64(x, y uint64) uint64 { return bid.Mul64(x, y, bid.ToNearestEven) }
func ddiv64(x, y uint64) uint64 { return bid.Div64(x, y, bid.ToNearestEven) }

func deq64(x, y uint64) bool { return bid.Equal64(x, y) }
func dlt64(x, y uint64) bool { return bid.Less64(x, y) }
func dle64(x, y uint64) bool { return bid.LessEqual64(x, y) }

func dmin64(x, y uint64) uint64 { return bid.Min64(x, y) }
func dmax64(x, y uint64) uint64 { return bid.Max64(x, y) }

// printdecimal64 prints x for the builtin print and println, in the character
// form of internal/bid, which preserves the quantum.
func printdecimal64(x decimal64) {
	var buf [bid.MaxLen64]byte
	gwrite(bid.Append64(buf[:0], *(*uint64)(unsafe.Pointer(&x))))
}
