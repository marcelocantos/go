// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package bid_test

import (
	"fmt"
	"internal/bid"
	"math"
	"math/rand/v2"
	"runtime"
	"testing"
)

// The language's decimal operators, compared with the kernel. On every
// target but linux/ppc64le the operators are runtime calls into the kernel,
// so this checks the compiler's calls and the memory form. On linux/ppc64le
// most of them are POWER's decimal floating-point instructions (see
// cmd/compile/internal/ssagen/dpd.go), and this is the differential test of
// the hardware path: every case of the oracle's generators, run through the
// operator and through the kernel under round-half-even (toward zero for the
// conversions to integers, as the language does), must give the same BID
// encoding. TestOperatorsUseHardware checks that the hardware path is the
// one being run.

//go:noinline
func opAdd64(x, y decimal64) decimal64 { return x + y }

//go:noinline
func opSub64(x, y decimal64) decimal64 { return x - y }

//go:noinline
func opMul64(x, y decimal64) decimal64 { return x * y }

//go:noinline
func opDiv64(x, y decimal64) decimal64 { return x / y }

//go:noinline
func opAdd128(x, y decimal128) decimal128 { return x + y }

//go:noinline
func opSub128(x, y decimal128) decimal128 { return x - y }

//go:noinline
func opMul128(x, y decimal128) decimal128 { return x * y }

//go:noinline
func opDiv128(x, y decimal128) decimal128 { return x / y }

//go:noinline
func opCmp64(x, y decimal64) [6]bool {
	return [6]bool{x == y, x != y, x < y, x <= y, x > y, x >= y}
}

//go:noinline
func opCmp128(x, y decimal128) [6]bool {
	return [6]bool{x == y, x != y, x < y, x <= y, x > y, x >= y}
}

//go:noinline
func opMin64(x, y decimal64) decimal64 { return min(x, y) }

//go:noinline
func opMax64(x, y decimal64) decimal64 { return max(x, y) }

//go:noinline
func opMin128(x, y decimal128) decimal128 { return min(x, y) }

//go:noinline
func opMax128(x, y decimal128) decimal128 { return max(x, y) }

//go:noinline
func opWiden(x decimal64) decimal128 { return decimal128(x) }

//go:noinline
func opNarrow(x decimal128) decimal64 { return decimal64(x) }

//go:noinline
func opFromInt64(x int64) decimal64 { return decimal64(x) }

//go:noinline
func opFromInt64To128(x int64) decimal128 { return decimal128(x) }

//go:noinline
func opFromInt32(x int32) decimal64 { return decimal64(x) }

//go:noinline
func opFromUint32(x uint32) decimal64 { return decimal64(x) }

//go:noinline
func opFromUint64(x uint64) decimal64 { return decimal64(x) }

//go:noinline
func opFromUint64To128(x uint64) decimal128 { return decimal128(x) }

//go:noinline
func opToInt64(x decimal64) int64 { return int64(x) }

//go:noinline
func opToInt64From128(x decimal128) int64 { return int64(x) }

//go:noinline
func opToInt32(x decimal64) int32 { return int32(x) }

//go:noinline
func opNeg64(x decimal64) decimal64 { return -x }

//go:noinline
func opNeg128(x decimal128) decimal128 { return -x }

// The conversions that stay runtime calls on every target. They are here
// because on linux/ppc64le they convert between DPD and BID around the
// kernel call.

//go:noinline
func opToUint64(x decimal64) uint64 { return uint64(x) }

//go:noinline
func opToUint64From128(x decimal128) uint64 { return uint64(x) }

//go:noinline
func opFromFloat64(x float64) decimal64 { return decimal64(x) }

//go:noinline
func opToFloat64(x decimal64) float64 { return float64(x) }

//go:noinline
func opFromFloat64To128(x float64) decimal128 { return decimal128(x) }

//go:noinline
func opToFloat64From128(x decimal128) float64 { return float64(x) }

func d64(x bits) decimal64 { return math.Decimal64frombits(x.Lo) }

func d128(x bits) decimal128 { return math.Decimal128frombits(x.Hi, x.Lo) }

func b64(d decimal64) bits { return bits{Lo: math.Decimal64bits(d)} }

func b128(d decimal128) bits {
	hi, lo := math.Decimal128bits(d)
	return bits{Hi: hi, Lo: lo}
}

// opCase is one operator case: its operands for display, and what the
// operator and the kernel give, formatted so that equal means identical.
type opCase struct {
	args      string
	got, want string
}

