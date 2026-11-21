// Package ops holds raw-word prototypes of decimal128 inline fast paths for
// three in-memory layouts, each in front of a call to a general routine:
//
//	U  UBD as the ubd128 compiler lowers it at ubdlevel 9 (ssagen/bid.go),
//	   transcribed to Go source: + first level, - second level, * narrowed
//	   to 2^56, == and < for one qExp and one sign.
//	N  UBD with the fast paths narrowed to a one-word coefficient (the high
//	   word's 49 coefficient bits clear): the control, which separates what
//	   narrowing buys from what the layout buys. Same sign policy as U.
//	C  the money form: hi == 0, lo = c<<4 | scale, c a 60-bit two's
//	   complement coefficient, scale = -qExp in [0,15]; everything else
//	   (including -0) is general form, UBD with qExp offset by 8192 so its
//	   high word is never 0. Overflow by the portable xor test (no jo).
//
// Every function takes and returns words low first, as UBD128{Lo, Hi}.
package ops

import "math/bits"

const (
	expShift = 50
	signHi   = 1 << 49
	coeffHi  = 1<<49 - 1
	nanTag   = 15
	minExp   = -6176
	maxExp   = 6111
)

// Slow counts calls to the general routines (the stubs below), so the
// workload simulation can tell which path an operation took.
var Slow int

//go:noinline
func slowAdd(xl, xh, yl, yh uint64) (uint64, uint64) { Slow++; return 0, 0 }

//go:noinline
func slowSub(xl, xh, yl, yh uint64) (uint64, uint64) { Slow++; return 0, 0 }

//go:noinline
func slowMul(xl, xh, yl, yh uint64) (uint64, uint64) { Slow++; return 0, 0 }

//go:noinline
func slowEq(xl, xh, yl, yh uint64) bool { Slow++; return false }

//go:noinline
func slowLt(xl, xh, yl, yh uint64) bool { Slow++; return false }

// ---- U: UBD, ubdlevel 9 ----

//go:noinline
func UAdd(xl, xh, yl, yh uint64) (uint64, uint64) {
	differ := xh ^ yh
	ych := yh & coeffHi
	coeffs := (xh - yh) + (ych + ych)
	if differ>>(expShift-1) != 0 || coeffs>>48 != 0 {
		return slowAdd(xl, xh, yl, yh)
	}
	lo, c := bits.Add64(xl, yl, 0)
	hi, _ := bits.Add64(xh, ych, c)
	return lo, hi
}

//go:noinline
func USub(xl, xh, yl, yh uint64) (uint64, uint64) {
	yf := yh ^ signHi
	differ := xh ^ yf
	if differ>>(expShift-1) != 1 || (xh|yf)&(1<<48) != 0 {
		return slowSub(xl, xh, yl, yh)
	}
	dlo, b := bits.Sub64(xl, yl, 0)
	dhi, _ := bits.Sub64(xh^signHi, yf, b)
	swap := int64(dhi) < 0
	nlo, b2 := bits.Sub64(0, dlo, 0)
	nhi, _ := bits.Sub64(0, dhi, b2)
	lo, hi, hsel := dlo, dhi, xh
	if swap {
		lo, hi, hsel = nlo, nhi, yf
	}
	header := hsel >> (expShift - 1) << (expShift - 1)
	if lo|hi == 0 {
		header &^= signHi
	}
	return lo, header | hi
}

//go:noinline
func UMul(xl, xh, yl, yh uint64) (uint64, uint64) {
	small := (xl | yl) >> 56
	words := (xh|yh)<<(64-expShift+1) | small
	exp := uint64(int64(xh)>>expShift + int64(yh)>>expShift)
	if words != 0 || exp+(-minExp) > maxExp-minExp {
		return slowMul(xl, xh, yl, yh)
	}
	phi, plo := bits.Mul64(xl, yl)
	sign := (xh ^ yh) & signHi
	return plo, sign | exp<<expShift | phi
}

//go:noinline
func UEq(xl, xh, yl, yh uint64) bool {
	differ := xh ^ yh
	if differ>>(expShift-1) != 0 || xh>>59 == nanTag {
		return slowEq(xl, xh, yl, yh)
	}
	return differ|(xl^yl) == 0
}

//go:noinline
func ULt(xl, xh, yl, yh uint64) bool {
	differ := xh ^ yh
	if differ>>(expShift-1) != 0 || xh>>59 == nanTag {
		return slowLt(xl, xh, yl, yh)
	}
	m := uint64(int64(xh<<(64-expShift)) >> 63)
	lo, b := bits.Sub64(xl^m, yl^m, 0)
	_, b = bits.Sub64(xh^m, yh^m, b)
	_ = lo
	return b != 0
}

