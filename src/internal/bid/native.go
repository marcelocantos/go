// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !ppc64le

package bid

// The native encoding of the target this package is compiled for: the
// encoding that a decimal value in memory holds once the zero mask is
// removed. It is BID on every target but linux/ppc64le (see
// native_ppc64le.go). The names are for code compiled for the target, the
// runtime and math; cmd/compile, which runs on one target and compiles for
// another, chooses by its target instead (cmd/compile/internal/ir).

// NativeDPD reports whether the native encoding is DPD.
const NativeDPD = false

// NativeZero64 and NativeZero128Hi are the native encodings of 0E+0, the
// masks that make all bits clear the zero value in memory.
const (
	NativeZero64    = Zero64
	NativeZero128Hi = Zero128Hi
)

// FromNative64 returns the BID encoding of the native encoding x.
func FromNative64(x uint64) uint64 { return x }

// ToNative64 returns the native encoding of the BID encoding x. On a DPD
// target it is the canonical encoding of the value x reads as.
func ToNative64(x uint64) uint64 { return x }

// FromNative128 is FromNative64 for decimal128.
func FromNative128(x Uint128) Uint128 { return x }

// ToNative128 is ToNative64 for decimal128.
func ToNative128(x Uint128) Uint128 { return x }
