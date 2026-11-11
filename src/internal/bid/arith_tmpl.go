// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zarith64.go and zarith128.go, written once
// in the vocabulary of "Width parameterisation" in the package
// documentation: every identifier that depends on the width ends in a
// capital W, and the generator (gen_arith.go) replaces that W with 64 or
// 128. Nothing else is substituted. This file is not compiled; the build
// ignores it, and only the generated files are compiled. Everything from
// the build constraint to the package clause is commentary on the template
// and is dropped from the generated files, which nobody edits.
//
// Nothing below the package clause, in code or in a comment, may end an
// identifier with that capital letter unless it is meant to be replaced.
package bid

// magCmpW compares the magnitudes of two finite values, returning -1, 0 or
// +1: cCmpFiniteW with both signs positive, which see.
//
// Precondition: xc and yc are at most MaxCoeffW.
func magCmpW(xc coefW, xe int, yc coefW, ye int) int {
	return cCmpFiniteW(false, xc, xe, false, yc, ye)
}

// nanW returns the NaN result of an operation whose operands x, y and z,
// in that order, include a NaN: the first signaling NaN among them if there
// is one, and otherwise the first NaN, made quiet with its sign and payload
// kept. An operation of one or two operands repeats its last operand. See
// "NaN results" in the package documentation.
func nanW(x, y, z coefW) coefW {
	switch {
	case IsSignalingW(x), !IsSignalingW(y) && !IsSignalingW(z) && IsNaNW(x):
		return cQuietW(x)
	case IsSignalingW(y), !IsSignalingW(z) && IsNaNW(y):
		return cQuietW(y)
	}
	return cQuietW(z)
}

// quietCmpSlowW returns the ordering of x and y as -1, 0 or +1 with ok =
// true, or 0 and ok = false when either is a NaN. It is the general path of
// equalW, lessW and lessEqualW, which compare word operands of one sign and
// one exponent inline by their keys from sameQuantumKeysW (see "Inline fast
// paths" in the package documentation), and the whole of CompareW, minW and
// maxW, where such a front would not pay: Compare64 and Compare128 miss the
// inlining budget with it, and out of line it costs every operand it refuses
// a second call.
//
// The compiler calls the decimal128 instantiation of this function
// directly, so its parameters and results are a contract with it: see the
// end of "Inline fast paths" in the package documentation.
func quietCmpSlowW(x, y coefW) (c int, ok bool) {
	if xn, xc, xe, ok := unpackWordW(x); ok {
		if yn, yc, ye, ok := unpackWordW(y); ok {
			return compareWords(xn, xc, xe, yn, yc, ye), true
		}
	}
	// Two finite values in the first form, the pair that is not word
	// operands and not special: one call that makes none decides them, and
	// nothing is kept across it or across the call for the rest.
	xneg, xc, xe, xok := unpackFirstW(x)
	yneg, yc, ye, yok := unpackFirstW(y)
	if xok && yok {
		return cCmpFiniteW(xneg, xc, xe, yneg, yc, ye), true
	}
	return quietCmpRestW(x, y)
}

// quietCmpRestW is quietCmpSlowW for a pair with an operand that is not a
// canonical finite value in the first form: a NaN, an infinity, the second
// form or a non-canonical encoding.
func quietCmpRestW(x, y coefW) (c int, ok bool) {
	xk, xneg, xc, xe := unpackRestW(x)
	yk, yneg, yc, ye := unpackRestW(y)
	switch {
	case xk == KindNaN || yk == KindNaN:
		return 0, false
	case xk == KindFinite && yk == KindFinite:
		return cCmpFiniteW(xneg, xc, xe, yneg, yc, ye), true
	case xk == KindInf && yk == KindInf:
		// Two infinities: equal if of one sign, else the negative is the lesser.
		if xneg == yneg {
			return 0, true
		}
		if xneg {
			return -1, true
		}
		return 1, true
	case xk == KindInf:
		// An infinity against a finite value, finite zero included.
		if xneg {
			return -1, true
		}
		return 1, true
	}
	if yneg {
		return 1, true
	}
	return -1, true
}

