// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// Conversion between decimal values and character sequences
// (IEEE 754-2019 clause 5.4.2 and 5.12, convertFromDecimalCharacter and
// convertToDecimalCharacter). The character form is the to-scientific-string
// form of the General Decimal Arithmetic specification, which the standard
// adopts; it is defined in "Values, cohorts and the character form" in the
// package documentation, and the exponent and Status rules of Parse are in
// "Character conversions". The form preserves the quantum, so a string
// converts back to the same bit pattern.

// MaxLen64 and MaxLen128 bound the number of bytes Append64 and Append128
// write. The longest forms are the plain notation of a full-precision
// coefficient with the smallest adjusted exponent that avoids exponential
// notation, "-0.00000ddd…d" (a sign, "0.", five zeros and Precision
// digits), and the exponential notation "-d.ddd…dE-dddd" (a sign,
// Precision digits, a point, "E", a sign and the exponent digits), which
// is never longer.
const (
	MaxLen64  = 1 + len("0.") + 5 + Precision64
	MaxLen128 = 1 + len("0.") + 5 + Precision128
)

// Append64 appends the character form of x to dst and returns the extended
// buffer (IEEE 754-2019 clause 5.12, convertToDecimalCharacter). It does
// not allocate when dst has MaxLen64 bytes of spare capacity. The output
// is exactly what Parse64 reads back to the canonical encoding of x.
func Append64(dst []byte, x uint64) []byte {
	return append64(dst, x)
}

// Append128 is Append64 for decimal128; it needs MaxLen128 bytes of spare
// capacity to avoid allocating.
func Append128(dst []byte, x Uint128) []byte {
	return append128(dst, x)
}

// Parse64 converts the character sequence s to decimal64, rounded once in
// mode from the exact value s denotes (IEEE 754-2019 clause 5.4.2 and
// 5.12, convertFromDecimalCharacter).
//
// The accepted grammar is that of clause 5.12: an optional sign, then
// either a decimal numeral (digits with an optional point among them, at
// least one digit, followed by an optional exponent part "e" or "E", an
// optional sign and at least one digit) or one of the words "inf",
// "infinity", "nan" and "snan", all case-insensitive. Nothing else is
// accepted: no surrounding space, no underscores, no hexadecimal forms, no
// NaN payload, and no characters after a special word. Callers that
// accept Go literal syntax normalize it before calling.
//
// The numeral may have any number of digits and an exponent part of any
// length; the value is rounded once. Status reports Syntax (the result is
// then NaN64), and otherwise Exact, Inexact, Overflow and Underflow as
// Round64 reports them.
func Parse64(s string, mode Mode) (uint64, Status) {
	return parse64(s, mode)
}

// Parse128 is Parse64 for decimal128.
func Parse128(s string, mode Mode) (Uint128, Status) {
	return parse128(s, mode)
}

// The width-independent half of the conversions (rule 7 of "Width
// parameterisation" in the package documentation). string_tmpl.go
// instantiates the part that mentions a coefficient: it unpacks an operand
// and hands the digits to appendDecimal, and it hands the output of
// scanDecimal to the rounding routine. Everything about the character form
// itself is here, once, for both widths.

const (
	// u64Digits is the number of decimal digits that every uint64 holds:
	// 10^19 is the largest power of ten below 2^64. A coefficient of either
	// width is split into two words of that many digits, the low one
	// holding exactly u64Digits.
	u64Digits = 19

	// digitBufLen is the room writeDigits needs: two words of u64Digits
	// digits. A decimal128 coefficient has 34 digits.
	digitBufLen = 2 * u64Digits

	// maxScanDigits bounds the number of significant digits scanDecimal
	// keeps, which are accumulated in a Uint128: 10^38 is the largest power
	// of ten below 2^128. The widths keep Precision+1 digits (at most 35).
	maxScanDigits = 38

	// expSaturate is the value at which the digits of an exponent part
	// stop counting. It is far outside any result (the exponent of a
	// finite value lies within a few thousand places of zero) and far
	// enough from the limits of an int64 that adding the length of the
	// string, which cannot approach 2^58 bytes, to it cannot overflow.
	expSaturate = 1 << 58

	// expClamp bounds the exponent scanDecimal reports, so that it is an
	// int on every platform. The digits it reports (at most 38) cannot
	// bring a value with an exponent that large back into range.
	expClamp = 1 << 30
)

