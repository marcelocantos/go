// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// The bounds of the integer types, spelled out because the package imports
// only math/bits (see "Dependencies" in the package documentation).
const (
	maxInt64  = 1<<63 - 1
	minInt64  = -1 << 63
	maxUint64 = 1<<64 - 1
)

// The width-independent half of the integer conversions (rule 7 of "Width
// parameterisation" in the package documentation). The template
// intconv_tmpl.go rounds a decimal to an integer magnitude; the functions
// here split an integer into sign and magnitude and apply the range and
// Status rules of "Integer conversions", which are stated there and
// nowhere else.

// int64Parts returns the sign and magnitude of x. The magnitude of
// minInt64 is 2^63, which an int64 cannot hold, so it is formed by
// negating in uint64.
func int64Parts(x int64) (neg bool, m uint64) {
	if x < 0 {
		return true, -uint64(x)
	}
	return false, uint64(x)
}

// int64Result applies the range rules to the rounded integer that
// intPartsW describes. A NaN gives 0 and an infinity the bound of its sign,
// both Invalid. A finite value outside the range gives the nearest bound
// and Inexact|Overflow, whether or not it had a fraction.
func int64Result(kind Kind, neg bool, m uint64, big, inexact bool) (int64, Status) {
	switch {
	case kind == KindNaN:
		return 0, Invalid
	case kind == KindInf:
		if neg {
			return minInt64, Invalid
		}
		return maxInt64, Invalid
	case neg && (big || m > 1<<63):
		return minInt64, Inexact | Overflow
	case !neg && (big || m > maxInt64):
		return maxInt64, Inexact | Overflow
	}
	st := Exact
	if inexact {
		st = Inexact
	}
	if neg {
		return int64(-m), st
	}
	return int64(m), st
}

// uint64Result is int64Result for a uint64. A negative value that rounds
// to a non-zero integer is below the range; one that rounds to zero is in
// it.
func uint64Result(kind Kind, neg bool, m uint64, big, inexact bool) (uint64, Status) {
	switch {
	case kind == KindNaN:
		return 0, Invalid
	case kind == KindInf:
		if neg {
			return 0, Invalid
		}
		return maxUint64, Invalid
	case neg && (big || m != 0):
		return 0, Inexact | Overflow
	case big:
		return maxUint64, Inexact | Overflow
	}
	st := Exact
	if inexact {
		st = Inexact
	}
	return m, st
}
