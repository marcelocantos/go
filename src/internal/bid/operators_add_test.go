// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package bid_test

import (
	"math/big"
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
		for _, c := range []struct {
			name      string
			got, want bits
		}{
			{"add", b128(opAdd128(d128(x), d128(y))), dec128.add(x, y, rne)},
			{"sub", b128(opSub128(d128(x), d128(y))), dec128.sub(x, y, rne)},
		} {
			if c.got != c.want {
				t.Fatalf("compiled %s(%x, %x): got %x, want %x", c.name, x, y, c.got, c.want)
			}
		}
	}
	for prefix := uint64(0); prefix < 1<<16; prefix++ {
		hi := prefix << 48
		for _, pair := range [][2]uint64{{0, 0}, {0, 1}, {1, 1}, {^uint64(0), 1}, {^uint64(0), ^uint64(0)}} {
			check(bits{Hi: hi, Lo: pair[0]}, bits{Hi: hi, Lo: pair[1]})
		}
	}
	// Width boundaries and preferred-quantum/cancellation cases, in both
	// operand orders. The reference parses the decimal strings independently.
	var values []bits
	for _, s := range []string{"0", "-0", "0.00", "-0.00", "1", "-1", "1.0", "1.00", "-1.0", "-1.00", "0.001", "-0.001",
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

// TestOperatorAlignedReference checks every representable quantum at gaps
// around the word-alignment boundary, then draws word coefficients directly
// from a fresh stream. The independent encoder supplies the operands and the
// reference model determines result bits, including cancellation and quantum.
func TestOperatorAlignedReference(t *testing.T) {
	check := func(x, y bits) {
		t.Helper()
		if got, want := b128(opAdd128(d128(x), d128(y))), dec128.add(x, y, rne); got != want {
			t.Fatalf("aligned add(%x, %x): got %x, want %x", x, y, got, want)
		}
		if got, want := b128(opSub128(d128(x), d128(y))), dec128.sub(x, y, rne); got != want {
			t.Fatalf("aligned sub(%x, %x): got %x, want %x", x, y, got, want)
		}
	}
	word := func(c uint64, exp int, neg bool) bits {
		return dec128.encode(value{kind: finite, neg: neg, coeff: new(big.Int).SetUint64(c), exp: exp})
	}
	coeffs := [...]uint64{0, 1, ^uint64(0)}
	for _, gap := range []int{0, 1, 14, 15} {
		for exp := dec128.minExp; exp <= dec128.maxExp-gap; exp++ {
			i := exp - dec128.minExp
			x := word(coeffs[i%len(coeffs)], exp+gap, i&1 != 0)
			y := word(^uint64(0), exp, i&2 != 0)
			check(x, y)
			check(y, x)
		}
	}
	r := rand.New(rand.NewPCG(*seedFlag, streamSeed("operator-word-alignment")))
	for range numCases() {
		gap := r.IntN(17)
		exp := dec128.minExp + r.IntN(dec128.maxExp-dec128.minExp+1-gap)
		x := word(r.Uint64(), exp+gap, r.IntN(2) != 0)
		y := word(r.Uint64(), exp, r.IntN(2) != 0)
		check(x, y)
		check(y, x)
	}
}
