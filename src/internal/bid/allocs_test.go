// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// The allocation lock of the kernel: every exported function of the
// package, at both widths, makes no allocation on any path (target T1.6.4).
//
// TestAllocs holds one entry per exported function, keyed by its name. Each
// entry is a closure that calls the function over a set of operands chosen
// to reach the slow paths as well as the common ones, in every rounding
// mode (and in a mode outside the five, which behaves as ToNearestEven).
// The operand sets are in allocOperands and the notes there say which slow
// path each member is for.
//
// Completeness is enforced by parsing the package source: exportedAPI lists
// every exported function and method declared in a non-test file of the
// package (skipping the //go:build ignore templates and generator), and
// TestAllocs fails for an exported name with no entry and for an entry
// with no exported name. A new exported function therefore fails the test
// until it is given an entry here. The benchmark table in bench_test.go is
// checked against the same list by TestBenchCoversAPI.
//
// An entry's closure is measured whole: it loops over the operands, so one
// run of it is thousands of calls, and a path taken by one operand in one
// mode allocating once per run is reported. testing.AllocsPerRun returns
// the average rounded down, so a path that allocated on some runs only
// would be hidden; no path of this package depends on state that varies
// between runs, so the closure takes the same paths in every run.
//
// The test also holds the contract of the two functions that take a buffer:
// Append64 and Append128 do not allocate when dst has MaxLen64 or MaxLen128
// bytes of spare capacity, and are measured that way.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"internal/bid"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Sinks. The closures store every result in a package variable so that no
// call can be removed as dead.
var (
	asU64  uint64
	asU128 bid.Uint128
	asF64  float64
	asF32  float32
	asInt  int
	asI64  int64
	asSt   bid.Status
	asKey  bid.Key
	asBool bool
	asKind bid.Kind
	asBuf  []byte
)

// allocModes are the five rounding modes and one outside them.
var allocModes = append([]bid.Mode{7}, allModes...)

// exportedAPI returns the sorted names of the exported functions and the
// exported methods of exported types declared in the package's source files
// that are part of the build, methods as "Type.Method".
func exportedAPI(t testing.TB) []string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if ignoredByBuildTag(string(src)) {
			continue
		}
		file, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() {
				continue
			}
			if fn.Recv == nil {
				names = append(names, fn.Name.Name)
				continue
			}
			typ := fn.Recv.List[0].Type
			if star, ok := typ.(*ast.StarExpr); ok {
				typ = star.X
			}
			if id, ok := typ.(*ast.Ident); ok && id.IsExported() {
				names = append(names, id.Name+"."+fn.Name.Name)
			}
		}
	}
	sort.Strings(names)
	return names
}

// ignoredByBuildTag reports whether src has a "//go:build ignore" line
// before its package clause: the generator and the templates.
func ignoredByBuildTag(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "//go:build ignore" {
			return true
		}
		if strings.HasPrefix(line, "package ") {
			return false
		}
	}
	return false
}

// allocOpnd is a finite operand: (-1)^neg × coeff × 10^exp.
type allocOpnd struct {
	neg   bool
	coeff string
	exp   int
}

