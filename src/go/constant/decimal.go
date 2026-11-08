// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal || compiler_bootstrap

package constant

// Quantum returns x's decimal exponent and whether that exponent is known.
// Decimal literals retain their quantum through exact constant evaluation.
func Quantum(x Value) (exp int, ok bool) { return decimalQuantum(x) }
