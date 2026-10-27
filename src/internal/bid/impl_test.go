// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// impl is the package API seen through one width-generic interface, so
// that the contract examples and the differential tests drive the
// reference model and the kernel through the same calls. The format
// argument selects the width; a decimal64 travels in the Lo word of a
// bits value.

import (
	"fmt"
	"internal/bid"
	"math"
	"math/big"
)

type impl interface {
	add(f *format, x, y bits, m bid.Mode) bits
	sub(f *format, x, y bits, m bid.Mode) bits
	mul(f *format, x, y bits, m bid.Mode) bits
	div(f *format, x, y bits, m bid.Mode) bits
	fma(f *format, x, y, z bits, m bid.Mode) bits
	rem(f *format, x, y bits) bits
	mod(f *format, x, y bits) bits
	roundToIntegral(f *format, x bits, m bid.Mode) bits
	quantize(f *format, x, y bits, m bid.Mode) bits
	reduce(f *format, x bits) bits
	sameQuantum(f *format, x, y bits) bool
	min(f *format, x, y bits) bits
	max(f *format, x, y bits) bits
	equal(f *format, x, y bits) bool
	less(f *format, x, y bits) bool
	lessEqual(f *format, x, y bits) bool
	compare(f *format, x, y bits) (int, bool)
	totalOrder(f *format, x, y bits) bool
	key(f *format, x bits) bid.Key

	unpack(f *format, x bits) (bid.Kind, bool, bid.Uint128, int)
	pack(f *format, neg bool, c bid.Uint128, exp int) (bits, bool)
	isNaN(f *format, x bits) bool
	isSignaling(f *format, x bits) bool
	isInf(f *format, x bits) bool
	isFinite(f *format, x bits) bool
	isZero(f *format, x bits) bool
	isSubnormal(f *format, x bits) bool
	isCanonical(f *format, x bits) bool
	signbit(f *format, x bits) bool
	neg(f *format, x bits) bits
	abs(f *format, x bits) bits
	copySign(f *format, x, s bits) bits

	widen(x bits) bits
	narrow(x bits, m bid.Mode) bits
	int64To(f *format, x int64, m bid.Mode) bits
	uint64To(f *format, x uint64, m bid.Mode) bits
	int64From(f *format, x bits, m bid.Mode) (int64, bid.Status)
	uint64From(f *format, x bits, m bid.Mode) (uint64, bid.Status)
	float64To(f *format, x float64, m bid.Mode) bits
	float32To(f *format, x float32, m bid.Mode) bits
	float64From(f *format, x bits, m bid.Mode) float64
	float32From(f *format, x bits, m bid.Mode) float32

	parse(f *format, s string, m bid.Mode) (bits, bid.Status)
	appendTo(f *format, x bits) string
	round(f *format, neg bool, c bid.Uint128, exp int, sticky bool, m bid.Mode) (bits, bid.Status)
}

// reference is the reference model as an impl.
type reference struct{}

func (reference) add(f *format, x, y bits, m bid.Mode) bits    { return f.add(x, y, m) }
func (reference) sub(f *format, x, y bits, m bid.Mode) bits    { return f.sub(x, y, m) }
func (reference) mul(f *format, x, y bits, m bid.Mode) bits    { return f.mul(x, y, m) }
func (reference) div(f *format, x, y bits, m bid.Mode) bits    { return f.div(x, y, m) }
func (reference) fma(f *format, x, y, z bits, m bid.Mode) bits { return f.fma(x, y, z, m) }
func (reference) rem(f *format, x, y bits) bits                { return f.remainder(x, y, false) }
func (reference) mod(f *format, x, y bits) bits                { return f.remainder(x, y, true) }
func (reference) roundToIntegral(f *format, x bits, m bid.Mode) bits {
	return f.roundToIntegral(x, m)
}
func (reference) quantize(f *format, x, y bits, m bid.Mode) bits { return f.quantize(x, y, m) }
func (reference) reduce(f *format, x bits) bits                  { return f.reduce(x) }
func (reference) sameQuantum(f *format, x, y bits) bool          { return f.sameQuantum(x, y) }
func (reference) min(f *format, x, y bits) bits                  { return f.minMax(x, y, false) }
func (reference) max(f *format, x, y bits) bits                  { return f.minMax(x, y, true) }
func (reference) equal(f *format, x, y bits) bool {
	c, ok := f.compare(x, y)
	return ok && c == 0
}
func (reference) less(f *format, x, y bits) bool {
	c, ok := f.compare(x, y)
	return ok && c < 0
}
func (reference) lessEqual(f *format, x, y bits) bool {
	c, ok := f.compare(x, y)
	return ok && c <= 0
}
func (reference) compare(f *format, x, y bits) (int, bool) { return f.compare(x, y) }
func (reference) totalOrder(f *format, x, y bits) bool     { return f.totalOrder(x, y) }
func (reference) key(f *format, x bits) bid.Key            { return f.key(x) }

