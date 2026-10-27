// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// The differential tests. TestReferenceModel runs the reference model on
// every generated case and checks its results for internal consistency;
// with -bid.dump it also writes every case in the contract's example form,
// so that an independent decimal implementation can check the model.
// TestKernel compares the kernel with the reference model, case by case,
// and reports each mismatch with its operands and a classification.

import (
	"bufio"
	"flag"
	"fmt"
	"hash/fnv"
	"internal/bid"
	"math"
	"math/big"
	"math/rand/v2"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

var (
	longTest = flag.Bool("bid.long", false, "run 10^6 cases per operation and width (default 10^5, or 10^4 with -short)")
	seedFlag = flag.Uint64("bid.seed", 1, "seed of the operand generators")
	dumpPath = flag.String("bid.dump", "", "write every case of TestReferenceModel and its result to this file, in the contract's example form")
	showFlag = flag.Int("bid.show", 3, "print at most this many mismatches per class and operation; the rest are counted")
)

func numCases() int {
	switch {
	case *longTest:
		return 1_000_000
	case testing.Short():
		return 10_000
	}
	return 100_000
}

// outcome is the result of one case, from either implementation.
type outcome struct {
	text  string  // the result in the contract's example form
	key   string  // what must match exactly: bits, Status, values
	dec   *bits   // the decimal result, if there is one
	f     *format // the format of dec
	bitOp bool    // dec comes from a sign operation and need not be canonical
}

func decOut(f *format, x bits) outcome {
	return outcome{text: display(f, x), key: fmt.Sprintf("%016x%016x", x.Hi, x.Lo), dec: &x, f: f}
}

func decStOut(f *format, x bits, st bid.Status) outcome {
	o := decOut(f, x)
	o.text += ", " + statusString(st)
	o.key += "/" + statusString(st)
	return o
}

func textOut(s string) outcome { return outcome{text: s, key: s} }

func float64Text(v float64) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func float32Text(v float32) string {
	if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
		return float64Text(float64(v))
	}
	return strconv.FormatFloat(float64(v), 'g', -1, 32)
}

// testCase is one drawn case: its arguments in the contract's example form
// and the evaluation of the operation on them.
type testCase struct {
	args string
	eval func(im impl) outcome
}

// opSpec is an operation under test. draw draws one case from g, whose
// format is the operation's width.
type opSpec struct {
	name   string    // without the width suffix
	widths []*format // nil for Widen and Narrow, which have one form each
	draw   func(g *gen) testCase
}

var both = []*format{dec64, dec128}

func binaryOp(name, tie string, fn func(im impl, f *format, x, y bits, m bid.Mode) bits) opSpec {
	return opSpec{name: name, widths: both, draw: func(g *gen) testCase {
		f := g.f
		x, y := g.pair(tie)
		m := g.mode()
		return testCase{
			args: fmt.Sprintf("%s, %s, %s", display(f, x), display(f, y), modeName(m)),
			eval: func(im impl) outcome { return decOut(f, fn(im, f, x, y, m)) },
		}
	}}
}

func predicate(name string, fn func(im impl, f *format, x, y bits) bool) opSpec {
	return opSpec{name: name, widths: both, draw: func(g *gen) testCase {
		f := g.f
		x, y := g.comparable()
		return testCase{
			args: fmt.Sprintf("%s, %s", display(f, x), display(f, y)),
			eval: func(im impl) outcome { return textOut(strconv.FormatBool(fn(im, f, x, y))) },
		}
	}}
}

func unaryDec(name string, fn func(im impl, f *format, x bits) outcome) opSpec {
	return opSpec{name: name, widths: both, draw: func(g *gen) testCase {
		f := g.f
		x := g.dec()
		return testCase{args: display(f, x), eval: func(im impl) outcome { return fn(im, f, x) }}
	}}
}

