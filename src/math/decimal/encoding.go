// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package decimal

import (
	"internal/bid"
	impl "internal/decimal"
)

// Representation identifies the stored representation of a decimal value.
// It describes the words returned by math.Decimal64bits and math.Decimal128bits,
// rather than an interchange encoding. Both representations XOR the native
// IEEE encoding with the encoding of 0E+0 so that all-zero memory is Go's zero.
type Representation uint8

const (
	// BIDXOR is IEEE BID XORed with 0x31c0000000000000 for decimal64,
	// or 0x3040000000000000 in the high word for decimal128.
	BIDXOR Representation = Representation(bid.EncodingBID)
	// DPDXOR is IEEE DPD XORed with 0x2238000000000000 for decimal64,
	// or 0x2208000000000000 in the high word for decimal128.
	DPDXOR Representation = Representation(bid.EncodingDPD)
	// NativeRepresentation is the representation used by the current target.
	// It is DPDXOR on ppc64le and BIDXOR on the other supported targets.
	NativeRepresentation Representation = Representation(bid.NativeEncoding)
)

// BID64bits returns the IEEE 754 BID encoding of d, without the zero-value XOR.
// If the native representation uses DPD, this converts the value to canonical
// BID, preserving its sign, quantum and NaN payload. If it uses BID, every
// encoding bit is preserved. For exact stored bits, use math.Decimal64bits.
func BID64bits(d decimal64) uint64 { return impl.BID64bits(d) }

// BID64frombits returns the decimal64 denoted by the IEEE 754 BID encoding b.
// It applies the native zero-value XOR. If the native representation uses DPD,
// it converts to canonical DPD; non-canonical BID encodings are interpreted as
// their IEEE values. If it uses BID, every encoding bit is preserved.
func BID64frombits(b uint64) decimal64 { return impl.BID64frombits(b) }

// BID128bits is BID64bits for decimal128. It returns the high word first,
// independently of the target's byte order.
func BID128bits(d decimal128) (hi, lo uint64) { return impl.BID128bits(d) }

// BID128frombits is BID64frombits for decimal128, with the high word first.
func BID128frombits(hi, lo uint64) decimal128 { return impl.BID128frombits(hi, lo) }

// DPD64bits returns the IEEE 754 DPD encoding of d, without the zero-value XOR.
// If the native representation uses BID, this converts the value to canonical
// DPD, preserving its sign, quantum and NaN payload. If it uses DPD, every
// encoding bit is preserved. For exact stored bits, use math.Decimal64bits.
func DPD64bits(d decimal64) uint64 { return impl.DPD64bits(d) }

// DPD64frombits returns the decimal64 denoted by the IEEE 754 DPD encoding b.
// It applies the native zero-value XOR. If the native representation uses BID,
// it converts to canonical BID; redundant DPD declets and reserved bits are
// interpreted as their IEEE values. If it uses DPD, every encoding bit survives.
func DPD64frombits(b uint64) decimal64 { return impl.DPD64frombits(b) }

// DPD128bits is DPD64bits for decimal128. It returns the high word first,
// independently of the target's byte order.
func DPD128bits(d decimal128) (hi, lo uint64) { return impl.DPD128bits(d) }

// DPD128frombits is DPD64frombits for decimal128, with the high word first.
func DPD128frombits(hi, lo uint64) decimal128 { return impl.DPD128frombits(hi, lo) }
