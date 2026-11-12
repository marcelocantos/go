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
	b := b128(x)
	if bid.IsNaN128(b) {
		return d128(bid.Add128(b, b, bid.ToNearestEven))
	}
	return d128(op(&format128, unpack128(b)).pack128())
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
	b := b128(x)
	if bid.IsNaN128(b) && !(one && n == 0 && !bid.IsSignaling128(b)) {
		return d128(bid.Add128(b, b, bid.ToNearestEven))
	}
	return d128(op(&format128, unpack128(b), int64(n)).pack128())
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

func LogB[D Decimal](x D) D {
	if is64(x) {
		return D(logB64(decimal64(x)))
	}
	return D(logB128(decimal128(x)))
}

func logB64(x decimal64) decimal64    { return unary64(x, (*format).logBDec) }
func logB128(x decimal128) decimal128 { return unary128(x, (*format).logBDec) }

func NextUp[D Decimal](x D) D {
	if is64(x) {
		return D(nextUp64(decimal64(x)))
	}
	return D(nextUp128(decimal128(x)))
}

func nextUp64(x decimal64) decimal64    { return unary64(x, (*format).nextUpDec) }
func nextUp128(x decimal128) decimal128 { return unary128(x, (*format).nextUpDec) }

func NextDown[D Decimal](x D) D {
	if is64(x) {
		return D(nextDown64(decimal64(x)))
	}
	return D(nextDown128(decimal128(x)))
}

func nextDown64(x decimal64) decimal64    { return unary64(x, (*format).nextDownDec) }
func nextDown128(x decimal128) decimal128 { return unary128(x, (*format).nextDownDec) }

func ScaleB[D Decimal](x D, n int) D {
	if is64(x) {
		return D(scaleB64(decimal64(x), n))
	}
	return D(scaleB128(decimal128(x), n))
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
	bx, by := b128(x), b128(y)
	if bid.IsSignaling128(bx) || bid.IsSignaling128(by) {
		return d128(bid.Add128(bx, by, bid.ToNearestEven))
	}
	r := format128.powDec(unpack128(bx), unpack128(by))
	if r.kind == bid.KindNaN && (bid.IsNaN128(bx) || bid.IsNaN128(by)) {
		return d128(bid.Add128(bx, by, bid.ToNearestEven))
	}
	return d128(r.pack128())
}
