// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package decimal_test

import (
	"fmt"
	"internal/bid"
	"math/big"
	"math/decimal"
	"math/rand/v2"
	"strconv"
	"testing"
)

// The operations of this file are the ones fixed-scale money arithmetic
// needs beyond IEEE 754: the AwayFromZero mode, division rounded once to a
// quantum, and access to a value's coefficient and exponent. Each test
// checks a function against something it does not call: math/big for the
// values, the kernel for the bits, strconv.ParseDecimal for Compose's
// rounding.

var allModes = []decimal.RoundingMode{
	decimal.ToNearestEven, decimal.ToNearestAway, decimal.ToZero,
	decimal.AwayFromZero, decimal.ToNegativeInf, decimal.ToPositiveInf,
}

func TestRoundingModeIsBigs(t *testing.T) {
	bigModes := []big.RoundingMode{
		big.ToNearestEven, big.ToNearestAway, big.ToZero,
		big.AwayFromZero, big.ToNegativeInf, big.ToPositiveInf,
	}
	for i, m := range allModes {
		if int(m) != int(bigModes[i]) {
			t.Errorf("%v = %d, math/big's is %d", m, m, bigModes[i])
		}
		if m.String() != bigModes[i].String() {
			t.Errorf("RoundingMode(%d).String() = %q, math/big's is %q", m, m.String(), bigModes[i].String())
		}
	}
	for _, n := range []int{6, 9, 42, 255} {
		if got, want := decimal.RoundingMode(n).String(), big.RoundingMode(n).String(); got != want {
			t.Errorf("RoundingMode(%d).String() = %q, want %q", n, got, want)
		}
	}
}

// width describes one decimal type for these tests: its precision, and how
// to make a value from a numeral and read one back through the kernel's
// encoding, which none of the functions under test is.
type width struct {
	name    string
	prec    int
	parse   func(t *testing.T, s string) bid.Uint128
	quo     func(x, y, q bid.Uint128, m decimal.RoundingMode) bid.Uint128
	kquo    func(x, y, q bid.Uint128, m bid.Mode) bid.Uint128
	unpack  func(x bid.Uint128) (bid.Kind, bool, *big.Int, int)
	compose func(neg bool, hi, lo uint64, exp int) bid.Uint128
	decomp  func(x bid.Uint128) (neg bool, hi, lo uint64, exp int, ok bool)
	random  func(r *rand.Rand) bid.Uint128
}

