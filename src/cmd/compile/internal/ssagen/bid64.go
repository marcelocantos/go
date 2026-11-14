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
	"cmd/internal/obj"
)

// The decimal64 operators on the targets of bidKernelDirect. A decimal64 is
// one word, so its fast paths are internal/bid's inline fronts written in
// SSA: addSameQuantum64, subSameQuantum64 and mulShort64. Each is a few
// integer instructions and one test, without branches of its own, and the
// operands it refuses go straight to the kernel's general path with the
// rounding mode, as decimal128's do (bidKernelOp64). The fronts decide
// exact results only, encoded as the general path encodes them; the kernel
// documents and tests their bounds ("Inline fast paths").

var bidKernelAdd64, bidKernelMul64, bidKernelDiv64 *obj.LSym

func initBIDKernelSyms64() {
	bidKernelAdd64 = base.PkgLinksym("internal/bid", "addSlow64", obj.ABIInternal)
	bidKernelMul64 = base.PkgLinksym("internal/bid", "mulSlow64", obj.ABIInternal)
	bidKernelDiv64 = base.PkgLinksym("internal/bid", "div64", obj.ABIInternal)
}

// bidSignMask64 and bidNotCoeff64 are uint64 variables so that converting
// them to the int64 of an SSA constant keeps their bits.
var (
	bidSignMask64 = bid.SignMask64
	bidNotCoeff64 = ^uint64(1<<bid.CoeffBits64 - 1)
)

// bidInlineWordBit is internal/bid's inlineWordBit: bit 62 clear proves a
// first-form encoding, and it reads the same in the stored form, since the
// storage XOR (bid.Zero64) has no bit there.
const bidInlineWordBit = 1 << 62

// bidKernelOp64 calls the kernel's general path for the decimal64
// arithmetic operator op, rounding to nearest, ties to even, or returns nil
// for an operator it does not take.
func (s *state) bidKernelOp64(op ir.Op, rt *types.Type, x, y *ssa.Value) *ssa.Value {
	u64, boolT := types.Types[types.TUINT64], types.Types[types.TBOOL]
	zero := s.constInt64(u64, int64(ir.DecimalZero64()))
	xn := s.newValue2(ssaop.OpXor64, u64, s.newValue1(ssaop.OpCopy, u64, x), zero)
	yn := s.newValue2(ssaop.OpXor64, u64, s.newValue1(ssaop.OpCopy, u64, y), zero)
	var fn *obj.LSym
	switch op {
	case ir.OADD:
		fn = bidKernelAdd64
	case ir.OSUB:
		// x - y is x + (-y), except that a NaN y keeps its sign (see
		// bidKernelOp).
		fn = bidKernelAdd64
		nanMask := s.constInt64(u64, int64(bid.NaN64))
		nan := s.newValue2(ssaop.OpEq64, boolT, s.newValue2(ssaop.OpAnd64, u64, yn, nanMask), nanMask)
		negated := s.newValue2(ssaop.OpXor64, u64, yn, s.constInt64(u64, int64(bidSignMask64)))
		yn = s.newValue3(ssaop.OpCondSelect, u64, yn, negated, nan)
	case ir.OMUL:
		fn = bidKernelMul64
	case ir.ODIV:
		fn = bidKernelDiv64
	default:
		return nil
	}
	mode := s.constInt8(types.Types[types.TUINT8], int8(bid.ToNearestEven))
	res := s.rtcall(fn, true, []*types.Type{u64}, xn, yn, mode)
	return s.newValue1(ssaop.OpCopy, rt, s.newValue2(ssaop.OpXor64, u64, res[0], zero))
}