// ---- N: UBD narrowed to a one-word coefficient ----

//go:noinline
func NAdd(xl, xh, yl, yh uint64) (uint64, uint64) {
	// One qExp, one sign, and no high coefficient bits: the high words are
	// equal and x's has nothing below bit 49. That excludes the specials.
	if (xh^yh)|(xh<<15) != 0 {
		return slowAdd(xl, xh, yl, yh)
	}
	lo, c := bits.Add64(xl, yl, 0)
	return lo, xh + c // below 2^65 < 10^34
}

//go:noinline
func NSub(xl, xh, yl, yh uint64) (uint64, uint64) {
	// x - y of one sign: an effective subtraction of word coefficients.
	if (xh^yh)|(xh<<15) != 0 {
		return slowSub(xl, xh, yl, yh)
	}
	d, b := bits.Sub64(xl, yl, 0)
	m := -b
	lo := (d ^ m) - m
	hi := xh ^ (m & signHi)
	if lo == 0 {
		hi &^= signHi
	}
	return lo, hi
}

// NMul is UMul: UBD's multiply path already takes word coefficients only.
//
//go:noinline
func NMul(xl, xh, yl, yh uint64) (uint64, uint64) {
	small := (xl | yl) >> 56
	words := (xh|yh)<<(64-expShift+1) | small
	exp := uint64(int64(xh)>>expShift + int64(yh)>>expShift)
	if words != 0 || exp+(-minExp) > maxExp-minExp {
		return slowMul(xl, xh, yl, yh)
	}
	phi, plo := bits.Mul64(xl, yl)
	sign := (xh ^ yh) & signHi
	return plo, sign | exp<<expShift | phi
}

//go:noinline
func NEq(xl, xh, yl, yh uint64) bool {
	// Word coefficients exclude NaN (its prefix is in the high word).
	if (xh^yh)|(xh<<15) != 0 {
		return slowEq(xl, xh, yl, yh)
	}
	return xl == yl
}

//go:noinline
func NLt(xl, xh, yl, yh uint64) bool {
	if (xh^yh)|(xh<<15) != 0 {
		return slowLt(xl, xh, yl, yh)
	}
	m := uint64(int64(xh<<(64-expShift)) >> 63)
	return xl^m < yl^m
}

// ---- C: the money form ----

//go:noinline
func CAdd(xl, xh, yl, yh uint64) (uint64, uint64) {
	yc := yl &^ 15
	s := xl + yc
	if xh|yh != 0 || (xl^yl)&15 != 0 || int64((xl^s)&(yc^s)) < 0 {
		return slowAdd(xl, xh, yl, yh)
	}
	return s, 0
}

//go:noinline
func CSub(xl, xh, yl, yh uint64) (uint64, uint64) {
	yc := yl &^ 15
	s := xl - yc
	if xh|yh != 0 || (xl^yl)&15 != 0 || int64((xl^yc)&(xl^s)) < 0 {
		return slowSub(xl, xh, yl, yh)
	}
	return s, 0
}

//go:noinline
func CMul(xl, xh, yl, yh uint64) (uint64, uint64) {
	// Scales at most 7 each, so their sum fits the nibble.
	if xh|yh == 0 && (xl|yl)&8 == 0 {
		a := int64(xl) >> 4  // cx
		b := int64(yl &^ 15) // cy<<4
		hi, lo := bits.Mul64(uint64(a), uint64(b))
		// The signed high word, from the unsigned one.
		shi := hi - uint64(a>>63)&uint64(b) - uint64(b>>63)&uint64(a)
		// No overflow, and not a zero product (which may be -0).
		if shi == uint64(int64(lo)>>63) && lo != 0 {
			return lo | (xl+yl)&15, 0
		}
	}
	return slowMul(xl, xh, yl, yh)
}

//go:noinline
func CEq(xl, xh, yl, yh uint64) bool {
	if xh|yh != 0 || (xl^yl)&15 != 0 {
		return slowEq(xl, xh, yl, yh)
	}
	return xl == yl
}

//go:noinline
func CLt(xl, xh, yl, yh uint64) bool {
	if xh|yh != 0 || (xl^yl)&15 != 0 {
		return slowLt(xl, xh, yl, yh)
	}
	return int64(xl) < int64(yl)
}