// comparable returns operands for the comparisons: often members of one
// cohort, zeros of opposite signs, or equal NaNs, so that equality and the
// cohort ordering of TotalOrder are exercised.
func (g *gen) comparable() (bits, bits) {
	f := g.f
	x := g.dec()
	v := f.decode(x)
	if g.r.IntN(2) == 0 && v.kind == finite {
		// A member of x's cohort, or of the opposite-signed one for zero.
		k := g.r.IntN(f.prec) - g.r.IntN(f.prec)
		w := value{kind: finite, neg: v.neg, coeff: new(big.Int).Set(v.coeff), exp: v.exp}
		if v.coeff.Sign() == 0 {
			w.exp = clamp(v.exp+k, f.minExp, f.maxExp)
			w.neg = g.r.IntN(2) == 0
		} else if k > 0 && digits(v.coeff)+k <= f.prec && v.exp-k >= f.minExp {
			w.coeff.Mul(w.coeff, pow10(k))
			w.exp -= k
		} else if k < 0 && trailingZeros(v.coeff) >= -k && v.exp-k <= f.maxExp {
			w.coeff.Quo(w.coeff, pow10(-k))
			w.exp -= k
		}
		return x, f.encode(w)
	}
	return g.pair("add")
}

var ops = []opSpec{
	binaryOp("Add", "add", impl.add),
	{name: "Sub", widths: both, draw: func(g *gen) testCase {
		f := g.f
		x, y := g.pair("add")
		if g.r.IntN(2) == 0 {
			y = f.withSign(y, !f.signbit(y)) // so that ties and cancellations occur
		}
		m := g.mode()
		return testCase{
			args: fmt.Sprintf("%s, %s, %s", display(f, x), display(f, y), modeName(m)),
			eval: func(im impl) outcome { return decOut(f, im.sub(f, x, y, m)) },
		}
	}},
	binaryOp("Mul", "mul", impl.mul),
	binaryOp("Div", "div", impl.div),
	{name: "FMA", widths: both, draw: func(g *gen) testCase {
		f := g.f
		x, y := g.pair("mul")
		var z bits
		switch g.r.IntN(3) {
		case 0:
			// Minus the rounded product: the sum is the product's rounding
			// error, or an exact zero.
			p := f.mul(x, y, bid.ToNearestEven)
			z = f.withSign(p, !f.signbit(p))
		case 1:
			z = f.encode(g.near(f.decode(f.mul(x, y, bid.TowardZero))))
		default:
			z = g.dec()
		}
		m := g.mode()
		return testCase{
			args: fmt.Sprintf("%s, %s, %s, %s", display(f, x), display(f, y), display(f, z), modeName(m)),
			eval: func(im impl) outcome { return decOut(f, im.fma(f, x, y, z, m)) },
		}
	}},
	{name: "Rem", widths: both, draw: func(g *gen) testCase { return g.remCase(impl.rem) }},
	{name: "Mod", widths: both, draw: func(g *gen) testCase { return g.remCase(impl.mod) }},
	{name: "RoundToIntegral", widths: both, draw: func(g *gen) testCase {
		f := g.f
		x := g.dec()
		if g.r.IntN(3) == 0 {
			// A fraction of exactly one half, or just either side of it.
			c := g.randDigits(1 + g.r.IntN(f.prec-1))
			c.Mul(c, bigTen).Add(c, big.NewInt(int64([...]int{5, 5, 4, 6}[g.r.IntN(4)])))
			x = f.encode(value{kind: finite, neg: g.r.IntN(2) == 0, coeff: c, exp: -1})
		}
		m := g.mode()
		return testCase{
			args: fmt.Sprintf("%s, %s", display(f, x), modeName(m)),
			eval: func(im impl) outcome { return decOut(f, im.roundToIntegral(f, x, m)) },
		}
	}},
	{name: "Quantize", widths: both, draw: func(g *gen) testCase {
		f := g.f
		x, y := g.dec(), g.dec()
		if v := f.decode(x); v.kind == finite && g.r.IntN(2) == 0 {
			y = f.encode(g.near(v))
		}
		m := g.mode()
		return testCase{
			args: fmt.Sprintf("%s, %s, %s", display(f, x), display(f, y), modeName(m)),
			eval: func(im impl) outcome { return decOut(f, im.quantize(f, x, y, m)) },
		}
	}},
	unaryDec("Reduce", func(im impl, f *format, x bits) outcome { return decOut(f, im.reduce(f, x)) }),
	predicate("SameQuantum", impl.sameQuantum),
	{name: "Min", widths: both, draw: func(g *gen) testCase { return g.minMaxCase(impl.min) }},
	{name: "Max", widths: both, draw: func(g *gen) testCase { return g.minMaxCase(impl.max) }},
	predicate("Equal", impl.equal),
	predicate("Less", impl.less),
	predicate("LessEqual", impl.lessEqual),
	predicate("TotalOrder", impl.totalOrder),
	{name: "Compare", widths: both, draw: func(g *gen) testCase {
		f := g.f
		x, y := g.comparable()
		return testCase{
			args: fmt.Sprintf("%s, %s", display(f, x), display(f, y)),
			eval: func(im impl) outcome {
				c, ok := im.compare(f, x, y)
				return textOut(fmt.Sprintf("%d, %t", c, ok))
			},
		}
	}},
	unaryDec("Key", func(im impl, f *format, x bits) outcome { return textOut(fmt.Sprintf("%+v", im.key(f, x))) }),
	unaryDec("Unpack", func(im impl, f *format, x bits) outcome {
		k, neg, c, exp := im.unpack(f, x)
		return textOut(fmt.Sprintf("%d, %t, %s, %d", k, neg, fromUint128(c), exp))
	}),
	{name: "Pack", widths: both, draw: func(g *gen) testCase {
		f := g.f
		neg := g.r.IntN(2) == 0
		c := toUint128(g.coeff())
		exp := g.exp()
		switch g.r.IntN(6) {
		case 0:
			c = toUint128(new(big.Int).Add(f.maxCoeff, big.NewInt(int64(g.r.IntN(3)))))
		case 1:
			exp = [...]int{f.minExp - 1, f.maxExp + 1, math.MinInt, math.MaxInt}[g.r.IntN(4)]
		}
		if f.width == 64 {
			c.Hi = 0
		}
		return testCase{
			args: fmt.Sprintf("%t, %s, %d", neg, fromUint128(c), exp),
			eval: func(im impl) outcome {
				x, ok := im.pack(f, neg, c, exp)
				o := decOut(f, x)
				o.text += fmt.Sprintf(", %t", ok)
				o.key += fmt.Sprintf("/%t", ok)
				return o
			},
		}
	}},
	unaryDec("Is", func(im impl, f *format, x bits) outcome {
		return textOut(fmt.Sprintf("NaN=%t Signaling=%t Inf=%t Finite=%t Zero=%t Subnormal=%t Canonical=%t Signbit=%t",
			im.isNaN(f, x), im.isSignaling(f, x), im.isInf(f, x), im.isFinite(f, x),
			im.isZero(f, x), im.isSubnormal(f, x), im.isCanonical(f, x), im.signbit(f, x)))
	}),
	unaryDec("Neg", func(im impl, f *format, x bits) outcome {
		o := decOut(f, im.neg(f, x))
		o.bitOp = true
		return o
	}),
	unaryDec("Abs", func(im impl, f *format, x bits) outcome {
		o := decOut(f, im.abs(f, x))
		o.bitOp = true
		return o
	}),
	{name: "CopySign", widths: both, draw: func(g *gen) testCase {
		f := g.f
		x, s := g.dec(), g.dec()
		return testCase{
			args: fmt.Sprintf("%s, %s", display(f, x), display(f, s)),
			eval: func(im impl) outcome {
				o := decOut(f, im.copySign(f, x, s))
				o.bitOp = true
				return o
			},
		}
	}},
	{name: "Widen", draw: func(g *gen) testCase {
		g.f = dec64
		x := g.dec()
		return testCase{args: display(dec64, x), eval: func(im impl) outcome { return decOut(dec128, im.widen(x)) }}
	}},
	{name: "Narrow", draw: func(g *gen) testCase {
		g.f = dec128
		x := g.dec()
		switch g.r.IntN(4) {
		case 0:
			x = dec128.encode(g.halfway(dec64)) // a decimal64 tie
		case 1:
			// Near the ends of the decimal64 range.
			x = dec128.encode(value{kind: finite, neg: g.r.IntN(2) == 0, coeff: g.randDigits(1 + g.r.IntN(dec128.prec)),
				exp: [...]int{dec64.minExp, dec64.maxExp, dec64.emin}[g.r.IntN(3)] - g.r.IntN(dec128.prec)})
		}
		m := g.mode()
		return testCase{
			args: fmt.Sprintf("%s, %s", display(dec128, x), modeName(m)),
			eval: func(im impl) outcome { return decOut(dec64, im.narrow(x, m)) },
		}
	}},
	{name: "Int64To", widths: both, draw: func(g *gen) testCase {
		f := g.f
		n := g.int64()
		m := g.mode()
		args := fmt.Sprintf("%d, %s", n, modeName(m))
		if f.width == 128 {
			args = strconv.FormatInt(n, 10)
		}
		return testCase{args: args, eval: func(im impl) outcome { return decOut(f, im.int64To(f, n, m)) }}
	}},
	{name: "Uint64To", widths: both, draw: func(g *gen) testCase {
		f := g.f
		n := g.uint64()
		m := g.mode()
		args := fmt.Sprintf("%d, %s", n, modeName(m))
		if f.width == 128 {
			args = strconv.FormatUint(n, 10)
		}
		return testCase{args: args, eval: func(im impl) outcome { return decOut(f, im.uint64To(f, n, m)) }}
	}},
	{name: "Int64From", widths: both, draw: func(g *gen) testCase {
		f := g.f
		x := g.intLike()
		m := g.mode()
		return testCase{
			args: fmt.Sprintf("%s, %s", display(f, x), modeName(m)),
			eval: func(im impl) outcome {
				n, st := im.int64From(f, x, m)
				return textOut(fmt.Sprintf("%d, %s", n, statusString(st)))
			},
		}
	}},
	{name: "Uint64From", widths: both, draw: func(g *gen) testCase {
		f := g.f
		x := g.intLike()
		m := g.mode()
		return testCase{
			args: fmt.Sprintf("%s, %s", display(f, x), modeName(m)),
			eval: func(im impl) outcome {
				n, st := im.uint64From(f, x, m)
				return textOut(fmt.Sprintf("%d, %s", n, statusString(st)))
			},
		}
	}},
	{name: "Float64To", widths: both, draw: func(g *gen) testCase {
		f := g.f
		v := g.float64()
		m := g.mode()
		return testCase{
			args: fmt.Sprintf("%s, %s", float64Text(v), modeName(m)),
			eval: func(im impl) outcome { return decOut(f, im.float64To(f, v, m)) },
		}
	}},
	{name: "Float32To", widths: both, draw: func(g *gen) testCase {
		f := g.f
		v := g.float32()
		m := g.mode()
		return testCase{
			args: fmt.Sprintf("%s, %s", float32Text(v), modeName(m)),
			eval: func(im impl) outcome { return decOut(f, im.float32To(f, v, m)) },
		}
	}},
	{name: "Float64From", widths: both, draw: func(g *gen) testCase {
		f := g.f
		x := g.floatLike(binary64)
		m := g.mode()
		return testCase{
			args: fmt.Sprintf("%s, %s", display(f, x), modeName(m)),
			eval: func(im impl) outcome {
				v := im.float64From(f, x, m)
				return outcome{text: float64Text(v), key: fmt.Sprintf("%016x", math.Float64bits(v))}
			},
		}
	}},
	{name: "Float32From", widths: both, draw: func(g *gen) testCase {
		f := g.f
		x := g.floatLike(binary32)
		m := g.mode()
		return testCase{
			args: fmt.Sprintf("%s, %s", display(f, x), modeName(m)),
			eval: func(im impl) outcome {
				v := im.float32From(f, x, m)
				return outcome{text: float32Text(v), key: fmt.Sprintf("%08x", math.Float32bits(v))}
			},
		}
	}},
	{name: "Parse", widths: both, draw: func(g *gen) testCase {
		f := g.f
		s := g.numeral()
		m := g.mode()
		return testCase{
			args: fmt.Sprintf("%s, %s", strconv.Quote(s), modeName(m)),
			eval: func(im impl) outcome {
				x, st := im.parse(f, s, m)
				return decStOut(f, x, st)
			},
		}
	}},
	unaryDec("Append", func(im impl, f *format, x bits) outcome { return textOut(strconv.Quote(im.appendTo(f, x))) }),
	{name: "Round", widths: both, draw: func(g *gen) testCase {
		f := g.f
		neg, c, exp, sticky := g.parts()
		m := g.mode()
		return testCase{
			args: fmt.Sprintf("%t, %s, %d, %t, %s", neg, fromUint128(c), exp, sticky, modeName(m)),
			eval: func(im impl) outcome {
				x, st := im.round(f, neg, c, exp, sticky, m)
				return decStOut(f, x, st)
			},
		}
	}},
}

