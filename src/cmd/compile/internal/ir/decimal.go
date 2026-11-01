// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ir

import (
	"go/constant"
	"go/token"
	"internal/bid"
	"math/big"
	"strconv"
	"strings"

	"cmd/compile/internal/base"
	"cmd/compile/internal/types"
)

// log10Of2 is log10(2), the decimal digits per bit. Exponent estimates use it
// and are corrected or bounded where they are used.
const log10Of2 = 0.30103

// DecimalConst returns the numeric constant c as the decimal type t, in words
// of its memory form: the bits are lo for decimal64, with hi zero, and hi, lo
// for decimal128. The memory form is the BID encoding XORed with the encoding
// of 0E+0, so that all bits clear is the zero value. Package bid rounds and
// encodes the value;
// this function only reduces it to the parts bid.Round64 and bid.Round128 take.
// The exact value is c.Val(), so no rounding happens before bid's.
//
// A value too large for t encodes as an infinity and a value too small as a
// zero, which is bid's default result. The type checker has already rejected
// a source constant above the largest finite value of t.
func DecimalConst(t *types.Type, c Node) (lo, hi uint64) {
	val, lit := c.Val(), ""
	if b, ok := c.(*BasicLit); ok {
		lit = b.OrigLit()
	}
	if t.Kind() == types.TDECIMAL128 {
		neg, coeff, exp, sticky := decimalParts(val, lit, bid.Precision128, bid.MinExp128, bid.Emax128)
		words, low := new(big.Int).QuoRem(coeff, new(big.Int).Lsh(big.NewInt(1), 64), new(big.Int))
		bits, _ := bid.Round128(neg, bid.Uint128{Hi: words.Uint64(), Lo: low.Uint64()}, exp, sticky, bid.ToNearestEven)
		return bits.Lo, bits.Hi ^ bid.Zero128Hi
	}
	neg, coeff, exp, sticky := decimalParts(val, lit, bid.Precision64, bid.MinExp64, bid.Emax64)
	bits, _ := bid.Round64(neg, coeff.Uint64(), exp, sticky, bid.ToNearestEven)
	return bits ^ bid.Zero64, 0
}

// LiteralFloat64Val returns the value of the source literal of the float64
// constant c, with the sign of c, if c.Val() is that value rounded to float64,
// and otherwise c.Val(). It is for the conversion T(0.1) to a type parameter T:
// by the time the operand has been read it has its default type, float64, and
// the IR holds its value rounded to that type, but the spec converts the
// constant. T(float64(0.1)) looks the same in the IR and takes this path too.
func LiteralFloat64Val(c Node) constant.Value {
	val := c.Val()
	b, ok := c.(*BasicLit)
	if !ok {
		return val
	}
	exact := constant.ToFloat(constant.MakeFromLiteral(strings.ReplaceAll(b.OrigLit(), "_", ""), token.FLOAT, 0))
	if exact.Kind() != constant.Float {
		return val
	}
	want, _ := constant.Float64Val(val)
	got, _ := constant.Float64Val(exact)
	if want != got && want != -got {
		return val
	}
	if constant.Sign(val) < 0 {
		return constant.UnaryOp(token.SUB, exact, 0)
	}
	return exact
}

