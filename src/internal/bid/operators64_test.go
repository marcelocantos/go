// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package bid_test

import (
	"math/rand/v2"
	"testing"
)

// TestOperator64Reference exercises the compiled decimal64 operators,
// including the compiler's integer fast paths, against the independent
// reference. The sweep over the top sixteen bits takes every sign, exponent,
// special and form prefix with coefficients that carry out of 53 bits, cancel,
// borrow and stay put; the listed values cover the fast paths' bounds in both
// operand orders.
func TestOperator64Reference(t *testing.T) {
	check := func(x, y bits) {
		t.Helper()
		for _, c := range []struct {
			name      string
			got, want bits
		}{
			{"add", b64(opAdd64(d64(x), d64(y))), dec64.add(x, y, rne)},
			{"sub", b64(opSub64(d64(x), d64(y))), dec64.sub(x, y, rne)},
			{"mul", b64(opMul64(d64(x), d64(y))), dec64.mul(x, y, rne)},
			{"div", b64(opDiv64(d64(x), d64(y))), dec64.div(x, y, rne)},
		} {
			if c.got != c.want {
				t.Fatalf("compiled %s(%x, %x): got %x, want %x", c.name, x.Lo, y.Lo, c.got.Lo, c.want.Lo)
			}
		}
	}
	const low = 1<<48 - 1
	for prefix := uint64(0); prefix < 1<<16; prefix++ {
		hi := prefix << 48
		for _, pair := range [][2]uint64{{0, 0}, {0, 1}, {1, 1}, {1, 2}, {low, 1}, {low, low}} {
			check(bits{Lo: hi | pair[0]}, bits{Lo: hi | pair[1]})
			check(bits{Lo: hi | pair[0]}, bits{Lo: hi ^ 1<<63 | pair[1]})
		}
	}
	var values []bits
	for _, s := range []string{
		"0", "-0", "0.00", "-0.00", "1", "-1", "1.0", "1.00", "-1.0", "-1.00", "0.001", "-0.001",
		"1234.56", "-78.90", "78.90", "94906265", "94906266", "4503599627370495", "4503599627370496",
		"9007199254740991", "9007199254740992", "-9007199254740991", "9999999999999999", "-9999999999999999",
		"1E-398", "1E+369", "1E+113", "1E+114", "-1E+114", "9999999999999999E+369",
	} {
		values = append(values, addOperand(t, dec64, s))
	}
	for _, x := range values {
		for _, y := range values {
			check(x, y)
		}
	}
	for _, op := range []string{"add", "mul", "div"} {
		g := &gen{f: dec64, r: rand.New(rand.NewPCG(*seedFlag, streamSeed("operator64-reference-"+op)))}
		for i := range numCases() {
			g.regime = regime(i % int(numRegimes))
			x, y := g.pair(op)
			check(x, y)
		}
	}
}