func (g *gen) remCase(fn func(im impl, f *format, x, y bits) bits) testCase {
	f := g.f
	x, y := g.pair("div")
	if g.regime == boundary && g.r.IntN(3) == 0 {
		// A quotient with far more digits than the precision, which GDA
		// refuses and the contract computes exactly.
		x = f.encode(value{kind: finite, neg: g.r.IntN(2) == 0, coeff: g.randDigits(f.prec), exp: f.maxExp - g.r.IntN(20)})
		y = f.encode(value{kind: finite, neg: g.r.IntN(2) == 0, coeff: g.randDigits(1 + g.r.IntN(f.prec)), exp: f.minExp + g.r.IntN(20)})
	}
	return testCase{
		args: fmt.Sprintf("%s, %s", display(f, x), display(f, y)),
		eval: func(im impl) outcome { return decOut(f, fn(im, f, x, y)) },
	}
}

func (g *gen) minMaxCase(fn func(im impl, f *format, x, y bits) bits) testCase {
	f := g.f
	x, y := g.comparable()
	return testCase{
		args: fmt.Sprintf("%s, %s", display(f, x), display(f, y)),
		eval: func(im impl) outcome { return decOut(f, fn(im, f, x, y)) },
	}
}

// floatLike returns a decimal operand for conversion to the binary format
// b: often a tie between two binary values, or a value near b's overflow
// and underflow thresholds.
func (g *gen) floatLike(b binary) bits {
	f := g.f
	switch g.r.IntN(4) {
	case 0:
		return g.binaryTie(b.prec)
	case 1:
		// Within a few orders of magnitude of the largest finite binary
		// value or of the smallest subnormal one.
		maxAdj := int(float64(b.emax) * 0.30103)
		minAdj := int(float64(b.emin-b.prec+1) * 0.30103)
		adj := [...]int{maxAdj, minAdj, minAdj - 1}[g.r.IntN(3)] + g.r.IntN(5) - 2
		c := g.randDigits(f.prec)
		return f.encode(value{kind: finite, neg: g.r.IntN(2) == 0, coeff: c, exp: adj - f.prec + 1})
	}
	return g.dec()
}

