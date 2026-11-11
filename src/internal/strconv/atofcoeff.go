// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package strconv

import "math/bits"

// This file is ParseFloat without the parsing: it converts a value already
// held as an integer coefficient and a power of ten, for internal/bid, whose
// decimal floating-point formats hold exactly that. It adds the two things
// ParseFloat has no use for: a coefficient of up to 128 bits and a choice of
// rounding direction.

// A Rounding says how Float64FromCoeff and Float32FromCoeff round a value to
// the binary format. The directions are of the magnitude, so a caller holding
// a rounding toward positive or negative infinity chooses between
// RoundTowardZero and RoundAwayFromZero by the sign of the value.
type Rounding uint8

const (
	RoundNearestEven  Rounding = iota // nearest, ties to the even significand (what ParseFloat does)
	RoundNearestAway                  // nearest, ties away from zero
	RoundTowardZero                   // the neighbor of smaller magnitude
	RoundAwayFromZero                 // the neighbor of larger magnitude
)

// Float64FromCoeff returns (-1)^neg × (hi×2^64 + lo) × 10^exp10 rounded once,
// in direction r, to float64. The result has float64's gradual underflow and
// signed zeros. A value too large for the format gives the infinity of its
// sign, or the largest finite magnitude under RoundTowardZero; the sign of a
// zero coefficient is kept. Float64FromCoeff does not allocate.
//
// Any 128-bit coefficient is accepted. Precondition: exp10 is small enough
// that adding 39 to its magnitude does not overflow an int.
func Float64FromCoeff(hi, lo uint64, exp10 int, neg bool, r Rounding) float64 {
	if optimize && r == RoundNearestEven && (hi|lo) != 0 {
		sign := bool2[uint64](neg) << 63
		if hi == 0 && lo < uint64pow10[19] {
			// Outside these bounds, as in atof64, parseFloat64 is not
			// needed: the exact path decides such a value from its size.
			if -345 <= exp10 && exp10 <= 310 {
				f, _ := parseFloat64(lo, exp10, sign)
				return f
			}
		} else if b, ok := coeffScale(hi, lo, exp10, &float64info); ok {
			return float64frombits(sign | b)
		}
	}
	return float64frombits(coeffFloatBits(hi, lo, exp10, neg, r, &float64info))
}

// Float32FromCoeff is Float64FromCoeff with a float32 result. It rounds once,
// from the exact value directly to float32, and never by way of float64.
func Float32FromCoeff(hi, lo uint64, exp10 int, neg bool, r Rounding) float32 {
	if optimize && r == RoundNearestEven && (hi|lo) != 0 {
		sign := bool2[uint32](neg) << 31
		if hi == 0 && lo < uint64pow10[19] {
			// See Float64FromCoeff; the bounds are atof32's.
			if -70 <= exp10 && exp10 <= 40 {
				f, _ := parseFloat32(lo, exp10, sign)
				return f
			}
		} else if b, ok := coeffScale(hi, lo, exp10, &float32info); ok {
			return float32frombits(sign | uint32(b))
		}
	}
	return float32frombits(uint32(coeffFloatBits(hi, lo, exp10, neg, r, &float32info)))
}

// coeffExactPow10 is the largest p for which pow10Tab holds 10^p exactly:
// 10^p scaled to 128 bits is 5^p, and 5^55 is the last power below 2^128.
const coeffExactPow10 = 55

