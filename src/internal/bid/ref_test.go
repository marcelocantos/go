// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// This file and refops_test.go are the reference model of the kernel: an
// exact implementation of the contract in the package documentation of
// internal/bid, written with math/big alone. The contract, not the kernel,
// is the reference model's source of truth. Where the two disagree the
// contract is right, and the reference model or the contract is fixed
// before the kernel is changed. The worked examples of the contract are
// checked against this model by TestContractExamples.
//
// The model shares no code with the kernel. It writes down its own format
// parameters and its own BID codec, and computes every result as an exact
// value n/d × 10^e before rounding it once in round. It uses only the
// kernel's API types (Uint128, Mode, Status, Key, Kind) and their
// constants, so that its results can be compared with the kernel's.
//
// The model favours being obviously correct over being fast; it is fast
// enough that go test -short runs every operation 10^4 times per width.

import (
	"internal/bid"
	"math/big"
	"sync"
)

// format holds the parameters of one decimal interchange format, written
// down here from IEEE 754-2008 clause 3.3 and Table 3.6 rather than taken
// from the kernel, so that a wrong constant in the kernel shows up as a
// mismatch. TestFormatConstants compares the two.
type format struct {
	name     string
	width    int // 64 or 128
	prec     int // precision in digits
	emin     int // 10^emin is the smallest normal magnitude
	minExp   int // exponent of the smallest subnormal (etiny)
	maxExp   int // largest exponent of an encoding
	bias     int
	maxCoeff *big.Int // 10^prec - 1
}

var (
	dec64  = &format{name: "decimal64", width: 64, prec: 16, emin: -383, minExp: -398, maxExp: 369, bias: 398}
	dec128 = &format{name: "decimal128", width: 128, prec: 34, emin: -6143, minExp: -6176, maxExp: 6111, bias: 6176}
)

func init() {
	for _, f := range []*format{dec64, dec128} {
		f.maxCoeff = new(big.Int).Sub(pow10(f.prec), bigOne)
	}
}

var (
	bigOne = big.NewInt(1)
	bigTen = big.NewInt(10)
)

// pow10Cache holds 10^n for the n the model has needed so far.
var pow10Cache sync.Map // int -> *big.Int

// pow10 returns 10^n for n >= 0. The result is shared and must not be
// modified.
func pow10(n int) *big.Int {
	if n < 0 {
		panic("pow10: negative exponent")
	}
	if p, ok := pow10Cache.Load(n); ok {
		return p.(*big.Int)
	}
	p := new(big.Int).Exp(bigTen, big.NewInt(int64(n)), nil)
	pow10Cache.Store(n, p)
	return p
}

// digits returns the number of decimal digits of n > 0, and 0 for n = 0.
func digits(n *big.Int) int {
	if n.Sign() == 0 {
		return 0
	}
	// log10(2) is just above 0.30103, so this estimate is at most one low.
	d := int(float64(n.BitLen()-1)*0.30102999566398) + 1
	if n.CmpAbs(pow10(d)) >= 0 {
		d++
	}
	return d
}

