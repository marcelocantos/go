// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package decimal_test

import (
	"flag"
	"fmt"
	"internal/bid"
	"math/big"
	"math/decimal"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
)

// These tests check Sqrt, Exp, Log, Log10, Pow, Pown, Compound, Rootn,
// ScaleB, LogB, NextUp and NextDown against a reference that shares
// nothing with their implementation.
//
// The reference computes each function in math/big floating point at
// refPrec bits, by methods the implementation does not use (square roots
// down to 1 for the logarithm, halving and squaring from 2^-100 for the
// exponential, no stored constants), and rounds the value to the format
// with integer arithmetic. It decides whether a result is exact from the
// value alone: a result that agrees with a number of 60 digits to more
// than 400 places is that number. The implementation decides the same thing from
// the factors of the operands, so the two can only agree by both being
// right.

var elemCases = flag.Int("decimal.cases", 0, "cases per elementary function and width (default 400, or 60 with -short)")

func caseCount() int {
	switch {
	case *elemCases > 0:
		return *elemCases
	case testing.Short():
		return 60
	}
	return 400
}

const refPrec = 1100

func newf() *big.Float { return new(big.Float).SetPrec(refPrec) }

func fint(n int64) *big.Float { return newf().SetInt64(n) }

// ffrom returns the finite v as a float; it is exact to refPrec bits.
func ffrom(v dval) *big.Float {
	r := new(big.Rat).SetInt(v.coeff)
	if v.exp >= 0 {
		r.Mul(r, new(big.Rat).SetInt(pow10(v.exp)))
	} else {
		r.Quo(r, new(big.Rat).SetInt(pow10(-v.exp)))
	}
	f := newf().SetRat(r)
	if v.neg {
		f.Neg(f)
	}
	return f
}

// small reports whether |x| < 2^e.
func small(x *big.Float, e int) bool {
	return x.Sign() == 0 || x.MantExp(nil) <= e
}

// refSlack is the number of bits by which a reference value may be wrong:
// the methods here lose about 120 of refPrec.
const refSlack = 250

// refExp returns e^t. tiny is the sign of t if e^t differs from 1 by less
// than the reference can tell from an error of its own, and the result
// is infinite or zero beyond e^±40000.
func refExp(t *big.Float) (v *big.Float, tiny int) {
	switch {
	case small(t, -(refPrec - refSlack - 50)):
		return fint(1), t.Sign()
	case !small(t, 16):
		if t.Sign() < 0 {
			return newf(), 0
		}
		return newf().SetInf(false), 0
	}
	m := 100 + max(t.MantExp(nil), 0)
	r := newf().SetMantExp(t, -m)
	sum, term := fint(1), fint(1)
	for i := int64(1); ; i++ {
		term.Mul(term, r)
		term.Quo(term, fint(i))
		if small(term, -(refPrec + 20)) {
			break
		}
		sum.Add(sum, term)
	}
	for ; m > 0; m-- {
		sum.Mul(sum, sum)
	}
	return sum, 0
}

// refLog returns ln x for a positive x.
func refLog(x *big.Float) *big.Float {
	one := fint(1)
	y, k := newf().Set(x), 0
	for !small(newf().Sub(y, one), -30) {
		y.Sqrt(y)
		k++
	}
	return newf().SetMantExp(refAtanh2(newf().Quo(newf().Sub(y, one), newf().Add(y, one))), k)
}

// refAtanh2 returns 2 atanh(z) for a small z.
func refAtanh2(z *big.Float) *big.Float {
	if z.Sign() == 0 {
		return newf()
	}
	z2 := newf().Mul(z, z)
	sum, term := newf(), newf().Set(z)
	for i := int64(1); ; i += 2 {
		sum.Add(sum, newf().Quo(term, fint(i)))
		term.Mul(term, z2)
		if small(newf().Quo(term, z), -(refPrec + 20)) {
			break
		}
	}
	return sum.Mul(sum, fint(2))
}

// refLog1p returns ln(1+x) for the finite x > -1.
func refLog1p(x dval) *big.Float {
	f := ffrom(x)
	if small(f, -64) {
		return refAtanh2(newf().Quo(f, newf().Add(fint(2), f)))
	}
	// 1+x exactly, as a rational: the sum in floating point would drop
	// digits of a small x.
	r := new(big.Rat).SetInt(x.coeff)
	if x.exp >= 0 {
		r.Mul(r, new(big.Rat).SetInt(pow10(x.exp)))
	} else {
		r.Quo(r, new(big.Rat).SetInt(pow10(-x.exp)))
	}
	if x.neg {
		r.Neg(r)
	}
	r.Add(r, big.NewRat(1, 1))
	return refLog(newf().SetRat(r))
}

// A dfmt is the part of a dwidth the reference rounds to.
type dfmt struct{ prec, minExp, maxExp int }

// roundRat returns the positive rational r rounded to the format, to
// nearest with ties to even, with the least exponent the format allows.
func (w dfmt) roundRat(r *big.Rat, neg bool) dval {
	n, d := r.Num(), r.Denom()
	// e is the exponent at which r has prec digits, or one more.
	e := len(n.String()) - len(d.String()) - w.prec
	for {
		e = max(e, w.minExp)
		var c *big.Int
		if e >= 0 {
			c = roundQuo(n, new(big.Int).Mul(d, pow10(e)), neg, bid.ToNearestEven)
		} else {
			c = roundQuo(new(big.Int).Mul(n, pow10(-e)), d, neg, bid.ToNearestEven)
		}
		if c.Cmp(pow10(w.prec)) >= 0 {
			e++ // a digit too many: round again from the exact value
			continue
		}
		if e > w.maxExp {
			return dval{kind: bid.KindInf, neg: neg}
		}
		return dval{neg: neg, coeff: c, exp: e}
	}
}

