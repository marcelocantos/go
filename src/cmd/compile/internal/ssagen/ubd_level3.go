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

// ubdLevel 3: the kernel's own first paths, promoted inline one at a time
// behind level 2's, for 64-bit coefficients. Each is the kernel leaf's
// code for the operand pairs it settles, so that it gives the kernel's
// result by construction, and every other pair is the kernel call
// (research/ubd-level4/README.md).

// ubd3 holds the blocks of one level-3 lowering: the kernel call that
// every refused pair reaches, and the join where the result is read.
type ubd3 struct {
	s           *state
	res         *ir.Name
	call, after *ssa.Block
}

func (s *state) newUBD3(name string) *ubd3 {
	return &ubd3{s: s, res: ssaMarker(name), call: s.f.NewBlock(block.BlockPlain), after: s.f.NewBlock(block.BlockPlain)}
}

// guard continues in a new block where cond holds and sends the rest to
// otherwise.
func (u *ubd3) guard(cond *ssa.Value, otherwise *ssa.Block) {
	s := u.s
	b := s.endBlock()
	b.Kind = block.BlockIf
	b.SetControl(cond)
	next := s.f.NewBlock(block.BlockPlain)
	b.AddEdgeTo(next)
	b.AddEdgeTo(otherwise)
	s.startBlock(next)
}

// branch ends the current block in a two-way branch on cond and returns
// the blocks for true and false.
func (u *ubd3) branch(cond *ssa.Value) (yes, no *ssa.Block) {
	return u.branchLikely(cond, ssa.BranchUnknown)
}

// branchLikely is branch with the prediction for cond.
func (u *ubd3) branchLikely(cond *ssa.Value, likely ssa.BranchPrediction) (yes, no *ssa.Block) {
	s := u.s
	b := s.endBlock()
	b.Kind = block.BlockIf
	b.SetControl(cond)
	b.Likely = likely
	yes, no = s.f.NewBlock(block.BlockPlain), s.f.NewBlock(block.BlockPlain)
	b.AddEdgeTo(yes)
	b.AddEdgeTo(no)
	return yes, no
}

// done makes v the result of the current block and ends it at the join.
func (u *ubd3) done(v *ssa.Value) {
	u.s.vars[u.res] = v
	u.s.endBlock().AddEdgeTo(u.after)
}

// finish emits the kernel call, built by call, and returns the result at
// the join.
func (u *ubd3) finish(rt *types.Type, call func() *ssa.Value) *ssa.Value {
	s := u.s
	s.startBlock(u.call)
	u.done(call())
	s.startBlock(u.after)
	r := s.variable(u.res, rt)
	delete(s.vars, u.res)
	return r
}

