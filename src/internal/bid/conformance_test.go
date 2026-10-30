// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// The conformance suite in testdata fixes the expected result of every
// operation of the package at every boundary the package documentation
// names. The vectors were derived from that documentation and from the
// text of IEEE 754-2008, not from this package, from the reference oracle
// or from any other library; testdata/README documents the format and
// tabulates the vectors. This file reads the suite and runs it against an
// implementation table, [conformanceOps]. The table filled with the
// kernel's functions is [kernelOps]; another table can run the same suite
// against a different implementation.
//
// The reader and writer of decimal values below are deliberately
// independent of the package: they map the character form of the vectors
// to bit patterns through the IEEE 754-2008 clause 3.5.2 layout with no
// call into the kernel, so the suite still means something when the
// kernel is wrong.

import (
	"bufio"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"internal/bid"
)

// conformanceOps is an implementation of the package API, one function
// per operation and width. The conformance runner dispatches each vector
// to the field named by the vector's operation.
type conformanceOps struct {
	// arith.go
	Add64, Sub64, Mul64, Div64     func(x, y uint64, mode bid.Mode) uint64
	Add128, Sub128, Mul128, Div128 func(x, y bid.Uint128, mode bid.Mode) bid.Uint128
	FMA64                          func(x, y, z uint64, mode bid.Mode) uint64
	FMA128                         func(x, y, z bid.Uint128, mode bid.Mode) bid.Uint128
	Rem64, Mod64                   func(x, y uint64) uint64
	Rem128, Mod128                 func(x, y bid.Uint128) bid.Uint128
	RoundToIntegral64              func(x uint64, mode bid.Mode) uint64
	RoundToIntegral128             func(x bid.Uint128, mode bid.Mode) bid.Uint128
	Quantize64                     func(x, y uint64, mode bid.Mode) uint64
	Quantize128                    func(x, y bid.Uint128, mode bid.Mode) bid.Uint128
	Reduce64                       func(x uint64) uint64
	Reduce128                      func(x bid.Uint128) bid.Uint128
	SameQuantum64                  func(x, y uint64) bool
	SameQuantum128                 func(x, y bid.Uint128) bool
	Min64, Max64                   func(x, y uint64) uint64
	Min128, Max128                 func(x, y bid.Uint128) bid.Uint128

	// round.go
	Round64  func(neg bool, coeff uint64, exp int, sticky bool, mode bid.Mode) (uint64, bid.Status)
	Round128 func(neg bool, coeff bid.Uint128, exp int, sticky bool, mode bid.Mode) (bid.Uint128, bid.Status)

	// compare.go
	Equal64, Less64, LessEqual64, TotalOrder64     func(x, y uint64) bool
	Equal128, Less128, LessEqual128, TotalOrder128 func(x, y bid.Uint128) bool
	Compare64                                      func(x, y uint64) (int, bool)
	Compare128                                     func(x, y bid.Uint128) (int, bool)
	Key64                                          func(x uint64) bid.Key
	Key128                                         func(x bid.Uint128) bid.Key

	// convert.go
	Widen         func(x uint64) bid.Uint128
	Narrow        func(x bid.Uint128, mode bid.Mode) uint64
	Int64To64     func(x int64, mode bid.Mode) uint64
	Int64To128    func(x int64) bid.Uint128
	Uint64To64    func(x uint64, mode bid.Mode) uint64
	Uint64To128   func(x uint64) bid.Uint128
	Int64From64   func(x uint64, mode bid.Mode) (int64, bid.Status)
	Int64From128  func(x bid.Uint128, mode bid.Mode) (int64, bid.Status)
	Uint64From64  func(x uint64, mode bid.Mode) (uint64, bid.Status)
	Uint64From128 func(x bid.Uint128, mode bid.Mode) (uint64, bid.Status)

	// float.go
	Float64To64    func(f float64, mode bid.Mode) uint64
	Float64To128   func(f float64, mode bid.Mode) bid.Uint128
	Float32To64    func(f float32, mode bid.Mode) uint64
	Float32To128   func(f float32, mode bid.Mode) bid.Uint128
	Float64From64  func(x uint64, mode bid.Mode) float64
	Float64From128 func(x bid.Uint128, mode bid.Mode) float64
	Float32From64  func(x uint64, mode bid.Mode) float32
	Float32From128 func(x bid.Uint128, mode bid.Mode) float32

	// string.go
	Append64  func(dst []byte, x uint64) []byte
	Append128 func(dst []byte, x bid.Uint128) []byte
	Parse64   func(s string, mode bid.Mode) (uint64, bid.Status)
	Parse128  func(s string, mode bid.Mode) (bid.Uint128, bid.Status)

	// encoding.go
	Unpack64  func(x uint64) (bid.Kind, bool, uint64, int)
	Unpack128 func(x bid.Uint128) (bid.Kind, bool, bid.Uint128, int)
	Pack64    func(neg bool, coeff uint64, exp int) (uint64, bool)
	Pack128   func(neg bool, coeff bid.Uint128, exp int) (bid.Uint128, bool)

	IsNaN64, IsSignaling64, IsInf64, IsFinite64, IsZero64, IsSubnormal64, IsCanonical64, Signbit64         func(x uint64) bool
	IsNaN128, IsSignaling128, IsInf128, IsFinite128, IsZero128, IsSubnormal128, IsCanonical128, Signbit128 func(x bid.Uint128) bool

	Neg64, Abs64   func(x uint64) uint64
	Neg128, Abs128 func(x bid.Uint128) bid.Uint128
	CopySign64     func(x, sign uint64) uint64
	CopySign128    func(x, sign bid.Uint128) bid.Uint128
}