// snap returns the decimal of at most 60 digits that v equals, if it
// agrees with one to all the bits the reference vouches for: its digits
// without trailing zeros and its exponent.
func snap(v *big.Float) (c *big.Int, e int, ok bool) {
	s := v.Text('e', 59) // d.ddd...de±xx
	mant, expStr, _ := strings.Cut(s, "e")
	var x int
	fmt.Sscanf(expStr, "%d", &x)
	digits := strings.TrimRight(strings.Replace(mant, ".", "", 1), "0")
	c, _ = new(big.Int).SetString(digits, 10)
	e = x - (len(digits) - 1)
	back := ffrom(dval{coeff: c, exp: e})
	diff := newf().Sub(v, back)
	if diff.Sign() == 0 {
		return c, e, true
	}
	return c, e, diff.MantExp(nil)-v.MantExp(nil) < -(refPrec - refSlack)
}

// finish returns the result of a function whose value is the positive v,
// with the sign neg. If the value is exact its exponent is the nearest
// to pref that holds it.
func (w dfmt) finish(v *big.Float, tiny int, neg bool, pref int) dval {
	switch {
	case tiny != 0:
		// 1 and a bit, or 1 less a bit: 1.000...0 either way.
		return dval{neg: neg, coeff: pow10(w.prec - 1), exp: -(w.prec - 1)}
	case v.IsInf():
		return dval{kind: bid.KindInf, neg: neg}
	case v.Sign() == 0:
		return dval{neg: neg, coeff: new(big.Int), exp: w.minExp} // underflow to zero
	}
	if c, e, ok := snap(v); ok {
		if d := len(c.String()); d <= w.prec && e >= w.minExp && e-(w.prec-d) <= w.maxExp {
			if e > pref {
				k := min(e-pref, w.prec-d, e-w.minExp)
				c, e = new(big.Int).Mul(c, pow10(k)), e-k
			}
			if e > w.maxExp {
				c, e = new(big.Int).Mul(c, pow10(e-w.maxExp)), w.maxExp
			}
			return dval{neg: neg, coeff: c, exp: e}
		}
		// Exact but too long, or out of range: round the exact value.
		r := new(big.Rat).SetInt(c)
		if e >= 0 {
			r.Mul(r, new(big.Rat).SetInt(pow10(e)))
		} else {
			r.Quo(r, new(big.Rat).SetInt(pow10(-e)))
		}
		return w.roundRat(r, neg)
	}
	r, _ := v.Rat(nil)
	return w.roundRat(r, neg)
}

func inf(neg bool) dval { return dval{kind: bid.KindInf, neg: neg} }

func zero(neg bool, e int) dval { return dval{neg: neg, coeff: new(big.Int), exp: e} }

func (w dfmt) one() dval { return dval{coeff: big.NewInt(1)} }

// clampZero returns a zero with the exponent nearest e the format has.
func (w dfmt) clampZero(neg bool, e int) dval {
	return zero(neg, min(max(e, w.minExp), w.maxExp))
}

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// isInt reports whether the finite v is an integer, and whether it is
// odd.
func isInt(v dval) (integer, odd bool) {
	if v.coeff.Sign() == 0 {
		return true, false
	}
	if v.exp >= 0 {
		return true, v.exp == 0 && v.coeff.Bit(0) == 1
	}
	q, r := new(big.Int).QuoRem(v.coeff, pow10(-v.exp), new(big.Int))
	return r.Sign() == 0, r.Sign() == 0 && q.Bit(0) == 1
}

// cmpOne compares |v| with 1 for a finite v.
func cmpOne(v dval) int {
	f := ffrom(v)
	return f.Abs(f).Cmp(fint(1))
}

// The reference for each function. A NaN operand gives a NaN unless the
// function says otherwise.

func (w dfmt) wantSqrt(x dval) dval {
	switch {
	case x.kind == bid.KindNaN:
		return nanVal()
	case x.isZero():
		return w.clampZero(x.neg, floorDiv(x.exp, 2))
	case x.neg:
		return nanVal()
	case x.kind == bid.KindInf:
		return x
	}
	return w.finish(newf().Sqrt(ffrom(x)), 0, false, floorDiv(x.exp, 2))
}

func (w dfmt) wantExp(x dval) dval {
	switch {
	case x.kind == bid.KindNaN:
		return nanVal()
	case x.kind == bid.KindInf:
		if x.neg {
			return zero(false, 0)
		}
		return x
	case x.isZero():
		return w.one()
	}
	v, tiny := refExp(ffrom(x))
	return w.finish(v, tiny, false, 0)
}

func (w dfmt) wantLog(x dval, base10 bool) dval {
	switch {
	case x.kind == bid.KindNaN:
		return nanVal()
	case x.isZero():
		return inf(true)
	case x.neg:
		return nanVal()
	case x.kind == bid.KindInf:
		return x
	}
	if cmpOne(x) == 0 {
		return zero(false, 0)
	}
	v := refLog(ffrom(x))
	if base10 {
		v.Quo(v, refLog(fint(10)))
	}
	neg := v.Sign() < 0
	return w.finish(v.Abs(v), 0, neg, 0)
}

