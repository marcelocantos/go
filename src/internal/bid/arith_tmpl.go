// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zarith64.go and zarith128.go, written once
// in the vocabulary of "Width parameterisation" in the package
// documentation: every identifier that depends on the width ends in a
// capital W, and the generator (gen_arith.go) replaces that W with 64 or
// 128. Nothing else is substituted. This file is not compiled; the build
// ignores it, and only the generated files are compiled. Everything from
// the build constraint to the package clause is commentary on the template
// and is dropped from the generated files, which nobody edits.
//
// Nothing below the package clause, in code or in a comment, may end an
// identifier with that capital letter unless it is meant to be replaced.
package bid

// mulReduceW multiplies two coefficients and reduces the exact product to
// at most PrecisionW+1 digits, the first step of a multiplication. The
// returned q is ⌊a × b / 10^k⌋, k is the number of digits dropped (the
// amount by which the exponent of the product rises), and sticky reports
// whether any dropped digit was non-zero. One digit beyond the precision is
// kept so that a single later rounding sees the digit that decides it and,
// in sticky, whether anything lies below. Nothing is rounded here.
//
// Precondition: a and b are at most MaxCoeffW (a decimal coefficient).
func mulReduceW(a, b coefW) (q coefW, k int, sticky bool) {
	p := cMulFullW(a, b)
	k = max(cLenFullW(p)-(PrecisionW+1), 0)
	q, sticky = cReduceW(p, k)
	return q, k, sticky
}
