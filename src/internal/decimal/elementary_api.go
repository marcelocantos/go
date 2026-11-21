// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package decimal

import "internal/bid"

// The generic functions of elementary.go's operations, in the shape of
// arithmetic.go: a test of the width and a call to a function of
// concrete type. The documentation is math/decimal's.

// unary64 returns op of x, or x made quiet if it is a NaN.
func unary64(x decimal64, op func(*format, dec) result) decimal64 {
	b := BID64bits(x)
	if bid.IsNaN64(b) {
		return BID64frombits(bid.Add64(b, b, bid.ToNearestEven))
	}
	return BID64frombits(op(&format64, unpack64(b)).pack64())
}

func unary128(x decimal128, op func(*format, dec) result) decimal128 {
	if isNaN128(x) {
		return nanResult128(x, x)
	}
	return op(&format128, unpackDec128(x)).decimal128()
}

// count64 returns op of x and n. A NaN x is returned quiet, unless n is
// zero and the operation is one that then returns 1 whatever x is; a
// signaling NaN is an invalid operand even then.
func count64(x decimal64, n int, one bool, op func(*format, dec, int64) result) decimal64 {
	b := BID64bits(x)
	if bid.IsNaN64(b) && !(one && n == 0 && !bid.IsSignaling64(b)) {
		return BID64frombits(bid.Add64(b, b, bid.ToNearestEven))
	}
	return BID64frombits(op(&format64, unpack64(b), int64(n)).pack64())
}

func count128(x decimal128, n int, one bool, op func(*format, dec, int64) result) decimal128 {
	if isNaN128(x) && !(one && n == 0 && !isSignaling128(x)) {
		return nanResult128(x, x)
	}
	return op(&format128, unpackDec128(x), int64(n)).decimal128()
}

func Sqrt[D Decimal](x D) D {
	if is64(x) {
		return D(sqrt64(decimal64(x)))
	}
	return D(sqrt128(decimal128(x)))
}

func sqrt64(x decimal64) decimal64    { return unary64(x, (*format).sqrtDec) }
func sqrt128(x decimal128) decimal128 { return unary128(x, (*format).sqrtDec) }

func Exp[D Decimal](x D) D {
	if is64(x) {
		return D(exp64(decimal64(x)))
	}
	return D(exp128(decimal128(x)))
}

func exp64(x decimal64) decimal64    { return unary64(x, (*format).expDec) }
func exp128(x decimal128) decimal128 { return unary128(x, (*format).expDec) }

func Log[D Decimal](x D) D {
	if is64(x) {
		return D(log64(decimal64(x)))
	}
	return D(log128(decimal128(x)))
}

func logE(f *format, x dec) result   { return f.logDec(x, false) }
func logTen(f *format, x dec) result { return f.logDec(x, true) }

func log64(x decimal64) decimal64    { return unary64(x, logE) }
func log128(x decimal128) decimal128 { return unary128(x, logE) }

func Log10[D Decimal](x D) D {
	if is64(x) {
		return D(logTen64(decimal64(x)))
	}
	return D(logTen128(decimal128(x)))
}

func logTen64(x decimal64) decimal64    { return unary64(x, logTen) }
func logTen128(x decimal128) decimal128 { return unary128(x, logTen) }

func Logb[D Decimal](x D) D {
	if is64(x) {
		return D(logB64(decimal64(x)))
	}
	return D(logB128(decimal128(x)))
}

func logB64(x decimal64) decimal64    { return unary64(x, (*format).logBDec) }
func logB128(x decimal128) decimal128 { return unary128(x, (*format).logBDec) }

// The results of Ilogb for the special cases, which are math.Ilogb's.
const (
	ilogbInfNaN = 1<<31 - 1 // math.MaxInt32
	ilogbZero   = -1 << 31  // math.MinInt32
)

func Ilogb[D Decimal](x D) int {
	switch {
	case IsNaN(x) || x-x != 0:
		return ilogbInfNaN
	case x == 0:
		return ilogbZero
	}
	return int(Logb(x))
}

func Nextafter[D Decimal](x, y D) D {
	switch {
	case IsNaN(x) || IsNaN(y):
		return x + y // the NaN that arithmetic on the two propagates
	case x == y:
		// x, canonical: the same value, sign and exponent in the encoding
		// every operation but the sign operations gives.
		return Quantize(x, x)
	case y > x:
		return nextUp(x)
	}
	return nextDown(x)
}

func nextUp[D Decimal](x D) D {
	if is64(x) {
		return D(nextUp64(decimal64(x)))
	}
	return D(nextUp128(decimal128(x)))
}

func nextUp64(x decimal64) decimal64    { return unary64(x, (*format).nextUpDec) }
func nextUp128(x decimal128) decimal128 { return unary128(x, (*format).nextUpDec) }

func nextDown[D Decimal](x D) D {
	if is64(x) {
		return D(nextDown64(decimal64(x)))
	}
	return D(nextDown128(decimal128(x)))
}

func nextDown64(x decimal64) decimal64    { return unary64(x, (*format).nextDownDec) }
func nextDown128(x decimal128) decimal128 { return unary128(x, (*format).nextDownDec) }