// power returns |x|^k for a real exponent k given as its logarithm
// multiplier: e^(k × ln|x|).
func (w dfmt) power(lnx *big.Float, k *big.Float, neg bool, pref int) dval {
	v, tiny := refExp(newf().Mul(k, lnx))
	return w.finish(v, tiny, neg, pref)
}

func (w dfmt) wantPown(x dval, n int) dval {
	neg := x.neg && n&1 != 0
	switch {
	case n == 0:
		return w.one()
	case x.kind == bid.KindNaN:
		return nanVal()
	case x.isZero():
		if n < 0 {
			return inf(neg)
		}
		return w.clampZero(neg, x.exp*n)
	case x.kind == bid.KindInf:
		if n < 0 {
			return zero(neg, 0)
		}
		return inf(neg)
	}
	if cmpOne(x) == 0 {
		return w.finish(fint(1), 0, neg, x.exp*n)
	}
	f := ffrom(x)
	return w.power(refLog(f.Abs(f)), fint(int64(n)), neg, x.exp*n)
}

func (w dfmt) wantCompound(x dval, n int) dval {
	switch {
	case x.kind == bid.KindNaN:
		if n == 0 {
			return w.one()
		}
		return nanVal()
	case x.kind == bid.KindInf:
		switch {
		case x.neg:
			return nanVal()
		case n == 0:
			return w.one()
		case n < 0:
			return zero(false, 0)
		}
		return x
	case x.neg && cmpOne(x) > 0:
		return nanVal()
	case n == 0 || x.isZero():
		return w.one()
	case x.neg && cmpOne(x) == 0:
		if n < 0 {
			return inf(false)
		}
		return zero(false, 0)
	}
	if !x.neg && x.exp+len(x.coeff.String()) > 150 {
		// x is so large that 1+x is x to most or all of the bits of the
		// reference, which would then take (1+x)^n for x^n. It is x^n and
		// a little more, or a little less for a negative n: move the
		// value that way by more than the reference's own error, so
		// that it rounds as the true one does.
		v, tiny := refExp(newf().Mul(fint(int64(n)), refLog(ffrom(x))))
		nudge := newf().SetMantExp(fint(1), -(refPrec - refSlack - 40))
		if n < 0 {
			nudge.Neg(nudge)
		}
		return w.finish(v.Mul(v, nudge.Add(nudge, fint(1))), tiny, false, 0)
	}
	return w.power(refLog1p(x), fint(int64(n)), false, n*min(x.exp, 0))
}

func (w dfmt) wantRootn(x dval, n int) dval {
	neg := x.neg && n&1 != 0
	switch {
	case x.kind == bid.KindNaN || n == 0:
		return nanVal()
	case x.isZero():
		if n < 0 {
			return inf(neg)
		}
		return w.clampZero(neg, floorDiv(x.exp, n))
	case x.neg && n&1 == 0:
		return nanVal()
	case x.kind == bid.KindInf:
		if n < 0 {
			return zero(neg, 0)
		}
		return inf(neg)
	}
	pref := floorDiv(x.exp, n)
	if cmpOne(x) == 0 {
		return w.finish(fint(1), 0, neg, pref)
	}
	f := ffrom(x)
	return w.power(refLog(f.Abs(f)), newf().Quo(fint(1), fint(int64(n))), neg, pref)
}

func (w dfmt) wantPow(x, y dval) dval {
	xOne := x.kind == bid.KindFinite && !x.isZero() && cmpOne(x) == 0
	switch {
	case y.isZero():
		return w.one()
	case xOne && !x.neg && y.kind != bid.KindFinite:
		return w.one()
	case x.kind == bid.KindNaN || y.kind == bid.KindNaN:
		return nanVal()
	}
	yInt, yOdd := false, false
	if y.kind == bid.KindFinite {
		yInt, yOdd = isInt(y)
	}
	neg := x.neg && yOdd
	switch {
	case x.isZero():
		if y.neg {
			return inf(neg)
		}
		return zero(neg, 0)
	case y.kind == bid.KindInf:
		switch {
		case xOne:
			return w.one()
		case (x.kind == bid.KindFinite && cmpOne(x) < 0) == y.neg:
			return inf(false)
		}
		return zero(false, 0)
	case x.kind == bid.KindInf:
		if y.neg {
			return zero(neg, 0)
		}
		return inf(neg)
	case x.neg && !yInt:
		return nanVal()
	}
	// floor(y × Q(x)), the preferred exponent of an exact power.
	p := new(big.Int).Mul(y.coeff, big.NewInt(int64(x.exp)))
	if y.neg {
		p.Neg(p)
	}
	if y.exp >= 0 {
		p.Mul(p, pow10(y.exp))
	} else {
		p.Div(p, pow10(-y.exp)) // Euclidean, which floors for a positive divisor
	}
	pref := 1 << 30
	if p.IsInt64() && p.Int64() > -1<<30 && p.Int64() < 1<<30 {
		pref = int(p.Int64())
	} else if p.Sign() < 0 {
		pref = -pref
	}
	if xOne {
		return w.finish(fint(1), 0, neg, pref)
	}
	f := ffrom(x)
	return w.power(refLog(f.Abs(f)), ffrom(y), neg, pref)
}

