// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package math_test

import (
	"fmt"
	"internal/bid"
	. "math"
	"math/big"
	"testing"
)

// The decimal helpers of package math are thin wrappers over internal/bid.
// These tests check two things for Mod, Remainder, FMA, Quantize, the
// integral roundings and SameQuantum, at both widths:
//
//   - each helper returns, bit for bit, what the kernel function it should
//     call returns (TestDecimalHelpersAreKernelCalls), and
//   - that result is the exact answer, computed here with math/big from
//     the definitions in IEEE 754-2008 clauses 5.2, 5.3 and 5.4.1 and the
//     exponent rules of internal/bid's package documentation
//     (TestDecimalHelpersAgreeWithBig).
//
// The operands are built and the results read back with bid.Pack and
// bid.Unpack, which none of the checked operations is; the expected values
// come from math/big alone. The kernel's own oracle stays in internal/bid.

// dval is a decimal value in parts: a finite (-1)^neg × coeff × 10^exp, an
// infinity or a NaN.
type dval struct {
	kind  bid.Kind
	neg   bool
	coeff *big.Int
	exp   int
}

func (v dval) String() string {
	switch v.kind {
	case bid.KindNaN:
		return "NaN"
	case bid.KindInf:
		if v.neg {
			return "-Inf"
		}
		return "Inf"
	}
	sign := ""
	if v.neg {
		sign = "-"
	}
	return fmt.Sprintf("%s%sE%+d", sign, v.coeff, v.exp)
}

func (v dval) isZero() bool { return v.kind == bid.KindFinite && v.coeff.Sign() == 0 }

// parseDval reads "NaN", "Inf", "-Inf" or [-]digits[.digits][E[+-]digits].
func parseDval(t *testing.T, s string) dval {
	t.Helper()
	switch s {
	case "NaN":
		return dval{kind: bid.KindNaN}
	case "Inf":
		return dval{kind: bid.KindInf}
	case "-Inf":
		return dval{kind: bid.KindInf, neg: true}
	}
	v := dval{coeff: new(big.Int)}
	if s[0] == '-' {
		v.neg = true
		s = s[1:]
	}
	mant, expStr := s, ""
	for i := 0; i < len(s); i++ {
		if s[i] == 'E' {
			mant, expStr = s[:i], s[i+1:]
			break
		}
	}
	digits := ""
	for i := 0; i < len(mant); i++ {
		if mant[i] == '.' {
			v.exp = -(len(mant) - i - 1)
			continue
		}
		digits += string(mant[i])
	}
	if _, ok := v.coeff.SetString(digits, 10); !ok {
		t.Fatalf("bad decimal %q", s)
	}
	if expStr != "" {
		var e int
		if _, err := fmt.Sscanf(expStr, "%d", &e); err != nil {
			t.Fatalf("bad decimal %q: %v", s, err)
		}
		v.exp += e
	}
	return v
}

// dwidth describes one decimal format and how to move values in and out of
// bit patterns. A decimal64 pattern is held in a bid.Uint128's Lo word.
type dwidth struct {
	name   string
	prec   int
	minExp int
	maxExp int
	mod    func(x, y bid.Uint128) bid.Uint128
	rem    func(x, y bid.Uint128) bid.Uint128
	fma    func(x, y, z bid.Uint128) bid.Uint128
	quant  func(x, y bid.Uint128) bid.Uint128
	same   func(x, y bid.Uint128) bool
	kmod   func(x, y bid.Uint128) bid.Uint128
	krem   func(x, y bid.Uint128) bid.Uint128
	kfma   func(x, y, z bid.Uint128) bid.Uint128
	kquant func(x, y bid.Uint128) bid.Uint128
	ksame  func(x, y bid.Uint128) bool
	rti    map[string]func(x bid.Uint128) bid.Uint128 // by helper name
	krti   func(x bid.Uint128, m bid.Mode) bid.Uint128
	pack   func(v dval) (bid.Uint128, bool)
	unpack func(x bid.Uint128) dval
}

func u64(x uint64) bid.Uint128 { return bid.Uint128{Lo: x} }

func packSpecial(v dval, inf, nan bid.Uint128, neg func(bid.Uint128) bid.Uint128) (bid.Uint128, bool) {
	switch v.kind {
	case bid.KindNaN:
		return nan, true
	case bid.KindInf:
		if v.neg {
			return neg(inf), true
		}
		return inf, true
	}
	panic("not special")
}