func Ldexp[D Decimal](frac D, exp int) D {
	if is64(frac) {
		return D(scaleB64(decimal64(frac), exp))
	}
	return D(scaleB128(decimal128(frac), exp))
}

func scaleB64(x decimal64, n int) decimal64 { return count64(x, n, false, (*format).scaleBDec) }
func scaleB128(x decimal128, n int) decimal128 {
	return count128(x, n, false, (*format).scaleBDec)
}

func Pown[D Decimal](x D, n int) D {
	if is64(x) {
		return D(pown64(decimal64(x), n))
	}
	return D(pown128(decimal128(x), n))
}

func pown64(x decimal64, n int) decimal64    { return count64(x, n, true, (*format).pownDec) }
func pown128(x decimal128, n int) decimal128 { return count128(x, n, true, (*format).pownDec) }

func Compound[D Decimal](x D, n int) D {
	if is64(x) {
		return D(compound64(decimal64(x), n))
	}
	return D(compound128(decimal128(x), n))
}

func compound64(x decimal64, n int) decimal64 {
	return count64(x, n, true, (*format).compoundDec)
}
func compound128(x decimal128, n int) decimal128 {
	return count128(x, n, true, (*format).compoundDec)
}

func Rootn[D Decimal](x D, n int) D {
	if is64(x) {
		return D(rootn64(decimal64(x), n))
	}
	return D(rootn128(decimal128(x), n))
}

func rootn64(x decimal64, n int) decimal64    { return count64(x, n, false, (*format).rootnDec) }
func rootn128(x decimal128, n int) decimal128 { return count128(x, n, false, (*format).rootnDec) }

func Pow[D Decimal](x, y D) D {
	if is64(x) {
		return D(pow64(decimal64(x), decimal64(y)))
	}
	return D(pow128(decimal128(x), decimal128(y)))
}

func pow64(x, y decimal64) decimal64 {
	bx, by := BID64bits(x), BID64bits(y)
	if bid.IsSignaling64(bx) || bid.IsSignaling64(by) {
		// A signaling NaN is invalid in every operation, even where a
		// quiet one is passed over: Pow(sNaN, 0) is a NaN.
		return BID64frombits(bid.Add64(bx, by, bid.ToNearestEven))
	}
	r := format64.powDec(unpack64(bx), unpack64(by))
	if r.kind == bid.KindNaN && (bid.IsNaN64(bx) || bid.IsNaN64(by)) {
		return BID64frombits(bid.Add64(bx, by, bid.ToNearestEven))
	}
	return BID64frombits(r.pack64())
}

func pow128(x, y decimal128) decimal128 {
	if isSignaling128(x) || isSignaling128(y) {
		return nanResult128(x, y)
	}
	r := format128.powDec(unpackDec128(x), unpackDec128(y))
	if r.kind == bid.KindNaN && (isNaN128(x) || isNaN128(y)) {
		return nanResult128(x, y)
	}
	return r.decimal128()
}

// The decimal128 boundary of the functions above, in decimal128's stored
// encoding: UBD where it is stored as UBD (bid.NativeUBD128), with no
// conversion, and otherwise BID. bid.NativeUBD128 is a constant, so each
// is one of its two branches.

// ubdWords returns the stored words of x where decimal128 is UBD.
func ubdWords(x decimal128) bid.UBD128 {
	hi, lo := Bits128(x)
	return bid.UBD128{Lo: lo, Hi: hi}
}

func isNaN128(x decimal128) bool {
	if bid.NativeUBD128 {
		return ubdWords(x).Hi>>59 == bid.UBDNaNTag
	}
	return bid.IsNaN128(b128(x))
}

func isSignaling128(x decimal128) bool {
	if bid.NativeUBD128 {
		return ubdWords(x).Hi>>58 == bid.UBDSNaNTag
	}
	return bid.IsSignaling128(b128(x))
}

// nanResult128 is the NaN result of an operation on x and y, at least one
// a NaN: the first signaling NaN, else the first NaN, made quiet, as the
// arithmetic operations propagate them. For one operand pass it twice.
func nanResult128(x, y decimal128) decimal128 {
	if bid.NativeUBD128 {
		r := bid.UBDNaN128(ubdWords(x), ubdWords(y))
		return FromBits128(r.Hi, r.Lo)
	}
	return d128(bid.Add128(b128(x), b128(y), bid.ToNearestEven))
}

// unpackDec128 returns x as a dec.
func unpackDec128(x decimal128) dec {
	if bid.NativeUBD128 {
		kind, neg, c, e := bid.UBDUnpack128(ubdWords(x))
		return dec{kind, neg, c, e}
	}
	return unpack128(b128(x))
}

// decimal128 rounds r to decimal128, to nearest with ties to even.
func (r result) decimal128() decimal128 {
	if bid.NativeUBD128 {
		switch r.kind {
		case bid.KindNaN:
			return FromBits128(bid.UBDNaNHi, 0)
		case bid.KindInf:
			hi := bid.UBDInfHi
			if r.neg {
				hi |= bid.UBDSignHi
			}
			return FromBits128(hi, 0)
		}
		x, _ := bid.UBDRound128(r.neg, r.c, r.e, r.sticky, bid.ToNearestEven)
		return FromBits128(x.Hi, x.Lo)
	}
	return d128(r.pack128())
}
