// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package strconv

import (
	"internal/bid"
	"internal/decimal"
	"math/bits"
	_ "unsafe"
)

// This file reads the syntax ParseFloat reads into a decimal value: Go's
// floating-point literals, with underscores between digits, hexadecimal
// mantissas with a binary exponent, and the words ParseFloat knows. The
// kernel's Parse reads the character form of IEEE 754-2019 clause 5.12,
// which has none of the Go syntax; a decimal numeral is passed to it with
// its underscores removed, and the value of a hexadecimal one is worked out
// here, exactly, and passed as a decimal numeral that rounds the same way.

// readDecimal converts s, in the syntax of ParseFloat, to a decimal value
// of the given bitSize, rounded once to nearest even. The status is the
// kernel's.
func readDecimal(s string, bitSize int) (decimal128, bid.Status) {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	rest := s[i:]
	switch {
	case len(rest) > 0 && rest[0] != '.' && (rest[0] < '0' || rest[0] > '9'):
		// A word. As for ParseFloat, an infinity may have a sign and a NaN
		// may not, and there is no signaling NaN.
		inf := foldEqual(rest, "inf") || foldEqual(rest, "infinity")
		if !inf && (i > 0 || !foldEqual(rest, "nan")) {
			return 0, bid.Syntax
		}
		return kernelParse(s, bitSize)
	case len(rest) > 2 && rest[0] == '0' && (rest[1] == 'x' || rest[1] == 'X'):
		if !underscoreOK(s) {
			return 0, bid.Syntax
		}
		return readHexDecimal(i > 0 && s[0] == '-', removeUnderscores(rest[2:]), bitSize)
	}
	if !underscoreOK(s) {
		return 0, bid.Syntax
	}
	return kernelParse(removeUnderscores(s), bitSize)
}

// parseDecimalBinExp converts the decimal numeral mant, which has no
// exponent part, multiplied by 2**exp2, to a decimal value of the given
// bitSize, rounded once. It reads fmt's scanning form "1.2p4", which fmt
// reads for floats as ParseFloat of the mantissa scaled by math.Ldexp; a
// decimal is not scaled by a power of two exactly, so the product is
// formed exactly here and rounded once.
//
//go:linkname parseDecimalBinExp
func parseDecimalBinExp(mant string, exp2 int, bitSize int) (decimal128, error) {
	d, st := readDecimalBinExp(mant, exp2, bitSize)
	if st&bid.Syntax != 0 {
		return 0, syntaxError("ParseDecimal", mant)
	}
	if st&bid.Overflow != 0 {
		return d, rangeError("ParseDecimal", mant)
	}
	return d, nil
}

func readDecimalBinExp(mant string, exp2 int, bitSize int) (decimal128, bid.Status) {
	neg := false
	s := mant
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	if !underscoreOK(mant) {
		return 0, bid.Syntax
	}
	s = removeUnderscores(s)
	var digs []byte
	frac, point := 0, false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '.' && !point:
			point = true
		case '0' <= c && c <= '9':
			digs = append(digs, c)
			if point {
				frac++
			}
		default:
			return 0, bid.Syntax
		}
	}
	if len(digs) == 0 {
		return 0, bid.Syntax
	}
	m := natFromDecimal(digs)
	if len(m) == 0 {
		return signedZero(neg, bitSize)
	}
	return scaledDecimal(neg, m, int64(exp2), int64(-frac), bitSize)
}

