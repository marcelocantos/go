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

// Decimal operators on targets whose native decimal encoding is DPD
// (ir.DecimalDPD, linux/ppc64le). There the operators and most conversions
// become the generic DPD ops, which the back end lowers to the decimal
// floating-point instructions; the rest stay runtime calls (softDecimalOp,
// softDecimalCall). A DPD op takes and returns native encodings, so the only
// work around it is the XOR with the zero mask between the memory form and
// the native encoding, the same XOR the runtime calls have on BID targets;
// the optimizer cancels the pair of XORs between two operations. Every
// lowering below gives the result internal/bid's contract fixes, bit for bit
// once converted to BID; internal/bid's TestHardware checks that on the
// target. The contract's rules that the hardware meets by itself:
//
//   - the value, quantum, overflow, underflow and signed zeros of +, -, *
//     and / under round-half-even, which is FPSCR[DRN] = 0 (see below);
//   - NaN propagation: the first signaling NaN, else the first NaN, made
//     quiet with its sign and payload, and the canonical quiet NaN for an
//     invalid operation (decNumber's rule, which the POWER unit implements);
//   - Sub propagating a NaN y with y's own sign;
//   - comparisons by value with NaN unordered, -0 == +0;
//   - Widen, and Narrow of every operand but a NaN.
//
// What the hardware does differently, and the code here adds:
//
//   - DCTFIX converts a NaN to math.MinInt64, the contract to 0: a select
//     on the operand's NaN bits.
//   - DRDPQ keeps the low digits of a decimal128 NaN payload that does not
//     fit decimal64, the contract makes it zero: dpdNarrow narrows a NaN
//     in GPRs.
//   - There is no unsigned convert-from-fixed before ISA 3.1: uint64 to
//     decimal goes through decimal128 (see dpdFromUint64).
//   - Go's min and max are not instructions: dpdMinMax.
//
// Rounding. The arithmetic instructions round by FPSCR[DRN] and have no
// rounding-mode operand. Go relies on DRN being 0, round to nearest with
// ties to even, as it relies on FPSCR[RN] being 0 for float64: the kernel
// zeroes the FPSCR at exec, a new thread inherits its creator's, Go code
// never writes either field (the one FPSCR write in the tree, MOVFL F0,
// FPSCR in runtime/preempt_ppc64x.s, restores the value asynchronous
// preemption saved), and cgo callees that change the rounding mode and
// return without restoring it break float64 arithmetic just as they break
// decimal. internal/bid's TestDecimalRoundingMode reads FPSCR[DRN] on the
// target. Where an instruction has an explicit rounding-mode
// control, the conversions to integer use it (DRINTN with R=0, RMC=01,
// toward zero) because the language truncates there.

// dpdMask64 and dpdMask128 are the masks between the memory form and the
// native encoding (of the high word, for decimal128).
func (s *state) dpdMask64() *ssa.Value {
	return s.constInt64(types.Types[types.TUINT64], int64(bid.DPDZero64))
}

func (s *state) dpdMask128() *ssa.Value {
	return s.constInt64(types.Types[types.TUINT64], int64(bid.DPDZero128Hi))
}

// dpdIn64 returns the native encoding of the decimal64 a.
func (s *state) dpdIn64(a *ssa.Value) *ssa.Value {
	u64 := types.Types[types.TUINT64]
	return s.newValue2(ssaop.OpXor64, u64, s.newValue1(ssaop.OpCopy, u64, a), s.dpdMask64())
}

// dpdIn128 returns the native encoding of the decimal128 a, as its halves.
func (s *state) dpdIn128(a *ssa.Value) (hi, lo *ssa.Value) {
	u64 := types.Types[types.TUINT64]
	hi = s.newValue2(ssaop.OpXor64, u64, s.newValue1(ssaop.OpDecimal128Hi, u64, a), s.dpdMask128())
	return hi, s.newValue1(ssaop.OpDecimal128Lo, u64, a)
}

// dpdOut64 returns the decimal64 of type t whose native encoding is x.
func (s *state) dpdOut64(t *types.Type, x *ssa.Value) *ssa.Value {
	u64 := types.Types[types.TUINT64]
	return s.newValue1(ssaop.OpCopy, t, s.newValue2(ssaop.OpXor64, u64, x, s.dpdMask64()))
}