// kernelOps is the package under test.
var kernelOps = conformanceOps{
	Add64: bid.Add64, Sub64: bid.Sub64, Mul64: bid.Mul64, Div64: bid.Div64,
	Add128: bid.Add128, Sub128: bid.Sub128, Mul128: bid.Mul128, Div128: bid.Div128,
	FMA64: bid.FMA64, FMA128: bid.FMA128,
	Rem64: bid.Rem64, Mod64: bid.Mod64, Rem128: bid.Rem128, Mod128: bid.Mod128,
	RoundToIntegral64: bid.RoundToIntegral64, RoundToIntegral128: bid.RoundToIntegral128,
	Quantize64: bid.Quantize64, Quantize128: bid.Quantize128,
	Reduce64: bid.Reduce64, Reduce128: bid.Reduce128,
	SameQuantum64: bid.SameQuantum64, SameQuantum128: bid.SameQuantum128,
	Min64: bid.Min64, Max64: bid.Max64, Min128: bid.Min128, Max128: bid.Max128,

	Round64: bid.Round64, Round128: bid.Round128,

	Equal64: bid.Equal64, Less64: bid.Less64, LessEqual64: bid.LessEqual64, TotalOrder64: bid.TotalOrder64,
	Equal128: bid.Equal128, Less128: bid.Less128, LessEqual128: bid.LessEqual128, TotalOrder128: bid.TotalOrder128,
	Compare64: bid.Compare64, Compare128: bid.Compare128,
	Key64: bid.Key64, Key128: bid.Key128,

	Widen: bid.Widen, Narrow: bid.Narrow,
	Int64To64: bid.Int64To64, Int64To128: bid.Int64To128,
	Uint64To64: bid.Uint64To64, Uint64To128: bid.Uint64To128,
	Int64From64: bid.Int64From64, Int64From128: bid.Int64From128,
	Uint64From64: bid.Uint64From64, Uint64From128: bid.Uint64From128,

	Float64To64: bid.Float64To64, Float64To128: bid.Float64To128,
	Float32To64: bid.Float32To64, Float32To128: bid.Float32To128,
	Float64From64: bid.Float64From64, Float64From128: bid.Float64From128,
	Float32From64: bid.Float32From64, Float32From128: bid.Float32From128,

	Append64: bid.Append64, Append128: bid.Append128,
	Parse64: bid.Parse64, Parse128: bid.Parse128,

	Unpack64: bid.Unpack64, Unpack128: bid.Unpack128,
	Pack64: bid.Pack64, Pack128: bid.Pack128,
	IsNaN64: bid.IsNaN64, IsSignaling64: bid.IsSignaling64, IsInf64: bid.IsInf64, IsFinite64: bid.IsFinite64,
	IsZero64: bid.IsZero64, IsSubnormal64: bid.IsSubnormal64, IsCanonical64: bid.IsCanonical64, Signbit64: bid.Signbit64,
	IsNaN128: bid.IsNaN128, IsSignaling128: bid.IsSignaling128, IsInf128: bid.IsInf128, IsFinite128: bid.IsFinite128,
	IsZero128: bid.IsZero128, IsSubnormal128: bid.IsSubnormal128, IsCanonical128: bid.IsCanonical128, Signbit128: bid.Signbit128,
	Neg64: bid.Neg64, Abs64: bid.Abs64, Neg128: bid.Neg128, Abs128: bid.Abs128,
	CopySign64: bid.CopySign64, CopySign128: bid.CopySign128,
}

// conformanceOpNames lists every operation the suite covers, in the order
// of the README table. The runner rejects a vector naming any other
// operation, and TestConformanceREADME requires every operation here to
// have a row.
var conformanceOpNames = []string{
	"Add64", "Add128", "Sub64", "Sub128", "Mul64", "Mul128", "Div64", "Div128", "FMA64", "FMA128",
	"Rem64", "Rem128", "Mod64", "Mod128",
	"RoundToIntegral64", "RoundToIntegral128", "Quantize64", "Quantize128", "Reduce64", "Reduce128",
	"SameQuantum64", "SameQuantum128", "Min64", "Min128", "Max64", "Max128",
	"Round64", "Round128",
	"Equal64", "Equal128", "Less64", "Less128", "LessEqual64", "LessEqual128",
	"Compare64", "Compare128", "TotalOrder64", "TotalOrder128", "Key64", "Key128",
	"Widen", "Narrow", "Int64To64", "Int64To128", "Uint64To64", "Uint64To128",
	"Int64From64", "Int64From128", "Uint64From64", "Uint64From128",
	"Float64To64", "Float64To128", "Float32To64", "Float32To128",
	"Float64From64", "Float64From128", "Float32From64", "Float32From128",
	"Append64", "Append128", "Parse64", "Parse128",
	"Unpack64", "Unpack128", "Pack64", "Pack128",
	"IsNaN64", "IsNaN128", "IsSignaling64", "IsSignaling128", "IsInf64", "IsInf128",
	"IsFinite64", "IsFinite128", "IsZero64", "IsZero128", "IsSubnormal64", "IsSubnormal128",
	"IsCanonical64", "IsCanonical128", "Signbit64", "Signbit128",
	"Neg64", "Neg128", "Abs64", "Abs128", "CopySign64", "CopySign128",
}

// conformanceClasses are the boundary classes of the README, in table
// order.
var conformanceClasses = []string{
	"exact", "inexact", "tie", "carry", "overflow", "underflow",
	"clamp", "zero", "inf", "nan", "noncanon", "grammar",
}

// conformanceModes maps the mode token of a vector to a Mode. A token of
// one or more digits is the Mode with that numeric value, so the suite can
// name a Mode outside the five defined ones; "-" is for operations that
// take no mode.
var conformanceModes = map[string]bid.Mode{
	"even": bid.ToNearestEven,
	"away": bid.ToNearestAway,
	"zero": bid.TowardZero,
	"up":   bid.TowardPositive,
	"down": bid.TowardNegative,
}

// A vector is one line of the suite: an operation, a mode, a boundary
// class, operand tokens and expected result tokens.
type vector struct {
	pos   string
	op    string
	mode  bid.Mode
	class string
	args  []string
	want  []string
}

// notImplementedPanic is the panic value of an operation the kernel has
// declared but not implemented. The runner skips vectors for such an
// operation and counts them; a vector failing in any other way fails the
// test.
const notImplementedPanic = "internal/bid: not implemented"

// Independent model of a decimal value: (-1)^neg × coeff × 10^exp, an
// infinity or a NaN. The coefficient is a big.Int so one model serves both
// widths.
type decimal struct {
	kind  bid.Kind
	neg   bool
	snan  bool
	coeff *big.Int
	exp   int
}

// IEEE 754-2008 clause 3.5.2 layout of the BID encodings, written out here
// rather than taken from the package so that the suite reads encodings on
// its own authority.
const (
	sign64Shift   = 63
	expBits64     = 10
	form1Shift64  = 53 // exponent field above 53 coefficient bits
	form2Shift64  = 51 // exponent field above 51 coefficient bits, implicit 100
	specialBits64 = 0xF << 59
	nanBit64      = 1 << 58
	snanBit64     = 1 << 57
	bias64        = 398
	minExp64      = -398
	maxExp64      = 369
	precision64   = 16

	expBits128     = 14
	form1Shift128  = 49 // within the high word; the low word is coefficient
	form2Shift128  = 47
	specialBits128 = 0xF << 59
	nanBit128      = 1 << 58
	snanBit128     = 1 << 57
	bias128        = 6176
	minExp128      = -6176
	maxExp128      = 6111
	precision128   = 34
)

var (
	big10       = big.NewInt(10)
	maxCoeff64  = new(big.Int).Sub(new(big.Int).Exp(big10, big.NewInt(precision64), nil), big.NewInt(1))
	maxCoeff128 = new(big.Int).Sub(new(big.Int).Exp(big10, big.NewInt(precision128), nil), big.NewInt(1))
	two64       = new(big.Int).Lsh(big.NewInt(1), 64)
)

