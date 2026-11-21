// Command sim runs the prototypes of package ops over operand workloads,
// counts the share of operations each layout sends to the general routine,
// and checks every fast-path result against an exact math/big reference
// (value, exponent and sign, including the sign of zero).
package main

import (
	"fmt"
	"math"
	"math/big"
	mrand "math/rand"
	"math/rand/v2"
	"os"

	"moneyform/ops"
)

// val is a decimal128 value: finite (c >= 0 at quantum q), or a special.
type val struct {
	neg     bool
	c       *big.Int
	q       int
	special int // 0 finite, 1 inf, 2 qNaN
}

var (
	two64    = new(big.Int).Lsh(big.NewInt(1), 64)
	two59    = new(big.Int).Lsh(big.NewInt(1), 59)
	max34    = new(big.Int).Sub(new(big.Int).Exp(big.NewInt(10), big.NewInt(34), nil), big.NewInt(1))
	mask64   = new(big.Int).Sub(two64, big.NewInt(1))
	ubdInfHi = uint64(7168)<<50 | 0x1F<<44
	ubdNaNHi = uint64(7680)<<50 | 0x1F<<44
)

func ubd(v val) (lo, hi uint64) {
	switch v.special {
	case 1:
		hi = ubdInfHi
	case 2:
		hi = ubdNaNHi
	default:
		lo = new(big.Int).And(v.c, mask64).Uint64()
		hi = uint64(int64(v.q))<<50 | new(big.Int).Rsh(v.c, 64).Uint64()
	}
	if v.neg {
		hi |= 1 << 49
	}
	return
}

// money reports whether v has a money-form encoding, and its low word.
func money(v val) (uint64, bool) {
	if v.special != 0 || v.q > 0 || v.q < -15 {
		return 0, false
	}
	if v.neg && v.c.Sign() == 0 { // -0 is general form
		return 0, false
	}
	c := new(big.Int).Set(v.c)
	if v.neg {
		c.Neg(c)
	}
	if c.Cmp(two59) >= 0 || c.Cmp(new(big.Int).Neg(two59)) < 0 {
		return 0, false
	}
	return uint64(c.Int64())<<4 | uint64(-v.q), true
}

func cform(v val) (lo, hi uint64) {
	if w, ok := money(v); ok {
		return w, 0
	}
	lo, hi = ubd(v)
	return lo, hi ^ 1<<63 // qExp offset by 8192: the high word is never 0
}

func decUBD(lo, hi uint64) val {
	c := new(big.Int).Lsh(new(big.Int).SetUint64(hi&(1<<49-1)), 64)
	c.Or(c, new(big.Int).SetUint64(lo))
	return val{neg: hi>>49&1 == 1, c: c, q: int(int64(hi) >> 50)}
}

func decC(lo, hi uint64) val {
	if hi == 0 {
		c := int64(lo) >> 4
		v := val{neg: c < 0, c: big.NewInt(c), q: -int(lo & 15)}
		v.c.Abs(v.c)
		return v
	}
	return decUBD(lo, hi^1<<63)
}

func pow10(n int) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }

func signed(v val) *big.Int {
	c := new(big.Int).Set(v.c)
	if v.neg {
		c.Neg(c)
	}
	return c
}

// align returns the signed coefficients of x and y at the smaller quantum.
func align(x, y val) (*big.Int, *big.Int, int) {
	q := min(x.q, y.q)
	a := new(big.Int).Mul(signed(x), pow10(x.q-q))
	b := new(big.Int).Mul(signed(y), pow10(y.q-q))
	return a, b, q
}

// The exact reference results. Fast paths take only exact cases, so no
// rounding is needed; ok is false where the exact result would not fit.
func refAdd(x, y val, sub bool) (val, bool) {
	a, b, q := align(x, y)
	if sub {
		b.Neg(b)
	}
	s := new(big.Int).Add(a, b)
	r := val{neg: s.Sign() < 0, c: new(big.Int).Abs(s), q: q}
	if s.Sign() == 0 { // ties-to-even: +0 unless both addends are -0
		bneg := y.neg != sub
		r.neg = x.neg && bneg
	}
	return r, r.c.Cmp(max34) <= 0
}

func refMul(x, y val) (val, bool) {
	c := new(big.Int).Mul(x.c, y.c)
	r := val{neg: x.neg != y.neg, c: c, q: x.q + y.q}
	return r, c.Cmp(max34) <= 0 && r.q >= -6176 && r.q <= 6111
}

func refCmp(x, y val) int {
	inf := func(v val) int { // +-1 for an infinity, 0 for a finite value
		if v.special != 1 {
			return 0
		}
		if v.neg {
			return -1
		}
		return 1
	}
	if ix, iy := inf(x), inf(y); ix != 0 || iy != 0 {
		switch {
		case ix == iy:
			return 0
		case ix > iy || iy == 0 && ix > 0:
			return 1
		}
		return -1
	}
	a, b, _ := align(x, y)
	return a.Cmp(b)
}