func (reference) unpack(f *format, x bits) (bid.Kind, bool, bid.Uint128, int) {
	k, neg, c, exp := f.unpack(x)
	return k, neg, toUint128(c), exp
}
func (reference) pack(f *format, neg bool, c bid.Uint128, exp int) (bits, bool) {
	return f.pack(neg, fromUint128(c), exp)
}
func (reference) isNaN(f *format, x bits) bool       { return f.decode(x).kind == notANumber }
func (reference) isSignaling(f *format, x bits) bool { return f.decode(x).signaling }
func (reference) isInf(f *format, x bits) bool       { return f.decode(x).kind == infinite }
func (reference) isFinite(f *format, x bits) bool    { return f.decode(x).kind == finite }
func (reference) isZero(f *format, x bits) bool {
	v := f.decode(x)
	return v.kind == finite && v.coeff.Sign() == 0
}
func (reference) isSubnormal(f *format, x bits) bool { return f.isSubnormal(x) }
func (reference) isCanonical(f *format, x bits) bool { return f.canonical(x) }
func (reference) signbit(f *format, x bits) bool     { return f.signbit(x) }
func (reference) neg(f *format, x bits) bits         { return f.withSign(x, !f.signbit(x)) }
func (reference) abs(f *format, x bits) bits         { return f.withSign(x, false) }
func (reference) copySign(f *format, x, s bits) bits { return f.withSign(x, f.signbit(s)) }
func (reference) widen(x bits) bits                  { return widen(x) }
func (reference) narrow(x bits, m bid.Mode) bits     { return narrow(x, m) }
func (reference) int64To(f *format, x int64, m bid.Mode) bits {
	return f.fromInt(big.NewInt(x), m)
}
func (reference) uint64To(f *format, x uint64, m bid.Mode) bits {
	return f.fromInt(new(big.Int).SetUint64(x), m)
}
func (reference) int64From(f *format, x bits, m bid.Mode) (int64, bid.Status) {
	n, st := f.toInt(x, m, minInt64, maxInt64)
	return n.Int64(), st
}
func (reference) uint64From(f *format, x bits, m bid.Mode) (uint64, bid.Status) {
	n, st := f.toInt(x, m, bigZero, maxUint64)
	return n.Uint64(), st
}
func (reference) float64To(f *format, x float64, m bid.Mode) bits { return f.fromFloat(x, m) }
func (reference) float32To(f *format, x float32, m bid.Mode) bits {
	return f.fromFloat(float64(x), m)
}
func (reference) float64From(f *format, x bits, m bid.Mode) float64 {
	return math.Float64frombits(f.toFloat(x, m, binary64))
}
func (reference) float32From(f *format, x bits, m bid.Mode) float32 {
	return math.Float32frombits(uint32(f.toFloat(x, m, binary32)))
}
func (reference) parse(f *format, s string, m bid.Mode) (bits, bid.Status) { return f.parse(s, m) }
func (reference) appendTo(f *format, x bits) string                        { return f.format(x) }
func (reference) round(f *format, neg bool, c bid.Uint128, exp int, sticky bool, m bid.Mode) (bits, bid.Status) {
	if f.width == 64 && c.Hi != 0 {
		panic("round: decimal64 coefficient wider than 64 bits")
	}
	return f.roundParts(neg, fromUint128(c), exp, sticky, m)
}

// kernel is internal/bid as an impl.
type kernel struct{}

func w64(f *format) bool { return f.width == 64 }

func lo(x uint64) bits { return bits{Lo: x} }