func (w dfmt) wantScaleB(x dval, n int) dval {
	if x.kind != bid.KindFinite {
		return x
	}
	if x.isZero() {
		return w.clampZero(x.neg, x.exp+n)
	}
	r := new(big.Rat).SetInt(x.coeff)
	e := x.exp + n
	// The exact value, at its own exponent if the format has it.
	d := len(x.coeff.String())
	if e >= w.minExp && e <= w.maxExp {
		return dval{neg: x.neg, coeff: x.coeff, exp: e}
	}
	if e > w.maxExp && e-(w.prec-d) <= w.maxExp {
		return dval{neg: x.neg, coeff: new(big.Int).Mul(x.coeff, pow10(e-w.maxExp)), exp: w.maxExp}
	}
	if e > w.maxExp {
		return inf(x.neg)
	}
	r.Quo(r, new(big.Rat).SetInt(pow10(-e)))
	return w.roundRat(r, x.neg)
}

func (w dfmt) wantLogB(x dval) dval {
	switch {
	case x.kind == bid.KindNaN:
		return nanVal()
	case x.kind == bid.KindInf:
		return inf(false)
	case x.isZero():
		return inf(true)
	}
	l := len(x.coeff.String()) - 1 + x.exp
	return dval{neg: l < 0, coeff: new(big.Int).Abs(big.NewInt(int64(l)))}
}

func (w dfmt) wantNextUp(x dval) dval {
	largest := dval{coeff: new(big.Int).Sub(pow10(w.prec), big.NewInt(1)), exp: w.maxExp}
	switch {
	case x.kind == bid.KindNaN:
		return nanVal()
	case x.kind == bid.KindInf:
		if x.neg {
			largest.neg = true
			return largest
		}
		return x
	case x.isZero():
		return dval{coeff: big.NewInt(1), exp: w.minExp}
	}
	// One unit of the last place the format can hold beside x, added to
	// x exactly.
	e := max(x.exp-(w.prec-len(x.coeff.String())), w.minExp)
	c := scaleTo(x, e)
	if x.neg {
		c.Sub(c, big.NewInt(1))
		if e > w.minExp && c.Cmp(pow10(w.prec-1)) < 0 {
			c.Mul(c, big.NewInt(10)).Add(c, big.NewInt(9))
			e--
		}
	} else {
		c.Add(c, big.NewInt(1))
		if c.Cmp(pow10(w.prec)) == 0 {
			c, e = pow10(w.prec-1), e+1
		}
		if e > w.maxExp {
			return inf(false)
		}
	}
	return dval{neg: x.neg, coeff: c, exp: e}
}

func (w dfmt) wantNextDown(x dval) dval {
	x.neg = !x.neg
	r := w.wantNextUp(x)
	if r.kind != bid.KindNaN {
		r.neg = !r.neg
	}
	return r
}

// An elem holds the functions under test at one width, on bit patterns.
type elem struct {
	w     *dwidth
	unary map[string]func(bid.Uint128) bid.Uint128
	count map[string]func(bid.Uint128, int) bid.Uint128
	pow   func(x, y bid.Uint128) bid.Uint128
}

func elems() []*elem {
	ws := widthTable()
	f64 := func(x bid.Uint128) decimal64 { return decimal.BID64frombits(x.Lo) }
	t64 := func(x decimal64) bid.Uint128 { return u64(decimal.BID64bits(x)) }
	f128 := func(x bid.Uint128) decimal128 { return decimal.BID128frombits(x.Hi, x.Lo) }
	t128 := func(x decimal128) bid.Uint128 {
		hi, lo := decimal.BID128bits(x)
		return bid.Uint128{Hi: hi, Lo: lo}
	}
	un64 := func(f func(decimal64) decimal64) func(bid.Uint128) bid.Uint128 {
		return func(x bid.Uint128) bid.Uint128 { return t64(f(f64(x))) }
	}
	cn64 := func(f func(decimal64, int) decimal64) func(bid.Uint128, int) bid.Uint128 {
		return func(x bid.Uint128, n int) bid.Uint128 { return t64(f(f64(x), n)) }
	}
	un128 := func(f func(decimal128) decimal128) func(bid.Uint128) bid.Uint128 {
		return func(x bid.Uint128) bid.Uint128 { return t128(f(f128(x))) }
	}
	cn128 := func(f func(decimal128, int) decimal128) func(bid.Uint128, int) bid.Uint128 {
		return func(x bid.Uint128, n int) bid.Uint128 { return t128(f(f128(x), n)) }
	}
	return []*elem{
		{
			w: ws[0],
			unary: map[string]func(bid.Uint128) bid.Uint128{
				"Sqrt": un64(decimal.Sqrt[decimal64]), "Exp": un64(decimal.Exp[decimal64]),
				"Log": un64(decimal.Log[decimal64]), "Log10": un64(decimal.Log10[decimal64]),
				"LogB": un64(decimal.LogB[decimal64]), "NextUp": un64(decimal.NextUp[decimal64]),
				"NextDown": un64(decimal.NextDown[decimal64]),
			},
			count: map[string]func(bid.Uint128, int) bid.Uint128{
				"Pown": cn64(decimal.Pown[decimal64]), "Compound": cn64(decimal.Compound[decimal64]),
				"Rootn": cn64(decimal.Rootn[decimal64]), "ScaleB": cn64(decimal.ScaleB[decimal64]),
			},
			pow: func(x, y bid.Uint128) bid.Uint128 { return t64(decimal.Pow(f64(x), f64(y))) },
		},
		{
			w: ws[1],
			unary: map[string]func(bid.Uint128) bid.Uint128{
				"Sqrt": un128(decimal.Sqrt[decimal128]), "Exp": un128(decimal.Exp[decimal128]),
				"Log": un128(decimal.Log[decimal128]), "Log10": un128(decimal.Log10[decimal128]),
				"LogB": un128(decimal.LogB[decimal128]), "NextUp": un128(decimal.NextUp[decimal128]),
				"NextDown": un128(decimal.NextDown[decimal128]),
			},
			count: map[string]func(bid.Uint128, int) bid.Uint128{
				"Pown": cn128(decimal.Pown[decimal128]), "Compound": cn128(decimal.Compound[decimal128]),
				"Rootn": cn128(decimal.Rootn[decimal128]), "ScaleB": cn128(decimal.ScaleB[decimal128]),
			},
			pow: func(x, y bid.Uint128) bid.Uint128 { return t128(decimal.Pow(f128(x), f128(y))) },
		},
	}
}