// coeffScale returns the bits, without the sign, of c × 10^p rounded to
// nearest, ties to even, in format flt, for a coefficient c = hi×2^64 + lo
// that is not zero, if it can decide them from one product of c and the
// 128-bit power of ten, and ok = false if it cannot.
//
// parseFloat64 takes a coefficient of 19 digits. A longer one was cut to a
// 19-digit prefix, and decided only when the prefix and its successor
// rounded alike; with some 60 bits in the prefix against the format's 53,
// about one random coefficient in a few hundred was left to the exact
// path, which costs hundreds of nanoseconds, and tens of microseconds for
// a large exponent. Here the whole coefficient is scaled.
//
// With n the bit length of c, x = c << (128-n) has its top bit set, and
// pm, the table's 10^p × 2^(127-lp) rounded up, has too, lp being
// ⌊log₂ 10^p⌋. The product Q = x × pm has 256 bits, and
//
//	c × 10^p = T × 2^(n+lp-255),  T = x × 10^p × 2^(127-lp),  Q - 2^128 < T ≤ Q,
//
// because pm exceeds its power by less than one and x is below 2^128; T is
// Q exactly when the table's entry is exact, 0 ≤ p ≤ coeffExactPow10.
// The significand is the leading bits of T above a place k, and its
// rounding is decided by R, the k bits of T below, against the half
// H = 2^(k-1). k is at least 128 + 2, so of Q's low bits Rq:
//
//   - Rq < H gives Q's significand: either R ≤ Rq < H, or T fell below
//     the significand's multiple, by less than 2^128, and rounds up to it.
//   - Rq ≥ H + 2^128 gives the next: R > H.
//   - Between the two R may lie on either side of H, or on it. An exact
//     product decides that as well, the tie included; any other is left
//     to the exact path. That is one value in 2^(k-129), and k is 202 at
//     least for float64: never, for a value not built to be a tie. A
//     value that is a tie exactly, as the midpoint of two floats written
//     out in decimal is, always lands there, and is decided exactly.
//
// A subnormal result keeps fewer bits, by a larger k, up to none of the
// 256, when the value is below half of the smallest subnormal and rounds
// to zero.
//
// The table ends at 10^pow10Min, and a long coefficient reaches the
// format's range from some places below that. Such a c is cut by those
// places first, to c' with c' ≤ c / 10^j < c' + 1, and c' scaled by
// 10^pow10Min. If the cut dropped anything T may then exceed Q as well,
// by less than 2^(256-n) for the n bits of c', which a value in range
// leaves well below the half: Rq must stay under H by that much to give
// Q's significand, and the next is given as before.
func coeffScale(hi, lo uint64, p int, flt *floatInfo) (b uint64, ok bool) {
	cut := false
	if p < pow10Min {
		j := pow10Min - p
		if j >= len(uint64pow10) {
			return 0, false
		}
		d := uint64pow10[j]
		var rem uint64
		hi, rem = hi/d, hi%d
		lo, rem = bits.Div64(rem, lo, d)
		if hi == 0 {
			// Under 2^64 the cut leaves too little to decide by; a
			// value this small is the exact path's by its size.
			return 0, false
		}
		p, cut = pow10Min, rem != 0
	} else if p > pow10Max {
		return 0, false
	}
	// x = c << (128-n).
	n := 64 + bits.Len64(hi)
	xh, xl := hi, lo
	if hi == 0 {
		n = bits.Len64(lo)
		xh, xl = lo, 0
	}
	sh := uint(-n) & 63
	xh, xl = xh<<sh|xl>>1>>(63-sh), xl<<sh

	// Q = x × pm, with pm = pmh×2^64 + pml as the table's hi<<64 - lo.
	t := &pow10Tab[p-pow10Min]
	pml := -t.lo
	pmh := t.hi - bool2[uint64](t.lo != 0)
	h00, q0 := bits.Mul64(xl, pml)
	h01, l01 := bits.Mul64(xl, pmh)
	h10, l10 := bits.Mul64(xh, pml)
	q3, l11 := bits.Mul64(xh, pmh)
	q1, c1 := bits.Add64(h00, l01, 0)
	q2, c1 := bits.Add64(h01, l11, c1)
	q3 += c1
	q1, c1 = bits.Add64(q1, l10, 0)
	q2, c1 = bits.Add64(q2, h10, c1)
	q3 += c1

	// The significand's place: mantbits+1 bits from the top bit of Q,
	// which is bit 255 or 254, or higher for a subnormal.
	mb := int(flt.mantbits) + 1
	k := 255 + int(q3>>63) - mb
	e := k + n + log2Pow10(p) - 255 // the value is the significand × 2^e
	if minE := flt.bias + 1 - int(flt.mantbits); e < minE {
		k, e = k+minE-e, minE
		if k > 256 {
			return 0, true // T is below 2^256, at most the half
		}
	}
	// k is at least 192 + 1, so the place is in q3, or just above it.
	s := uint(k - 192)
	m := q3 >> s
	r3 := q3 & (1<<s - 1)
	h3 := uint64(1) << (s - 1)
	switch {
	case r3 < h3:
		// 2^(256-n) is 2^(128-n) in q2: n is above 64 after a cut.
		if cut && r3 == h3-1 && q2 >= -(uint64(1)<<(uint(128-n)&63)) {
			return 0, false
		}
	case r3 > h3 || q2 != 0:
		m++
	case cut || uint(p) > coeffExactPow10:
		return 0, false
	case q1|q0 != 0:
		m++
	default:
		m += m & 1 // a tie
	}
	if m>>mb != 0 {
		// The rounding carried out of the significand.
		m >>= 1
		e++
	}
	if m>>flt.mantbits == 0 {
		return m, true // subnormal, or zero
	}
	be := uint64(e + int(flt.mantbits) - flt.bias)
	if be >= 1<<flt.expbits-1 {
		return (1<<flt.expbits - 1) << flt.mantbits, true // infinity
	}
	return be<<flt.mantbits | m&(1<<flt.mantbits-1), true
}

