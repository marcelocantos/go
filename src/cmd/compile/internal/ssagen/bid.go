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

// bidCompareZero implements every comparison with a constant zero. Once NaNs
// and all encodings of zero are excluded, the sign alone determines ordering,
// including infinities. No decimal magnitude comparison or runtime call is
// needed. x has already been evaluated exactly once, in source order.
func (s *state) bidCompareZero(op ir.Op, x *ssa.Value, zeroOnLeft bool) *ssa.Value {
	isZero := s.bidIsZero(x)
	boolT, u64 := types.Types[types.TBOOL], types.Types[types.TUINT64]
	if op == ir.OEQ {
		return isZero
	}
	if op == ir.ONE {
		return s.newValue1(ssaop.OpNot, boolT, isZero)
	}
	if zeroOnLeft {
		switch op {
		case ir.OLT:
			op = ir.OGT
		case ir.OLE:
			op = ir.OGE
		case ir.OGT:
			op = ir.OLT
		case ir.OGE:
			op = ir.OLE
		}
	}
	var hi *ssa.Value
	if x.Type.Kind() == types.TDECIMAL64 {
		hi = s.newValue2(ssaop.OpXor64, u64, s.newValue1(ssaop.OpCopy, u64, x), s.constInt64(u64, int64(bid.Zero64)))
	} else {
		hi = s.newValue2(ssaop.OpXor64, u64, s.newValue1(ssaop.OpDecimal128Hi, u64, x), s.constInt64(u64, int64(bid.Zero128Hi)))
	}
	// These fields occupy the same high-word bits in both BID widths.
	mask := s.constInt64(u64, int64(bid.SpecialMask64|bid.NaNBit64))
	ordered := s.newValue2(ssaop.OpNeq64, boolT, s.newValue2(ssaop.OpAnd64, u64, hi, mask), mask)
	negative := s.newValue2(ssaop.OpLess64, boolT, hi, s.constInt64(u64, 0))
	var result *ssa.Value
	switch op {
	case ir.OLT:
		result = s.newValue2(ssaop.OpAndB, boolT, negative, s.newValue1(ssaop.OpNot, boolT, isZero))
	case ir.OLE:
		result = s.newValue2(ssaop.OpOrB, boolT, negative, isZero)
	case ir.OGT:
		result = s.newValue2(ssaop.OpAndB, boolT, s.newValue1(ssaop.OpNot, boolT, negative), s.newValue1(ssaop.OpNot, boolT, isZero))
	case ir.OGE:
		result = s.newValue2(ssaop.OpOrB, boolT, s.newValue1(ssaop.OpNot, boolT, negative), isZero)
	default:
		s.Fatalf("unexpected decimal zero comparison %v", op)
	}
	return s.newValue2(ssaop.OpAndB, boolT, ordered, result)
}

var bidAddVar = ssaMarker("bidadd")

