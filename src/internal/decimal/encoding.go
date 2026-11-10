// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package decimal

import (
	"internal/bid"
	"unsafe"
)

// BID64bits returns the IEEE 754 BID encoding of d, without the zero-value XOR.
// If the native representation uses DPD, this converts the value to canonical
// BID, preserving its sign, quantum and NaN payload. If it uses BID, every
// encoding bit is preserved. Bits64 gives the stored bits.
func BID64bits(d decimal64) uint64 {
	return bid.FromNative64(Bits64(d) ^ bid.NativeZero64)
}

// BID64frombits returns the decimal64 denoted by the IEEE 754 BID encoding b.
// It applies the native zero-value XOR. If the native representation uses DPD,
// it converts to canonical DPD; non-canonical BID encodings are interpreted as
// their IEEE values. If it uses BID, every encoding bit is preserved.
func BID64frombits(b uint64) decimal64 {
	return FromBits64(bid.ToNative64(b) ^ bid.NativeZero64)
}

// BID128bits is BID64bits for decimal128. It returns the high word first,
// independently of the target's byte order.
func BID128bits(d decimal128) (hi, lo uint64) {
	hi, lo = Bits128(d)
	x := bid.FromNative128(bid.Uint128{Hi: hi ^ bid.NativeZero128Hi, Lo: lo})
	return x.Hi, x.Lo
}

// BID128frombits is BID64frombits for decimal128, with the high word first.
func BID128frombits(hi, lo uint64) decimal128 {
	x := bid.ToNative128(bid.Uint128{Hi: hi, Lo: lo})
	return FromBits128(x.Hi^bid.NativeZero128Hi, x.Lo)
}

// DPD64bits returns the IEEE 754 DPD encoding of d, without the zero-value XOR.
// If the native representation uses BID, this converts the value to canonical
// DPD, preserving its sign, quantum and NaN payload. If it uses DPD, every
// encoding bit is preserved. Bits64 gives the stored bits.
func DPD64bits(d decimal64) uint64 {
	if bid.NativeDPD {
		return Bits64(d) ^ bid.NativeZero64
	}
	return bid.DPDFromBID64(BID64bits(d))
}

// DPD64frombits returns the decimal64 denoted by the IEEE 754 DPD encoding b.
// It applies the native zero-value XOR. If the native representation uses BID,
// it converts to canonical BID; redundant DPD declets and reserved bits are
// interpreted as their IEEE values. If it uses DPD, every encoding bit survives.
func DPD64frombits(b uint64) decimal64 {
	if bid.NativeDPD {
		return FromBits64(b ^ bid.NativeZero64)
	}
	return BID64frombits(bid.BIDFromDPD64(b))
}

// DPD128bits is DPD64bits for decimal128. It returns the high word first,
// independently of the target's byte order.
func DPD128bits(d decimal128) (hi, lo uint64) {
	if bid.NativeDPD {
		hi, lo = Bits128(d)
		return hi ^ bid.NativeZero128Hi, lo
	}
	hi, lo = BID128bits(d)
	x := bid.DPDFromBID128(bid.Uint128{Hi: hi, Lo: lo})
	return x.Hi, x.Lo
}

// DPD128frombits is DPD64frombits for decimal128, with the high word first.
func DPD128frombits(hi, lo uint64) decimal128 {
	if bid.NativeDPD {
		return FromBits128(hi^bid.NativeZero128Hi, lo)
	}
	x := bid.BIDFromDPD128(bid.Uint128{Hi: hi, Lo: lo})
	return BID128frombits(x.Hi, x.Lo)
}

// Bits64 and FromBits64 copy the native stored representation.
func Bits64(d decimal64) uint64     { return *(*uint64)(unsafe.Pointer(&d)) }
func FromBits64(b uint64) decimal64 { return *(*decimal64)(unsafe.Pointer(&b)) }
func Bits128(d decimal128) (hi, lo uint64) {
	p := (*[2]uint64)(unsafe.Pointer(&d))
	return p[1], p[0]
}
func FromBits128(hi, lo uint64) decimal128 {
	var d decimal128
	p := (*[2]uint64)(unsafe.Pointer(&d))
	p[0], p[1] = lo, hi
	return d
}