func same(a, b val) bool {
	return a.special == b.special && a.neg == b.neg && a.q == b.q && a.c.Cmp(b.c) == 0
}

type layout struct {
	name          string
	enc           func(val) (uint64, uint64)
	dec           func(uint64, uint64) val
	add, sub, mul func(a, b, c, d uint64) (uint64, uint64)
	eq, lt        func(a, b, c, d uint64) bool
}

var layouts = []layout{
	{"U", ubd, decUBD, ops.UAdd, ops.USub, ops.UMul, ops.UEq, ops.ULt},
	{"N", ubd, decUBD, ops.NAdd, ops.NSub, ops.NMul, ops.NEq, ops.NLt},
	{"C", cform, decC, ops.CAdd, ops.CSub, ops.CMul, ops.CEq, ops.CLt},
}

var errors int

// run performs op on (x, y) in layout l; it reports whether the fast path
// took it, and checks a fast result.
func run(l layout, op string, x, y val) bool {
	xl, xh := l.enc(x)
	yl, yh := l.enc(y)
	before := ops.Slow
	var rl, rh uint64
	var rb bool
	switch op {
	case "+":
		rl, rh = l.add(xl, xh, yl, yh)
	case "-":
		rl, rh = l.sub(xl, xh, yl, yh)
	case "*":
		rl, rh = l.mul(xl, xh, yl, yh)
	case "==":
		rb = l.eq(xl, xh, yl, yh)
	case "<":
		rb = l.lt(xl, xh, yl, yh)
	}
	if ops.Slow != before {
		return false
	}
	bad := ""
	if (x.special != 0 || y.special != 0) && !(op == "==" || op == "<") || x.special == 2 || y.special == 2 {
		bad = "special operand on the fast path"
	} else {
		switch op {
		case "+", "-", "*":
			var want val
			var ok bool
			if op == "*" {
				want, ok = refMul(x, y)
			} else {
				want, ok = refAdd(x, y, op == "-")
			}
			wl, wh := l.enc(want)
			switch {
			case !ok:
				bad = "fast path took an inexact result"
			case !same(l.dec(rl, rh), want):
				bad = fmt.Sprintf("got %+v want %+v", l.dec(rl, rh), want)
			case rl != wl || rh != wh:
				bad = "result not canonical"
			}
		case "==":
			if rb != (refCmp(x, y) == 0) {
				bad = "== wrong"
			}
		case "<":
			if rb != (refCmp(x, y) < 0) {
				bad = "< wrong"
			}
		}
	}
	if bad != "" {
		errors++
		if errors < 20 {
			fmt.Fprintf(os.Stderr, "%s %v %s %v: %s\n", l.name, x, op, y, bad)
		}
	}
	return true
}

// ---- operand generators ----

var rng = rand.New(rand.NewPCG(1, 2))

// logUniform returns an integer log-uniformly distributed in [lo, hi].
func logUniform(lo, hi float64) *big.Int {
	f := math.Round(lo * math.Pow(hi/lo, rng.Float64()))
	b, _ := new(big.Float).SetFloat64(f).Int(nil)
	return b
}

var randSrc = mrand.New(mrand.NewSource(3))

func digits(n int) *big.Int { // uniform among n-digit integers
	lo := pow10(n - 1)
	span := new(big.Int).Sub(pow10(n), lo)
	r := new(big.Int).Rand(randSrc, span)
	return r.Add(r, lo)
}

func amount(scale int, lo, hi float64, negShare float64) val {
	return val{neg: rng.Float64() < negShare, c: logUniform(lo, hi), q: -scale}
}

type workload struct {
	name string
	op   string
	n    int
	gen  func() (val, val)
}