// writeDigits writes the decimal digits of hi × 10^19 + lo to the end of buf,
// without leading zeros ("0" for zero), and returns the number written.
// Bytes before the digits are unchanged. buf must have room for digitBufLen
// digits, or for the digits of the value.
//
// The low word holds exactly u64Digits digits, leading zeros included: two
// chunks of eight below three digits. Each chunk converts on its own
// (eightASCII) and is stored as one word, and the high word converts beside
// them, so the dependent steps are the two divisions that cut the chunks
// out, not the nineteen that a digit at a time would take. The chunks pay
// here because the low word's length is fixed: no branch depends on it.
func writeDigits(buf []byte, hi, lo uint64) int {
	if hi == 0 {
		return writeWordDigits(buf, lo)
	}
	const chunk = 100_000_000 // eight digits
	i := len(buf)
	q1 := lo / chunk
	q2 := lo / (chunk * chunk) // below 1000: the top three digits of the word
	putEight(buf[i-8:], eightASCII(lo-q1*chunk))
	putEight(buf[i-16:], eightASCII(q1-q2*chunk))
	i -= 16
	for range 3 {
		i--
		buf[i] = byte('0' + q2%10)
		q2 /= 10
	}
	return u64Digits + writeWordDigits(buf[:i], hi)
}

// writeWordDigits writes a word backwards into the end of buf without
// a temporary buffer or a digit-counting pass. It returns the digit count.
// cSplitW establishes lo < 10^19, so u64Digits bytes suffice even for zero.
// A digit at a time is the right shape here: the word is usually short
// (money has six digits), and a conversion in chunks of eight costs its
// three multiplications whatever the length and a branch on the length
// that mixed operands mispredict (see "Digit conversion" in the package
// documentation).
func writeWordDigits(buf []byte, lo uint64) int {
	i := len(buf)
	for {
		i--
		buf[i] = byte('0' + lo%10)
		lo /= 10
		if lo == 0 {
			break
		}
	}
	return len(buf) - i
}

// eightASCII returns the eight decimal digits of v, which is below 10^8,
// as ASCII bytes packed in a word with the most significant digit in the
// lowest byte, so that putEight, which stores the word little-endian, lays
// them out in reading order. Leading zeros are kept.
//
// It is three multiplications instead of seven divisions: the value is
// split into halves of four digits, each half into pairs, each pair into
// digits, with the parts of one level side by side in the lanes of one
// word so that a single multiply-and-shift divides them all. At each level
// the quotient of a lane is recovered by a shift that lands it in the low
// bits of its own lane, and the bits that the lane above shifts down are
// masked off, which is sound because every quotient is below the mask and
// every product stays inside its lane:
//
//   - Halves. t = v / 10^4 and u = v - 10^4 t are below 10^4. z holds t in
//     bits 0–31 and u in bits 32–63.
//   - Pairs. For q < 10^4, q / 100 = (q × 10486) >> 20: 10486 = ⌈2^20 / 100⌉,
//     and the error q × (10486 × 100 − 2^20) / 2^20 = 24 q / 2^20 is below
//     1 for q < 43691. q × 10486 < 2^27 stays inside a 32-bit lane; after
//     the shift the quotient is in bits 0–6 of the lane (it is below 100)
//     and whatever the upper lane shifted down starts at bit 12, so the
//     mask 0x7F per lane keeps the quotient alone. The remainder is
//     q − 100 × quotient, also inside the lane. y then holds the four
//     pairs in 16-bit lanes, most significant lowest: t / 100, t mod 100,
//     u / 100, u mod 100.
//   - Digits. For p < 100, p / 10 = (p × 103) >> 10: 103 = ⌈2^10 / 10⌉ and
//     the error p × (103 × 10 − 2^10) / 2^10 = 6 p / 2^10 is below 1 for
//     p < 171. p × 103 < 2^14 stays inside a 16-bit lane; after the shift
//     the tens digit is in bits 0–3 of its lane and the lane above lands at
//     bit 6, so the mask 0xF per lane keeps it. The units are p − 10 × tens.
//     Tens in the low byte of each lane, units in the high byte, plus '0'
//     in every byte, is the result.
//
// TestEightASCII checks every value below 10^8 against the division loop.
func eightASCII(v uint64) uint64 {
	t := v / 10_000
	z := t | (v-t*10_000)<<32
	hundreds := ((z * 10486) >> 20) & 0x0000_007F_0000_007F
	y := hundreds | (z-hundreds*100)<<16
	tens := ((y * 103) >> 10) & 0x000F_000F_000F_000F
	return tens | (y-tens*10)<<8 | 0x3030_3030_3030_3030
}

