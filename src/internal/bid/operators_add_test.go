// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package bid_test

import (
	"math/rand/v2"
	"testing"
)

// TestOperatorAddReference exercises the compiled operator, including its
// integer fast path, against the independent big.Int reference. The high-word
// sweep includes all sign/exponent/special prefixes, both finite encoding
// forms, zeros, word carries, and coefficients just too wide for the fast path.
func TestOperatorAddReference(t *testing.T) {
	check := func(x, y bits) {
		t.Helper()
		got := b128(opAdd128(d128(x), d128(y)))
		want := dec128.add(x, y, rne)
		if got != want {
			t.Fatalf("compiled add(%x, %x): got %x, want %x", x, y, got, want)
		}
	}
	for prefix := uint64(0); prefix < 1<<16; prefix++ {
		hi := prefix << 48
		for _, pair := range [][2]uint64{{0, 0}, {0, 1}, {1, 1}, {^uint64(0), 1}, {^uint64(0), ^uint64(0)}} {
			check(bits{Hi: hi, Lo: pair[0]}, bits{Hi: hi, Lo: pair[1]})
		}
	}
	// Width boundaries and preferred-quantum/cancellation fallbacks, in both
	// operand orders. The reference parses the decimal strings independently.
	var values []bits
	for _, s := range []string{"0", "-0", "0.00", "-0.00", "1", "-1", "0.001", "-0.001",
		"18446744073709551615", "18446744073709551616", "-18446744073709551615",
		"9999999999999999999999999999999999", "1E-6176", "1E+6111"} {
		values = append(values, addOperand(t, dec128, s))
	}
	for _, x := range values {
		for _, y := range values {
			check(x, y)
		}
	}
	g := &gen{f: dec128, r: rand.New(rand.NewPCG(*seedFlag, streamSeed("operator-add-reference")))}
	for i := range numCases() {
		g.regime = regime(i % int(numRegimes))
		x, y := g.pair("add")
		check(x, y)
	}
}
