// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"internal/bid"
	"math/big"
	"math/rand/v2"
	"testing"
)

// TestDivPowersOfTen checks the entire coefficient-power range and every
// possible trailing-zero count against the independent reference. Quantum
// limits deliberately include exact clamping, overflow, subnormal ties and
// underflow; none of those may be replaced by an unchecked exponent shift.
func TestDivPowersOfTen(t *testing.T) {
	for _, f := range both {
		t.Run(f.name, func(t *testing.T) {
			check := func(x, y bits) {
				t.Helper()
				for m := bid.Mode(0); m < 6; m++ {
					got, want := (kernel{}).div(f, x, y, m), f.div(x, y, m)
					if got != want {
						t.Fatalf("power division %s(%x, %x, %s): got %x, want %x", f.name, x, y, modeName(m), got, want)
					}
				}
			}
			exponents := [][2]int{{0, 0}, {f.minExp, 1}, {f.minExp, -1}, {f.maxExp, -1}, {f.maxExp, 1}, {0, f.minExp}, {0, f.maxExp}}
			for k := 0; k < f.prec; k++ {
				for zeros := 0; zeros < f.prec; zeros++ {
					for _, lead := range []int64{1, 5, 9} {
						c := new(big.Int).Mul(big.NewInt(lead), pow10(zeros))
						for i, e := range exponents {
							x := f.encode(value{kind: finite, neg: i&1 != 0, coeff: c, exp: e[0]})
							y := f.encode(value{kind: finite, neg: i&2 != 0, coeff: pow10(k), exp: e[1]})
							check(x, y)
						}
					}
				}
			}
			r := rand.New(rand.NewPCG(*seedFlag, streamSeed("division-powers-"+f.name)))
			for range numCases() {
				x := f.encode(value{kind: finite, neg: r.IntN(2) != 0, coeff: operandCoeff(r, f.prec), exp: f.minExp + r.IntN(f.maxExp-f.minExp+1)})
				y := f.encode(value{kind: finite, neg: r.IntN(2) != 0, coeff: pow10(r.IntN(f.prec)), exp: f.minExp + r.IntN(f.maxExp-f.minExp+1)})
				check(x, y)
			}
		})
	}
}

func BenchmarkDivPowersOfTen(b *testing.B) {
	for _, c := range []struct {
		name string
		x, y [3]string
	}{
		{"one", [3]string{"123.45", "123.45"}, [3]string{"1", "1"}},
		{"word", [3]string{"123.45", "123.45"}, [3]string{"100", "100"}},
		{"trailing", [3]string{"12300", "12300"}, [3]string{"100", "100"}},
		{"wide", [3]string{"1234567890123456", "1234567890123456789012345678901234"}, [3]string{"1000000000000000", "1000000000000000000000000000000000"}},
		{"general", [3]string{"123.45", "123.45"}, [3]string{"1.234567890123456", "1.234567890123456789012345678901234"}},
	} {
		o := fixedOpnds(c.x, c.y, [3]string{}, 0, 0, 0)
		b.Run(c.name+"/64", func(b *testing.B) {
			loop2(b, o.x64, o.y64, func(x, y uint64) uint64 { return bid.Div64(x, y, rne) })
		})
		b.Run(c.name+"/128", func(b *testing.B) {
			loop2(b, o.x128, o.y128, func(x, y bits) bits { return bid.Div128(x, y, rne) })
		})
	}
}
