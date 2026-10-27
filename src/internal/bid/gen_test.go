// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// Operand generators for the differential tests. Each case draws from one
// of three regimes: moderate exponents, where the strawman kernel's
// arithmetic was wrong (go-decimal-proposal audit R1 to R5); the full
// exponent range; and boundary-biased operands (all-nines and power-of-ten
// coefficients, exponents at the ends of the range and at the subnormal
// edge, special values, non-canonical encodings and exact half-way ties).

import (
	"internal/bid"
	"math"
	"math/big"
	"math/rand/v2"
	"strings"
)

type regime int

const (
	moderate regime = iota
	fullRange
	boundary
	numRegimes
)

func (r regime) String() string {
	return [...]string{"moderate", "full", "boundary"}[r]
}

type gen struct {
	r      *rand.Rand
	f      *format
	regime regime
}

// mode returns one of the five modes, and rarely a value outside them,
// which must behave as ToNearestEven.
func (g *gen) mode() bid.Mode {
	if g.r.IntN(50) == 0 {
		return bid.Mode(5 + g.r.IntN(250))
	}
	return bid.Mode(g.r.IntN(5))
}

// randDigits returns a random coefficient of exactly n digits, n >= 1.
func (g *gen) randDigits(n int) *big.Int {
	lo := pow10(n - 1)
	c := g.below(new(big.Int).Sub(pow10(n), lo))
	return c.Add(c, lo)
}

// below returns a uniformly random integer in [0, n), for n > 0.
func (g *gen) below(n *big.Int) *big.Int {
	nb := n.BitLen()
	words := (nb + 63) / 64
	for {
		x := new(big.Int)
		for range words {
			x.Lsh(x, 64)
			x.Or(x, new(big.Int).SetUint64(g.r.Uint64()))
		}
		x.Rsh(x, uint(words*64-nb))
		if x.Cmp(n) < 0 {
			return x
		}
	}
}

// coeff returns a coefficient of at most prec digits shaped by the regime.
func (g *gen) coeff() *big.Int {
	p := g.f.prec
	if g.regime == boundary {
		k := g.r.IntN(p) // 0 .. p-1
		switch g.r.IntN(8) {
		case 0:
			return big.NewInt(1)
		case 1:
			return new(big.Int).Set(pow10(k))
		case 2:
			return new(big.Int).Sub(pow10(k+1), bigOne) // all nines
		case 3:
			return new(big.Int).Set(g.f.maxCoeff)
		case 4:
			return new(big.Int).Mul(big.NewInt(5), pow10(k)) // half of a power of ten
		case 5:
			// Full precision with trailing zeros.
			c := g.randDigits(p - k)
			return c.Mul(c, pow10(k))
		}
		return g.randDigits(p)
	}
	if g.r.IntN(3) == 0 {
		return g.randDigits(p)
	}
	return g.randDigits(1 + g.r.IntN(p))
}

// exp returns an exponent in [minExp, maxExp] shaped by the regime.
func (g *gen) exp() int {
	f := g.f
	switch g.regime {
	case moderate:
		return g.r.IntN(21) - 10
	case fullRange:
		return f.minExp + g.r.IntN(f.maxExp-f.minExp+1)
	}
	near := func(e, spread int) int { return clamp(e+g.r.IntN(2*spread+1)-spread, f.minExp, f.maxExp) }
	switch g.r.IntN(6) {
	case 0:
		return f.minExp + g.r.IntN(f.prec+2)
	case 1:
		return f.maxExp - g.r.IntN(f.prec+2)
	case 2:
		return near(f.emin-f.prec+1, f.prec) // the subnormal edge
	case 3:
		return near(f.maxExp-f.prec, f.prec) // where padding stops fitting
	case 4:
		return near(-f.prec, f.prec)
	}
	return near(0, 3)
}

