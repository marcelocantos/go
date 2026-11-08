// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ssagen

import (
	"internal/bid"

	"cmd/compile/internal/ir"
	"cmd/compile/internal/ssa"
	"cmd/compile/internal/ssa/block"
	"cmd/compile/internal/ssa/ssaop"
	"cmd/compile/internal/types"
)

// bidIsZero compares a BID-native decimal with zero using integer SSA.
// Every finite noncanonical encoding also denotes zero. In particular,
// decimal128's second finite form always has an oversized coefficient;
// decimal64's second form can be canonical and needs its coefficient bound.
// The sign and quantum do not affect equality with zero. No memory operation
// or call is needed, and the operand has already been evaluated by s.expr.
func (s *state) bidIsZero(x *ssa.Value) *ssa.Value {
	u64, boolT := types.Types[types.TUINT64], types.Types[types.TBOOL]
	zero := s.constInt64(u64, 0)
	var hi, lo, coeff, noncanonical, form2 *ssa.Value
	if x.Type.Kind() == types.TDECIMAL64 {
		hi = s.newValue2(ssaop.OpXor64, u64, s.newValue1(ssaop.OpCopy, u64, x), s.constInt64(u64, int64(bid.Zero64)))
		mask := s.constInt64(u64, int64(bid.Form2Mask64))
		form2 = s.newValue2(ssaop.OpEq64, boolT, s.newValue2(ssaop.OpAnd64, u64, hi, mask), mask)
		coeff = s.newValue2(ssaop.OpAnd64, u64, hi, s.constInt64(u64, 1<<bid.CoeffBits64-1))
		second := s.newValue2(ssaop.OpAnd64, u64, hi, s.constInt64(u64, 1<<bid.Form2CoeffBits64-1))
		// The implicit 2^53 bit is above the stored coefficient bits.
		limit := s.constInt64(u64, int64(bid.MaxCoeff64-(1<<bid.CoeffBits64)))
		noncanonical = s.newValue2(ssaop.OpAndB, boolT, form2, s.newValue2(ssaop.OpLess64U, boolT, limit, second))
		coeff = s.newValue2(ssaop.OpEq64, boolT, coeff, zero)
		coeff = s.newValue2(ssaop.OpAndB, boolT, s.newValue1(ssaop.OpNot, boolT, form2), coeff)
	} else {
		hi = s.newValue2(ssaop.OpXor64, u64, s.newValue1(ssaop.OpDecimal128Hi, u64, x), s.constInt64(u64, int64(bid.Zero128Hi)))
		lo = s.newValue1(ssaop.OpDecimal128Lo, u64, x)
		mask := s.constInt64(u64, int64(bid.Form2Mask128Hi))
		form2 = s.newValue2(ssaop.OpEq64, boolT, s.newValue2(ssaop.OpAnd64, u64, hi, mask), mask)
		coeff = s.newValue2(ssaop.OpAnd64, u64, hi, s.constInt64(u64, int64(bid.CoeffMask128Hi)))
		maxHi, maxLo := s.constInt64(u64, int64(bid.MaxCoeff128Hi)), s.constInt64(u64, int64(bid.MaxCoeff128Lo))
		noncanonical = s.newValue2(ssaop.OpOrB, boolT,
			s.newValue2(ssaop.OpLess64U, boolT, maxHi, coeff),
			s.newValue2(ssaop.OpAndB, boolT, s.newValue2(ssaop.OpEq64, boolT, coeff, maxHi), s.newValue2(ssaop.OpLess64U, boolT, maxLo, lo)))
		noncanonical = s.newValue2(ssaop.OpOrB, boolT, form2, noncanonical)
		coeff = s.newValue2(ssaop.OpEq64, boolT, s.newValue2(ssaop.OpOr64, u64, coeff, lo), zero)
	}
	// SpecialMask64 is the same field in decimal128's high word.
	special := s.constInt64(u64, int64(bid.SpecialMask64))
	finite := s.newValue2(ssaop.OpNeq64, boolT, s.newValue2(ssaop.OpAnd64, u64, hi, special), special)
	return s.newValue2(ssaop.OpAndB, boolT, finite, s.newValue2(ssaop.OpOrB, boolT, coeff, noncanonical))
}

var bidAddVar = ssaMarker("bidadd")

// bidAdd128 adds same-sign, same-quantum word-sized coefficients directly.
// The equal stored high words establish both sign and quantum equality. A
// zero high coefficient and first-form encoding establish canonical finite
// operands. The coefficient sum is at most 2^65-2, below 10^34, at an already
// representable exponent. This is an exact packing exception: neither rounding
// nor exponent clamping is needed, and even negative zero keeps its sign.
//
// Both operands were evaluated before this method. Only the slow branch calls
// the runtime, with the original operands. The normal variable/memory phis at
// bAfter join that call's memory with the unchanged fast-branch memory.
func (s *state) bidAdd128(rt *types.Type, x, y *ssa.Value) *ssa.Value {
	u64, boolT := types.Types[types.TUINT64], types.Types[types.TBOOL]
	zero := s.constInt64(u64, 0)
	xh := s.newValue1(ssaop.OpDecimal128Hi, u64, x)
	yh := s.newValue1(ssaop.OpDecimal128Hi, u64, y)
	xl := s.newValue1(ssaop.OpDecimal128Lo, u64, x)
	yl := s.newValue1(ssaop.OpDecimal128Lo, u64, y)
	same := s.newValue2(ssaop.OpEq64, boolT, xh, yh)
	coeff := s.newValue2(ssaop.OpAnd64, u64, xh, s.constInt64(u64, int64(bid.CoeffMask128Hi)))
	word := s.newValue2(ssaop.OpEq64, boolT, coeff, zero)
	mask := s.constInt64(u64, int64(bid.Form2Mask128Hi))
	// Compare in stored form: apply the storage XOR to the constant instead
	// of decoding the operand before deciding whether to take the slow path.
	second := s.constInt64(u64, int64(bid.Form2Mask128Hi^(bid.Zero128Hi&bid.Form2Mask128Hi)))
	finite := s.newValue2(ssaop.OpNeq64, boolT, s.newValue2(ssaop.OpAnd64, u64, xh, mask), second)

	bSlow := s.f.NewBlock(block.BlockPlain)
	bAfter := s.f.NewBlock(block.BlockPlain)
	// Short-circuit the guards, particularly the common mismatch in quantum
	// or sign. Branches also let the backend use condition codes directly
	// instead of materializing and combining three boolean values.
	for _, guard := range []*ssa.Value{same, word, finite} {
		b := s.endBlock()
		b.Kind = block.BlockIf
		b.SetControl(guard)
		next := s.f.NewBlock(block.BlockPlain)
		b.AddEdgeTo(next)
		b.AddEdgeTo(bSlow)
		s.startBlock(next)
	}

	sum := s.newValue3(ssaop.OpAdd64carry, types.NewTuple(u64, u64), xl, yl, zero)
	lo := s.newValue1(ssaop.OpSelect0, u64, sum)
	carry := s.newValue1(ssaop.OpSelect1, u64, sum)
	hi := s.newValue2(ssaop.OpOr64, u64, xh, carry)
	s.vars[bidAddVar] = s.newValue2(ssaop.OpDecimal128Make, rt, lo, hi)
	s.endBlock().AddEdgeTo(bAfter)

	s.startBlock(bSlow)
	s.vars[bidAddVar] = s.softDecimalOp(ir.OADD, rt, x, y)
	s.endBlock().AddEdgeTo(bAfter)

	s.startBlock(bAfter)
	return s.variable(bidAddVar, rt)
}