// trailingZeros returns the number of trailing decimal zeros of n > 0.
func trailingZeros(n *big.Int) int {
	// A decimal zero needs a factor of two, so the answer is at most the
	// number of trailing zero bits; find it by bisection on divisibility.
	lo, hi := 0, int(n.TrailingZeroBits())
	r := new(big.Int)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if r.Rem(n, pow10(mid)).Sign() == 0 {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// kind classifies a decoded value.
type kind uint8

const (
	finite kind = iota
	infinite
	notANumber
)

// value is a decoded decimal: (-1)^neg × coeff × 10^exp when finite. For an
// infinity only neg is meaningful. For a NaN, neg is the sign bit, which
// only the sign operations and TotalOrder read; signaling records the
// signaling bit, which only the predicates read.
type value struct {
	kind      kind
	neg       bool
	signaling bool
	coeff     *big.Int
	exp       int
}

// bits is a bit pattern of either width. A decimal64 is held in Lo with Hi
// zero, so that one codec and one set of operations serve both widths.
type bits = bid.Uint128

// top returns the word that holds the sign and the combination field.
func (f *format) top(b bits) uint64 {
	if f.width == 64 {
		return b.Lo
	}
	return b.Hi
}

// decode reads b as the contract says (IEEE 754-2008 clause 3.5.2): the
// second encoding form has an implicit 100 above its stored coefficient
// bits, and a finite encoding whose coefficient exceeds 10^prec - 1 is a
// zero with the encoded sign and exponent.
func (f *format) decode(b bits) value {
	t := f.top(b)
	neg := t>>63 != 0
	if t>>59&0xF == 0xF {
		if t>>58&1 == 0 {
			return value{kind: infinite, neg: neg}
		}
		return value{kind: notANumber, neg: neg, signaling: t>>57&1 != 0}
	}
	coeff := new(big.Int)
	var biased uint64
	if f.width == 64 {
		if t>>61&3 == 3 {
			biased = t >> 51 & (1<<10 - 1)
			coeff.SetUint64(1<<53 | t&(1<<51-1))
		} else {
			biased = t >> 53 & (1<<10 - 1)
			coeff.SetUint64(t & (1<<53 - 1))
		}
	} else {
		if t>>61&3 == 3 {
			// The implicit 100 puts every second-form coefficient at or
			// above 2^113, which exceeds 10^34 - 1: always a zero.
			biased = t >> 47 & (1<<14 - 1)
		} else {
			biased = t >> 49 & (1<<14 - 1)
			coeff.SetUint64(t & (1<<49 - 1))
			coeff.Lsh(coeff, 64)
			coeff.Or(coeff, new(big.Int).SetUint64(b.Lo))
		}
	}
	if coeff.Cmp(f.maxCoeff) > 0 {
		coeff.SetInt64(0)
	}
	return value{kind: finite, neg: neg, coeff: coeff, exp: int(biased) - f.bias}
}

// encode returns the canonical encoding of v. Every NaN encodes as the
// canonical positive quiet NaN, whatever v.neg says, because the model
// produces no other NaN.
func (f *format) encode(v value) bits {
	var t, lo uint64
	sign := uint64(0)
	if v.neg {
		sign = 1 << 63
	}
	switch v.kind {
	case notANumber:
		t = 0x1F << 58
	case infinite:
		t = sign | 0x1E<<58
	default:
		if v.coeff.Sign() < 0 || v.coeff.Cmp(f.maxCoeff) > 0 || v.exp < f.minExp || v.exp > f.maxExp {
			panic("encode: value out of range: model bug")
		}
		biased := uint64(v.exp + f.bias)
		if f.width == 64 {
			c := v.coeff.Uint64()
			if c < 1<<53 {
				t = sign | biased<<53 | c
			} else {
				t = sign | 3<<61 | biased<<51 | c&(1<<51-1)
			}
		} else {
			hi := new(big.Int).Rsh(v.coeff, 64).Uint64()
			lo = new(big.Int).And(v.coeff, new(big.Int).SetUint64(^uint64(0))).Uint64()
			t = sign | biased<<49 | hi
		}
	}
	if f.width == 64 {
		return bits{Lo: t}
	}
	return bits{Hi: t, Lo: lo}
}

// canonical reports whether b is a canonical encoding: what IsCanonical
// reports. A finite encoding is canonical when its coefficient does not
// exceed 10^prec - 1; an infinity when every bit below the five-bit special
// prefix is zero; a NaN when the bits between the signaling bit and the
// trailing coefficient field are zero and its payload is below
// 10^(prec-1).
func (f *format) canonical(b bits) bool {
	t := f.top(b)
	if t>>59&0xF == 0xF {
		if t>>58&1 == 0 {
			if f.width == 64 {
				return t&(1<<58-1) == 0
			}
			return t&(1<<58-1) == 0 && b.Lo == 0
		}
		// The combination field is 13 bits for decimal64 and 17 for
		// decimal128; after the five-bit prefix and the signaling bit the
		// rest of it must be zero. The trailing field is the payload.
		var payload *big.Int
		if f.width == 64 {
			if t>>50&(1<<7-1) != 0 {
				return false
			}
			payload = new(big.Int).SetUint64(t & (1<<50 - 1))
		} else {
			if t>>46&(1<<11-1) != 0 {
				return false
			}
			payload = new(big.Int).SetUint64(t & (1<<46 - 1))
			payload.Lsh(payload, 64)
			payload.Or(payload, new(big.Int).SetUint64(b.Lo))
		}
		return payload.Cmp(pow10(f.prec-1)) < 0
	}
	if f.width == 64 {
		if t>>61&3 == 3 {
			return 1<<53|t&(1<<51-1) <= 9999999999999999
		}
		return true
	}
	if t>>61&3 == 3 {
		return false
	}
	c := new(big.Int).SetUint64(t & (1<<49 - 1))
	c.Lsh(c, 64)
	c.Or(c, new(big.Int).SetUint64(b.Lo))
	return c.Cmp(f.maxCoeff) <= 0
}

// largest returns the largest finite value of the format with sign neg.
func (f *format) largest(neg bool) value {
	return value{kind: finite, neg: neg, coeff: new(big.Int).Set(f.maxCoeff), exp: f.maxExp}
}

// normMode maps a Mode outside the five to ToNearestEven, as the contract
// requires.
func normMode(m bid.Mode) bid.Mode {
	if m > bid.TowardNegative {
		return bid.ToNearestEven
	}
	return m
}

// roundQuo returns num/den rounded to an integer in mode, for num >= 0 and
// den > 0, where neg is the sign of the value being rounded, and reports
// whether a non-zero remainder was discarded.
func roundQuo(num, den *big.Int, neg bool, mode bid.Mode) (*big.Int, bool) {
	q, r := new(big.Int).QuoRem(num, den, new(big.Int))
	if r.Sign() == 0 {
		return q, false
	}
	up := false
	switch normMode(mode) {
	case bid.ToNearestEven:
		c := new(big.Int).Lsh(r, 1).Cmp(den)
		up = c > 0 || c == 0 && q.Bit(0) == 1
	case bid.ToNearestAway:
		up = new(big.Int).Lsh(r, 1).Cmp(den) >= 0
	case bid.TowardZero:
	case bid.TowardPositive:
		up = !neg
	case bid.TowardNegative:
		up = neg
	}
	if up {
		q.Add(q, bigOne)
	}
	return q, true
}

// overflowed returns the default result of an overflow of sign neg in mode
// (IEEE 754-2008 clause 7.4): an infinity under the nearest modes and in
// the direction of the overflow, the largest finite value otherwise.
func (f *format) overflowed(neg bool, mode bid.Mode) value {
	switch normMode(mode) {
	case bid.TowardZero:
		return f.largest(neg)
	case bid.TowardPositive:
		if neg {
			return f.largest(true)
		}
	case bid.TowardNegative:
		if !neg {
			return f.largest(false)
		}
	}
	return value{kind: infinite, neg: neg}
}

// round is the model's one rounding routine. It returns the value
// (-1)^neg × n/d × 10^e rounded to f in mode, with preferred exponent pref,
// and the Status the contract gives it; n >= 0 and d > 0. A zero n is an
// exact zero of sign neg. Every rule of the contract's sections "Exponent
// of a result", "Overflow" and "Underflow and subnormal results" is here.
func (f *format) round(neg bool, n, d *big.Int, e, pref int, mode bid.Mode) (value, bid.Status) {
	if n.Sign() == 0 {
		return value{kind: finite, neg: neg, coeff: new(big.Int), exp: clamp(pref, f.minExp, f.maxExp)}, bid.Exact
	}
	n = new(big.Int).Set(n)
	d = new(big.Int).Set(d)

	// Values far outside the format round the same as a nearer proxy;
	// replace them so that no power of ten grows without bound. The
	// adjusted exponent of n/d × 10^e is within one of digits(n) -
	// digits(d) + e. At or above 10^(maxExp+prec) every mode overflows;
	// below 10^(minExp-1) every mode gives the zero or the smallest
	// subnormal that a value below half the smallest subnormal gives.
	adj := digits(n) - digits(d) + e
	switch {
	case adj > f.maxExp+f.prec+2:
		n.SetInt64(1)
		d.SetInt64(1)
		e = f.maxExp + f.prec + 1
	case adj < f.minExp-3:
		n.SetInt64(1)
		d.SetInt64(1)
		e = f.minExp - 2
		// 10^(minExp-2) is not representable, so the proxy stays inexact.
	}

	// Reduce n/d, and if d then has no prime factors but 2 and 5, the
	// value is a terminating decimal: rewrite it as n × 10^e with d = 1.
	g := new(big.Int).GCD(nil, nil, n, d)
	n.Quo(n, g)
	d.Quo(d, g)
	if d.Cmp(bigOne) != 0 {
		r := new(big.Int).Set(d)
		twos := int(r.TrailingZeroBits())
		r.Rsh(r, uint(twos))
		fives := 0
		five := big.NewInt(5)
		m := new(big.Int)
		for {
			q, rem := new(big.Int).QuoRem(r, five, m)
			if rem.Sign() != 0 {
				break
			}
			r = q
			fives++
		}
		if r.Cmp(bigOne) == 0 {
			k := max(twos, fives)
			n.Mul(n, new(big.Int).Lsh(bigOne, uint(k-twos)))
			n.Mul(n, new(big.Int).Exp(five, big.NewInt(int64(k-fives)), nil))
			e -= k
			d.SetInt64(1)
		}
	}

	// An exact result is the member of its cohort whose exponent is
	// closest to pref: with c the coefficient stripped of trailing zeros
	// at exponent ce, the exponents available are those x <= ce at which
	// c × 10^(ce-x) has at most prec digits, within [minExp, maxExp].
	if d.Cmp(bigOne) == 0 {
		tz := trailingZeros(n)
		c := new(big.Int).Quo(n, pow10(tz))
		ce := e + tz
		if nd := digits(c); nd <= f.prec {
			lo := max(ce-(f.prec-nd), f.minExp)
			hi := min(ce, f.maxExp)
			if lo <= hi {
				x := clamp(pref, lo, hi)
				return value{kind: finite, neg: neg, coeff: c.Mul(c, pow10(ce-x)), exp: x}, bid.Exact
			}
		}
	}

	// Inexact: the least exponent at which the result has prec digits, or
	// minExp if that is lower. a is the adjusted exponent, floor(log10).
	// floor(log10(n/d)) is digits(n) - digits(d) or one less.
	a := digits(n) - digits(d)
	var atLeast bool
	if a >= 0 {
		atLeast = n.Cmp(new(big.Int).Mul(d, pow10(a))) >= 0
	} else {
		atLeast = new(big.Int).Mul(n, pow10(-a)).Cmp(d) >= 0
	}
	if !atLeast {
		a--
	}
	a += e
	tiny := a < f.emin
	x := max(a-f.prec+1, f.minExp)
	num, den := new(big.Int).Set(n), new(big.Int).Set(d)
	if s := e - x; s >= 0 {
		num.Mul(num, pow10(s))
	} else {
		den.Mul(den, pow10(-s))
	}
	c, _ := roundQuo(num, den, neg, mode)
	if c.Cmp(pow10(f.prec)) == 0 {
		c.Quo(c, bigTen)
		x++
	}
	if x > f.maxExp {
		return f.overflowed(neg, mode), bid.Inexact | bid.Overflow
	}
	st := bid.Inexact
	if tiny {
		st |= bid.Underflow
	}
	return value{kind: finite, neg: neg, coeff: c, exp: x}, st
}

func clamp(x, lo, hi int) int {
	return min(max(x, lo), hi)
}
