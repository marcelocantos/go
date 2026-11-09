// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"fmt"
	"internal/bid"
	"testing"
)

// The inline fast paths (see "Inline fast paths" in the package
// documentation) must give the bits of the general path behind them for every
// operand they decide, and decide exactly the operands their bounds say.
// TestInlinePaths checks the first over every pair of a pool of encodings
// placed on both sides of each bound, every mode, both through the helper and
// through the exported function; TestInlineBounds checks the second, operand
// pair by operand pair, at each bound.

// enc64 and enc128 are first-form encodings with a biased exponent.
func enc64(neg bool, c uint64, biased uint64) uint64 {
	x := biased<<bid.ExpShift64 | c
	if neg {
		x |= bid.SignMask64
	}
	return x
}

func enc128(neg bool, hi, lo uint64, biased uint64) bid.Uint128 {
	x := bid.Uint128{Hi: biased<<bid.ExpShift128 | hi, Lo: lo}
	if neg {
		x.Hi |= bid.SignMask128Hi
	}
	return x
}

var inlineModes = []bid.Mode{bid.ToNearestEven, bid.ToNearestAway, bid.TowardZero, bid.TowardPositive, bid.TowardNegative}

func inlinePool64() []uint64 {
	coeffs := []uint64{0, 1, 2, 9, 1 << 26, 1<<27 - 1, 1 << 27, 1 << 52, 1<<53 - 2, 1<<53 - 1}
	// Biased exponents: the ends of the range, the bias, the inline bound
	// 2^9 and the end of the first form's exponents for the largest
	// coefficients (768 = 3<<8 puts bits 62 and 61 in the second form).
	exps := []uint64{0, 1, 199, 397, 398, 399, 510, 511, 512, 513, 767}
	var pool []uint64
	for _, neg := range []bool{false, true} {
		for _, e := range exps {
			for _, c := range coeffs {
				pool = append(pool, enc64(neg, c, e))
			}
		}
	}
	return append(pool,
		bid.Form2Mask64|398<<bid.Form2ExpShift64|1,                // 2^53 + 1, the second form
		bid.Form2Mask64|398<<bid.Form2ExpShift64|(1<<51-1),        // non-canonical
		bid.SignMask64|bid.Form2Mask64|1<<bid.Form2ExpShift64|123, // negative second form
		bid.Inf64, bid.Inf64|bid.SignMask64, bid.NaN64, bid.NaN64|bid.SignMask64|7, bid.SNaN64|42)
}

func inlinePool128() []bid.Uint128 {
	type c struct{ hi, lo uint64 }
	coeffs := []c{{0, 0}, {0, 1}, {0, 2}, {0, 9}, {0, 1<<32 - 1}, {0, 1 << 32}, {0, 1<<32 + 1},
		{0, 1<<48 - 1}, {0, 1 << 48}, {0, 1 << 63}, {0, 1<<64 - 2}, {0, 1<<64 - 1}, {1, 0}, {1, 1<<64 - 1}}
	// Biased exponents: the ends of the range, the bias, the inline bound
	// 2^13 and the end of the first form's exponents (12288 = 3<<12).
	exps := []uint64{0, 1, 3088, 6175, 6176, 6177, 8190, 8191, 8192, 8193, 12287}
	var pool []bid.Uint128
	for _, neg := range []bool{false, true} {
		for _, e := range exps {
			for _, c := range coeffs {
				pool = append(pool, enc128(neg, c.hi, c.lo, e))
			}
		}
	}
	return append(pool,
		bid.Uint128{Hi: bid.Form2Mask128Hi | 6176<<bid.Form2ExpShift128, Lo: 5}, // second form, non-canonical
		bid.Uint128{Hi: bid.MaxCoeff128Hi + 1 | 6176<<bid.ExpShift128},          // first form, non-canonical
		bid.Uint128{Hi: bid.Inf128Hi}, bid.Uint128{Hi: bid.Inf128Hi | bid.SignMask128Hi},
		bid.Uint128{Hi: bid.NaN128Hi}, bid.Uint128{Hi: bid.NaN128Hi | bid.SignMask128Hi, Lo: 7}, bid.Uint128{Hi: bid.SNaN128Hi, Lo: 42})
}

