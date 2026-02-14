// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ssa

import "cmd/compile/internal/types"

// softdecimal converts decimal64/decimal128 SSA operations to runtime calls.
// Unlike softfloat, this pass always runs since no consumer hardware has
// decimal floating-point instructions.
//
// For decimal64 (8 bytes), values are retyped to uint64.
// For decimal128 (16 bytes), decomposition into two uint64 halves is handled
// by expand_calls (Decimal128Make/Lo/Hi ops) and the decomposeBuiltin pass
// (dec.rules). This pass only handles Neg128D for decimal128.
func softdecimal(f *Func) {
	for _, b := range f.Blocks {
		for _, v := range b.Values {
			if v.Type.IsDecimal() {
				if v.Type.Size() == 8 {
					// decimal64: retype to uint64
					f.unCache(v)
					switch v.Op {
					case OpPhi, OpLoad, OpArg:
						v.Type = f.Config.Types.UInt64
					case OpConst64D:
						v.Op = OpConst64
						v.Type = f.Config.Types.UInt64
					case OpNeg64D:
						arg0 := v.Args[0]
						v.reset(OpXor64)
						v.Type = f.Config.Types.UInt64
						v.AddArg(arg0)
						mask := v.Block.NewValue0(v.Pos, OpConst64, v.Type)
						mask.AuxInt = -0x8000000000000000
						v.AddArg(mask)
					}
				} else {
					// decimal128: only Neg128D needs handling here.
					// Decomposition (Phi/Load/Arg/Store) is handled by
					// expand_calls + decomposeBuiltin (dec.rules).
					switch v.Op {
					case OpNeg128D:
						f.unCache(v)
						arg0 := v.Args[0]
						lo := v.Block.NewValue1(v.Pos, OpDecimal128Lo, f.Config.Types.UInt64, arg0)
						hi := v.Block.NewValue1(v.Pos, OpDecimal128Hi, f.Config.Types.UInt64, arg0)
						mask := v.Block.NewValue0(v.Pos, OpConst64, f.Config.Types.UInt64)
						mask.AuxInt = -0x8000000000000000
						newHi := v.Block.NewValue2(v.Pos, OpXor64, f.Config.Types.UInt64, hi, mask)
						v.reset(OpDecimal128Make)
						v.AddArg(lo)
						v.AddArg(newHi)
					}
				}
			} else if (v.Op == OpStore || v.Op == OpZero || v.Op == OpMove) && v.Aux.(*types.Type).IsDecimal() && v.Aux.(*types.Type).Size() == 8 {
				// decimal64 Store/Zero/Move: retype Aux to uint64
				v.Aux = f.Config.Types.UInt64
			}
		}
	}
}
