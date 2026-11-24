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

var (
	bidCompareVar = ssaMarker("bidcompare")
	bidCompareXC  = ssaMarker("bidcomparexc")
	bidCompareYC  = ssaMarker("bidcompareyc")
	bidCompareXE  = ssaMarker("bidcomparexe")
	bidCompareYE  = ssaMarker("bidcompareye")
)

// bidCompare compares two decimal64 values, stored as BID64, by their
// word coefficients. The guards and the second-form decoder establish
// finite canonical operands. Signs, zeros, equal quanta and large gaps
// decide many comparisons directly. For nearby quanta, scaling the
// higher-quantum coefficient fits exactly in 128 bits: gap is at most 19,
// so both factors fit uint64. Comparing that product with the other word
// needs no decimal rounding, division or digit counting. Special values
// reached after the word guard are decided by kind and sign. Both operands
// are already evaluated. decimal128 comparisons on these targets are
// ubdComparePred's (until 2026-11-23 the former ubdlevel 3 used a
// decimal128 branch of this function).
func (s *state) bidCompare(op ir.Op, x, y *ssa.Value) *ssa.Value {
	if x.Type.Kind() != types.TDECIMAL64 {
		s.Fatalf("bidCompare of %v", x.Type)
	}
	u64, boolT := types.Types[types.TUINT64], types.Types[types.TBOOL]
	zero := s.constInt64(u64, 0)
	xh, yh := s.newValue1(ssaop.OpCopy, u64, x), s.newValue1(ssaop.OpCopy, u64, y)
	cmask := s.constInt64(u64, 1<<bid.CoeffBits64-1)
	xc, yc := s.newValue2(ssaop.OpAnd64, u64, xh, cmask), s.newValue2(ssaop.OpAnd64, u64, yh, cmask)
	shift, expMask, storageZero, formMask := uint64(bid.ExpShift64), uint64(bid.ExpMask64), uint64(bid.Zero64), uint64(bid.Form2Mask64)
	bSecond, bFinite := s.f.NewBlock(block.BlockPlain), s.f.NewBlock(block.BlockPlain)
	bAfter := s.f.NewBlock(block.BlockPlain)
	bSpecial := s.f.NewBlock(block.BlockPlain)
	guard := func(v *ssa.Value, fail *ssa.Block) {
		b := s.endBlock()
		b.Kind = block.BlockIf
		b.SetControl(v)
		next := s.f.NewBlock(block.BlockPlain)
		b.AddEdgeTo(next)
		b.AddEdgeTo(fail)
		s.startBlock(next)
	}
	for _, h := range []*ssa.Value{xh, yh} {
		mask := s.constInt64(u64, int64(formMask))
		second := s.constInt64(u64, int64(formMask^(storageZero&formMask)))
		guard(s.newValue2(ssaop.OpNeq64, boolT, s.newValue2(ssaop.OpAnd64, u64, h, mask), second), bSpecial)
	}
	// negativeOf reports the sign of a stored word.
	negativeOf := func(h *ssa.Value) *ssa.Value {
		return s.newValue2(ssaop.OpLess64, boolT, h, zero)
	}
	exponent := func(h *ssa.Value) *ssa.Value {
		native := s.newValue2(ssaop.OpXor64, u64, h, s.constInt64(u64, int64(storageZero)))
		return s.newValue2(ssaop.OpAnd64, u64, s.newValue2(ssaop.OpRsh64Ux64, u64, native, s.constInt64(u64, int64(shift))), s.constInt64(u64, int64(expMask)))
	}
	// Join the cheap first-form decode with the less common second form
	// below. Every finite decimal64 coefficient fits uint64.
	s.vars[bidCompareXC], s.vars[bidCompareYC] = xc, yc
	s.vars[bidCompareXE], s.vars[bidCompareYE] = exponent(xh), exponent(yh)
	s.endBlock().AddEdgeTo(bFinite)
	s.startBlock(bFinite)
	xc, yc = s.variable(bidCompareXC, u64), s.variable(bidCompareYC, u64)
	xe, ye := s.variable(bidCompareXE, u64), s.variable(bidCompareYE, u64)
	xzero, yzero := s.newValue2(ssaop.OpEq64, boolT, xc, zero), s.newValue2(ssaop.OpEq64, boolT, yc, zero)
	var xneg, differentSigns, sameSigns *ssa.Value // set in bScale, which dominates their uses
	// order is the result for two values of one effective sign, or for
	// the infinities, whose result the caller has already decided.
	order := func(less, equal, reverse *ssa.Value) *ssa.Value {
		switch op {
		case ir.OEQ:
			return equal
		case ir.ONE:
			return s.newValue1(ssaop.OpNot, boolT, equal)
		case ir.OLT, ir.OLE:
			less = s.newValue2(ssaop.OpNeqB, boolT, less, reverse)
			if op == ir.OLT {
				return s.newValue2(ssaop.OpAndB, boolT, less, s.newValue1(ssaop.OpNot, boolT, equal))
			}
			return s.newValue2(ssaop.OpOrB, boolT, less, equal)
		}
		s.Fatalf("unexpected word decimal comparison %v", op)
		return nil
	}
	// finish ends a block of the finite comparison: the order of the
	// magnitudes if the signs agree, and the signs' own answer if not.
	finish := func(less, equal, reverse *ssa.Value) {
		same := s.newValue2(ssaop.OpAndB, boolT, sameSigns, order(less, equal, reverse))
		var opposite *ssa.Value
		switch op {
		case ir.OEQ:
			opposite = s.constBool(false)
		case ir.ONE, ir.OLT, ir.OLE:
			// Not equal; and x is the lesser exactly when it is the
			// negative one.
			opposite = differentSigns
			if op != ir.ONE {
				opposite = s.newValue2(ssaop.OpAndB, boolT, differentSigns, xneg)
			}
		}
		s.vars[bidCompareVar] = s.newValue2(ssaop.OpOrB, boolT, same, opposite)
		s.endBlock().AddEdgeTo(bAfter)
	}
	direct := s.newValue2(ssaop.OpOrB, boolT, s.newValue2(ssaop.OpEq64, boolT, xe, ye), s.newValue2(ssaop.OpOrB, boolT, xzero, yzero))
	bDirect, bScale := s.f.NewBlock(block.BlockPlain), s.f.NewBlock(block.BlockPlain)
	b := s.endBlock()
	b.Kind = block.BlockIf
	b.SetControl(direct)
	b.AddEdgeTo(bDirect)
	b.AddEdgeTo(bScale)
	s.startBlock(bDirect)
	// One quantum, or a zero, which has no magnitude to scale: the values
	// order as their signed coefficients, and each coefficient is negated
	// by its sign with a mask, so that the comparison is one signed
	// integer comparison whatever the signs are. A negative zero negates
	// to zero, as it must compare. The negation needs the coefficient
	// below 2^63, which every decimal64 coefficient is.
	i64 := types.Types[types.TINT64]
	key := func(h, c *ssa.Value) *ssa.Value {
		m := s.newValue2(ssaop.OpRsh64x64, i64, h, s.constInt64(u64, 63))
		return s.newValue2(ssaop.OpSub64, i64, s.newValue2(ssaop.OpXor64, i64, c, m), m)
	}
	kx, ky := key(xh, xc), key(yh, yc)
	switch op {
	case ir.OEQ:
		s.vars[bidCompareVar] = s.newValue2(ssaop.OpEq64, boolT, kx, ky)
	case ir.ONE:
		s.vars[bidCompareVar] = s.newValue2(ssaop.OpNeq64, boolT, kx, ky)
	case ir.OLT:
		s.vars[bidCompareVar] = s.newValue2(ssaop.OpLess64, boolT, kx, ky)
	case ir.OLE:
		s.vars[bidCompareVar] = s.newValue2(ssaop.OpLeq64, boolT, kx, ky)
	default:
		s.Fatalf("unexpected word decimal comparison %v", op)
	}
	s.endBlock().AddEdgeTo(bAfter)
	s.startBlock(bScale)
	// Both coefficients are non-zero here. Opposite signs decide the
	// result whatever the magnitudes: the negative operand is the lesser,
	// and the two are not equal. finish folds that into each result as
	// boolean arithmetic; it is not taken as a branch, because the sign of
	// a value is as unpredictable as its digits, and a branch on it
	// mispredicts every other time on values of mixed sign.
	xneg = negativeOf(xh)
	differentSigns = s.newValue2(ssaop.OpNeqB, boolT, xneg, negativeOf(yh))
	sameSigns = s.newValue1(ssaop.OpNot, boolT, differentSigns)
	swap := s.newValue2(ssaop.OpLess64U, boolT, xe, ye)
	gap := s.newValue3(ssaop.OpCondSelect, u64, s.newValue2(ssaop.OpSub64, u64, ye, xe), s.newValue2(ssaop.OpSub64, u64, xe, ye), swap)
	const maxGap = bidMaxWordPower // 10^19 is the largest power of ten fitting uint64.
	near := s.newValue2(ssaop.OpLeq64U, boolT, gap, s.constInt64(u64, maxGap))
	bNear, bFar := s.f.NewBlock(block.BlockPlain), s.f.NewBlock(block.BlockPlain)
	b = s.endBlock()
	b.Kind = block.BlockIf
	b.SetControl(near)
	b.AddEdgeTo(bNear)
	b.AddEdgeTo(bFar)
	s.startBlock(bFar)
	// Both coefficients are nonzero. Multiplication by at least 10^20
	// exceeds every uint64, so the larger exponent has larger magnitude.
	finish(swap, s.constBool(false), xneg)
	s.startBlock(bNear)
	higher := s.newValue3(ssaop.OpCondSelect, u64, yc, xc, swap)
	lower := s.newValue3(ssaop.OpCondSelect, u64, xc, yc, swap)
	// The gap guard proves the immutable content-addressed load is in bounds.
	const wordBytes = bidWordBytes
	ptrT := types.NewPtr(u64)
	table := s.newValue1A(ssaop.OpAddr, ptrT, bidComparePowers, s.sb)
	index := s.newValue2(ssaop.OpMul64, u64, gap, s.constInt64(u64, wordBytes))
	scale := s.rawLoad(u64, s.newValue2(ssaop.OpAddPtr, ptrT, table, index))
	product := s.newValue2(ssaop.OpMul64uhilo, types.NewTuple(u64, u64), higher, scale)
	hi, lo := s.newValue1(ssaop.OpSelect0, u64, product), s.newValue1(ssaop.OpSelect1, u64, product)
	fits := s.newValue2(ssaop.OpEq64, boolT, hi, zero)
	equal := s.newValue2(ssaop.OpAndB, boolT, fits, s.newValue2(ssaop.OpEq64, boolT, lo, lower))
	less := s.newValue2(ssaop.OpAndB, boolT, fits, s.newValue2(ssaop.OpLess64U, boolT, lo, lower))
	finish(less, equal, s.newValue2(ssaop.OpNeqB, boolT, swap, xneg))
	s.startBlock(bSpecial)
	// A failed first-form guard may be a NaN, infinity or second finite
	// form. NaNs are unordered; infinities depend only on kind and sign.
	// A finite second form needs its separate coefficient decoder.
	class := func(h *ssa.Value, mask uint64) *ssa.Value {
		m := s.constInt64(u64, int64(mask))
		native := s.newValue2(ssaop.OpXor64, u64, h, s.constInt64(u64, int64(storageZero)))
		return s.newValue2(ssaop.OpEq64, boolT, s.newValue2(ssaop.OpAnd64, u64, native, m), m)
	}
	nan := s.newValue2(ssaop.OpOrB, boolT, class(xh, bid.NaN64), class(yh, bid.NaN64))
	bNaN, bInf := s.f.NewBlock(block.BlockPlain), s.f.NewBlock(block.BlockPlain)
	b = s.endBlock()
	b.Kind = block.BlockIf
	b.SetControl(nan)
	b.AddEdgeTo(bNaN)
	b.AddEdgeTo(bInf)
	s.startBlock(bNaN)
	s.vars[bidCompareVar] = s.constBool(op == ir.ONE)
	s.endBlock().AddEdgeTo(bAfter)
	s.startBlock(bInf)
	xinf, yinf := class(xh, bid.SpecialMask64), class(yh, bid.SpecialMask64)
	guard(s.newValue2(ssaop.OpOrB, boolT, xinf, yinf), bSecond)
	xn, yn := s.newValue2(ssaop.OpLess64, boolT, xh, zero), s.newValue2(ssaop.OpLess64, boolT, yh, zero)
	equal = s.newValue2(ssaop.OpAndB, boolT, s.newValue2(ssaop.OpAndB, boolT, xinf, yinf), s.newValue2(ssaop.OpEqB, boolT, xn, yn))
	less = s.newValue2(ssaop.OpOrB, boolT,
		s.newValue2(ssaop.OpAndB, boolT, s.newValue2(ssaop.OpAndB, boolT, xinf, xn), s.newValue1(ssaop.OpNot, boolT, equal)),
		s.newValue2(ssaop.OpAndB, boolT, s.newValue2(ssaop.OpAndB, boolT, yinf, s.newValue1(ssaop.OpNot, boolT, yn)), s.newValue1(ssaop.OpNot, boolT, xinf)))
	s.vars[bidCompareVar] = order(less, equal, s.constBool(false))
	s.endBlock().AddEdgeTo(bAfter)
	s.startBlock(bSecond)
	decode := func(h *ssa.Value) (c, e *ssa.Value) {
		native := s.newValue2(ssaop.OpXor64, u64, h, s.constInt64(u64, int64(storageZero)))
		mask := s.constInt64(u64, int64(formMask))
		second := s.newValue2(ssaop.OpEq64, boolT, s.newValue2(ssaop.OpAnd64, u64, native, mask), mask)
		c1 := s.newValue2(ssaop.OpAnd64, u64, native, s.constInt64(u64, 1<<bid.CoeffBits64-1))
		c2 := s.newValue2(ssaop.OpOr64, u64,
			s.newValue2(ssaop.OpAnd64, u64, native, s.constInt64(u64, 1<<bid.Form2CoeffBits64-1)), s.constInt64(u64, 1<<bid.CoeffBits64))
		// Oversized second-form coefficients denote zero, at any sign
		// or quantum. The common numeric path normalizes zero's sign.
		canonical := s.newValue2(ssaop.OpLeq64U, boolT, c2, s.constInt64(u64, int64(bid.MaxCoeff64)))
		c2 = s.newValue3(ssaop.OpCondSelect, u64, c2, zero, canonical)
		c = s.newValue3(ssaop.OpCondSelect, u64, c2, c1, second)
		e2 := s.newValue2(ssaop.OpAnd64, u64,
			s.newValue2(ssaop.OpRsh64Ux64, u64, native, s.constInt64(u64, bid.Form2ExpShift64)), s.constInt64(u64, int64(expMask)))
		e = s.newValue3(ssaop.OpCondSelect, u64, e2, exponent(h), second)
		return c, e
	}
	s.vars[bidCompareXC], s.vars[bidCompareXE] = decode(xh)
	s.vars[bidCompareYC], s.vars[bidCompareYE] = decode(yh)
	s.endBlock().AddEdgeTo(bFinite)
	s.startBlock(bAfter)
	return s.variable(bidCompareVar, boolT)
}