func (el *elem) fmt() dfmt { return dfmt{el.w.prec, el.w.minExp, el.w.maxExp} }

func (el *elem) wantUnary(op string, x dval) dval {
	w := el.fmt()
	switch op {
	case "Sqrt":
		return w.wantSqrt(x)
	case "Exp":
		return w.wantExp(x)
	case "Log":
		return w.wantLog(x, false)
	case "Log10":
		return w.wantLog(x, true)
	case "LogB":
		return w.wantLogB(x)
	case "NextUp":
		return w.wantNextUp(x)
	case "NextDown":
		return w.wantNextDown(x)
	}
	panic(op)
}

func (el *elem) wantCount(op string, x dval, n int) dval {
	w := el.fmt()
	switch op {
	case "Pown":
		return w.wantPown(x, n)
	case "Compound":
		return w.wantCompound(x, n)
	case "Rootn":
		return w.wantRootn(x, n)
	case "ScaleB":
		return w.wantScaleB(x, n)
	}
	panic(op)
}

func (el *elem) checkUnary(t *testing.T, op string, x dval) {
	t.Helper()
	b, ok := el.w.pack(x)
	if !ok {
		t.Fatalf("decimal%s: cannot pack %v", el.w.name, x)
	}
	check(t, el.w, fmt.Sprintf("%s(%v)", op, x), el.unary[op](b), el.wantUnary(op, x))
}

func (el *elem) checkCount(t *testing.T, op string, x dval, n int) {
	t.Helper()
	b, ok := el.w.pack(x)
	if !ok {
		t.Fatalf("decimal%s: cannot pack %v", el.w.name, x)
	}
	check(t, el.w, fmt.Sprintf("%s(%v, %d)", op, x, n), el.count[op](b, n), el.wantCount(op, x, n))
}

func (el *elem) checkPow(t *testing.T, x, y dval) {
	t.Helper()
	bx, ok1 := el.w.pack(x)
	by, ok2 := el.w.pack(y)
	if !ok1 || !ok2 {
		t.Fatalf("decimal%s: cannot pack %v, %v", el.w.name, x, y)
	}
	check(t, el.w, fmt.Sprintf("Pow(%v, %v)", x, y), el.pow(bx, by), el.fmt().wantPow(x, y))
}

// A gen draws operands.
type gen struct {
	*rand.Rand
	w *dwidth
}

// coeff returns a coefficient of 1 to digits digits; one in four ends in
// zeros.
func (g gen) coeff(digits int) *big.Int {
	d := 1 + g.IntN(digits)
	c := new(big.Int)
	for i := 0; i < d; i++ {
		c.Mul(c, big.NewInt(10)).Add(c, big.NewInt(int64(g.IntN(10))))
	}
	if c.Sign() == 0 {
		c.SetInt64(1)
	}
	if z := d - len(c.String()); g.IntN(4) == 0 && z < digits-d {
		c.Mul(c, pow10(g.IntN(digits-d-z)+1))
	}
	if c.Cmp(pow10(digits)) >= 0 {
		c.Quo(c, pow10(len(c.String())-digits))
	}
	return c
}

// val returns a value with an exponent in [lo, hi], limited to the
// format's.
func (g gen) val(lo, hi int, neg bool) dval {
	lo, hi = max(lo, g.w.minExp), min(hi, g.w.maxExp)
	return dval{neg: neg, coeff: g.coeff(g.w.prec), exp: lo + g.IntN(hi-lo+1)}
}

// mag returns a value of about 10^m: a coefficient of full or short
// length placed so.
func (g gen) mag(m int, neg bool) dval {
	c := g.coeff(g.w.prec)
	return dval{neg: neg, coeff: c, exp: min(max(m-len(c.String())+1, g.w.minExp), g.w.maxExp)}
}

// nearOne returns 1 ± a value of about 10^-k.
func (g gen) nearOne(k int) dval {
	k = min(k, g.w.prec-1)
	c := pow10(k)
	d := big.NewInt(int64(1 + g.IntN(9)))
	if g.IntN(2) == 0 {
		c.Add(c, d)
	} else {
		c.Sub(c, d)
	}
	return dval{coeff: c, exp: -k}
}