// bidAddSub64 adds or subtracts word operands of one exponent in place. It
// works on the stored form: the storage XOR touches only exponent bits, so
// equal stored headers are equal native ones, and a coefficient sum that
// does not carry into the exponent field leaves the stored header as it was.
//
// The operand the sum path takes is y for x + y and y with its sign flipped
// for x - y: the two have one sign when the magnitudes add. Then
// addSameQuantum64 decides x and that operand of one sign and one exponent,
// and subSameQuantum64 decides x and the operand flipped back, which share
// x's header when the magnitudes subtract. Anything either refuses (other
// exponents, a sum of more than 53 bits, a cancellation to zero, a second
// form or a special) goes to the kernel with the original operands.
func (s *state) bidAddSub64(op ir.Op, rt *types.Type, x, y *ssa.Value) *ssa.Value {
	result := ssaMarker("bidadd64") // one per lowering; see bidAddSub128
	u64, boolT := types.Types[types.TUINT64], types.Types[types.TBOOL]
	zero := s.constInt64(u64, 0)
	m := s.constInt64(u64, int64(1<<bid.CoeffBits64-1))
	shift := s.constInt64(u64, bid.ExpShift64)
	signBit := s.constInt64(u64, int64(bidSignMask64))
	inline := s.constInt64(u64, bidInlineWordBit)
	xs := s.newValue1(ssaop.OpCopy, u64, x)
	ys := s.newValue1(ssaop.OpCopy, u64, y)
	yadd := ys
	if op == ir.OSUB {
		yadd = s.newValue2(ssaop.OpXor64, u64, ys, signBit)
	}

	bAfter := s.f.NewBlock(block.BlockPlain)
	bDiff := s.f.NewBlock(block.BlockPlain)
	bSlow := s.f.NewBlock(block.BlockPlain)

	// addSameQuantum64: s = x + y's coefficient, if x and y share sign and
	// exponent, s did not carry out of the coefficient field and x is in
	// the first form.
	sum := s.newValue2(ssaop.OpAdd64, u64, xs, s.newValue2(ssaop.OpAnd64, u64, yadd, m))
	bad := s.newValue2(ssaop.OpOr64, u64,
		s.newValue2(ssaop.OpRsh64Ux64, u64,
			s.newValue2(ssaop.OpOr64, u64, s.newValue2(ssaop.OpXor64, u64, xs, yadd), s.newValue2(ssaop.OpXor64, u64, sum, xs)), shift),
		s.newValue2(ssaop.OpAnd64, u64, xs, inline))
	ok := s.newValue2(ssaop.OpEq64, boolT, bad, zero)
	b := s.endBlock()
	b.Kind = block.BlockIf
	b.SetControl(ok)
	bSum := s.f.NewBlock(block.BlockPlain)
	b.AddEdgeTo(bSum)
	b.AddEdgeTo(bDiff)
	s.startBlock(bSum)
	s.vars[result] = s.newValue1(ssaop.OpCopy, rt, sum)
	s.endBlock().AddEdgeTo(bAfter)

	// subSameQuantum64 of x and ysub = yadd with its sign flipped: the
	// difference of the coefficients, negated with x's sign flipped when
	// it borrows, if x and ysub share sign and exponent, x is in the first
	// form and the coefficients differ.
	s.startBlock(bDiff)
	ysub := s.newValue2(ssaop.OpXor64, u64, yadd, signBit)
	diff := s.newValue3(ssaop.OpSub64borrow, types.NewTuple(u64, u64),
		s.newValue2(ssaop.OpAnd64, u64, xs, m), s.newValue2(ssaop.OpAnd64, u64, ysub, m), zero)
	d := s.newValue1(ssaop.OpSelect0, u64, diff)
	borrow := s.newValue1(ssaop.OpSelect1, u64, diff)
	one := s.constInt64(u64, 1)
	// d ^ -borrow + borrow is |d| when the difference borrowed.
	mag := s.newValue2(ssaop.OpAdd64, u64, s.newValue2(ssaop.OpXor64, u64, d, s.newValue1(ssaop.OpNeg64, u64, borrow)), borrow)
	header := s.newValue2(ssaop.OpXor64, u64,
		s.newValue2(ssaop.OpAnd64, u64, xs, s.newValue1(ssaop.OpCom64, u64, m)),
		s.newValue2(ssaop.OpLsh64x64, u64, borrow, s.constInt64(u64, 63)))
	res := s.newValue2(ssaop.OpOr64, u64, header, mag)
	bad = s.newValue2(ssaop.OpOr64, u64,
		s.newValue2(ssaop.OpRsh64Ux64, u64,
			s.newValue2(ssaop.OpOr64, u64, s.newValue2(ssaop.OpXor64, u64, xs, ysub), s.newValue2(ssaop.OpAnd64, u64, xs, inline)), shift),
		// (d-1) &^ d has its top bit set only for d = 0.
		s.newValue2(ssaop.OpRsh64Ux64, u64,
			s.newValue2(ssaop.OpAnd64, u64, s.newValue2(ssaop.OpSub64, u64, d, one), s.newValue1(ssaop.OpCom64, u64, d)),
			s.constInt64(u64, 63)))
	ok = s.newValue2(ssaop.OpEq64, boolT, bad, zero)
	b = s.endBlock()
	b.Kind = block.BlockIf
	b.SetControl(ok)
	bDiffOK := s.f.NewBlock(block.BlockPlain)
	b.AddEdgeTo(bDiffOK)
	b.AddEdgeTo(bSlow)
	s.startBlock(bDiffOK)
	s.vars[result] = s.newValue1(ssaop.OpCopy, rt, res)
	s.endBlock().AddEdgeTo(bAfter)

	s.startBlock(bSlow)
	s.vars[result] = s.bidKernelOp64(op, rt, x, y)
	s.endBlock().AddEdgeTo(bAfter)

	s.startBlock(bAfter)
	r := s.variable(result, rt)
	delete(s.vars, result)
	return r
}