// streamSeed derives a generator stream from the seed flag and the name of
// the operation and width, so that each operation's cases are fixed and
// independent of which other operations run.
func streamSeed(name string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(name))
	return h.Sum64()
}

type subtest struct {
	name string
	op   opSpec
	f    *format
}

func subtests() []subtest {
	var sts []subtest
	for _, op := range ops {
		if op.widths == nil {
			sts = append(sts, subtest{op.name, op, nil})
			continue
		}
		for _, f := range op.widths {
			sts = append(sts, subtest{fmt.Sprintf("%s%d", op.name, f.width), op, f})
		}
	}
	return sts
}

// TestReferenceModel runs the reference model on every case of every
// operation and checks that each decimal result is canonical (except those
// of the sign operations) and survives the character form: formatting it
// and parsing the text back gives the same encoding, exactly.
func TestReferenceModel(t *testing.T) {
	n := numCases()
	var dumpMu sync.Mutex
	var dump *bufio.Writer
	if *dumpPath != "" {
		file, err := os.Create(*dumpPath)
		if err != nil {
			t.Fatal(err)
		}
		dump = bufio.NewWriterSize(file, 1<<20)
		t.Cleanup(func() {
			if err := dump.Flush(); err != nil {
				t.Error(err)
			}
			file.Close()
		})
	}
	t.Run("group", func(t *testing.T) {
		for _, st := range subtests() {
			t.Run(st.name, func(t *testing.T) {
				t.Parallel()
				g := &gen{r: rand.New(rand.NewPCG(*seedFlag, streamSeed(st.name)))}
				var lines []string
				for i := range n {
					g.f = st.f
					g.regime = regime(i % int(numRegimes))
					c := st.op.draw(g)
					o := c.eval(reference{})
					if o.dec != nil && !o.bitOp {
						if !o.f.canonical(*o.dec) {
							t.Errorf("%s(%s): result %s is not canonical (%016x%016x)", st.name, c.args, o.text, o.dec.Hi, o.dec.Lo)
						}
						if back, s := o.f.parse(o.f.format(*o.dec), bid.ToNearestEven); back != *o.dec || s != bid.Exact {
							t.Errorf("%s(%s) = %s does not survive the character form", st.name, c.args, o.text)
						}
					}
					if dump != nil {
						lines = append(lines, fmt.Sprintf("//\t%s(%s) = %s\n", st.name, c.args, o.text))
					}
				}
				if dump != nil {
					dumpMu.Lock()
					for _, l := range lines {
						dump.WriteString(l)
					}
					dumpMu.Unlock()
				}
			})
		}
	})
}

