// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package bid_test

import (
	"math/rand/v2"
	"testing"
)

// TestOperatorMulReference compares the compiled operator with the independent
// exact-product/rounding model. In particular, (10^17-1)*(10^17+1) is exactly
// the largest coefficient, while 10^17*10^17 must change quantum. The reference
// also decides zero clamping, sign, subnormal results, overflow and NaN payloads.
func TestOperatorMulReference(t *testing.T) {
	check := func(x, y bits) {
		t.Helper()
		got := b128(opMul128(d128(x), d128(y)))
		want := dec128.mul(x, y, rne)
		if got != want {
			t.Fatalf("compiled mul(%x, %x): got %x, want %x", x, y, got, want)
		}
	}
	for prefix := uint64(0); prefix < 1<<16; prefix++ {
		hi := prefix << 48
		for _, pair := range [][2]uint64{{0, 0}, {0, 1}, {1, 1}, {^uint64(0), 1}, {^uint64(0), ^uint64(0)}} {
			check(bits{Hi: hi, Lo: pair[0]}, bits{Hi: hi, Lo: pair[1]})
		}
	}
	var values []bits
	for _, s := range []string{
		"0", "-0", "0.00", "-0.00", "1", "-1", "0.001", "-0.001",
		"99999999999999999", "100000000000000000", "100000000000000001",
		"18446744073709551615", "18446744073709551616", "-18446744073709551615",
		"9999999999999999999999999999999999", "0E-6176", "-0E+6111",
		"1E-6176", "1E+6111", "0.1", "1E+1",
	} {
		values = append(values, addOperand(t, dec128, s))
	}
	for _, x := range values {
		for _, y := range values {
			check(x, y)
		}
	}
	g := &gen{f: dec128, r: rand.New(rand.NewPCG(*seedFlag, streamSeed("operator-mul-reference")))}
	for i := range numCases() {
		g.regime = regime(i % int(numRegimes))
		x, y := g.pair("mul")
		check(x, y)
	}
}
