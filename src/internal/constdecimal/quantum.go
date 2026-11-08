// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !compiler_bootstrap

package constdecimal

import (
	"go/constant"
	_ "unsafe"
)

// Quantum reads the constant metadata needed by the compiler in either mode.
//
//go:linkname Quantum go/constant.decimalQuantum
func Quantum(v constant.Value) (int, bool)
