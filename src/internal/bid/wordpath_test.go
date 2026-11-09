// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math/big"
	"strings"
	"testing"
)

// The word fast paths of addition, multiplication and quantize, and the
// swamped-operand path of addition, at their bounds. Each fast path states
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

// TestUnpackSplit checks Unpack64 and Unpack128 against the reference at the
// bound that divides their inlined first-form half from the call to the
// rest: for decimal128 a high coefficient word just below, at and above
// that of MaxCoeff128, with low words around MaxCoeff128's; for decimal64
// the two forms around 2^53 and MaxCoeff64. Every sign, a spread of
// exponent fields and both special values are included.
func TestUnpackSplit(t *testing.T) {
	var k kernel
	check := func(f *format, x bits) {
		t.Helper()
		kk, kneg, kc, ke := k.unpack(f, x)
		rk, rneg, rc, re := f.unpack(x)
		want := fromUint128(kc)
		if kk != rk || kneg != rneg || want.Cmp(rc) != 0 || ke != re {
			t.Fatalf("%s Unpack(%#x:%#x) = %v %v %v %d, reference %v %v %v %d", f.name, x.Hi, x.Lo, kk, kneg, want, ke, rk, rneg, rc, re)
		}
	}
	for _, top := range []uint64{0, 1, 0x2000, 0x2fff, 0x3000, 0x3fff, 0x7800, 0x7c00, 0x7e00} {
		for _, sign := range []uint64{0, 1 << 63} {
			for _, hi := range []uint64{0, bid.MaxCoeff128Hi - 1, bid.MaxCoeff128Hi, bid.MaxCoeff128Hi + 1, bid.CoeffMask128Hi} {
				for _, lo := range []uint64{0, 1, bid.MaxCoeff128Lo - 1, bid.MaxCoeff128Lo, bid.MaxCoeff128Lo + 1, ^uint64(0)} {
					check(dec128, bits{Hi: sign | top<<49 | hi, Lo: lo})
				}
			}
			for _, c := range []uint64{0, 1, 1<<53 - 1, 1 << 53, bid.MaxCoeff64, bid.MaxCoeff64 + 1, 1<<53 - 1 | 1<<51} {
				x := sign | top<<53 | c&(1<<53-1)
				check(dec64, bits{Lo: x})
				check(dec64, bits{Lo: x | 3<<61})
			}
		}
	}
}

// TestSwampedAdd checks the swamped-operand path of addition at its bound:
// with x scaled by m places to exactly Precision digits, y is swamped when
// the gap exceeds m by more than the digits of y. The gaps here put that
// excess at the digits of y minus one, at them, one more and far beyond, for
// coefficients of x that are powers of ten (whose difference takes one more
// place, or the general path) and that are not, in both widths, both signs
// of y and every mode.
func TestSwampedAdd(t *testing.T) {
	var k kernel
	for _, f := range []*format{dec64, dec128} {
		p := f.prec
		pow := func(n int) string { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil).String() }
		nines := func(n int) string { return strings.Repeat("9", n) }
		xcs := []string{"1", "5", "123456789", pow(p - 1), pow(p - 1)[:p-1] + "1", nines(p), nines(p - 1)}
		ycs := []string{"1", "5", "49", "50", "51", "123456789", pow(p - 1), nines(p)}
		n := 0
		for _, xc := range xcs {
			dx := len(xc)
			m := p - dx
			for _, yc := range ycs {
				dy := len(yc)
				for _, excess := range []int{dy - 1, dy, dy + 1, dy + 2, dy + 40} {
					gap := m + excess
					for _, xe := range []int{0, f.maxExp, f.minExp + gap} {
						x, ok := wordOperand(f, false, xc, xe)
						if !ok {
							continue
						}
						for _, neg := range []bool{false, true} {
							y, ok := wordOperand(f, neg, yc, xe-gap)
							if !ok {
								continue
							}
							for _, m := range allModes {
								n++
								for _, pair := range [][2]bits{{x, y}, {y, x}, {k.neg(f, x), k.neg(f, y)}} {
									if got, want := k.add(f, pair[0], pair[1], m), f.add(pair[0], pair[1], m); got != want {
										t.Fatalf("%s add(%s, %s, %s) = %s, want %s", f.name, f.format(pair[0]), f.format(pair[1]), modeName(m), f.format(got), f.format(want))
									}
								}
							}
						}
					}
				}
			}
		}
		t.Logf("%s: %d swamped cases", f.name, n)
	}
}

