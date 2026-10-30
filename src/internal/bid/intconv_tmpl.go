// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This file is the template for zintconv64.go and zintconv128.go: the
// integer conversions of "Integer conversions" in the package
// documentation, written once in the vocabulary of "Width
// parameterisation". The width-independent halves, the split of an integer
// into sign and magnitude and the range and Status rules of the result,
// are in intconv.go. Everything from the build constraint to the package
// clause is commentary on the template and is dropped from the generated
// files, which nobody edits.
package bid

// intToW is convertFromInt for a magnitude m of sign neg: m rounded once to
// PrecisionW digits in mode, with preferred exponent 0, by roundW. A
// decimal128 holds every 64-bit magnitude, so mode is moot at that width;
// roundW is still the only packer.
func intToW(neg bool, m uint64, mode Mode) coefW {
	x, _ := roundW(neg, wideW{Lo: cFromU64W(m)}, 0, classZero, mode, MinExpW)
	return x
}

// int64ToW is Int64ToW: x converted to a decimal of this width.
func int64ToW(x int64, mode Mode) coefW {
	neg, m := int64Parts(x)
	return intToW(neg, m, mode)
}

// uint64ToW is Uint64ToW: x converted to a decimal of this width.
func uint64ToW(x uint64, mode Mode) coefW {
	return intToW(false, x, mode)
}

// intPartsW rounds x to an integer in mode and describes the result as a
// magnitude: m is the rounded integer's magnitude and neg its sign, valid
// unless big, which says that the magnitude exceeds a uint64 (m is then
// zero). inexact says that a non-zero fraction was discarded. For a NaN or
// an infinity, kind says so and the rest is meaningless.
//
// The value is c × 10^exp. With a non-negative exponent it is an integer
// and scaled, which cScaleW does without overflow for any exponent, up to
// the type. With a negative one the digits below the units are dropped in a
// single shift, whose class is all that the rounding direction needs; the
// quotient is at most a coefficient, so adding one cannot carry out of the
// type, though at decimal128 it may exceed a uint64.
func intPartsW(x coefW, mode Mode) (kind Kind, neg bool, m uint64, big, inexact bool) {
	kind, neg, c, exp := UnpackW(x)
	if kind != KindFinite {
		return kind, neg, 0, false, false
	}
	if exp >= 0 {
		scaled, over := cScaleW(c, exp)
		m, fits := cToU64W(scaled)
		if over || !fits {
			return kind, neg, 0, true, false
		}
		return kind, neg, m, false, false
	}
	q, class := cShiftW(c, -exp)
	if class != classZero {
		inexact = true
		if roundUp(class, mode, neg, cOddW(q)) {
			q = cIncW(q)
		}
	}
	m, fits := cToU64W(q)
	if !fits {
		return kind, neg, 0, true, inexact
	}
	return kind, neg, m, false, inexact
}

// int64FromW is Int64FromW.
func int64FromW(x coefW, mode Mode) (int64, Status) {
	kind, neg, m, big, inexact := intPartsW(x, mode)
	return int64Result(kind, neg, m, big, inexact)
}

// uint64FromW is Uint64FromW.
func uint64FromW(x coefW, mode Mode) (uint64, Status) {
	kind, neg, m, big, inexact := intPartsW(x, mode)
	return uint64Result(kind, neg, m, big, inexact)
}