// bidMul64 multiplies word operands in place when mulShort64 decides them:
// a product below 2^53 at a biased exponent below 2^(ExpBits64-1), exact,
// in the first form, at the sum of the exponents with the exclusive or of
// the signs. The header arithmetic is on native encodings, so it removes
// the storage XOR first and restores it after.
func (s *state) bidMul64(rt *types.Type, x, y *ssa.Value) *ssa.Value {
	result := ssaMarker("bidmul64") // one per lowering; see bidAddSub128
	u64, boolT := types.Types[types.TUINT64], types.Types[types.TBOOL]
	zero := s.constInt64(u64, 0)
	storage := s.constInt64(u64, int64(ir.DecimalZero64()))
	m := s.constInt64(u64, int64(1<<bid.CoeffBits64-1))
	notm := s.constInt64(u64, int64(bidNotCoeff64))
	inline := s.constInt64(u64, bidInlineWordBit)
	xn := s.newValue2(ssaop.OpXor64, u64, s.newValue1(ssaop.OpCopy, u64, x), storage)
	yn := s.newValue2(ssaop.OpXor64, u64, s.newValue1(ssaop.OpCopy, u64, y), storage)
	product := s.newValue2(ssaop.OpMul64uhilo, types.NewTuple(u64, u64),
		s.newValue2(ssaop.OpAnd64, u64, xn, m), s.newValue2(ssaop.OpAnd64, u64, yn, m))
	hi := s.newValue1(ssaop.OpSelect0, u64, product)
	lo := s.newValue1(ssaop.OpSelect1, u64, product)
	t := s.newValue2(ssaop.OpSub64, u64,
		s.newValue2(ssaop.OpAdd64, u64, s.newValue2(ssaop.OpAnd64, u64, xn, notm), s.newValue2(ssaop.OpAnd64, u64, yn, notm)),
		s.constInt64(u64, int64(uint64(bid.Bias64)<<bid.ExpShift64)))
	words := s.newValue2(ssaop.OpOr64, u64, s.newValue2(ssaop.OpOr64, u64, xn, yn),
		s.newValue2(ssaop.OpAnd64, u64, t, s.constInt64(u64, int64(^bid.SignMask64))))
	bad := s.newValue2(ssaop.OpOr64, u64,
		s.newValue2(ssaop.OpOr64, u64, s.newValue2(ssaop.OpAnd64, u64, words, inline), hi),
		s.newValue2(ssaop.OpRsh64Ux64, u64, lo, s.constInt64(u64, bid.CoeffBits64)))
	ok := s.newValue2(ssaop.OpEq64, boolT, bad, zero)

	bAfter := s.f.NewBlock(block.BlockPlain)
	bSlow := s.f.NewBlock(block.BlockPlain)
	bFast := s.f.NewBlock(block.BlockPlain)
	b := s.endBlock()
	b.Kind = block.BlockIf
	b.SetControl(ok)
	b.AddEdgeTo(bFast)
	b.AddEdgeTo(bSlow)
	s.startBlock(bFast)
	r := s.newValue2(ssaop.OpXor64, u64, s.newValue2(ssaop.OpOr64, u64, t, lo), storage)
	s.vars[result] = s.newValue1(ssaop.OpCopy, rt, r)
	s.endBlock().AddEdgeTo(bAfter)

	s.startBlock(bSlow)
	s.vars[result] = s.bidKernelOp64(ir.OMUL, rt, x, y)
	s.endBlock().AddEdgeTo(bAfter)

	s.startBlock(bAfter)
	v := s.variable(result, rt)
	delete(s.vars, result)
	return v
}