// equalW reports whether x == y by value; it is false if either is a NaN.
func equalW(x, y coefW) bool {
	if kx, ky, ok := sameQuantumKeysW(x, y); ok {
		return kx == ky
	}
	c, ok := quietCmpSlowW(x, y)
	return ok && c == 0
}

// lessW reports whether x < y; it is false if either is a NaN.
func lessW(x, y coefW) bool {
	if kx, ky, ok := sameQuantumKeysW(x, y); ok {
		return kx < ky
	}
	c, ok := quietCmpSlowW(x, y)
	return ok && c < 0
}

// lessEqualW reports whether x <= y; it is false if either is a NaN.
func lessEqualW(x, y coefW) bool {
	if kx, ky, ok := sameQuantumKeysW(x, y); ok {
		return kx <= ky
	}
	c, ok := quietCmpSlowW(x, y)
	return ok && c <= 0
}

// totalOrderW reports whether x precedes or equals y in the total order of
// IEEE 754-2019 clause 5.10 as the package defines it: by sign first
// (negative operands, NaNs included, precede positive ones), then, within
// one sign, finite values by value, then infinities, then NaNs; two finite
// values of equal value, zeros included, by exponent. For negative operands
// everything within the sign reverses. All NaNs of one sign are equal.
func totalOrderW(x, y coefW) bool {
	xneg, xc, xe, xok := unpackFirstW(x)
	xk := KindFinite
	if !xok {
		xk, xneg, xc, xe = unpackRestW(x)
	}
	yneg, yc, ye, yok := unpackFirstW(y)
	yk := KindFinite
	if !yok {
		yk, yneg, yc, ye = unpackRestW(y)
	}
	if xneg != yneg {
		return xneg
	}
	// Order the magnitudes: c is -1, 0 or +1 as |x| precedes, equals or
	// follows |y|, which for positive operands is the order of the values.
	c := 0
	switch {
	case xk != yk:
		// Kind orders finite, infinity, NaN.
		c = -1
		if xk > yk {
			c = 1
		}
	case xk != KindFinite:
		// Infinities of one sign are equal, as are NaNs.
	default:
		xz, yz := cIsZeroW(xc), cIsZeroW(yc)
		switch {
		case xz && yz:
		case xz:
			c = -1
		case yz:
			c = 1
		default:
			c = magCmpW(xc, xe, yc, ye)
		}
		if c == 0 {
			// One value, two quanta: the smaller exponent first.
			switch {
			case xe < ye:
				c = -1
			case xe > ye:
				c = 1
			}
		}
	}
	if xneg {
		return c >= 0
	}
	return c <= 0
}

// keyW returns the fields of the hash key of x. See Key.
func keyW(x coefW) (kind Kind, neg bool, c coefW, exp int) {
	neg, c, exp, ok := unpackFirstW(x)
	kind = KindFinite
	if !ok {
		kind, neg, c, exp = unpackRestW(x)
	}
	if kind == KindNaN {
		return KindNaN, false, c, 0
	}
	if kind == KindInf {
		return KindInf, neg, c, 0
	}
	if cIsZeroW(c) {
		return KindFinite, false, c, 0
	}
	// Strip the trailing zeros of the coefficient by binary search on their
	// number: at most PrecisionW-1 of them, so steps of the powers of two
	// from the largest one not above PrecisionW down to 1 can remove any
	// count. Most coefficients have none, which one division settles.
	q, ok := cDivExactPow10W(c, 1)
	if !ok {
		return kind, neg, c, exp
	}
	c, exp = q, exp+1
	step := 1
	for step*2 <= PrecisionW {
		step *= 2
	}
	for ; step > 0; step /= 2 {
		if q, ok := cDivExactPow10W(c, step); ok {
			c, exp = q, exp+step
		}
	}
	return kind, neg, c, exp
}

