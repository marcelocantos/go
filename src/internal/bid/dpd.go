// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// The densely packed decimal (DPD) encodings of decimal64 and decimal128
// (IEEE 754-2019 clause 3.5.2, the "decimal" encoding of the trailing
// significand field), which the decimal floating-point units of POWER and
// IBM Z operate on. On those targets a decimal value in memory is its DPD
// encoding XORed with DPDZero64 or DPDZero128Hi, as it is its BID encoding
// XORed with Zero64 or Zero128Hi everywhere else, and the conversions below
// run where the BID encoding is needed: in explicit codecs and
// around the kernel calls that remain. The kernel itself stays BID only.
//
// A DPD encoding has the same sign bit and the same combination-field
// prefix for the special values as a BID encoding: bits 58 to 62 (of the
// high word, for decimal128) are 11110 for an infinity and 11111 for a NaN,
// and bit 57 is the signaling bit. For a finite value the combination field
// holds the two leading exponent bits and the leading coefficient digit,
// the exponent continuation field the rest of the biased exponent, and the
// trailing significand field the other coefficient digits, three to each
// 10-bit declet. A NaN's payload is its trailing significand field read as
// declets.
//
// Every DPD bit pattern reads as a canonical value: each of the 1024
// declets decodes to three decimal digits, and the coefficient of a finite
// decimal64 or decimal128 can have at most Precision digits, so no DPD
// coefficient is out of range as a BID one can be. The non-canonical DPD
// encodings are the 24 redundant declets (which decode to digits 8 and 9
// and have canonical twins), non-zero reserved bits in an infinity or a
// NaN, and non-zero bits in an infinity's trailing field (clause 3.5.2).
//
// The conversions read their operand as the value clause 3.5.2 assigns it,
// exactly as every operation of this package does ("Non-canonical encodings
// read as their canonical value" in the package documentation), and return
// the canonical encoding of that value: sign, exponent and coefficient, or
// sign, kind and payload for the special values. They are therefore exact
// inverses on canonical encodings, and canonicalize everything else: a BID
// encoding with an out-of-range coefficient becomes the DPD zero with its
// sign and exponent, and a BID NaN payload of 10^(Precision-1) or more
// becomes zero, so the round trip BID → DPD → BID returns the canonical
// form of a non-canonical BID operand rather than its bits.

// Encodings used by the native representation, before its zero mask.
const (
	EncodingBID = iota
	EncodingDPD
)

// DPDZero64 is the DPD encoding of 0E+0 in decimal64, and DPDZero128Hi the
// high word of 0E+0 in decimal128, whose low word is zero. They have the
// role Zero64 and Zero128Hi have for BID.
const (
	DPDZero64    uint64 = 0x2238000000000000
	DPDZero128Hi uint64 = 0x2208000000000000
)

// DPD field layout. The exponent continuation field follows the five
// combination bits; the trailing significand field is the low 50 bits of a
// decimal64 and the low 110 bits of a decimal128.
const (
	dpdExpContShift64    = 50
	dpdExpContMask64     = 1<<8 - 1
	dpdTrailingMask64    = 1<<50 - 1
	dpdExpContShift128   = 46 // within Hi
	dpdExpContMask128    = 1<<12 - 1
	dpdTrailingMask128Hi = 1<<46 - 1
	dpdCombShift         = 58
	dpdDecletMask        = 1<<10 - 1
	pow10_15             = 1_000_000_000_000_000
	pow10_18             = 1_000_000_000_000_000_000
)

// dpdEncode15 returns the five declets of the 15 low-order decimal digits
// of c, which must be below 10^15, as the low 50 bits of the result.
func dpdEncode15(c uint64) uint64 {
	// Two independent chains of divisions by 1000, below and above 10^9,
	// rather than one of five: each division by a constant is a multiply,
	// and the chain's length is the latency.
	hi, lo := c/1_000_000_000, c%1_000_000_000
	return dpdEncode9(lo) | uint64(bin2dpd[hi%1000])<<30 | uint64(bin2dpd[hi/1000])<<40
}

// dpdEncode9 returns the three declets of c, which must be below 10^9, as
// the low 30 bits of the result.
func dpdEncode9(c uint64) uint64 {
	return uint64(bin2dpd[c%1000]) | uint64(bin2dpd[c/1000%1000])<<10 | uint64(bin2dpd[c/1_000_000])<<20
}

// dpdDecode15 is the inverse of dpdEncode15: the integer below 10^15 whose
// digits the five declets in the low 50 bits of d hold.
func dpdDecode15(d uint64) uint64 {
	// Independent products rather than a chain of five multiply-adds; see
	// dpdEncode15.
	hi := uint64(dpd2bin[d>>40&dpdDecletMask])*1000 + uint64(dpd2bin[d>>30&dpdDecletMask])
	return hi*1_000_000_000 + dpdDecode9(d)
}

// dpdDecode9 is the inverse of dpdEncode9: the integer below 10^9 whose
// digits the three declets in the low 30 bits of d hold.
func dpdDecode9(d uint64) uint64 {
	return uint64(dpd2bin[d>>20&dpdDecletMask])*1_000_000 + uint64(dpd2bin[d>>10&dpdDecletMask])*1000 + uint64(dpd2bin[d&dpdDecletMask])
}