// decimalParts reduces the constant val to the arguments of bid.Round64 or
// bid.Round128 for a format with the given precision, minimum exponent and
// Emax: the value is (-1)^neg × (coeff + f) × 10^exp, where f is zero when
// sticky is false and 0 < f < 1 when it is true, and coeff has at most
// precision+1 digits, so it fits either coefficient type and keeps the round
// digit that bid's sticky contract requires of an inexact value.
//
// The exponent asked of bid is that of lit, the source literal, or for a
// value with no literal that of a 16-digit coefficient in both widths. A zero
// with no literal is the all-bits-zero encoding, the zero value of the type.
// (These are the strawman's quantum sources; the constant model replaces
// them.) If the value is not an integer multiple of 10^exp, exp moves down to
// the nearest exponent where it is, and if it has no such exponent with few
// enough digits, the result is inexact and bid gives it precision digits.
func decimalParts(val constant.Value, lit string, precision, minExp, emax int) (neg bool, coeff *big.Int, exp int, sticky bool) {
	v := constant.ToFloat(val)
	if v.Kind() != constant.Float {
		base.Fatalf("decimal constant %v has no real value", val)
	}
	neg = constant.Sign(v) < 0
	quantum, hasQuantum := literalExponent(lit)

	// go/constant holds a value between 2^-4096 and 2^4096 as a rational, which
	// is exact, and any other as a 512-bit float.
	var x *big.Rat
	switch u := constant.Val(v).(type) {
	case *big.Rat:
		x = new(big.Rat).Abs(u)
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
		x, _ = abs.Rat(nil)
		// A decimal literal is exact where the float is not: 1E+2000 is not an
		// integer multiple of 10^2000 in 512 bits.
		if clean := strings.TrimSuffix(strings.ReplaceAll(lit, "_", ""), "i"); clean != "" {
			parsed := constant.ToFloat(constant.MakeFromLiteral(clean, token.FLOAT, 0))
			if p, ok := constant.Val(parsed).(*big.Float); ok && p.Cmp(abs) == 0 {
				if exact, ok := new(big.Rat).SetString(clean); ok {
					x = exact
				}
			}
		}
	default:
		base.Fatalf("decimal constant %v has unexpected representation %T", val, u)
	}

	// A constant with no literal to give it a quantum has the exponent the
	// integer conversion would give it, 0, or failing that the smallest
	// coefficient that holds its value.
	if x.Sign() == 0 {
		if !hasQuantum {
			quantum = 0
		}
		return false, new(big.Int), quantum, false
	}

	// The estimate is within 1.3 of log10(x), so x scaled by 10^(2-estimate)
	// is above 5 and its integer part names the adjusted exponent exactly.
	estimate := int(float64(x.Num().BitLen()-x.Denom().BitLen()) * log10Of2)
	scaled, _ := scaleDecimal(x, estimate-2)
	adjusted := estimate - 2 + len(scaled.String()) - 1
	if !hasQuantum {
		quantum = 0
		if adjusted >= precision {
			quantum = adjusted
		}
	}

	// x has a finite expansion when its denominator, which is in lowest terms,
	// has no prime factor but 2 and 5. Its exponent is then at most -places,
	// or the count of its trailing zeros when it is an integer.
	rest := new(big.Int).Set(x.Denom())
	twos := int(rest.TrailingZeroBits())
	rest.Rsh(rest, uint(twos))
	five, fives := big.NewInt(5), 0
	for {
		q, r := new(big.Int).QuoRem(rest, five, new(big.Int))
		if r.Sign() != 0 {
			break
		}
		rest, fives = q, fives+1
	}
	if rest.IsInt64() && rest.Int64() == 1 {
		exp = -max(twos, fives)
		if exp == 0 {
			ten, n := big.NewInt(10), x.Num()
			for exp < quantum {
				q, r := new(big.Int).QuoRem(n, ten, new(big.Int))
				if r.Sign() != 0 {
					break
				}
				n, exp = q, exp+1
			}
		}
		exp = min(exp, quantum)
		if adjusted-exp+1 <= precision+1 {
			coeff, _ = scaleDecimal(x, exp)
			return neg, coeff, exp, false
		}
	}

	exp = adjusted - precision
	coeff, rem := scaleDecimal(x, exp)
	return neg, coeff, exp, rem.Sign() != 0
}

// scaleDecimal returns the integer part of x × 10^-exp and the remainder of the
// division that produces it, which is zero when the result is exact.
func scaleDecimal(x *big.Rat, exp int) (q, r *big.Int) {
	num, den := x.Num(), x.Denom()
	if exp < 0 {
		num = new(big.Int).Mul(num, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-exp)), nil))
	} else {
		den = new(big.Int).Mul(den, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp)), nil))
	}
	return new(big.Int).QuoRem(num, den, new(big.Int))
}

// literalExponent returns the decimal exponent that the source literal lit
// denotes, its exponent part less its fraction digits, and whether lit is a
// literal at all. A rune literal and a literal in another base have no decimal
// digits to count and denote exponent 0. An exponent part beyond the range of
// an int saturates, which is far outside every format.
func literalExponent(lit string) (exp int, ok bool) {
	if lit == "" {
		return 0, false
	}
	lower := strings.ToLower(strings.ReplaceAll(lit, "_", ""))
	if lower[0] == '\'' || strings.HasPrefix(lower, "0x") || strings.HasPrefix(lower, "0o") || strings.HasPrefix(lower, "0b") {
		return 0, true
	}
	mantissa, exponent, hasExponent := strings.Cut(strings.TrimSuffix(lower, "i"), "e")
	if hasExponent {
		// Atoi returns the nearest int, with an error, for an out-of-range value.
		exp, _ = strconv.Atoi(exponent)
		exp = max(min(exp, 1<<30), -1<<30)
	}
	if _, fraction, ok := strings.Cut(mantissa, "."); ok {
		exp -= len(fraction)
	}
	return exp, true
}
