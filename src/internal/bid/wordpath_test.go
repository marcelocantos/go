// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"math/big"
	"testing"
)

// The word fast paths of addition, multiplication and quantize at their
// bounds. Each fast path states
// a bound in its comment; the operands here sit on both sides of it, in
// both widths and in every mode, and the reference model decides every
// result bit, quantum and sign of zero included.

// wordCoeffs are the coefficients that sit at the bounds of the word fast
// paths: zero and one, the limits of the decimal64 first form (2^53) and
// coefficient (MaxCoeff64), the powers of ten around them, the largest word,
// and a coefficient one past a word, which the word paths must refuse.
var wordCoeffs = []string{
	"0", "1", "9", "10", "99999", "9007199254740991", "9007199254740992",
	"999999999999999", "1000000000000000", "9999999999999999", "10000000000000000",
	"9999999999999999999", "10000000000000000000", "18446744073709551615", "18446744073709551616",
}

// wordOperand returns the encoding of c × 10^exp, and ok false when it is not
// exactly representable.
func wordOperand(f *format, neg bool, c string, exp int) (bits, bool) {
	n, _ := new(big.Int).SetString(c, 10)
	if n.Cmp(f.maxCoeff) > 0 || exp < f.minExp || exp > f.maxExp {
		return bits{}, false
	}
	return f.encode(value{kind: finite, neg: neg, coeff: n, exp: exp}), true
}

// TestWordFastPaths drives add, sub, mul and quantize over word coefficients
// at gaps 0 to maxWordGap+1 (19 is the largest power in pow10tab64), around
// the decimal128 bound of the compiler's add (14) and at exponents at both
// ends of each format, where the exponent of a product or of the scaled
// operand leaves the format.
func TestWordFastPaths(t *testing.T) {
	var k kernel
	for _, f := range []*format{dec64, dec128} {
		bases := []int{f.minExp, f.minExp + 1, -2, 0, f.maxExp - 25, f.maxExp - 20, f.maxExp}
		gaps := []int{0, 1, 2, 13, 14, 15, 16, 17, 18, 19, 20, 21}
		var xs []bits
		for _, c := range wordCoeffs {
			for _, e := range bases {
				for _, neg := range []bool{false, true} {
					if x, ok := wordOperand(f, neg, c, e); ok {
						xs = append(xs, x)
					}
				}
			}
		}
		n := 0
		for _, x := range xs {
			_, _, _, xe := k.unpack(f, x)
			for _, g := range gaps {
				for _, c := range wordCoeffs {
					for _, neg := range []bool{false, true} {
						for _, ye := range []int{xe - g, xe + g} {
							y, ok := wordOperand(f, neg, c, ye)
							if !ok {
								continue
							}
							for _, m := range allModes {
								n++
								if got, want := k.add(f, x, y, m), f.add(x, y, m); got != want {
									t.Fatalf("%s add(%s, %s, %s) = %s, want %s", f.name, f.format(x), f.format(y), modeName(m), f.format(got), f.format(want))
								}
								if got, want := k.sub(f, x, y, m), f.sub(x, y, m); got != want {
									t.Fatalf("%s sub(%s, %s, %s) = %s, want %s", f.name, f.format(x), f.format(y), modeName(m), f.format(got), f.format(want))
								}
								if got, want := k.mul(f, x, y, m), f.mul(x, y, m); got != want {
									t.Fatalf("%s mul(%s, %s, %s) = %s, want %s", f.name, f.format(x), f.format(y), modeName(m), f.format(got), f.format(want))
								}
								if got, want := k.quantize(f, x, y, m), f.quantize(x, y, m); got != want {
									t.Fatalf("%s quantize(%s, %s, %s) = %s, want %s", f.name, f.format(x), f.format(y), modeName(m), f.format(got), f.format(want))
								}
							}
						}
					}
				}
			}
		}
		t.Logf("%s: %d operand pairs × 4 operations", f.name, n)
	}
}
