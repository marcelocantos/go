// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

import "math/bits"

// The long values: a finite canonical decimal128 with a coefficient of
// nine digits or more, which is every coefficient PutShort128 does not
// take. PutLong128 writes Go's canonical form of one straight into the
// caller's buffer; see "Digit conversion" in the package documentation.

// LongLen128 is the room PutLong128 is given: the longest form it writes,
// a sign, 34 digits, a point and "e+6111", which is MaxLen128.
const LongLen128 = 42

// longBody is the form after its sign, and longDigits the room for the
// digits before they are laid out: forty characters, and slack that lets
// an offset of six bits be a proof.
const (
	longBody   = LongLen128 - 1
	longDigits = 72
)

// IsLong128 reports whether x is finite and canonical with a coefficient of
// at least 10^8: a value PutLong128 formats.
func IsLong128(x Uint128) bool {
	c, first := longCoeff128(x)
	if !first {
		return false
	}
	if c.Hi == 0 {
		return c.Lo >= shortCoeff
	}
	return !u128GtMaxCoeff(c)
}

// PutLong128 writes the character form of x, as Append128 gives it, to the
// start of buf and returns its length, for an x that IsLong128 accepts; for
// any other x what it writes is unspecified. It writes exactly the bytes of
// the result, none after them, so buf may be the spare capacity of a buffer
// whose later bytes are in use.
//
// Three branches depend on the value, and operands of one kind take each
// one way: whether the coefficient fits sixteen digits, whether the form is
// exponential, and then whether the exponent has two digits. The work is in
// three parts.
//
// Digits. The coefficient is cut into five lanes of eight digits, the first
// holding at most two, and eightPairs converts each: the forty characters
// of the coefficient with zeros on its left, stored in d. A coefficient of
// n digits is the last n of them, so its character i is d[40-n+i], and any
// eight consecutive characters are one load. A coefficient of sixteen
// digits or fewer is the last two lanes alone.
//
// Layout. The form is a sign, in plain notation below one a lead of "0."
// and zeros, the digits with a point after the first p of them, and in
// exponential notation an exponent: p is 1 in exponential notation, the
// count of digits before the point when it falls inside the coefficient,
// and 0, with no point, for an integer and after a lead. The count after
// the point is f.
//
// Stores. The form after its sign is laid out from the start of a view of
// buf one byte on when the sign is set. Every store of digits is a whole
// word and lies inside the result. The digits before the point are stored
// from the left, as the words of the coefficient from its start; those
// after it from the right, as the lanes themselves, which end where the
// digits end, and the eight that follow the point. A word that would cross
// an end of its part is not stored there: it goes to a scratch buffer by a
// selected address, and the bytes it would have supplied come from the
// last word stored, the final eight bytes of the digits, which holds the
// end of each part that reaches it: the last lane's characters after the
// point, and the same lane a byte further on before it. The point is
// stored after the words it overlaps.
func PutLong128(buf *[LongLen128]byte, x Uint128) int {
	const (
		e8  = 100_000_000
		e16 = e8 * e8
	)
	c, _ := longCoeff128(x)
	var d [longDigits]byte
	var l uint64
	var n int
	wide := c.Hi != 0 || c.Lo >= e16
	if !wide {
		// Sixteen digits or fewer: two lanes, and nothing reads the
		// three before them.
		l = c.Lo
		n = wordDigits(l)
	} else {
		// c = h × 10^16 + l by the division of two words by one of
		// Möller and Granlund ("Improved division by invariant
		// integers", algorithm 4): the divisor is 10^16 with its
		// leading bit moved to the top of a word, ten places, and c is
		// below 2^113, so the dividend moved as far has a high word
		// below the divisor. The candidate quotient is right or one
		// too large as the remainder compares with the fraction word,
		// which is a coin toss: that correction is a mask. The second
		// is rare.
		const (
			shift = 10
			dv    = e16 << shift
			v     = (1<<128-1)/dv - 1<<64
		)
		u1, u0 := c.Hi<<shift|c.Lo>>(64-shift), c.Lo<<shift
		q1, q0 := bits.Mul64(v, u1)
		q0, carry := bits.Add64(q0, u0, 0)
		q1 += u1 + carry + 1
		r := u0 - q1*dv
		_, lt := bits.Sub64(q0, r, 0) // 1 when r > q0
		q1 -= lt
		r += dv & -lt
		if r >= dv {
			q1++
			r -= dv
		}
		l = r >> shift
		// h, below 10^18, is two digits and two lanes.
		hq := q1 / e8
		h2 := hq / e8
		putLane(&d, 0, 0x3030_3030_3030|uint64(pairTab[h2&127])<<48)
		putLane(&d, 8, eightQuads(hq-h2*e8))
		putLane(&d, 16, eightQuads(q1-hq*e8))
		n = cDigits128(c)
	}
	l1 := l / e8
	putLane(&d, 24, eightQuads(l1))
	last := eightQuads(l - l1*e8)
	putLane(&d, 32, last)

	sign, exp := longParts128(x)
	z := uint(40 - n) // where the digits start in d
	e := exp + n - 1  // the exponent of the leading digit
	// Everything after the sign is laid out from the start of b, in which
	// an index of five bits is a proof for a word.
	buf[0] = '-' // overwritten unless the sign is set
	b := (*[longBody]byte)(buf[sign&1:])
	var scratch [longBody]byte

	if e < canonicalMinExp || e >= canonicalMaxExp || exp > 0 {
		// d.ddd…e±dd: the first digit and the point, the rest from
		// the right, and the exponent: "e", a sign and two digits or
		// more.
		f := uint(n - 1)
		s1 := 2 + f
		putTwoAt(b, 0, uint16(d[z&63])|'.'<<8)
		// The eight characters after the first, from the two lanes
		// they lie in: each is loaded as it was stored, and so
		// forwarded from its store, where one load across both would
		// wait for them to reach the cache.
		o := z + 1
		sh := 8 * (o & 7)
		g := loadEightAt(&d, o&^7)>>(sh&63) | loadEightAt(&d, o&^7+8)<<1<<((63-sh)&63)
		putEightAt(b, 2, g)
		if wide {
			putEightAt(pick(b, &scratch, f >= 32), s1-32, loadEightAt(&d, 8))
			putEightAt(pick(b, &scratch, f >= 24), s1-24, loadEightAt(&d, 16))
		}
		putEightAt(pick(b, &scratch, f >= 16), s1-16, loadEightAt(&d, 24))
		putEightAt(b, s1-8, last)
		neg := uint(e) >> intSign
		ae := (uint(e) ^ -neg) + neg
		if ae < 100 {
			// Two digits, which is every exponent a coefficient
			// meets unless its value is far from one: a pair.
			putFourAt(b, s1, 'e'|uint32('+'+2*neg)<<8|uint32(pairTab[ae&127])<<16)
			return int(sign + s1 + 4)
		}
		k := 3 + (999-ae)>>intSign // its digits, three or four
		eh := ae * 5243 >> 19      // ae / 100: ae is below 43699
		sfx := uint64('e') | uint64('+'+2*neg)<<8 |
			(uint64(pairTab[eh&127])|uint64(pairTab[(ae-eh*100)&127])<<16)>>(8*(4-k)&63)<<16
		putFourAt(b, s1, uint32(sfx))
		putTwoAt(b, s1+k, uint16(sfx>>(8*k&63)))
		return int(sign + s1 + 2 + k)
	}

	// Plain notation: exp is zero or negative and e from -4 to 20. Each
	// condition is a sign bit.
	below := uint(e) >> intSign             // below one: a lead
	inside := uint(exp) >> intSign &^ below // the point within the digits
	p := uint(e+1) & -inside
	lead := uint(1-e) & -below
	f := uint(n) - p
	m := uint(n) + inside // the digits and their point, which start at lead
	s1 := lead + m        // and end here
	// The final eight bytes: the lane where it lies after the point,
	// and the lane a byte on where it lies before.
	hi := ^uint64(0) << ((64 - 8*min(f, 8)) & 63)
	putEightAt(b, 0, 0x3030_3030_3030_2E30)
	putEightAt(b, lead, loadEightAt(&d, z))
	putEightAt(pick(b, &scratch, m >= 16), lead+8, loadEightAt(&d, z+8))
	if wide {
		putEightAt(pick(b, &scratch, m >= 24), lead+16, loadEightAt(&d, z+16))
		putEightAt(pick(b, &scratch, f >= 32), s1-32, loadEightAt(&d, 8))
		putEightAt(pick(b, &scratch, f >= 24), s1-24, loadEightAt(&d, 16))
	}
	// After every word stored from the left, which it overlaps.
	putEightAt(pick(b, &scratch, f >= 8), lead+p+inside, loadEightAt(&d, z+p))
	putEightAt(pick(b, &scratch, f >= 16), s1-16, loadEightAt(&d, 24))
	putEightAt(b, s1-8, last&hi|last>>8&^hi)
	pick(b, &scratch, inside != 0)[(lead+p)&31] = '.'
	return int(sign + s1)
}

