// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ir

import (
	"go/constant"
	"go/token"
	"internal/bid"
	"internal/constdecimal"
	"strings"

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

// LiteralFloat64Val returns the value of the source literal of the float64
// constant c, with its quantum and the sign of c, if c.Val() is that value
// rounded to float64, and otherwise c.Val(). It is for the conversion T(0.1)
// to a type parameter T: by the time the operand has been read it has its
// default type, float64, and the IR holds its value rounded to that type, but
// the spec converts the constant. T(float64(0.1)) looks the same in the IR and
// takes this path too.
func LiteralFloat64Val(c Node) constant.Value {
	val := c.Val()
	b, ok := c.(*BasicLit)
	if !ok {
		return val
	}
	exact := constant.MakeFromLiteral(strings.ReplaceAll(b.OrigLit(), "_", ""), token.FLOAT, 0)
	if exact.Kind() != constant.Float {
		return val
	}
	want, _ := constant.Float64Val(val)
	got, _ := constant.Float64Val(exact)
	if want != got && want != -got {
		return val
	}
	if constant.Sign(val) < 0 {
		return constant.UnaryOp(token.SUB, exact, 0)
	}
	return exact
}