// dpdOut128 returns the decimal128 of type t whose native encoding has the
// halves hi and lo.
func (s *state) dpdOut128(t *types.Type, hi, lo *ssa.Value) *ssa.Value {
	u64 := types.Types[types.TUINT64]
	return s.newValue2(ssaop.OpDecimal128Make, t, lo, s.newValue2(ssaop.OpXor64, u64, hi, s.dpdMask128()))
}

// dpdTuple splits the (hi, lo) result of a decimal128 DPD op.
func (s *state) dpdTuple(v *ssa.Value) (hi, lo *ssa.Value) {
	u64 := types.Types[types.TUINT64]
	return s.newValue1(ssaop.OpSelect0, u64, v), s.newValue1(ssaop.OpSelect1, u64, v)
}

func (s *state) dpdTupleType() *types.Type {
	u64 := types.Types[types.TUINT64]
	return types.NewTuple(u64, u64)
}

var dpdArith = map[opAndType]ssaop.Op{
	{ir.OADD, types.TDECIMAL64}:  ssaop.OpDPDAdd64,
	{ir.OSUB, types.TDECIMAL64}:  ssaop.OpDPDSub64,
	{ir.OMUL, types.TDECIMAL64}:  ssaop.OpDPDMul64,
	{ir.ODIV, types.TDECIMAL64}:  ssaop.OpDPDDiv64,
	{ir.OADD, types.TDECIMAL128}: ssaop.OpDPDAdd128,
	{ir.OSUB, types.TDECIMAL128}: ssaop.OpDPDSub128,
	{ir.OMUL, types.TDECIMAL128}: ssaop.OpDPDMul128,
	{ir.ODIV, types.TDECIMAL128}: ssaop.OpDPDDiv128,
	{ir.OEQ, types.TDECIMAL64}:   ssaop.OpDPDEq64,
	{ir.ONE, types.TDECIMAL64}:   ssaop.OpDPDEq64,
	{ir.OLT, types.TDECIMAL64}:   ssaop.OpDPDLess64,
	{ir.OLE, types.TDECIMAL64}:   ssaop.OpDPDLeq64,
	{ir.OEQ, types.TDECIMAL128}:  ssaop.OpDPDEq128,
	{ir.ONE, types.TDECIMAL128}:  ssaop.OpDPDEq128,
	{ir.OLT, types.TDECIMAL128}:  ssaop.OpDPDLess128,
	{ir.OLE, types.TDECIMAL128}:  ssaop.OpDPDLeq128,
}

// decimalOp is the operator op on decimal operands of a common type, with
// result type rt: on a DPD target the hardware's instructions, elsewhere a
// runtime call.
func (s *state) decimalOp(op ir.Op, rt *types.Type, args ...*ssa.Value) *ssa.Value {
	if !ir.DecimalDPD() {
		return s.softDecimalOp(op, rt, args...)
	}
	x, y := args[0], args[1]
	if op == ir.OMIN || op == ir.OMAX {
		return s.dpdMinMax(op == ir.OMAX, rt, x, y)
	}
	dop, ok := dpdArith[opAndType{op, x.Type.Kind()}]
	if !ok {
		s.Fatalf("unhandled decimal operator %v on %v", op, x.Type)
	}
	var r *ssa.Value
	if x.Type.Kind() == types.TDECIMAL64 {
		t := types.Types[types.TUINT64]
		if rt.IsBoolean() {
			t = rt
		}
		r = s.newValue2(dop, t, s.dpdIn64(x), s.dpdIn64(y))
	} else {
		xh, xl := s.dpdIn128(x)
		yh, yl := s.dpdIn128(y)
		if rt.IsBoolean() {
			r = s.newValue4(dop, rt, xh, xl, yh, yl)
		} else {
			hi, lo := s.dpdTuple(s.newValue4(dop, s.dpdTupleType(), xh, xl, yh, yl))
			return s.dpdOut128(rt, hi, lo)
		}
	}
	switch {
	case op == ir.ONE:
		return s.newValue1(ssaop.OpNot, rt, r)
	case rt.IsBoolean():
		return r
	}
	return s.dpdOut64(rt, r)
}