func widths() []width {
	b64 := func(x bid.Uint128) decimal64 { return decimal.BID64frombits(x.Lo) }
	u64 := func(d decimal64) bid.Uint128 { return bid.Uint128{Lo: decimal.BID64bits(d)} }
	b128 := func(x bid.Uint128) decimal128 { return decimal.BID128frombits(x.Hi, x.Lo) }
	u128 := func(d decimal128) bid.Uint128 {
		hi, lo := decimal.BID128bits(d)
		return bid.Uint128{Hi: hi, Lo: lo}
	}
	return []width{
		{
			name: "64", prec: 16,
			parse: func(t *testing.T, s string) bid.Uint128 {
				d, err := strconv.ParseDecimal(s, 64)
				if err != nil {
					t.Fatalf("ParseDecimal(%q, 64): %v", s, err)
				}
				return u64(decimal64(d))
			},
			quo: func(x, y, q bid.Uint128, m decimal.RoundingMode) bid.Uint128 {
				return u64(decimal.QuoQuantize(b64(x), b64(y), b64(q), m))
			},
			kquo: func(x, y, q bid.Uint128, m bid.Mode) bid.Uint128 {
				return bid.Uint128{Lo: bid.QuoQuantize64(x.Lo, y.Lo, q.Lo, m)}
			},
			unpack: func(x bid.Uint128) (bid.Kind, bool, *big.Int, int) {
				k, neg, c, e := bid.Unpack64(x.Lo)
				return k, neg, new(big.Int).SetUint64(c), e
			},
			compose: func(neg bool, hi, lo uint64, exp int) bid.Uint128 {
				return u64(decimal.Compose[decimal64](neg, hi, lo, exp))
			},
			decomp: func(x bid.Uint128) (bool, uint64, uint64, int, bool) { return decimal.Decompose(b64(x)) },
			random: func(r *rand.Rand) bid.Uint128 { return bid.Uint128{Lo: r.Uint64()} },
		},
		{
			name: "128", prec: 34,
			parse: func(t *testing.T, s string) bid.Uint128 {
				d, err := strconv.ParseDecimal(s, 128)
				if err != nil {
					t.Fatalf("ParseDecimal(%q, 128): %v", s, err)
				}
				return u128(d)
			},
			quo: func(x, y, q bid.Uint128, m decimal.RoundingMode) bid.Uint128 {
				return u128(decimal.QuoQuantize(b128(x), b128(y), b128(q), m))
			},
			kquo: bid.QuoQuantize128,
			unpack: func(x bid.Uint128) (bid.Kind, bool, *big.Int, int) {
				k, neg, c, e := bid.Unpack128(x)
				return k, neg, new(big.Int).Or(new(big.Int).Lsh(new(big.Int).SetUint64(c.Hi), 64), new(big.Int).SetUint64(c.Lo)), e
			},
			compose: func(neg bool, hi, lo uint64, exp int) bid.Uint128 {
				return u128(decimal.Compose[decimal128](neg, hi, lo, exp))
			},
			decomp: func(x bid.Uint128) (bool, uint64, uint64, int, bool) { return decimal.Decompose(b128(x)) },
			random: func(r *rand.Rand) bid.Uint128 { return bid.Uint128{Hi: r.Uint64(), Lo: r.Uint64()} },
		},
	}
}

// roundRat rounds the non-negative rational v to an integer in mode, for a
// value of sign neg. It is this test's own rounding, written from the
// definitions of the modes.
func roundRat(v *big.Rat, neg bool, m decimal.RoundingMode) *big.Int {
	q, r := new(big.Int).QuoRem(v.Num(), v.Denom(), new(big.Int))
	if r.Sign() == 0 {
		return q
	}
	c := new(big.Int).Lsh(r, 1).Cmp(v.Denom()) // the fraction against one half
	var up bool
	switch m {
	case decimal.ToNearestEven:
		up = c > 0 || c == 0 && q.Bit(0) == 1
	case decimal.ToNearestAway:
		up = c >= 0
	case decimal.ToZero:
	case decimal.AwayFromZero:
		up = true
	case decimal.ToNegativeInf:
		up = neg
	case decimal.ToPositiveInf:
		up = !neg
	}
	if up {
		q.Add(q, big.NewInt(1))
	}
	return q
}

func pow10Rat(n int) *big.Rat {
	p := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs(n))), nil)
	if n < 0 {
		return new(big.Rat).SetFrac(big.NewInt(1), p)
	}
	return new(big.Rat).SetInt(p)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func TestQuoQuantizeAgreesWithBig(t *testing.T) {
	dividends := []string{"0", "-0", "1", "2", "-1", "7", "0.06", "1.50", "-7.5", "18375.0000000",
		"2.009999999999999", "123.456", "9999999999999999", "1E+20", "1E-20", "3.14159", "-0.005"}
	divisors := []string{"1", "2", "3", "-3", "7", "8", "12", "0.1", "2.5", "1E-10", "1E+10", "9999999"}
	quanta := []string{"0.01", "1", "0.1", "1E+2", "1E-10", "1E-15", "1E-30", "-5.000"}
	for _, w := range widths() {
		var n, nan int
		for _, xs := range dividends {
			x := w.parse(t, xs)
			_, xneg, xc, xe := w.unpack(x)
			for _, ys := range divisors {
				y := w.parse(t, ys)
				_, yneg, yc, ye := w.unpack(y)
				for _, qs := range quanta {
					q := w.parse(t, qs)
					_, _, _, e := w.unpack(q)
					neg := xneg != yneg
					// |x / y| / 10^e, exactly.
					v := new(big.Rat).SetFrac(xc, yc)
					v.Mul(v, pow10Rat(xe-ye-e))
					for _, m := range allModes {
						got := w.quo(x, y, q, m)
						gk, gneg, gc, ge := w.unpack(got)
						want := roundRat(v, neg, m)
						if len(want.String()) > w.prec {
							if gk != bid.KindNaN {
								t.Errorf("decimal%s QuoQuantize(%s, %s, %s, %v) = %s, want NaN (coefficient %s does not fit)",
									w.name, xs, ys, qs, m, show(gk, gneg, gc, ge), want)
							}
							nan++
							continue
						}
						if gk != bid.KindFinite || gneg != neg || gc.Cmp(want) != 0 || ge != e {
							t.Errorf("decimal%s QuoQuantize(%s, %s, %s, %v) = %s, want %s",
								w.name, xs, ys, qs, m, show(gk, gneg, gc, ge), show(bid.KindFinite, neg, want, e))
						}
						n++
					}
				}
			}
		}
		t.Logf("decimal%s: %d finite results and %d that do not fit checked", w.name, n, nan)
	}
}