// parseDecimal reads the character form of the package documentation
// extended by the operand spellings -NaN, sNaN and -sNaN.
func parseDecimal(s string) (decimal, error) {
	var d decimal
	orig := s
	if strings.HasPrefix(s, "-") {
		d.neg = true
		s = s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	switch s {
	case "Infinity":
		d.kind = bid.KindInf
		return d, nil
	case "NaN":
		d.kind = bid.KindNaN
		return d, nil
	case "sNaN":
		d.kind = bid.KindNaN
		d.snan = true
		return d, nil
	}
	mant, expPart, _ := strings.Cut(s, "E")
	exp := 0
	if expPart != "" || strings.Contains(s, "E") {
		if len(expPart) < 2 || (expPart[0] != '+' && expPart[0] != '-') {
			return d, fmt.Errorf("decimal %q: exponent needs a sign", orig)
		}
		e, err := strconv.Atoi(expPart)
		if err != nil {
			return d, fmt.Errorf("decimal %q: %v", orig, err)
		}
		exp = e
	}
	intPart, frac, hasPoint := strings.Cut(mant, ".")
	digits := intPart + frac
	if digits == "" || (hasPoint && frac == "") {
		return d, fmt.Errorf("decimal %q: not a numeral", orig)
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return d, fmt.Errorf("decimal %q: not a numeral", orig)
		}
	}
	d.coeff, _ = new(big.Int).SetString(digits, 10)
	d.exp = exp - len(frac)
	return d, nil
}

// formatDecimal writes the character form of the package documentation,
// with -NaN, sNaN and -sNaN for the NaN encodings Append never
// distinguishes, so that a mismatch report shows what the bits say.
func formatDecimal(d decimal) string {
	var b strings.Builder
	if d.neg {
		b.WriteByte('-')
	}
	switch d.kind {
	case bid.KindInf:
		b.WriteString("Infinity")
		return b.String()
	case bid.KindNaN:
		if d.snan {
			b.WriteByte('s')
		}
		b.WriteString("NaN")
		return b.String()
	}
	digits := d.coeff.String()
	adj := d.exp + len(digits) - 1
	if d.exp <= 0 && adj >= -6 {
		if d.exp == 0 {
			b.WriteString(digits)
			return b.String()
		}
		point := len(digits) + d.exp
		if point <= 0 {
			b.WriteString("0.")
			b.WriteString(strings.Repeat("0", -point))
			b.WriteString(digits)
			return b.String()
		}
		b.WriteString(digits[:point])
		b.WriteByte('.')
		b.WriteString(digits[point:])
		return b.String()
	}
	b.WriteByte(digits[0])
	if len(digits) > 1 {
		b.WriteByte('.')
		b.WriteString(digits[1:])
	}
	b.WriteByte('E')
	if adj >= 0 {
		b.WriteByte('+')
	}
	b.WriteString(strconv.Itoa(adj))
	return b.String()
}

// encode64 is the canonical decimal64 encoding of d; it fails when d is not
// a decimal64 value, which marks a malformed vector.
func encode64(d decimal) (uint64, error) {
	var x uint64
	if d.neg {
		x = 1 << sign64Shift
	}
	switch d.kind {
	case bid.KindInf:
		return x | specialBits64, nil
	case bid.KindNaN:
		x |= specialBits64 | nanBit64
		if d.snan {
			x |= snanBit64
		}
		return x, nil
	}
	if d.coeff.Cmp(maxCoeff64) > 0 || d.exp < minExp64 || d.exp > maxExp64 {
		return 0, fmt.Errorf("%s is not a decimal64 value", formatDecimal(d))
	}
	c := d.coeff.Uint64()
	e := uint64(d.exp + bias64)
	if c < 1<<form1Shift64 {
		return x | e<<form1Shift64 | c, nil
	}
	return x | 3<<61 | e<<form2Shift64 | c&(1<<form2Shift64-1), nil
}

// decode64 reads any decimal64 bit pattern as the value the contract gives
// it: a non-canonical coefficient reads as zero.
func decode64(x uint64) decimal {
	d := decimal{neg: x>>sign64Shift != 0, coeff: new(big.Int)}
	switch {
	case x&(specialBits64|nanBit64) == specialBits64|nanBit64:
		d.kind = bid.KindNaN
		d.snan = x&snanBit64 != 0
	case x&specialBits64 == specialBits64:
		d.kind = bid.KindInf
	case x>>61&3 == 3:
		d.exp = int(x>>form2Shift64&(1<<expBits64-1)) - bias64
		d.coeff.SetUint64(1<<53 | x&(1<<form2Shift64-1))
	default:
		d.exp = int(x>>form1Shift64&(1<<expBits64-1)) - bias64
		d.coeff.SetUint64(x & (1<<form1Shift64 - 1))
	}
	if d.coeff.Cmp(maxCoeff64) > 0 {
		d.coeff.SetUint64(0)
	}
	return d
}

func encode128(d decimal) (bid.Uint128, error) {
	var x bid.Uint128
	if d.neg {
		x.Hi = 1 << 63
	}
	switch d.kind {
	case bid.KindInf:
		x.Hi |= specialBits128
		return x, nil
	case bid.KindNaN:
		x.Hi |= specialBits128 | nanBit128
		if d.snan {
			x.Hi |= snanBit128
		}
		return x, nil
	}
	if d.coeff.Cmp(maxCoeff128) > 0 || d.exp < minExp128 || d.exp > maxExp128 {
		return x, fmt.Errorf("%s is not a decimal128 value", formatDecimal(d))
	}
	hi, lo := splitUint128(d.coeff)
	x.Hi |= uint64(d.exp+bias128)<<form1Shift128 | hi
	x.Lo = lo
	return x, nil
}

func decode128(x bid.Uint128) decimal {
	d := decimal{neg: x.Hi>>63 != 0, coeff: new(big.Int)}
	switch {
	case x.Hi&(specialBits128|nanBit128) == specialBits128|nanBit128:
		d.kind = bid.KindNaN
		d.snan = x.Hi&snanBit128 != 0
	case x.Hi&specialBits128 == specialBits128:
		d.kind = bid.KindInf
	case x.Hi>>61&3 == 3:
		d.exp = int(x.Hi>>form2Shift128&(1<<expBits128-1)) - bias128
		d.coeff = joinUint128(1<<49|x.Hi&(1<<form2Shift128-1), x.Lo)
	default:
		d.exp = int(x.Hi>>form1Shift128&(1<<expBits128-1)) - bias128
		d.coeff = joinUint128(x.Hi&(1<<form1Shift128-1), x.Lo)
	}
	if d.coeff.Cmp(maxCoeff128) > 0 {
		d.coeff.SetUint64(0)
	}
	return d
}

func splitUint128(c *big.Int) (hi, lo uint64) {
	var q, r big.Int
	q.DivMod(c, two64, &r)
	return q.Uint64(), r.Uint64()
}

func joinUint128(hi, lo uint64) *big.Int {
	c := new(big.Int).SetUint64(hi)
	c.Lsh(c, 64)
	return c.Or(c, new(big.Int).SetUint64(lo))
}

// The canonical spellings the runner compares. Every result, expected or
// obtained, is rendered through one of these so that a mismatch report
// shows both sides the same way.

func show64(x uint64) string {
	return fmt.Sprintf("%s [0x%016x]", formatDecimal(decode64(x)), x)
}

func show128(x bid.Uint128) string {
	return fmt.Sprintf("%s [0x%016x%016x]", formatDecimal(decode128(x)), x.Hi, x.Lo)
}