// bidAddSub128 adds or subtracts word-sized coefficients directly.
// First handle effective same-sign, same-quantum addition:
// The equal stored high words establish both sign and quantum equality. A
// zero high coefficient and first-form encoding establish canonical finite
// operands. The coefficient sum is at most 2^65-2, below 10^34, at an already
// representable exponent. For subtraction, flip y's sign only in the fast
// path; the fallback receives the original y, including its NaN sign.
// This is an exact packing exception: neither rounding
// nor exponent clamping is needed, and even negative zero keeps its sign.
//
// Both operands were evaluated before this method. Only the slow branch calls
// the runtime, with the original operands. The normal variable/memory phis at
// bAfter join that call's memory with the unchanged fast-branch memory.
func (s *state) bidAddSub128(op ir.Op, rt *types.Type, x, y *ssa.Value) *ssa.Value {
	u64, boolT := types.Types[types.TUINT64], types.Types[types.TBOOL]
	zero := s.constInt64(u64, 0)
	xh := s.newValue1(ssaop.OpDecimal128Hi, u64, x)
	yh := s.newValue1(ssaop.OpDecimal128Hi, u64, y)
	signMask := bid.SignMask128Hi
	signBit := s.constInt64(u64, int64(signMask))
	if op == ir.OSUB {
		yh = s.newValue2(ssaop.OpXor64, u64, yh, signBit)
	}
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
	bAlign := s.f.NewBlock(block.BlockPlain)
	// Short-circuit the guards, particularly the common mismatch in quantum
	// or sign. Branches also let the backend use condition codes directly
	// instead of materializing and combining three boolean values.
	for i, guard := range []*ssa.Value{same, word, finite} {
		b := s.endBlock()
		b.Kind = block.BlockIf
		b.SetControl(guard)
		next := s.f.NewBlock(block.BlockPlain)
		b.AddEdgeTo(next)
		if i == 0 {
			// A sign or quantum mismatch may still have an exact result.
			b.AddEdgeTo(bAlign)
		} else {
			b.AddEdgeTo(bSlow)
		}
		s.startBlock(next)
	}

	sum := s.newValue3(ssaop.OpAdd64carry, types.NewTuple(u64, u64), xl, yl, zero)
	lo := s.newValue1(ssaop.OpSelect0, u64, sum)
	carry := s.newValue1(ssaop.OpSelect1, u64, sum)
	hi := s.newValue2(ssaop.OpOr64, u64, xh, carry)
	s.vars[bidAddVar] = s.newValue2(ssaop.OpDecimal128Make, rt, lo, hi)
	s.endBlock().AddEdgeTo(bAfter)

	s.startBlock(bAlign)
	// For two uint64 coefficients and a quantum gap at most 14, the largest
	// aligned sum is (2^64-1)*(10^14+1) < 10^34. A difference is no larger.
	// Both are exact at the smaller input exponent, including subnormals.
	const maxGap = 14
	coeffs := s.newValue2(ssaop.OpAnd64, u64, s.newValue2(ssaop.OpOr64, u64, xh, yh), s.constInt64(u64, int64(bid.CoeffMask128Hi)))
	words := s.newValue2(ssaop.OpEq64, boolT, coeffs, zero)
	yfinite := s.newValue2(ssaop.OpNeq64, boolT, s.newValue2(ssaop.OpAnd64, u64, yh, mask), second)
	for _, guard := range []*ssa.Value{words, finite, yfinite} {
		b := s.endBlock()
		b.Kind = block.BlockIf
		b.SetControl(guard)
		next := s.f.NewBlock(block.BlockPlain)
		b.AddEdgeTo(next)
		b.AddEdgeTo(bSlow)
		s.startBlock(next)
	}
	xnative := s.newValue2(ssaop.OpXor64, u64, xh, s.constInt64(u64, int64(bid.Zero128Hi)))
	ynative := s.newValue2(ssaop.OpXor64, u64, yh, s.constInt64(u64, int64(bid.Zero128Hi)))
	shift := s.constInt64(u64, bid.ExpShift128)
	expMask := s.constInt64(u64, int64(bid.ExpMask128))
	xe := s.newValue2(ssaop.OpAnd64, u64, s.newValue2(ssaop.OpRsh64Ux64, u64, xnative, shift), expMask)
	ye := s.newValue2(ssaop.OpAnd64, u64, s.newValue2(ssaop.OpRsh64Ux64, u64, ynative, shift), expMask)
	swap := s.newValue2(ssaop.OpLess64U, boolT, xe, ye)
	gap := s.newValue3(ssaop.OpCondSelect, u64,
		s.newValue2(ssaop.OpSub64, u64, ye, xe), s.newValue2(ssaop.OpSub64, u64, xe, ye), swap)
	bounded := s.newValue2(ssaop.OpLeq64U, boolT, gap, s.constInt64(u64, maxGap))
	b := s.endBlock()
	b.Kind = block.BlockIf
	b.SetControl(bounded)
	bScale := s.f.NewBlock(block.BlockPlain)
	b.AddEdgeTo(bScale)
	b.AddEdgeTo(bSlow)
	s.startBlock(bScale)

	higher := s.newValue3(ssaop.OpCondSelect, u64, yl, xl, swap)
	lower := s.newValue3(ssaop.OpCondSelect, u64, xl, yl, swap)
	highHeader := s.newValue3(ssaop.OpCondSelect, u64, yh, xh, swap)
	lowHeader := s.newValue3(ssaop.OpCondSelect, u64, xh, yh, swap)
	// Use the compiler's immutable content-addressed literal data, not a
	// mutable runtime table. The guarded gap proves the load is in bounds.
	// The selected ARM64/AMD64 backends support unaligned uint64 loads.
	const wordBytes = 8
	var powers [(maxGap + 1) * wordBytes]byte
	pow := uint64(1)
	for i := 0; i <= maxGap; i++ {
		Arch.LinkArch.ByteOrder.PutUint64(powers[i*wordBytes:], pow)
		pow *= 10
	}
	ptrT := types.NewPtr(u64)
	table := s.newValue1A(ssaop.OpAddr, ptrT, s.f.Frontend().StringData(string(powers[:])), s.sb)
	index := s.newValue2(ssaop.OpMul64, u64, gap, s.constInt64(u64, wordBytes))
	power := s.rawLoad(u64, s.newValue2(ssaop.OpAddPtr, ptrT, table, index))
	product := s.newValue2(ssaop.OpMul64uhilo, types.NewTuple(u64, u64), higher, power)
	phi := s.newValue1(ssaop.OpSelect0, u64, product)
	plo := s.newValue1(ssaop.OpSelect1, u64, product)
	signs := s.newValue2(ssaop.OpAnd64, u64, s.newValue2(ssaop.OpXor64, u64, xh, yh), signBit)
	sameSign := s.newValue2(ssaop.OpEq64, boolT, signs, zero)
	b = s.endBlock()
	b.Kind = block.BlockIf
	b.SetControl(sameSign)
	bSum := s.f.NewBlock(block.BlockPlain)
	bDiff := s.f.NewBlock(block.BlockPlain)
	b.AddEdgeTo(bSum)
	b.AddEdgeTo(bDiff)
	s.startBlock(bSum)
	sum = s.newValue3(ssaop.OpAdd64carry, types.NewTuple(u64, u64), plo, lower, zero)
	lo = s.newValue1(ssaop.OpSelect0, u64, sum)
	carry = s.newValue1(ssaop.OpSelect1, u64, sum)
	hi = s.newValue2(ssaop.OpAdd64, u64, phi, carry)
	s.vars[bidAddVar] = s.newValue2(ssaop.OpDecimal128Make, rt, lo, s.newValue2(ssaop.OpOr64, u64, lowHeader, hi))
	s.endBlock().AddEdgeTo(bAfter)

	s.startBlock(bDiff)
	// The lower-quantum operand has a word coefficient. It can exceed the
	// aligned operand only when that operand's high product word is zero.
	less := s.newValue2(ssaop.OpAndB, boolT,
		s.newValue2(ssaop.OpEq64, boolT, phi, zero), s.newValue2(ssaop.OpLess64U, boolT, plo, lower))
	diff := s.newValue3(ssaop.OpSub64borrow, types.NewTuple(u64, u64), plo, lower, zero)
	lo = s.newValue1(ssaop.OpSelect0, u64, diff)
	borrow := s.newValue1(ssaop.OpSelect1, u64, diff)
	hi = s.newValue2(ssaop.OpSub64, u64, phi, borrow)
	lo = s.newValue3(ssaop.OpCondSelect, u64, s.newValue2(ssaop.OpSub64, u64, lower, plo), lo, less)
	hi = s.newValue3(ssaop.OpCondSelect, u64, zero, hi, less)
	negative := s.newValue2(ssaop.OpAnd64, u64, s.newValue3(ssaop.OpCondSelect, u64, lowHeader, highHeader, less), signBit)
	isZero := s.newValue2(ssaop.OpEq64, boolT, s.newValue2(ssaop.OpOr64, u64, hi, lo), zero)
	// Cancellation under round-half-even gives +0 at the preferred quantum.
	negative = s.newValue3(ssaop.OpCondSelect, u64, zero, negative, isZero)
	header := s.newValue2(ssaop.OpAnd64, u64, lowHeader, s.constInt64(u64, int64(^signMask)))
	header = s.newValue2(ssaop.OpOr64, u64, header, negative)
	s.vars[bidAddVar] = s.newValue2(ssaop.OpDecimal128Make, rt, lo, s.newValue2(ssaop.OpOr64, u64, header, hi))
	s.endBlock().AddEdgeTo(bAfter)

	s.startBlock(bSlow)
	s.vars[bidAddVar] = s.softDecimalOp(op, rt, x, y)
	s.endBlock().AddEdgeTo(bAfter)

	s.startBlock(bAfter)
	return s.variable(bidAddVar, rt)
}