// ubdAddWord3 lowers x + y and x - y at ubdLevel 3 where level 2's guard
// has failed (ubdAddNarrow): the kernel's ubdAddWordTTE for a pair of
// one-word coefficients, the ties-to-even entries' first path. x - y is
// x + (-y): a word is not a NaN, so y's sign is flipped as it is.
//
//   - One exponent: level 2 took one sign (x + y) or an effective
//     subtraction (x - y), so what is left is the other: for x + y the
//     difference of the low words, the one that did not borrow chosen by
//     a select, the sign flipped by the borrow and an exact zero +0; for
//     x - y the sum, an add and its carry.
//   - Exponents 1 to 14 apart: a, the operand with the larger exponent,
//     is scaled by 10^gap, one 64×64 multiply from the table, and b added
//     or subtracted. Which operand is a goes with the data's kind (an
//     amount and a rate, a total and an amount), so it is a branch, not
//     a select: a select would put the comparison of the high words on
//     the running total's dependency chain, ahead of the multiply. A
//     difference that borrows out of the high word is b's magnitude
//     less a's, the sign flipped; an exact zero is +0. The result is at
//     b's exponent, the preferred one, and needs no test: a and b are
//     below 2^64 and 10^14 + 1 is below 2^48, so a × 10^gap + b is below
//     2^112 < 10^34, and a difference no larger. The BID-derived add of
//     the former level 3 had the same bound for the same reason
//     (ubdAddMaxGap).
//
// Every other pair (a two-word coefficient, a special, exponents more than
// 14 apart) is the kernel call.
func (s *state) ubdAddWord3(op ir.Op, rt *types.Type, x, y, xh, yh, xl, yl *ssa.Value) *ssa.Value {
	u := s.newUBD3("ubdadd3")
	u64, boolT := types.Types[types.TUINT64], types.Types[types.TBOOL]
	tup := types.NewTuple(u64, u64)
	zero := s.constInt64(u64, 0)
	c := func(v int64) *ssa.Value { return s.constInt64(u64, v) }
	signHi := c(bid.UBDSignHi)
	signBit := func(h *ssa.Value) *ssa.Value { // 0 or 1
		return s.newValue2(ssaop.OpAnd64, u64, s.newValue2(ssaop.OpRsh64Ux64, u64, h, c(bid.UBDSignShift)), c(1))
	}
	// Both coefficients one word: no coefficient bits in either high word,
	// which also excludes the specials.
	words := s.newValue2(ssaop.OpLsh64x64, u64, s.newValue2(ssaop.OpOr64, u64, xh, yh), c(64-bid.UBDExpShift+1))
	u.guard(s.newValue2(ssaop.OpEq64, boolT, words, zero), u.call)
	if op == ir.OSUB {
		yh = s.newValue2(ssaop.OpXor64, u64, yh, signHi)
	}
	sameExp := s.newValue2(ssaop.OpEq64, boolT, s.newValue2(ssaop.OpRsh64Ux64, u64, s.newValue2(ssaop.OpXor64, u64, xh, yh), c(bid.UBDExpShift)), zero)
	bSame, bGap := u.branch(sameExp)

	s.startBlock(bSame)
	if op == ir.OADD {
		// Opposite signs.
		d := s.newValue3(ssaop.OpSub64borrow, tup, xl, yl, zero)
		lo, bw := s.newValue1(ssaop.OpSelect0, u64, d), s.newValue1(ssaop.OpSelect1, u64, d)
		borrowed := s.newValue2(ssaop.OpNeq64, boolT, bw, zero)
		lo = s.newValue3(ssaop.OpCondSelect, u64, s.newValue2(ssaop.OpSub64, u64, yl, xl), lo, borrowed)
		nonzero := s.newValue1(ssaop.OpZeroExt8to64, u64, s.newValue1(ssaop.OpCvtBoolToUint8, types.Types[types.TUINT8], s.newValue2(ssaop.OpNeq64, boolT, lo, zero)))
		sign := s.newValue2(ssaop.OpAnd64, u64, s.newValue2(ssaop.OpXor64, u64, signBit(xh), bw), nonzero)
		hi := s.newValue2(ssaop.OpOr64, u64, s.newValue2(ssaop.OpAnd64, u64, xh, c(^int64(bid.UBDSignHi))), s.newValue2(ssaop.OpLsh64x64, u64, sign, c(bid.UBDSignShift)))
		u.done(s.newValue2(ssaop.OpDecimal128Make, rt, lo, hi))
	} else {
		// One sign after the flip: the sum.
		sum := s.newValue3(ssaop.OpAdd64carry, tup, xl, yl, zero)
		hi := s.newValue2(ssaop.OpOr64, u64, xh, s.newValue1(ssaop.OpSelect1, u64, sum))
		u.done(s.newValue2(ssaop.OpDecimal128Make, rt, s.newValue1(ssaop.OpSelect0, u64, sum), hi))
	}

	// Exponents apart: a has the larger (qExp is the high word's top bits,
	// and the exponents differ, so the signed order of the high words is
	// theirs).
	s.startBlock(bGap)
	ah, al, bh, bl := ssaMarker("ubdadd4ah"), ssaMarker("ubdadd4al"), ssaMarker("ubdadd4bh"), ssaMarker("ubdadd4bl")
	bSwap, bKeep := u.branch(s.newValue2(ssaop.OpLess64, boolT, xh, yh))
	bOrdered := s.f.NewBlock(block.BlockPlain)
	for _, p := range []struct {
		b              *ssa.Block
		hA, lA, hB, lB *ssa.Value
	}{{bSwap, yh, yl, xh, xl}, {bKeep, xh, xl, yh, yl}} {
		s.startBlock(p.b)
		s.vars[ah], s.vars[al], s.vars[bh], s.vars[bl] = p.hA, p.lA, p.hB, p.lB
		s.endBlock().AddEdgeTo(bOrdered)
	}
	s.startBlock(bOrdered)
	aH, aL, bH, bL := s.variable(ah, u64), s.variable(al, u64), s.variable(bh, u64), s.variable(bl, u64)
	for _, m := range []*ir.Name{ah, al, bh, bl} {
		delete(s.vars, m)
	}
	shift := c(bid.UBDExpShift)
	gap := s.newValue2(ssaop.OpSub64, u64, s.newValue2(ssaop.OpRsh64x64, u64, aH, shift), s.newValue2(ssaop.OpRsh64x64, u64, bH, shift))
	u.guard(s.newValue2(ssaop.OpLeq64U, boolT, gap, c(ubdAddMaxGap)), u.call)
	ptrT := types.NewPtr(u64)
	table := s.newValue1A(ssaop.OpAddr, ptrT, ubdAddPowers, s.sb)
	power := s.rawLoad(u64, s.newValue2(ssaop.OpAddPtr, ptrT, table, s.newValue2(ssaop.OpMul64, u64, gap, c(bidWordBytes))))
	prod := s.newValue2(ssaop.OpMul64uhilo, tup, aL, power)
	phi, plo := s.newValue1(ssaop.OpSelect0, u64, prod), s.newValue1(ssaop.OpSelect1, u64, prod)
	sameSign := s.newValue2(ssaop.OpEq64, boolT, s.newValue2(ssaop.OpAnd64, u64, s.newValue2(ssaop.OpXor64, u64, aH, bH), signHi), zero)
	rlo, rhi, rsign := ssaMarker("ubdadd4lo"), ssaMarker("ubdadd4hi"), ssaMarker("ubdadd4sign")
	bAdd, bSub := u.branch(sameSign)
	bCheck := s.f.NewBlock(block.BlockPlain)

	s.startBlock(bAdd)
	sum := s.newValue3(ssaop.OpAdd64carry, tup, plo, bL, zero)
	s.vars[rlo] = s.newValue1(ssaop.OpSelect0, u64, sum)
	s.vars[rhi] = s.newValue2(ssaop.OpAdd64, u64, phi, s.newValue1(ssaop.OpSelect1, u64, sum))
	s.vars[rsign] = signBit(aH)
	s.endBlock().AddEdgeTo(bCheck)

	s.startBlock(bSub)
	d := s.newValue3(ssaop.OpSub64borrow, tup, plo, bL, zero)
	dlo := s.newValue1(ssaop.OpSelect0, u64, d)
	h := s.newValue3(ssaop.OpSub64borrow, tup, phi, zero, s.newValue1(ssaop.OpSelect1, u64, d))
	dhi, bw := s.newValue1(ssaop.OpSelect0, u64, h), s.newValue1(ssaop.OpSelect1, u64, h)
	// A borrow out of the high word: a × 10^gap is below 2^64 and below
	// b, and the difference is b's magnitude less it, with b's sign.
	under := s.newValue2(ssaop.OpNeq64, boolT, bw, zero)
	lo := s.newValue3(ssaop.OpCondSelect, u64, s.newValue1(ssaop.OpNeg64, u64, dlo), dlo, under)
	hi := s.newValue3(ssaop.OpCondSelect, u64, zero, dhi, under)
	sign := s.newValue2(ssaop.OpXor64, u64, signBit(aH), bw)
	nonzero := s.newValue1(ssaop.OpZeroExt8to64, u64, s.newValue1(ssaop.OpCvtBoolToUint8, types.Types[types.TUINT8],
		s.newValue2(ssaop.OpNeq64, boolT, s.newValue2(ssaop.OpOr64, u64, hi, lo), zero)))
	s.vars[rlo], s.vars[rhi], s.vars[rsign] = lo, hi, s.newValue2(ssaop.OpAnd64, u64, sign, nonzero)
	s.endBlock().AddEdgeTo(bCheck)

	s.startBlock(bCheck)
	Lo, Hi, Sign := s.variable(rlo, u64), s.variable(rhi, u64), s.variable(rsign, u64)
	for _, m := range []*ir.Name{rlo, rhi, rsign} {
		delete(s.vars, m)
	}
	header := s.newValue2(ssaop.OpOr64, u64, s.newValue2(ssaop.OpAnd64, u64, bH, c(^int64(bid.UBDSignHi))),
		s.newValue2(ssaop.OpOr64, u64, s.newValue2(ssaop.OpLsh64x64, u64, Sign, c(bid.UBDSignShift)), Hi))
	u.done(s.newValue2(ssaop.OpDecimal128Make, rt, Lo, header))

	return u.finish(rt, func() *ssa.Value { return s.softDecimalOp(op, rt, x, y) })
}