var (
	two64 = new(big.Int).Lsh(big.NewInt(1), 64)
	mask  = new(big.Int).Sub(two64, big.NewInt(1))
)

func widthTable() []*dwidth {
	w64 := &dwidth{
		name: "64", prec: bid.Precision64, minExp: bid.MinExp64, maxExp: bid.MaxExp64,
		mod: func(x, y bid.Uint128) bid.Uint128 {
			return u64(Decimal64bits(Decimal64Mod(Decimal64frombits(x.Lo), Decimal64frombits(y.Lo))))
		},
		rem: func(x, y bid.Uint128) bid.Uint128 {
			return u64(Decimal64bits(Decimal64Remainder(Decimal64frombits(x.Lo), Decimal64frombits(y.Lo))))
		},
		fma: func(x, y, z bid.Uint128) bid.Uint128 {
			return u64(Decimal64bits(Decimal64FMA(Decimal64frombits(x.Lo), Decimal64frombits(y.Lo), Decimal64frombits(z.Lo))))
		},
		quant: func(x, y bid.Uint128) bid.Uint128 {
			return u64(Decimal64bits(Decimal64Quantize(Decimal64frombits(x.Lo), Decimal64frombits(y.Lo))))
		},
		same: func(x, y bid.Uint128) bool {
			return Decimal64SameQuantum(Decimal64frombits(x.Lo), Decimal64frombits(y.Lo))
		},
		kmod:   func(x, y bid.Uint128) bid.Uint128 { return u64(bid.Mod64(x.Lo, y.Lo)) },
		krem:   func(x, y bid.Uint128) bid.Uint128 { return u64(bid.Rem64(x.Lo, y.Lo)) },
		kfma:   func(x, y, z bid.Uint128) bid.Uint128 { return u64(bid.FMA64(x.Lo, y.Lo, z.Lo, bid.ToNearestEven)) },
		kquant: func(x, y bid.Uint128) bid.Uint128 { return u64(bid.Quantize64(x.Lo, y.Lo, bid.ToNearestEven)) },
		ksame:  func(x, y bid.Uint128) bool { return bid.SameQuantum64(x.Lo, y.Lo) },
		krti:   func(x bid.Uint128, m bid.Mode) bid.Uint128 { return u64(bid.RoundToIntegral64(x.Lo, m)) },
		rti: map[string]func(bid.Uint128) bid.Uint128{
			"Trunc": func(x bid.Uint128) bid.Uint128 { return u64(Decimal64bits(Decimal64Trunc(Decimal64frombits(x.Lo)))) },
			"Floor": func(x bid.Uint128) bid.Uint128 { return u64(Decimal64bits(Decimal64Floor(Decimal64frombits(x.Lo)))) },
			"Ceil":  func(x bid.Uint128) bid.Uint128 { return u64(Decimal64bits(Decimal64Ceil(Decimal64frombits(x.Lo)))) },
			"Round": func(x bid.Uint128) bid.Uint128 { return u64(Decimal64bits(Decimal64Round(Decimal64frombits(x.Lo)))) },
			"RoundToEven": func(x bid.Uint128) bid.Uint128 {
				return u64(Decimal64bits(Decimal64RoundToEven(Decimal64frombits(x.Lo))))
			},
		},
		pack: func(v dval) (bid.Uint128, bool) {
			if v.kind != bid.KindFinite {
				return packSpecial(v, u64(bid.Inf64), u64(bid.NaN64), func(x bid.Uint128) bid.Uint128 { return u64(bid.Neg64(x.Lo)) })
			}
			if v.coeff.BitLen() > 64 {
				return bid.Uint128{}, false
			}
			x, ok := bid.Pack64(v.neg, v.coeff.Uint64(), v.exp)
			return u64(x), ok
		},
		unpack: func(x bid.Uint128) dval {
			kind, neg, c, e := bid.Unpack64(x.Lo)
			return dval{kind: kind, neg: neg, coeff: new(big.Int).SetUint64(c), exp: e}
		},
	}

	d := func(x bid.Uint128) decimal128 { return Decimal128frombits(x.Hi, x.Lo) }
	b := func(x decimal128) bid.Uint128 {
		hi, lo := Decimal128bits(x)
		return bid.Uint128{Hi: hi, Lo: lo}
	}
	w128 := &dwidth{
		name: "128", prec: bid.Precision128, minExp: bid.MinExp128, maxExp: bid.MaxExp128,
		mod:    func(x, y bid.Uint128) bid.Uint128 { return b(Decimal128Mod(d(x), d(y))) },
		rem:    func(x, y bid.Uint128) bid.Uint128 { return b(Decimal128Remainder(d(x), d(y))) },
		fma:    func(x, y, z bid.Uint128) bid.Uint128 { return b(Decimal128FMA(d(x), d(y), d(z))) },
		quant:  func(x, y bid.Uint128) bid.Uint128 { return b(Decimal128Quantize(d(x), d(y))) },
		same:   func(x, y bid.Uint128) bool { return Decimal128SameQuantum(d(x), d(y)) },
		kmod:   bid.Mod128,
		krem:   bid.Rem128,
		kfma:   func(x, y, z bid.Uint128) bid.Uint128 { return bid.FMA128(x, y, z, bid.ToNearestEven) },
		kquant: func(x, y bid.Uint128) bid.Uint128 { return bid.Quantize128(x, y, bid.ToNearestEven) },
		ksame:  bid.SameQuantum128,
		krti:   bid.RoundToIntegral128,
		rti: map[string]func(bid.Uint128) bid.Uint128{
			"Trunc":       func(x bid.Uint128) bid.Uint128 { return b(Decimal128Trunc(d(x))) },
			"Floor":       func(x bid.Uint128) bid.Uint128 { return b(Decimal128Floor(d(x))) },
			"Ceil":        func(x bid.Uint128) bid.Uint128 { return b(Decimal128Ceil(d(x))) },
			"Round":       func(x bid.Uint128) bid.Uint128 { return b(Decimal128Round(d(x))) },
			"RoundToEven": func(x bid.Uint128) bid.Uint128 { return b(Decimal128RoundToEven(d(x))) },
		},
		pack: func(v dval) (bid.Uint128, bool) {
			if v.kind != bid.KindFinite {
				return packSpecial(v, bid.Uint128{Hi: bid.Inf128Hi}, bid.Uint128{Hi: bid.NaN128Hi}, bid.Neg128)
			}
			if v.coeff.BitLen() > 128 {
				return bid.Uint128{}, false
			}
			c := bid.Uint128{
				Hi: new(big.Int).Rsh(v.coeff, 64).Uint64(),
				Lo: new(big.Int).And(v.coeff, mask).Uint64(),
			}
			return bid.Pack128(v.neg, c, v.exp)
		},
		unpack: func(x bid.Uint128) dval {
			kind, neg, c, e := bid.Unpack128(x)
			coeff := new(big.Int).Lsh(new(big.Int).SetUint64(c.Hi), 64)
			coeff.Or(coeff, new(big.Int).SetUint64(c.Lo))
			return dval{kind: kind, neg: neg, coeff: coeff, exp: e}
		},
	}
	return []*dwidth{w64, w128}
}