// TestWordDecodeBounds checks the word decoders at the bounds of what they
// accept, through every operation that branches on them (add, sub, mul,
// quantize and, for unpackWord, the comparisons). For unpackWordCanon, a
// decimal64 word operand is any finite
// canonical encoding, the second form included, so the second form is taken
// at its smallest coefficient, at MaxCoeff64 and one past it (non-canonical:
// a zero), at its largest field, and as the special values the second-form
// bits also mark. A decimal128 word operand is a first form whose high
// coefficient word is zero; one bit above it, the largest canonical
// coefficient, a non-canonical first form and the specials are refused.
// Each is paired, both ways round, with word operands at gaps of 0, 1, 19
// and 20 and with itself, in every mode, against the reference.
func TestWordDecodeBounds(t *testing.T) {
	var k kernel
	const form2 = 3 << 61
	exp64 := func(e int) uint64 { return uint64(e+398) << 51 } // second-form exponent field
	var raw64 []uint64
	for _, c := range []uint64{1 << 53, bid.MaxCoeff64 - 1, bid.MaxCoeff64, bid.MaxCoeff64 + 1, 1<<54 - 1} {
		for _, e := range []int{-398, -20, 0, 369} {
			for _, sign := range []uint64{0, 1 << 63} {
				raw64 = append(raw64, sign|form2|exp64(e)|c&(1<<51-1))
			}
		}
	}
	raw64 = append(raw64, 0x78<<56, 0xf8<<56, 0x7c<<56, 0x7e<<56|5, 0xfc<<56|1)
	var raw128 []bits
	exp128 := func(e int) uint64 { return uint64(e+6176) << 49 }
	for _, c := range []bits{{Hi: 1}, {Hi: 1, Lo: 7}, {Hi: bid.MaxCoeff128Hi, Lo: bid.MaxCoeff128Lo}, {Hi: bid.MaxCoeff128Hi, Lo: bid.MaxCoeff128Lo + 1}, {Hi: bid.CoeffMask128Hi, Lo: ^uint64(0)}} {
		for _, e := range []int{-6176, -20, 0, 6111} {
			for _, sign := range []uint64{0, 1 << 63} {
				raw128 = append(raw128, bits{Hi: sign | exp128(e) | c.Hi, Lo: c.Lo})
			}
		}
	}
	raw128 = append(raw128, bits{Hi: form2 | exp128(0)>>2}, bits{Hi: 0x78 << 56}, bits{Hi: 0xf8 << 56}, bits{Hi: 0x7c << 56}, bits{Hi: 0x7e<<56 | 3, Lo: 9})
	for _, f := range []*format{dec64, dec128} {
		var edge []bits
		if f == dec64 {
			for _, x := range raw64 {
				edge = append(edge, bits{Lo: x})
			}
		} else {
			edge = raw128
		}
		n := 0
		for _, x := range edge {
			ys := []bits{x}
			_, _, _, xe := f.unpack(x)
			for _, g := range []int{0, 1, 19, 20} {
				for _, c := range []string{"0", "1", "123456", "9999999999999999"} {
					for _, ye := range []int{xe - g, xe + g} {
						if y, ok := wordOperand(f, g%2 == 1, c, ye); ok {
							ys = append(ys, y)
						}
					}
				}
			}
			for _, y := range ys {
				for _, p := range [][2]bits{{x, y}, {y, x}} {
					a, b := p[0], p[1]
					for _, m := range allModes {
						n++
						if got, want := k.add(f, a, b, m), f.add(a, b, m); got != want {
							t.Fatalf("%s add(%#x:%#x, %#x:%#x, %s) = %s, want %s", f.name, a.Hi, a.Lo, b.Hi, b.Lo, modeName(m), f.format(got), f.format(want))
						}
						if got, want := k.sub(f, a, b, m), f.sub(a, b, m); got != want {
							t.Fatalf("%s sub(%#x:%#x, %#x:%#x, %s) = %s, want %s", f.name, a.Hi, a.Lo, b.Hi, b.Lo, modeName(m), f.format(got), f.format(want))
						}
						if got, want := k.mul(f, a, b, m), f.mul(a, b, m); got != want {
							t.Fatalf("%s mul(%#x:%#x, %#x:%#x, %s) = %s, want %s", f.name, a.Hi, a.Lo, b.Hi, b.Lo, modeName(m), f.format(got), f.format(want))
						}
						if got, want := k.quantize(f, a, b, m), f.quantize(a, b, m); got != want {
							t.Fatalf("%s quantize(%#x:%#x, %#x:%#x, %s) = %s, want %s", f.name, a.Hi, a.Lo, b.Hi, b.Lo, modeName(m), f.format(got), f.format(want))
						}
					}
					gotC, gotOK := k.compare(f, a, b)
					wantC, wantOK := f.compare(a, b)
					if gotC != wantC || gotOK != wantOK {
						t.Fatalf("%s compare(%#x:%#x, %#x:%#x) = %d %v, want %d %v", f.name, a.Hi, a.Lo, b.Hi, b.Lo, gotC, gotOK, wantC, wantOK)
					}
					if got, want := k.less(f, a, b), (reference{}).less(f, a, b); got != want {
						t.Fatalf("%s less(%#x:%#x, %#x:%#x) = %v, want %v", f.name, a.Hi, a.Lo, b.Hi, b.Lo, got, want)
					}
				}
			}
		}
		t.Logf("%s: %d operand pairs × 4 operations", f.name, n)
	}
}

