// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

import (
	"fmt"
	"internal/bid"
	"maps"
	"math"
	"math/big"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// minExamples is the number of worked examples the contract had when this
// test was written. Examples may be added; losing one fails the test, so
// that a careless edit to the package documentation cannot silently drop
// part of the contract.
const minExamples = 699

var exampleRE = regexp.MustCompile(`^//\t([A-Z][A-Za-z0-9]*)\((.*)\) = (.*)$`)

type example struct {
	line int
	text string
	name string
	args []string
	want string
}

// readExamples returns the worked examples in the package documentation.
// Every tab-indented line of the "Worked examples" section must be an
// example except the two lines of the format template, whose function
// name is "Function".
func readExamples(t *testing.T) []example {
	src, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	var exs []example
	inSection := false
	for i, line := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(line, "// # ") {
			inSection = line == "// # Worked examples"
			continue
		}
		m := exampleRE.FindStringSubmatch(line)
		if m == nil {
			if inSection && strings.HasPrefix(line, "//\t") {
				t.Errorf("doc.go:%d: unparsed example line: %s", i+1, line)
			}
			continue
		}
		if m[1] == "Function" {
			continue
		}
		exs = append(exs, example{line: i + 1, text: strings.TrimPrefix(line, "//\t"), name: m[1], args: splitArgs(m[2]), want: m[3]})
	}
	if len(exs) < minExamples {
		t.Errorf("found %d worked examples in doc.go, want at least %d", len(exs), minExamples)
	}
	return exs
}

