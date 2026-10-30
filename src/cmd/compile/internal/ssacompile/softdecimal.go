// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ssacompile

import (
	"cmd/compile/internal/ssa"
	"cmd/compile/internal/ssa/rewrite/dec64"
	"cmd/compile/internal/ssa/ssaop"
	"cmd/compile/internal/types"
)

// softdecimal removes the decimal64 and decimal128 types from the values that
// remain after ssagen, which has already lowered every decimal operator and
// conversion to a runtime call. Unlike softfloat, this pass always runs since
// no target has decimal floating-point instructions.
//
// For decimal64 (8 bytes), values are retyped to uint64.
// For decimal128 (16 bytes), decomposition into two uint64 halves is handled
// by expand_calls (Decimal128Make/Lo/Hi ops) and the decomposeBuiltin pass
// (dec.rules). This pass only handles Neg128D for decimal128.
func softdecimal(f *ssa.Func) {
	for _, b := range f.Blocks {
		for _, v := range b.Values {
			if v.Type.IsDecimal() {
				if v.Type.Size() == 8 {
					// decimal64: retype to uint64
					f.UnCache(v)
					switch v.Op {
					case ssaop.OpPhi, ssaop.OpLoad, ssaop.OpArg:
						v.Type = f.Config.Types.UInt64
					case ssaop.OpConst64D:
						v.Op = ssaop.OpConst64
						v.Type = f.Config.Types.UInt64
					case ssaop.OpNeg64D:
						arg0 := v.Args[0]
						v.Reset(ssaop.OpXor64)
						v.Type = f.Config.Types.UInt64
						v.AddArg(arg0)
						mask := v.Block.NewValue0(v.Pos, ssaop.OpConst64, v.Type)
						mask.AuxInt = -0x8000000000000000
						v.AddArg(mask)
					}
				} else {
					// decimal128: only Neg128D needs handling here.
					// Decomposition (Phi/Load/Arg/Store) is handled by
					// expand_calls + decomposeBuiltin (dec.rules).
					switch v.Op {
					case ssaop.OpNeg128D:
						f.UnCache(v)
						arg0 := v.Args[0]
						lo := v.Block.NewValue1(v.Pos, ssaop.OpDecimal128Lo, f.Config.Types.UInt64, arg0)
						hi := v.Block.NewValue1(v.Pos, ssaop.OpDecimal128Hi, f.Config.Types.UInt64, arg0)
						mask := v.Block.NewValue0(v.Pos, ssaop.OpConst64, f.Config.Types.UInt64)
						mask.AuxInt = -0x8000000000000000
						newHi := v.Block.NewValue2(v.Pos, ssaop.OpXor64, f.Config.Types.UInt64, hi, mask)
						v.Reset(ssaop.OpDecimal128Make)
						v.AddArg(lo)
						v.AddArg(newHi)
					}
				}
			} else if (v.Op == ssaop.OpStore || v.Op == ssaop.OpZero || v.Op == ssaop.OpMove) && v.Aux.(*types.Type).IsDecimal() && v.Aux.(*types.Type).Size() == 8 {
				// decimal64 Store/Zero/Move: retype Aux to uint64
				v.Aux = f.Config.Types.UInt64
			}
		}
	}

	if f.Config.RegSize == 4 {
		// On 32-bit architectures, decompose the uint64 values introduced
		// above, as softfloat does. This has to run now: the dec64 rule for
		// Arg does not fire during the decompose builtin pass.
		decomposeBuiltin(f)
		applyRewrite(f, dec64.RewriteBlock, dec64.RewriteValue, ssa.RemoveDeadValues)
	}
}
