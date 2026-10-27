// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// The operations of the reference model. Each decodes its operands, settles
// the NaN, infinity and zero cases the contract lists, computes the exact
// result and rounds it once with format.round. The section names in the
// comments are those of the contract in the package documentation.

import (
	"internal/bid"
	"math"
	"math/big"
	"strings"
)

var nan = value{kind: notANumber}

// signed returns (-1)^neg × c × 10^(exp-at) as an integer, for at <= exp.
func signed(neg bool, c *big.Int, exp, at int) *big.Int {
	s := new(big.Int).Mul(c, pow10(exp-at))
	if neg {
		s.Neg(s)
	}
	return s
}

// sum returns x + y rounded in mode; x and y are finite, and their
// coefficients and exponents may lie outside the format (FMA passes the
// exact product). See "Sign of a result" for the zero cases.
func (f *format) sum(x, y value, mode bid.Mode) value {
	m := min(x.exp, y.exp)
	s := new(big.Int).Add(signed(x.neg, x.coeff, x.exp, m), signed(y.neg, y.coeff, y.exp, m))
	neg := s.Sign() < 0
	if s.Sign() == 0 {
		if x.neg == y.neg {
			neg = x.neg
		} else {
			neg = normMode(mode) == bid.TowardNegative
		}
	}
	r, _ := f.round(neg, s.Abs(s), bigOne, m, m, mode)
	return r
}

func (f *format) add(x, y bits, mode bid.Mode) bits {
	a, b := f.decode(x), f.decode(y)
	return f.encode(f.addValues(a, b, mode))
}

func (f *format) sub(x, y bits, mode bid.Mode) bits {
	a, b := f.decode(x), f.decode(y)
	b.neg = !b.neg
	return f.encode(f.addValues(a, b, mode))
}

func (f *format) addValues(a, b value, mode bid.Mode) value {
	switch {
	case a.kind == notANumber || b.kind == notANumber:
		return nan
	case a.kind == infinite && b.kind == infinite:
		if a.neg != b.neg {
			return nan
		}
		return a
	case a.kind == infinite:
		return a
	case b.kind == infinite:
		return b
	}
	return f.sum(a, b, mode)
}

func (f *format) mul(x, y bits, mode bid.Mode) bits {
	a, b := f.decode(x), f.decode(y)
	neg := a.neg != b.neg
	switch {
	case a.kind == notANumber || b.kind == notANumber:
		return f.encode(nan)
	case a.kind == infinite || b.kind == infinite:
		if a.kind == finite && a.coeff.Sign() == 0 || b.kind == finite && b.coeff.Sign() == 0 {
			return f.encode(nan)
		}
		return f.encode(value{kind: infinite, neg: neg})
	}
	r, _ := f.round(neg, new(big.Int).Mul(a.coeff, b.coeff), bigOne, a.exp+b.exp, a.exp+b.exp, mode)
	return f.encode(r)
}

func (f *format) div(x, y bits, mode bid.Mode) bits {
	a, b := f.decode(x), f.decode(y)
	neg := a.neg != b.neg
	switch {
	case a.kind == notANumber || b.kind == notANumber:
		return f.encode(nan)
	case a.kind == infinite && b.kind == infinite:
		return f.encode(nan)
	case a.kind == infinite:
		return f.encode(value{kind: infinite, neg: neg})
	case b.kind == infinite:
		// "Exponent of a result": x / ∞ is a zero at minExp.
		return f.encode(value{kind: finite, neg: neg, coeff: new(big.Int), exp: f.minExp})
	case b.coeff.Sign() == 0:
		if a.coeff.Sign() == 0 {
			return f.encode(nan)
		}
		return f.encode(value{kind: infinite, neg: neg})
	}
	r, _ := f.round(neg, a.coeff, b.coeff, a.exp-b.exp, a.exp-b.exp, mode)
	return f.encode(r)
}