// dpdComb returns the combination field for the leading digit lead and the
// two leading bits top of the biased exponent.
func dpdComb(lead, top uint64) uint64 {
	if lead < 8 {
		return top<<3 | lead
	}
	return 0b11000 | top<<1 | lead&1
}

// dpdSplitComb is the inverse of dpdComb for a finite value's
// combination field comb.
func dpdSplitComb(comb uint64) (lead, top uint64) {
	if comb>>3 != 3 {
		return comb & 7, comb >> 3
	}
	return 8 | comb&1, comb >> 1 & 3
}

// DPDFromBID64 returns the canonical DPD encoding of the decimal64 that
// the BID encoding x reads as.
func DPDFromBID64(x uint64) uint64 {
	sign := x & SignMask64
	kind, _, coeff, exp := Unpack64(x)
	switch kind {
	case KindInf:
		return sign | Inf64
	case KindNaN:
		p := x & nanPayloadMask64
		if p >= maxNaNPayload64 {
			p = 0
		}
		return sign | x&(NaN64|SignalingBit64) | dpdEncode15(p)
	}
	biased := uint64(exp + Bias64)
	lead := coeff / pow10_15
	return sign | dpdComb(lead, biased>>8)<<dpdCombShift |
		(biased&dpdExpContMask64)<<dpdExpContShift64 | dpdEncode15(coeff%pow10_15)
}

// BIDFromDPD64 returns the canonical BID encoding of the decimal64 that
// the DPD encoding x reads as.
func BIDFromDPD64(x uint64) uint64 {
	sign := x & SignMask64
	comb := x >> dpdCombShift & 0x1F
	switch comb {
	case 0x1E:
		return sign | Inf64
	case 0x1F:
		return sign | x&(NaN64|SignalingBit64) | dpdDecode15(x&dpdTrailingMask64)
	}
	lead, top := dpdSplitComb(comb)
	biased := top<<8 | x>>dpdExpContShift64&dpdExpContMask64
	coeff := lead*pow10_15 + dpdDecode15(x&dpdTrailingMask64)
	r, _ := Pack64(sign != 0, coeff, int(biased)-Bias64)
	return r
}

// DPDFromBID128 is DPDFromBID64 for decimal128.
func DPDFromBID128(x Uint128) Uint128 {
	sign := x.Hi & SignMask128Hi
	kind, _, coeff, exp := Unpack128(x)
	var head uint64 // the high word's bits above the trailing field
	switch kind {
	case KindInf:
		return Uint128{Hi: sign | Inf128Hi}
	case KindNaN:
		head = sign | x.Hi&(NaN128Hi|SignalingBit128Hi)
		coeff = Uint128{Hi: x.Hi & nanPayloadMask128Hi, Lo: x.Lo}
		if !u128Lt(coeff, maxNaNPayload128) {
			coeff = Uint128{}
		}
	}
	// Split the coefficient (below 10^34, or a payload below 10^33) at
	// 10^18: q < 10^16 holds the leading 16 digits and r < 10^18 the
	// trailing 18, by the reciprocal division of recip128.go.
	qw, rw := divPow10Recip128(coeff, 18)
	q, r := qw.Lo, rw.Lo
	lead := q / pow10_15
	// Declets 0 to 5 hold r and declets 6 to 10 the low 15 digits of q.
	lo6 := dpdEncode9(r%1_000_000_000) | dpdEncode9(r/1_000_000_000)<<30
	t := u128Or(u128Shl(Uint128{Lo: dpdEncode15(q % pow10_15)}, 60), Uint128{Lo: lo6})
	if kind == KindFinite {
		biased := uint64(exp + Bias128)
		head = sign | dpdComb(lead, biased>>12)<<dpdCombShift | (biased&dpdExpContMask128)<<dpdExpContShift128
	}
	return Uint128{Hi: head | t.Hi, Lo: t.Lo}
}

// BIDFromDPD128 is BIDFromDPD64 for decimal128.
func BIDFromDPD128(x Uint128) Uint128 {
	sign := x.Hi & SignMask128Hi
	comb := x.Hi >> dpdCombShift & 0x1F
	if comb == 0x1E {
		return Uint128{Hi: sign | Inf128Hi}
	}
	// The trailing field's 33 digits: the high 15 in declets 6 to 10 and
	// the low 18 in declets 0 to 5.
	t := Uint128{Hi: x.Hi & dpdTrailingMask128Hi, Lo: x.Lo}
	lo18 := dpdDecode9(t.Lo>>30)*1_000_000_000 + dpdDecode9(t.Lo) // declets 0 to 5 are bits 0 to 59
	hi15 := dpdDecode15(u128Shr(t, 60).Lo)
	if comb == 0x1F {
		hi, lo := bits.Mul64(hi15, pow10_18)
		lo, c := bits.Add64(lo, lo18, 0)
		return Uint128{Hi: sign | x.Hi&(NaN128Hi|SignalingBit128Hi) | hi + c, Lo: lo}
	}
	lead, top := dpdSplitComb(comb)
	biased := top<<12 | x.Hi>>dpdExpContShift128&dpdExpContMask128
	hi, lo := bits.Mul64(lead*pow10_15+hi15, pow10_18)
	lo, c := bits.Add64(lo, lo18, 0)
	r, _ := Pack128(sign != 0, Uint128{Hi: hi + c, Lo: lo}, int(biased)-Bias128)
	return r
}