// allocOperands64 returns the decimal64 operand set and its core. The core
// is the subset that three-operand and per-mode loops run over.
//
// The members, and the paths they reach: zeros of every exponent (the zero
// rules of sign and quantum); exact small values (the no-rounding fast
// paths); full and over-full coefficients (the rounding paths); the gap
// pairs 1E+300 with -1.5E-300 and 1E+200 with 1E-398 (the sticky-bit
// alignment of Add and the large-gap Rem); the extremes of the exponent
// range (overflow, the subnormal range, clamping); 2^53+1 (the tie of
// Float64From); the values of the original per-function tests; the
// infinities, quiet and signaling NaNs, a NaN with a payload and a
// non-canonical encoding (the special-value exits).
func allocOperands64() (all, core []uint64) {
	fin := func(l []allocOpnd) (r []uint64) {
		for _, o := range l {
			r = append(r, decimalOf(dec64, o.neg, o.coeff, o.exp).Lo)
		}
		return r
	}
	const nonCanonical uint64 = bid.Form2Mask64 | bid.Bias64<<bid.Form2ExpShift64 | (1<<bid.Form2CoeffBits64 - 1)
	specials := []uint64{
		bid.Inf64, bid.Inf64 | bid.SignMask64, bid.NaN64, bid.SNaN64, bid.NaN64 | bid.SignMask64,
		bid.NaN64 | 0x3FFFFFFFFFFFF, bid.SNaN64 | 12345, nonCanonical,
	}
	coreFin := fin([]allocOpnd{
		{false, "0", 0},
		{true, "0", dec64.maxExp},
		{false, "125", -2},
		{true, "15", -1},
		{true, "1234567890123456", -3},
		{false, "9999999999999999", dec64.maxExp},
		{true, "9999999999999997", dec64.minExp},
		{false, "1", 300},
		{true, "15", -301},
		{false, "1", 200},
		{false, "5", -390},
		{false, "1", 100},
	})
	core = append(core, coreFin...)
	core = append(core, bid.Inf64, bid.NaN64, nonCanonical)
	rest := fin([]allocOpnd{
		{false, "0", dec64.minExp},
		{true, "0", 0},
		{false, "1", 0},
		{false, "1", -1},
		{false, "215", -2},
		{false, "12", 3},
		{false, "1200", 1},
		{false, "150", -2},
		{true, "2", 0},
		{false, "800", -2},
		{false, "70000000000000", 0},
		{false, "1234567890123456", 0},
		{false, "7777777777777777", 0},
		{false, "9007199254740993", 0},
		{false, "9999999999999999", -320},
		{true, "9999999999999999", 293},
		{false, "1", dec64.minExp},
		{false, "0", 369},
		{false, "1234567890123456", 369},
	})
	all = append(all, coreFin...)
	all = append(all, rest...)
	// Raw encodings: a multiplication pair, the zero of exponent 0, the
	// largest-coefficient form of exponent 0, and their negatives.
	all = append(all, 0x31A0000000000123, 0x3180000000000456, 0, bid.Zero64, 0x6c7386f26fc0ffff, 0xfc00000000000000, 1<<63|0x6c7386f26fc0ffff)
	all = append(all, specials...)
	return all, core
}

// bigLong returns (2^53+1) × 5^25, the coefficient of the Float64From tie.
func bigLong() *big.Int {
	return new(big.Int).Mul(big.NewInt(1<<53+1), new(big.Int).Exp(big.NewInt(5), big.NewInt(25), nil))
}

