// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package constdecimal converts Go constants to and from the decimal
// floating-point formats, and evaluates arithmetic on constants of a decimal
// type. The type checkers and the compiler share it, so that they agree on the
// value and quantum of every decimal constant. The arithmetic is internal/bid's.
//
// A constant's quantum (see [constant.Quantum]) chooses its encoding:
//
//   - An exact conversion keeps the quantum if the coefficient fits the
//     format's precision, and otherwise uses the exponent closest to it.
//   - A constant without a quantum takes exponent 0 if it is an integer that
//     fits, and otherwise the smallest coefficient that holds its value.
//   - An inexact conversion rounds to nearest, ties to even, at the format's
//     precision, and keeps every digit of the rounded coefficient.
package constdecimal

import (
	"go/constant"
	"go/token"
	"internal/bid"
	"math/big"
	"strconv"
)

// Width is a decimal format: 64 for decimal64 or 128 for decimal128.
type Width int

const (
	Decimal64  Width = 64
	Decimal128 Width = 128
)

func (w Width) precision() int {
	if w == Decimal128 {
		return bid.Precision128
	}
	return bid.Precision64
}

func (w Width) minExp() int {
	if w == Decimal128 {
		return bid.MinExp128
	}
	return bid.MinExp64
}

func (w Width) emax() int {
	if w == Decimal128 {
		return bid.Emax128
	}
	return bid.Emax64
}

// log10Of2 is log10(2), the decimal digits per bit. Exponent estimates use it
// and are corrected or bounded where they are used.
const log10Of2 = 0.30103

// exactBits is how close, as a power of two relative to the value, a value
// that go/constant holds as a 512-bit float must be to a coefficient times
// ten to its quantum to be taken as exactly that. Such a value came from a
// decimal literal or exact arithmetic beyond the range of a fraction, and its
// float carries 512 bits of the true value.
const exactBits = 480

// Encode returns the BID encoding of the real constant x in the format w, and
// whether x is finite in that format. For decimal64 the encoding is the low
// word. A constant above the largest finite value encodes as an infinity.
func Encode(x constant.Value, w Width) (bid.Uint128, bool) {
	neg, coeff, exp, sticky := parts(x, w)
	if w == Decimal128 {
		words, low := new(big.Int).QuoRem(coeff, new(big.Int).Lsh(big.NewInt(1), 64), new(big.Int))
		bits, _ := bid.Round128(neg, bid.Uint128{Hi: words.Uint64(), Lo: low.Uint64()}, exp, sticky, bid.ToNearestEven)
		k, _, _, _ := bid.Unpack128(bits)
		return bits, k == bid.KindFinite
	}
	bits, _ := bid.Round64(neg, coeff.Uint64(), exp, sticky, bid.ToNearestEven)
	k, _, _, _ := bid.Unpack64(bits)
	return bid.Uint128{Lo: bits}, k == bid.KindFinite
}

// Decode returns the constant whose value and quantum are those of the finite
// decimal bits in the format w. The sign of a zero is dropped, since constants
// have no negative zero.
func Decode(bits bid.Uint128, w Width) constant.Value {
	var neg bool
	var digits string
	var exp int
	if w == Decimal128 {
		_, n, c, e := bid.Unpack128(bits)
		neg, exp = n, e
		digits = new(big.Int).Or(new(big.Int).Lsh(new(big.Int).SetUint64(c.Hi), 64), new(big.Int).SetUint64(c.Lo)).String()
	} else {
		_, n, c, e := bid.Unpack64(bits.Lo)
		neg, exp = n, e
		digits = strconv.FormatUint(c, 10)
	}
	v := constant.MakeFromLiteral(digits+"e"+strconv.Itoa(exp), token.FLOAT, 0)
	if neg {
		v = constant.UnaryOp(token.SUB, v, 0)
	}
	return v
}

// Round returns the real constant x as the format w holds it, with its value
// and quantum, and whether it is finite in that format.
func Round(x constant.Value, w Width) (constant.Value, bool) {
	bits, ok := Encode(x, w)
	if !ok {
		return x, false
	}
	return Decode(bits, w), true
}

// BinaryOp returns the result of x op y for constants of the decimal format w,
// rounded as the run-time operation rounds it, and whether the result is
// finite. op is one of token.ADD, token.SUB, token.MUL and token.QUO. x and y
// must be finite in the format, and y must not be zero for token.QUO.
func BinaryOp(x constant.Value, op token.Token, y constant.Value, w Width) (constant.Value, bool) {
	a, okx := Encode(x, w)
	b, oky := Encode(y, w)
	if !okx || !oky {
		return x, false
	}
	var z bid.Uint128
	var kind bid.Kind
	if w == Decimal128 {
		switch op {
		case token.ADD:
			z = bid.Add128(a, b, bid.ToNearestEven)
		case token.SUB:
			z = bid.Sub128(a, b, bid.ToNearestEven)
		case token.MUL:
			z = bid.Mul128(a, b, bid.ToNearestEven)
		case token.QUO:
			z = bid.Div128(a, b, bid.ToNearestEven)
		default:
			panic("constdecimal: invalid operation " + op.String())
		}
		kind, _, _, _ = bid.Unpack128(z)
	} else {
		var r uint64
		switch op {
		case token.ADD:
			r = bid.Add64(a.Lo, b.Lo, bid.ToNearestEven)
		case token.SUB:
			r = bid.Sub64(a.Lo, b.Lo, bid.ToNearestEven)
		case token.MUL:
			r = bid.Mul64(a.Lo, b.Lo, bid.ToNearestEven)
		case token.QUO:
			r = bid.Div64(a.Lo, b.Lo, bid.ToNearestEven)
		default:
			panic("constdecimal: invalid operation " + op.String())
		}
		z = bid.Uint128{Lo: r}
		kind, _, _, _ = bid.Unpack64(r)
	}
	if kind != bid.KindFinite {
		return x, false
	}
	return Decode(z, w), true
}