// TestAddAlignBound checks the general path of addition where the scale of
// the operand with the larger exponent reaches addDigits, the most digits
// the coefficient type holds for every value (19 in 64 bits, 38 in 128):
// with dx digits and a gap of addDigits - dx or less the sum is exact and
// rounded once, and one place more shifts the other operand by a digit
// with its class. The gaps put dx + gap at addDigits - 1, addDigits and one
// and two beyond it, for coefficients of x of Precision digits and fewer,
// and coefficients of y whose low digits make the dropped part of the sum
// exactly a half, just below and just above it, with both signs and every
// mode, against the reference.
func TestAddAlignBound(t *testing.T) {
	var k kernel
	for _, f := range []*format{dec64, dec128} {
		p := f.prec
		addDigits := 19
		if f == dec128 {
			addDigits = 38
		}
		nines := func(n int) string { return strings.Repeat("9", n) }
		pow := func(n int) string { return "1" + strings.Repeat("0", n) }
		xcs := []string{nines(p), pow(p - 1), "1" + strings.Repeat("2", p-1), "7" + strings.Repeat("0", p-3) + "1", "12345"}
		ycs := []string{"1", "5", "50", "4999", "5000", "5001", "15000", "25000", "49999", "50000", "50001", nines(p), pow(p - 1), "5" + strings.Repeat("0", p-1)}
		n := 0
		for _, xc := range xcs {
			dx := len(xc)
			for _, over := range []int{-1, 0, 1, 2} {
				gap := addDigits - dx + over
				for _, yc := range ycs {
					for _, ye := range []int{-gap, f.minExp} {
						x, ok := wordOperand(f, false, xc, ye+gap)
						if !ok {
							continue
						}
						for _, neg := range []bool{false, true} {
							y, ok := wordOperand(f, neg, yc, ye)
							if !ok {
								continue
							}
							for _, m := range allModes {
								n++
								for _, pair := range [][2]bits{{x, y}, {y, x}, {k.neg(f, x), k.neg(f, y)}} {
									if got, want := k.add(f, pair[0], pair[1], m), f.add(pair[0], pair[1], m); got != want {
										t.Fatalf("%s add(%s, %s, %s) = %s, want %s", f.name, f.format(pair[0]), f.format(pair[1]), modeName(m), f.format(got), f.format(want))
									}
								}
							}
						}
					}
				}
			}
		}
		t.Logf("%s: %d cases", f.name, n)
	}
}