func hex(x bits) string { return fmt.Sprintf("%016x%016x", x.Hi, x.Lo) }

// operatorTests lists, for each operator, how to draw a case from g, whose
// format is set to the width the name ends with.
var operatorTests = []struct {
	name string
	f    *format
	draw func(g *gen) opCase
}{
	{"Add64", dec64, func(g *gen) opCase {
		x, y := g.pair("add")
		return opCase{display(dec64, x) + ", " + display(dec64, y), hex(b64(opAdd64(d64(x), d64(y)))), hex(bits{Lo: bid.Add64(x.Lo, y.Lo, rne)})}
	}},
	{"Sub64", dec64, func(g *gen) opCase {
		x, y := g.pair("add")
		if g.r.IntN(2) == 0 {
			y = dec64.withSign(y, !dec64.signbit(y))
		}
		return opCase{display(dec64, x) + ", " + display(dec64, y), hex(b64(opSub64(d64(x), d64(y)))), hex(bits{Lo: bid.Sub64(x.Lo, y.Lo, rne)})}
	}},
	{"Mul64", dec64, func(g *gen) opCase {
		x, y := g.pair("mul")
		return opCase{display(dec64, x) + ", " + display(dec64, y), hex(b64(opMul64(d64(x), d64(y)))), hex(bits{Lo: bid.Mul64(x.Lo, y.Lo, rne)})}
	}},
	{"Div64", dec64, func(g *gen) opCase {
		x, y := g.pair("div")
		return opCase{display(dec64, x) + ", " + display(dec64, y), hex(b64(opDiv64(d64(x), d64(y)))), hex(bits{Lo: bid.Div64(x.Lo, y.Lo, rne)})}
	}},
	{"Add128", dec128, func(g *gen) opCase {
		x, y := g.pair("add")
		return opCase{display(dec128, x) + ", " + display(dec128, y), hex(b128(opAdd128(d128(x), d128(y)))), hex(bid.Add128(x, y, rne))}
	}},
	{"Sub128", dec128, func(g *gen) opCase {
		x, y := g.pair("add")
		if g.r.IntN(2) == 0 {
			y = dec128.withSign(y, !dec128.signbit(y))
		}
		return opCase{display(dec128, x) + ", " + display(dec128, y), hex(b128(opSub128(d128(x), d128(y)))), hex(bid.Sub128(x, y, rne))}
	}},
	{"Mul128", dec128, func(g *gen) opCase {
		x, y := g.pair("mul")
		return opCase{display(dec128, x) + ", " + display(dec128, y), hex(b128(opMul128(d128(x), d128(y)))), hex(bid.Mul128(x, y, rne))}
	}},
	{"Div128", dec128, func(g *gen) opCase {
		x, y := g.pair("div")
		return opCase{display(dec128, x) + ", " + display(dec128, y), hex(b128(opDiv128(d128(x), d128(y)))), hex(bid.Div128(x, y, rne))}
	}},
	{"Compare64", dec64, func(g *gen) opCase {
		x, y := g.comparable()
		a, b := x.Lo, y.Lo
		want := [6]bool{bid.Equal64(a, b), !bid.Equal64(a, b), bid.Less64(a, b), bid.LessEqual64(a, b), bid.Less64(b, a), bid.LessEqual64(b, a)}
		return opCase{display(dec64, x) + ", " + display(dec64, y), fmt.Sprint(opCmp64(d64(x), d64(y))), fmt.Sprint(want)}
	}},
	{"Compare128", dec128, func(g *gen) opCase {
		x, y := g.comparable()
		want := [6]bool{bid.Equal128(x, y), !bid.Equal128(x, y), bid.Less128(x, y), bid.LessEqual128(x, y), bid.Less128(y, x), bid.LessEqual128(y, x)}
		return opCase{display(dec128, x) + ", " + display(dec128, y), fmt.Sprint(opCmp128(d128(x), d128(y))), fmt.Sprint(want)}
	}},
	{"Min64", dec64, func(g *gen) opCase {
		x, y := g.comparable()
		return opCase{display(dec64, x) + ", " + display(dec64, y), hex(b64(opMin64(d64(x), d64(y)))), hex(bits{Lo: bid.Min64(x.Lo, y.Lo)})}
	}},
	{"Max64", dec64, func(g *gen) opCase {
		x, y := g.comparable()
		return opCase{display(dec64, x) + ", " + display(dec64, y), hex(b64(opMax64(d64(x), d64(y)))), hex(bits{Lo: bid.Max64(x.Lo, y.Lo)})}
	}},
	{"Min128", dec128, func(g *gen) opCase {
		x, y := g.comparable()
		return opCase{display(dec128, x) + ", " + display(dec128, y), hex(b128(opMin128(d128(x), d128(y)))), hex(bid.Min128(x, y))}
	}},
	{"Max128", dec128, func(g *gen) opCase {
		x, y := g.comparable()
		return opCase{display(dec128, x) + ", " + display(dec128, y), hex(b128(opMax128(d128(x), d128(y)))), hex(bid.Max128(x, y))}
	}},
	{"Neg64", dec64, func(g *gen) opCase {
		// The sign operations copy bits, so on a DPD target, which cannot
		// hold a non-canonical BID encoding, they copy the canonical one.
		x := g.dec()
		want := x.Lo
		if bid.NativeDPD {
			want = canonical64(want)
		}
		return opCase{display(dec64, x), hex(b64(opNeg64(d64(x)))), hex(bits{Lo: bid.Neg64(want)})}
	}},
	{"Neg128", dec128, func(g *gen) opCase {
		x := g.dec()
		want := x
		if bid.NativeDPD {
			want = canonical128(want)
		}
		return opCase{display(dec128, x), hex(b128(opNeg128(d128(x)))), hex(bid.Neg128(want))}
	}},
	{"Widen", dec64, func(g *gen) opCase {
		x := g.dec()
		return opCase{display(dec64, x), hex(b128(opWiden(d64(x)))), hex(bid.Widen(x.Lo))}
	}},
	{"Narrow", dec128, func(g *gen) opCase {
		x := g.dec()
		switch g.r.IntN(4) {
		case 0:
			x = dec128.encode(g.halfway(dec64))
		case 1:
			x = dec128.encode(value{kind: finite, neg: g.r.IntN(2) == 0, coeff: g.randDigits(1 + g.r.IntN(dec128.prec)),
				exp: dec64.minExp - 40 + g.r.IntN(dec64.maxExp-dec64.minExp+80)})
		}
		return opCase{display(dec128, x), hex(b64(opNarrow(d128(x)))), hex(bits{Lo: bid.Narrow(x, rne)})}
	}},
	{"FromInt64", dec64, func(g *gen) opCase {
		n := g.int64()
		return opCase{fmt.Sprint(n), hex(b64(opFromInt64(n))), hex(bits{Lo: bid.Int64To64(n, rne)})}
	}},
	{"FromInt64To128", dec128, func(g *gen) opCase {
		n := g.int64()
		return opCase{fmt.Sprint(n), hex(b128(opFromInt64To128(n))), hex(bid.Int64To128(n))}
	}},
	{"FromInt32", dec64, func(g *gen) opCase {
		n := int32(g.int64())
		return opCase{fmt.Sprint(n), hex(b64(opFromInt32(n))), hex(bits{Lo: bid.Int64To64(int64(n), rne)})}
	}},
	{"FromUint32", dec64, func(g *gen) opCase {
		n := uint32(g.uint64())
		return opCase{fmt.Sprint(n), hex(b64(opFromUint32(n))), hex(bits{Lo: bid.Uint64To64(uint64(n), rne)})}
	}},
	{"FromUint64", dec64, func(g *gen) opCase {
		n := g.uint64()
		return opCase{fmt.Sprint(n), hex(b64(opFromUint64(n))), hex(bits{Lo: bid.Uint64To64(n, rne)})}
	}},
	{"FromUint64To128", dec128, func(g *gen) opCase {
		n := g.uint64()
		return opCase{fmt.Sprint(n), hex(b128(opFromUint64To128(n))), hex(bid.Uint64To128(n))}
	}},
	{"ToInt64", dec64, func(g *gen) opCase {
		x := g.intLike()
		want, _ := bid.Int64From64(x.Lo, bid.TowardZero)
		return opCase{display(dec64, x), fmt.Sprint(opToInt64(d64(x))), fmt.Sprint(want)}
	}},
	{"ToInt64From128", dec128, func(g *gen) opCase {
		x := g.intLike()
		want, _ := bid.Int64From128(x, bid.TowardZero)
		return opCase{display(dec128, x), fmt.Sprint(opToInt64From128(d128(x))), fmt.Sprint(want)}
	}},
	{"ToInt32", dec64, func(g *gen) opCase {
		x := g.intLike()
		want, _ := bid.Int64From64(x.Lo, bid.TowardZero)
		return opCase{display(dec64, x), fmt.Sprint(opToInt32(d64(x))), fmt.Sprint(int32(want))}
	}},
	{"ToUint64", dec64, func(g *gen) opCase {
		x := g.intLike()
		want, _ := bid.Uint64From64(x.Lo, bid.TowardZero)
		return opCase{display(dec64, x), fmt.Sprint(opToUint64(d64(x))), fmt.Sprint(want)}
	}},
	{"ToUint64From128", dec128, func(g *gen) opCase {
		x := g.intLike()
		want, _ := bid.Uint64From128(x, bid.TowardZero)
		return opCase{display(dec128, x), fmt.Sprint(opToUint64From128(d128(x))), fmt.Sprint(want)}
	}},
	{"FromFloat64", dec64, func(g *gen) opCase {
		v := g.float64()
		return opCase{fmt.Sprint(v), hex(b64(opFromFloat64(v))), hex(bits{Lo: bid.Float64To64(v, rne)})}
	}},
	{"FromFloat64To128", dec128, func(g *gen) opCase {
		v := g.float64()
		return opCase{fmt.Sprint(v), hex(b128(opFromFloat64To128(v))), hex(bid.Float64To128(v, rne))}
	}},
	{"ToFloat64", dec64, func(g *gen) opCase {
		x := g.floatLike(binary64)
		return opCase{display(dec64, x), fmt.Sprint(math.Float64bits(opToFloat64(d64(x)))), fmt.Sprint(math.Float64bits(bid.Float64From64(x.Lo, rne)))}
	}},
	{"ToFloat64From128", dec128, func(g *gen) opCase {
		x := g.floatLike(binary64)
		return opCase{display(dec128, x), fmt.Sprint(math.Float64bits(opToFloat64From128(d128(x)))), fmt.Sprint(math.Float64bits(bid.Float64From128(x, rne)))}
	}},
}