// coeffFloatBits is the exact path of Float64FromCoeff and Float32FromCoeff,
// returning the bits of the result in format flt. It decides the values
// plainly outside the format's range from the size of the coefficient alone.
// The rest it converts through the decimal type, as ParseFloat's slow path
// does.
//
// The decimal type is exact here. It holds 800 digits, and Shift is its only
// operation that adds any. Each stage of floatBits holds c × 10^q × 2^m, with
// m the net binary shift so far. Where m ≥ 0 that has at most the digits of
// c × 2^m, and m stays near 1133 at most for a value not decided first (the
// 1075 of the smallest subnormal and the 53 of the significand, and a few
// bits of overshoot): about 39 + 341 digits. Where m < 0 it has at most the
// digits of c × 5^-m, and m stays above about -1030: 39 + 720 digits. So
// nothing is truncated, and the fraction left once the significand is
// extracted is the exact remainder, which is what the directed roundings
// need. The decimal lives on the stack: it does not escape, and 800 bytes
// cost little beside the shifts.
func coeffFloatBits(hi, lo uint64, exp10 int, neg bool, r Rounding, flt *floatInfo) uint64 {
	var sign uint64
	if neg {
		sign = 1 << flt.mantbits << flt.expbits
	}
	if hi|lo == 0 {
		return sign
	}

	// With n the bit length of c, 10^log10Pow2(n-1) ≤ c < 10^(log10Pow2(n)+1).
	n := bits.Len64(lo)
	if hi != 0 {
		n = 64 + bits.Len64(hi)
	}

	// The value is at least the power of ten above 2^(2^(expbits-1)), which
	// is the first magnitude beyond every finite value of the format.
	if log10Pow2(n-1)+exp10 > log10Pow2(1<<(flt.expbits-1)) {
		if r == RoundTowardZero {
			return sign | (1<<flt.expbits-2)<<flt.mantbits | (1<<flt.mantbits - 1)
		}
		return sign | (1<<flt.expbits-1)<<flt.mantbits
	}
	// The value is below the power of ten under 2^(bias-mantbits), which is
	// half the smallest subnormal.
	if log10Pow2(n)+1+exp10 <= log10Pow2(flt.bias-int(flt.mantbits)) {
		if r == RoundAwayFromZero {
			return sign | 1
		}
		return sign
	}

	var d decimal
	d.assignCoeff(hi, lo, exp10, neg)
	b, _ := d.floatBits(flt, r)
	return b
}

// assignCoeff sets d to (-1)^neg × (hi×2^64 + lo) × 10^exp10.
func (d *decimal) assignCoeff(hi, lo uint64, exp10 int, neg bool) {
	var buf [39]byte // 2^128 has 39 decimal digits
	i := len(buf)
	for hi != 0 || lo != 0 {
		rem := hi % 10
		hi /= 10
		lo, rem = bits.Div64(rem, lo, 10)
		i--
		buf[i] = byte(rem) + '0'
	}
	d.nd = copy(d.d[:], buf[i:])
	d.dp = d.nd + exp10
	d.neg = neg
	trim(d)
}