var rtiModeOf = map[string]bid.Mode{
	"Trunc":       bid.TowardZero,
	"Floor":       bid.TowardNegative,
	"Ceil":        bid.TowardPositive,
	"Round":       bid.ToNearestAway,
	"RoundToEven": bid.ToNearestEven,
}

// operands is the sample the helpers are run on. It holds the cases the
// audit of the old implementation found (a quotient beyond 16 digits and
// beyond 34), ties of every kind, cohort members, zeros of both signs,
// values at the edges of both formats and the special values. A value that
// does not fit a format is left out for that format.
var operands = []string{
	"0", "-0", "0.00", "-0.00", "0E+5", "0E-20", "-0E+300",
	"1", "-1", "2", "-2", "3", "-3", "6", "7", "-7", "97", "10",
	"0.1", "0.3", "-0.3", "0.5", "-0.5", "1.5", "-1.5", "2.5", "-2.5", "3.5", "-3.5",
	"0.4", "0.6", "-0.4", "0.50", "0.500000000000000", "2.50", "1.30", "-1.3", "3.6", "2.1",
	"5", "5E-1", "15E-1", "25E-1", "123.456", "-123.456", "100.0", "1E+3", "1.0E+6",
	"1E+15", "1E+16", "1E+17", "-1E+17", "1E+20", "1E+30", "-1E+30", "1E+34",
	"9999999999999999", "-9999999999999999", "9999999999999999E+369",
	"1234567890123456", "123456789012345.6", "1234567890123456.5",
	"1E-3", "1E-17", "1E-20", "1E-30", "3E-301", "1E-300", "1E+300", "7E+200",
	"1E-398", "-1E-398", "1E+369", "0.01", "0.001", "2.17", "-2.25", "2.35", "217",
	"1E+368", "1E+369", "9.999999999999999E+384",
	"99999999999999999999999999999999", "9999999999999999999999999999999999",
	"1234567890123456789012345678901234", "1234567890123456789012345678901234E-20",
	"1E-6176", "1E+6111", "3E+6000", "1E+1000", "1E-1000",
	"Inf", "-Inf", "NaN",
}

