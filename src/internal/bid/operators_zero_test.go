// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package bid_test

import (
	"internal/bid"
	"math/rand/v2"
	"testing"
)

// Keep constants in these functions so the compiler must exercise its
// constant-zero comparison path, including a zero with another quantum.
//
//go:noinline
func opZero64(x decimal64) [16]bool {
	z := decimal64(0.00)
	return [16]bool{x == 0, x != 0, 0.00 == x, 0.00 != x,
		x < 0, x <= 0, x > 0, x >= 0,
		0.00 < x, 0.00 <= x, 0.00 > x, 0.00 >= x,
		x < z, x <= z, z < x, z <= x}
}

//go:noinline
func opZero128(x decimal128) [16]bool {
	z := decimal128(0.00)
	return [16]bool{x == 0, x != 0, 0.00 == x, 0.00 != x,
		x < 0, x <= 0, x > 0, x >= 0,
		0.00 < x, 0.00 <= x, 0.00 > x, 0.00 >= x,
		x < z, x <= z, z < x, z <= x}
}

// A join must not keep the zero fact from only one predecessor.
//
//go:noinline
func opMergedZero64(x decimal64, replace bool) bool {
	z := decimal64(0.00)
	if replace {
		z = 1
	}
	return x < z
}

//go:noinline
func opMergedZero128(x decimal128, replace bool) bool {
	z := decimal128(0.00)
	if replace {
		z = 1
	}
	return x < z
}

// TestOperatorZero compares the compiled predicates to the independent
// reference decoder, including every sign/exponent/special prefix and the
// exact boundary between canonical coefficients and encodings of zero.
func TestOperatorZero(t *testing.T) {
	check := func(f *format, x bits) {
		t.Helper()
		v := f.decode(x)
		zero := v.kind == finite && v.coeff.Sign() == 0
		negative := v.kind != notANumber && !zero && v.neg
		positive := v.kind != notANumber && !zero && !v.neg
		want := [16]bool{zero, !zero, zero, !zero,
			negative, negative || zero, positive, positive || zero,
			positive, positive || zero, negative, negative || zero,
			negative, negative || zero, positive, positive || zero}
		var got [16]bool
		if f == dec64 {
			got = opZero64(d64(x))
		} else {
			got = opZero128(d128(x))
		}
		if got != want {
			t.Fatalf("%s zero comparison of %x: got %v, want %v", f.name, x, got, want)
		}
		one := bits{Lo: bid.Zero64 | 1}
		var merged [2]bool
		if f == dec64 {
			merged = [2]bool{opMergedZero64(d64(x), false), opMergedZero64(d64(x), true)}
		} else {
			one = bits{Hi: bid.Zero128Hi, Lo: 1}
			merged = [2]bool{opMergedZero128(d128(x), false), opMergedZero128(d128(x), true)}
		}
		c, ordered := f.compare(x, one)
		if want := [2]bool{negative, ordered && c < 0}; merged != want {
			t.Fatalf("%s merged zero comparison of %x: got %v, want %v", f.name, x, merged, want)
		}
	}
	for prefix := uint64(0); prefix < 1<<16; prefix++ {
		for _, lo := range []uint64{0, 1, 1<<48 - 1} {
			check(dec64, bits{Lo: prefix<<48 | lo})
			check(dec128, bits{Hi: prefix << 48, Lo: lo})
		}
	}
	for _, sign := range []uint64{0, 1 << 63} {
		for _, c := range []uint64{bid.MaxCoeff64 - 1, bid.MaxCoeff64, bid.MaxCoeff64 + 1} {
			check(dec64, bits{Lo: sign | bid.Form2Mask64 | c&(1<<bid.Form2CoeffBits64-1)})
		}
		for _, hi := range []uint64{bid.MaxCoeff128Hi - 1, bid.MaxCoeff128Hi, bid.MaxCoeff128Hi + 1} {
			for _, lo := range []uint64{0, bid.MaxCoeff128Lo - 1, bid.MaxCoeff128Lo, bid.MaxCoeff128Lo + 1, ^uint64(0)} {
				check(dec128, bits{Hi: sign | bid.Zero128Hi | hi, Lo: lo})
			}
		}
	}
	r := rand.New(rand.NewPCG(*seedFlag, streamSeed("operator-zero")))
	for range numCases() {
		check(dec64, bits{Lo: r.Uint64()})
		check(dec128, bits{Hi: r.Uint64(), Lo: r.Uint64()})
	}
}
