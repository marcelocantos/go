// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ssa

import "cmd/compile/internal/types"

// softdecimal converts decimal64 SSA operations to runtime function calls.
// Unlike softfloat, this pass always runs since no consumer hardware has
// decimal floating-point instructions.
func softdecimal(f *Func) {
	for _, b := range f.Blocks {
		for _, v := range b.Values {
			if v.Type.IsDecimal() {
				f.unCache(v)
				switch v.Op {
				case OpPhi, OpLoad, OpArg:
					// Treat as uint64 (BID64 is just bit patterns)
					v.Type = f.Config.Types.UInt64
				case OpConst64D:
					// BID64 constant bits stored as int64; reinterpret as uint64
					v.Op = OpConst64
					v.Type = f.Config.Types.UInt64
				case OpNeg64D:
					// Flip sign bit (bit 63)
					arg0 := v.Args[0]
					v.reset(OpXor64)
					v.Type = f.Config.Types.UInt64
					v.AddArg(arg0)
					mask := v.Block.NewValue0(v.Pos, OpConst64, v.Type)
					mask.AuxInt = -0x8000000000000000 // sign bit mask
					v.AddArg(mask)
				}
			} else if (v.Op == OpStore || v.Op == OpZero || v.Op == OpMove) && v.Aux.(*types.Type).IsDecimal() {
				v.Aux = f.Config.Types.UInt64
			}
		}
	}
}