// pairTab[p] is the two characters of p, below 100, the tens in the low
// byte: a pair as putEightAt stores it. The entries from 100 are never
// read; they make an index of seven bits a proof. It is a literal, not
// computed, so that it is data in the binary: the runtime prints with it
// before any package is initialized. TestPairTab recomputes it.
var pairTab = [128]uint16{
	0x3030, 0x3130, 0x3230, 0x3330, 0x3430, 0x3530, 0x3630, 0x3730, 0x3830, 0x3930,
	0x3031, 0x3131, 0x3231, 0x3331, 0x3431, 0x3531, 0x3631, 0x3731, 0x3831, 0x3931,
	0x3032, 0x3132, 0x3232, 0x3332, 0x3432, 0x3532, 0x3632, 0x3732, 0x3832, 0x3932,
	0x3033, 0x3133, 0x3233, 0x3333, 0x3433, 0x3533, 0x3633, 0x3733, 0x3833, 0x3933,
	0x3034, 0x3134, 0x3234, 0x3334, 0x3434, 0x3534, 0x3634, 0x3734, 0x3834, 0x3934,
	0x3035, 0x3135, 0x3235, 0x3335, 0x3435, 0x3535, 0x3635, 0x3735, 0x3835, 0x3935,
	0x3036, 0x3136, 0x3236, 0x3336, 0x3436, 0x3536, 0x3636, 0x3736, 0x3836, 0x3936,
	0x3037, 0x3137, 0x3237, 0x3337, 0x3437, 0x3537, 0x3637, 0x3737, 0x3837, 0x3937,
	0x3038, 0x3138, 0x3238, 0x3338, 0x3438, 0x3538, 0x3638, 0x3738, 0x3838, 0x3938,
	0x3039, 0x3139, 0x3239, 0x3339, 0x3439, 0x3539, 0x3639, 0x3739, 0x3839, 0x3939,
}