func TestDecimalHelpersAreKernelCalls(t *testing.T) {
	for _, w := range widthTable() {
		var xs []bid.Uint128
		for _, s := range operands {
			if x, ok := w.pack(parseDval(t, s)); ok {
				xs = append(xs, x)
			}
		}
		// A few encodings no value text reaches: a signaling NaN, a
		// negative NaN and non-canonical coefficients.
		if w.name == "64" {
			xs = append(xs, u64(bid.SNaN64), u64(bid.NaN64|bid.SignMask64), u64(bid.Form2Mask64|5<<51|0x7))
		} else {
			xs = append(xs, bid.Uint128{Hi: bid.SNaN128Hi}, bid.Uint128{Hi: bid.NaN128Hi | bid.SignMask128Hi},
				bid.Uint128{Hi: bid.Form2Mask128Hi | 7<<47, Lo: 3})
		}
		n := 0
		for _, x := range xs {
			for name, f := range w.rti {
				if got, want := f(x), w.krti(x, rtiModeOf[name]); got != want {
					t.Errorf("decimal%s %s(%v) = %v, kernel RoundToIntegral gives %v", w.name, name, w.unpack(x), w.unpack(got), w.unpack(want))
				}
				n++
			}
			for _, y := range xs {
				if got, want := w.mod(x, y), w.kmod(x, y); got != want {
					t.Errorf("decimal%s Mod(%v, %v) = %v, kernel gives %v", w.name, w.unpack(x), w.unpack(y), w.unpack(got), w.unpack(want))
				}
				if got, want := w.rem(x, y), w.krem(x, y); got != want {
					t.Errorf("decimal%s Remainder(%v, %v) = %v, kernel gives %v", w.name, w.unpack(x), w.unpack(y), w.unpack(got), w.unpack(want))
				}
				if got, want := w.quant(x, y), w.kquant(x, y); got != want {
					t.Errorf("decimal%s Quantize(%v, %v) = %v, kernel gives %v", w.name, w.unpack(x), w.unpack(y), w.unpack(got), w.unpack(want))
				}
				if got, want := w.same(x, y), w.ksame(x, y); got != want {
					t.Errorf("decimal%s SameQuantum(%v, %v) = %v, kernel gives %v", w.name, w.unpack(x), w.unpack(y), got, want)
				}
				n += 4
			}
		}
		// FMA over every triple of a subset keeps the run short.
		for i := 0; i < len(xs); i += 2 {
			for j := 0; j < len(xs); j += 3 {
				for k := 0; k < len(xs); k += 2 {
					x, y, z := xs[i], xs[j], xs[k]
					if got, want := w.fma(x, y, z), w.kfma(x, y, z); got != want {
						t.Errorf("decimal%s FMA(%v, %v, %v) = %v, kernel gives %v", w.name, w.unpack(x), w.unpack(y), w.unpack(z), w.unpack(got), w.unpack(want))
					}
					n++
				}
			}
		}
		t.Logf("decimal%s: %d helper calls compared with the kernel", w.name, n)
	}
}

// pow10 returns 10^n for n >= 0.
func pow10(n int) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }

// scaleTo returns the coefficient of v expressed at exponent e <= v.exp.
func scaleTo(v dval, e int) *big.Int {
	return new(big.Int).Mul(v.coeff, pow10(v.exp-e))
}