// putEight stores w little-endian in b[0:8]: the compiler emits one store.
func putEight(b []byte, w uint64) {
	_ = b[7]
	b[0] = byte(w)
	b[1] = byte(w >> 8)
	b[2] = byte(w >> 16)
	b[3] = byte(w >> 24)
	b[4] = byte(w >> 32)
	b[5] = byte(w >> 40)
	b[6] = byte(w >> 48)
	b[7] = byte(w >> 56)
}

// appendDecimal appends the to-scientific-string form of the finite value
// with the sign neg, the coefficient digits digs (at least one, no leading
// zero unless the coefficient is zero and then just "0") and the exponent
// exp, as defined under "Values, cohorts and the character form".
func appendDecimal(dst []byte, neg bool, digs []byte, exp int) []byte {
	if neg {
		dst = append(dst, '-')
	}
	n := len(digs)
	// adj is the adjusted exponent: the power of ten of the leading digit.
	adj := exp + n - 1
	if exp <= 0 && adj >= -6 {
		// Plain notation.
		switch {
		case exp == 0:
			dst = append(dst, digs...)
		case n > -exp:
			dst = append(dst, digs[:n+exp]...)
			dst = append(dst, '.')
			dst = append(dst, digs[n+exp:]...)
		default:
			dst = append(dst, '0', '.')
			for range -exp - n {
				dst = append(dst, '0')
			}
			dst = append(dst, digs...)
		}
		return dst
	}
	// Exponential notation.
	dst = append(dst, digs[0])
	if n > 1 {
		dst = append(dst, '.')
		dst = append(dst, digs[1:]...)
	}
	dst = append(dst, 'E', '+')
	if adj < 0 {
		dst[len(dst)-1] = '-'
		adj = -adj
	}
	var tmp [8]byte // a decimal128 adjusted exponent has at most four digits
	i := len(tmp)
	for {
		i--
		tmp[i] = byte('0' + adj%10)
		adj /= 10
		if adj == 0 {
			break
		}
	}
	return append(dst, tmp[i:]...)
}

// A scanKind says what scanDecimal found.
type scanKind uint8

const (
	scanSyntax  scanKind = iota // not in the grammar of Parse64
	scanInf                     // an infinity, of sign numeral.neg
	scanNaN                     // a NaN, quiet or signaling
	scanNumeral                 // a decimal numeral
)

// A numeral is the outcome of scanDecimal. For kind scanNumeral the exact
// value of the string is (-1)^neg × (mant + f) × 10^exp, with f = 0 unless
// sticky and otherwise 0 < f < 1, in the form that the rounding routine
// takes. Only neg is meaningful for scanInf.
type numeral struct {
	kind   scanKind
	neg    bool
	mant   Uint128
	exp    int
	sticky bool
}