func showStatus(s bid.Status) string {
	if s == bid.Exact {
		return "Exact"
	}
	var parts []string
	for _, f := range []struct {
		bit  bid.Status
		name string
	}{{bid.Inexact, "Inexact"}, {bid.Overflow, "Overflow"}, {bid.Underflow, "Underflow"}, {bid.Invalid, "Invalid"}, {bid.Syntax, "Syntax"}} {
		if s&f.bit != 0 {
			parts = append(parts, f.name)
			s &^= f.bit
		}
	}
	if s != 0 {
		parts = append(parts, fmt.Sprintf("0x%x", uint8(s)))
	}
	return strings.Join(parts, "|")
}

func showKind(k bid.Kind) string {
	switch k {
	case bid.KindFinite:
		return "finite"
	case bid.KindInf:
		return "inf"
	case bid.KindNaN:
		return "nan"
	}
	return fmt.Sprintf("kind(%d)", k)
}

func showFloat64(f float64) string {
	return fmt.Sprintf("%v [0x%016x]", f, math.Float64bits(f))
}

func showFloat32(f float32) string {
	return fmt.Sprintf("%v [0x%08x]", f, math.Float32bits(f))
}

func showKey(k bid.Key) string {
	return fmt.Sprintf("%s %d %s %t", joinUint128(k.Coeff.Hi, k.Coeff.Lo), k.Exp, showKind(k.Kind), k.Neg)
}

// Operand readers. Each reads one or more vector tokens and fails on a
// token that does not denote a value of the type, which marks a malformed
// vector.

type operands struct {
	toks []string
	err  error
}

func (o *operands) next() string {
	if len(o.toks) == 0 {
		o.fail("missing operand")
		return ""
	}
	t := o.toks[0]
	o.toks = o.toks[1:]
	return t
}

func (o *operands) fail(format string, args ...any) {
	if o.err == nil {
		o.err = fmt.Errorf(format, args...)
	}
}

func (o *operands) dec64() uint64 {
	t := o.next()
	if strings.HasPrefix(t, "0x") {
		if len(t) != 2+16 {
			o.fail("hex decimal64 %q needs 16 hex digits", t)
			return 0
		}
		x, err := strconv.ParseUint(t[2:], 16, 64)
		if err != nil {
			o.fail("%v", err)
		}
		return x
	}
	d, err := parseDecimal(t)
	if err != nil {
		o.fail("%v", err)
		return 0
	}
	x, err := encode64(d)
	if err != nil {
		o.fail("%v", err)
	}
	return x
}

func (o *operands) dec128() bid.Uint128 {
	t := o.next()
	if strings.HasPrefix(t, "0x") {
		if len(t) != 2+32 {
			o.fail("hex decimal128 %q needs 32 hex digits", t)
			return bid.Uint128{}
		}
		hi, err1 := strconv.ParseUint(t[2:18], 16, 64)
		lo, err2 := strconv.ParseUint(t[18:], 16, 64)
		if err1 != nil || err2 != nil {
			o.fail("bad hex %q", t)
		}
		return bid.Uint128{Hi: hi, Lo: lo}
	}
	d, err := parseDecimal(t)
	if err != nil {
		o.fail("%v", err)
		return bid.Uint128{}
	}
	x, err := encode128(d)
	if err != nil {
		o.fail("%v", err)
	}
	return x
}

func (o *operands) boolean() bool {
	switch t := o.next(); t {
	case "true":
		return true
	case "false":
		return false
	default:
		o.fail("%q is not a bool", t)
		return false
	}
}

func (o *operands) int() int {
	t := o.next()
	v, err := strconv.Atoi(t)
	if err != nil {
		o.fail("%v", err)
	}
	return v
}

func (o *operands) int64() int64 {
	t := o.next()
	v, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		o.fail("%v", err)
	}
	return v
}

func (o *operands) uint64() uint64 {
	t := o.next()
	v, err := strconv.ParseUint(t, 10, 64)
	if err != nil {
		o.fail("%v", err)
	}
	return v
}

func (o *operands) uint128() bid.Uint128 {
	t := o.next()
	c, ok := new(big.Int).SetString(t, 10)
	if !ok || c.Sign() < 0 || c.BitLen() > 128 {
		o.fail("%q is not a Uint128", t)
		return bid.Uint128{}
	}
	hi, lo := splitUint128(c)
	return bid.Uint128{Hi: hi, Lo: lo}
}

// float reads a Go floating-point literal of the given size with Go's
// semantics (the nearest binary value), including the hexadecimal form,
// +Inf, -Inf, NaN and -0.
func (o *operands) float(bitSize int) float64 {
	t := o.next()
	f, err := strconv.ParseFloat(t, bitSize)
	if err != nil {
		o.fail("%v", err)
		return 0
	}
	return f
}

func (o *operands) str() string {
	t := o.next()
	s, err := strconv.Unquote(t)
	if err != nil {
		o.fail("string operand %q must be a Go quoted string", t)
	}
	return s
}

func (o *operands) done() {
	if len(o.toks) != 0 && o.err == nil {
		o.fail("%d operand(s) too many: %q", len(o.toks), o.toks)
	}
}

// Expected-result readers render the want tokens of a vector through the
// show functions so they compare textually with what the operation
// returned.

func (o *operands) wantDec64() string   { return show64(o.dec64()) }
func (o *operands) wantDec128() string  { return show128(o.dec128()) }
func (o *operands) wantBool() string    { return strconv.FormatBool(o.boolean()) }
func (o *operands) wantInt() string     { return strconv.Itoa(o.int()) }
func (o *operands) wantInt64() string   { return strconv.FormatInt(o.int64(), 10) }
func (o *operands) wantUint64() string  { return strconv.FormatUint(o.uint64(), 10) }
func (o *operands) wantStr() string     { return strconv.Quote(o.str()) }
func (o *operands) wantUint128() string { x := o.uint128(); return joinUint128(x.Hi, x.Lo).String() }
func (o *operands) wantKind() string {
	t := o.next()
	switch t {
	case "finite", "inf", "nan":
		return t
	}
	o.fail("%q is not a kind", t)
	return t
}

func (o *operands) wantStatus() string {
	t := o.next()
	var s bid.Status
	for _, name := range strings.Split(t, "|") {
		switch name {
		case "Exact":
		case "Inexact":
			s |= bid.Inexact
		case "Overflow":
			s |= bid.Overflow
		case "Underflow":
			s |= bid.Underflow
		case "Invalid":
			s |= bid.Invalid
		case "Syntax":
			s |= bid.Syntax
		default:
			o.fail("%q is not a Status", t)
		}
	}
	return showStatus(s)
}

// wantFloat renders an expected binary result. The token NaN denotes the
// quiet NaN with the sign bit clear and a zero payload, which is the only
// NaN the contract lets a conversion produce.
func (o *operands) wantFloat64() string {
	t := o.toks
	f := o.float(64)
	if math.IsNaN(f) {
		if len(t) > 0 && t[0] != "NaN" {
			o.fail("spell the float64 NaN result NaN")
		}
		return showFloat64(math.Float64frombits(0x7FF8000000000000))
	}
	return showFloat64(f)
}

