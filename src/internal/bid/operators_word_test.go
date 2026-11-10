// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package bid_test

import (
	"internal/bid"
	"testing"
)

// TestWordPathsAgreeWithCompiler runs the kernel's word fast paths and the
// compiler's inline lowering of + - * and the comparisons (ssagen's
// bidAddSub128, bidMul128 and bidCompare) on the operands at their bounds:
// word coefficients at gaps around both the compiler's add bound (14) and
// the kernel's (19), at both ends of the exponent range. The two were
// written to the same proofs; this checks that they agree bit for bit,
// whichever path each takes. The compiled decimal64 operators call the
// kernel, so they are checked too, against it.
func TestWordPathsAgreeWithCompiler(t *testing.T) {
	for _, f := range []*format{dec64, dec128} {
		var xs []bits
		for _, c := range wordCoeffs {
			for _, e := range []int{f.minExp, -2, 0, f.maxExp - 20, f.maxExp} {
				for _, neg := range []bool{false, true} {
					if x, ok := wordOperand(f, neg, c, e); ok {
						xs = append(xs, x)
					}
				}
			}
		}
		n := 0
		for _, x := range xs {
			_, _, _, xe := kernel{}.unpack(f, x)
			for _, g := range []int{0, 1, 13, 14, 15, 18, 19, 20} {
				for _, c := range wordCoeffs {
					for _, neg := range []bool{false, true} {
						for _, ye := range []int{xe - g, xe + g} {
							y, ok := wordOperand(f, neg, c, ye)
							if !ok {
								continue
							}
							n++
							if f == dec128 {
								checkCompiled128(t, x, y)
							} else {
								checkCompiled64(t, x, y)
							}
						}
					}
				}
			}
		}
		t.Logf("%s: %d operand pairs", f.name, n)
	}
}

func checkCompiled128(t *testing.T, x, y bits) {
	t.Helper()
	for _, c := range []struct {
		op        string
		got, want bits
	}{
		{"+", b128(opAdd128(d128(x), d128(y))), bid.Add128(x, y, rne)},
		{"-", b128(opSub128(d128(x), d128(y))), bid.Sub128(x, y, rne)},
		{"*", b128(opMul128(d128(x), d128(y))), bid.Mul128(x, y, rne)},
	} {
		if c.got != c.want {
			t.Fatalf("decimal128 %s %s %s: compiled %s, kernel %s", dec128.format(x), c.op, dec128.format(y), dec128.format(c.got), dec128.format(c.want))
		}
	}
	cmp, ok := bid.Compare128(x, y)
	if got, want := opCmp128(d128(x), d128(y)), cmpBools(cmp, ok); got != want {
		t.Fatalf("decimal128 compare(%s, %s): compiled %v, kernel %v", dec128.format(x), dec128.format(y), got, want)
	}
}

func checkCompiled64(t *testing.T, x, y bits) {
	t.Helper()
	for _, c := range []struct {
		op        string
		got, want bits
	}{
		{"+", b64(opAdd64(d64(x), d64(y))), bits{Lo: bid.Add64(x.Lo, y.Lo, rne)}},
		{"-", b64(opSub64(d64(x), d64(y))), bits{Lo: bid.Sub64(x.Lo, y.Lo, rne)}},
		{"*", b64(opMul64(d64(x), d64(y))), bits{Lo: bid.Mul64(x.Lo, y.Lo, rne)}},
	} {
		if c.got != c.want {
			t.Fatalf("decimal64 %s %s %s: compiled %s, kernel %s", dec64.format(x), c.op, dec64.format(y), dec64.format(c.got), dec64.format(c.want))
		}
	}
	cmp, ok := bid.Compare64(x.Lo, y.Lo)
	if got, want := opCmp64(d64(x), d64(y)), cmpBools(cmp, ok); got != want {
		t.Fatalf("decimal64 compare(%s, %s): compiled %v, kernel %v", dec64.format(x), dec64.format(y), got, want)
	}
}

// cmpBools is what == != < <= > >= give for an ordering c, or for unordered
// operands when ok is false.
func cmpBools(c int, ok bool) [6]bool {
	if !ok {
		return [6]bool{false, true, false, false, false, false}
	}
	return [6]bool{c == 0, c != 0, c < 0, c <= 0, c > 0, c >= 0}
}
