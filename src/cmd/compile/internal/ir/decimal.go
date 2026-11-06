// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ir

import (
	"internal/bid"
	"internal/buildcfg"
	"internal/constdecimal"

	"cmd/compile/internal/types"
)

// DecimalDPD reports whether the target's native decimal encoding is DPD
// rather than BID: whether a decimal value in memory is its DPD encoding
// XORed with the DPD encoding of 0E+0. That is so on linux/ppc64le, whose
// decimal floating-point unit works on DPD and to whose instructions the
// compiler lowers the decimal operators. It is the target's choice, so the
// compiler decides it here and never through internal/bid's Native names,
// which describe the target internal/bid itself was compiled for.
func DecimalDPD() bool { return buildcfg.GOARCH == "ppc64le" }

// DecimalZero64 and DecimalZero128Hi return the target's native encodings
// of 0E+0, the masks between a value's native encoding and its memory form.
func DecimalZero64() uint64 {
	if DecimalDPD() {
		return bid.DPDZero64
	}
	return bid.Zero64
}

func DecimalZero128Hi() uint64 {
	if DecimalDPD() {
		return bid.DPDZero128Hi
	}
	return bid.Zero128Hi
}

// DecimalConst returns the numeric constant c as the decimal type t, in words
// of its memory form: the bits are lo for decimal64, with hi zero, and hi, lo
// for decimal128. The memory form is the target's native encoding XORed with
// the native encoding of 0E+0, so that all bits clear is the zero value; see
// DecimalDPD. The constant's value and
// quantum choose the encoding, as internal/constdecimal describes, so the
// compiler and the type checkers agree on it.
//
// A value too large for t encodes as an infinity, which is the kernel's
// default result. The type checker has already rejected a source constant
// above the largest finite value of t.
func DecimalConst(t *types.Type, c Node) (lo, hi uint64) {
	if t.Kind() == types.TDECIMAL128 {
		bits, _ := constdecimal.Encode(c.Val(), constdecimal.Decimal128)
		if DecimalDPD() {
			bits = bid.DPDFromBID128(bits)
		}
		return bits.Lo, bits.Hi ^ DecimalZero128Hi()
	}
	bits, _ := constdecimal.Encode(c.Val(), constdecimal.Decimal64)
	if DecimalDPD() {
		bits.Lo = bid.DPDFromBID64(bits.Lo)
	}
	return bits.Lo ^ DecimalZero64(), 0
}