// dpdIsNaN reports whether the native word w holding the combination field
// is a NaN's: bits 58 to 62 are 11111 in DPD as in BID.
func (s *state) dpdIsNaN(w *ssa.Value) *ssa.Value {
	u64 := types.Types[types.TUINT64]
	m := s.constInt64(u64, int64(bid.NaN64))
	return s.newValue2(ssaop.OpEq64, types.Types[types.TBOOL], s.newValue2(ssaop.OpAnd64, u64, w, m), m)
}

func (s *state) dpdSignbit(w *ssa.Value) *ssa.Value {
	return s.newValue2(ssaop.OpLess64, types.Types[types.TBOOL], s.newValue1(ssaop.OpCopy, types.Types[types.TINT64], w), s.constInt64(types.Types[types.TINT64], 0))
}

var dpdMinMaxVar = ssaMarker("dpdminmax")

// dpdMinMax is the built-in min (or max, if isMax) of the decimals x and y,
// as internal/bid's Min64 and Max64 define it. If either is a NaN the result
// is the NaN that "NaN results" chooses, made quiet, which is what DADD of
// the two operands gives; that path is a branch, since NaNs are rare.
// Otherwise the result is the lesser (greater) by value with -0 below +0,
// and x when they are equal in value and sign. Divergence: Min64 also
// canonicalizes x, but every DPD encoding is canonical in value, and a
// redundant declet survives only from unsafe memory, so the result is the
// operand's encoding as it is.
func (s *state) dpdMinMax(isMax bool, rt *types.Type, x, y *ssa.Value) *ssa.Value {
	boolT := types.Types[types.TBOOL]
	wide := rt.Kind() == types.TDECIMAL128
	var xh, xl, yh, yl *ssa.Value
	if wide {
		xh, xl = s.dpdIn128(x)
		yh, yl = s.dpdIn128(y)
	} else {
		xh, yh = s.dpdIn64(x), s.dpdIn64(y)
	}
	nan := s.newValue2(ssaop.OpOrB, boolT, s.dpdIsNaN(xh), s.dpdIsNaN(yh))

	b := s.endBlock()
	b.Kind = block.BlockIf
	b.SetControl(nan)
	b.Likely = ssa.BranchUnlikely
	bNaN := s.f.NewBlock(block.BlockPlain)
	bNum := s.f.NewBlock(block.BlockPlain)
	bAfter := s.f.NewBlock(block.BlockPlain)

	b.AddEdgeTo(bNaN)
	s.startBlock(bNaN)
	if wide {
		hi, lo := s.dpdTuple(s.newValue4(ssaop.OpDPDAdd128, s.dpdTupleType(), xh, xl, yh, yl))
		s.vars[dpdMinMaxVar] = s.dpdOut128(rt, hi, lo)
	} else {
		s.vars[dpdMinMaxVar] = s.dpdOut64(rt, s.newValue2(ssaop.OpDPDAdd64, types.Types[types.TUINT64], xh, yh))
	}
	s.endBlock()
	bNaN.AddEdgeTo(bAfter)

	b.AddEdgeTo(bNum)
	s.startBlock(bNum)
	// For min, take y when y < x, or when they are equal and y is -0 and
	// x is +0. For max, swap the roles in both tests.
	less := func(ah, al, ch, cl *ssa.Value) *ssa.Value { // a < c
		if wide {
			return s.newValue4(ssaop.OpDPDLess128, boolT, ah, al, ch, cl)
		}
		return s.newValue2(ssaop.OpDPDLess64, boolT, ah, ch)
	}
	var eq *ssa.Value
	if wide {
		eq = s.newValue4(ssaop.OpDPDEq128, boolT, xh, xl, yh, yl)
	} else {
		eq = s.newValue2(ssaop.OpDPDEq64, boolT, xh, yh)
	}
	var below, signs *ssa.Value
	sx, sy := s.dpdSignbit(xh), s.dpdSignbit(yh)
	if isMax {
		below = less(xh, xl, yh, yl)
		signs = s.newValue2(ssaop.OpAndB, boolT, sx, s.newValue1(ssaop.OpNot, boolT, sy))
	} else {
		below = less(yh, yl, xh, xl)
		signs = s.newValue2(ssaop.OpAndB, boolT, sy, s.newValue1(ssaop.OpNot, boolT, sx))
	}
	pickY := s.newValue2(ssaop.OpOrB, boolT, below, s.newValue2(ssaop.OpAndB, boolT, eq, signs))
	u64 := types.Types[types.TUINT64]
	if wide {
		hi := s.newValue3(ssaop.OpCondSelect, u64, yh, xh, pickY)
		lo := s.newValue3(ssaop.OpCondSelect, u64, yl, xl, pickY)
		s.vars[dpdMinMaxVar] = s.dpdOut128(rt, hi, lo)
	} else {
		s.vars[dpdMinMaxVar] = s.dpdOut64(rt, s.newValue3(ssaop.OpCondSelect, u64, yh, xh, pickY))
	}
	s.endBlock()
	bNum.AddEdgeTo(bAfter)

	s.startBlock(bAfter)
	return s.variable(dpdMinMaxVar, rt)
}