// splitArgs splits on commas outside double quotes.
func splitArgs(s string) []string {
	var args []string
	quoted := false
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			quoted = !quoted
		case '\\':
			if quoted {
				i++
			}
		case ',':
			if !quoted {
				args = append(args, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(args, strings.TrimSpace(s[start:]))
}

// TestContractExamples checks every worked example of the contract against
// the reference model, and against the kernel for the functions the kernel
// implements.
func TestContractExamples(t *testing.T) {
	exs := readExamples(t)
	t.Run("reference", func(t *testing.T) {
		for _, ex := range exs {
			if err := checkExample(reference{}, ex); err != nil {
				t.Errorf("doc.go:%d: %s\n\t%v", ex.line, ex.text, err)
			}
		}
	})
	t.Run("kernel", func(t *testing.T) {
		missing := map[string]int{}
		skipped := 0
		for _, ex := range exs {
			var err error
			implemented, perr := try(func() { err = checkExample(kernel{}, ex) })
			switch {
			case perr != nil:
				t.Errorf("doc.go:%d: %s\n\tkernel %v", ex.line, ex.text, perr)
			case !implemented:
				missing[ex.name]++
				skipped++
			case err != nil:
				t.Errorf("doc.go:%d: %s\n\tkernel: %v", ex.line, ex.text, err)
			}
		}
		if skipped > 0 {
			t.Skipf("%d of %d examples not checked: the kernel does not implement %s", skipped, len(exs), sortedKeys(missing))
		}
	})
}

func sortedKeys(m map[string]int) string {
	return strings.Join(slices.Sorted(maps.Keys(m)), ", ")
}

var modes = map[string]bid.Mode{
	"ToNearestEven":  bid.ToNearestEven,
	"ToNearestAway":  bid.ToNearestAway,
	"TowardZero":     bid.TowardZero,
	"TowardPositive": bid.TowardPositive,
	"TowardNegative": bid.TowardNegative,
}

var modeNames = []string{"ToNearestEven", "ToNearestAway", "TowardZero", "TowardPositive", "TowardNegative"}

func modeName(m bid.Mode) string {
	if int(m) < len(modeNames) {
		return modeNames[m]
	}
	return fmt.Sprintf("Mode(%d)", m)
}

var statusNames = []struct {
	st   bid.Status
	name string
}{
	{bid.Inexact, "Inexact"},
	{bid.Overflow, "Overflow"},
	{bid.Underflow, "Underflow"},
	{bid.Invalid, "Invalid"},
	{bid.Syntax, "Syntax"},
}

func statusString(st bid.Status) string {
	if st == bid.Exact {
		return "Exact"
	}
	var names []string
	for _, s := range statusNames {
		if st&s.st != 0 {
			names = append(names, s.name)
			st &^= s.st
		}
	}
	if st != 0 {
		names = append(names, fmt.Sprintf("Status(%#x)", uint8(st)))
	}
	return strings.Join(names, "|")
}

func parseStatus(s string) (bid.Status, error) {
	if s == "Exact" {
		return bid.Exact, nil
	}
	var st bid.Status
	for _, name := range strings.Split(s, "|") {
		found := false
		for _, n := range statusNames {
			if n.name == name {
				st |= n.st
				found = true
			}
		}
		if !found {
			return 0, fmt.Errorf("unknown Status %q", name)
		}
	}
	return st, nil
}

// display writes x in the character form, and writes the NaNs that only
// the sign operations produce as -NaN, sNaN and -sNaN, as the contract's
// examples do.
func display(f *format, x bits) string {
	v := f.decode(x)
	if v.kind != notANumber {
		return f.format(x)
	}
	s := "NaN"
	if v.signaling {
		s = "sNaN"
	}
	if v.neg {
		s = "-" + s
	}
	return s
}

// operand reads a decimal operand of an example: the character form, or
// one of the NaN forms of display. A numeral must denote a value of f
// exactly; a written exponent out of range reads as its clamped cohort
// member, as Parse reads it.
func operand(f *format, s string) (bits, error) {
	neg := strings.HasPrefix(s, "-")
	switch strings.TrimPrefix(s, "-") {
	case "NaN", "sNaN":
		t := uint64(0x1F) << 58
		if strings.HasSuffix(s, "sNaN") {
			t |= 1 << 57
		}
		var x bits
		if f.width == 64 {
			x = bits{Lo: t}
		} else {
			x = bits{Hi: t}
		}
		return f.withSign(x, neg), nil
	}
	x, st := f.parse(s, bid.ToNearestEven)
	if st != bid.Exact {
		return bits{}, fmt.Errorf("operand %s is not exactly a %s (%s)", s, f.name, statusString(st))
	}
	return x, nil
}

func floatBits64(s string) (uint64, error) {
	if s == "NaN" {
		return 0x7FF8000000000000, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	return math.Float64bits(v), err
}

func floatBits32(s string) (uint32, error) {
	if s == "NaN" {
		return 0x7FC00000, nil
	}
	v, err := strconv.ParseFloat(s, 32)
	return math.Float32bits(float32(v)), err
}

var nameRE = regexp.MustCompile(`^(.*?)(64|128)$`)

// checkExample evaluates ex with im and compares the result with the
// example's.
func checkExample(im impl, ex example) error {
	base, f := ex.name, dec64
	if m := nameRE.FindStringSubmatch(ex.name); m != nil {
		base = m[1]
		if m[2] == "128" {
			f = dec128
		}
	}
	args := ex.args
	want := ex.want
	// Split off a trailing Status, which every Status-returning function
	// writes after a comma.
	var wantSt bid.Status
	splitStatus := func() error {
		i := strings.LastIndex(want, ", ")
		if i < 0 {
			return fmt.Errorf("missing Status")
		}
		st, err := parseStatus(want[i+2:])
		want, wantSt = want[:i], st
		return err
	}
	mode := func(i int) (bid.Mode, error) {
		if i >= len(args) {
			return 0, fmt.Errorf("missing mode argument")
		}
		m, ok := modes[args[i]]
		if !ok {
			return 0, fmt.Errorf("unknown mode %q", args[i])
		}
		return m, nil
	}
	dec := func(i int) (bits, error) {
		if i >= len(args) {
			return bits{}, fmt.Errorf("missing argument %d", i+1)
		}
		return operand(f, args[i])
	}
	var got string
	var gotSt bid.Status
	hasSt := false

	var err error
	var x, y, z bits
	var m bid.Mode
	switch base {
	case "Add", "Sub", "Mul", "Div", "Quantize":
		if x, err = dec(0); err == nil {
			if y, err = dec(1); err == nil {
				m, err = mode(2)
			}
		}
		if err != nil {
			return err
		}
		var r bits
		switch base {
		case "Add":
			r = im.add(f, x, y, m)
		case "Sub":
			r = im.sub(f, x, y, m)
		case "Mul":
			r = im.mul(f, x, y, m)
		case "Div":
			r = im.div(f, x, y, m)
		case "Quantize":
			r = im.quantize(f, x, y, m)
		}
		got = display(f, r)
	case "FMA":
		if x, err = dec(0); err == nil {
			if y, err = dec(1); err == nil {
				if z, err = dec(2); err == nil {
					m, err = mode(3)
				}
			}
		}
		if err != nil {
			return err
		}
		got = display(f, im.fma(f, x, y, z, m))
	case "Rem", "Mod", "Min", "Max", "CopySign":
		if x, err = dec(0); err == nil {
			y, err = dec(1)
		}
		if err != nil {
			return err
		}
		var r bits
		switch base {
		case "Rem":
			r = im.rem(f, x, y)
		case "Mod":
			r = im.mod(f, x, y)
		case "Min":
			r = im.min(f, x, y)
		case "Max":
			r = im.max(f, x, y)
		case "CopySign":
			r = im.copySign(f, x, y)
		}
		got = display(f, r)
	case "Equal", "Less", "LessEqual", "TotalOrder", "SameQuantum":
		if x, err = dec(0); err == nil {
			y, err = dec(1)
		}
		if err != nil {
			return err
		}
		var r bool
		switch base {
		case "Equal":
			r = im.equal(f, x, y)
		case "Less":
			r = im.less(f, x, y)
		case "LessEqual":
			r = im.lessEqual(f, x, y)
		case "TotalOrder":
			r = im.totalOrder(f, x, y)
		case "SameQuantum":
			r = im.sameQuantum(f, x, y)
		}
		got = strconv.FormatBool(r)
	case "Compare":
		if x, err = dec(0); err == nil {
			y, err = dec(1)
		}
		if err != nil {
			return err
		}
		c, ok := im.compare(f, x, y)
		got = fmt.Sprintf("%d, %t", c, ok)
	case "RoundToIntegral":
		if x, err = dec(0); err == nil {
			m, err = mode(1)
		}
		if err != nil {
			return err
		}
		got = display(f, im.roundToIntegral(f, x, m))
	case "Reduce", "Neg", "Abs":
		if x, err = dec(0); err != nil {
			return err
		}
		var r bits
		switch base {
		case "Reduce":
			r = im.reduce(f, x)
		case "Neg":
			r = im.neg(f, x)
		case "Abs":
			r = im.abs(f, x)
		}
		got = display(f, r)
	case "Widen":
		if x, err = operand(dec64, args[0]); err != nil {
			return err
		}
		got = display(dec128, im.widen(x))
	case "Narrow":
		if x, err = operand(dec128, args[0]); err == nil {
			m, err = mode(1)
		}
		if err != nil {
			return err
		}
		got = display(dec64, im.narrow(x, m))
	case "Int64To", "Uint64To":
		m = bid.ToNearestEven
		if len(args) > 1 {
			if m, err = mode(1); err != nil {
				return err
			}
		}
		if base == "Int64To" {
			n, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return err
			}
			got = display(f, im.int64To(f, n, m))
		} else {
			n, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return err
			}
			got = display(f, im.uint64To(f, n, m))
		}
	case "Int64From", "Uint64From":
		if x, err = dec(0); err == nil {
			m, err = mode(1)
		}
		if err == nil {
			err = splitStatus()
		}
		if err != nil {
			return err
		}
		hasSt = true
		if base == "Int64From" {
			n, st := im.int64From(f, x, m)
			got, gotSt = strconv.FormatInt(n, 10), st
		} else {
			n, st := im.uint64From(f, x, m)
			got, gotSt = strconv.FormatUint(n, 10), st
		}
	case "Float64To", "Float32To":
		if m, err = mode(1); err != nil {
			return err
		}
		if base == "Float64To" {
			v, err := strconv.ParseFloat(args[0], 64)
			if err != nil {
				return err
			}
			got = display(f, im.float64To(f, v, m))
		} else {
			v, err := strconv.ParseFloat(args[0], 32)
			if err != nil {
				return err
			}
			got = display(f, im.float32To(f, float32(v), m))
		}
	case "Float64From", "Float32From":
		if x, err = dec(0); err == nil {
			m, err = mode(1)
		}
		if err != nil {
			return err
		}
		if base == "Float64From" {
			wb, err := floatBits64(want)
			if err != nil {
				return err
			}
			r := im.float64From(f, x, m)
			if math.Float64bits(r) != wb {
				return fmt.Errorf("got %v (%#016x), want %s (%#016x)", r, math.Float64bits(r), want, wb)
			}
		} else {
			wb, err := floatBits32(want)
			if err != nil {
				return err
			}
			r := im.float32From(f, x, m)
			if math.Float32bits(r) != wb {
				return fmt.Errorf("got %v (%#08x), want %s (%#08x)", r, math.Float32bits(r), want, wb)
			}
		}
		return nil
	case "Parse":
		s, err := strconv.Unquote(args[0])
		if err != nil {
			return err
		}
		if m, err = mode(1); err == nil {
			err = splitStatus()
		}
		if err != nil {
			return err
		}
		r, st := im.parse(f, s, m)
		got, gotSt, hasSt = display(f, r), st, true
	case "Round":
		if len(args) != 5 {
			return fmt.Errorf("Round takes 5 arguments")
		}
		neg, err := strconv.ParseBool(args[0])
		if err != nil {
			return err
		}
		c, ok := new(big.Int).SetString(args[1], 10)
		if !ok || c.Sign() < 0 || c.BitLen() > 128 {
			return fmt.Errorf("bad coefficient %q", args[1])
		}
		exp, err := strconv.Atoi(args[2])
		if err != nil {
			return err
		}
		sticky, err := strconv.ParseBool(args[3])
		if err != nil {
			return err
		}
		if m, err = mode(4); err == nil {
			err = splitStatus()
		}
		if err != nil {
			return err
		}
		r, st := im.round(f, neg, toUint128(c), exp, sticky, m)
		got, gotSt, hasSt = display(f, r), st, true
	default:
		return fmt.Errorf("no evaluator for %s", ex.name)
	}
	if got != want || hasSt && gotSt != wantSt {
		if hasSt {
			return fmt.Errorf("got %s, %s", got, statusString(gotSt))
		}
		return fmt.Errorf("got %s", got)
	}
	return nil
}