// readHexDecimal converts the hexadecimal mantissa and binary exponent in s
// (what follows the "0x" of a hexadecimal floating-point literal, without
// underscores) to a decimal value. The value is exact in binary, and it is
// rounded once to the decimal format. A hexadecimal literal has no quantum,
// so the result is chosen as a constant's is (see "Conversions" in the
// specification): an exact value that is an integer that fits has exponent
// 0, any other exact value has the smallest coefficient that holds it, and
// an inexact one keeps every digit of the rounded result.
func readHexDecimal(neg bool, s string, bitSize int) (decimal128, bid.Status) {
	var digs []byte // significant hexadecimal digits, as values
	frac := 0       // digits after the point, leading zeros included
	point, saw := false, false
	i := 0
	for ; i < len(s); i++ {
		c := s[i]
		var v byte
		switch {
		case c == '.':
			if point {
				return 0, bid.Syntax
			}
			point = true
			continue
		case '0' <= c && c <= '9':
			v = c - '0'
		case 'a' <= c|0x20 && c|0x20 <= 'f':
			v = c | 0x20 - 'a' + 10
		default:
			goto exponent
		}
		saw = true
		if point {
			frac++
		}
		if v != 0 || len(digs) > 0 {
			digs = append(digs, v)
		}
	}
exponent:
	if !saw || i == len(s) || s[i]|0x20 != 'p' {
		return 0, bid.Syntax // ParseFloat requires the exponent of a hexadecimal mantissa
	}
	i++
	eneg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		eneg = s[i] == '-'
		i++
	}
	if i == len(s) {
		return 0, bid.Syntax
	}
	var e int64
	for ; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, bid.Syntax
		}
		if e < maxExp2 {
			e = e*10 + int64(c-'0')
		}
	}
	if eneg {
		e = -e
	}

	// The mantissa as a natural number, four bits a digit.
	m := make(nat, (len(digs)+15)/16)
	for k := range digs {
		d := digs[len(digs)-1-k]
		m[k/16] |= uint64(d) << (4 * (k % 16))
	}
	m = m.norm()
	if len(m) == 0 {
		return signedZero(neg, bitSize)
	}
	return scaledDecimal(neg, m, e-4*int64(frac), 0, bitSize)
}

// maxExp2 bounds the binary exponents readHexDecimal and parseDecimalBinExp
// keep: far outside both formats, and far from the limits of an int64.
const maxExp2 = 1 << 40

// signedZero returns the zero of sign neg with exponent 0.
func signedZero(neg bool, bitSize int) (decimal128, bid.Status) {
	if neg {
		return kernelParse("-0", bitSize)
	}
	return kernelParse("0", bitSize)
}

// scaledDecimal returns m × 2^e2 × 10^q10, for a natural number m that is not
// zero and q10 <= 0, rounded once to nearest even to the decimal format, with
// the quantum chosen as readHexDecimal says.
//
// The result needs the leading digits of the value and whether any digit
// below them is not zero, and no more. scaledDecimal works out the first 38
// digits or so, floor(v / 10^t) for a t from the magnitude, and whether the
// division is exact, which every rounding depends on, and hands the kernel
// the digits with one more non-zero digit if it is not. The work is in
// proportion to the length of m: m is multiplied or divided by a power of ten
// that the format's range bounds, and shifted.
func scaledDecimal(neg bool, m nat, e2, q10 int64, bitSize int) (decimal128, bid.Status) {
	sign := ""
	if neg {
		sign = "-"
	}
	e2 = min(max(e2, -maxExp2), maxExp2)
	tz := m.trailingZeros()
	m = m.shr(uint(tz))
	e2 += int64(tz)

	// The exponent of the leading digit is lead or lead+1: the value is in
	// [2^(b-1), 2^b) × 10^q10, and 1292913986 / 2^32 is log10(2) to within
	// 3e-11, so that the error over the range of b clamped here stays below
	// a tenth of a digit. A b beyond the clamp is a value beyond both
	// formats, unless a mantissa of a billion digits brings it back.
	b := min(max(int64(m.bitLen())+e2, -1<<31), 1<<31)
	lead := (b-1)*1292913986>>32 + q10
	switch {
	case lead > maxDecimalExp:
		return kernelParse(sign+"1e99999", bitSize) // beyond the largest finite value
	case lead < -maxDecimalExp:
		return kernelParse(sign+"1e-99999", bitSize) // below the smallest subnormal
	}
	t := lead - 38 // the exponent of the last digit worked out
	n := m
	sticky := false
	if u := q10 - t; u > 0 {
		n = n.mulPow10(int(u))
	}
	if e2 >= 0 {
		n = n.shl(uint(e2))
	} else {
		var s bool
		n, s = n.shrSticky(uint(-e2))
		sticky = sticky || s
	}
	if u := q10 - t; u < 0 {
		var s bool
		n, s = n.divPow10(int(-u))
		sticky = sticky || s
	}

	digits := n.decimal()
	exp := t
	if sticky {
		// Below the last digit, a non-zero fraction: one more digit, also
		// not zero, is a numeral between the same two neighbours.
		digits = append(digits, '1')
		exp--
	} else {
		for len(digits) > 1 && digits[len(digits)-1] == '0' {
			digits = digits[:len(digits)-1]
			exp++
		}
		prec := bid.Precision128
		if bitSize == 64 {
			prec = bid.Precision64
		}
		if exp > 0 && int64(len(digits))+exp <= int64(prec) {
			for ; exp > 0; exp-- {
				digits = append(digits, '0')
			}
		}
	}
	return kernelParse(sign+string(digits)+"e"+Itoa(int(exp)), bitSize)
}

