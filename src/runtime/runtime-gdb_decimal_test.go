// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime_test

import (
	"bytes"
	"debug/dwarf"
	"debug/elf"
	"fmt"
	"internal/bid"
	"internal/testenv"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The decimal printer in runtime-gdb.py undoes the XOR of the memory form
// with the encoding of 0E+0 and decodes the BID or DPD encoding itself.
// TestGdbDecimal checks it in gdb, on the host's encoding; the tests below
// check its decoder on both encodings without gdb, and the constants that
// tell it the encoding on each GOARCH.

var decimalGdbSource = `
package main

import (
	"math/decimal"
	"runtime"
)

type pair struct {
	D64  decimal64
	D128 decimal128
}

var (
	g64   decimal64  = 1.50
	g128  decimal128 = 1.50
	gzero decimal64
	gpair = pair{D64: 2.50, D128: 3.250}
	gptr  = &gpair
	garr  = [2]decimal64{1.5, 1.50}
	gsl   = []decimal128{1.5, 1.50}
	sink  *pair
)

//go:noinline
func entry(a decimal64, b decimal128) (decimal64, decimal128) {
	return a + a, b + b
}

func main() {
	var z64 decimal64
	var z128 decimal128
	nz64, nz128 := decimal.Copysign(z64, -1), decimal.Copysign(z128, -1)
	sp64 := [5]decimal64{z64, nz64, decimal.NaN[decimal64](), decimal.Inf[decimal64](1), decimal.Inf[decimal64](-1)}
	sp128 := [5]decimal128{z128, nz128, decimal.NaN[decimal128](), decimal.Inf[decimal128](1), decimal.Inf[decimal128](-1)}
	p := &pair{D64: 7.70, D128: 8.800}
	sink = p
	m := map[string]decimal128{"k": 4.20}
	sl := []decimal64{9.90, 9.9}
	s := pair{D64: 5.5, D128: 6.60}
	r64, r128 := entry(1.50, 1.50)
	runtime.KeepAlive(nz64) // set breakpoint here
	runtime.KeepAlive(nz128)
	runtime.KeepAlive(&sp64)
	runtime.KeepAlive(&sp128)
	runtime.KeepAlive(p)
	runtime.KeepAlive(m)
	runtime.KeepAlive(sl)
	runtime.KeepAlive(&s)
	runtime.KeepAlive(r64)
	runtime.KeepAlive(r128)
	runtime.KeepAlive(g64)
	runtime.KeepAlive(g128)
	runtime.KeepAlive(gzero)
	runtime.KeepAlive(gptr)
	runtime.KeepAlive(&garr)
	runtime.KeepAlive(gsl)
}
`

// decimalGdbEntryChecks and decimalGdbChecks are the expressions printed
// at the entry of main.entry and then at the breakpoint in main, and the
// values gdb should show for them. In the optimized build gdb cannot find
// some variables, or finds a register that has been reused, as it does for
// variables of any type, so only the checks marked opt run there.
type decimalGdbCheck struct {
	expr, want string
	opt        bool
}

var decimalGdbEntryChecks = []decimalGdbCheck{
	// Parameters, in registers at the entry.
	{"a", "1.50", true},
	{"b", "1.50", true},
}

var decimalGdbChecks = []decimalGdbCheck{
	// Globals.
	{"'main.g64'", "1.50", true},
	{"'main.g128'", "1.50", true},
	{"'main.gzero'", "0", true},
	{"'main.gpair'", "{D64 = 2.50, D128 = 3.250}", true},
	{"'main.gpair'.D128", "3.250", true},
	{"*'main.gptr'", "{D64 = 2.50, D128 = 3.250}", true},
	{"'main.gptr'.D64", "2.50", true},
	{"'main.garr'", "{1.5, 1.50}", false},
	{"'main.garr'[1]", "1.50", false},
	{"'main.gsl'", "[]decimal128 = {1.5, 1.50}", true},
	// Locals.
	{"z64", "0", false},
	{"z128", "0", false},
	{"nz64", "-0", true},
	{"nz128", "-0", true},
	{"sp64", "{0, -0, NaN, Infinity, -Infinity}", true},
	{"sp128", "{0, -0, NaN, Infinity, -Infinity}", true},
	{"sp128[4]", "-Infinity", true},
	{"*p", "{D64 = 7.70, D128 = 8.800}", true},
	{"p.D128", "8.800", true},
	{"m", `map[string]decimal128 = {["k"] = 4.20}`, false},
	{"sl", "[]decimal64 = {9.90, 9.9}", false},
	{"s", "{D64 = 5.5, D128 = 6.60}", true},
	// Results, in registers after the call.
	{"r64", "3.00", true},
	{"r128", "3.00", true},
}

func TestGdbDecimal(t *testing.T) {
	checkGdbEnvironment(t)
	t.Parallel()
	checkGdbVersion(t)
	checkGdbPython(t)
	checkPtraceScope(t)
	if major := gdbMajorVersion(t); major < 14 {
		// Before gdb 14 a Python printer cannot read a value held in
		// a register; the printer says so instead of decoding it.
		t.Skipf("skipping: gdb %d is older than 14", major)
	}

	dir := t.TempDir()
	src := []byte(decimalGdbSource)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), src, 0644); err != nil {
		t.Fatal(err)
	}
	bp := 0
	for i, l := range bytes.Split(src, []byte("\n")) {
		if bytes.Contains(l, []byte("set breakpoint here")) {
			bp = i + 1
		}
	}

	// The optimized build keeps values in registers; the unoptimized
	// one keeps every variable in its stack slot.
	for _, build := range []struct {
		name, gcflags string
		opt           bool
	}{
		{"optimized", "", true},
		{"unoptimized", "-N -l", false},
	} {
		t.Run(build.name, func(t *testing.T) {
			exe := filepath.Join(dir, build.name+".exe")
			cmd := exec.Command(testenv.GoToolPath(t), "build", "-gcflags=all="+build.gcflags, "-o", exe, "main.go")
			cmd.Dir = dir
			if out, err := testenv.CleanCmdEnv(cmd).CombinedOutput(); err != nil {
				t.Fatalf("building source %v\n%s", err, out)
			}

			args := []string{"-nx", "-q", "--batch",
				"-iex", "add-auto-load-safe-path " + filepath.Join(testenv.GOROOT(t), "src", "runtime"),
				"-ex", "set startup-with-shell off",
				"-ex", "set print thread-events off",
				"-ex", "set python print-stack full",
				"-ex", "break main.entry",
				"-ex", fmt.Sprintf("break main.go:%d", bp),
				"-ex", "run",
			}
			for _, c := range decimalGdbEntryChecks {
				args = append(args, "-ex", "echo BEGIN "+c.expr+"\n", "-ex", "print "+c.expr, "-ex", "echo END\n")
			}
			args = append(args, "-ex", "continue")
			for _, c := range decimalGdbChecks {
				args = append(args, "-ex", "echo BEGIN "+c.expr+"\n", "-ex", "print "+c.expr, "-ex", "echo END\n")
			}
			args = append(args, exe)
			gdbArgsFixup(args)
			got, err := exec.Command("gdb", args...).CombinedOutput()
			t.Logf("gdb output:\n%s", got)
			if err != nil {
				t.Fatalf("gdb exited with error: %v", err)
			}
			got = bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n"))
			blocks := map[string]string{}
			partRe := regexp.MustCompile(`(?ms)^BEGIN ([^\n]*)\n(.*?)\nEND`)
			for _, subs := range partRe.FindAllSubmatch(got, -1) {
				blocks[string(subs[1])] = string(subs[2])
			}
			valueRe := regexp.MustCompile(`^\$[0-9]+ = (.*)$`)
			for _, c := range append(decimalGdbEntryChecks, decimalGdbChecks...) {
				if build.opt && !c.opt {
					continue
				}
				m := valueRe.FindStringSubmatch(blocks[c.expr])
				if m == nil || m[1] != c.want {
					t.Errorf("print %s: got %q, want %q", c.expr, blocks[c.expr], "$N = "+c.want)
				}
			}
		})
	}
}