func TestInlinePaths(t *testing.T) {
	t.Run("64", func(t *testing.T) {
		pool := inlinePool64()
		decided := map[string]int{}
		check := func(op string, x, y, got, want uint64, m bid.Mode) {
			t.Helper()
			if got != want {
				t.Fatalf("%s(%#016x, %#016x, %v) = %#016x, general path %#016x", op, x, y, m, got, want)
			}
		}
		for _, x := range pool {
			for _, y := range pool {
				r1, ok1 := bid.AddSameQuantum64(x, y)
				r2, ok2 := bid.SubSameQuantum64(x, y)
				r3, ok3 := bid.MulShort64(x, y)
				same := bid.QuantizeSame64(x, y)
				for _, m := range inlineModes {
					add, sub, mul, quant := bid.AddSlow64(x, y, m), bid.SubSlow64(x, y, m), bid.MulSlow64(x, y, m), bid.QuantizeSlow64(x, y, m)
					check("Add64", x, y, bid.Add64(x, y, m), add, m)
					check("Sub64", x, y, bid.Sub64(x, y, m), sub, m)
					check("Mul64", x, y, bid.Mul64(x, y, m), mul, m)
					check("Quantize64", x, y, bid.Quantize64(x, y, m), quant, m)
					if ok1 {
						check("addSameQuantum64", x, y, r1, add, m)
					}
					if ok2 {
						check("subSameQuantum64", x, y, r2, sub, m)
					}
					if ok3 {
						check("mulShort64", x, y, r3, mul, m)
					}
					if same {
						check("quantizeSame64", x, y, x, quant, m)
					}
				}
				c, ok := bid.QuietCmpSlow64(x, y)
				if gc, gok := bid.Compare64(x, y); gc != c || gok != ok {
					t.Fatalf("Compare64(%#016x, %#016x) = %d, %v, general path %d, %v", x, y, gc, gok, c, ok)
				}
				if bid.Equal64(x, y) != (ok && c == 0) || bid.Less64(x, y) != (ok && c < 0) || bid.LessEqual64(x, y) != (ok && c <= 0) {
					t.Fatalf("Equal64, Less64 or LessEqual64(%#016x, %#016x) disagrees with the general path's %d, %v", x, y, c, ok)
				}
				kx, ky, kok := bid.SameQuantumKeys64(x, y)
				if kok {
					kc := 0
					if kx < ky {
						kc = -1
					} else if kx > ky {
						kc = 1
					}
					if !ok || kc != c {
						t.Fatalf("sameQuantumKeys64(%#016x, %#016x) orders %d, general path %d, %v", x, y, kc, c, ok)
					}
				}
				for name, d := range map[string]bool{"add": ok1, "sub": ok2, "mul": ok3, "quantize": same, "keys": kok} {
					if d {
						decided[name]++
					}
				}
			}
		}
		t.Logf("%d pairs; decided inline: %v", len(pool)*len(pool), decided)
	})
	t.Run("128", func(t *testing.T) {
		pool := inlinePool128()
		decided := map[string]int{}
		check := func(op string, x, y, got, want bid.Uint128, m bid.Mode) {
			t.Helper()
			if got != want {
				t.Fatalf("%s(%#x, %#x, %v) = %#x, general path %#x", op, x, y, m, got, want)
			}
		}
		for _, x := range pool {
			for _, y := range pool {
				r1, ok1 := bid.AddSameQuantum128(x, y)
				r2, ok2 := bid.SubSameQuantum128(x, y)
				r3, ok3 := bid.MulShort128(x, y)
				same := bid.QuantizeSame128(x, y)
				for _, m := range inlineModes {
					add, sub, mul, quant := bid.AddSlow128(x, y, m), bid.SubSlow128(x, y, m), bid.MulSlow128(x, y, m), bid.QuantizeSlow128(x, y, m)
					check("Add128", x, y, bid.Add128(x, y, m), add, m)
					check("Sub128", x, y, bid.Sub128(x, y, m), sub, m)
					check("Mul128", x, y, bid.Mul128(x, y, m), mul, m)
					check("Quantize128", x, y, bid.Quantize128(x, y, m), quant, m)
					if ok1 {
						check("addSameQuantum128", x, y, r1, add, m)
					}
					if ok2 {
						check("subSameQuantum128", x, y, r2, sub, m)
					}
					if ok3 {
						check("mulShort128", x, y, r3, mul, m)
					}
					if same {
						check("quantizeSame128", x, y, x, quant, m)
					}
				}
				c, ok := bid.QuietCmpSlow128(x, y)
				if gc, gok := bid.Compare128(x, y); gc != c || gok != ok {
					t.Fatalf("Compare128(%#x, %#x) = %d, %v, general path %d, %v", x, y, gc, gok, c, ok)
				}
				if bid.Equal128(x, y) != (ok && c == 0) || bid.Less128(x, y) != (ok && c < 0) || bid.LessEqual128(x, y) != (ok && c <= 0) {
					t.Fatalf("Equal128, Less128 or LessEqual128(%#x, %#x) disagrees with the general path's %d, %v", x, y, c, ok)
				}
				kx, ky, kok := bid.SameQuantumKeys128(x, y)
				if kok {
					kc := 0
					if kx < ky {
						kc = -1
					} else if kx > ky {
						kc = 1
					}
					if !ok || kc != c {
						t.Fatalf("sameQuantumKeys128(%#x, %#x) orders %d, general path %d, %v", x, y, kc, c, ok)
					}
				}
				for name, d := range map[string]bool{"add": ok1, "sub": ok2, "mul": ok3, "quantize": same, "keys": kok} {
					if d {
						decided[name]++
					}
				}
			}
		}
		t.Logf("%d pairs; decided inline: %v", len(pool)*len(pool), decided)
	})
}