func (o *operands) wantFloat32() string {
	t := o.toks
	f := o.float(32)
	if math.IsNaN(f) {
		if len(t) > 0 && t[0] != "NaN" {
			o.fail("spell the float32 NaN result NaN")
		}
		return showFloat32(math.Float32frombits(0x7FC00000))
	}
	return showFloat32(float32(f))
}

func (o *operands) wantKey() string {
	coeff := o.wantUint128()
	exp := o.wantInt()
	kind := o.wantKind()
	neg := o.wantBool()
	return coeff + " " + exp + " " + kind + " " + neg
}

// eval runs v against ops and returns what the operation produced and what
// the vector expects, each as a list of rendered results. skipped is set
// when the operation is not implemented. A malformed vector is an error.
func (ops *conformanceOps) eval(v *vector) (got, want []string, skipped bool, err error) {
	a := &operands{toks: v.args}
	w := &operands{toks: v.want}
	m := v.mode
	g := func(ss ...string) { got = ss }
	var run func()
	switch v.op {
	case "Add64", "Sub64", "Mul64", "Div64":
		fn := map[string]func(x, y uint64, mode bid.Mode) uint64{"Add64": ops.Add64, "Sub64": ops.Sub64, "Mul64": ops.Mul64, "Div64": ops.Div64}[v.op]
		x, y := a.dec64(), a.dec64()
		want = []string{w.wantDec64()}
		run = func() { g(show64(fn(x, y, m))) }
	case "Add128", "Sub128", "Mul128", "Div128":
		fn := map[string]func(x, y bid.Uint128, mode bid.Mode) bid.Uint128{"Add128": ops.Add128, "Sub128": ops.Sub128, "Mul128": ops.Mul128, "Div128": ops.Div128}[v.op]
		x, y := a.dec128(), a.dec128()
		want = []string{w.wantDec128()}
		run = func() { g(show128(fn(x, y, m))) }
	case "FMA64":
		x, y, z := a.dec64(), a.dec64(), a.dec64()
		want = []string{w.wantDec64()}
		run = func() { g(show64(ops.FMA64(x, y, z, m))) }
	case "FMA128":
		x, y, z := a.dec128(), a.dec128(), a.dec128()
		want = []string{w.wantDec128()}
		run = func() { g(show128(ops.FMA128(x, y, z, m))) }
	case "Rem64", "Mod64", "Min64", "Max64":
		fn := map[string]func(x, y uint64) uint64{"Rem64": ops.Rem64, "Mod64": ops.Mod64, "Min64": ops.Min64, "Max64": ops.Max64}[v.op]
		x, y := a.dec64(), a.dec64()
		want = []string{w.wantDec64()}
		run = func() { g(show64(fn(x, y))) }
	case "Rem128", "Mod128", "Min128", "Max128":
		fn := map[string]func(x, y bid.Uint128) bid.Uint128{"Rem128": ops.Rem128, "Mod128": ops.Mod128, "Min128": ops.Min128, "Max128": ops.Max128}[v.op]
		x, y := a.dec128(), a.dec128()
		want = []string{w.wantDec128()}
		run = func() { g(show128(fn(x, y))) }
	case "RoundToIntegral64":
		x := a.dec64()
		want = []string{w.wantDec64()}
		run = func() { g(show64(ops.RoundToIntegral64(x, m))) }
	case "RoundToIntegral128":
		x := a.dec128()
		want = []string{w.wantDec128()}
		run = func() { g(show128(ops.RoundToIntegral128(x, m))) }
	case "Quantize64":
		x, y := a.dec64(), a.dec64()
		want = []string{w.wantDec64()}
		run = func() { g(show64(ops.Quantize64(x, y, m))) }
	case "Quantize128":
		x, y := a.dec128(), a.dec128()
		want = []string{w.wantDec128()}
		run = func() { g(show128(ops.Quantize128(x, y, m))) }
	case "Reduce64", "Neg64", "Abs64":
		fn := map[string]func(x uint64) uint64{"Reduce64": ops.Reduce64, "Neg64": ops.Neg64, "Abs64": ops.Abs64}[v.op]
		x := a.dec64()
		want = []string{w.wantDec64()}
		run = func() { g(show64(fn(x))) }
	case "Reduce128", "Neg128", "Abs128":
		fn := map[string]func(x bid.Uint128) bid.Uint128{"Reduce128": ops.Reduce128, "Neg128": ops.Neg128, "Abs128": ops.Abs128}[v.op]
		x := a.dec128()
		want = []string{w.wantDec128()}
		run = func() { g(show128(fn(x))) }
	case "CopySign64":
		x, s := a.dec64(), a.dec64()
		want = []string{w.wantDec64()}
		run = func() { g(show64(ops.CopySign64(x, s))) }
	case "CopySign128":
		x, s := a.dec128(), a.dec128()
		want = []string{w.wantDec128()}
		run = func() { g(show128(ops.CopySign128(x, s))) }
	case "SameQuantum64", "Equal64", "Less64", "LessEqual64", "TotalOrder64":
		fn := map[string]func(x, y uint64) bool{"SameQuantum64": ops.SameQuantum64, "Equal64": ops.Equal64, "Less64": ops.Less64, "LessEqual64": ops.LessEqual64, "TotalOrder64": ops.TotalOrder64}[v.op]
		x, y := a.dec64(), a.dec64()
		want = []string{w.wantBool()}
		run = func() { g(strconv.FormatBool(fn(x, y))) }
	case "SameQuantum128", "Equal128", "Less128", "LessEqual128", "TotalOrder128":
		fn := map[string]func(x, y bid.Uint128) bool{"SameQuantum128": ops.SameQuantum128, "Equal128": ops.Equal128, "Less128": ops.Less128, "LessEqual128": ops.LessEqual128, "TotalOrder128": ops.TotalOrder128}[v.op]
		x, y := a.dec128(), a.dec128()
		want = []string{w.wantBool()}
		run = func() { g(strconv.FormatBool(fn(x, y))) }
	case "Compare64":
		x, y := a.dec64(), a.dec64()
		want = []string{w.wantInt(), w.wantBool()}
		run = func() { c, ok := ops.Compare64(x, y); g(strconv.Itoa(c), strconv.FormatBool(ok)) }
	case "Compare128":
		x, y := a.dec128(), a.dec128()
		want = []string{w.wantInt(), w.wantBool()}
		run = func() { c, ok := ops.Compare128(x, y); g(strconv.Itoa(c), strconv.FormatBool(ok)) }
	case "Key64":
		x := a.dec64()
		want = []string{w.wantKey()}
		run = func() { g(showKey(ops.Key64(x))) }
	case "Key128":
		x := a.dec128()
		want = []string{w.wantKey()}
		run = func() { g(showKey(ops.Key128(x))) }
	case "Round64":
		neg, coeff, exp, sticky := a.boolean(), a.uint64(), a.int(), a.boolean()
		want = []string{w.wantDec64(), w.wantStatus()}
		run = func() { x, s := ops.Round64(neg, coeff, exp, sticky, m); g(show64(x), showStatus(s)) }
	case "Round128":
		neg, coeff, exp, sticky := a.boolean(), a.uint128(), a.int(), a.boolean()
		want = []string{w.wantDec128(), w.wantStatus()}
		run = func() { x, s := ops.Round128(neg, coeff, exp, sticky, m); g(show128(x), showStatus(s)) }
	case "Widen":
		x := a.dec64()
		want = []string{w.wantDec128()}
		run = func() { g(show128(ops.Widen(x))) }
	case "Narrow":
		x := a.dec128()
		want = []string{w.wantDec64()}
		run = func() { g(show64(ops.Narrow(x, m))) }
	case "Int64To64":
		x := a.int64()
		want = []string{w.wantDec64()}
		run = func() { g(show64(ops.Int64To64(x, m))) }
	case "Int64To128":
		x := a.int64()
		want = []string{w.wantDec128()}
		run = func() { g(show128(ops.Int64To128(x))) }
	case "Uint64To64":
		x := a.uint64()
		want = []string{w.wantDec64()}
		run = func() { g(show64(ops.Uint64To64(x, m))) }
	case "Uint64To128":
		x := a.uint64()
		want = []string{w.wantDec128()}
		run = func() { g(show128(ops.Uint64To128(x))) }
	case "Int64From64":
		x := a.dec64()
		want = []string{w.wantInt64(), w.wantStatus()}
		run = func() { i, s := ops.Int64From64(x, m); g(strconv.FormatInt(i, 10), showStatus(s)) }
	case "Int64From128":
		x := a.dec128()
		want = []string{w.wantInt64(), w.wantStatus()}
		run = func() { i, s := ops.Int64From128(x, m); g(strconv.FormatInt(i, 10), showStatus(s)) }
	case "Uint64From64":
		x := a.dec64()
		want = []string{w.wantUint64(), w.wantStatus()}
		run = func() { u, s := ops.Uint64From64(x, m); g(strconv.FormatUint(u, 10), showStatus(s)) }
	case "Uint64From128":
		x := a.dec128()
		want = []string{w.wantUint64(), w.wantStatus()}
		run = func() { u, s := ops.Uint64From128(x, m); g(strconv.FormatUint(u, 10), showStatus(s)) }
	case "Float64To64":
		f := a.float(64)
		want = []string{w.wantDec64()}
		run = func() { g(show64(ops.Float64To64(f, m))) }
	case "Float64To128":
		f := a.float(64)
		want = []string{w.wantDec128()}
		run = func() { g(show128(ops.Float64To128(f, m))) }
	case "Float32To64":
		f := float32(a.float(32))
		want = []string{w.wantDec64()}
		run = func() { g(show64(ops.Float32To64(f, m))) }
	case "Float32To128":
		f := float32(a.float(32))
		want = []string{w.wantDec128()}
		run = func() { g(show128(ops.Float32To128(f, m))) }
	case "Float64From64":
		x := a.dec64()
		want = []string{w.wantFloat64()}
		run = func() { g(showFloat64(ops.Float64From64(x, m))) }
	case "Float64From128":
		x := a.dec128()
		want = []string{w.wantFloat64()}
		run = func() { g(showFloat64(ops.Float64From128(x, m))) }
	case "Float32From64":
		x := a.dec64()
		want = []string{w.wantFloat32()}
		run = func() { g(showFloat32(ops.Float32From64(x, m))) }
	case "Float32From128":
		x := a.dec128()
		want = []string{w.wantFloat32()}
		run = func() { g(showFloat32(ops.Float32From128(x, m))) }
	case "Append64":
		x := a.dec64()
		want = []string{w.wantStr()}
		run = func() { g(strconv.Quote(string(ops.Append64(nil, x)))) }
	case "Append128":
		x := a.dec128()
		want = []string{w.wantStr()}
		run = func() { g(strconv.Quote(string(ops.Append128(nil, x)))) }
	case "Parse64":
		s := a.str()
		want = []string{w.wantDec64(), w.wantStatus()}
		run = func() { x, st := ops.Parse64(s, m); g(show64(x), showStatus(st)) }
	case "Parse128":
		s := a.str()
		want = []string{w.wantDec128(), w.wantStatus()}
		run = func() { x, st := ops.Parse128(s, m); g(show128(x), showStatus(st)) }
	case "Unpack64":
		x := a.dec64()
		want = []string{w.wantKind(), w.wantBool(), w.wantUint64(), w.wantInt()}
		run = func() {
			k, neg, c, e := ops.Unpack64(x)
			g(showKind(k), strconv.FormatBool(neg), strconv.FormatUint(c, 10), strconv.Itoa(e))
		}
	case "Unpack128":
		x := a.dec128()
		want = []string{w.wantKind(), w.wantBool(), w.wantUint128(), w.wantInt()}
		run = func() {
			k, neg, c, e := ops.Unpack128(x)
			g(showKind(k), strconv.FormatBool(neg), joinUint128(c.Hi, c.Lo).String(), strconv.Itoa(e))
		}
	case "Pack64":
		neg, coeff, exp := a.boolean(), a.uint64(), a.int()
		want = []string{w.wantDec64(), w.wantBool()}
		run = func() { x, ok := ops.Pack64(neg, coeff, exp); g(show64(x), strconv.FormatBool(ok)) }
	case "Pack128":
		neg, coeff, exp := a.boolean(), a.uint128(), a.int()
		want = []string{w.wantDec128(), w.wantBool()}
		run = func() { x, ok := ops.Pack128(neg, coeff, exp); g(show128(x), strconv.FormatBool(ok)) }
	case "IsNaN64", "IsSignaling64", "IsInf64", "IsFinite64", "IsZero64", "IsSubnormal64", "IsCanonical64", "Signbit64":
		fn := map[string]func(x uint64) bool{
			"IsNaN64": ops.IsNaN64, "IsSignaling64": ops.IsSignaling64, "IsInf64": ops.IsInf64, "IsFinite64": ops.IsFinite64,
			"IsZero64": ops.IsZero64, "IsSubnormal64": ops.IsSubnormal64, "IsCanonical64": ops.IsCanonical64, "Signbit64": ops.Signbit64,
		}[v.op]
		x := a.dec64()
		want = []string{w.wantBool()}
		run = func() { g(strconv.FormatBool(fn(x))) }
	case "IsNaN128", "IsSignaling128", "IsInf128", "IsFinite128", "IsZero128", "IsSubnormal128", "IsCanonical128", "Signbit128":
		fn := map[string]func(x bid.Uint128) bool{
			"IsNaN128": ops.IsNaN128, "IsSignaling128": ops.IsSignaling128, "IsInf128": ops.IsInf128, "IsFinite128": ops.IsFinite128,
			"IsZero128": ops.IsZero128, "IsSubnormal128": ops.IsSubnormal128, "IsCanonical128": ops.IsCanonical128, "Signbit128": ops.Signbit128,
		}[v.op]
		x := a.dec128()
		want = []string{w.wantBool()}
		run = func() { g(strconv.FormatBool(fn(x))) }
	default:
		return nil, nil, false, fmt.Errorf("unknown operation %q", v.op)
	}
	a.done()
	w.done()
	if a.err != nil {
		return nil, nil, false, fmt.Errorf("operands: %v", a.err)
	}
	if w.err != nil {
		return nil, nil, false, fmt.Errorf("expected result: %v", w.err)
	}
	skipped, perr := catchNotImplemented(run)
	if perr != nil {
		return nil, want, false, fmt.Errorf("panic: %v", perr)
	}
	return got, want, skipped, nil
}