// roundQuo returns the quotient n/d of non-negative n and positive d
// rounded to an integer in mode; neg is the sign of the value n/d stands
// for (it decides the direction of the directed modes).
func roundQuo(n, d *big.Int, neg bool, mode bid.Mode) *big.Int {
	q, r := new(big.Int).QuoRem(n, d, new(big.Int))
	if r.Sign() == 0 {
		return q
	}
	up := false
	switch mode {
	case bid.TowardZero:
	case bid.TowardPositive:
		up = !neg
	case bid.TowardNegative:
		up = neg
	default:
		c := new(big.Int).Lsh(r, 1).Cmp(d)
		up = c > 0 || (c == 0 && (mode == bid.ToNearestAway || q.Bit(0) == 1))
	}
	if up {
		q.Add(q, big.NewInt(1))
	}
	return q
}

func nanVal() dval { return dval{kind: bid.KindNaN} }

// wantQuantize is the exact quantize of x to the exponent of y in mode.
func (w *dwidth) wantQuantize(x dval, ey int, mode bid.Mode) dval {
	var c *big.Int
	if x.exp >= ey {
		c = scaleTo(x, ey)
	} else {
		c = roundQuo(x.coeff, pow10(ey-x.exp), x.neg, mode)
	}
	if c.Cmp(new(big.Int).Sub(pow10(w.prec), big.NewInt(1))) > 0 {
		return nanVal()
	}
	return dval{kind: bid.KindFinite, neg: x.neg, coeff: c, exp: ey}
}

// wantQuantizeOp is the quantize operation on operands that may be special.
func (w *dwidth) wantQuantizeOp(x, y dval) dval {
	switch {
	case x.kind == bid.KindNaN || y.kind == bid.KindNaN:
		return nanVal()
	case x.kind == bid.KindInf && y.kind == bid.KindInf:
		return x
	case x.kind == bid.KindInf || y.kind == bid.KindInf:
		return nanVal()
	}
	return w.wantQuantize(x, y.exp, bid.ToNearestEven)
}

// wantRoundToIntegral is the exact roundToIntegral of x in mode.
func (w *dwidth) wantRoundToIntegral(x dval, mode bid.Mode) dval {
	if x.kind != bid.KindFinite {
		if x.kind == bid.KindNaN {
			return nanVal()
		}
		return x
	}
	e := x.exp
	if e < 0 {
		e = 0
	}
	return w.wantQuantize(x, e, mode)
}

// wantRem is the exact IEEE remainder (nearest) or truncated modulo of x
// and y, with exponent min(Q(x), Q(y)) and the sign of x.
func (w *dwidth) wantRem(x, y dval, nearest bool) dval {
	switch {
	case x.kind == bid.KindNaN || y.kind == bid.KindNaN || x.kind == bid.KindInf:
		return nanVal()
	case y.kind == bid.KindInf:
		return x
	case y.isZero():
		return nanVal()
	}
	e := min(x.exp, y.exp)
	X, Y := scaleTo(x, e), scaleTo(y, e)
	mode := bid.TowardZero
	if nearest {
		mode = bid.ToNearestEven
	}
	n := roundQuo(X, Y, false, mode)
	r := new(big.Int).Sub(X, new(big.Int).Mul(n, Y))
	neg := x.neg
	if r.Sign() < 0 {
		// Only the nearest form goes below zero in magnitude terms: the
		// remainder has the opposite sign to the unsigned difference.
		r.Neg(r)
		neg = !neg
	}
	return dval{kind: bid.KindFinite, neg: neg, coeff: r, exp: e}
}