func show(k bid.Kind, neg bool, c *big.Int, e int) string {
	switch k {
	case bid.KindNaN:
		return "NaN"
	case bid.KindInf:
		if neg {
			return "-Inf"
		}
		return "+Inf"
	}
	s := fmt.Sprintf("%sE%+d", c, e)
	if neg {
		s = "-" + s
	}
	return s
}

func TestQuoQuantizeSpecialCases(t *testing.T) {
	for _, w := range widths() {
		for _, c := range []struct{ x, y, q, want string }{
			{"Inf", "Inf", "0.01", "NaN"},
			{"0", "0", "0.01", "NaN"},
			{"1", "0", "0.01", "NaN"},
			{"1", "0", "Inf", "+Inf"},
			{"-1", "0", "Inf", "-Inf"},
			{"Inf", "2", "Inf", "+Inf"},
			{"Inf", "-2", "Inf", "-Inf"},
			{"Inf", "2", "0.01", "NaN"},
			{"5", "Inf", "0.01", "0E-2"},
			{"-5", "Inf", "1E+3", "-0E+3"},
			{"5", "Inf", "Inf", "NaN"},
			{"2", "3", "Inf", "NaN"},
			{"NaN", "1", "0.01", "NaN"},
			{"1", "NaN", "0.01", "NaN"},
			{"1", "1", "NaN", "NaN"},
		} {
			k, neg, coeff, e := w.unpack(w.quo(w.parse(t, c.x), w.parse(t, c.y), w.parse(t, c.q), decimal.ToNearestEven))
			if got := show(k, neg, coeff, e); got != c.want {
				t.Errorf("decimal%s QuoQuantize(%s, %s, %s) = %s, want %s", w.name, c.x, c.y, c.q, got, c.want)
			}
		}
	}
}

// TestQuoQuantizeDoubleRounding pins the case the function exists for: the
// operator's rounding to the precision lands on the tie that quantizing
// then decides.
func TestQuoQuantizeDoubleRounding(t *testing.T) {
	x, y := decimal64(2.009999999999999), decimal64(2)
	if got := decimal.QuantizeMode(x/y, 0.01, decimal.ToNearestAway); got != 1.01 {
		t.Fatalf("QuantizeMode(x/y, 0.01, ToNearestAway) = %v; the example no longer rounds twice", got)
	}
	if got := decimal.QuoQuantize(x, y, 0.01, decimal.ToNearestAway); got != 1.00 || !decimal.SameQuantum(got, 0.01) {
		t.Errorf("QuoQuantize(%v, %v, 0.01, ToNearestAway) = %v, want 1.00", x, y, got)
	}
}

func TestQuoQuantizeIsKernelCall(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for _, w := range widths() {
		for i := 0; i < 20000; i++ {
			x, y, q := w.random(r), w.random(r), w.random(r)
			m := decimal.RoundingMode(r.IntN(8))
			if got, want := w.quo(x, y, q, m), w.kquo(x, y, q, bid.Mode(m)); got != want {
				t.Fatalf("decimal%s QuoQuantize(%x, %x, %x, %d) = %x, kernel %x", w.name, x, y, q, m, got, want)
			}
		}
	}
}