// special returns an infinity, a NaN of either kind and sign, or a
// non-canonical encoding.
func (g *gen) special() bits {
	f := g.f
	neg := g.r.IntN(2) == 0
	var t uint64
	switch g.r.IntN(5) {
	case 0, 1:
		return f.encode(value{kind: infinite, neg: neg})
	case 2:
		t = 0x1F << 58 // quiet NaN
	case 3:
		t = 0x1F<<58 | 1<<57 // signaling NaN
	default:
		// A random encoding: often non-canonical (an infinity or NaN with
		// stray bits, or a coefficient field above 10^prec - 1).
		x := bits{Lo: g.r.Uint64()}
		if f.width == 128 {
			x.Hi = g.r.Uint64()
		}
		return x
	}
	// NaN payload and reserved bits, sometimes.
	x := bits{}
	if g.r.IntN(2) == 0 {
		t |= g.r.Uint64() & (1<<50 - 1)
	}
	if f.width == 64 {
		x.Lo = t
	} else {
		x.Hi = t
		if g.r.IntN(2) == 0 {
			x.Lo = g.r.Uint64()
		}
	}
	return f.withSign(x, neg)
}

// dec returns an operand encoding.
func (g *gen) dec() bits {
	switch n := g.r.IntN(100); {
	case n < 4 || g.regime == boundary && n < 12:
		return g.special()
	case n < 10:
		return g.f.encode(value{kind: finite, neg: g.r.IntN(2) == 0, coeff: new(big.Int), exp: g.exp()})
	}
	return g.f.encode(g.finite())
}

func (g *gen) finite() value {
	return value{kind: finite, neg: g.r.IntN(2) == 0, coeff: g.coeff(), exp: g.exp()}
}

// near returns a finite operand whose exponent is within prec+2 of x's,
// so that the two overlap or nearly overlap when added.
func (g *gen) near(x value) value {
	y := g.finite()
	if x.kind == finite {
		y.exp = clamp(x.exp+g.r.IntN(2*g.f.prec+5)-g.f.prec-2, g.f.minExp, g.f.maxExp)
	}
	return y
}

// tie returns operands x, y of the binary operation op ("add", "mul" or
// "div") whose exact result lies exactly half way between two adjacent
// results at full precision, with exponents in the regime's range where
// possible. ok is false if no tie was found quickly.
func (g *gen) tie(op string) (x, y value, ok bool) {
	f := g.f
	p := f.prec
	neg := g.r.IntN(2) == 0
	switch op {
	case "add":
		// A prec-digit x plus 5 × 10^(e-1) has prec+1 digits ending in 5.
		e := clamp(g.exp(), f.minExp+1, f.maxExp)
		return value{kind: finite, neg: neg, coeff: g.randDigits(p), exp: e},
			value{kind: finite, neg: neg, coeff: big.NewInt(5), exp: e - 1}, true
	case "mul":
		// cx ending in 5 times an odd cy, with prec+1 digits in all.
		for range 20 {
			d1 := 1 + g.r.IntN(p)
			d2 := p + 1 - d1 + g.r.IntN(2)
			if d2 < 1 || d2 > p {
				continue
			}
			cx := g.randDigits(d1)
			cx.Sub(cx, new(big.Int).Rem(cx, bigTen)).Add(cx, big.NewInt(5))
			cy := g.randDigits(d2)
			cy.SetBit(cy, 0, 1)
			if digits(new(big.Int).Mul(cx, cy)) == p+1 {
				ex := g.exp() / 2
				return value{kind: finite, neg: neg, coeff: cx, exp: ex},
					value{kind: finite, coeff: cy, exp: clamp(g.exp()/2, f.minExp, f.maxExp)}, true
			}
		}
	case "div":
		// An odd cx divided by 2^k is cx × 5^k × 10^-k; choose k to give
		// prec+1 digits, which end in 5.
		cx := g.randDigits(1 + g.r.IntN(p))
		cx.SetBit(cx, 0, 1)
		for k := 1; k < 4*p; k++ {
			q := new(big.Int).Mul(cx, new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(k)), nil))
			if nd := digits(q); nd == p+1 {
				cy := new(big.Int).Lsh(bigOne, uint(k))
				if digits(cy) > p {
					break
				}
				return value{kind: finite, neg: neg, coeff: cx, exp: g.exp() / 2},
					value{kind: finite, coeff: cy, exp: g.exp() / 2}, true
			} else if nd > p+1 {
				break
			}
		}
	}
	return value{}, value{}, false
}