var bidMulVar = ssaMarker("bidmul")

// bidMul128 multiplies two canonical finite word coefficients directly when
// the full product and the sum of exponents fit decimal128. Mul64uhilo gives
// the exact product, including every bit needed to decide the coefficient
// bound. An unsigned range check on the biased exponent sum rejects both
// underflow and overflow. Within these bounds the preferred quantum is the
// exponent sum, the sign is the XOR of the operand signs (also for zero), and
// no digit is discarded. This is an exact packing exception; every failed
// guard calls the original multiplication, which handles rounding, clamping,
// noncanonical encodings and specials.
func (s *state) bidMul128(rt *types.Type, x, y *ssa.Value) *ssa.Value {
	u64, boolT := types.Types[types.TUINT64], types.Types[types.TBOOL]
	zero := s.constInt64(u64, 0)
	xh := s.newValue1(ssaop.OpDecimal128Hi, u64, x)
	yh := s.newValue1(ssaop.OpDecimal128Hi, u64, y)
	xl := s.newValue1(ssaop.OpDecimal128Lo, u64, x)
	yl := s.newValue1(ssaop.OpDecimal128Lo, u64, y)
	coeff := s.newValue2(ssaop.OpAnd64, u64, s.newValue2(ssaop.OpOr64, u64, xh, yh), s.constInt64(u64, int64(bid.CoeffMask128Hi)))
	words := s.newValue2(ssaop.OpEq64, boolT, coeff, zero)
	mask := s.constInt64(u64, int64(bid.Form2Mask128Hi))
	second := s.constInt64(u64, int64(bid.Form2Mask128Hi^(bid.Zero128Hi&bid.Form2Mask128Hi)))
	xfinite := s.newValue2(ssaop.OpNeq64, boolT, s.newValue2(ssaop.OpAnd64, u64, xh, mask), second)
	yfinite := s.newValue2(ssaop.OpNeq64, boolT, s.newValue2(ssaop.OpAnd64, u64, yh, mask), second)
	bSlow := s.f.NewBlock(block.BlockPlain)
	bAfter := s.f.NewBlock(block.BlockPlain)
	for _, guard := range []*ssa.Value{words, xfinite, yfinite} {
		b := s.endBlock()
		b.Kind = block.BlockIf
		b.SetControl(guard)
		next := s.f.NewBlock(block.BlockPlain)
		b.AddEdgeTo(next)
		b.AddEdgeTo(bSlow)
		s.startBlock(next)
	}

	xnative := s.newValue2(ssaop.OpXor64, u64, xh, s.constInt64(u64, int64(bid.Zero128Hi)))
	ynative := s.newValue2(ssaop.OpXor64, u64, yh, s.constInt64(u64, int64(bid.Zero128Hi)))
	shift := s.constInt64(u64, bid.ExpShift128)
	expMask := s.constInt64(u64, int64(bid.ExpMask128))
	xe := s.newValue2(ssaop.OpAnd64, u64, s.newValue2(ssaop.OpRsh64Ux64, u64, xnative, shift), expMask)
	ye := s.newValue2(ssaop.OpAnd64, u64, s.newValue2(ssaop.OpRsh64Ux64, u64, ynative, shift), expMask)
	exp := s.newValue2(ssaop.OpSub64, u64, s.newValue2(ssaop.OpAdd64, u64, xe, ye), s.constInt64(u64, bid.Bias128))
	expOK := s.newValue2(ssaop.OpLeq64U, boolT, exp, s.constInt64(u64, bid.MaxExp128+bid.Bias128))
	b := s.endBlock()
	b.Kind = block.BlockIf
	b.SetControl(expOK)
	bProduct := s.f.NewBlock(block.BlockPlain)
	b.AddEdgeTo(bProduct)
	b.AddEdgeTo(bSlow)
	s.startBlock(bProduct)

	product := s.newValue2(ssaop.OpMul64uhilo, types.NewTuple(u64, u64), xl, yl)
	hi := s.newValue1(ssaop.OpSelect0, u64, product)
	lo := s.newValue1(ssaop.OpSelect1, u64, product)
	maxHi, maxLo := s.constInt64(u64, int64(bid.MaxCoeff128Hi)), s.constInt64(u64, int64(bid.MaxCoeff128Lo))
	// Most word products are well below the 34-digit limit. Only inspect
	// the low-word boundary when the high word cannot establish that.
	below := s.newValue2(ssaop.OpLess64U, boolT, hi, maxHi)
	b = s.endBlock()
	b.Kind = block.BlockIf
	b.SetControl(below)
	bExact := s.f.NewBlock(block.BlockPlain)
	bBoundary := s.f.NewBlock(block.BlockPlain)
	b.AddEdgeTo(bExact)
	b.AddEdgeTo(bBoundary)
	s.startBlock(bBoundary)
	coeffOK := s.newValue2(ssaop.OpAndB, boolT, s.newValue2(ssaop.OpEq64, boolT, hi, maxHi), s.newValue2(ssaop.OpLeq64U, boolT, lo, maxLo))
	b = s.endBlock()
	b.Kind = block.BlockIf
	b.SetControl(coeffOK)
	b.AddEdgeTo(bExact)
	b.AddEdgeTo(bSlow)
	s.startBlock(bExact)

	signMask := bid.SignMask128Hi
	sign := s.newValue2(ssaop.OpAnd64, u64, s.newValue2(ssaop.OpXor64, u64, xh, yh), s.constInt64(u64, int64(signMask)))
	header := s.newValue2(ssaop.OpOr64, u64, sign, s.newValue2(ssaop.OpLsh64x64, u64, exp, shift))
	header = s.newValue2(ssaop.OpXor64, u64, header, s.constInt64(u64, int64(bid.Zero128Hi)))
	s.vars[bidMulVar] = s.newValue2(ssaop.OpDecimal128Make, rt, lo, s.newValue2(ssaop.OpOr64, u64, header, hi))
	s.endBlock().AddEdgeTo(bAfter)

	s.startBlock(bSlow)
	s.vars[bidMulVar] = s.softDecimalOp(ir.OMUL, rt, x, y)
	s.endBlock().AddEdgeTo(bAfter)
	s.startBlock(bAfter)
	return s.variable(bidMulVar, rt)
}