func TestElementaryAgainstBig(t *testing.T) {
	for _, el := range elems() {
		t.Run(el.w.name, func(t *testing.T) {
			g := gen{rand.New(rand.NewPCG(1, uint64(el.w.prec))), el.w}
			n := caseCount()
			p := el.w.prec
			for i := 0; i < n && !t.Failed(); i++ {
				// Square roots: any magnitude, and squares.
				el.checkUnary(t, "Sqrt", g.val(el.w.minExp, el.w.maxExp, false))
				s := g.coeff(p / 2)
				el.checkUnary(t, "Sqrt", dval{coeff: s.Mul(s, s), exp: g.IntN(41) - 20})
				el.checkUnary(t, "Sqrt", g.nearOne(1+g.IntN(p)))

				// Exponentials: arguments from tiny to the edge of
				// the range, which is at about ±2.3 × the exponents.
				el.checkUnary(t, "Exp", g.mag(g.IntN(8)-4, g.IntN(2) == 0))
				el.checkUnary(t, "Exp", g.mag(-g.IntN(3*p), g.IntN(2) == 0))
				if i%8 == 0 {
					el.checkUnary(t, "Exp", g.val(el.w.minExp, el.w.minExp+3*p, g.IntN(2) == 0))
				}

				// Logarithms: any magnitude, near 1, and powers of ten.
				for _, op := range []string{"Log", "Log10"} {
					el.checkUnary(t, op, g.val(el.w.minExp, el.w.maxExp, false))
					el.checkUnary(t, op, g.mag(g.IntN(7)-3, false))
					el.checkUnary(t, op, g.nearOne(1+g.IntN(p)))
				}
				el.checkUnary(t, "Log10", dval{coeff: pow10(g.IntN(p)), exp: g.IntN(601) - 300})

				// Integer powers: small bases exactly, rates, and
				// bases near 1 to large powers.
				el.checkCount(t, "Pown", dval{neg: g.IntN(2) == 0, coeff: g.coeff(3), exp: g.IntN(9) - 6}, g.IntN(41)-20)
				el.checkCount(t, "Pown", g.mag(g.IntN(5)-2, g.IntN(2) == 0), g.IntN(801)-400)
				el.checkCount(t, "Pown", g.nearOne(1+g.IntN(p)), g.IntN(200001)-100000)
				el.checkCount(t, "Pown", g.val(el.w.minExp, el.w.maxExp, false), g.IntN(7)-3)
				el.checkCount(t, "Pown", g.nearOne(1+g.IntN(p)), g.IntN(7)-3)

				// Compound interest: rates, tiny rates, losses, and
				// short sums that are exact.
				el.checkCount(t, "Compound", g.mag(-1-g.IntN(4), g.IntN(4) == 0), g.IntN(1201)-600)
				el.checkCount(t, "Compound", g.mag(-g.IntN(el.w.maxExp), g.IntN(2) == 0), g.IntN(100001)-50000)
				el.checkCount(t, "Compound", dval{neg: g.IntN(3) == 0, coeff: g.coeff(2), exp: -2}, g.IntN(31)-15)
				el.checkCount(t, "Compound", g.mag(g.IntN(60), false), g.IntN(9)-4)
				el.checkCount(t, "Compound", dval{neg: g.IntN(2) == 0, coeff: big.NewInt(int64(1 + g.IntN(9))), exp: -(1 + g.IntN(2*p))}, g.IntN(7)-3)

				// Roots: any magnitude, and exact powers.
				el.checkCount(t, "Rootn", g.val(el.w.minExp, el.w.maxExp, false), 1+g.IntN(40))
				el.checkCount(t, "Rootn", g.mag(g.IntN(9)-4, g.IntN(2) == 0), g.IntN(41)-20)
				k := 2 + g.IntN(4)
				u := g.coeff(p / k)
				el.checkCount(t, "Rootn", dval{neg: g.IntN(2) == 0, coeff: u.Exp(u, big.NewInt(int64(k)), nil), exp: k * (g.IntN(9) - 4)}, k*(1-2*g.IntN(2)))

				// Powers: a real exponent, exact roots, and integer
				// exponents written as decimals.
				el.checkPow(t, g.mag(g.IntN(9)-4, false), g.mag(g.IntN(5)-3, g.IntN(2) == 0))
				el.checkPow(t, g.val(el.w.minExp, el.w.maxExp, false), g.mag(-g.IntN(5), g.IntN(2) == 0))
				el.checkPow(t, g.nearOne(1+g.IntN(p)), g.mag(g.IntN(2*p), g.IntN(2) == 0))
				sq := g.coeff(p / 2)
				el.checkPow(t, dval{coeff: sq.Mul(sq, sq), exp: 2 * (g.IntN(9) - 4)}, dval{neg: g.IntN(2) == 0, coeff: big.NewInt(int64(5 * (1 + 2*g.IntN(3)))), exp: -1})
				el.checkPow(t, dval{neg: g.IntN(2) == 0, coeff: g.coeff(3), exp: g.IntN(5) - 3}, dval{neg: g.IntN(2) == 0, coeff: big.NewInt(int64(g.IntN(30)) * 10), exp: -1})
				el.checkPow(t, dval{coeff: pow10(g.IntN(4)), exp: g.IntN(41) - 20}, g.mag(g.IntN(3)-1, g.IntN(2) == 0))

				// The operations on the representation.
				x := g.val(el.w.minExp, el.w.maxExp, g.IntN(2) == 0)
				el.checkUnary(t, "LogB", x)
				el.checkUnary(t, "NextUp", x)
				el.checkUnary(t, "NextDown", x)
				el.checkCount(t, "ScaleB", x, g.IntN(4*el.w.maxExp)-2*el.w.maxExp)
				el.checkCount(t, "ScaleB", g.mag(g.IntN(21)-10, g.IntN(2) == 0), g.IntN(41)-20)
			}
		})
	}
}

