// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math/big"
	"testing"
)

// TestQuantizeBoundaries exercises changes of quantum around coefficient
// and exponent limits, ties and carries, including the default invalid mode.
// Expected encodings come from the independent exact reference model.
func TestQuantizeBoundaries(t *testing.T) {
	for _, f := range both {
		t.Run(f.name, func(t *testing.T) {
			coeffs := []*big.Int{big.NewInt(0), big.NewInt(1)}
			for digits := 0; digits < f.prec; digits++ {
				for _, lead := range []int64{1, 5, 9} {
					for _, delta := range []int64{-1, 0, 1} {
						c := new(big.Int).Mul(big.NewInt(lead), pow10(digits))
						c.Add(c, big.NewInt(delta))
						if c.Sign() >= 0 && c.Cmp(pow10(f.prec)) < 0 {
							coeffs = append(coeffs, c)
						}
					}
				}
			}
			for _, c := range coeffs {
				for _, exp := range []int{f.minExp, f.minExp + 1, -f.prec, -1, 0, f.maxExp - 1, f.maxExp} {
					for _, neg := range []bool{false, true} {
						x := f.encode(value{kind: finite, neg: neg, coeff: c, exp: exp})
						for _, target := range []int{f.minExp, exp - f.prec - 1, exp - 1, exp, exp + 1, exp + f.prec, exp + f.prec + 1, f.maxExp} {
							if target < f.minExp || target > f.maxExp {
								continue
							}
							y := f.encode(value{kind: finite, coeff: big.NewInt(1), exp: target})
							for m := bid.Mode(0); m <= 5; m++ {
								got, want := (kernel{}).quantize(f, x, y, m), (reference{}).quantize(f, x, y, m)
								if got != want {
									t.Fatalf("quantize %x to %x mode %d: got %x, want %x", x, y, m, got, want)
								}
							}
						}
						for m := bid.Mode(0); m <= 5; m++ {
							got, want := (kernel{}).roundToIntegral(f, x, m), (reference{}).roundToIntegral(f, x, m)
							if got != want {
								t.Fatalf("integral %x mode %d: got %x, want %x", x, m, got, want)
							}
						}
					}
				}
			}
		})
	}
}

func BenchmarkQuantizeCases(b *testing.B) {
	for _, c := range []struct {
		name string
		x, y [3]string
	}{
		{"same", [3]string{"123.45", "123.45"}, [3]string{"0.01", "0.01"}},
		{"wordExact", [3]string{"123.4500", "123.4500"}, [3]string{"0.01", "0.01"}},
		{"wordTie", [3]string{"123.455", "123.455"}, [3]string{"0.01", "0.01"}},
		{"scale", [3]string{"123.45", "123.45"}, [3]string{"0.000001", "0.000001"}},
		{"wide", [3]string{"123456789012345.5", "123456789012345678901234567890123.5"}, [3]string{"1", "1"}},
		{"far", [3]string{"1e-398", "1e-6176"}, [3]string{"1e369", "1e6111"}},
		{"special", [3]string{"NaN", "NaN"}, [3]string{"0.01", "0.01"}},
	} {
		o := fixedOpnds(c.x, c.y, [3]string{}, 0, 0, 0)
		b.Run(c.name+"/64", func(b *testing.B) {
			loop2(b, o.x64, o.y64, func(x, y uint64) uint64 { return bid.Quantize64(x, y, rne) })
		})
		b.Run(c.name+"/128", func(b *testing.B) {
			loop2(b, o.x128, o.y128, func(x, y bits) bits { return bid.Quantize128(x, y, rne) })
		})
	}
}

// TestQuantizeWordScale drives the word fast path of quantizeSlowW at its
// bounds against the reference: word coefficients of x up to the largest
// word and one past it, scaled to the exponent of y across gaps on both
// sides of the word table (19, 20), of the format's precision, of the
// coefficient table (38, 39) and far beyond it, including the gaps at which
// the scaled coefficient first exceeds the precision (the NaN), both signs,
// a zero x at every gap, y a NaN and an infinity, and every mode.
func TestQuantizeWordScale(t *testing.T) {
	var k kernel
	for _, f := range []*format{dec64, dec128} {
		var xcs []string
		for _, c := range []string{"0", "1", "9", "123456", "999999999999999", "9999999999999999", "10000000000000000", "9999999999999999999", "10000000000000000000", "18446744073709551615", "18446744073709551616"} {
			n, _ := new(big.Int).SetString(c, 10)
			if n.Cmp(f.maxCoeff) <= 0 {
				xcs = append(xcs, c)
			}
		}
		gaps := []int{-1, 0, 1, 2, 14, 15, 16, 17, 18, 19, 20, 21, 24, 25, 33, 34, 35, 38, 39, 40, 100, f.maxExp - f.minExp}
		n := 0
		for _, xc := range xcs {
			for _, xe := range []int{f.minExp, -2, 0, f.maxExp} {
				for _, neg := range []bool{false, true} {
					x, ok := wordOperand(f, neg, xc, xe)
					if !ok {
						continue
					}
					ys := []bits{f.encode(nan), f.encode(value{kind: infinite})}
					for _, g := range gaps {
						if y, ok := wordOperand(f, false, "1", xe-g); ok {
							ys = append(ys, y)
						}
					}
					for _, y := range ys {
						for _, m := range allModes {
							n++
							if got, want := k.quantize(f, x, y, m), f.quantize(x, y, m); got != want {
								t.Fatalf("%s quantize(%s, %s, %s) = %s, want %s", f.name, f.format(x), f.format(y), modeName(m), f.format(got), f.format(want))
							}
						}
					}
				}
			}
		}
		t.Logf("%s: %d cases", f.name, n)
	}
}