// pair returns operands for a binary operation: independent operands, or,
// in the boundary regime, overlapping ones and exact ties.
func (g *gen) pair(op string) (bits, bits) {
	if g.regime == boundary {
		switch g.r.IntN(4) {
		case 0:
			if x, y, ok := g.tie(op); ok {
				return g.f.encode(x), g.f.encode(y)
			}
		case 1:
			x := g.finite()
			return g.f.encode(x), g.f.encode(g.near(x))
		}
	} else if g.r.IntN(4) == 0 {
		x := g.finite()
		return g.f.encode(x), g.f.encode(g.near(x))
	}
	return g.dec(), g.dec()
}

// halfway returns a value with one more digit than the format holds,
// ending in 5, or a value of the format with a digit beyond its exponent:
// the tie cases of the one-operand roundings (Narrow, Parse, Round).
func (g *gen) halfway(f *format) value {
	c := g.randDigits(f.prec + 1)
	c.Sub(c, new(big.Int).Rem(c, bigTen)).Add(c, big.NewInt(5))
	return value{kind: finite, neg: g.r.IntN(2) == 0, coeff: c, exp: max(g.exp()-1, g.f.minExp)}
}

// int64 returns an int64 operand.
func (g *gen) int64() int64 {
	switch g.regime {
	case moderate:
		return g.r.Int64N(2_000_001) - 1_000_000
	case fullRange:
		return int64(g.r.Uint64())
	}
	switch g.r.IntN(6) {
	case 0:
		return [...]int64{math.MinInt64, math.MaxInt64, 0, -1, 1, math.MinInt64 + 1}[g.r.IntN(6)]
	case 1:
		// 17 to 19 digits ending in 5: ties in decimal64.
		n := int64(g.randDigits(17 + g.r.IntN(2)).Int64())
		n = n - n%10 + 5
		if g.r.IntN(2) == 0 {
			n = -n
		}
		return n
	case 2:
		n := pow10(g.r.IntN(19)).Int64()
		return n + int64(g.r.IntN(3)) - 1
	}
	return int64(g.r.Uint64()) >> g.r.IntN(64)
}

// uint64 returns a uint64 operand.
func (g *gen) uint64() uint64 {
	if g.regime == boundary && g.r.IntN(3) == 0 {
		return [...]uint64{0, 1, math.MaxUint64, math.MaxUint64 - 1, 1 << 63, 10000000000000000005}[g.r.IntN(6)]
	}
	if g.regime == moderate {
		return g.r.Uint64N(2_000_000)
	}
	return g.r.Uint64() >> g.r.IntN(64)
}

// intLike returns a decimal operand for the integer conversions: values
// near the integer bounds and with fractions of exactly one half, as well
// as general operands.
func (g *gen) intLike() bits {
	f := g.f
	if g.regime != boundary || g.r.IntN(2) == 0 {
		x := g.dec()
		if g.regime == moderate && g.r.IntN(2) == 0 {
			// A value with a fraction, within the integer range.
			c := new(big.Int).SetUint64(g.r.Uint64() >> g.r.IntN(64))
			c.Rem(c, pow10(f.prec))
			return f.encode(value{kind: finite, neg: g.r.IntN(2) == 0, coeff: c, exp: -g.r.IntN(4)})
		}
		return x
	}
	if f.width == 64 {
		// The integer bounds have more digits than decimal64 holds; take
		// the decimal64 values either side of each, which convert to the
		// last integer in range or saturate.
		c := big.NewInt([...]int64{9223372036854775, 9223372036854776, 1844674407370955, 1844674407370956}[g.r.IntN(4)])
		exp := 3
		if c.Int64() < 2e15 {
			exp = 4
		}
		c.Add(c, big.NewInt(int64(g.r.IntN(3)-1)))
		return f.encode(value{kind: finite, neg: g.r.IntN(2) == 0, coeff: c, exp: exp})
	}
	var c *big.Int
	switch g.r.IntN(4) {
	case 0:
		c = new(big.Int).SetUint64(math.MaxUint64)
	case 1:
		c = new(big.Int).SetUint64(1 << 63)
	case 2:
		c = new(big.Int).SetUint64(math.MaxInt64)
	default:
		c = new(big.Int).SetUint64(g.r.Uint64() >> g.r.IntN(64))
	}
	c.Add(c, big.NewInt(int64(g.r.IntN(5)-2)))
	exp := 0
	if g.r.IntN(2) == 0 {
		// Append a fraction digit: 5 is a tie.
		c.Mul(c, bigTen).Add(c, big.NewInt(int64([...]int{5, 5, 4, 6, 1, 9}[g.r.IntN(6)])))
		exp = -1
	}
	if digits(c) > f.prec {
		return g.dec()
	}
	return f.encode(value{kind: finite, neg: g.r.IntN(2) == 0, coeff: c.Abs(c), exp: exp})
}

