// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ssagen

import (
	"internal/bid"

	"cmd/compile/internal/base"
	"cmd/compile/internal/ir"
	"cmd/compile/internal/ssa"
	"cmd/compile/internal/ssa/block"
	"cmd/compile/internal/ssa/ssaop"
	"cmd/compile/internal/types"
)

// The lowering of the decimal64 operators below -d=decimalinline=3, on the
// targets of bidKernelDirect. decimal128's is in bid.go (ubdLevel).
//
// An operator is expanded wherever it appears, so what it puts at the call
// site is multiplied by every use in a program. At the compact setting, the
// default, a comparison gets one case, operands of one quantum, behind one
// test and one kernel call; operands at different quanta and special values
// are the kernel's. The sum, difference and product keep the full setting's
// paths, which are already of that size (bid64.go): one test for both signs
// saved nothing measurable. A difference's kernel call is subSlow64, so that
// the call site does not carry the negation of y. Quantize and integral
// rounding also keep the full setting's paths: rounding is what money
// arithmetic does after each product and quotient, and as a call it measured
// too slow for the size it saved (bidRounding). At the none setting every
// operation is a kernel call, the rounding ones included. A comparison with
// a constant zero needs neither call nor decoding, and is written in place at
// every setting (bidCompareZero).
//
// The tests work on the stored form. Bit 62 and the sign are outside the
// storage XOR, and bit 62 clear proves a finite value in the first form
// (bidInlineWordBit). Operands whose words differ in the sign and the
// coefficient alone then have one quantum, and if x passes that test so
// does y.

// bid64BelowFull lowers op on decimal64 operands at the none and compact
// settings, or returns nil for an operation whose lowering does not depend
// on the setting.
func (s *state) bid64BelowFull(op ir.Op, rt *types.Type, x, y *ssa.Value) *ssa.Value {
	compact := base.Debug.DecimalInline == base.DecimalInlineCompact
	switch op {
	case ir.OEQ, ir.ONE, ir.OLT, ir.OLE:
		if compact {
			return s.bidCompareCompact64(op, x, y)
		}
		return s.bidCompareCall64(op, x, y)
	case ir.OADD, ir.OSUB, ir.OMUL:
		if !compact {
			return s.softDecimalOp(op, rt, x, y)
		}
	}
	return nil
}

// bidBranch ends the current block with a branch on ok, to fast, which is
// expected, or to slow. Each returns the result in its arm, so that neither
// arm's work is done on the other.
func (s *state) bidBranch(t *types.Type, ok *ssa.Value, fast, slow func() *ssa.Value) *ssa.Value {
	result := ssaMarker("bidbranch") // one per lowering; see bidAddSub128
	bFast, bSlow, bAfter := s.f.NewBlock(block.BlockPlain), s.f.NewBlock(block.BlockPlain), s.f.NewBlock(block.BlockPlain)
	b := s.endBlock()
	b.Kind = block.BlockIf
	b.SetControl(ok)
	b.Likely = ssa.BranchLikely
	b.AddEdgeTo(bFast)
	b.AddEdgeTo(bSlow)

	s.startBlock(bFast)
	s.vars[result] = fast()
	s.endBlock().AddEdgeTo(bAfter)

	s.startBlock(bSlow)
	s.vars[result] = slow()
	s.endBlock().AddEdgeTo(bAfter)

	s.startBlock(bAfter)
	r := s.variable(result, t)
	delete(s.vars, result)
	return r
}

// bidCompareCompact64 orders operands of one quantum as signed
// coefficients, which puts the two zeros together. Every coefficient in the
// first form has the spare top bit that needs.
func (s *state) bidCompareCompact64(op ir.Op, x, y *ssa.Value) *ssa.Value {
	u64, i64, boolT := types.Types[types.TUINT64], types.Types[types.TINT64], types.Types[types.TBOOL]
	zero, one, top := s.constInt64(u64, 0), s.constInt64(u64, 1), s.constInt64(u64, 63)
	xh, yh := s.newValue1(ssaop.OpCopy, u64, x), s.newValue1(ssaop.OpCopy, u64, y)
	m := s.constInt64(u64, 1<<bid.CoeffBits64-1)
	xc, yc := s.newValue2(ssaop.OpAnd64, u64, xh, m), s.newValue2(ssaop.OpAnd64, u64, yh, m)
	header := s.newValue2(ssaop.OpOr64, u64, s.newValue2(ssaop.OpXor64, u64, xh, yh),
		s.newValue2(ssaop.OpAnd64, u64, xh, s.constInt64(u64, bidInlineWordBit)))
	// Left one drops the sign, then right drops the coefficient.
	bad := s.newValue2(ssaop.OpRsh64Ux64, u64,
		s.newValue2(ssaop.OpLsh64x64, u64, header, one),
		s.constInt64(u64, bid.ExpShift64+1))
	return s.bidBranch(boolT, s.newValue2(ssaop.OpEq64, boolT, bad, zero),
		func() *ssa.Value {
			key := func(h, c *ssa.Value) *ssa.Value {
				m := s.newValue2(ssaop.OpRsh64x64, i64, h, top)
				return s.newValue2(ssaop.OpSub64, i64, s.newValue2(ssaop.OpXor64, i64, c, m), m)
			}
			kx, ky := key(xh, xc), key(yh, yc)
			switch op {
			case ir.OEQ:
				return s.newValue2(ssaop.OpEq64, boolT, kx, ky)
			case ir.ONE:
				return s.newValue2(ssaop.OpNeq64, boolT, kx, ky)
			case ir.OLT:
				return s.newValue2(ssaop.OpLess64, boolT, kx, ky)
			case ir.OLE:
				return s.newValue2(ssaop.OpLeq64, boolT, kx, ky)
			}
			s.Fatalf("unexpected decimal comparison %v", op)
			return nil
		},
		func() *ssa.Value { return s.bidCompareCall64(op, x, y) })
}

// bidCompareCall64 is the kernel's comparison of decimal64 x and y.
func (s *state) bidCompareCall64(op ir.Op, x, y *ssa.Value) *ssa.Value {
	u64, i64, boolT := types.Types[types.TUINT64], types.Types[types.TINT64], types.Types[types.TBOOL]
	storage := s.constInt64(u64, int64(ir.DecimalZero64()))
	xn := s.newValue2(ssaop.OpXor64, u64, s.newValue1(ssaop.OpCopy, u64, x), storage)
	yn := s.newValue2(ssaop.OpXor64, u64, s.newValue1(ssaop.OpCopy, u64, y), storage)
	res := s.rtcall(bidKernelCmp64, true, []*types.Type{i64, boolT}, xn, yn)
	c, ordered := res[0], res[1]
	zero := s.constInt64(i64, 0)
	var r *ssa.Value
	switch op {
	case ir.OLT:
		r = s.newValue2(ssaop.OpLess64, boolT, c, zero)
	case ir.OLE:
		r = s.newValue2(ssaop.OpLeq64, boolT, c, zero)
	default:
		r = s.newValue2(ssaop.OpEq64, boolT, c, zero)
	}
	// Unordered operands, a NaN among them, satisfy only !=.
	r = s.newValue2(ssaop.OpAndB, boolT, ordered, r)
	if op == ir.ONE {
		r = s.newValue1(ssaop.OpNot, boolT, r)
	}
	return r
}