// maxDecimalExp is beyond the exponent of the leading digit of any finite
// decimal128 (6144) and of the smallest subnormal (-6176), with room for the
// digits a rounding can carry.
const maxDecimalExp = 6250

// kernelParse is the kernel's Parse of the given bitSize, which reads the
// character form, to nearest even.
func kernelParse(s string, bitSize int) (decimal128, bid.Status) {
	if bitSize == 64 {
		x, st := bid.Parse64(s, bid.ToNearestEven)
		return decimal128(decimal.BID64frombits(x)), st
	}
	x, st := bid.Parse128(s, bid.ToNearestEven)
	return decimal.BID128frombits(x.Hi, x.Lo), st
}

// underscoreOK reports whether the underscores in s are allowed, as
// ParseFloat allows them: only between digits, or between a base prefix and
// a digit.
func underscoreOK(s string) bool {
	// saw is the class of the last character: '^' for the beginning of the
	// number, '0' for a digit or base prefix, '_' for an underscore and '!'
	// for anything else.
	saw := '^'
	i := 0
	if len(s) >= 1 && (s[0] == '-' || s[0] == '+') {
		s = s[1:]
	}
	hex := false
	if len(s) >= 2 && s[0] == '0' && (s[1]|0x20 == 'b' || s[1]|0x20 == 'o' || s[1]|0x20 == 'x') {
		i = 2
		saw = '0'
		hex = s[1]|0x20 == 'x'
	}
	for ; i < len(s); i++ {
		if '0' <= s[i] && s[i] <= '9' || hex && 'a' <= s[i]|0x20 && s[i]|0x20 <= 'f' {
			saw = '0'
			continue
		}
		if s[i] == '_' {
			if saw != '0' {
				return false
			}
			saw = '_'
			continue
		}
		if saw == '_' {
			return false
		}
		saw = '!'
	}
	return saw != '_'
}

// removeUnderscores returns s without its underscores.
func removeUnderscores(s string) string {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '_' {
			n++
		}
	}
	if n == 0 {
		return s
	}
	b := make([]byte, 0, len(s)-n)
	for i := 0; i < len(s); i++ {
		if s[i] != '_' {
			b = append(b, s[i])
		}
	}
	return string(b)
}

// foldEqual reports whether s is word, a lower-case ASCII word, in any case.
func foldEqual(s, word string) bool {
	if len(s) != len(word) {
		return false
	}
	for i := range len(s) {
		if s[i]|0x20 != word[i] {
			return false
		}
	}
	return true
}

// A nat is a natural number, its words least significant first, with no
// most significant zero word. It is what the hexadecimal reader needs and
// no more; strconv cannot use math/big, which imports it.
type nat []uint64

func (z nat) norm() nat {
	for len(z) > 0 && z[len(z)-1] == 0 {
		z = z[:len(z)-1]
	}
	return z
}

func (z nat) bitLen() int {
	if len(z) == 0 {
		return 0
	}
	return (len(z)-1)*64 + bits.Len64(z[len(z)-1])
}

// trailingZeros returns the number of zero bits below the lowest one; z is
// not zero.
func (z nat) trailingZeros() int {
	i := 0
	for z[i] == 0 {
		i++
	}
	return i*64 + bits.TrailingZeros64(z[i])
}