// TestReferenceProperties checks relations between the reference model's
// operations that the contract states: Key equality is value equality (or
// both NaN), a widened decimal64 has the decimal64's Key, TotalOrder is a
// total order consistent with Compare, and Widen then Narrow is the
// identity.
func TestReferenceProperties(t *testing.T) {
	n := numCases()
	for _, f := range both {
		t.Run(f.name, func(t *testing.T) {
			t.Parallel()
			g := &gen{f: f, r: rand.New(rand.NewPCG(*seedFlag, streamSeed("properties"+f.name)))}
			for i := range n {
				g.regime = regime(i % int(numRegimes))
				x, y := g.comparable()
				a, b := f.decode(x), f.decode(y)
				c, ok := f.compare(x, y)
				bothNaN := a.kind == notANumber && b.kind == notANumber
				if (f.key(x) == f.key(y)) != (ok && c == 0 || bothNaN) {
					t.Errorf("%s: Key(%s) == Key(%s) is %t, but Compare is %d, %t", f.name, display(f, x), display(f, y), f.key(x) == f.key(y), c, ok)
				}
				xy, yx := f.totalOrder(x, y), f.totalOrder(y, x)
				if !xy && !yx {
					t.Errorf("%s: TotalOrder(%s, %s) and its converse are both false", f.name, display(f, x), display(f, y))
				}
				if ok && c < 0 && !xy || ok && c > 0 && !yx {
					t.Errorf("%s: TotalOrder(%s, %s) disagrees with Compare %d", f.name, display(f, x), display(f, y), c)
				}
				if xy && yx && !bothNaN && (f.encode(a) != f.encode(b) || a.neg != b.neg) {
					t.Errorf("%s: TotalOrder holds both ways for distinct %s and %s", f.name, display(f, x), display(f, y))
				}
				if f == dec64 {
					if dec128.key(widen(x)) != dec64.key(x) {
						t.Errorf("Key128(Widen(%s)) != Key64", display(f, x))
					}
					if w := narrow(widen(x), bid.ToNearestEven); a.kind != notANumber && w != dec64.encode(a) {
						t.Errorf("Narrow(Widen(%s)) = %s", display(f, x), display(dec64, w))
					}
				}
			}
		})
	}
}