// roundFiniteW is the bounded-coefficient part of the rounding routine.
// c is canonical (at most PrecisionW digits), floor is within the format,
// and exp <= MaxExpW. It preserves exp when exp >= floor; otherwise it
// rounds once at floor. Discarding at least one digit leaves room for an
// increment, so the result cannot carry beyond PrecisionW digits. Zero
// keeps its sign and takes max(exp, floor). No overflow or fold-down is
// possible under these preconditions.
//
// Quantize and RoundToIntegral already establish these bounds. roundW
// shares this helper after checking them, and derives conversion Status
// from the returned discarded class plus the original magnitude.
func roundFiniteW(neg bool, c coefW, exp int, mode Mode, floor int) (coefW, digitClass) {
	class := classZero
	if exp < floor {
		// Clamp before subtracting so an arbitrary negative int exponent
		// from RoundW cannot overflow. Such a far shift discards all digits.
		shift := floor - max(exp, floor-maxWideDigits)
		c, class = cShiftW(c, shift)
		// The increment is added, not branched on (roundInc), and an
		// exact shift, for which roundInc is not defined, adds zero.
		inc := roundInc(class, mode, neg, cOddW(c))
		if class == classZero {
			inc = 0
		}
		c = cAddU64W(c, inc)
		exp = floor
	}
	return packW(neg, c, exp), class
}

