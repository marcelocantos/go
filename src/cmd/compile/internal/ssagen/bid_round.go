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
	"cmd/internal/obj"
)

var bidRoundVar = ssaMarker("bidround")

// bidRounding lowers quantization and integral rounding of first-form word
// coefficients on BID targets. The guards prove a canonical finite source,
// a finite target, and either no change of quantum or removal of 1..19 digits.
// In the latter case a single unsigned division gives the exact quotient and
// remainder. Dropping at least one digit leaves room for the rounding carry;
// no overflow, underflow adjustment or second coefficient form is possible.
//
// The rounding predicate is the integer form of bid.roundUp: the remainder
// decides exact/below-half/tie/above-half, and invalid modes default to even.
// No approximate reciprocal or floating operation is involved. Constants
// propagated through the typed calls remove unused mode and target decoding.
// Any unproved case calls the portable typed helper with already-evaluated args;
// this preserves special values, arbitrary encodings and argument evaluation.
func (s *state) bidRounding(n *ir.CallExpr, args []*ssa.Value, integral bool, fallback *obj.LSym) *ssa.Value {
	u64, u8, boolT := types.Types[types.TUINT64], types.Types[types.TUINT8], types.Types[types.TBOOL]
	zero, one := s.constInt64(u64, 0), s.constInt64(u64, 1)
	x, mode := args[0], args[len(args)-1]
	is64 := x.Type.Kind() == types.TDECIMAL64
	shift, bias := bid.ExpShift128, bid.Bias128
	zeroEncoding := uint64(bid.Zero128Hi)
	expMask, coeffMask, formMask := uint64(bid.ExpMask128), uint64(bid.CoeffMask128Hi), uint64(bid.Form2Mask128Hi)
	if is64 {
		shift, bias, zeroEncoding = bid.ExpShift64, bid.Bias64, bid.Zero64
		expMask, coeffMask, formMask = bid.ExpMask64, 1<<bid.CoeffBits64-1, bid.Form2Mask64
	}
	high := func(v *ssa.Value) *ssa.Value {
		if is64 {
			return s.newValue1(ssaop.OpCopy, u64, v)
		}
		return s.newValue1(ssaop.OpDecimal128Hi, u64, v)
	}
	xh := high(x)
	native := s.newValue2(ssaop.OpXor64, u64, xh, s.constInt64(u64, int64(zeroEncoding)))
	coeff := s.newValue2(ssaop.OpAnd64, u64, native, s.constInt64(u64, int64(coeffMask)))
	finite := func(h *ssa.Value) *ssa.Value {
		mask := s.constInt64(u64, int64(formMask))
		second := s.constInt64(u64, int64(formMask^(zeroEncoding&formMask)))
		return s.newValue2(ssaop.OpNeq64, boolT, s.newValue2(ssaop.OpAnd64, u64, h, mask), second)
	}
	exponent := func(v *ssa.Value) *ssa.Value {
		return s.newValue2(ssaop.OpAnd64, u64, s.newValue2(ssaop.OpRsh64Ux64, u64, v, s.constInt64(u64, int64(shift))), s.constInt64(u64, int64(expMask)))
	}
	bSlow, bAfter := s.f.NewBlock(block.BlockPlain), s.f.NewBlock(block.BlockPlain)
	guard := func(v *ssa.Value) {
		b := s.endBlock()
		b.Kind = block.BlockIf
		b.SetControl(v)
		next := s.f.NewBlock(block.BlockPlain)
		b.AddEdgeTo(next)
		b.AddEdgeTo(bSlow)
		s.startBlock(next)
	}
	guard(finite(xh))
	if !is64 {
		guard(s.newValue2(ssaop.OpEq64, boolT, coeff, zero))
		coeff = s.newValue1(ssaop.OpDecimal128Lo, u64, x)
	}
	xe := exponent(native)
	var ye, unchanged *ssa.Value
	if integral {
		ye = s.constInt64(u64, int64(bias))
		unchanged = s.newValue2(ssaop.OpLeq64U, boolT, ye, xe)
	} else {
		yh := high(args[1])
		guard(finite(yh))
		ye = exponent(s.newValue2(ssaop.OpXor64, u64, yh, s.constInt64(u64, int64(zeroEncoding))))
		unchanged = s.newValue2(ssaop.OpEq64, boolT, xe, ye)
	}
	b := s.endBlock()
	b.Kind = block.BlockIf
	b.SetControl(unchanged)
	bSame, bShift := s.f.NewBlock(block.BlockPlain), s.f.NewBlock(block.BlockPlain)
	b.AddEdgeTo(bSame)
	b.AddEdgeTo(bShift)
	s.startBlock(bSame)
	s.vars[bidRoundVar] = x
	s.endBlock().AddEdgeTo(bAfter)
	s.startBlock(bShift)
	header := func() *ssa.Value {
		signMask := uint64(bid.SignMask64)
		h := s.newValue2(ssaop.OpOr64, u64, s.newValue2(ssaop.OpAnd64, u64, native, s.constInt64(u64, int64(signMask))), s.newValue2(ssaop.OpLsh64x64, u64, ye, s.constInt64(u64, int64(shift))))
		return s.newValue2(ssaop.OpXor64, u64, h, s.constInt64(u64, int64(zeroEncoding)))
	}
	if !integral && !is64 {
		// A decimal128 word at a larger exponent than y's by at most 19
		// places is scaled up to it, exactly, when the product is still a
		// word: 5 quantized to cents is 5.00. One multiplication by a
		// power of ten from the table, with the high word as the test. A
		// word is below 10^34, so the result is canonical, and a zero
		// stays a zero at the exponent of y. Whether x is above or below
		// y's exponent is a property of the data's kind, and a branch.
		above := s.newValue2(ssaop.OpLess64U, boolT, ye, xe)
		b := s.endBlock()
		b.Kind = block.BlockIf
		b.SetControl(above)
		bUp, bDown := s.f.NewBlock(block.BlockPlain), s.f.NewBlock(block.BlockPlain)
		b.AddEdgeTo(bUp)
		b.AddEdgeTo(bDown)
		s.startBlock(bUp)
		up := s.newValue2(ssaop.OpSub64, u64, xe, ye)
		guard(s.newValue2(ssaop.OpLeq64U, boolT, up, s.constInt64(u64, bidMaxWordPower)))
		ptrT := types.NewPtr(u64)
		table := s.newValue1A(ssaop.OpAddr, ptrT, bidComparePowers, s.sb)
		power := s.rawLoad(u64, s.newValue2(ssaop.OpAddPtr, ptrT, table, s.newValue2(ssaop.OpMul64, u64, up, s.constInt64(u64, bidWordBytes))))
		product := s.newValue2(ssaop.OpMul64uhilo, types.NewTuple(u64, u64), coeff, power)
		guard(s.newValue2(ssaop.OpEq64, boolT, s.newValue1(ssaop.OpSelect0, u64, product), zero))
		s.vars[bidRoundVar] = s.newValue2(ssaop.OpDecimal128Make, n.Type(), s.newValue1(ssaop.OpSelect1, u64, product), header())
		s.endBlock().AddEdgeTo(bAfter)
		s.startBlock(bDown)
	}
	gap := s.newValue2(ssaop.OpSub64, u64, ye, xe)
	const maxShift = bidMaxWordPower // 10^19 is the largest power of ten fitting uint64.
	guard(s.newValue2(ssaop.OpLeq64U, boolT, s.newValue2(ssaop.OpSub64, u64, gap, one), s.constInt64(u64, maxShift-1)))
	// Immutable content-addressed data, as in the exact addition lowering.
	const wordBytes = bidWordBytes
	ptrT := types.NewPtr(u64)
	table := s.newValue1A(ssaop.OpAddr, ptrT, bidRoundPowers, s.sb)
	index := s.newValue2(ssaop.OpMul64, u64, s.newValue2(ssaop.OpSub64, u64, gap, one), s.constInt64(u64, wordBytes))
	divisor := s.rawLoad(u64, s.newValue2(ssaop.OpAddPtr, ptrT, table, index))
	q := s.newValue2(ssaop.OpDiv64u, u64, coeff, divisor)
	r := s.newValue2(ssaop.OpSub64, u64, coeff, s.newValue2(ssaop.OpMul64, u64, q, divisor))
	half := s.newValue2(ssaop.OpRsh64Ux64, u64, divisor, one)
	above := s.newValue2(ssaop.OpLess64U, boolT, half, r)
	tie := s.newValue2(ssaop.OpEq64, boolT, half, r)
	odd := s.newValue2(ssaop.OpNeq64, boolT, s.newValue2(ssaop.OpAnd64, u64, q, one), zero)
	up := s.newValue2(ssaop.OpOrB, boolT, above, s.newValue2(ssaop.OpAndB, boolT, tie, odd))
	// Public and kernel rounding modes share these values. The portable
	// wrapper's mapping and independent public-API tests enforce the contract.
	increment := s.newValue1(ssaop.OpZeroExt8to64, u64, s.newValue1(ssaop.OpCvtBoolToUint8, u8, up))
	selectMode := func(m bid.Mode, v *ssa.Value) {
		eq := s.newValue2(ssaop.OpEq8, boolT, mode, s.constInt8(u8, int8(m)))
		v = s.newValue1(ssaop.OpZeroExt8to64, u64, s.newValue1(ssaop.OpCvtBoolToUint8, u8, v))
		increment = s.newValue3(ssaop.OpCondSelect, u64, v, increment, eq)
	}
	negative := s.newValue2(ssaop.OpLess64, boolT, native, zero)
	nonzero := s.newValue2(ssaop.OpNeq64, boolT, r, zero)
	selectMode(bid.ToNearestAway, s.newValue2(ssaop.OpOrB, boolT, above, tie))
	selectMode(bid.TowardZero, s.constBool(false))
	selectMode(bid.TowardPositive, s.newValue2(ssaop.OpAndB, boolT, nonzero, s.newValue1(ssaop.OpNot, boolT, negative)))
	selectMode(bid.TowardNegative, s.newValue2(ssaop.OpAndB, boolT, nonzero, negative))
	q = s.newValue2(ssaop.OpAdd64, u64, q, increment)
	if is64 {
		s.vars[bidRoundVar] = s.newValue1(ssaop.OpCopy, n.Type(), s.newValue2(ssaop.OpOr64, u64, header(), q))
	} else {
		s.vars[bidRoundVar] = s.newValue2(ssaop.OpDecimal128Make, n.Type(), q, header())
	}
	s.endBlock().AddEdgeTo(bAfter)
	s.startBlock(bSlow)
	if !is64 && bidKernelDirect() {
		// The kernel's general path, called directly as the operators'
		// are (bidKernelOp): the typed helper would only convert the
		// operands and the mode, whose values the public and the kernel
		// modes share, and call it.
		zeroHi := s.constInt64(u64, int64(zeroEncoding))
		kargs := []*ssa.Value{s.newValue2(ssaop.OpXor64, u64, xh, zeroHi), s.newValue1(ssaop.OpDecimal128Lo, u64, x)}
		fn := bidKernelIntegral
		if !integral {
			fn = bidKernelQuantize
			kargs = append(kargs, s.newValue2(ssaop.OpXor64, u64, high(args[1]), zeroHi), s.newValue1(ssaop.OpDecimal128Lo, u64, args[1]))
		}
		res := s.rtcall(fn, true, []*types.Type{u64, u64}, append(kargs, mode)...)
		s.vars[bidRoundVar] = s.newValue2(ssaop.OpDecimal128Make, n.Type(), res[1], s.newValue2(ssaop.OpXor64, u64, res[0], zeroHi))
	} else {
		s.vars[bidRoundVar] = s.rtcall(fallback, true, []*types.Type{n.Type()}, args...)[0]
	}
	s.endBlock().AddEdgeTo(bAfter)
	s.startBlock(bAfter)
	return s.variable(bidRoundVar, n.Type())
}