// TestKernel compares the kernel with the reference model on every case
// of every operation the kernel implements.
func TestKernel(t *testing.T) {
	n := numCases()
	var missing []string
	var mu sync.Mutex
	t.Run("group", func(t *testing.T) {
		for _, st := range subtests() {
			t.Run(st.name, func(t *testing.T) {
				t.Parallel()
				g := &gen{r: rand.New(rand.NewPCG(*seedFlag, streamSeed(st.name)))}
				shown := map[string]int{}
				counts := map[string]int{}
				unimplemented := 0
				for i := range n {
					g.f = st.f
					g.regime = regime(i % int(numRegimes))
					c := st.op.draw(g)
					want := c.eval(reference{})
					var got outcome
					implemented, err := try(func() { got = c.eval(kernel{}) })
					switch {
					case err != nil:
						counts["panic"]++
						if shown["panic"] < *showFlag {
							shown["panic"]++
							t.Errorf("%s(%s): kernel %v (%s regime)", st.name, c.args, err, g.regime)
						}
						continue
					case !implemented:
						if i == 0 {
							mu.Lock()
							missing = append(missing, st.name)
							mu.Unlock()
							t.Skipf("the kernel does not implement %s", st.name)
						}
						unimplemented++
						continue
					}
					if got.key == want.key {
						continue
					}
					class := classify(want, got)
					counts[class]++
					if shown[class] < *showFlag {
						shown[class]++
						t.Errorf("%s(%s) [%s, %s regime]\n\tkernel:    %s\n\treference: %s", st.name, c.args, class, g.regime, got.text, want.text)
					}
				}
				if unimplemented > 0 {
					t.Errorf("%s: the kernel panicked %q on %d of %d cases after implementing the first", st.name, notImplemented, unimplemented, n)
				}
				if len(counts) > 0 {
					t.Errorf("%s: mismatches by class over %d cases: %v", st.name, n, counts)
				}
			})
		}
	})
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Logf("not implemented by the kernel (%d of %d): %s", len(missing), len(subtests()), strings.Join(missing, ", "))
	}
}