// dpdConv converts v from the kind fk to the kind tk, where one of them is a
// decimal kind and an integer kind has been widened to TINT64 or TUINT64;
// smallUnsigned reports that v is an unsigned integer narrower than 64 bits,
// zero-extended, and so a non-negative int64. tt is the result type, or the
// 64-bit integer type for an integer result. dpdConv returns nil for the
// conversions that stay runtime calls on a DPD target: those between decimal
// and binary floating point, which the hardware lacks, and those from a
// decimal to an unsigned integer, for which POWER has no instruction before
// ISA 3.1 (DCTFIXQQ converts to a signed 128-bit integer) and which would
// need a decimal128 subtraction and a compare to synthesize.
func (s *state) dpdConv(v *ssa.Value, fk, tk types.Kind, smallUnsigned bool, tt *types.Type) *ssa.Value {
	i64 := types.Types[types.TINT64]
	switch {
	case fk == types.TDECIMAL64 && tk == types.TDECIMAL128:
		hi, lo := s.dpdTuple(s.newValue1(ssaop.OpDPDWiden, s.dpdTupleType(), s.dpdIn64(v)))
		return s.dpdOut128(tt, hi, lo)
	case fk == types.TDECIMAL128 && tk == types.TDECIMAL64:
		hi, lo := s.dpdIn128(v)
		return s.dpdOut64(tt, s.dpdNarrow(hi, lo))
	case fk == types.TINT64 || fk == types.TUINT64 && smallUnsigned:
		v = s.newValue1(ssaop.OpCopy, i64, v)
		if tk == types.TDECIMAL64 {
			return s.dpdOut64(tt, s.newValue1(ssaop.OpDPDFromInt64, types.Types[types.TUINT64], v))
		}
		hi, lo := s.dpdTuple(s.newValue1(ssaop.OpDPDFromInt64To128, s.dpdTupleType(), v))
		return s.dpdOut128(tt, hi, lo)
	case fk == types.TUINT64:
		return s.dpdFromUint64(v, tk, tt)
	case tk == types.TINT64 && (fk == types.TDECIMAL64 || fk == types.TDECIMAL128):
		// DCTFIX gives math.MinInt64 for a NaN and the contract 0.
		var r, hi *ssa.Value
		if fk == types.TDECIMAL64 {
			hi = s.dpdIn64(v)
			r = s.newValue1(ssaop.OpDPDToInt64, i64, hi)
		} else {
			var lo *ssa.Value
			hi, lo = s.dpdIn128(v)
			r = s.newValue2(ssaop.OpDPDToInt64From128, i64, hi, lo)
		}
		return s.newValue3(ssaop.OpCondSelect, i64, s.constInt64(i64, 0), r, s.dpdIsNaN(hi))
	}
	return nil
}