// eightPairs returns the eight decimal digits of v, which is below 10^8,
// packed as eightASCII packs them, and is eightASCII for a caller whose
// digits are wanted soon: the four pairs of digits are cut out of v side by
// side, three multiplications deep in eightASCII and one here, and each
// pair is its two characters by pairTab.
//
// The quotients are those of a 32-bit division by a constant: for v below
// 2^32, v / 10^6 = (v × 1125899907) >> 50, v / 10^4 = (v × 3518437209) >> 45
// and v / 100 = (v × 1374389535) >> 37, each multiplier the ceiling of its
// power of two over the divisor and each product below 2^64.
// TestEightPairs checks every value below 10^8 against eightASCII.
func eightPairs(v uint64) uint64 {
	a := v * 1125899907 >> 50
	b := v * 3518437209 >> 45
	c := v * 1374389535 >> 37
	return uint64(pairTab[a&127]) | uint64(pairTab[(b-a*100)&127])<<16 |
		uint64(pairTab[(c-b*100)&127])<<32 | uint64(pairTab[(v-c*100)&127])<<48
}

// loadEightAt returns the word putEightAt stores at d[o:o+8].
func loadEightAt(d *[longDigits]byte, o uint) uint64 {
	o &= 63
	return loadEight(d[o : o+8])
}

// loadEight returns the first eight bytes of b as putEight stores them:
// the compiler emits one load.
func loadEight(b []byte) uint64 {
	_ = b[7]
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}

// pick returns a if c and b otherwise: the address of a store that must
// not happen is that of a scratch buffer.
func pick(a, b *[longBody]byte, c bool) *[longBody]byte {
	if c {
		return a
	}
	return b
}

// putLane stores w, eight characters, at d[i:i+8].
func putLane(d *[longDigits]byte, i uint, w uint64) {
	i &= 63
	putEight(d[i:i+8], w)
}

// putEightAt stores w little-endian in b[i:i+8]. The mask changes no
// index of a store inside the result, which is at most 31; it gives the
// compiler the proof of the slice, and putEight one store.
func putEightAt(b *[longBody]byte, i uint, w uint64) {
	i &= 31
	putEight(b[i:i+8], w)
}

// putFourAt and putTwoAt are putEightAt for four bytes and two, at an
// index of at most 35 and 39: the exponent, which follows the digits.
func putFourAt(b *[longBody]byte, i uint, w uint32) {
	i = min(i, longBody-4)
	b[i] = byte(w)
	b[i+1] = byte(w >> 8)
	b[i+2] = byte(w >> 16)
	b[i+3] = byte(w >> 24)
}

func putTwoAt(b *[longBody]byte, i uint, w uint16) {
	i = min(i, longBody-2)
	b[i] = byte(w)
	b[i+1] = byte(w >> 8)
}