// gdbMajorVersion returns the major version of the gdb on PATH; it is
// called after checkGdbVersion, which skips when there is none.
func gdbMajorVersion(t *testing.T) int {
	out, err := exec.Command("gdb", "--version").CombinedOutput()
	if err != nil {
		t.Skipf("skipping: error executing gdb: %v", err)
	}
	m := regexp.MustCompile(`([0-9]+)\.[0-9]+`).FindSubmatch(out)
	if m == nil {
		t.Skipf("skipping: can't determine gdb version from\n%s", out)
	}
	major, _ := strconv.Atoi(string(m[1]))
	return major
}

// decoderHarness loads runtime-gdb.py with a stand-in for gdb's Python
// module, which provides just enough for the script to load, and prints
// decimal_memory_string for each line of input: the encodings of 0E+0
// (decimal64, then the decimal128 high word) and the words of a memory
// form, in memory order, all in hexadecimal.
const decoderHarness = `
import sys, types

gdb = types.ModuleType('gdb')
class error(Exception):
	pass
class Objfile:
	pretty_printers = []
class Function:
	def __init__(self, *args):
		pass
class Command:
	def __init__(self, *args):
		pass
def parse_and_eval(expr):
	raise error(expr)
gdb.error = error
gdb.Function = Function
gdb.Command = Command
gdb.parse_and_eval = parse_and_eval
gdb.current_objfile = lambda: Objfile()
gdb.objfiles = lambda: [Objfile()]
gdb.__getattr__ = lambda name: 0
sys.modules['gdb'] = gdb

script = sys.argv[1]
ns = {'__name__': 'runtime_gdb'}
exec(compile(open(script).read(), script, 'exec'), ns)
for line in sys.stdin:
	f = [int(w, 16) for w in line.split()]
	zero64, zero128hi, words = f[0], f[1], tuple(f[2:])
	enc = ns['decimal_encoding'](zero64, zero128hi)
	print(ns['decimal_memory_string'](words, enc, zero64, zero128hi))
`