func (f *format) fma(x, y, z bits, mode bid.Mode) bits {
	a, b, c := f.decode(x), f.decode(y), f.decode(z)
	if a.kind == notANumber || b.kind == notANumber || c.kind == notANumber {
		return f.encode(nan)
	}
	neg := a.neg != b.neg
	if a.kind == infinite || b.kind == infinite {
		if a.kind == finite && a.coeff.Sign() == 0 || b.kind == finite && b.coeff.Sign() == 0 {
			return f.encode(nan)
		}
		return f.encode(f.addValues(value{kind: infinite, neg: neg}, c, mode))
	}
	if c.kind == infinite {
		return f.encode(c)
	}
	p := value{kind: finite, neg: neg, coeff: new(big.Int).Mul(a.coeff, b.coeff), exp: a.exp + b.exp}
	return f.encode(f.sum(p, c, mode))
}

// remainder computes Rem (truncate false) or Mod (truncate true); see
// "Remainder and modulo". It is exact, so it takes no mode.
func (f *format) remainder(x, y bits, truncate bool) bits {
	a, b := f.decode(x), f.decode(y)
	switch {
	case a.kind != finite || b.kind == notANumber:
		return f.encode(nan)
	case b.kind == infinite:
		return f.encode(a)
	case b.coeff.Sign() == 0:
		return f.encode(nan)
	}
	m := min(a.exp, b.exp)
	ax := signed(false, a.coeff, a.exp, m)
	by := signed(false, b.coeff, b.exp, m)
	// With |x| / |y| rounded to the integer n (to nearest, ties to even,
	// or truncated), x - y × sign(x/y) × n = sign(x) × (|x| - n|y|).
	var n *big.Int
	if truncate {
		n = new(big.Int).Quo(ax, by)
	} else {
		n, _ = roundQuo(ax, by, false, bid.ToNearestEven)
	}
	r := new(big.Int).Sub(ax, n.Mul(n, by))
	neg := a.neg
	if r.Sign() < 0 {
		neg = !neg
	}
	v, st := f.round(neg, r.Abs(r), bigOne, m, m, bid.ToNearestEven)
	if st != bid.Exact {
		panic("remainder: inexact: model bug")
	}
	return f.encode(v)
}

func (f *format) roundToIntegral(x bits, mode bid.Mode) bits {
	a := f.decode(x)
	if a.kind != finite || a.exp >= 0 {
		return f.encode(a)
	}
	n, _ := roundQuo(a.coeff, pow10(-a.exp), a.neg, mode)
	return f.encode(value{kind: finite, neg: a.neg, coeff: n, exp: 0})
}

func (f *format) quantize(x, y bits, mode bid.Mode) bits {
	a, b := f.decode(x), f.decode(y)
	switch {
	case a.kind == notANumber || b.kind == notANumber:
		return f.encode(nan)
	case a.kind == infinite && b.kind == infinite:
		return f.encode(a)
	case a.kind == infinite || b.kind == infinite:
		return f.encode(nan)
	}
	var c *big.Int
	if a.exp >= b.exp {
		if a.coeff.Sign() != 0 && digits(a.coeff)+a.exp-b.exp > f.prec {
			return f.encode(nan)
		}
		c = new(big.Int).Mul(a.coeff, pow10(a.exp-b.exp))
	} else {
		c, _ = roundQuo(a.coeff, pow10(b.exp-a.exp), a.neg, mode)
	}
	if c.Cmp(f.maxCoeff) > 0 {
		return f.encode(nan)
	}
	return f.encode(value{kind: finite, neg: a.neg, coeff: c, exp: b.exp})
}

func (f *format) reduce(x bits) bits {
	a := f.decode(x)
	switch {
	case a.kind != finite:
		return f.encode(a)
	case a.coeff.Sign() == 0:
		return f.encode(value{kind: finite, neg: a.neg, coeff: a.coeff, exp: 0})
	}
	k := min(trailingZeros(a.coeff), f.maxExp-a.exp)
	return f.encode(value{kind: finite, neg: a.neg, coeff: new(big.Int).Quo(a.coeff, pow10(k)), exp: a.exp + k})
}

func (f *format) sameQuantum(x, y bits) bool {
	a, b := f.decode(x), f.decode(y)
	if a.kind != b.kind {
		return false
	}
	return a.kind != finite || a.exp == b.exp
}