// float64 returns a binary operand for the Float*To conversions.
func (g *gen) float64() float64 {
	switch g.regime {
	case moderate:
		return float64(g.r.Int64N(2_000_001)-1_000_000) / float64(pow10(g.r.IntN(7)).Int64())
	case fullRange:
		return math.Float64frombits(g.r.Uint64())
	}
	switch g.r.IntN(6) {
	case 0:
		return [...]float64{0, math.Copysign(0, -1), math.Inf(1), math.Inf(-1), math.NaN(),
			math.MaxFloat64, math.SmallestNonzeroFloat64, 0x1p-1022, 0.1, 1e23}[g.r.IntN(10)]
	case 1:
		return math.Float64frombits(g.r.Uint64() & (1<<52 - 1)) // subnormal
	case 2:
		// An odd m over a power of two whose exact decimal expansion has
		// prec+1 digits ending in 5: a tie in the destination.
		if v, ok := g.decimalTieFloat(g.f.prec); ok {
			return v
		}
	case 3:
		return math.Ldexp(1, g.r.IntN(2098)-1074)
	}
	return math.Float64frombits(g.r.Uint64())
}

// decimalTieFloat returns a float64 whose exact decimal expansion has
// exactly p+1 significant digits, the last a 5.
func (g *gen) decimalTieFloat(p int) (float64, bool) {
	for range 20 {
		m := g.r.Int64N(1<<53) | 1
		mb := new(big.Int).SetInt64(m)
		for k := 1; k < 300; k++ {
			q := new(big.Int).Mul(mb, new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(k)), nil))
			nd := digits(q)
			if nd == p+1 {
				return math.Ldexp(float64(m), -k), true
			}
			if nd > p+1 {
				break
			}
		}
	}
	return 0, false
}

// float32 returns a binary32 operand.
func (g *gen) float32() float32 {
	switch g.regime {
	case moderate:
		return float32(g.r.Int64N(200_001)-100_000) / float32(pow10(g.r.IntN(5)).Int64())
	case fullRange:
		return math.Float32frombits(g.r.Uint32())
	}
	switch g.r.IntN(3) {
	case 0:
		return [...]float32{0, float32(math.Copysign(0, -1)), float32(math.Inf(1)), math.MaxFloat32,
			math.SmallestNonzeroFloat32, 0x1p-126, 0.1, 16777217}[g.r.IntN(8)]
	case 1:
		return math.Float32frombits(g.r.Uint32() & (1<<23 - 1))
	}
	return math.Float32frombits(g.r.Uint32())
}

// binaryTie returns a decimal operand lying exactly half way between two
// adjacent binary values of precision p bits: an odd integer of p+1 bits
// times a power of two, when it is exact in the format.
func (g *gen) binaryTie(p int) bits {
	o := g.below(new(big.Int).Lsh(bigOne, uint(p)))
	o.SetBit(o, p, 1).SetBit(o, 0, 1)
	o.Lsh(o, uint(g.r.IntN(8)))
	if digits(o) > g.f.prec {
		return g.dec()
	}
	return g.f.encode(value{kind: finite, neg: g.r.IntN(2) == 0, coeff: o, exp: 0})
}