// ubdMulWord3 is x * y at ubdLevel 3 where level 2's guard has failed
// (ubdMulFast): the kernel's ubdMulShort128 for a pair of one-word
// coefficients, whose product needs no test of the factors, only of
// itself. The 64×64 product below 2^112, so below 10^34, is exact at the
// sum of the exponents, which must be in the format; the sign is the XOR
// of the operands'. Level 2 takes factors below 2^56; this takes the
// products of larger word factors that still fit. Every other pair is the
// kernel call.
func (s *state) ubdMulWord3(rt *types.Type, x, y, xh, yh, xl, yl *ssa.Value) *ssa.Value {
	u := s.newUBD3("ubdmul3")
	u64, boolT := types.Types[types.TUINT64], types.Types[types.TBOOL]
	zero := s.constInt64(u64, 0)
	c := func(v int64) *ssa.Value { return s.constInt64(u64, v) }
	words := s.newValue2(ssaop.OpLsh64x64, u64, s.newValue2(ssaop.OpOr64, u64, xh, yh), c(64-bid.UBDExpShift+1))
	u.guard(s.newValue2(ssaop.OpEq64, boolT, words, zero), u.call)
	shift := c(bid.UBDExpShift)
	exp := s.newValue2(ssaop.OpAdd64, u64, s.newValue2(ssaop.OpRsh64x64, u64, xh, shift), s.newValue2(ssaop.OpRsh64x64, u64, yh, shift))
	u.guard(s.newValue2(ssaop.OpLeq64U, boolT, s.newValue2(ssaop.OpAdd64, u64, exp, c(-bid.MinExp128)), c(bid.MaxExp128-bid.MinExp128)), u.call)
	prod := s.newValue2(ssaop.OpMul64uhilo, types.NewTuple(u64, u64), xl, yl)
	phi, plo := s.newValue1(ssaop.OpSelect0, u64, prod), s.newValue1(ssaop.OpSelect1, u64, prod)
	u.guard(s.newValue2(ssaop.OpEq64, boolT, s.newValue2(ssaop.OpRsh64Ux64, u64, phi, c(48)), zero), u.call)
	sign := s.newValue2(ssaop.OpAnd64, u64, s.newValue2(ssaop.OpXor64, u64, xh, yh), c(bid.UBDSignHi))
	header := s.newValue2(ssaop.OpOr64, u64, sign, s.newValue2(ssaop.OpLsh64x64, u64, exp, shift))
	u.done(s.newValue2(ssaop.OpDecimal128Make, rt, plo, s.newValue2(ssaop.OpOr64, u64, header, phi)))
	return u.finish(rt, func() *ssa.Value { return s.softDecimalOp(ir.OMUL, rt, x, y) })
}