// parts reduces the real constant x to the arguments bid.Round64 and
// bid.Round128 take: a sign, a coefficient, the exponent of its last digit,
// and a sticky bit standing for nonzero digits below it.
func parts(x constant.Value, w Width) (neg bool, coeff *big.Int, exp int, sticky bool) {
	v := constant.ToFloat(x)
	if v.Kind() != constant.Float {
		panic("constdecimal: constant " + x.String() + " has no real value")
	}
	precision, minExp, emax := w.precision(), w.minExp(), w.emax()
	neg = constant.Sign(v) < 0
	quantum, hasQuantum := constant.Quantum(v)

	// go/constant holds a value between 2^-4096 and 2^4096 as a fraction,
	// which is exact, and any other as a 512-bit float.
	var r *big.Rat
	switch u := constant.Val(v).(type) {
	case *big.Rat:
		r = new(big.Rat).Abs(u)
	case *big.Float:
		// A binary exponent can be far too large to expand, so answer from its
		// size when the value is out of range by more than the estimate's error.
		adjusted := int(float64(u.MantExp(nil)) * log10Of2)
		if adjusted > emax+1 {
			return neg, big.NewInt(1), adjusted, false
		}
		if adjusted < minExp-2 {
			return neg, new(big.Int), minExp, true
		}
		abs := new(big.Float).Abs(u)
		r, _ = abs.Rat(nil)
		// A value with a quantum came from decimal digits. If its float is
		// within rounding error of a coefficient times ten to that quantum,
		// it is that value: 1E+2000 is not an integer multiple of 10^2000
		// in 512 bits.
		if hasQuantum {
			if c, ok := nearCoefficient(abs, quantum); ok {
				r = new(big.Rat).SetInt(c)
				if quantum < 0 {
					r.Quo(r, new(big.Rat).SetInt(pow10(-quantum)))
				} else {
					r.Mul(r, new(big.Rat).SetInt(pow10(quantum)))
				}
			}
		}
	default:
		panic("constdecimal: unexpected representation of " + x.String())
	}

	if r.Sign() == 0 {
		if !hasQuantum {
			quantum = 0
		}
		return false, new(big.Int), quantum, false
	}

	// The estimate is within 1.3 of log10(r), so r scaled by 10^(2-estimate)
	// is above 5 and its integer part names the adjusted exponent exactly.
	estimate := int(float64(r.Num().BitLen()-r.Denom().BitLen()) * log10Of2)
	scaled, _ := scale(r, estimate-2)
	adjusted := estimate - 2 + len(scaled.String()) - 1
	if !hasQuantum {
		// Exponent 0 for an integer that fits; otherwise the largest
		// exponent at which the value is exact, the smallest coefficient.
		quantum = 0
		if adjusted >= precision {
			quantum = adjusted
		}
	}

	// r has a finite expansion when its denominator, which is in lowest terms,
	// has no prime factor but 2 and 5. Its exponent is then at most -places,
	// or the count of its trailing zeros when it is an integer.
	rest := new(big.Int).Set(r.Denom())
	twos := int(rest.TrailingZeroBits())
	rest.Rsh(rest, uint(twos))
	five, fives := big.NewInt(5), 0
	for {
		q, rem := new(big.Int).QuoRem(rest, five, new(big.Int))
		if rem.Sign() != 0 {
			break
		}
		rest, fives = q, fives+1
	}
	if rest.IsInt64() && rest.Int64() == 1 {
		exp = -max(twos, fives)
		if exp == 0 {
			ten, n := big.NewInt(10), r.Num()
			for exp < quantum {
				q, rem := new(big.Int).QuoRem(n, ten, new(big.Int))
				if rem.Sign() != 0 {
					break
				}
				n, exp = q, exp+1
			}
		}
		exp = min(exp, quantum)
		if adjusted-exp+1 <= precision+1 {
			coeff, _ = scale(r, exp)
			return neg, coeff, exp, false
		}
	}

	exp = adjusted - precision
	coeff, rem := scale(r, exp)
	return neg, coeff, exp, rem.Sign() != 0
}

// nearCoefficient returns the integer c nearest to x × 10^-quantum, and
// whether x is within 2^-exactBits of c × 10^quantum relative to x.
func nearCoefficient(x *big.Float, quantum int) (*big.Int, bool) {
	const prec = 1024
	p := new(big.Float).SetPrec(prec).SetInt(pow10(max(quantum, -quantum)))
	s := new(big.Float).SetPrec(prec).Set(x)
	if quantum < 0 {
		s.Mul(s, p)
	} else {
		s.Quo(s, p)
	}
	s.Add(s, big.NewFloat(0.5))
	c, _ := s.Int(nil)
	back := new(big.Float).SetPrec(prec).SetInt(c)
	if quantum < 0 {
		back.Quo(back, p)
	} else {
		back.Mul(back, p)
	}
	diff := new(big.Float).SetPrec(prec).Sub(back, x)
	if diff.Sign() == 0 {
		return c, true
	}
	return c, diff.Abs(diff).MantExp(nil) <= x.MantExp(nil)-exactBits
}

func pow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

// scale returns the integer part of r × 10^-exp and the remainder of the
// division that produces it, which is zero when the result is exact.
func scale(r *big.Rat, exp int) (q, rem *big.Int) {
	num, den := r.Num(), r.Denom()
	if exp < 0 {
		num = new(big.Int).Mul(num, pow10(-exp))
	} else {
		den = new(big.Int).Mul(den, pow10(exp))
	}
	return new(big.Int).QuoRem(num, den, new(big.Int))
}