// TestOperators runs every operator on the oracle's operands, 10^5 cases
// per operator (10^4 with -short, 10^6 with -bid.long), and requires the
// kernel's result exactly.
func TestOperators(t *testing.T) {
	n := numCases()
	for _, ot := range operatorTests {
		t.Run(ot.name, func(t *testing.T) {
			t.Parallel()
			g := &gen{f: ot.f, r: rand.New(rand.NewPCG(*seedFlag, streamSeed("operator"+ot.name)))}
			fails := 0
			for i := range n {
				g.regime = regime(i % int(numRegimes))
				c := ot.draw(g)
				if c.got != c.want {
					fails++
					if fails <= *showFlag {
						t.Errorf("%s(%s) [%s regime]\n\toperator: %s\n\tkernel:   %s", ot.name, c.args, g.regime, c.got, c.want)
					}
				}
			}
			if fails > 0 {
				t.Errorf("%s: %d of %d cases differ from the kernel", ot.name, fails, n)
			}
		})
	}
}

// TestDecimalRoundingMode checks that the operators round to nearest with
// ties to even. On linux/ppc64le the instructions round by FPSCR[DRN],
// which Go relies on being 0 (see cmd/compile/internal/ssagen/dpd.go), so
// this fails there if any of the other seven modes is in effect, on the
// test's thread or on a new one.
func TestDecimalRoundingMode(t *testing.T) {
	check := func(where string) {
		const big = 1234567890123456 // 16 digits: adding a half is a tie
		for _, c := range []struct {
			x, y decimal64
			want string
		}{
			{big, 0.5, "1234567890123456"},  // tie, down to even
			{big, 1.5, "1234567890123458"},  // tie, up to even
			{big, 0.51, "1234567890123457"}, // above the tie
			{-big, -0.5, "-1234567890123456"},
			{-big, -1.5, "-1234567890123458"},
			{big, 0.49, "1234567890123456"},
		} {
			got := string(bid.Append64(nil, math.Decimal64bits(opAdd64(c.x, c.y))))
			if got != c.want {
				t.Errorf("%s: %v + %v = %s, want %s", where, c.x, c.y, got, c.want)
			}
		}
		var x decimal128 = 1234567890123456789012345678901234 // 34 digits
		if got := b128(opAdd128(x, 0.5)); got != b128(x) {
			t.Errorf("%s: decimal128 tie did not round to even", where)
		}
		if got := opNarrow(1234567890123456.5); got != 1234567890123456 {
			t.Errorf("%s: Narrow(1234567890123456.5) = %v", where, got)
		}
	}
	check("test goroutine")
	done := make(chan bool)
	go func() {
		runtime.LockOSThread() // a new OS thread is created for the next goroutine to need one
		check("locked goroutine")
		done <- true
	}()
	<-done
}