// ubdCmpWord3 is the comparison op of x and y at ubdLevel 3 where level
// 2's guard has failed (ubdCompareNarrow): the kernel's ubdCmpWord, the
// predicate entries' first path, for a pair of one-word coefficients
// (which excludes NaN) whose exponents are at most 19 apart, written for
// the one predicate the site asks. a, the operand with the larger
// exponent, scaled by 10^gap, is compared with b once each way, P > b and
// P < b, one borrow chain each; which of the two is x < y depends on
// whether x is a and on the sign, and one select picks it. Signs that
// differ decide by themselves, unless both values are zero. Branch-free
// past the guards, since a comparison's outcome is the data's, not its
// kind's. Every other pair is the predicate entry's call.
func (s *state) ubdCmpWord3(op ir.Op, xh, yh, xl, yl *ssa.Value) *ssa.Value {
	u := s.newUBD3("ubdcmp3")
	u64, boolT := types.Types[types.TUINT64], types.Types[types.TBOOL]
	zero := s.constInt64(u64, 0)
	notB := func(a *ssa.Value) *ssa.Value { return s.newValue1(ssaop.OpNot, boolT, a) }
	less, equal := s.ubdWordOrder(u, xh, yh, xl, yl)
	var res *ssa.Value
	switch op {
	case ir.OEQ:
		res = equal()
	case ir.ONE:
		res = notB(equal())
	case ir.OLT:
		res = s.newValue2(ssaop.OpNeq64, boolT, less(true), zero)
	case ir.OGT:
		res = s.newValue2(ssaop.OpNeq64, boolT, less(false), zero)
	case ir.OLE:
		res = s.newValue2(ssaop.OpEq64, boolT, less(false), zero)
	case ir.OGE:
		res = s.newValue2(ssaop.OpEq64, boolT, less(true), zero)
	default:
		s.Fatalf("ubdCmpWord3: %v", op)
	}
	u.done(res)
	return u.finish(boolT, func() *ssa.Value { return s.ubdComparePredCall(op, xl, xh, yl, yh) })
}

