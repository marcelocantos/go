// check reads gen.py's cases on standard input, applies the compiled Go
// operators and math/decimal functions to them, and reports every result
// that differs from Python's, by operation. It also round-trips every
// finite operand through strconv's 'f' and 'g' formats and checks that the
// value and its quantum come back. Build at each -d=ubdlevel (and with
// -tags w64 for decimal64) to check the compiler's inline paths and the
// kernel behind them.
package main

import (
	"bufio"
	"fmt"
	"math/decimal"
	"os"
	"sort"
	"strconv"
	"strings"
)

func parse(s string) T {
	d, err := strconv.ParseDecimal(s, bits)
	if err != nil {
		panic(fmt.Sprintf("parse %q: %v", s, err))
	}
	return T(d)
}

func b(x T) string { return strconv.FormatDecimal(decimal128(x), 'b', -1, bits) }

//go:noinline
func add(x, y T) T { return x + y }

//go:noinline
func sub(x, y T) T { return x - y }

//go:noinline
func mul(x, y T) T { return x * y }

//go:noinline
func div(x, y T) T { return x / y }

//go:noinline
func neg(x T) T { return -x }

//go:noinline
func eq(x, y T) bool { return x == y }

//go:noinline
func ne(x, y T) bool { return x != y }

//go:noinline
func lt(x, y T) bool { return x < y }

//go:noinline
func le(x, y T) bool { return x <= y }

//go:noinline
func gt(x, y T) bool { return x > y }

//go:noinline
func ge(x, y T) bool { return x >= y }

//go:noinline
func mn(x, y T) T { return min(x, y) }

//go:noinline
func mx(x, y T) T { return max(x, y) }

//go:noinline
func toint(x T) int64 { return int64(x) }

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	count := map[string]int{}
	bad := map[string]int{}
	shown := map[string]int{}
	report := func(op, line, got string) {
		bad[op]++
		if shown[op]++; shown[op] <= 5 {
			fmt.Printf("MISMATCH %s: got %s\n", line, got)
		}
	}
	// roundTrip formats x and parses it back: 'g', the canonical form, must
	// give back the value and its quantum; 'f' writes zeros for a positive
	// exponent (4.35E+3 is "4350"), so it is checked at exponents <= 0.
	roundTrip := func(line string, x T) {
		_, _, _, exp, _ := decimal.Decompose(x)
		for _, f := range []byte{'f', 'g'} {
			if f == 'f' && exp > 0 {
				continue
			}
			op := "roundtrip-" + string(f)
			count[op]++
			s := strconv.FormatDecimal(decimal128(x), f, -1, bits)
			if back := parse(s); b(back) != b(x) {
				report(op, line, s+" -> "+b(back))
			}
		}
	}
	for in.Scan() {
		line := in.Text()
		f := strings.Split(line, "\t")
		op, want := f[0], f[5]
		x := parse(f[1])
		var y, z T
		if f[2] != "-" {
			y = parse(f[2])
		}
		if f[3] != "-" {
			z = parse(f[3])
		}
		var got string
		switch op {
		case "add":
			got = b(add(x, y))
		case "sub":
			got = b(sub(x, y))
		case "mul":
			got = b(mul(x, y))
		case "div":
			got = b(div(x, y))
		case "neg":
			got = b(neg(x))
		case "eq":
			got = strconv.FormatBool(eq(x, y))
		case "ne":
			got = strconv.FormatBool(ne(x, y))
		case "lt":
			got = strconv.FormatBool(lt(x, y))
		case "le":
			got = strconv.FormatBool(le(x, y))
		case "gt":
			got = strconv.FormatBool(gt(x, y))
		case "ge":
			got = strconv.FormatBool(ge(x, y))
		case "min":
			got = b(mn(x, y))
		case "max":
			got = b(mx(x, y))
		case "quantize":
			got = b(decimal.Quantize(x, y))
		case "quantizemode":
			m, _ := strconv.Atoi(f[4])
			got = b(decimal.QuantizeMode(x, y, decimal.RoundingMode(m)))
		case "trunc":
			got = b(decimal.Trunc(x))
		case "floor":
			got = b(decimal.Floor(x))
		case "ceil":
			got = b(decimal.Ceil(x))
		case "round":
			got = b(decimal.Round(x))
		case "roundeven":
			got = b(decimal.RoundToEven(x))
		case "fma":
			got = b(decimal.FMA(x, y, z))
		case "sqrt":
			got = b(decimal.Sqrt(x))
		case "rem":
			got = b(decimal.Remainder(x, y))
		case "mod":
			got = b(decimal.Mod(x, y))
		case "toint":
			got = strconv.FormatInt(toint(x), 10)
		case "fmtf":
			got = strconv.FormatDecimal(decimal128(x), 'f', -1, bits)
			roundTrip(line, x)
		default:
			panic("unknown op " + op)
		}
		count[op]++
		if got != want {
			report(op, line, got)
		}
	}
	if err := in.Err(); err != nil {
		panic(err)
	}
	var ops []string
	for op := range count {
		ops = append(ops, op)
	}
	sort.Strings(ops)
	total, totalBad := 0, 0
	for _, op := range ops {
		total += count[op]
		totalBad += bad[op]
		if bad[op] > 0 {
			fmt.Printf("%-14s %7d cases, %d mismatches\n", op, count[op], bad[op])
		}
	}
	fmt.Printf("TOTAL %d cases, %d mismatches, decimal%d\n", total, totalBad, bits)
}
