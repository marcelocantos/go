// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// On linux/ppc64le the native encoding is DPD, the encoding of POWER's
// decimal floating-point unit, so that the compiler can lower the decimal
// operators to its instructions with no conversion. See native.go.

// NativeEncoding identifies the encoding before the zero mask is applied.
const NativeEncoding = EncodingDPD

const NativeDPD = NativeEncoding == EncodingDPD

const (
	NativeZero64    = DPDZero64
	NativeZero128Hi = DPDZero128Hi
)

func FromNative64(x uint64) uint64 { return BIDFromDPD64(x) }

func ToNative64(x uint64) uint64 { return DPDFromBID64(x) }

func FromNative128(x Uint128) Uint128 { return BIDFromDPD128(x) }

func ToNative128(x Uint128) Uint128 { return DPDFromBID128(x) }