// TestInlineBounds checks that each inline path decides the operands on the
// inside of each of its bounds and refuses those just outside it.
func TestInlineBounds(t *testing.T) {
	const b64, b128 = bid.Bias64, bid.Bias128
	type case64 struct {
		name string
		x, y uint64
		want bool
	}
	ok64 := func(f func(x, y uint64) (uint64, bool)) func(x, y uint64) bool {
		return func(x, y uint64) bool { _, ok := f(x, y); return ok }
	}
	keys64 := func(x, y uint64) bool { _, _, ok := bid.SameQuantumKeys64(x, y); return ok }
	for _, tc := range []struct {
		op    string
		f     func(x, y uint64) bool
		cases []case64
	}{
		{"addSameQuantum64", ok64(bid.AddSameQuantum64), []case64{
			{"sum 2^53-1", enc64(false, 1<<53-2, b64), enc64(false, 1, b64), true},
			{"sum 2^53", enc64(false, 1<<53-1, b64), enc64(false, 1, b64), false},
			{"negative", enc64(true, 5, b64), enc64(true, 7, b64), true},
			{"opposite signs", enc64(false, 5, b64), enc64(true, 7, b64), false},
			{"exponents apart", enc64(false, 5, b64), enc64(false, 7, b64+1), false},
			{"biased exponent 511", enc64(false, 5, 511), enc64(false, 7, 511), true},
			{"biased exponent 512", enc64(false, 5, 512), enc64(false, 7, 512), false},
			{"zeros", enc64(true, 0, 0), enc64(true, 0, 0), true},
			{"infinities", bid.Inf64, bid.Inf64, false},
		}},
		{"subSameQuantum64", ok64(bid.SubSameQuantum64), []case64{
			{"larger x", enc64(false, 9, b64), enc64(false, 2, b64), true},
			{"larger y", enc64(true, 2, b64), enc64(true, 9, b64), true},
			{"equal", enc64(false, 9, b64), enc64(false, 9, b64), false},
			{"zero minus", enc64(false, 0, b64), enc64(false, 1<<53-1, b64), true},
			{"opposite signs", enc64(false, 9, b64), enc64(true, 2, b64), false},
			{"biased exponent 511", enc64(false, 9, 511), enc64(false, 2, 511), true},
			{"biased exponent 512", enc64(false, 9, 512), enc64(false, 2, 512), false},
		}},
		{"mulShort64", ok64(bid.MulShort64), []case64{
			{"product 2^53-1", enc64(false, 1<<53-1, b64), enc64(true, 1, b64), true},
			{"product below 2^53", enc64(false, 1<<26, b64), enc64(false, 1<<27-1, b64), true},
			{"product 2^53", enc64(false, 1<<26, b64), enc64(false, 1<<27, b64), false},
			{"biased exponent 0", enc64(false, 3, b64), enc64(false, 3, 0), true},
			{"biased exponent -1", enc64(false, 3, b64-1), enc64(false, 3, 0), false},
			{"biased exponent 511", enc64(false, 3, 455), enc64(false, 3, 454), true},
			{"biased exponent 512", enc64(false, 3, 455), enc64(false, 3, 455), false},
			{"operand exponent 512", enc64(false, 3, 512), enc64(false, 3, 0), false},
			{"second form", bid.Form2Mask64 | b64<<bid.Form2ExpShift64, enc64(false, 0, b64), false},
		}},
		{"quantizeSame64", bid.QuantizeSame64, []case64{
			{"same exponent", enc64(true, 12345, b64-2), enc64(false, 1, b64-2), true},
			{"other exponent", enc64(true, 12345, b64-4), enc64(false, 1, b64-2), false},
			{"biased exponent 511", enc64(false, 1, 511), enc64(false, 1, 511), true},
			{"biased exponent 512", enc64(false, 1, 512), enc64(false, 1, 512), false},
			{"y second form", enc64(false, 1, b64), bid.Form2Mask64 | b64<<bid.Form2ExpShift64, false},
			{"y NaN", enc64(false, 1, b64), bid.NaN64, false},
		}},
		{"sameQuantumKeys64", keys64, []case64{
			{"same quantum", enc64(true, 1, b64), enc64(true, 2, b64), true},
			{"zeros of two signs", enc64(true, 0, b64), enc64(false, 0, b64), false},
			{"exponents apart", enc64(false, 1, b64), enc64(false, 1, b64+1), false},
			{"biased exponent 511", enc64(false, 1, 511), enc64(false, 2, 511), true},
			{"biased exponent 512", enc64(false, 1, 512), enc64(false, 2, 512), false},
			{"NaNs", bid.NaN64, bid.NaN64, false},
		}},
	} {
		for _, c := range tc.cases {
			if got := tc.f(c.x, c.y); got != c.want {
				t.Errorf("%s, %s (%#016x, %#016x): decided = %v, want %v", tc.op, c.name, c.x, c.y, got, c.want)
			}
		}
	}

	type case128 struct {
		name string
		x, y bid.Uint128
		want bool
	}
	ok128 := func(f func(x, y bid.Uint128) (bid.Uint128, bool)) func(x, y bid.Uint128) bool {
		return func(x, y bid.Uint128) bool { _, ok := f(x, y); return ok }
	}
	keys128 := func(x, y bid.Uint128) bool { _, _, ok := bid.SameQuantumKeys128(x, y); return ok }
	w := func(neg bool, lo uint64, e uint64) bid.Uint128 { return enc128(neg, 0, lo, e) }
	for _, tc := range []struct {
		op    string
		f     func(x, y bid.Uint128) bool
		cases []case128
	}{
		{"addSameQuantum128", ok128(bid.AddSameQuantum128), []case128{
			{"carry into the high word", w(false, 1<<64-1, b128), w(false, 1, b128), true},
			{"negative", w(true, 5, b128), w(true, 7, b128), true},
			{"high coefficient", enc128(false, 1, 0, b128), enc128(false, 1, 0, b128), false},
			{"opposite signs", w(false, 5, b128), w(true, 7, b128), false},
			{"exponents apart", w(false, 5, b128), w(false, 7, b128+1), false},
			{"biased exponent 8191", w(false, 5, 8191), w(false, 7, 8191), true},
			{"biased exponent 8192", w(false, 5, 8192), w(false, 7, 8192), false},
			{"infinities", bid.Uint128{Hi: bid.Inf128Hi}, bid.Uint128{Hi: bid.Inf128Hi}, false},
		}},
		{"subSameQuantum128", ok128(bid.SubSameQuantum128), []case128{
			{"larger x", w(false, 9, b128), w(false, 2, b128), true},
			{"larger y", w(true, 2, b128), w(true, 1<<64-1, b128), true},
			{"equal", w(false, 9, b128), w(false, 9, b128), false},
			{"opposite signs", w(false, 9, b128), w(true, 2, b128), false},
			{"high coefficient", enc128(false, 1, 9, b128), enc128(false, 1, 2, b128), false},
			{"biased exponent 8191", w(false, 9, 8191), w(false, 2, 8191), true},
			{"biased exponent 8192", w(false, 9, 8192), w(false, 2, 8192), false},
		}},
		{"mulShort128", ok128(bid.MulShort128), []case128{
			{"product below 2^96", w(false, 1<<48, b128), w(true, 1<<48-1, b128), true},
			{"product 2^96", w(false, 1<<48, b128), w(false, 1<<48, b128), false},
			{"high word 2^32-1", w(false, 1<<64-1, b128), w(false, 1<<32, b128), true},
			{"high word 2^32", w(false, 1<<64-1, b128), w(false, 1<<32+1, b128), false},
			{"biased exponent 0", w(false, 3, b128), w(false, 3, 0), true},
			{"biased exponent -1", w(false, 3, b128-1), w(false, 3, 0), false},
			{"biased exponent 8191", w(false, 3, 7184), w(false, 3, 7183), true},
			{"biased exponent 8192", w(false, 3, 7184), w(false, 3, 7184), false},
			{"operand exponent 8192", w(false, 3, 8192), w(false, 3, 0), false},
			{"high coefficient", enc128(false, 1, 0, b128), w(false, 1, b128), false},
		}},
		{"quantizeSame128", bid.QuantizeSame128, []case128{
			{"same exponent", w(true, 12345, b128-2), enc128(false, 1, 5, b128-2), true},
			{"other exponent", w(true, 12345, b128-4), w(false, 1, b128-2), false},
			{"high coefficient", enc128(false, 1, 0, b128), w(false, 1, b128), false},
			{"biased exponent 8191", w(false, 1, 8191), w(false, 1, 8191), true},
			{"biased exponent 8192", w(false, 1, 8192), w(false, 1, 8192), false},
			{"y NaN", w(false, 1, b128), bid.Uint128{Hi: bid.NaN128Hi}, false},
		}},
		{"sameQuantumKeys128", keys128, []case128{
			{"same quantum", w(true, 1, b128), w(true, 2, b128), true},
			{"zeros of two signs", w(true, 0, b128), w(false, 0, b128), false},
			{"high coefficient", enc128(false, 1, 0, b128), enc128(false, 1, 0, b128), false},
			{"biased exponent 8191", w(false, 1, 8191), w(false, 2, 8191), true},
			{"biased exponent 8192", w(false, 1, 8192), w(false, 2, 8192), false},
		}},
	} {
		for _, c := range tc.cases {
			if got := tc.f(c.x, c.y); got != c.want {
				t.Errorf("%s, %s (%s, %s): decided = %v, want %v", tc.op, c.name, fmt.Sprintf("%#x", c.x), fmt.Sprintf("%#x", c.y), got, c.want)
			}
		}
	}
}