// catchNotImplemented runs fn, reporting whether it panicked with the
// kernel's not-implemented message; any other panic is returned.
func catchNotImplemented(fn func()) (skipped bool, perr any) {
	defer func() {
		if r := recover(); r != nil {
			if s, ok := r.(string); ok && s == notImplementedPanic {
				skipped = true
				return
			}
			perr = r
		}
	}()
	fn()
	return false, nil
}

// splitVectorLine splits a line into tokens on spaces, keeping a Go quoted
// string as one token.
func splitVectorLine(line string) ([]string, error) {
	var toks []string
	for i := 0; i < len(line); {
		c := line[i]
		if c == ' ' || c == '\t' {
			i++
			continue
		}
		if c == '"' {
			j := i + 1
			for ; j < len(line); j++ {
				if line[j] == '\\' {
					j++
					continue
				}
				if line[j] == '"' {
					break
				}
			}
			if j >= len(line) {
				return nil, fmt.Errorf("unterminated string")
			}
			toks = append(toks, line[i:j+1])
			i = j + 1
			continue
		}
		j := i
		for j < len(line) && line[j] != ' ' && line[j] != '\t' {
			j++
		}
		toks = append(toks, line[i:j])
		i = j
	}
	return toks, nil
}

// loadConformance reads every .txt file in testdata and returns its
// vectors. Blank lines and lines starting with # are ignored.
func loadConformance(t *testing.T) []vector {
	files, err := filepath.Glob(filepath.Join("testdata", "*.txt"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no conformance files: %v", err)
	}
	ops := make(map[string]bool, len(conformanceOpNames))
	for _, name := range conformanceOpNames {
		ops[name] = true
	}
	classes := make(map[string]bool, len(conformanceClasses))
	for _, name := range conformanceClasses {
		classes[name] = true
	}
	var vectors []vector
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		for n := 1; sc.Scan(); n++ {
			pos := fmt.Sprintf("%s:%d", file, n)
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			toks, err := splitVectorLine(line)
			if err != nil {
				t.Errorf("%s: %v", pos, err)
				continue
			}
			if len(toks) < 4 {
				t.Errorf("%s: a vector is op mode class operands... -> results...", pos)
				continue
			}
			v := vector{pos: pos, op: toks[0], class: toks[2]}
			if !ops[v.op] {
				t.Errorf("%s: unknown operation %q", pos, v.op)
				continue
			}
			if !classes[v.class] {
				t.Errorf("%s: unknown boundary class %q", pos, v.class)
				continue
			}
			if m, ok := conformanceModes[toks[1]]; ok {
				v.mode = m
			} else if toks[1] != "-" {
				n, err := strconv.ParseUint(toks[1], 10, 8)
				if err != nil {
					t.Errorf("%s: unknown mode %q", pos, toks[1])
					continue
				}
				v.mode = bid.Mode(n)
			}
			arrow := -1
			for i, tok := range toks {
				if tok == "->" {
					arrow = i
				}
			}
			if arrow < 3 {
				t.Errorf("%s: no -> between operands and results", pos)
				continue
			}
			v.args = toks[3:arrow]
			v.want = toks[arrow+1:]
			vectors = append(vectors, v)
		}
		if err := sc.Err(); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	return vectors
}

// conformanceRun configures runConformance. The zero value is the run
// against the kernel: a vector whose operation is not implemented is
// skipped and counted. In a strict run nothing may be skipped for that
// reason, because the implementation has no operation it does not
// implement; a not-implemented operation is a failure. Known names vectors
// (by position, file:line) that are expected to fail and the reason; each
// is reported and counted as a skip, and one that passes fails the run, so
// a stale entry cannot linger.
type conformanceRun struct {
	strict bool
	known  map[string]string
}

// runConformance runs every vector against ops. Vectors whose operation
// is not implemented are skipped and the skips are reported per operation,
// unless the run is strict; every other vector must match exactly.
func runConformance(t *testing.T, vectors []vector, ops *conformanceOps, run conformanceRun) {
	skipped := make(map[string]int)
	ran := make(map[string]int)
	known := make(map[string]int)
	seen := make(map[string]bool)
	failures := 0
	for i := range vectors {
		v := &vectors[i]
		got, want, skip, err := ops.eval(v)
		if err != nil {
			t.Errorf("%s: %s %v -> %v: %v", v.pos, v.op, v.args, v.want, err)
			failures++
			continue
		}
		if skip {
			if run.strict {
				t.Errorf("%s: %s is not implemented", v.pos, v.op)
				failures++
				continue
			}
			skipped[v.op]++
			continue
		}
		ran[v.op]++
		mismatch := strings.Join(got, "; ") != strings.Join(want, "; ")
		if reason, ok := run.known[v.pos]; ok {
			seen[v.pos] = true
			if !mismatch {
				t.Errorf("%s: %s(%s) passes but is listed as a known failure (%s)", v.pos, v.op, strings.Join(v.args, ", "), reason)
				failures++
				continue
			}
			known[reason]++
			continue
		}
		if mismatch {
			t.Errorf("%s: %s(%s) [%s]\n\t got %s\n\twant %s", v.pos, v.op, strings.Join(v.args, ", "), v.class,
				strings.Join(got, "; "), strings.Join(want, "; "))
			failures++
			if failures > 200 {
				t.Fatalf("too many failures; stopping")
			}
		}
	}
	for pos, reason := range run.known {
		if !seen[pos] {
			t.Errorf("known failure %s (%s) names no vector", pos, reason)
		}
	}
	var skippedOps, ranOps []string
	n, s := 0, 0
	for _, op := range conformanceOpNames {
		if skipped[op] > 0 {
			skippedOps = append(skippedOps, fmt.Sprintf("%s(%d)", op, skipped[op]))
			s += skipped[op]
		}
		if ran[op] > 0 {
			ranOps = append(ranOps, fmt.Sprintf("%s(%d)", op, ran[op]))
			n += ran[op]
		}
	}
	t.Logf("%d vectors; %d ran: %s", len(vectors), n, strings.Join(ranOps, " "))
	if s > 0 {
		t.Logf("%d skipped, not implemented: %s", s, strings.Join(skippedOps, " "))
	}
	for reason, c := range known {
		t.Logf("%d known failures skipped: %s", c, reason)
	}
}

func TestConformance(t *testing.T) {
	vectors := loadConformance(t)
	if t.Failed() {
		t.FailNow()
	}
	runConformance(t, vectors, &kernelOps, conformanceRun{})
}

// TestConformanceModel checks the suite's own reader and writer against
// the layout facts of IEEE 754-2008 clause 3.5.2 and against the character
// forms tabulated in the package documentation, so that a failure of the
// suite is a disagreement with the kernel and not a bug in the harness.
func TestConformanceModel(t *testing.T) {
	cases := []struct {
		text string
		bits uint64
	}{
		{"0", 0x31C0000000000000},
		{"1", 0x31C0000000000001},
		{"-0", 0xB1C0000000000000},
		{"1.50", 0x3180000000000096},
		{"1.5E+3", 0x3200000000000003 | 15},
		{"0E-398", 0},
		{"1E-398", 1},
		{"9.999999999999999E+384", 0x77FB86F26FC0FFFF},
		{"9007199254740992", 0x6C70000000000000},
		{"Infinity", 0x7800000000000000},
		{"-Infinity", 0xF800000000000000},
		{"NaN", 0x7C00000000000000},
		{"-NaN", 0xFC00000000000000},
		{"sNaN", 0x7E00000000000000},
		{"-sNaN", 0xFE00000000000000},
	}
	for _, c := range cases {
		d, err := parseDecimal(c.text)
		if err != nil {
			t.Errorf("parse %q: %v", c.text, err)
			continue
		}
		bits, err := encode64(d)
		if err != nil {
			t.Errorf("encode %q: %v", c.text, err)
			continue
		}
		if bits != c.bits {
			t.Errorf("encode64(%q) = 0x%016x, want 0x%016x", c.text, bits, c.bits)
		}
		if back := formatDecimal(decode64(c.bits)); back != c.text {
			t.Errorf("format(decode64(0x%016x)) = %q, want %q", c.bits, back, c.text)
		}
	}
	// Non-canonical: a second-form coefficient of 2^53 + 2^51 - 1, above
	// 10^16 - 1, reads as zero with the encoded exponent and sign. (Every
	// first-form decimal64 coefficient is below 2^53 < 10^16 and so
	// canonical.)
	if got := formatDecimal(decode64(0x6C70000000000000 | 1<<51 - 1)); got != "0" {
		t.Errorf("non-canonical second form reads as %q, want 0", got)
	}
	if got := formatDecimal(decode64(0xEC70000000000000 | 1<<51 - 1)); got != "-0" {
		t.Errorf("negative non-canonical second form reads as %q, want -0", got)
	}
	if got := formatDecimal(decode64(0x6C40000000000000 | 1<<51 - 1)); got != "0.000000" {
		t.Errorf("non-canonical second form with exponent -6 reads as %q, want 0.000000", got)
	}
	// Character form table of the package documentation.
	forms := []struct {
		coeff, exp int64
		text       string
	}{
		{150, -2, "1.50"}, {15, 2, "1.5E+3"}, {123, 0, "123"}, {123, 1, "1.23E+3"},
		{1, -6, "0.000001"}, {10, 369, "1.0E+370"}, {0, 0, "0"}, {0, -2, "0.00"},
		{0, 1, "0E+1"}, {0, -398, "0E-398"}, {1, -7, "1E-7"}, {12345, -7, "0.0012345"},
	}
	for _, f := range forms {
		d := decimal{coeff: big.NewInt(f.coeff), exp: int(f.exp)}
		if got := formatDecimal(d); got != f.text {
			t.Errorf("format(%d, %d) = %q, want %q", f.coeff, f.exp, got, f.text)
		}
	}
	// decimal128 layout: 1E+6111 and the largest finite value.
	d, _ := parseDecimal("9.999999999999999999999999999999999E+6144")
	x, err := encode128(d)
	if err != nil {
		t.Fatal(err)
	}
	if want := (bid.Uint128{Hi: 0x5FFFED09BEAD87C0, Lo: 0x378D8E63FFFFFFFF}); x != want {
		t.Errorf("encode128(max) = %016x%016x, want %016x%016x", x.Hi, x.Lo, want.Hi, want.Lo)
	}
	if back := formatDecimal(decode128(x)); back != "9.999999999999999999999999999999999E+6144" {
		t.Errorf("decode128(max) = %q", back)
	}
	if back := formatDecimal(decode128(bid.Uint128{Hi: 0x3040000000000000, Lo: 0})); back != "0" {
		t.Errorf("decode128(+0) = %q, want 0", back)
	}
}

// TestConformanceREADME checks that the vector-count table in
// testdata/README matches the suite, so that the table cannot drift. A
// cell holding "-" declares that the class does not apply to the
// operation and must have no vectors; every other cell holds its count
// and must be positive.
func TestConformanceREADME(t *testing.T) {
	vectors := loadConformance(t)
	if t.Failed() {
		t.FailNow()
	}
	counts := make(map[string]map[string]int)
	for _, v := range vectors {
		if counts[v.op] == nil {
			counts[v.op] = make(map[string]int)
		}
		counts[v.op][v.class]++
	}

	readme, err := os.ReadFile(filepath.Join("testdata", "README"))
	if err != nil {
		t.Fatal(err)
	}
	const begin, end = "<!-- counts:begin -->", "<!-- counts:end -->"
	text := string(readme)
	i, j := strings.Index(text, begin), strings.Index(text, end)
	if i < 0 || j < i {
		t.Fatalf("README has no %s ... %s table", begin, end)
	}
	table := strings.TrimSpace(text[i+len(begin) : j])

	// Render the table the suite implies, taking the "-" cells from the
	// README so that only a wrong count or a wrongly declared cell differs.
	declared := make(map[string]map[string]bool)
	for _, line := range strings.Split(table, "\n") {
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) != len(conformanceClasses)+2 {
			continue
		}
		op := strings.TrimSpace(cells[0])
		declared[op] = make(map[string]bool)
		for k, class := range conformanceClasses {
			if strings.TrimSpace(cells[k+1]) == "-" {
				declared[op][class] = true
			}
		}
	}
	var b strings.Builder
	b.WriteString("| Operation |")
	for _, class := range conformanceClasses {
		b.WriteString(" " + class + " |")
	}
	b.WriteString(" Total |\n|---|")
	for range conformanceClasses {
		b.WriteString("---:|")
	}
	b.WriteString("---:|\n")
	classTotals := make(map[string]int)
	total := 0
	for _, op := range conformanceOpNames {
		b.WriteString("| " + op + " |")
		rowTotal := 0
		for _, class := range conformanceClasses {
			n := counts[op][class]
			switch {
			case n == 0 && declared[op][class]:
				b.WriteString(" - |")
			case n == 0:
				t.Errorf("%s has no %s vector and the README does not declare the class inapplicable", op, class)
				b.WriteString(" 0 |")
			case declared[op][class]:
				t.Errorf("README declares %s/%s inapplicable but the suite has %d vector(s)", op, class, n)
				fallthrough
			default:
				fmt.Fprintf(&b, " %d |", n)
			}
			rowTotal += n
			classTotals[class] += n
		}
		fmt.Fprintf(&b, " %d |\n", rowTotal)
		total += rowTotal
	}
	b.WriteString("| Total |")
	for _, class := range conformanceClasses {
		fmt.Fprintf(&b, " %d |", classTotals[class])
	}
	fmt.Fprintf(&b, " %d |", total)
	if want := b.String(); table != want {
		t.Errorf("README vector table does not match the suite; it should read:\n%s", want)
	}
}