// shl returns z × 2^s.
func (z nat) shl(s uint) nat {
	w, b := int(s/64), s%64
	r := make(nat, len(z)+w+1)
	for i, x := range z {
		r[i+w] |= x << b
		if b > 0 {
			r[i+w+1] = x >> (64 - b)
		}
	}
	return r.norm()
}

// shr returns z / 2^s for an s no more than the trailing zeros of z.
func (z nat) shr(s uint) nat {
	r, _ := z.shrSticky(s)
	return r
}

// shrSticky returns floor(z / 2^s) and whether the bits shifted out are not
// all zero.
func (z nat) shrSticky(s uint) (nat, bool) {
	w, b := int(min(s/64, uint(len(z)))), s%64
	sticky := false
	for _, x := range z[:w] {
		sticky = sticky || x != 0
	}
	if w == len(z) {
		return nil, sticky
	}
	if b > 0 {
		sticky = sticky || z[w]<<(64-b) != 0
	}
	r := make(nat, len(z)-w)
	for i := range r {
		r[i] = z[i+w] >> b
		if b > 0 && i+w+1 < len(z) {
			r[i] |= z[i+w+1] << (64 - b)
		}
	}
	return r.norm(), sticky
}

// mulAdd returns z × m + a.
func (z nat) mulAdd(m, a uint64) nat {
	r := make(nat, len(z)+1)
	carry := a
	for i, x := range z {
		hi, lo := bits.Mul64(x, m)
		lo, c := bits.Add64(lo, carry, 0)
		r[i], carry = lo, hi+c
	}
	r[len(z)] = carry
	return r.norm()
}

// divSmall returns floor(z / d) and the remainder.
func (z nat) divSmall(d uint64) (nat, uint64) {
	r := make(nat, len(z))
	var rem uint64
	for i := len(z) - 1; i >= 0; i-- {
		r[i], rem = bits.Div64(rem, z[i], d)
	}
	return r.norm(), rem
}

// pow10Word is 10^19, the largest power of ten in a word.
const pow10Word = 1e19

// mulPow10 returns z × 10^k.
func (z nat) mulPow10(k int) nat {
	for ; k >= 19; k -= 19 {
		z = z.mulAdd(pow10Word, 0)
	}
	return z.mulAdd(uint64pow10[k], 0)
}

// divPow10 returns floor(z / 10^k) and whether the remainder is not zero.
func (z nat) divPow10(k int) (nat, bool) {
	sticky := false
	var rem uint64
	for ; k >= 19 && len(z) > 0; k -= 19 {
		z, rem = z.divSmall(pow10Word)
		sticky = sticky || rem != 0
	}
	if len(z) == 0 {
		return z, sticky
	}
	if k >= 19 {
		return nil, sticky
	}
	z, rem = z.divSmall(uint64pow10[k])
	return z, sticky || rem != 0
}

// decimal returns the decimal digits of z, "0" for zero.
func (z nat) decimal() []byte {
	if len(z) == 0 {
		return []byte{'0'}
	}
	var chunks []uint64 // groups of 19 digits, least significant first
	for len(z) > 0 {
		var c uint64
		z, c = z.divSmall(pow10Word)
		chunks = append(chunks, c)
	}
	b := AppendUint(nil, chunks[len(chunks)-1], 10)
	for i := len(chunks) - 2; i >= 0; i-- {
		var buf [19]byte
		c := chunks[i]
		for j := 18; j >= 0; j-- {
			buf[j] = byte(c%10) + '0'
			c /= 10
		}
		b = append(b, buf[:]...)
	}
	return b
}

// natFromDecimal returns the value of the decimal digits digs.
func natFromDecimal(digs []byte) nat {
	var z nat
	for len(digs) > 0 {
		k := min(len(digs), 19)
		var v uint64
		for _, c := range digs[:k] {
			v = v*10 + uint64(c-'0')
		}
		z = z.mulAdd(uint64pow10[k], v)
		digs = digs[k:]
	}
	return z.norm()
}

// uint64pow10 holds 10^0 to 10^19.
var uint64pow10 = [20]uint64{
	1e00, 1e01, 1e02, 1e03, 1e04, 1e05, 1e06, 1e07, 1e08, 1e09,
	1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18, 1e19,
}
