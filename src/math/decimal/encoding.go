// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package decimal

import (
	"internal/bid"
	impl "internal/decimal"
	"unsafe"
)

// Representation identifies the stored representation of a decimal value.
// It describes the words returned by [Native64bits] and [Native128bits],
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
// encoding bit is preserved. For exact stored bits, use [Native64bits].
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
// Where decimal128 is stored as UBD (every target but ppc64le), a
// non-canonical encoding is stored as the value IEEE 754-2019 clause 3.5.2
// item c.2 gives it, a significand above 10^34 - 1 (10^33 - 1 as a NaN's
// payload) being zero, so BID128bits then returns the canonical encoding.
func BID128frombits(hi, lo uint64) decimal128 { return impl.BID128frombits(hi, lo) }

// DPD64bits returns the IEEE 754 DPD encoding of d, without the zero-value XOR.
// If the native representation uses BID, this converts the value to canonical
// DPD, preserving its sign, quantum and NaN payload. If it uses DPD, every
// encoding bit is preserved. For exact stored bits, use [Native64bits].
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

// IsCanonicalBID128 reports whether hi, lo (high word first) is a canonical
// BID encoding of a decimal128 (the isCanonical operation of IEEE 754-2019
// clause 5.7.2 on the interchange format, as clause 3.5.2 defines it). It is
// not when it is:
//
//   - a finite number whose significand is above 10^34 - 1, which is every
//     significand of the second form;
//   - an infinity with any bit set below its five-bit tag;
//   - a NaN with any bit set between the signaling bit and the trailing
//     significand field, or a payload above 10^33 - 1.
//
// [BID128frombits] reads a non-canonical encoding as the value clause
// 3.5.2 gives it. Where decimal128 is stored as UBD, IsCanonicalBID128 is
// the way to tell that an encoding read from outside the program was not
// canonical, as [IsCanonical] is then true for every decimal128.
func IsCanonicalBID128(hi, lo uint64) bool { return impl.IsCanonicalBID128(hi, lo) }

// IsCanonicalDPD128 is IsCanonicalBID128 for the DPD encoding, which is not
// canonical when it is:
//
//   - a finite number with any of the 24 non-canonical declets;
//   - an infinity with any bit set below its five-bit tag;
//   - a NaN with any bit set between the signaling bit and the trailing
//     significand field, or a non-canonical declet in its payload.
//
// [DPD128frombits] reads a non-canonical declet as the value Table 3.3 of
// IEEE 754-2019 gives it.
func IsCanonicalDPD128(hi, lo uint64) bool { return impl.IsCanonicalDPD128(hi, lo) }

// Native64bits returns the exact stored representation of d, including the
// zero-value XOR. The representation is implementation-dependent:
// [NativeRepresentation] names what the bits mean, and the BID and DPD codecs
// convert to and from the interchange encodings. No encoding conversion or
// canonicalization is performed.
// Native64bits(Native64frombits(x)) == x for every x.
func Native64bits(d decimal64) uint64 { return *(*uint64)(unsafe.Pointer(&d)) }

// Native64frombits returns the decimal64 with the exact stored bits b.
// Every bit pattern is preserved, including non-canonical encodings.
// Native64frombits(Native64bits(x)) has exactly the same bits as x.
func Native64frombits(b uint64) decimal64 { return *(*decimal64)(unsafe.Pointer(&b)) }

// Native128bits returns the exact stored representation of d as its high
// and low uint64 words, including the zero-value XOR. See [Native64bits];
// [NativeRepresentation] names what the words mean. The word order of the
// result is independent of the target's byte order.
//
// In the UBD experiment (internal/bid/ubd.go) a decimal128 is stored as
// UBD on every target but ppc64le, with no zero-value XOR, and
// [NativeRepresentation] does not describe it.
func Native128bits(d decimal128) (hi, lo uint64) {
	p := (*[2]uint64)(unsafe.Pointer(&d)) // decimal128 is stored as [lo, hi]
	return p[1], p[0]
}

// Native128frombits returns the decimal128 with the exact stored words hi
// and lo, without encoding conversion. Where decimal128 is stored as UBD,
// a pattern that is not well-formed (canonical) UBD, such as a finite
// exponent outside the format, a coefficient above 10^34 - 1 or a NaN
// payload of 10^33 or more (see internal/bid.UBDWellFormed128), is
// stored as a positive quiet NaN instead: those patterns have no meaning,
// and the operators' inline paths assume they never occur. Every other
// pattern is preserved, and Native128bits(Native128frombits(hi, lo))
// returns (hi, lo).
//
// This function should be removed. UBD is an internal representation that
// is never read from an external source: values enter it only through
// conversions, which produce well-formed patterns, and this function is the
// one way to write arbitrary words into a decimal128 without unsafe. It has
// no purpose that BID128frombits does not serve.
func Native128frombits(hi, lo uint64) decimal128 {
	if bid.NativeUBD128 && !bid.UBDWellFormed128(bid.Uint128{Hi: hi, Lo: lo}) {
		hi, lo = bid.UBDNaNHi, 0
	}
	var d decimal128
	p := (*[2]uint64)(unsafe.Pointer(&d))
	p[0], p[1] = lo, hi
	return d
}