// TestElementarySpecial checks the special cases the documentation lists
// and the edges of the formats, against the same reference.
func TestElementarySpecial(t *testing.T) {
	for _, el := range elems() {
		w := el.w
		p := w.prec
		one := big.NewInt(1)
		nines := new(big.Int).Sub(pow10(p), one)
		vals := []dval{
			nanVal(), inf(false), inf(true),
			zero(false, 0), zero(true, 0), zero(false, 5), zero(true, -7), zero(false, w.minExp), zero(false, w.maxExp),
			{coeff: one}, {neg: true, coeff: one}, {coeff: big.NewInt(100), exp: -2}, {neg: true, coeff: big.NewInt(10), exp: -1},
			{coeff: big.NewInt(2)}, {neg: true, coeff: big.NewInt(2)}, {coeff: big.NewInt(3)}, {neg: true, coeff: big.NewInt(3)},
			{coeff: big.NewInt(4)}, {coeff: big.NewInt(5), exp: -1}, {neg: true, coeff: big.NewInt(5), exp: -1},
			{coeff: big.NewInt(25), exp: -2}, {coeff: big.NewInt(15), exp: -1}, {neg: true, coeff: big.NewInt(15), exp: -1},
			{coeff: one, exp: 30}, {neg: true, coeff: big.NewInt(3), exp: 30}, {coeff: big.NewInt(3333), exp: -4},
			// The values above are also the exponents Pow is given.
			{coeff: big.NewInt(10)}, {coeff: one, exp: 1}, {coeff: one, exp: -1}, {coeff: one, exp: 3}, {coeff: big.NewInt(1000)},
			{coeff: big.NewInt(1024)}, {coeff: big.NewInt(625), exp: -4}, {coeff: big.NewInt(27)}, {neg: true, coeff: big.NewInt(27)},
			{coeff: big.NewInt(7)}, {coeff: big.NewInt(20)}, {coeff: big.NewInt(21)}, {coeff: big.NewInt(1000000)},
			{coeff: one, exp: w.minExp}, {neg: true, coeff: one, exp: w.minExp}, {coeff: big.NewInt(2), exp: w.minExp},
			{coeff: nines, exp: w.maxExp}, {neg: true, coeff: nines, exp: w.maxExp},
			{coeff: pow10(p - 1), exp: w.minExp}, {coeff: nines, exp: w.minExp}, {coeff: pow10(p - 1), exp: w.maxExp},
			{coeff: pow10(p - 1), exp: -(p - 1)}, {neg: true, coeff: pow10(p - 1), exp: -(p - 1)},
			{coeff: nines, exp: -p}, {coeff: new(big.Int).Add(pow10(p-1), one), exp: -(p - 1)},
			{coeff: one, exp: w.maxExp},
			{coeff: one, exp: -(p + 3)}, {neg: true, coeff: one, exp: -(p + 3)}, {coeff: one, exp: -300}, {neg: true, coeff: one, exp: -300},
			{coeff: big.NewInt(1023)}, {coeff: big.NewInt(3), exp: 40}, {coeff: big.NewInt(99999), exp: -5},
		}
		const powYs = 26
		counts := []int{0, 1, -1, 2, -2, 3, -3, 4, 10, -10, 12, 55, -55, 127, 128, -128, 1000, -1000, 1 << 30, -(1 << 30)}
		if strconv.IntSize == 64 {
			// Counts beyond a 32-bit int, made at run time so that the
			// file compiles where int is 32 bits.
			for _, c := range []int64{1 << 40, -(1 << 40), 1<<62 + 1, -(1<<62 + 1)} {
				counts = append(counts, int(c))
			}
		}
		for _, x := range vals {
			if _, ok := w.pack(x); !ok {
				t.Fatalf("decimal%s: cannot pack %v", w.name, x)
			}
			for op := range el.unary {
				el.checkUnary(t, op, x)
			}
			for op := range el.count {
				for _, n := range counts {
					if op == "ScaleB" && (n >= 1<<30 || n <= -(1<<30)) {
						continue // the reference adds the count to an exponent
					}
					if (n >= 1<<30 || n <= -(1<<30)) && x.kind == bid.KindFinite && x.exp != 0 {
						continue // the reference multiplies the exponent by the count
					}
					el.checkCount(t, op, x, n)
				}
			}
			for _, y := range vals[:powYs] {
				el.checkPow(t, x, y)
			}
		}
	}
}

