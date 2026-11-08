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
func opZero64(x decimal64) [4]bool { return [4]bool{x == 0, x != 0, 0.00 == x, 0.00 != x} }

//go:noinline
func opZero128(x decimal128) [4]bool { return [4]bool{x == 0, x != 0, 0.00 == x, 0.00 != x} }

// TestOperatorZero compares the compiled predicates to the independent
// reference decoder, including every sign/exponent/special prefix and the
// exact boundary between canonical coefficients and encodings of zero.
func TestOperatorZero(t *testing.T) {
	check := func(f *format, x bits) {
		t.Helper()
		v := f.decode(x)
		zero := v.kind == finite && v.coeff.Sign() == 0
		want := [4]bool{zero, !zero, zero, !zero}
		var got [4]bool
		if f == dec64 {
			got = opZero64(d64(x))
		} else {
			got = opZero128(d128(x))
		}
		if got != want {
			t.Fatalf("%s zero comparison of %x: got %v, want %v", f.name, x, got, want)
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