func (kernel) add(f *format, x, y bits, m bid.Mode) bits {
	if w64(f) {
		return lo(bid.Add64(x.Lo, y.Lo, m))
	}
	return bid.Add128(x, y, m)
}
func (kernel) sub(f *format, x, y bits, m bid.Mode) bits {
	if w64(f) {
		return lo(bid.Sub64(x.Lo, y.Lo, m))
	}
	return bid.Sub128(x, y, m)
}
func (kernel) mul(f *format, x, y bits, m bid.Mode) bits {
	if w64(f) {
		return lo(bid.Mul64(x.Lo, y.Lo, m))
	}
	return bid.Mul128(x, y, m)
}
func (kernel) div(f *format, x, y bits, m bid.Mode) bits {
	if w64(f) {
		return lo(bid.Div64(x.Lo, y.Lo, m))
	}
	return bid.Div128(x, y, m)
}
func (kernel) fma(f *format, x, y, z bits, m bid.Mode) bits {
	if w64(f) {
		return lo(bid.FMA64(x.Lo, y.Lo, z.Lo, m))
	}
	return bid.FMA128(x, y, z, m)
}
func (kernel) rem(f *format, x, y bits) bits {
	if w64(f) {
		return lo(bid.Rem64(x.Lo, y.Lo))
	}
	return bid.Rem128(x, y)
}
func (kernel) mod(f *format, x, y bits) bits {
	if w64(f) {
		return lo(bid.Mod64(x.Lo, y.Lo))
	}
	return bid.Mod128(x, y)
}
func (kernel) roundToIntegral(f *format, x bits, m bid.Mode) bits {
	if w64(f) {
		return lo(bid.RoundToIntegral64(x.Lo, m))
	}
	return bid.RoundToIntegral128(x, m)
}
func (kernel) quantize(f *format, x, y bits, m bid.Mode) bits {
	if w64(f) {
		return lo(bid.Quantize64(x.Lo, y.Lo, m))
	}
	return bid.Quantize128(x, y, m)
}
func (kernel) reduce(f *format, x bits) bits {
	if w64(f) {
		return lo(bid.Reduce64(x.Lo))
	}
	return bid.Reduce128(x)
}
func (kernel) sameQuantum(f *format, x, y bits) bool {
	if w64(f) {
		return bid.SameQuantum64(x.Lo, y.Lo)
	}
	return bid.SameQuantum128(x, y)
}
func (kernel) min(f *format, x, y bits) bits {
	if w64(f) {
		return lo(bid.Min64(x.Lo, y.Lo))
	}
	return bid.Min128(x, y)
}
func (kernel) max(f *format, x, y bits) bits {
	if w64(f) {
		return lo(bid.Max64(x.Lo, y.Lo))
	}
	return bid.Max128(x, y)
}
func (kernel) equal(f *format, x, y bits) bool {
	if w64(f) {
		return bid.Equal64(x.Lo, y.Lo)
	}
	return bid.Equal128(x, y)
}
func (kernel) less(f *format, x, y bits) bool {
	if w64(f) {
		return bid.Less64(x.Lo, y.Lo)
	}
	return bid.Less128(x, y)
}
func (kernel) lessEqual(f *format, x, y bits) bool {
	if w64(f) {
		return bid.LessEqual64(x.Lo, y.Lo)
	}
	return bid.LessEqual128(x, y)
}
func (kernel) compare(f *format, x, y bits) (int, bool) {
	if w64(f) {
		return bid.Compare64(x.Lo, y.Lo)
	}
	return bid.Compare128(x, y)
}
func (kernel) totalOrder(f *format, x, y bits) bool {
	if w64(f) {
		return bid.TotalOrder64(x.Lo, y.Lo)
	}
	return bid.TotalOrder128(x, y)
}
func (kernel) key(f *format, x bits) bid.Key {
	if w64(f) {
		return bid.Key64(x.Lo)
	}
	return bid.Key128(x)
}
func (kernel) unpack(f *format, x bits) (bid.Kind, bool, bid.Uint128, int) {
	if w64(f) {
		k, neg, c, exp := bid.Unpack64(x.Lo)
		return k, neg, bid.Uint128{Lo: c}, exp
	}
	return bid.Unpack128(x)
}
func (kernel) pack(f *format, neg bool, c bid.Uint128, exp int) (bits, bool) {
	if w64(f) {
		if c.Hi != 0 {
			panic("pack: decimal64 coefficient wider than 64 bits")
		}
		x, ok := bid.Pack64(neg, c.Lo, exp)
		return lo(x), ok
	}
	return bid.Pack128(neg, c, exp)
}
func (kernel) isNaN(f *format, x bits) bool {
	if w64(f) {
		return bid.IsNaN64(x.Lo)
	}
	return bid.IsNaN128(x)
}
func (kernel) isSignaling(f *format, x bits) bool {
	if w64(f) {
		return bid.IsSignaling64(x.Lo)
	}
	return bid.IsSignaling128(x)
}
func (kernel) isInf(f *format, x bits) bool {
	if w64(f) {
		return bid.IsInf64(x.Lo)
	}
	return bid.IsInf128(x)
}
func (kernel) isFinite(f *format, x bits) bool {
	if w64(f) {
		return bid.IsFinite64(x.Lo)
	}
	return bid.IsFinite128(x)
}
func (kernel) isZero(f *format, x bits) bool {
	if w64(f) {
		return bid.IsZero64(x.Lo)
	}
	return bid.IsZero128(x)
}
func (kernel) isSubnormal(f *format, x bits) bool {
	if w64(f) {
		return bid.IsSubnormal64(x.Lo)
	}
	return bid.IsSubnormal128(x)
}
func (kernel) isCanonical(f *format, x bits) bool {
	if w64(f) {
		return bid.IsCanonical64(x.Lo)
	}
	return bid.IsCanonical128(x)
}
func (kernel) signbit(f *format, x bits) bool {
	if w64(f) {
		return bid.Signbit64(x.Lo)
	}
	return bid.Signbit128(x)
}
func (kernel) neg(f *format, x bits) bits {
	if w64(f) {
		return lo(bid.Neg64(x.Lo))
	}
	return bid.Neg128(x)
}
func (kernel) abs(f *format, x bits) bits {
	if w64(f) {
		return lo(bid.Abs64(x.Lo))
	}
	return bid.Abs128(x)
}
func (kernel) copySign(f *format, x, s bits) bits {
	if w64(f) {
		return lo(bid.CopySign64(x.Lo, s.Lo))
	}
	return bid.CopySign128(x, s)
}
func (kernel) widen(x bits) bits              { return bid.Widen(x.Lo) }
func (kernel) narrow(x bits, m bid.Mode) bits { return lo(bid.Narrow(x, m)) }
func (kernel) int64To(f *format, x int64, m bid.Mode) bits {
	if w64(f) {
		return lo(bid.Int64To64(x, m))
	}
	return bid.Int64To128(x)
}
func (kernel) uint64To(f *format, x uint64, m bid.Mode) bits {
	if w64(f) {
		return lo(bid.Uint64To64(x, m))
	}
	return bid.Uint64To128(x)
}
func (kernel) int64From(f *format, x bits, m bid.Mode) (int64, bid.Status) {
	if w64(f) {
		return bid.Int64From64(x.Lo, m)
	}
	return bid.Int64From128(x, m)
}
func (kernel) uint64From(f *format, x bits, m bid.Mode) (uint64, bid.Status) {
	if w64(f) {
		return bid.Uint64From64(x.Lo, m)
	}
	return bid.Uint64From128(x, m)
}
func (kernel) float64To(f *format, x float64, m bid.Mode) bits {
	if w64(f) {
		return lo(bid.Float64To64(x, m))
	}
	return bid.Float64To128(x, m)
}
func (kernel) float32To(f *format, x float32, m bid.Mode) bits {
	if w64(f) {
		return lo(bid.Float32To64(x, m))
	}
	return bid.Float32To128(x, m)
}
func (kernel) float64From(f *format, x bits, m bid.Mode) float64 {
	if w64(f) {
		return bid.Float64From64(x.Lo, m)
	}
	return bid.Float64From128(x, m)
}
func (kernel) float32From(f *format, x bits, m bid.Mode) float32 {
	if w64(f) {
		return bid.Float32From64(x.Lo, m)
	}
	return bid.Float32From128(x, m)
}
func (kernel) parse(f *format, s string, m bid.Mode) (bits, bid.Status) {
	if w64(f) {
		x, st := bid.Parse64(s, m)
		return lo(x), st
	}
	return bid.Parse128(s, m)
}
func (kernel) appendTo(f *format, x bits) string {
	if w64(f) {
		return string(bid.Append64(nil, x.Lo))
	}
	return string(bid.Append128(nil, x))
}
func (kernel) round(f *format, neg bool, c bid.Uint128, exp int, sticky bool, m bid.Mode) (bits, bid.Status) {
	if w64(f) {
		if c.Hi != 0 {
			panic("round: decimal64 coefficient wider than 64 bits")
		}
		x, st := bid.Round64(neg, c.Lo, exp, sticky, m)
		return lo(x), st
	}
	return bid.Round128(neg, c, exp, sticky, m)
}

// notImplemented is the panic value of an unimplemented kernel function.
const notImplemented = "internal/bid: not implemented"

// try runs fn and reports whether the kernel implements what it calls. A
// panic other than the not-implemented one is a contract violation (no
// function may panic on any input) and is returned as an error.
func try(fn func()) (implemented bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			if r == notImplemented {
				implemented = false
				return
			}
			implemented, err = true, fmt.Errorf("panic: %v", r)
		}
	}()
	fn()
	return true, nil
}