// TestElementaryExamples checks results written out by hand, quantum
// included, so that the reference's own rules are checked too.
func TestElementaryExamples(t *testing.T) {
	type ex struct{ op, x, arg, want string }
	examples := []ex{
		{"Sqrt", "2.25", "", "1.5"}, {"Sqrt", "4.00", "", "2.0"}, {"Sqrt", "4", "", "2"}, {"Sqrt", "1E+2", "", "1E+1"},
		{"Sqrt", "0.0625", "", "0.25"}, {"Sqrt", "100", "", "10"}, {"Sqrt", "1.0", "", "1.0"}, {"Sqrt", "0.00", "", "0.0"},
		{"Exp", "0", "", "1"}, {"Exp", "0.00", "", "1"}, {"Log", "1", "", "0"}, {"Log", "1.000", "", "0"},
		{"Log10", "1000", "", "3"}, {"Log10", "0.001", "", "-3"}, {"Log10", "1E+10", "", "10"}, {"Log10", "1", "", "0"},
		{"Pown", "1.5", "2", "2.25"}, {"Pown", "2", "-2", "0.25"}, {"Pown", "10", "3", "1000"}, {"Pown", "1.0", "2", "1.00"},
		{"Pown", "2", "10", "1024"}, {"Pown", "-2", "3", "-8"}, {"Pown", "0.5", "-3", "8"}, {"Pown", "5", "-1", "0.2"},
		{"Pown", "1E+1", "2", "1E+2"}, {"Pown", "4", "-1", "0.25"}, {"Pown", "1.10", "2", "1.2100"},
		{"Pow", "2.25", "0.5", "1.5"}, {"Pow", "4", "1.5", "8"}, {"Pow", "100", "0.5", "10"}, {"Pow", "0.25", "-0.5", "2"},
		{"Pow", "32", "0.2", "2"}, {"Pow", "1E+2", "0.5", "1E+1"}, {"Pow", "1.0", "3", "1.000"}, {"Pow", "10", "2.0", "100"},
		{"Pow", "1.5", "2", "2.25"}, {"Pow", "0.001", "-0.5", "31.62277660168379331998893544432719"},
		{"Compound", "0.05", "2", "1.1025"}, {"Compound", "-0.5", "2", "0.25"}, {"Compound", "1", "10", "1024"},
		{"Compound", "0.25", "-1", "0.8"}, {"Compound", "3", "-2", "0.0625"}, {"Compound", "0.05", "1", "1.05"},
		{"Rootn", "27", "3", "3"}, {"Rootn", "-27", "3", "-3"}, {"Rootn", "1E+6", "3", "1E+2"}, {"Rootn", "0.001", "3", "0.1"},
		{"Rootn", "4", "-2", "0.5"}, {"Rootn", "16", "4", "2"}, {"Rootn", "2", "1", "2"}, {"Rootn", "1000000", "3", "100"},
		{"ScaleB", "1.50", "2", "150"}, {"ScaleB", "150", "-2", "1.50"}, {"ScaleB", "0", "5", "0E+5"},
		{"LogB", "250", "", "2"}, {"LogB", "0.025", "", "-2"}, {"LogB", "1", "", "0"}, {"LogB", "9.99", "", "0"},
	}
	for _, el := range elems() {
		for _, e := range examples {
			if el.w.prec < 34 && len(e.want) > 20 {
				continue
			}
			x := parseDval(t, e.x)
			b, _ := el.w.pack(x)
			var got bid.Uint128
			var what string
			switch {
			case e.op == "Pow":
				y := parseDval(t, e.arg)
				by, _ := el.w.pack(y)
				got, what = el.pow(b, by), fmt.Sprintf("Pow(%s, %s)", e.x, e.arg)
			case e.arg != "":
				var n int
				fmt.Sscanf(e.arg, "%d", &n)
				got, what = el.count[e.op](b, n), fmt.Sprintf("%s(%s, %d)", e.op, e.x, n)
			default:
				got, what = el.unary[e.op](b), fmt.Sprintf("%s(%s)", e.op, e.x)
			}
			check(t, el.w, what, got, parseDval(t, e.want))
		}
	}
}

// TestElementarySignaling checks that a signaling NaN is an invalid
// operand even of the functions that pass a quiet NaN over, and that a
// NaN result carries the operand's payload.
func TestElementarySignaling(t *testing.T) {
	p := decimal128(42)
	s, q := decimal.SignalingNaNWithPayload(p), decimal.NaNWithPayload(p)
	one := decimal128(1)
	for name, got := range map[string]decimal128{
		"Pown(sNaN, 0)": decimal.Pown(s, 0), "Compound(sNaN, 0)": decimal.Compound(s, 0),
		"Pow(sNaN, 0)": decimal.Pow(s, 0), "Pow(1, sNaN)": decimal.Pow(one, s),
		"Sqrt(sNaN)": decimal.Sqrt(s), "Exp(NaN)": decimal.Exp(q), "Pow(NaN, 2)": decimal.Pow(q, 2),
		"ScaleB(sNaN, 3)": decimal.ScaleB(s, 3), "Rootn(NaN, 0)": decimal.Rootn(q, 0),
	} {
		if !decimal.IsNaN(got) || decimal.Payload(got) != p {
			t.Errorf("%s = %v with payload %v, want a NaN with payload 42", name, got, decimal.Payload(got))
		}
		if hi, _ := decimal.BID128bits(got); bid.IsSignaling128(bid.Uint128{Hi: hi}) {
			t.Errorf("%s is a signaling NaN", name)
		}
	}
	for name, got := range map[string]decimal128{
		"Pown(NaN, 0)": decimal.Pown(q, 0), "Compound(NaN, 0)": decimal.Compound(q, 0),
		"Pow(NaN, 0)": decimal.Pow(q, 0), "Pow(1, NaN)": decimal.Pow(one, q),
	} {
		if got != 1 {
			t.Errorf("%s = %v, want 1", name, got)
		}
	}
}