// ubdWordOrder emits ubdCmpWord3's guards, which send every other pair to
// u's call, and returns the orders of x and y: less(true) is 1 when
// x < y, else 0, less(false) the same for y < x, and equal() whether
// x == y.
func (s *state) ubdWordOrder(u *ubd3, xh, yh, xl, yl *ssa.Value) (less func(xFirst bool) *ssa.Value, equal func() *ssa.Value) {
	u64, boolT := types.Types[types.TUINT64], types.Types[types.TBOOL]
	tup := types.NewTuple(u64, u64)
	zero := s.constInt64(u64, 0)
	c := func(v int64) *ssa.Value { return s.constInt64(u64, v) }
	or := func(a, b *ssa.Value) *ssa.Value { return s.newValue2(ssaop.OpOr64, u64, a, b) }
	xor := func(a, b *ssa.Value) *ssa.Value { return s.newValue2(ssaop.OpXor64, u64, a, b) }
	sel := func(a, b, cond *ssa.Value) *ssa.Value { return s.newValue3(ssaop.OpCondSelect, u64, a, b, cond) }
	notB := func(a *ssa.Value) *ssa.Value { return s.newValue1(ssaop.OpNot, boolT, a) }
	words := s.newValue2(ssaop.OpLsh64x64, u64, or(xh, yh), c(64-bid.UBDExpShift+1))
	u.guard(s.newValue2(ssaop.OpEq64, boolT, words, zero), u.call)
	shift := c(bid.UBDExpShift)
	gap := s.newValue2(ssaop.OpSub64, u64, s.newValue2(ssaop.OpRsh64x64, u64, xh, shift), s.newValue2(ssaop.OpRsh64x64, u64, yh, shift))
	yIsA := s.newValue2(ssaop.OpLess64, boolT, gap, zero) // y has the larger exponent
	g := sel(s.newValue1(ssaop.OpNeg64, u64, gap), gap, yIsA)
	u.guard(s.newValue2(ssaop.OpLeq64U, boolT, g, c(bidMaxWordPower)), u.call)
	ptrT := types.NewPtr(u64)
	table := s.newValue1A(ssaop.OpAddr, ptrT, bidComparePowers, s.sb)
	power := s.rawLoad(u64, s.newValue2(ssaop.OpAddPtr, ptrT, table, s.newValue2(ssaop.OpMul64, u64, g, c(bidWordBytes))))
	a, b := sel(yl, xl, yIsA), sel(xl, yl, yIsA)
	prod := s.newValue2(ssaop.OpMul64uhilo, tup, a, power)
	phi, plo := s.newValue1(ssaop.OpSelect0, u64, prod), s.newValue1(ssaop.OpSelect1, u64, prod)
	// borrow is 1 when (xh, xl) < (yh, yl), else 0. The orders are kept
	// as 0 or 1 in a word and chosen by CondSelect on words, which both
	// targets lower to a conditional select; a bool is formed once, at
	// the end (a CondSelect of bools has no lowering on amd64).
	borrow := func(xh, xl, yh, yl *ssa.Value) *ssa.Value {
		d := s.newValue3(ssaop.OpSub64borrow, tup, xl, yl, zero)
		d = s.newValue3(ssaop.OpSub64borrow, tup, xh, yh, s.newValue1(ssaop.OpSelect1, u64, d))
		return s.newValue1(ssaop.OpSelect1, u64, d)
	}
	negX := s.newValue2(ssaop.OpNeq64, boolT, s.newValue2(ssaop.OpAnd64, u64, xh, c(bid.UBDSignHi)), zero)
	signsDiffer := s.newValue2(ssaop.OpNeq64, boolT, s.newValue2(ssaop.OpAnd64, u64, xor(xh, yh), c(bid.UBDSignHi)), zero)
	notBothZero := s.newValue2(ssaop.OpNeq64, boolT, or(xl, yl), zero)
	// x before y, 1 or 0, adjusted for the sign: with one sign, x < y is
	// |x| < |y| for a positive sign and |y| < |x| for a negative one, and
	// |x| < |y| is P < b when x is a, P > b when y is.
	less = func(xFirst bool) *ssa.Value { // x < y, or y < x when !xFirst
		pLess, pMore := borrow(phi, plo, zero, b), borrow(zero, b, phi, plo)
		flip := s.newValue2(ssaop.OpNeqB, boolT, yIsA, negX)
		if !xFirst {
			flip = notB(flip)
		}
		same := sel(pMore, pLess, flip)
		// Signs that differ: the negative one is less, unless both are 0.
		first := negX
		if !xFirst {
			first = notB(negX)
		}
		diff := sel(s.newValue1(ssaop.OpZeroExt8to64, u64, s.newValue1(ssaop.OpCvtBoolToUint8, types.Types[types.TUINT8],
			s.newValue2(ssaop.OpAndB, boolT, first, notBothZero))), same, signsDiffer)
		return diff
	}
	equal = func() *ssa.Value {
		// One sign and P == b, or both zero.
		eqMag := s.newValue2(ssaop.OpEq64, boolT, or(phi, xor(plo, b)), zero)
		return s.newValue2(ssaop.OpOrB, boolT, s.newValue2(ssaop.OpAndB, boolT, notB(signsDiffer), eqMag), notB(notBothZero))
	}
	return less, equal
}