// allocOperands128 is allocOperands64 for decimal128.
func allocOperands128() (all, core []bid.Uint128) {
	fin := func(l []allocOpnd) (r []bid.Uint128) {
		for _, o := range l {
			r = append(r, decimalOf(dec128, o.neg, o.coeff, o.exp))
		}
		return r
	}
	nonCanonical := bid.Uint128{
		Hi: bid.Form2Mask128Hi | bid.Bias128<<bid.Form2ExpShift128 | (1<<(bid.Form2CoeffBits128-64) - 1),
		Lo: math.MaxUint64,
	}
	specials := []bid.Uint128{
		{Hi: bid.Inf128Hi}, {Hi: bid.Inf128Hi | bid.SignMask128Hi}, {Hi: bid.NaN128Hi}, {Hi: bid.SNaN128Hi},
		{Hi: bid.NaN128Hi | bid.SignMask128Hi}, {Hi: bid.NaN128Hi, Lo: 99}, {Hi: bid.SNaN128Hi | 1<<63, Lo: 99},
		nonCanonical,
	}
	max34 := strings.Repeat("9", 34)
	long34 := "1234567890123456789012345678901234"
	coreFin := fin([]allocOpnd{
		{false, "0", 0},
		{true, "0", dec128.maxExp},
		{false, "125", -2},
		{true, "15", -1},
		{true, long34, -3},
		{false, max34, dec128.maxExp},
		{true, "9999999999999999999999999999999997", dec128.minExp},
		{false, "1", 3000},
		{true, "15", -3001},
		{false, "1", 100},
		{false, "5", -6170},
	})
	core = append(core, coreFin...)
	core = append(core, bid.Uint128{Hi: bid.Inf128Hi}, bid.Uint128{Hi: bid.NaN128Hi}, nonCanonical)
	// 1769609755703054503235303742165 is a 31-digit coefficient; (2^53+1)×5^25
	// is the operand of the Float64From tie.
	long := bigLong().String()
	rest := fin([]allocOpnd{
		{false, "0", dec128.minExp},
		{true, "0", 0},
		{false, "1", 0},
		{false, "1", -1},
		{false, "10", -1},
		{false, "100", -2},
		{false, "12", 3},
		{false, "1200", 1},
		{false, "150", -2},
		{true, "2", 0},
		{true, "2500", -3},
		{false, "12345678901234567890", 0},
		{false, long34, 0},
		{false, "7777777777777777777777777777777777", 0},
		{false, "1769609755703054503235303742165", 167},
		{true, "1769609755703054503235303742165", -6000},
		{false, "1769609755703054503235303742165", 6000},
		{false, "92233720368547758075", -1},
		{false, long, -25},
		{true, long, -25},
		{false, max34, -360},
		{false, max34, 274},
		{true, max34, 6000},
		{false, "1", dec128.minExp},
		{false, "1", 36},
		{false, "0", 1000},
		{false, "0", -6000},
		{false, "1", 2},
		{false, new(big.Int).Add(bigLong(), big.NewInt(1)).String(), -25},
		{false, max34, 0},
	})
	all = append(all, coreFin...)
	all = append(all, rest...)
	// Raw encodings: a multiplication pair, the zero of exponent 0, the
	// largest-coefficient form of exponent 0, and their negatives.
	all = append(all,
		bid.Uint128{Hi: 0x3040000000000123, Lo: 77}, bid.Uint128{Hi: 0x3040000000000456, Lo: 99},
		bid.Uint128{}, bid.Uint128{Hi: bid.Zero128Hi},
		bid.Uint128{Hi: 0x3041ed09bead87c0, Lo: 0x378d8e63ffffffff}, bid.Uint128{Hi: 1<<63 | 0x3041ed09bead87c0, Lo: 0x378d8e63ffffffff})
	all = append(all, specials...)
	return all, core
}

// Generic loop builders. Each returns a closure that calls f over every
// operand, pair or triple, in every mode.

func each[T any](xs []T, f func(x T)) func() {
	return func() {
		for _, x := range xs {
			f(x)
		}
	}
}

func eachMode[T any](xs []T, f func(x T, m bid.Mode)) func() {
	return func() {
		for _, x := range xs {
			for _, m := range allocModes {
				f(x, m)
			}
		}
	}
}

func pairs[T any](xs []T, f func(x, y T)) func() {
	return func() {
		for _, x := range xs {
			for _, y := range xs {
				f(x, y)
			}
		}
	}
}

func pairsMode[T any](xs []T, f func(x, y T, m bid.Mode)) func() {
	return func() {
		for _, x := range xs {
			for _, y := range xs {
				for _, m := range allocModes {
					f(x, y, m)
				}
			}
		}
	}
}

func triplesMode[T any](xs []T, f func(x, y, z T, m bid.Mode)) func() {
	return func() {
		for _, x := range xs {
			for _, y := range xs {
				for _, z := range xs {
					for _, m := range allocModes {
						f(x, y, z, m)
					}
				}
			}
		}
	}
}