// wantFMA is the exact x*y+z rounded once to nearest even. ok is false
// when the result's exponent would be clamped or the result is subnormal,
// which this oracle does not model.
func (w *dwidth) wantFMA(x, y, z dval) (got dval, ok bool) {
	if x.kind == bid.KindNaN || y.kind == bid.KindNaN || z.kind == bid.KindNaN {
		return nanVal(), true
	}
	pneg := x.neg != y.neg
	if x.kind == bid.KindInf || y.kind == bid.KindInf {
		if x.isZero() || y.isZero() {
			return nanVal(), true
		}
		if z.kind == bid.KindInf && z.neg != pneg {
			return nanVal(), true
		}
		return dval{kind: bid.KindInf, neg: pneg}, true
	}
	if z.kind == bid.KindInf {
		return z, true
	}
	e := min(x.exp+y.exp, z.exp)
	if e < w.minExp {
		return dval{}, false
	}
	P := new(big.Int).Mul(x.coeff, y.coeff)
	P.Mul(P, pow10(x.exp+y.exp-e))
	Z := scaleTo(z, e)
	sum := new(big.Int).Set(P)
	if pneg {
		sum.Neg(P)
	}
	if z.neg {
		sum.Sub(sum, Z)
	} else {
		sum.Add(sum, Z)
	}
	if sum.Sign() == 0 {
		// An exact zero has the sign of both addends when they agree and
		// is positive otherwise, in round-to-nearest.
		return dval{kind: bid.KindFinite, neg: pneg && z.neg, coeff: new(big.Int), exp: e}, e <= w.maxExp
	}
	neg := sum.Sign() < 0
	sum.Abs(sum)
	for sum.Cmp(pow10(w.prec)) >= 0 {
		// Drop the fewest digits that make the coefficient fit; rounding
		// the dropped part once is correct because the drop is one step
		// on the exact value, repeated from the exact value each time.
		digits := len(sum.String())
		k := digits - w.prec
		sum = roundQuo(sum, pow10(k), neg, bid.ToNearestEven)
		e += k
	}
	if e > w.maxExp || e < w.minExp {
		return dval{}, false
	}
	return dval{kind: bid.KindFinite, neg: neg, coeff: sum, exp: e}, true
}

func sameDval(a, b dval) bool {
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case bid.KindNaN:
		return true
	case bid.KindInf:
		return a.neg == b.neg
	}
	return a.neg == b.neg && a.exp == b.exp && a.coeff.Cmp(b.coeff) == 0
}

// check reports an error unless got reads as want: the same sign, coefficient
// and exponent for a finite value, the same sign for an infinity, and any
// NaN for a NaN.
func check(t *testing.T, w *dwidth, what string, got bid.Uint128, want dval) {
	t.Helper()
	if g := w.unpack(got); !sameDval(g, want) {
		t.Errorf("decimal%s %s = %v, want %v", w.name, what, g, want)
	}
}

func TestDecimalHelpersAgreeWithBig(t *testing.T) {
	for _, w := range widthTable() {
		type operand struct {
			v dval
			x bid.Uint128
		}
		var ops []operand
		for _, s := range operands {
			v := parseDval(t, s)
			if x, ok := w.pack(v); ok {
				ops = append(ops, operand{v, x})
			}
		}
		var nrem, nquant, nrti, nfma, nskip int
		for _, a := range ops {
			for name, f := range w.rti {
				check(t, w, fmt.Sprintf("%s(%v)", name, a.v), f(a.x), w.wantRoundToIntegral(a.v, rtiModeOf[name]))
				nrti++
			}
			for _, b := range ops {
				check(t, w, fmt.Sprintf("Mod(%v, %v)", a.v, b.v), w.mod(a.x, b.x), w.wantRem(a.v, b.v, false))
				check(t, w, fmt.Sprintf("Remainder(%v, %v)", a.v, b.v), w.rem(a.x, b.x), w.wantRem(a.v, b.v, true))
				check(t, w, fmt.Sprintf("Quantize(%v, %v)", a.v, b.v), w.quant(a.x, b.x), w.wantQuantizeOp(a.v, b.v))
				wantSame := a.v.kind == b.v.kind && (a.v.kind != bid.KindFinite || a.v.exp == b.v.exp)
				if got := w.same(a.x, b.x); got != wantSame {
					t.Errorf("decimal%s SameQuantum(%v, %v) = %v, want %v", w.name, a.v, b.v, got, wantSame)
				}
				nrem += 2
				nquant += 2
			}
		}
		for i := 0; i < len(ops); i += 2 {
			for j := 0; j < len(ops); j += 3 {
				for k := 0; k < len(ops); k++ {
					x, y, z := ops[i], ops[j], ops[k]
					want, ok := w.wantFMA(x.v, y.v, z.v)
					if !ok {
						nskip++
						continue
					}
					check(t, w, fmt.Sprintf("FMA(%v, %v, %v)", x.v, y.v, z.v), w.fma(x.x, y.x, z.x), want)
					nfma++
				}
			}
		}
		t.Logf("decimal%s: %d remainders, %d quantizes, %d roundings, %d FMAs checked (%d FMAs outside the oracle's range)",
			w.name, nrem, nquant, nrti, nfma, nskip)
		if nfma < 1000 {
			t.Errorf("decimal%s: only %d FMA cases in the oracle's range", w.name, nfma)
		}
	}
}