// ubdMinMax3 is the language's min (op OMIN) or max of x and y at
// ubdLevel 3: one exponent and one sign, as level 2 compares them, and
// then the kernel's ubdMin128 and ubdMax128 for a pair of one-word
// coefficients at most 19 places apart (ubdWordOrder) that are not equal:
// the lesser or the greater operand, chosen by selects. Equal values (-0
// and +0, members of one cohort, one pattern twice) and every other pair
// are the kernel call, whose totalOrder decides them.
func (s *state) ubdMinMax3(op ir.Op, rt *types.Type, x, y *ssa.Value) *ssa.Value {
	u := s.newUBD3("ubdminmax3")
	u64, boolT := types.Types[types.TUINT64], types.Types[types.TBOOL]
	zero := s.constInt64(u64, 0)
	xh, yh := s.newValue1(ssaop.OpDecimal128Hi, u64, x), s.newValue1(ssaop.OpDecimal128Hi, u64, y)
	xl, yl := s.newValue1(ssaop.OpDecimal128Lo, u64, x), s.newValue1(ssaop.OpDecimal128Lo, u64, y)
	c := func(v int64) *ssa.Value { return s.constInt64(u64, v) }
	// One exponent, one sign, one-word coefficients (level 2's comparison
	// guard): the low words order the values, reversed for a negative
	// sign by XORing both with the sign mask; equal low words are one
	// pattern, and the result is x.
	sameGuard := s.newValue2(ssaop.OpOr64, u64, s.newValue2(ssaop.OpXor64, u64, xh, yh),
		s.newValue2(ssaop.OpLsh64x64, u64, xh, c(64-bid.UBDExpShift+1)))
	bSame, bGeneral := u.branchLikely(s.newValue2(ssaop.OpEq64, boolT, sameGuard, zero), ssa.BranchLikely)
	s.startBlock(bSame)
	m := s.newValue2(ssaop.OpRsh64x64, u64, s.newValue2(ssaop.OpLsh64x64, u64, xh, c(64-bid.UBDExpShift)), c(63))
	xo, yo := s.newValue2(ssaop.OpXor64, u64, xl, m), s.newValue2(ssaop.OpXor64, u64, yl, m)
	takeY := s.newValue2(ssaop.OpLess64U, boolT, yo, xo) // min: y < x
	if op == ir.OMAX {
		takeY = s.newValue2(ssaop.OpLess64U, boolT, xo, yo)
	}
	u.done(s.newValue2(ssaop.OpDecimal128Make, rt, s.newValue3(ssaop.OpCondSelect, u64, yl, xl, takeY), xh))
	s.startBlock(bGeneral)
	less, _ := s.ubdWordOrder(u, xh, yh, xl, yl)
	lt, gt := less(true), less(false)
	u.guard(s.newValue2(ssaop.OpNeq64, boolT, s.newValue2(ssaop.OpOr64, u64, lt, gt), zero), u.call)
	takeX := lt // min: x when x < y
	if op == ir.OMAX {
		takeX = gt
	}
	pick := s.newValue2(ssaop.OpNeq64, boolT, takeX, zero)
	lo := s.newValue3(ssaop.OpCondSelect, u64, xl, yl, pick)
	hi := s.newValue3(ssaop.OpCondSelect, u64, xh, yh, pick)
	u.done(s.newValue2(ssaop.OpDecimal128Make, rt, lo, hi))
	return u.finish(rt, func() *ssa.Value { return s.softDecimalOp(op, rt, x, y) })
}