// numeral returns a string for Parse: a numeral in a random spelling of
// some value, a special word, or a string outside the grammar.
func (g *gen) numeral() string {
	f := g.f
	n := g.r.IntN(100)
	switch {
	case n < 5:
		w := [...]string{"inf", "infinity", "nan", "snan", "Inf", "INFINITY", "NaN", "sNaN", "SNAN", "iNfInItY"}[g.r.IntN(10)]
		return [...]string{"", "+", "-"}[g.r.IntN(3)] + w
	case n < 12:
		// Outside the grammar.
		return [...]string{"", "+", "-", ".", "e5", "1e", "1e+", "1.2.3", " 1", "1 ", "1_000", "0x10",
			"infinit", "infinityy", "nan0", "nan()", "1e5x", "--1", "+-1", "١", "1\x00", "1E+-5", "Inf1",
			" 1", "1,5", ".e1", "+.", "-e"}[g.r.IntN(28)]
	case n < 20:
		// Exponent parts far outside the range, and many digits.
		c := g.randDigits(1 + g.r.IntN(3*f.prec))
		if g.r.IntN(4) == 0 {
			c.SetInt64(0)
		}
		return c.String() + "E" + [...]string{"+99999999999", "-99999999999", "+6200", "-6300", "+400", "-420",
			"+" + strings.Repeat("9", 30), "-0000000000000000000000000000001"}[g.r.IntN(8)]
	case n < 30:
		h := g.halfway(f)
		return f.spell(h)
	}
	return f.spell(f.decode(g.dec()))
}

// spell writes a value as a numeral in a randomly chosen spelling, all of
// which Parse must read back to the same value and exponent: the
// character form, or digits with a point placed anywhere and the exponent
// adjusted to match, with optional sign, case and leading zeros.
func (f *format) spell(v value) string {
	if v.kind != finite {
		return f.format(f.encode(v))
	}
	r := rand.New(rand.NewPCG(uint64(v.exp), v.coeff.Uint64()))
	var sb strings.Builder
	if v.neg {
		sb.WriteByte('-')
	} else if r.IntN(4) == 0 {
		sb.WriteByte('+')
	}
	ds := strings.Repeat("0", r.IntN(3)) + v.coeff.String()
	// Place the point k digits from the right, 0 <= k <= len(ds), and
	// adjust the exponent part by k.
	k := r.IntN(len(ds) + 1)
	switch {
	case k == 0 && r.IntN(2) == 0:
		sb.WriteString(ds)
		if r.IntN(3) == 0 {
			sb.WriteByte('.')
		}
	case k == len(ds):
		if r.IntN(2) == 0 {
			sb.WriteByte('0')
		}
		sb.WriteByte('.')
		sb.WriteString(ds)
	default:
		sb.WriteString(ds[:len(ds)-k])
		sb.WriteByte('.')
		sb.WriteString(ds[len(ds)-k:])
	}
	e := v.exp + k
	if e != 0 || r.IntN(3) == 0 {
		sb.WriteString([...]string{"E", "e"}[r.IntN(2)])
		if e >= 0 && r.IntN(2) == 0 {
			sb.WriteByte('+')
		}
		sb.WriteString(big.NewInt(int64(e)).String())
	}
	return sb.String()
}

// parts returns arguments for Round: a coefficient of any width the
// format's word allows, any exponent, and a sticky bit.
func (g *gen) parts() (neg bool, c bid.Uint128, exp int, sticky bool) {
	f := g.f
	neg = g.r.IntN(2) == 0
	sticky = g.r.IntN(2) == 0
	switch g.regime {
	case moderate:
		c = bid.Uint128{Lo: g.r.Uint64() >> g.r.IntN(64)}
		exp = g.r.IntN(41) - 20
	case fullRange:
		c = bid.Uint128{Lo: g.r.Uint64()}
		if f.width == 128 {
			c.Hi = g.r.Uint64()
		}
		exp = f.minExp - 2*f.prec + g.r.IntN(f.maxExp-f.minExp+4*f.prec)
	default:
		switch g.r.IntN(4) {
		case 0:
			h := g.halfway(f)
			c = toUint128(h.coeff)
			exp = h.exp
		case 1:
			c = toUint128(f.maxCoeff)
			c.Lo += uint64(g.r.IntN(3))
			exp = f.maxExp + g.r.IntN(3) - 1
		case 2:
			c = bid.Uint128{Lo: uint64(g.r.IntN(30))}
			exp = f.minExp - g.r.IntN(3)
		default:
			c = toUint128(g.coeff())
			exp = [...]int{math.MinInt32, math.MaxInt32, f.minExp - 40, f.maxExp + f.prec, 0}[g.r.IntN(5)]
		}
	}
	if f.width == 64 {
		c.Hi = 0
	}
	return
}