// cmpMag compares |x| and |y| for x and y not NaN.
func cmpMag(x, y value) int {
	switch {
	case x.kind == infinite && y.kind == infinite:
		return 0
	case x.kind == infinite:
		return 1
	case y.kind == infinite:
		return -1
	}
	xz, yz := x.coeff.Sign() == 0, y.coeff.Sign() == 0
	switch {
	case xz && yz:
		return 0
	case xz:
		return -1
	case yz:
		return 1
	}
	ax, ay := digits(x.coeff)+x.exp, digits(y.coeff)+y.exp
	if ax != ay {
		return cmpInt(ax, ay)
	}
	// Equal adjusted exponents put the two exponents within prec digits
	// of each other, so aligning them is cheap.
	m := min(x.exp, y.exp)
	return signed(false, x.coeff, x.exp, m).Cmp(signed(false, y.coeff, y.exp, m))
}

// cmpValue compares x and y by value, for x and y not NaN: +0 equals -0.
func cmpValue(x, y value) int {
	sx, sy := sign(x), sign(y)
	if sx != sy {
		return cmpInt(sx, sy)
	}
	return sx * cmpMag(x, y)
}

// sign returns -1, 0 or +1 for a non-NaN value; zeros of either sign are 0.
func sign(x value) int {
	switch {
	case x.kind == finite && x.coeff.Sign() == 0:
		return 0
	case x.neg:
		return -1
	}
	return 1
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// compare is Compare: ok is false when either operand is a NaN.
func (f *format) compare(x, y bits) (int, bool) {
	a, b := f.decode(x), f.decode(y)
	if a.kind == notANumber || b.kind == notANumber {
		return 0, false
	}
	return cmpValue(a, b), true
}

// minMax is Min (wantMax false) or Max; see "Minimum and maximum".
func (f *format) minMax(x, y bits, wantMax bool) bits {
	a, b := f.decode(x), f.decode(y)
	if a.kind == notANumber || b.kind == notANumber {
		return f.encode(nan)
	}
	c := cmpValue(a, b)
	if c == 0 && a.neg != b.neg {
		// Zeros of opposite signs: -0 is the lesser.
		if a.neg == wantMax {
			return f.encode(b)
		}
		return f.encode(a)
	}
	if c == 0 || c < 0 != wantMax {
		return f.encode(a)
	}
	return f.encode(b)
}

// totalOrder is TotalOrder; see "Comparisons and ordering".
func (f *format) totalOrder(x, y bits) bool {
	a, b := f.decode(x), f.decode(y)
	if a.neg != b.neg {
		return a.neg
	}
	rank := func(v value) int { return int(v.kind) } // finite < infinite < NaN
	c := cmpInt(rank(a), rank(b))
	if c == 0 && a.kind == finite {
		c = cmpMag(a, b)
		if c == 0 {
			c = cmpInt(a.exp, b.exp)
		}
	}
	if a.neg {
		return c >= 0
	}
	return c <= 0
}

// key is Key64 or Key128; see the documentation of Key.
func (f *format) key(x bits) bid.Key {
	a := f.decode(x)
	switch {
	case a.kind == notANumber:
		return bid.Key{Kind: bid.KindNaN}
	case a.kind == infinite:
		return bid.Key{Kind: bid.KindInf, Neg: a.neg}
	case a.coeff.Sign() == 0:
		return bid.Key{Kind: bid.KindFinite}
	}
	tz := trailingZeros(a.coeff)
	c := new(big.Int).Quo(a.coeff, pow10(tz))
	return bid.Key{Coeff: toUint128(c), Exp: int32(a.exp + tz), Kind: bid.KindFinite, Neg: a.neg}
}

func toUint128(c *big.Int) bid.Uint128 {
	return bid.Uint128{
		Hi: new(big.Int).Rsh(c, 64).Uint64(),
		Lo: new(big.Int).And(c, new(big.Int).SetUint64(math.MaxUint64)).Uint64(),
	}
}

func fromUint128(u bid.Uint128) *big.Int {
	c := new(big.Int).SetUint64(u.Hi)
	c.Lsh(c, 64)
	return c.Or(c, new(big.Int).SetUint64(u.Lo))
}

// unpack is Unpack: the kind, the sign bit, and for a finite value the
// coefficient and exponent, read canonically.
func (f *format) unpack(x bits) (bid.Kind, bool, *big.Int, int) {
	a := f.decode(x)
	switch a.kind {
	case notANumber:
		return bid.KindNaN, a.neg, new(big.Int), 0
	case infinite:
		return bid.KindInf, a.neg, new(big.Int), 0
	}
	return bid.KindFinite, a.neg, a.coeff, a.exp
}

// pack is Pack: an exact encoding, or NaN and false.
func (f *format) pack(neg bool, c *big.Int, exp int) (bits, bool) {
	if c.Cmp(f.maxCoeff) > 0 || exp < f.minExp || exp > f.maxExp {
		return f.encode(nan), false
	}
	return f.encode(value{kind: finite, neg: neg, coeff: c, exp: exp}), true
}

// The sign operations are bit operations on every encoding alike; see
// "Sign operations".

func (f *format) withSign(x bits, neg bool) bits {
	if f.width == 64 {
		x.Lo = x.Lo&^(1<<63) | b2u(neg)<<63
	} else {
		x.Hi = x.Hi&^(1<<63) | b2u(neg)<<63
	}
	return x
}

func b2u(b bool) uint64 {
	if b {
		return 1
	}
	return 0
}

func (f *format) signbit(x bits) bool { return f.top(x)>>63 != 0 }

func (f *format) isSubnormal(x bits) bool {
	a := f.decode(x)
	return a.kind == finite && a.coeff.Sign() != 0 && digits(a.coeff)-1+a.exp < f.emin
}

// widen is Widen: exact, exponent preserved.
func widen(x bits) bits {
	return dec128.encode(dec64.decode(x))
}

// narrow is Narrow: rounded once with preferred exponent Q(x).
func narrow(x bits, mode bid.Mode) bits {
	a := dec128.decode(x)
	if a.kind != finite {
		return dec64.encode(a)
	}
	r, _ := dec64.round(a.neg, a.coeff, bigOne, a.exp, a.exp, mode)
	return dec64.encode(r)
}

// fromInt is Int64To*, Uint64To*: preferred exponent 0.
func (f *format) fromInt(n *big.Int, mode bid.Mode) bits {
	r, _ := f.round(n.Sign() < 0, new(big.Int).Abs(n), bigOne, 0, 0, mode)
	return f.encode(r)
}

// toInt is Int64From* (lo = MinInt64, hi = MaxInt64) and Uint64From* (lo
// = 0, hi = MaxUint64); see "Integer conversions".
func (f *format) toInt(x bits, mode bid.Mode, lo, hi *big.Int) (*big.Int, bid.Status) {
	a := f.decode(x)
	switch a.kind {
	case notANumber:
		return new(big.Int), bid.Invalid
	case infinite:
		if a.neg {
			return lo, bid.Invalid
		}
		return hi, bid.Invalid
	}
	var n *big.Int
	inexact := false
	switch {
	case a.exp < 0:
		n, inexact = roundQuo(a.coeff, pow10(-a.exp), a.neg, mode)
	case a.coeff.Sign() != 0 && digits(a.coeff)+a.exp > 40:
		n = pow10(40) // far outside both ranges; avoids huge powers
	default:
		n = new(big.Int).Mul(a.coeff, pow10(a.exp))
	}
	if a.neg {
		n = new(big.Int).Neg(n)
	}
	switch {
	case n.Cmp(lo) < 0:
		return lo, bid.Inexact | bid.Overflow
	case n.Cmp(hi) > 0:
		return hi, bid.Inexact | bid.Overflow
	case inexact:
		return n, bid.Inexact
	}
	return n, bid.Exact
}

var (
	minInt64  = big.NewInt(math.MinInt64)
	maxInt64  = big.NewInt(math.MaxInt64)
	maxUint64 = new(big.Int).SetUint64(math.MaxUint64)
	bigZero   = new(big.Int)
)

// fromFloat is Float64To* and Float32To* (a float32 widens to float64
// exactly): correctly rounded from the exact binary value, preferred
// exponent 0.
func (f *format) fromFloat(v float64, mode bid.Mode) bits {
	switch {
	case math.IsNaN(v):
		return f.encode(nan)
	case math.IsInf(v, 0):
		return f.encode(value{kind: infinite, neg: v < 0})
	case v == 0:
		return f.encode(value{kind: finite, neg: math.Signbit(v), coeff: new(big.Int), exp: 0})
	}
	q := new(big.Rat).SetFloat64(v)
	r, _ := f.round(v < 0, new(big.Int).Abs(q.Num()), q.Denom(), 0, 0, mode)
	return f.encode(r)
}

// binary describes a binary interchange format for toFloat.
type binary struct {
	prec       int // significand bits, including the implicit one
	emin, emax int // exponent range of normal values
	expBits    int
}

var (
	binary64 = binary{prec: 53, emin: -1022, emax: 1023, expBits: 11}
	binary32 = binary{prec: 24, emin: -126, emax: 127, expBits: 8}
)

// toFloat is Float64From* and Float32From*, returning the bits of the
// binary result: rounded once from the exact decimal value, with the
// binary format's overflow, gradual underflow and signed zero. A NaN is
// the quiet NaN with the sign bit clear and a zero payload.
func (f *format) toFloat(x bits, mode bid.Mode, b binary) uint64 {
	a := f.decode(x)
	width := 1 + b.expBits + (b.prec - 1) // sign, exponent, stored significand bits
	signBit := b2u(a.neg) << (width - 1)
	expMask := uint64(1)<<b.expBits - 1
	inf := signBit | expMask<<(b.prec-1)
	switch {
	case a.kind == notANumber:
		return expMask<<(b.prec-1) | 1<<(b.prec-2)
	case a.kind == infinite:
		return inf
	case a.coeff.Sign() == 0:
		return signBit
	}
	num, den := new(big.Int).Set(a.coeff), big.NewInt(1)
	if a.exp >= 0 {
		num.Mul(num, pow10(a.exp))
	} else {
		den.Set(pow10(-a.exp))
	}
	// e2 = floor(log2(num/den)) is BitLen(num) - BitLen(den) or one less.
	e2 := num.BitLen() - den.BitLen()
	if e2 >= 0 && num.Cmp(new(big.Int).Lsh(den, uint(e2))) < 0 || e2 < 0 && new(big.Int).Lsh(num, uint(-e2)).Cmp(den) < 0 {
		e2--
	}
	t := max(e2-b.prec+1, b.emin-b.prec+1)
	if t >= 0 {
		den.Lsh(den, uint(t))
	} else {
		num.Lsh(num, uint(-t))
	}
	m, _ := roundQuo(num, den, a.neg, mode)
	if m.BitLen() > b.prec {
		m.Rsh(m, 1)
		t++
	}
	if t > b.emax-b.prec+1 {
		toInf := true
		switch normMode(mode) {
		case bid.TowardZero:
			toInf = false
		case bid.TowardPositive:
			toInf = !a.neg
		case bid.TowardNegative:
			toInf = a.neg
		}
		if toInf {
			return inf
		}
		return signBit | (expMask-1)<<(b.prec-1) | (1<<(b.prec-1) - 1)
	}
	mant := m.Uint64()
	if mant < 1<<(b.prec-1) {
		return signBit | mant // subnormal or zero
	}
	biased := uint64(t + b.prec - 1 + (1<<(b.expBits-1) - 1))
	return signBit | biased<<(b.prec-1) | mant&(1<<(b.prec-1)-1)
}

// parse is Parse64 and Parse128; see "Character conversions" and the
// grammar documented at Parse64.
func (f *format) parse(s string, mode bid.Mode) (bits, bid.Status) {
	neg := false
	t := s
	if t != "" && (t[0] == '+' || t[0] == '-') {
		neg = t[0] == '-'
		t = t[1:]
	}
	switch strings.ToLower(t) {
	case "inf", "infinity":
		return f.encode(value{kind: infinite, neg: neg}), bid.Exact
	case "nan", "snan":
		return f.encode(nan), bid.Exact
	}
	i := 0
	coeff := new(big.Int)
	nd, frac := 0, 0
	point := false
	for ; i < len(t); i++ {
		c := t[i]
		switch {
		case c >= '0' && c <= '9':
			coeff.Mul(coeff, bigTen)
			coeff.Add(coeff, big.NewInt(int64(c-'0')))
			nd++
			if point {
				frac++
			}
			continue
		case c == '.' && !point:
			point = true
			continue
		}
		break
	}
	if nd == 0 {
		return f.encode(nan), bid.Syntax
	}
	exp := new(big.Int)
	if i < len(t) {
		if t[i] != 'e' && t[i] != 'E' {
			return f.encode(nan), bid.Syntax
		}
		i++
		eneg := false
		if i < len(t) && (t[i] == '+' || t[i] == '-') {
			eneg = t[i] == '-'
			i++
		}
		if i == len(t) {
			return f.encode(nan), bid.Syntax
		}
		for ; i < len(t); i++ {
			c := t[i]
			if c < '0' || c > '9' {
				return f.encode(nan), bid.Syntax
			}
			exp.Mul(exp, bigTen)
			exp.Add(exp, big.NewInt(int64(c-'0')))
		}
		if eneg {
			exp.Neg(exp)
		}
	}
	// Beyond ±10^9 every exponent part gives the same result, since the
	// numeral's digits cannot bring the value back into range.
	e := int(clamp64(exp, 1e9)) - frac
	r, st := f.round(neg, coeff, bigOne, e, e, mode)
	return f.encode(r), st
}

func clamp64(x *big.Int, limit int64) int64 {
	switch {
	case x.Cmp(big.NewInt(limit)) > 0:
		return limit
	case x.Cmp(big.NewInt(-limit)) < 0:
		return -limit
	}
	return x.Int64()
}

// format writes x in the character form of "Values, cohorts and the
// character form".
func (f *format) format(x bits) string {
	a := f.decode(x)
	switch a.kind {
	case notANumber:
		return "NaN"
	case infinite:
		if a.neg {
			return "-Infinity"
		}
		return "Infinity"
	}
	var sb strings.Builder
	if a.neg {
		sb.WriteByte('-')
	}
	c := a.coeff.String()
	adj := a.exp + len(c) - 1
	switch {
	case a.exp == 0:
		sb.WriteString(c)
	case a.exp < 0 && adj >= -6:
		if p := len(c) + a.exp; p > 0 {
			sb.WriteString(c[:p])
			sb.WriteByte('.')
			sb.WriteString(c[p:])
		} else {
			sb.WriteString("0.")
			sb.WriteString(strings.Repeat("0", -p))
			sb.WriteString(c)
		}
	default:
		sb.WriteString(c[:1])
		if len(c) > 1 {
			sb.WriteByte('.')
			sb.WriteString(c[1:])
		}
		sb.WriteByte('E')
		if adj >= 0 {
			sb.WriteByte('+')
		}
		sb.WriteString(big.NewInt(int64(adj)).String())
	}
	return sb.String()
}

// roundParts is Round64 and Round128; see "Rounding from exact parts".
// The value is (-1)^neg × (coeff + f) × 10^exp with 0 < f < 1 arbitrarily
// small when sticky is set.
func (f *format) roundParts(neg bool, coeff *big.Int, exp int, sticky bool, mode bid.Mode) (bits, bid.Status) {
	// coeff has at most 39 digits, so exponents beyond these bounds give
	// the same result as the bounds themselves: an overflow above, a value
	// below half the smallest subnormal (or an exponent that clamps) below.
	exp = clamp(exp, f.minExp-50, f.maxExp+f.prec+3)
	if !sticky {
		r, st := f.round(neg, coeff, bigOne, exp, exp, mode)
		return f.encode(r), st
	}
	// ε = 10^(min(exp, minExp) - 1) is below every rounding boundary's
	// distance from coeff × 10^exp, so adding it decides the rounding as an
	// arbitrarily small f does.
	k := exp - min(exp, f.minExp) + 1
	n := new(big.Int).Mul(coeff, pow10(k))
	n.Add(n, bigOne)
	r, st := f.round(neg, n, bigOne, exp-k, exp, mode)
	return f.encode(r), st
}