func main() {
	const n = 200000
	var acc val
	work := []workload{
		{"ledger: running total of amounts (scale 2, 10% refunds)", "+", n, func() (val, val) {
			if acc.c == nil {
				acc = val{c: big.NewInt(0), q: -2}
			}
			x := amount(2, 1, 1e6, 0.10)
			a := acc
			r, _ := refAdd(acc, x, false)
			acc = r
			return a, x
		}},
		{"pairs: amount + amount (scale 2, 10% negative)", "+", n, func() (val, val) {
			return amount(2, 1, 1e8, 0.10), amount(2, 1, 1e8, 0.10)
		}},
		{"pairs: amount - amount (scale 2, 10% negative)", "-", n, func() (val, val) {
			return amount(2, 1, 1e8, 0.10), amount(2, 1, 1e8, 0.10)
		}},
		{"price (scale 2) x quantity (integer, 1% zero, 5% credits)", "*", n, func() (val, val) {
			q := val{c: logUniform(1, 1000), q: 0}
			if rng.Float64() < 0.01 {
				q.c = big.NewInt(0)
			}
			return amount(2, 1, 1e5, 0.05), q
		}},
		{"amount (scale 2) x tax rate (scale 4)", "*", n, func() (val, val) {
			return amount(2, 1, 1e8, 0.10), val{c: logUniform(1, 2500), q: -4}
		}},
		{"amount (scale 2) x FX rate (scale 6)", "*", n, func() (val, val) {
			return amount(2, 1, 1e10, 0.10), val{c: logUniform(5e5, 2e6), q: -6}
		}},
		{"amount + amount x rate (scale 2 + scale 6)", "+", n, func() (val, val) {
			t, _ := refMul(amount(2, 1, 1e8, 0), val{c: logUniform(1, 2500), q: -4})
			return amount(2, 1, 1e8, 0), t
		}},
		{"compare amounts < (scale 2, 10% negative)", "<", n, func() (val, val) {
			return amount(2, 1, 1e8, 0.10), amount(2, 1, 1e8, 0.10)
		}},
		{"compare amounts == (scale 2, 20% equal)", "==", n, func() (val, val) {
			x := amount(2, 1, 1e8, 0.10)
			if rng.Float64() < 0.2 {
				return x, x
			}
			return x, amount(2, 1, 1e8, 0.10)
		}},
	}
	for _, op := range []string{"+", "-", "*", "<"} {
		for _, qr := range [][2]int{{-15, 0}, {-30, 10}} {
			for _, sg := range []float64{0, 0.5} {
				qr, sg := qr, sg
				name := fmt.Sprintf("16-34 digits, one qExp in [%d,%d], %.0f%% negative", qr[0], qr[1], sg*100)
				if op == "*" {
					name = fmt.Sprintf("16-34 digits, qExps in [%d,%d], %.0f%% negative", qr[0], qr[1], sg*100)
				}
				work = append(work, workload{name, op, n, func() (val, val) {
					q := qr[0] + rng.IntN(qr[1]-qr[0]+1)
					x := val{neg: rng.Float64() < sg, c: digits(16 + rng.IntN(19)), q: q}
					y := val{neg: rng.Float64() < sg, c: digits(16 + rng.IntN(19)), q: q}
					if op == "*" {
						y.q = qr[0] + rng.IntN(qr[1]-qr[0]+1)
					}
					return x, y
				}})
			}
		}
	}

	fmt.Printf("%-62s %3s  %7s %7s %7s   (share sent to the general routine)\n", "workload", "op", "U", "N", "C")
	for _, w := range work {
		slow := map[string]int{}
		for i := 0; i < w.n; i++ {
			x, y := w.gen()
			for _, l := range layouts {
				if !run(l, w.op, x, y) {
					slow[l.name]++
				}
			}
		}
		fmt.Printf("%-62s %3s  %6.1f%% %6.1f%% %6.1f%%\n", w.name, w.op,
			100*float64(slow["U"])/float64(w.n), 100*float64(slow["N"])/float64(w.n), 100*float64(slow["C"])/float64(w.n))
	}

	stress()
	fmt.Printf("\nfast-path results checked against math/big: %d errors\n", errors)
	if errors != 0 {
		os.Exit(1)
	}
}

// stress runs every op of every layout on boundary operands: coefficients
// at the edges of each layout's fast paths, zeros of both signs, scales at
// the edges of the money form, and the specials.
func stress() {
	var cs []*big.Int
	for _, e := range []int{0, 1, 4, 31, 32, 55, 56, 58, 59, 60, 63, 64, 65, 111, 112} {
		p := new(big.Int).Lsh(big.NewInt(1), uint(e))
		cs = append(cs, p, new(big.Int).Sub(p, big.NewInt(1)), new(big.Int).Add(p, big.NewInt(1)))
	}
	cs = append(cs, big.NewInt(0), big.NewInt(10), max34, new(big.Int).Sub(max34, big.NewInt(1)))
	var vs []val
	for _, c := range cs {
		if c.Cmp(max34) > 0 {
			continue
		}
		for _, q := range []int{0, -1, -2, -7, -8, -15, -16, 1, -6176, 6111} {
			vs = append(vs, val{c: c, q: q}, val{neg: true, c: c, q: q})
		}
	}
	vs = append(vs, val{c: big.NewInt(0), special: 1}, val{neg: true, c: big.NewInt(0), special: 1},
		val{c: big.NewInt(0), special: 2})
	count := 0
	for _, x := range vs {
		for _, y := range vs {
			for _, op := range []string{"+", "-", "*", "==", "<"} {
				for _, l := range layouts {
					run(l, op, x, y)
					count++
				}
			}
		}
	}
	fmt.Printf("\nstress: %d boundary operations (%d operands)\n", count, len(vs))
}