// roundW is the general rounding routine of the package. Together with
// its bounded-coefficient helper roundFiniteW, it is the only way an
// operation produces a rounded result under each of the five modes, but
// for the face-value rounding of addW's swamped addend, roundFaceW in
// add_tmpl.go (see "Face-value rounding" in the package documentation),
// which takes the one step of this routine that such an input reaches.
// Every rule of "Exponent of a result", "Overflow", "Underflow and
// subnormal results" and "Sign of a result" (for zeros) in the package
// documentation is implemented by these routines and nowhere else. The
// width-independent direction decision is roundInc in round.go.
//
// The input is the exact value
//
//	v = (-1)^neg × (p + f) × 10^exp
//
// with p a wide integer (the full product of two coefficients fits it) and
// f a fraction, 0 <= f < 1, described only by class: classZero for f = 0,
// otherwise where f lies relative to one half. floor is the least exponent
// the result may take: MinExpW for every operation with a preferred
// exponent (exp is then that preferred exponent), and the fixed exponent
// itself, equal to exp, for Quantize and RoundToIntegral, whose result
// exponent is fixed by the operation rather than by the precision; such a
// caller treats a result whose exponent is not exp as not fitting
// (Quantize returns NaN, RoundToIntegral keeps the carried value).
//
// What a caller must supply for the result to be correctly rounded:
//
//   - p and class together describe v exactly. A caller holding more
//     digits than p can carry (FMA, a quotient generated beyond the
//     precision, a sum whose operands are far apart) reduces them with the
//     digit primitives, cShiftFullW or cReduceClassW, and passes the class
//     they return as class; two reductions compose as digits.go says.
//   - If class is not classZero and stands for a fraction of unknown size,
//     p must have more than PrecisionW digits. roundW then discards at
//     least one digit of p itself, and withSticky combines the two
//     discarded parts exactly. A caller meets this by keeping PrecisionW+1
//     digits or more, or by scaling its exact part up to that many digits
//     before truncating the rest, or by passing the whole exact product, as
//     mulW does.
//   - If class is not classZero and p has PrecisionW digits or fewer, no
//     digit of p is discarded and class alone decides the rounding, taken
//     at face value: classHalf is a tie and classAboveHalf rounds up in the
//     nearest modes. roundPartsW passes such a class, classBelowHalf,
//     which is how RoundW's sticky bit means "p + ε for an arbitrarily
//     small ε > 0" ("Rounding from exact parts"). Among the operations
//     only addW does, for an operand so far below the other that it is a
//     fraction f of one unit with 0 < f < 1/10: it rounds p with exactly
//     PrecisionW digits, so that nothing is padded, and the class that f
//     or 1 - f truly has, classBelowHalf or classAboveHalf, through
//     roundFaceW, which gives this routine's result for that input. Every
//     other operation keeps a round digit. A p of zero with a non-zero
//     class is a value below 10^exp, and a caller meaning "arbitrarily
//     small" passes exp <= MinExpW with it, as roundPartsW does, so that it
//     rounds at MinExpW.
//
// The result exponent x is exp for an exact value, raised when p has more
// than PrecisionW digits, when exp is below floor, or when exp is above
// MaxExpW and the coefficient can be padded with zeros to make up the
// difference (fold-down); if it cannot, the value overflows. An inexact
// value is encoded at the least exponent at or above floor at which its
// rounded coefficient has at most PrecisionW digits, so p is padded or
// shifted to that place before the increment, and a coefficient that
// carries out to 10^PrecisionW becomes 10^(PrecisionW-1) one place up. An
// exact zero takes exp clamped into [floor, MaxExpW] and keeps the sign.
//
// Status: Exact when the result is v, including an exact subnormal, an
// exact fold-down and a zero; Inexact otherwise; Inexact|Underflow when,
// in addition, v is non-zero and below 10^EminW (tininess before
// rounding); Inexact|Overflow when the rounded value is above the largest
// finite one, in which case the result is the infinity or the largest
// finite value of sign neg that overflowsToInf selects. The arithmetic
// wrappers discard the Status; it is meaningful to RoundW, to the parser
// and to the integer conversions, which report it.
//
// Exact fast paths. PackW is the raw encoder; it rounds nothing and
// refuses what does not fit. Only the rounding routines may call it,
// unless it re-encodes a coefficient and exponent already known to be
// representable because UnpackW returned them, possibly with another
// sign or for a narrower width, decoded them from an encoding that cannot
// hold anything else, or accepts an integer type whose whole range fits,
// and is named both here and in the allowlist of TestRoundIsTheOnlyPacker,
// which parses the package source. Widen re-encodes the decimal64
// coefficient and exponent
// that Unpack64 returned as a decimal128 with Pack128, because every
// decimal64 value is a decimal128 value. It measured two to three times
// faster than the rounding routine on a 256-bit wide value. BIDFromDPD64
// and BIDFromDPD128 encode the coefficient and exponent of a DPD encoding,
// which has at most Precision digits (one in the combination field and
// three to each declet) and a biased exponent below 3 × 2^(ExpBits-2) (the
// combination field's two leading exponent bits are never 11), so that
// every DPD finite value is representable exactly; they are conversions
// between encodings, not operations, and round nothing. Int64To128 and
// Uint64To128 encode an exponent-zero magnitude below 2^64, which has at
// most 20 digits and therefore always fits Precision128's 34 digits.
// packWord64 and packWord128, in encoding.go, encode the exact results of
// the word fast paths of addW, mulW, fmaW, quantizeW and remainderW,
// operands whose coefficients fit a uint64: an exact sum, product, fused
// sum, rescaled coefficient or remainder at its preferred exponent, which
// roundW would encode
// unchanged, refusing it when it does not fit the format so that the caller
// takes its general path. Each of those functions says why the two paths
// agree.
func roundW(neg bool, p wideW, exp int, class digitClass, mode Mode, floor int) (coefW, Status) {
	// An exact coefficient already inside the format keeps its exponent.
	// Check its bound directly instead of counting wide digits and deriving
	// the same exponent through the general rounding path. floor still
	// applies to quantize and integral rounding, including exact results.
	if class == classZero {
		var zero coefW
		if p.Hi == zero {
			if cIsZeroW(p.Lo) {
				return packW(neg, zero, min(max(exp, floor), MaxExpW)), Exact
			}
			if exp <= MaxExpW && floor <= MaxExpW && !cGtMaxW(p.Lo) {
				if exp >= floor {
					return packW(neg, p.Lo, exp), Exact
				}
				// A bounded coefficient shifted right to floor cannot exceed
				// Precision digits, even after an increment. Quantize and
				// integral rounding therefore need no wide digit count.
				r, dropped := roundFiniteW(neg, p.Lo, exp, mode, floor)
				st := Exact
				if dropped != classZero {
					st = Inexact
					// Far-below-format exponents are tiny without adding to
					// them, which also avoids overflow for an int minimum.
					if exp < EminW && (exp < EminW-PrecisionW || cDigitsW(p.Lo)-1+exp < EminW) {
						st |= Underflow
					}
				}
				return r, st
			}
		}
	}
	// Far outside the format every exponent rounds alike: overflow in every
	// mode above, a value below a tenth of the smallest subnormal below.
	// Clamping keeps the exponent arithmetic below in range and the shift
	// counts within what the digit primitives are specified for.
	exp = min(max(exp, MinExpW-maxWideDigits-2), MaxExpW+PrecisionW+2)

	// a is the adjusted exponent of v, the power of ten of its leading
	// digit, and tiny whether v is below 10^EminW. For p = 0, v is below
	// 10^exp, which is what a = exp - 1 says. A p whose high half is zero,
	// which every sum is, has the digits of its low half, counted in the
	// coefficient type by a helper that inlines, and is reduced below by
	// cShiftW, which cReduceClassW would call for it after a test of its
	// own.
	var d int
	narrow := cIsZeroW(p.Hi)
	if narrow {
		d = cDigitsW(p.Lo)
	} else {
		d = cLenFullW(p)
	}
	a := d - 1 + exp
	tiny := a < EminW

	// x is the exponent of an inexact result: the least, at or above floor,
	// at which PrecisionW digits hold the value. s is the number of places
	// p moves to reach it: left (padding) when positive, right (dropping
	// digits, which can make an exact value inexact) when negative.
	x := max(a-(PrecisionW-1), floor)
	s := exp - x
	var q coefW
	if s < 0 {
		var dropped digitClass
		if narrow {
			q, dropped = cShiftW(p.Lo, -s)
		} else {
			q, dropped = cReduceClassW(p, -s)
		}
		class = dropped.withSticky(class != classZero)
	} else {
		q = cLoW(p)
	}

	if class == classZero {
		// Exact: the member of the cohort whose exponent is closest to exp.
		// Dropping digits raised it to x; otherwise it is exp, folded down
		// to MaxExpW if the coefficient has room for the zeros.
		if s < 0 {
			exp = x
		}
		if exp > MaxExpW {
			padded, overflow := cScaleW(q, exp-MaxExpW)
			if overflow || cGtMaxW(padded) {
				return overflowW(neg, mode)
			}
			q, exp = padded, MaxExpW
		}
		return packW(neg, q, exp), Exact
	}

	// Inexact: pad to the result's place, then increment or not.
	if s > 0 {
		q = cMulPow10W(q, s)
	}
	q = cAddU64W(q, roundInc(class, mode, neg, cOddW(q)))
	if cGtMaxW(q) {
		q, x = cPow10W(PrecisionW-1), x+1
	}
	if x > MaxExpW {
		return overflowW(neg, mode)
	}
	st := Inexact
	if tiny {
		st |= Underflow
	}
	return packW(neg, q, x), st
}