// scanDecimal parses s in the grammar documented at Parse64, whatever its
// length, keeping the first keep significant digits in mant (keep is at
// most maxScanDigits). The digits beyond them are not lost: each raises
// exp by one place, and if any of them is non-zero sticky is set, so that
// mant, exp and sticky describe the numeral exactly. When sticky is set
// mant has keep digits, which the caller chooses to be more than the
// precision, so that the rounding routine sees a round digit and the
// sticky bit stands only for what lies below it.
//
// Leading zeros are not significant digits and take no room in mant, so
// "0.000…0001" with any number of zeros keeps its one digit. The exponent
// part may have any number of digits; once its value exceeds expSaturate it
// stays there. The exponent reported is exact, the written exponent less
// the digits after the point plus the digits dropped, computed in an int64
// that the saturated value cannot overflow, and then clamped to ±expClamp:
// a numeral of at most keep significant digits whose exponent lies beyond
// that is far outside both formats, so the clamp changes no result. A long
// run of digits can therefore compensate for an exponent exactly as far as
// the string is long, and no further.
func scanDecimal(s string, keep int) numeral {
	var r numeral
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		r.neg = s[i] == '-'
		i++
	}
	if i < len(s) && s[i] != '.' && s[i]-'0' > 9 {
		// A word, if anything.
		switch rest := s[i:]; {
		case foldEqual(rest, "inf"), foldEqual(rest, "infinity"):
			r.kind = scanInf
		case foldEqual(rest, "nan"), foldEqual(rest, "snan"):
			r.kind = scanNaN
		}
		return r
	}

	// Significand: digits with at most one point, at least one digit.
	var digits, fraction, kept, dropped int
	var mant Uint128
	point, sticky := false, false
	wordKeep := min(keep, u64Digits)
	// Runs of eight digits go to scanEights, at the three places where a
	// long run can start: the first digit, the digit after the point, and a
	// digit beyond keep, and only when eight digits follow, which costs a
	// load and is checked before the call. The byte loop leaves itself to
	// make the call (run) and is entered again after it, so that it holds
	// no call and keeps its state in registers: short numerals pay only the
	// checks.
	run := i+8 <= len(s) && eightDigits(load8(s, i))
	for {
		if run {
			j := i
			j, kept, mant, dropped, sticky = scanEights(s, j, kept, keep, wordKeep, mant, dropped, sticky)
			digits += j - i
			if point {
				fraction += j - i
			}
			i, run = j, false
		}
	bytes:
		for ; i < len(s); i++ {
			c := s[i]
			if c == '.' {
				if point {
					return numeral{}
				}
				point = true
				if i+9 <= len(s) && eightDigits(load8(s, i+1)) {
					i, run = i+1, true
					break
				}
				continue
			}
			d := c - '0'
			if d > 9 {
				break
			}
			digits++
			if point {
				fraction++
			}
			switch {
			case kept == 0 && d == 0:
				// A leading zero.
			case kept < wordKeep:
				// At most 19 significant digits are below 10^19 and fit
				// one word. No carry or high-word multiply is needed.
				mant.Lo = mant.Lo*10 + uint64(d)
				kept++
			case kept < keep:
				mant, _ = u128Mul64(mant, 10)
				mant, _ = u128Add(mant, Uint128{Lo: uint64(d)})
				kept++
			default:
				dropped++
				sticky = sticky || d != 0
				if i+9 <= len(s) && eightDigits(load8(s, i+1)) {
					i, run = i+1, true
					break bytes
				}
			}
		}
		if !run {
			break
		}
	}
	r.mant, r.sticky = mant, sticky
	if digits == 0 {
		return numeral{}
	}

	// Exponent part: e or E, an optional sign, at least one digit.
	var written int64
	if i < len(s) && s[i]|0x20 == 'e' {
		i++
		eneg := false
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			eneg = s[i] == '-'
			i++
		}
		start := i
		for ; i < len(s) && s[i]-'0' <= 9; i++ {
			if written < expSaturate {
				written = written*10 + int64(s[i]-'0')
			}
		}
		if i == start {
			return numeral{}
		}
		written = min(written, expSaturate)
		if eneg {
			written = -written
		}
	}
	if i != len(s) {
		return numeral{}
	}
	exp := written - int64(fraction) + int64(dropped)
	r.exp = int(min(max(exp, -expClamp), expClamp))
	r.kind = scanNumeral
	return r
}