// allocCases returns the entry of every exported function.
func allocCases() map[string]func() {
	all64, core64 := allocOperands64()
	all128, core128 := allocOperands128()
	buf := make([]byte, 0, bid.MaxLen128)

	// Float operands. The largest and the smallest subnormal float64 are the
	// multiword paths of Float64To; the exactly representable ties and the
	// 1e23 neighbourhood are its rounding paths.
	floats := []float64{
		0, math.Copysign(0, -1), 1, -1, 0.5, 0x1p-24, 0.1, 123.456, 123456789, 1e22, 1e23,
		math.MaxFloat64, -math.MaxFloat64, 5e-324, 0x1p-1022, math.Ldexp(1, 500), math.Ldexp(1, -500),
		1 << 53, 1<<53 + 2, math.Inf(1), math.Inf(-1), math.NaN(),
	}
	floats32 := []float32{0, 1, -1, 0.1, 0.5, 1e-45, math.MaxFloat32, 1e30, 1e-30, float32(math.Inf(1)), float32(math.NaN()), math.Float32frombits(0x7F800001)}
	for _, v := range floats {
		floats32 = append(floats32, float32(v))
	}
	ints := []int64{math.MinInt64, math.MinInt64 + 1, -1234567890123456789, -1, 0, 1, 9007199254740993, math.MaxInt64}
	uints := []uint64{0, 1, 9007199254740993, 1 << 63, math.MaxUint64}

	// Round operands: coefficients below, at and above the format's
	// precision, and exponents that overflow, underflow and clamp.
	roundCoeffs64 := []uint64{0, 1, 5, 1234567890123456, 9999999999999999, 12345678901234567, math.MaxUint64}
	roundCoeffs128 := []bid.Uint128{
		{}, {Lo: 1}, {Lo: 5}, {Lo: 12345678901234567}, {Hi: 1, Lo: 1}, {Hi: 0x0001ED09BEAD87C0, Lo: 0x378D8E63FFFFFFFF},
		{Hi: 0x0001ED09BEAD87C0 + 1}, {Hi: math.MaxUint64, Lo: math.MaxUint64},
	}
	roundExps := []int{math.MinInt, -7000, -6177, -6176, -410, -400, -398, -5, 0, 5, 369, 400, 6111, 6112, 7000, math.MaxInt}

	strs := []string{
		"0", "-0", "1.50", "-2", "123.456", "0.1", ".5", "5.", "1e5", "1E-5", "0E-5", "0E+99999999999999999999",
		"NaN", "sNaN", "-nan", "inf", "-Infinity", "9999999999999999", "99999999999999995", "12345678901234567890123456789012345678901",
		"1E+99999999999999999999", "1E-99999999999999999999", "-0." + strings.Repeat("0", 500) + "E+99999999999999999999",
		"0." + strings.Repeat("0", 3000) + "123456789012345678901234567890123456789E+99999999999999999999",
		strings.Repeat("9", 5000), "1" + strings.Repeat("0", 5000) + "." + strings.Repeat("0", 5000) + "1",
		"", "-", "abc", "1e", "1e+", "1_0", "0x10", "1.2.3", " 1", "1 ", "nan1", "infinit",
	}
	keyOf := func(k bid.Key) { asKey = k; asKind = k.Kind }

	return map[string]func(){
		// arith.go: arithmetic.
		"Add64":  pairsMode(all64, func(x, y uint64, m bid.Mode) { asU64 ^= bid.Add64(x, y, m) }),
		"Add128": pairsMode(all128, func(x, y bid.Uint128, m bid.Mode) { asU128 = bid.Add128(x, y, m) }),
		"Sub64":  pairsMode(all64, func(x, y uint64, m bid.Mode) { asU64 ^= bid.Sub64(x, y, m) }),
		"Sub128": pairsMode(all128, func(x, y bid.Uint128, m bid.Mode) { asU128 = bid.Sub128(x, y, m) }),
		"Mul64":  pairsMode(all64, func(x, y uint64, m bid.Mode) { asU64 ^= bid.Mul64(x, y, m) }),
		"Mul128": pairsMode(all128, func(x, y bid.Uint128, m bid.Mode) { asU128 = bid.Mul128(x, y, m) }),
		"Div64":  pairsMode(all64, func(x, y uint64, m bid.Mode) { asU64 ^= bid.Div64(x, y, m) }),
		"Div128": pairsMode(all128, func(x, y bid.Uint128, m bid.Mode) { asU128 = bid.Div128(x, y, m) }),
		"FMA64":  triplesMode(core64, func(x, y, z uint64, m bid.Mode) { asU64 ^= bid.FMA64(x, y, z, m) }),
		"FMA128": triplesMode(core128, func(x, y, z bid.Uint128, m bid.Mode) { asU128 = bid.FMA128(x, y, z, m) }),
		"Rem64":  pairs(all64, func(x, y uint64) { asU64 ^= bid.Rem64(x, y) }),
		"Rem128": pairs(all128, func(x, y bid.Uint128) { asU128 = bid.Rem128(x, y) }),
		"Mod64":  pairs(all64, func(x, y uint64) { asU64 ^= bid.Mod64(x, y) }),
		"Mod128": pairs(all128, func(x, y bid.Uint128) { asU128 = bid.Mod128(x, y) }),

		"RoundToIntegral64":  eachMode(all64, func(x uint64, m bid.Mode) { asU64 ^= bid.RoundToIntegral64(x, m) }),
		"RoundToIntegral128": eachMode(all128, func(x bid.Uint128, m bid.Mode) { asU128 = bid.RoundToIntegral128(x, m) }),
		"Quantize64":         pairsMode(all64, func(x, y uint64, m bid.Mode) { asU64 ^= bid.Quantize64(x, y, m) }),
		"Quantize128":        pairsMode(all128, func(x, y bid.Uint128, m bid.Mode) { asU128 = bid.Quantize128(x, y, m) }),
		"Reduce64":           each(all64, func(x uint64) { asU64 ^= bid.Reduce64(x) }),
		"Reduce128":          each(all128, func(x bid.Uint128) { asU128 = bid.Reduce128(x) }),
		"SameQuantum64":      pairs(all64, func(x, y uint64) { asBool = bid.SameQuantum64(x, y) }),
		"SameQuantum128":     pairs(all128, func(x, y bid.Uint128) { asBool = bid.SameQuantum128(x, y) }),
		"Min64":              pairs(all64, func(x, y uint64) { asU64 ^= bid.Min64(x, y) }),
		"Min128":             pairs(all128, func(x, y bid.Uint128) { asU128 = bid.Min128(x, y) }),
		"Max64":              pairs(all64, func(x, y uint64) { asU64 ^= bid.Max64(x, y) }),
		"Max128":             pairs(all128, func(x, y bid.Uint128) { asU128 = bid.Max128(x, y) }),

		// round.go.
		"Round64": func() {
			for _, c := range roundCoeffs64 {
				for _, e := range roundExps {
					for _, sticky := range []bool{false, true} {
						for _, m := range allocModes {
							x, st := bid.Round64(c&1 != 0, c, e, sticky, m)
							asU64, asSt = x, asSt|st
							x, st = bid.Round64(c&1 == 0, c, e, sticky, m)
							asU64, asSt = x, asSt|st
						}
					}
				}
			}
		},
		"Round128": func() {
			for _, c := range roundCoeffs128 {
				for _, e := range roundExps {
					for _, sticky := range []bool{false, true} {
						for _, m := range allocModes {
							x, st := bid.Round128(c.Lo&1 != 0, c, e, sticky, m)
							asU128, asSt = x, asSt|st
							x, st = bid.Round128(c.Lo&1 == 0, c, e, sticky, m)
							asU128, asSt = x, asSt|st
						}
					}
				}
			}
		},

		// encoding.go.
		"Unpack64": each(all64, func(x uint64) {
			k, neg, c, e := bid.Unpack64(x)
			asKind, asBool, asU64, asInt = k, neg, c, e
		}),
		"Unpack128": each(all128, func(x bid.Uint128) {
			k, neg, c, e := bid.Unpack128(x)
			asKind, asBool, asU128, asInt = k, neg, c, e
		}),
		"Pack64": func() {
			for _, c := range roundCoeffs64 {
				for _, e := range roundExps {
					x, ok := bid.Pack64(c&1 != 0, c, e)
					asU64, asBool = x, ok
				}
			}
		},
		"Pack128": func() {
			for _, c := range roundCoeffs128 {
				for _, e := range roundExps {
					x, ok := bid.Pack128(c.Lo&1 != 0, c, e)
					asU128, asBool = x, ok
				}
			}
		},
		"IsNaN64":        each(all64, func(x uint64) { asBool = bid.IsNaN64(x) }),
		"IsNaN128":       each(all128, func(x bid.Uint128) { asBool = bid.IsNaN128(x) }),
		"IsSignaling64":  each(all64, func(x uint64) { asBool = bid.IsSignaling64(x) }),
		"IsSignaling128": each(all128, func(x bid.Uint128) { asBool = bid.IsSignaling128(x) }),
		"IsInf64":        each(all64, func(x uint64) { asBool = bid.IsInf64(x) }),
		"IsInf128":       each(all128, func(x bid.Uint128) { asBool = bid.IsInf128(x) }),
		"IsFinite64":     each(all64, func(x uint64) { asBool = bid.IsFinite64(x) }),
		"IsFinite128":    each(all128, func(x bid.Uint128) { asBool = bid.IsFinite128(x) }),
		"IsZero64":       each(all64, func(x uint64) { asBool = bid.IsZero64(x) }),
		"IsZero128":      each(all128, func(x bid.Uint128) { asBool = bid.IsZero128(x) }),
		"IsSubnormal64":  each(all64, func(x uint64) { asBool = bid.IsSubnormal64(x) }),
		"IsSubnormal128": each(all128, func(x bid.Uint128) { asBool = bid.IsSubnormal128(x) }),
		"IsCanonical64":  each(all64, func(x uint64) { asBool = bid.IsCanonical64(x) }),
		"IsCanonical128": each(all128, func(x bid.Uint128) { asBool = bid.IsCanonical128(x) }),
		"Signbit64":      each(all64, func(x uint64) { asBool = bid.Signbit64(x) }),
		"Signbit128":     each(all128, func(x bid.Uint128) { asBool = bid.Signbit128(x) }),
		"Neg64":          each(all64, func(x uint64) { asU64 ^= bid.Neg64(x) }),
		"Neg128":         each(all128, func(x bid.Uint128) { asU128 = bid.Neg128(x) }),
		"Abs64":          each(all64, func(x uint64) { asU64 ^= bid.Abs64(x) }),
		"Abs128":         each(all128, func(x bid.Uint128) { asU128 = bid.Abs128(x) }),
		"CopySign64":     pairs(all64, func(x, y uint64) { asU64 ^= bid.CopySign64(x, y) }),
		"CopySign128":    pairs(all128, func(x, y bid.Uint128) { asU128 = bid.CopySign128(x, y) }),

		// compare.go.
		"Equal64":       pairs(all64, func(x, y uint64) { asBool = bid.Equal64(x, y) }),
		"Equal128":      pairs(all128, func(x, y bid.Uint128) { asBool = bid.Equal128(x, y) }),
		"Less64":        pairs(all64, func(x, y uint64) { asBool = bid.Less64(x, y) }),
		"Less128":       pairs(all128, func(x, y bid.Uint128) { asBool = bid.Less128(x, y) }),
		"LessEqual64":   pairs(all64, func(x, y uint64) { asBool = bid.LessEqual64(x, y) }),
		"LessEqual128":  pairs(all128, func(x, y bid.Uint128) { asBool = bid.LessEqual128(x, y) }),
		"Compare64":     pairs(all64, func(x, y uint64) { asInt, asBool = bid.Compare64(x, y) }),
		"Compare128":    pairs(all128, func(x, y bid.Uint128) { asInt, asBool = bid.Compare128(x, y) }),
		"TotalOrder64":  pairs(all64, func(x, y uint64) { asBool = bid.TotalOrder64(x, y) }),
		"TotalOrder128": pairs(all128, func(x, y bid.Uint128) { asBool = bid.TotalOrder128(x, y) }),
		"Key64":         each(all64, func(x uint64) { keyOf(bid.Key64(x)) }),
		"Key128":        each(all128, func(x bid.Uint128) { keyOf(bid.Key128(x)) }),

		// convert.go.
		"Widen":  each(all64, func(x uint64) { asU128 = bid.Widen(x) }),
		"Narrow": eachMode(all128, func(x bid.Uint128, m bid.Mode) { asU64 ^= bid.Narrow(x, m) }),
		"Int64To64": func() {
			for _, v := range ints {
				for _, m := range allocModes {
					asU64 ^= bid.Int64To64(v, m)
				}
			}
		},
		"Int64To128": func() {
			for _, v := range ints {
				asU128 = bid.Int64To128(v)
			}
		},
		"Uint64To64": func() {
			for _, v := range uints {
				for _, m := range allocModes {
					asU64 ^= bid.Uint64To64(v, m)
				}
			}
		},
		"Uint64To128": func() {
			for _, v := range uints {
				asU128 = bid.Uint64To128(v)
			}
		},
		"Int64From64": eachMode(all64, func(x uint64, m bid.Mode) {
			v, st := bid.Int64From64(x, m)
			asI64, asSt = v, asSt|st
		}),
		"Int64From128": eachMode(all128, func(x bid.Uint128, m bid.Mode) {
			v, st := bid.Int64From128(x, m)
			asI64, asSt = v, asSt|st
		}),
		"Uint64From64": eachMode(all64, func(x uint64, m bid.Mode) {
			v, st := bid.Uint64From64(x, m)
			asU64, asSt = v, asSt|st
		}),
		"Uint64From128": eachMode(all128, func(x bid.Uint128, m bid.Mode) {
			v, st := bid.Uint64From128(x, m)
			asU64, asSt = v, asSt|st
		}),

		// float.go.
		"Float64To64":  eachMode(floats, func(v float64, m bid.Mode) { asU64 ^= bid.Float64To64(v, m) }),
		"Float64To128": eachMode(floats, func(v float64, m bid.Mode) { asU128 = bid.Float64To128(v, m) }),
		"Float32To64":  eachMode(floats32, func(v float32, m bid.Mode) { asU64 ^= bid.Float32To64(v, m) }),
		"Float32To128": eachMode(floats32, func(v float32, m bid.Mode) { asU128 = bid.Float32To128(v, m) }),
		// Float64From and Float32From take the exact path in every directed
		// mode and for the ties of ToNearest*; all modes run.
		"Float64From64":  eachMode(all64, func(x uint64, m bid.Mode) { asF64 += bid.Float64From64(x, m) }),
		"Float64From128": eachMode(all128, func(x bid.Uint128, m bid.Mode) { asF64 += bid.Float64From128(x, m) }),
		"Float32From64":  eachMode(all64, func(x uint64, m bid.Mode) { asF32 += bid.Float32From64(x, m) }),
		"Float32From128": eachMode(all128, func(x bid.Uint128, m bid.Mode) { asF32 += bid.Float32From128(x, m) }),

		// string.go. dst has the spare capacity the contract requires.
		"Append64":  each(all64, func(x uint64) { asBuf = bid.Append64(buf[:0], x) }),
		"Append128": each(all128, func(x bid.Uint128) { asBuf = bid.Append128(buf[:0], x) }),
		"Parse64": func() {
			for _, s := range strs {
				for _, m := range allocModes {
					x, st := bid.Parse64(s, m)
					asU64, asSt = x, asSt|st
				}
			}
		},
		"Parse128": func() {
			for _, s := range strs {
				for _, m := range allocModes {
					x, st := bid.Parse128(s, m)
					asU128, asSt = x, asSt|st
				}
			}
		},
	}
}

// TestAllocs asserts zero allocations for every exported function and that
// every exported function has an entry.
func TestAllocs(t *testing.T) {
	cases := allocCases()
	api := exportedAPI(t)
	inAPI := make(map[string]bool, len(api))
	for _, name := range api {
		inAPI[name] = true
		if cases[name] == nil {
			t.Errorf("exported function %s has no entry in allocCases", name)
		}
	}
	var names []string
	for name := range cases {
		if !inAPI[name] {
			t.Errorf("allocCases has an entry %s that is not an exported function of the package", name)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if n := testing.AllocsPerRun(5, cases[name]); n != 0 {
			t.Errorf("%s allocates: %v allocations per run", name, n)
		}
	}
	t.Logf("%d exported functions, each measured on its slow paths", len(api))
	_ = asBool
}