// dpdFromUint64 converts the uint64 x to the decimal kind tk. It converts x
// as an int64 to decimal128, exactly, and adds 2^64 when the int64 is
// negative: the sum x is exact in decimal128 and keeps the exponent 0 the
// contract gives, since the addend is 18446744073709551616E+0 or 0E+0. For
// decimal64 the exact decimal128 is then narrowed, which rounds once.
func (s *state) dpdFromUint64(x *ssa.Value, tk types.Kind, tt *types.Type) *ssa.Value {
	u64 := types.Types[types.TUINT64]
	i64 := types.Types[types.TINT64]
	ah, al := s.dpdTuple(s.newValue1(ssaop.OpDPDFromInt64To128, s.dpdTupleType(), s.newValue1(ssaop.OpCopy, i64, x)))
	two64, _ := bid.Pack128(false, bid.Uint128{Hi: 1}, 0)
	two64 = bid.DPDFromBID128(two64)
	// mask is all ones when x ≥ 2^63, and selects between 2^64 and 0E+0.
	mask := s.newValue2(ssaop.OpRsh64x64, i64, s.newValue1(ssaop.OpCopy, i64, x), s.constInt64(u64, 63))
	mask = s.newValue1(ssaop.OpCopy, u64, mask)
	ch := s.newValue2(ssaop.OpXor64, u64, s.dpdMask128(),
		s.newValue2(ssaop.OpAnd64, u64, mask, s.constInt64(u64, int64(two64.Hi^bid.DPDZero128Hi))))
	cl := s.newValue2(ssaop.OpAnd64, u64, mask, s.constInt64(u64, int64(two64.Lo)))
	hi, lo := s.dpdTuple(s.newValue4(ssaop.OpDPDAdd128, s.dpdTupleType(), ah, al, ch, cl))
	if tk == types.TDECIMAL128 {
		return s.dpdOut128(tt, hi, lo)
	}
	return s.dpdOut64(tt, s.newValue2(ssaop.OpDPDNarrow, u64, hi, lo)) // never a NaN
}

// dpdNarrow converts the decimal128 with native halves hi and lo to
// decimal64, as internal/bid's Narrow does. DRDPQ does all of it but for a
// NaN whose payload is 10^15 or more, which the contract makes zero and
// decNumber, and so QEMU, truncates to its low 15 digits instead. A NaN is
// therefore narrowed here, in GPRs, whatever the hardware does with it: the
// result has the operand's sign, the quiet NaN's combination field, no
// reserved bits, and the payload's low five declets if the other six are
// zero, which they are exactly when the payload is below 10^15 (no declet
// but 0 encodes the digits 000).
func (s *state) dpdNarrow(hi, lo *ssa.Value) *ssa.Value {
	u64 := types.Types[types.TUINT64]
	boolT := types.Types[types.TBOOL]
	c := func(x uint64) *ssa.Value { return s.constInt64(u64, int64(x)) }
	r := s.newValue2(ssaop.OpDPDNarrow, u64, hi, lo)
	high := s.newValue2(ssaop.OpOr64, u64,
		s.newValue2(ssaop.OpAnd64, u64, hi, c(1<<46-1)),
		s.newValue2(ssaop.OpRsh64Ux64, u64, lo, c(50)))
	small := s.newValue2(ssaop.OpEq64, boolT, high, c(0))
	payload := s.newValue3(ssaop.OpCondSelect, u64, s.newValue2(ssaop.OpAnd64, u64, lo, c(1<<50-1)), c(0), small)
	nan := s.newValue2(ssaop.OpOr64, u64, s.newValue2(ssaop.OpAnd64, u64, hi, c(bid.SignMask64)), c(bid.NaN64))
	nan = s.newValue2(ssaop.OpOr64, u64, nan, payload)
	return s.newValue3(ssaop.OpCondSelect, u64, nan, r, s.dpdIsNaN(hi))
}

// DecimalConvCalls reports whether the conversion from the type from to the
// type to calls the runtime: whether it involves a decimal type and is not
// one that dpdConv lowers on a DPD target.
func DecimalConvCalls(from, to *types.Type) bool {
	if !from.IsDecimal() && !to.IsDecimal() {
		return false
	}
	if !ir.DecimalDPD() {
		return true
	}
	return from.IsFloat() || to.IsFloat() || to.IsInteger() && !to.IsSigned()
}
