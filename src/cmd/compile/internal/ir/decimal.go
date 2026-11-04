// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ir

import (
	"internal/bid"
	"internal/constdecimal"

	"cmd/compile/internal/types"
)

// DecimalConst returns the numeric constant c as the decimal type t, in words
// of its memory form: the bits are lo for decimal64, with hi zero, and hi, lo
// for decimal128. The memory form is the BID encoding XORed with the encoding
// of 0E+0, so that all bits clear is the zero value. The constant's value and
// quantum choose the encoding, as internal/constdecimal describes, so the
// compiler and the type checkers agree on it.
//
// A value too large for t encodes as an infinity, which is the kernel's
// default result. The type checker has already rejected a source constant
// above the largest finite value of t.
func DecimalConst(t *types.Type, c Node) (lo, hi uint64) {
	if t.Kind() == types.TDECIMAL128 {
		bits, _ := constdecimal.Encode(c.Val(), constdecimal.Decimal128)
		return bits.Lo, bits.Hi ^ bid.Zero128Hi
	}
	bits, _ := constdecimal.Encode(c.Val(), constdecimal.Decimal64)
	return bits.Lo ^ bid.Zero64, 0
}