// TestDecimalHardCases pins the cases that the old implementation got
// wrong, with values fixed by hand and cross-checked in the test above.
func TestDecimalHardCases(t *testing.T) {
	for _, w := range widthTable() {
		get := func(s string) bid.Uint128 {
			x, ok := w.pack(parseDval(t, s))
			if !ok {
				t.Fatalf("decimal%s cannot hold %s", w.name, s)
			}
			return x
		}
		// Cases whose answer depends on the width: a tie that carries into
		// a new digit, and a quantize whose coefficient has one digit too
		// many.
		widthCases := map[string][]struct{ op, x, y, z, want string }{
			"64": {
				{"FMA", "9999999999999999", "10", "5", "1000000000000000E+2"},
				{"Quantize", "1E+16", "1", "", "NaN"},
			},
			"128": {
				{"FMA", "9999999999999999999999999999999999", "10", "5", "1000000000000000000000000000000000E+2"},
				{"Quantize", "1E+34", "1", "", "NaN"},
			},
		}
		for _, c := range append(widthCases[w.name], []struct{ op, x, y, z, want string }{
			// The audit's quotients beyond the digits of the format.
			{"Mod", "1E+17", "3", "", "1"},
			{"Mod", "1E+20", "3", "", "1"},
			{"Mod", "1E+30", "7", "", "1"},
			{"Mod", "-1E+30", "7", "", "-1"},
			{"Mod", "-6", "3", "", "-0"},
			{"Mod", "5.5", "2", "", "1.5"},
			{"Remainder", "1E+30", "7", "", "1"},
			{"Remainder", "1E+17", "6", "", "-2"},
			{"Remainder", "5", "2", "", "1"},
			{"Remainder", "7", "2", "", "-1"},
			{"Remainder", "-7", "2", "", "1"},
			{"Remainder", "2.5", "1", "", "0.5"},
			{"Remainder", "3.5", "1", "", "-0.5"},
			{"Remainder", "3.6", "1.3", "", "-0.3"},
			// A product of 32 digits that cancels against z to 1: two
			// roundings give 0.
			{"FMA", "9999999999999999", "9999999999999999", "-9999999999999998E+16", "1"},
			{"FMA", "1.000000000000001", "1.000000000000001", "-1.000000000000002", "1E-30"},
			// A tie that carries into a new digit.
			{"FMA", "2", "3", "-6", "0"},
			{"FMA", "-0", "5", "-0", "-0"},
			{"Quantize", "2.17", "0.001", "", "2.170"},
			{"Quantize", "2.25", "0.1", "", "2.2"},
			{"Quantize", "2.35", "0.1", "", "2.4"},
			{"Quantize", "-0.1", "1", "", "-0"},
			{"Quantize", "1E+15", "1", "", "1000000000000000"},
			{"Quantize", "Inf", "-Inf", "", "Inf"},
			{"Quantize", "Inf", "1", "", "NaN"},
			{"Trunc", "-0.5", "", "", "-0"},
			{"Floor", "-0.5", "", "", "-1"},
			{"Floor", "0.5", "", "", "0"},
			{"Ceil", "-0.5", "", "", "-0"},
			{"Ceil", "0.5", "", "", "1"},
			{"Round", "2.5", "", "", "3"},
			{"Round", "-2.5", "", "", "-3"},
			{"Round", "-0.4", "", "", "-0"},
			{"RoundToEven", "2.5", "", "", "2"},
			{"RoundToEven", "3.5", "", "", "4"},
			{"RoundToEven", "-0.5", "", "", "-0"},
			{"RoundToEven", "100.0", "", "", "100"},
		}...) {
			var got bid.Uint128
			switch c.op {
			case "Mod":
				got = w.mod(get(c.x), get(c.y))
			case "Remainder":
				got = w.rem(get(c.x), get(c.y))
			case "FMA":
				got = w.fma(get(c.x), get(c.y), get(c.z))
			case "Quantize":
				got = w.quant(get(c.x), get(c.y))
			default:
				got = w.rti[c.op](get(c.x))
			}
			check(t, w, fmt.Sprintf("%s(%s, %s, %s)", c.op, c.x, c.y, c.z), got, parseDval(t, c.want))
		}
	}
}