// overflowW returns the default result of an overflow of sign neg in mode
// and its Status (IEEE 754-2019 clause 7.4; "Overflow" in the package
// documentation).
func overflowW(neg bool, mode Mode) (coefW, Status) {
	if overflowsToInf(mode, neg) {
		return cInfW(neg), Inexact | Overflow
	}
	return packW(neg, cMaxW(), MaxExpW), Inexact | Overflow
}

// packW is PackW for a coefficient and exponent that a rounding routine
// has brought within the format. PackW refusing them is an internal bug,
// not an input condition, and panics rather than yielding a NaN.
func packW(neg bool, c coefW, exp int) coefW {
	x, ok := PackW(neg, c, exp)
	if !ok {
		panic(errRoundRange)
	}
	return x
}

// roundPartsW is RoundW: the sticky bit stands for an arbitrarily small
// fraction, which is classBelowHalf when p has PrecisionW digits or fewer
// and acts as a sticky bit when roundW drops digits of p (see roundW). A
// zero p with the bit set is a value below every subnormal, so it is
// placed at MinExpW, where it rounds to the zero or the smallest
// subnormal of its sign.
func roundPartsW(neg bool, p wideW, exp int, sticky bool, mode Mode) (coefW, Status) {
	class := classZero
	if sticky {
		class = classBelowHalf
		if p == (wideW{}) {
			exp = min(exp, MinExpW)
		}
	}
	return roundW(neg, p, exp, class, mode, MinExpW)
}