func TestDecompose(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for _, w := range widths() {
		for i := 0; i < 20000; i++ {
			x := w.random(r)
			neg, hi, lo, exp, ok := w.decomp(x)
			k, kneg, kc, ke := w.unpack(x)
			if ok != (k == bid.KindFinite) {
				t.Fatalf("decimal%s Decompose(%x) ok = %v, kernel kind %v", w.name, x, ok, k)
			}
			if !ok {
				if neg || hi != 0 || lo != 0 || exp != 0 {
					t.Fatalf("decimal%s Decompose(%x) = %v, %d, %d, %d, false; want zeros", w.name, x, neg, hi, lo, exp)
				}
				continue
			}
			c := new(big.Int).Or(new(big.Int).Lsh(new(big.Int).SetUint64(hi), 64), new(big.Int).SetUint64(lo))
			if neg != kneg || c.Cmp(kc) != 0 || exp != ke {
				t.Fatalf("decimal%s Decompose(%x) = %v, %s, %d; kernel %v, %s, %d", w.name, x, neg, c, exp, kneg, kc, ke)
			}
		}
	}
	neg, hi, lo, exp, ok := decimal.Decompose(decimal64(-1.50))
	if !neg || hi != 0 || lo != 150 || exp != -2 || !ok {
		t.Errorf("Decompose(-1.50) = %v, %d, %d, %d, %v; want true, 0, 150, -2, true", neg, hi, lo, exp, ok)
	}
}

// TestComposeRoundTrip checks that Compose gives back every finite
// canonical value from the parts Decompose returns.
func TestComposeRoundTrip(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	for _, w := range widths() {
		n := 0
		for i := 0; i < 20000; i++ {
			x := w.random(r)
			if w.name == "64" && !bid.IsCanonical64(x.Lo) || w.name == "128" && !bid.IsCanonical128(x) {
				continue
			}
			neg, hi, lo, exp, ok := w.decomp(x)
			if !ok {
				continue
			}
			if got := w.compose(neg, hi, lo, exp); got != x {
				t.Fatalf("decimal%s Compose(Decompose(%x)) = %x", w.name, x, got)
			}
			n++
		}
		if n < 1000 {
			t.Errorf("decimal%s: only %d finite canonical values drawn", w.name, n)
		}
	}
}

// TestComposeRoundsAsParsing checks Compose against strconv.ParseDecimal
// of the same digits: coefficients up to 128 bits, exponents across and
// beyond the range of each format.
func TestComposeRoundsAsParsing(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 8))
	for _, w := range widths() {
		for i := 0; i < 20000; i++ {
			neg := r.IntN(2) == 0
			hi, lo := r.Uint64()>>r.IntN(65), r.Uint64()>>r.IntN(65)
			exp := r.IntN(14000) - 7000
			if r.IntN(2) == 0 {
				exp = r.IntN(900) - 450
			}
			c := new(big.Int).Or(new(big.Int).Lsh(new(big.Int).SetUint64(hi), 64), new(big.Int).SetUint64(lo))
			s := fmt.Sprintf("%se%d", c, exp)
			if neg {
				s = "-" + s
			}
			// A value beyond the format is an infinity with ErrRange.
			d, err := strconv.ParseDecimal(s, map[string]int{"64": 64, "128": 128}[w.name])
			if err != nil && err.(*strconv.NumError).Err != strconv.ErrRange {
				t.Fatalf("ParseDecimal(%q): %v", s, err)
			}
			want := bid.Uint128{Lo: decimal.BID64bits(decimal64(d))}
			if w.name == "128" {
				want.Hi, want.Lo = decimal.BID128bits(d)
			}
			if got := w.compose(neg, hi, lo, exp); got != want {
				t.Fatalf("decimal%s Compose(%v, %d, %d, %d) = %x, ParseDecimal(%q) = %x", w.name, neg, hi, lo, exp, got, s, want)
			}
		}
	}
}