// scanEights is the eight-digit step of scanDecimal's significand loop. It
// takes digits from s[i:] eight at a time while all eight fall in one phase
// of that loop, whose counts and value it reproduces exactly: leading zeros
// (which it skips whatever their number, up to the first significant digit
// of a run of eight digits), digits kept in the word mant.Lo while kept
// stays at most wordKeep, digits kept in mant while it stays at most keep,
// and digits beyond keep, which raise dropped and set sticky if any is not
// zero. It returns the index of the first byte it did not take, which the
// byte loop takes next: a non-digit, the end of fewer than eight digits, or
// the first of eight that straddle keep. Every byte from i to that index is
// a digit, so the caller adds their number to its digit counts.
//
// It is a call and not part of the byte loop so that the loop, which short
// numerals never leave, keeps its registers.
//
//go:noinline
func scanEights(s string, i, kept, keep, wordKeep int, mant Uint128, dropped int, sticky bool) (int, int, Uint128, int, bool) {
	for i+8 <= len(s) {
		u := load8(s, i)
		if !eightDigits(u) {
			break
		}
		switch {
		case kept == 0 && u&0xFF == '0':
			// Leading zeros: skip all eight, or up to the first other
			// digit. The bytes are digits, so one that is not '0' differs
			// from it in its low nibble, and the lowest set bit of the
			// difference lies in the first such byte.
			if u == asciiZeros {
				i += 8
			} else {
				i += bits.TrailingZeros64(u^asciiZeros) / 8
			}
			continue
		case kept < keep && kept+8 > keep:
			return i, kept, mant, dropped, sticky
		case kept+8 <= wordKeep:
			mant.Lo = mant.Lo*1e8 + eightDigitsValue(u)
			kept += 8
		case kept < keep:
			mant, _ = u128Mul64(mant, 1e8)
			mant, _ = u128Add(mant, Uint128{Lo: eightDigitsValue(u)})
			kept += 8
		default:
			dropped += 8
			sticky = sticky || u != asciiZeros
		}
		i += 8
	}
	return i, kept, mant, dropped, sticky
}

// load8 returns s[i:i+8] as a word, s[i] in the low byte, on every
// platform; the compiler makes it one load where it can.
func load8(s string, i int) uint64 {
	s = s[i : i+8]
	return uint64(s[0]) | uint64(s[1])<<8 | uint64(s[2])<<16 | uint64(s[3])<<24 |
		uint64(s[4])<<32 | uint64(s[5])<<40 | uint64(s[6])<<48 | uint64(s[7])<<56
}

// asciiZeros is eight '0' bytes in one word.
const asciiZeros = 0x3030303030303030

// eightDigits reports whether each byte of u is an ASCII digit (Lemire,
// "Fast number parsing", is_made_of_eight_digits_fast): a byte is a digit
// exactly when its high nibble is 3 and adding 6 to it leaves the high
// nibble 3, that is, when its low nibble is at most 9. The first term puts
// the high nibble of every byte of u in the high nibble of the result,
// unchanged by the or, so equality needs every byte below 0x40; for such
// bytes adding 6 carries out of none of them.
func eightDigits(u uint64) bool {
	return u&0xF0F0F0F0F0F0F0F0|(u+0x0606060606060606)&0xF0F0F0F0F0F0F0F0>>4 == 0x3333333333333333
}

// eightDigitsValue returns the value of the eight ASCII digits in u, the
// first digit in the low byte, as Lemire's parse_eight_digits_unrolled:
// three multiply-and-shift steps combine pairs of digits, then pairs of
// pairs, then the two halves. Each product of a step stays within its lane.
func eightDigitsValue(u uint64) uint64 {
	u = (u & 0x0F0F0F0F0F0F0F0F) * (10<<8 + 1) >> 8
	u = (u & 0x00FF00FF00FF00FF) * (100<<16 + 1) >> 16
	return uint64(uint32((u & 0x0000FFFF0000FFFF) * (10000<<32 + 1) >> 32))
}

// foldEqual reports whether s is word with each letter in either case.
// word is lower case ASCII letters only: or-ing 0x20 maps the two cases of
// a letter to one and maps no other byte to a letter.
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