// TestGdbDecimalDecoder checks the decoder of runtime-gdb.py against
// internal/bid's character form, on BID and DPD encodings of both widths:
// special values, canonical values of many quanta and arbitrary bit
// patterns, which include non-canonical encodings. It needs python3 but
// not gdb, so it covers the DPD path on every host.
func TestGdbDecimalDecoder(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("skipping: no python3")
	}
	if out, err := exec.Command(python, "-c", "import sys; assert sys.version_info >= (3, 7)").CombinedOutput(); err != nil {
		t.Skipf("skipping: python3 3.7 or later is not usable: %v\n%s", err, out)
	}
	script := filepath.Join(testenv.GOROOT(t), "src", "runtime", "runtime-gdb.py")

	type vector struct {
		name  string
		input string // as decoderHarness reads it
		want  string
	}
	var vectors []vector
	add64 := func(name string, dpd bool, enc uint64) {
		zero, zeroHi, b := bid.Zero64, bid.Zero128Hi, enc
		if dpd {
			zero, zeroHi, b = bid.DPDZero64, bid.DPDZero128Hi, bid.BIDFromDPD64(enc)
		}
		vectors = append(vectors, vector{
			name:  fmt.Sprintf("%s decimal64 dpd=%v %#016x", name, dpd, enc),
			input: fmt.Sprintf("%x %x %x", zero, zeroHi, enc^zero),
			want:  string(bid.AppendSci64(nil, b)),
		})
	}
	add128 := func(name string, dpd bool, enc bid.Uint128) {
		zero, zeroHi, b := bid.Zero64, bid.Zero128Hi, enc
		if dpd {
			zero, zeroHi, b = bid.DPDZero64, bid.DPDZero128Hi, bid.BIDFromDPD128(enc)
		}
		vectors = append(vectors, vector{
			name:  fmt.Sprintf("%s decimal128 dpd=%v %#016x%016x", name, dpd, enc.Hi, enc.Lo),
			input: fmt.Sprintf("%x %x %x %x", zero, zeroHi, enc.Lo, enc.Hi^zeroHi),
			want:  string(bid.AppendSci128(nil, b)),
		})
	}

	r := rand.New(rand.NewPCG(1, 2))
	for _, dpd := range []bool{false, true} {
		// Encodings in BID, converted to DPD for the DPD vectors.
		to64 := func(x uint64) uint64 {
			if dpd {
				return bid.DPDFromBID64(x)
			}
			return x
		}
		to128 := func(x bid.Uint128) bid.Uint128 {
			if dpd {
				return bid.DPDFromBID128(x)
			}
			return x
		}
		special := []struct {
			name string
			x64  uint64
			x128 bid.Uint128
		}{
			{"zero", bid.Zero64, bid.Uint128{Hi: bid.Zero128Hi}},
			{"negative zero", bid.Zero64 | bid.SignMask64, bid.Uint128{Hi: bid.Zero128Hi | bid.SignMask128Hi}},
			{"NaN", bid.NaN64, bid.Uint128{Hi: bid.NaN128Hi}},
			{"signaling NaN", bid.SNaN64 | 123, bid.Uint128{Hi: bid.SNaN128Hi, Lo: 123}},
			{"infinity", bid.Inf64, bid.Uint128{Hi: bid.Inf128Hi}},
			{"negative infinity", bid.Inf64 | bid.SignMask64, bid.Uint128{Hi: bid.Inf128Hi | bid.SignMask128Hi}},
		}
		for _, s := range special {
			add64(s.name, dpd, to64(s.x64))
			add128(s.name, dpd, to128(s.x128))
		}
		// Canonical values: 1.5 and 1.50, every leading digit (which DPD
		// keeps in the combination field), both BID forms, the extreme
		// exponents and plain and exponential notation.
		for _, v := range []struct {
			neg   bool
			coeff uint64
			exp   int
		}{
			{false, 15, -1}, {false, 150, -2}, {true, 150, -2}, {false, 1, 3},
			{false, 1, -7}, {false, 1, -6}, {false, 123456, -10}, {false, 7, 0},
			{false, bid.MaxCoeff64, bid.MaxExp64}, {true, 1, bid.MinExp64},
			{false, 9_000_000_000_000_000, 0}, {false, 8_999_999_999_999_999, -5},
		} {
			x, ok := bid.Pack64(v.neg, v.coeff, v.exp)
			if !ok {
				t.Fatalf("Pack64(%v, %d, %d) failed", v.neg, v.coeff, v.exp)
			}
			add64("value", dpd, to64(x))
			y, ok := bid.Pack128(v.neg, bid.Uint128{Lo: v.coeff}, v.exp*10)
			if !ok {
				t.Fatalf("Pack128(%v, %d, %d) failed", v.neg, v.coeff, v.exp*10)
			}
			add128("value", dpd, to128(y))
		}
		add128("max", dpd, to128(bid.Uint128{Hi: bid.MaxCoeff128Hi | uint64(bid.MaxExp128+bid.Bias128)<<bid.ExpShift128, Lo: bid.MaxCoeff128Lo}))
		for lead := range uint64(10) {
			x, _ := bid.Pack64(false, lead*1_000_000_000_000_000+123_456_789_012_345, -3)
			add64("leading digit", dpd, to64(x))
		}
		// Arbitrary bit patterns, in the encoding itself.
		for range 2000 {
			add64("random", dpd, r.Uint64())
			add128("random", dpd, bid.Uint128{Hi: r.Uint64(), Lo: r.Uint64()})
		}
	}
	// Every declet, in the trailing field of a DPD decimal64.
	for d := range uint64(1024) {
		add64("declet", true, bid.DPDZero64|d<<20|d)
	}

	var input strings.Builder
	for _, v := range vectors {
		input.WriteString(v.input + "\n")
	}
	cmd := exec.Command(python, "-I", "-c", decoderHarness, script)
	cmd.Stdin = strings.NewReader(input.String())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("python3: %v\n%s", err, stderr.Bytes())
	}
	got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(got) != len(vectors) {
		t.Fatalf("got %d lines, want %d\n%s", len(got), len(vectors), stderr.Bytes())
	}
	fails := 0
	for i, v := range vectors {
		if got[i] != v.want {
			t.Errorf("%s: got %q, want %q", v.name, got[i], v.want)
			if fails++; fails > 20 {
				t.Fatal("too many failures")
			}
		}
	}
	t.Logf("%d vectors", len(vectors))
}