// classify names the way a kernel result differs from the reference.
func classify(want, got outcome) string {
	if want.dec == nil || got.dec == nil {
		return "result"
	}
	f := want.f
	a, b := f.decode(*want.dec), f.decode(*got.dec)
	switch {
	case a.kind != b.kind:
		return "kind"
	case a.kind == infinite && a.neg != b.neg:
		return "sign"
	case a.kind == finite && cmpValue(a, b) != 0:
		return "value"
	case a.kind == finite && a.neg != b.neg:
		return "sign"
	case a.kind == finite && a.exp != b.exp:
		return "quantum"
	case *want.dec != *got.dec:
		return "encoding"
	}
	return "status" // or Pack's ok flag: the decimal results are identical
}

// TestFormatConstants compares the kernel's format constants with the
// reference model's, which are written down independently.
func TestFormatConstants(t *testing.T) {
	check := func(name string, got, want int) {
		if got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
	check("Precision64", bid.Precision64, dec64.prec)
	check("Emin64", bid.Emin64, dec64.emin)
	check("MinExp64", bid.MinExp64, dec64.minExp)
	check("MaxExp64", bid.MaxExp64, dec64.maxExp)
	check("Bias64", bid.Bias64, dec64.bias)
	check("Precision128", bid.Precision128, dec128.prec)
	check("Emin128", bid.Emin128, dec128.emin)
	check("MinExp128", bid.MinExp128, dec128.minExp)
	check("MaxExp128", bid.MaxExp128, dec128.maxExp)
	check("Bias128", bid.Bias128, dec128.bias)
	if got := new(big.Int).SetUint64(bid.MaxCoeff64); got.Cmp(dec64.maxCoeff) != 0 {
		t.Errorf("MaxCoeff64 = %s, want %s", got, dec64.maxCoeff)
	}
	if got := fromUint128(bid.Uint128{Hi: bid.MaxCoeff128Hi, Lo: bid.MaxCoeff128Lo}); got.Cmp(dec128.maxCoeff) != 0 {
		t.Errorf("MaxCoeff128 = %s, want %s", got, dec128.maxCoeff)
	}
	for _, c := range []struct {
		name string
		got  bits
		want value
	}{
		{"Inf64", bits{Lo: bid.Inf64}, value{kind: infinite}},
		{"NaN64", bits{Lo: bid.NaN64}, nan},
		{"Zero64", bits{Lo: bid.Zero64}, value{kind: finite, coeff: new(big.Int)}},
	} {
		if w := dec64.encode(c.want); c.got != w {
			t.Errorf("%s = %#016x, want %#016x", c.name, c.got.Lo, w.Lo)
		}
	}
	for _, c := range []struct {
		name string
		got  bits
		want value
	}{
		{"Inf128Hi", bits{Hi: bid.Inf128Hi}, value{kind: infinite}},
		{"NaN128Hi", bits{Hi: bid.NaN128Hi}, nan},
		{"Zero128Hi", bits{Hi: bid.Zero128Hi}, value{kind: finite, coeff: new(big.Int)}},
	} {
		if w := dec128.encode(c.want); c.got != w {
			t.Errorf("%s = %#016x, want %#016x", c.name, c.got.Hi, w.Hi)
		}
	}
	// Encodings computed by hand from the field layout of IEEE 754-2008
	// clause 3.5.2, so that the model's codec is checked against something
	// other than itself. The largest finite decimal64, 9999999999999999 at
	// exponent 369, needs the second form: 11, the biased exponent 767 at
	// bit 51, and the coefficient's low 51 bits below an implicit 100. The
	// smallest second-form coefficient, 2^53, at exponent 0. The largest
	// finite decimal128, in the first form: biased exponent 12287 at bit
	// 49 of Hi. A decimal128 second-form encoding, always non-canonical: a
	// zero with biased exponent 6176 at bit 47 of Hi, that is, exponent 0.
	for _, c := range []struct {
		f     *format
		x     bits
		coeff string
		exp   int
	}{
		{dec64, bits{Lo: 0x77FB86F26FC0FFFF}, "9999999999999999", 369},
		{dec64, bits{Lo: 0x6C70000000000000}, "9007199254740992", 0},
		{dec128, bits{Hi: 0x5FFFED09BEAD87C0, Lo: 0x378D8E63FFFFFFFF}, "9999999999999999999999999999999999", 6111},
		{dec128, bits{Hi: 3<<61 | 6176<<47}, "0", 0},
	} {
		v := c.f.decode(c.x)
		if v.kind != finite || v.coeff.String() != c.coeff || v.exp != c.exp {
			t.Errorf("%s decode(%016x%016x) = %s × 10^%d, want %s × 10^%d", c.f.name, c.x.Hi, c.x.Lo, v.coeff, v.exp, c.coeff, c.exp)
		}
		if c.coeff != "0" && c.f.encode(v) != c.x {
			t.Errorf("%s encode(%s × 10^%d) = %v, want %016x%016x", c.f.name, c.coeff, c.exp, c.f.encode(v), c.x.Hi, c.x.Lo)
		}
	}
	if !dec64.decode(bits{Lo: bid.SNaN64}).signaling || !dec128.decode(bits{Hi: bid.SNaN128Hi}).signaling {
		t.Errorf("SNaN64 or SNaN128Hi is not a signaling NaN")
	}
}