// TestGdbDecimalZeroConsts checks that the DWARF of a linux binary records
// the encodings of 0E+0 that runtime-gdb.py reads, and that they are the
// DPD ones on ppc64le and the BID ones elsewhere.
func TestGdbDecimalZeroConsts(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	if testing.Short() {
		t.Skip("skipping in short mode: builds the runtime for three targets")
	}
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("package main\n\nfunc main() { var d decimal64 = 1.50; println(d) }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		goarch         string
		zero64, zeroHi uint64
	}{
		{"amd64", bid.Zero64, bid.Zero128Hi},
		{"arm64", bid.Zero64, bid.Zero128Hi},
		{"ppc64le", bid.DPDZero64, bid.DPDZero128Hi},
	} {
		t.Run(tc.goarch, func(t *testing.T) {
			exe := filepath.Join(dir, tc.goarch+".exe")
			cmd := exec.Command(testenv.GoToolPath(t), "build", "-o", exe, "main.go")
			cmd.Dir = dir
			cmd = testenv.CleanCmdEnv(cmd)
			cmd.Env = append(cmd.Env, "GOOS=linux", "GOARCH="+tc.goarch, "CGO_ENABLED=0")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("building for linux/%s: %v\n%s", tc.goarch, err, out)
			}
			f, err := elf.Open(exe)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			d, err := f.DWARF()
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]uint64{
				"runtime._DecimalZero64":    tc.zero64,
				"runtime._DecimalZero128Hi": tc.zeroHi,
			}
			got := map[string]uint64{}
			rd := d.Reader()
			for {
				e, err := rd.Next()
				if err != nil {
					t.Fatal(err)
				}
				if e == nil {
					break
				}
				if e.Tag != dwarf.TagConstant {
					continue
				}
				name, _ := e.Val(dwarf.AttrName).(string)
				if _, ok := want[name]; !ok {
					continue
				}
				v, ok := e.Val(dwarf.AttrConstValue).(int64)
				if !ok {
					t.Fatalf("%s: DW_AT_const_value is %T, want int64", name, e.Val(dwarf.AttrConstValue))
				}
				got[name] = uint64(v)
			}
			for name, w := range want {
				if g, ok := got[name]; !ok {
					t.Errorf("no DWARF constant %s", name)
				} else if g != w {
					t.Errorf("%s = %#x, want %#x", name, g, w)
				}
			}
		})
	}
}
