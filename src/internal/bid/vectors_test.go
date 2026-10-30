// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// This file is a local, throwaway smoke check of the kernel and of the
// reference model against two published sets of decimal test vectors: the
// IBM decTest files (General Decimal Arithmetic test cases) and the Intel
// Decimal Floating-Point Math Library's readtest.in. It is not part of the
// package's tests and nothing else depends on it: it is one self-contained
// file, run by the developer, and it is deleted before the CL series is cut
// (the ratchet that enforces this is target T1.6.2).
//
// # Where the vectors come from
//
// The vectors are not in the Go tree, in the go-decimal-proposal repository
// or in CI, and no tool copies, vendors or fetches them. They are read from
// directories on the developer's own machine named by environment
// variables:
//
//	BID_DECTEST_DIR     directory holding the *.decTest files
//	BID_INTEL_READTEST  directory holding readtest.in, or the file itself
//
// With a variable unset the corresponding part of TestPublishedVectors
// skips, so this file does nothing in all.bash or in CI.
//
// # Licence of each set, and why the sets stay out of tree
//
// IBM decTest. Every *.decTest file begins with the notice "Copyright (c)
// IBM Corporation, 1981, 2008. All rights reserved." (the years vary by
// file). The header adds that the cases are experimental, offered as-is
// and may contain errors, and asks readers to send corrections to the
// author; it grants no licence to copy, modify or redistribute the files.
// Source: the header comment (first twenty lines) of each file, as shipped
// in version 2.59 of the test cases, for example ddAdd.decTest. "All
// rights reserved" with no grant means the files cannot be committed to
// this repository, which must contain only material under the Go licence
// or a compatible grant.
//
// Intel readtest.in. The Intel Decimal Floating-Point Math Library
// (IntelRDFPMathLib), which ships TESTS/readtest.in, is distributed under a
// BSD-style licence: redistribution and use in source and binary forms,
// with or without modification, are permitted provided that the copyright
// notice, the list of conditions and the disclaimer are retained. This
// description follows the text of target T1.2.2; it was NOT checked against
// the licence file or the readtest.in header of an actual library release,
// because the file is not on the machine this runner was written on, and it
// must be checked before it is relied on. Even a permissive licence obliges
// the tree to carry Intel's notice with a copy of the file, and readtest.in
// is several megabytes of generated data, which is not wanted in the Go
// repository.
//
// So both sets stay out of tree: decTest because nothing licenses its
// redistribution, Intel readtest.in because its notice would have to travel
// with a very large generated file. The runner reads them in place.
//
// # What runs
//
// decTest. The runner parses the directives precision, rounding,
// maxexponent, minexponent, clamp, extended (case-insensitively, as the
// files mix cases) and test lines "id op operand... -> result condition...",
// with quoted operands. Directives otherwise ignored: version and dectest
// (the runner scans the whole directory itself, so it does not follow
// includes). A vector runs only in a context that is decimal64
// (precision 16, maxexponent 384, minexponent -383) or decimal128 (34,
// 6144, -6143) with extended 1, because those are the contexts whose
// arithmetic is the kernel's. Clamp 1 is the kernel's exponent clamping; a
// clamp 0 vector differs from it only in how an exponent above the format's
// maximum is written and kept, so a clamp 0 vector runs unless an operand or
// the result names such an exponent. The dd* files are decDouble (decimal64)
// and the dq* files decQuad (decimal128); general-arithmetic files such as
// add.decTest contribute their precision 16 and 34 sections.
//
// Each vector is mapped to one kernel call (see decOps). Operands are
// converted with the reference model's codec, even when the target is the
// kernel, so that a kernel operation is not judged through a kernel Parse;
// only apply and toSci exercise the target's Parse and Append. Results are
// compared bit for bit, except that any NaN expected matches any NaN got
// (the contract never propagates payloads, signs or signaling); those
// passes are counted as "NaN modulo payload".
//
// Conditions (the flags after the result) are not checked for arithmetic,
// because arithmetic returns no Status. For apply, which goes through
// Parse and does return one, Inexact, Overflow, Underflow and
// Conversion_syntax are checked; Rounded, Clamped, Subnormal and the other
// GDA conditions have no counterpart and are ignored.
//
// Skipped, with a reason counted per file: contexts other than the two
// formats, operations outside the kernel API (non-IEEE operations such as
// logb, scaleb, shift or power, and IEEE operations the API does not have),
// operands or results that cannot be represented or are outside the
// contract (NaN payloads, hex encodings, Division impossible), and rounding
// modes the contract does not have (half_down, up, 05up).
//
// Intel readtest.in. Lines have the form
//
//	bid64_add 0 [31c0000000000001] [31c0000000000002] [31c0000000000003] 00
//
// that is, function, rounding code (0 nearest even, 1 down, 2 up, 3 toward
// zero, 4 nearest away), the operands and the expected result as hex
// encodings in brackets (a decimal128 as 32 hex digits), and the expected
// status flags in hex, which are ignored for the same reason. The parser
// was written from the documented line format and has been run against
// hand-written lines only (TestIntelLineParser). It is unexercised against
// the real file.
//
// # Running it
//
//	BID_DECTEST_DIR=.../decimaltestdata go test -v -run PublishedVectors \
//	    internal/bid -bid.vectors=reference
//
// -bid.vectors selects the implementation under test, kernel (the default)
// or reference (the math/big model, to validate the model against the
// published vectors). A kernel function that is not implemented counts as a
// skip, never as a failure. -bid.vecshow and -bid.vecall control how many
// failing vectors are printed per file and -bid.vecfiles selects decTest
// files by regular expression.

import (
	"bufio"
	"flag"
	"fmt"
	"internal/bid"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	envDecTestDir = "BID_DECTEST_DIR"
	envIntel      = "BID_INTEL_READTEST"
)

var (
	vecTarget = flag.String("bid.vectors", "kernel", "implementation the published vectors run against: kernel or reference")
	vecShow   = flag.Int("bid.vecshow", 20, "print at most this many failing vectors per file")
	vecAll    = flag.Bool("bid.vecall", false, "print every failing vector")
	vecFiles  = flag.String("bid.vecfiles", "", "run only the decTest files whose names match this regular expression")
)

// vecRef is the reference model's codec, used to convert operands and
// expected results to bits and to render results.
var vecRef = reference{}

// vecOutcome is the result of one vector.
type vecOutcome struct {
	skip  string // non-empty: the vector was skipped, for this reason
	pass  bool
	loose bool // passed by NaN tolerance
	got   string
}

func vecSkip(reason string) vecOutcome { return vecOutcome{skip: reason} }

// vecTally counts the vectors of one file or of the whole run.
type vecTally struct {
	pass, fail, loose int
	skip              map[string]int
	failByOp          map[string]int
}

func newVecTally() *vecTally {
	return &vecTally{skip: map[string]int{}, failByOp: map[string]int{}}
}

func (t *vecTally) add(op string, o vecOutcome) {
	switch {
	case o.skip != "":
		t.skip[o.skip]++
	case o.pass:
		t.pass++
		if o.loose {
			t.loose++
		}
	default:
		t.fail++
		t.failByOp[op]++
	}
}

func (t *vecTally) merge(u *vecTally) {
	t.pass += u.pass
	t.fail += u.fail
	t.loose += u.loose
	for k, n := range u.skip {
		t.skip[k] += n
	}
	for k, n := range u.failByOp {
		t.failByOp[k] += n
	}
}

func (t *vecTally) skipped() int {
	n := 0
	for _, c := range t.skip {
		n += c
	}
	return n
}

// summary renders the tally as one line plus an indented line of skip
// reasons.
func (t *vecTally) summary(name string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%-28s pass=%-6d fail=%-6d skip=%-6d", name, t.pass, t.fail, t.skipped())
	if t.loose != 0 {
		fmt.Fprintf(&sb, " (pass includes %d with NaN modulo payload)", t.loose)
	}
	reasons := make([]string, 0, len(t.skip))
	for r := range t.skip {
		reasons = append(reasons, r)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if t.skip[reasons[i]] != t.skip[reasons[j]] {
			return t.skip[reasons[i]] > t.skip[reasons[j]]
		}
		return reasons[i] < reasons[j]
	})
	for _, r := range reasons {
		fmt.Fprintf(&sb, "\n    skip %-6d %s", t.skip[r], r)
	}
	return sb.String()
}

func vecTargetImpl(t *testing.T) impl {
	switch *vecTarget {
	case "kernel":
		return kernel{}
	case "reference":
		return reference{}
	}
	t.Fatalf("-bid.vectors=%q: want kernel or reference", *vecTarget)
	return nil
}

// TestPublishedVectors runs the published vectors named by the environment
// variables against -bid.vectors. It skips when neither variable is set.
func TestPublishedVectors(t *testing.T) {
	dir, intel := os.Getenv(envDecTestDir), os.Getenv(envIntel)
	if dir == "" && intel == "" {
		t.Skipf("published vectors not run: neither %s nor %s is set", envDecTestDir, envIntel)
	}
	im := vecTargetImpl(t)
	t.Logf("running published vectors against the %s", *vecTarget)
	total := newVecTally()
	if dir == "" {
		t.Logf("decTest vectors not run: %s is unset", envDecTestDir)
	} else {
		total.merge(runDecTestDir(t, im, dir))
	}
	if intel == "" {
		t.Logf("Intel vectors not run: %s is unset", envIntel)
	} else {
		total.merge(runIntelFile(t, im, intel))
	}
	t.Logf("TOTAL (%s)\n%s", *vecTarget, total.summary("all files"))
	if len(total.failByOp) != 0 {
		ops := make([]string, 0, len(total.failByOp))
		for op := range total.failByOp {
			ops = append(ops, op)
		}
		sort.Strings(ops)
		var sb strings.Builder
		sb.WriteString("failures by operation:")
		for _, op := range ops {
			fmt.Fprintf(&sb, " %s=%d", op, total.failByOp[op])
		}
		t.Log(sb.String())
	}
	if total.fail != 0 {
		t.Errorf("%d published vectors failed against the %s", total.fail, *vecTarget)
	}
}

// ---------------------------------------------------------------- decTest

type decContext struct {
	precision, maxExp, minExp int
	clamp, extended           int
	rounding                  string
}

// decVector is one test line with the context in force at that line.
type decVector struct {
	file   string
	line   int
	id, op string
	args   []string
	result string
	conds  []string
	ctx    decContext
}

type vecTok struct {
	s      string
	quoted bool
}

// vecTokens splits a decTest or readtest line into tokens. Single and
// double quotes delimit a token that may hold spaces and double a quote
// character to include it; "--" outside quotes starts a comment.
func vecTokens(line string) []vecTok {
	var toks []vecTok
	isSpace := func(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }
	for i := 0; i < len(line); {
		c := line[i]
		switch {
		case isSpace(c):
			i++
		case c == '-' && i+1 < len(line) && line[i+1] == '-':
			return toks
		case c == '\'' || c == '"':
			var sb strings.Builder
			i++
			for i < len(line) {
				if line[i] == c {
					if i+1 < len(line) && line[i+1] == c {
						sb.WriteByte(c)
						i += 2
						continue
					}
					break
				}
				sb.WriteByte(line[i])
				i++
			}
			i++ // the closing quote
			toks = append(toks, vecTok{sb.String(), true})
		default:
			j := i
			for j < len(line) && !isSpace(line[j]) {
				j++
			}
			toks = append(toks, vecTok{line[i:j], false})
			i = j
		}
	}
	return toks
}

// parseDecTest reads one decTest file and calls emit for every test line.
func parseDecTest(r io.Reader, file string, emit func(*decVector)) error {
	ctx := decContext{extended: 1}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for n := 1; sc.Scan(); n++ {
		toks := vecTokens(sc.Text())
		if len(toks) == 0 {
			continue
		}
		if first := toks[0]; !first.quoted && strings.Contains(first.s, ":") {
			key, val, _ := strings.Cut(first.s, ":")
			if val == "" && len(toks) > 1 {
				val = toks[1].s
			}
			vecDirective(&ctx, strings.ToLower(key), val)
			continue
		}
		v := &decVector{file: file, line: n, ctx: ctx}
		arrow := -1
		for i, tk := range toks {
			if !tk.quoted && tk.s == "->" {
				arrow = i
				break
			}
		}
		if arrow < 2 || arrow+1 >= len(toks) {
			v.id = toks[0].s
			v.op = "?malformed"
			emit(v)
			continue
		}
		v.id, v.op = toks[0].s, toks[1].s
		for _, tk := range toks[2:arrow] {
			v.args = append(v.args, tk.s)
		}
		v.result = toks[arrow+1].s
		for _, tk := range toks[arrow+2:] {
			v.conds = append(v.conds, strings.ToLower(tk.s))
		}
		emit(v)
	}
	return sc.Err()
}

func vecDirective(c *decContext, key, val string) {
	num := func(dst *int) {
		if n, err := strconv.Atoi(val); err == nil {
			*dst = n
		}
	}
	switch key {
	case "precision":
		num(&c.precision)
	case "maxexponent":
		num(&c.maxExp)
	case "minexponent":
		num(&c.minExp)
	case "clamp":
		num(&c.clamp)
	case "extended":
		num(&c.extended)
	case "rounding":
		c.rounding = strings.ToLower(val)
	}
}

func (v *decVector) hasCond(c string) bool {
	for _, x := range v.conds {
		if x == c {
			return true
		}
	}
	return false
}

func runDecTestDir(t *testing.T, im impl, dir string) *vecTally {
	files, err := filepath.Glob(filepath.Join(dir, "*.decTest"))
	if err != nil || len(files) == 0 {
		t.Fatalf("%s=%s: no *.decTest files", envDecTestDir, dir)
	}
	sort.Strings(files)
	var only *regexp.Regexp
	if *vecFiles != "" {
		only = regexp.MustCompile(*vecFiles)
	}
	total := newVecTally()
	for _, path := range files {
		name := filepath.Base(path)
		if only != nil && !only.MatchString(name) {
			continue
		}
		fh, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		tally := newVecTally()
		shown := 0
		err = parseDecTest(fh, name, func(v *decVector) {
			o := evalDec(im, v)
			tally.add(strings.ToLower(v.op), o)
			if o.skip == "" && !o.pass && (*vecAll || shown < *vecShow) {
				shown++
				t.Logf("FAIL %s:%d %s %s %s -> %s [%s p=%d] got %s", name, v.line, v.id, v.op,
					quoteArgs(v.args), v.result, v.ctx.rounding, v.ctx.precision, o.got)
			}
		})
		fh.Close()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if tally.pass+tally.fail+tally.skipped() == 0 {
			continue
		}
		t.Log(tally.summary(name))
		if tally.fail > shown {
			t.Logf("    %s: %d failing vectors not shown (-bid.vecshow, -bid.vecall)", name, tally.fail-shown)
		}
		total.merge(tally)
	}
	t.Logf("decTest total (%s)\n%s", *vecTarget, total.summary("decTest"))
	return total
}

func quoteArgs(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = "'" + a + "'"
	}
	return strings.Join(q, " ")
}

// vecMode maps a decTest rounding name to the kernel's modes.
func vecMode(name string) (bid.Mode, bool) {
	switch name {
	case "half_even":
		return bid.ToNearestEven, true
	case "half_up":
		return bid.ToNearestAway, true
	case "down":
		return bid.TowardZero, true
	case "ceiling":
		return bid.TowardPositive, true
	case "floor":
		return bid.TowardNegative, true
	}
	return 0, false
}

// vecFormat picks the format of a context, or says why there is none.
func vecFormat(c *decContext) (*format, string) {
	switch {
	case c.precision != 16 && c.precision != 34:
		return nil, "context: precision not 16 or 34"
	case c.precision == 16 && (c.maxExp != 384 || c.minExp != -383),
		c.precision == 34 && (c.maxExp != 6144 || c.minExp != -6143):
		return nil, "context: exponent range is not the format's"
	case c.extended != 1:
		return nil, "context: extended 0"
	case c.precision == 16:
		return dec64, ""
	}
	return dec128, ""
}

var (
	vecNaNPayload = regexp.MustCompile(`(?i)^[+-]?s?nan[0-9]+$`)
	vecNaNWord    = regexp.MustCompile(`(?i)^[+-]?s?nan[0-9]*$`)
)

// vecNumeral matches a decimal numeral: sign, digits with an optional point,
// optional exponent part.
var vecNumeral = regexp.MustCompile(`^[+-]?[0-9]*(?:\.([0-9]*))?(?:[eE]([+-]?[0-9]+))?$`)

// vecOutOfRange reports whether a numeral's own exponent lies outside the
// format's [MinExp, MaxExp]. With clamp 0 the vectors keep such an exponent
// as written, where the contract clamps it, so a vector that mentions one
// is not comparable.
func vecOutOfRange(f *format, s string) bool {
	m := vecNumeral.FindStringSubmatch(s)
	if m == nil {
		return false // not a numeral: a special word, hex or '?'
	}
	e := 0
	if m[2] != "" {
		var err error
		if e, err = strconv.Atoi(m[2]); err != nil {
			return true
		}
	}
	e -= len(m[1])
	return e > f.maxExp || e < f.minExp
}

// vecClampReason is the skip reason for a vector of a clamp 0 context that
// mentions an exponent the format cannot hold, or "" if it can run. With
// clamp 1 every vector can.
func vecClampReason(f *format, v *decVector) string {
	if v.ctx.clamp == 1 {
		return ""
	}
	for _, s := range append(append([]string(nil), v.args...), v.result) {
		if vecOutOfRange(f, s) {
			return "context: clamp 0 and an exponent outside the format's"
		}
	}
	return ""
}

// vecOperand converts an operand with the reference model's codec. The
// operand must be exactly representable; the vectors outside that are
// skipped.
func vecOperand(f *format, s string) (bits, string) {
	switch {
	case strings.HasPrefix(s, "#"):
		return bits{}, "operand: hex encoding"
	case vecNaNPayload.MatchString(s):
		return bits{}, "contract: NaN payload operand"
	}
	x, st := f.parse(s, bid.ToNearestEven)
	switch {
	case st&bid.Syntax != 0:
		return bits{}, "operand: outside the contract's numeral grammar"
	case st != bid.Exact:
		return bits{}, "operand: not representable in the format"
	}
	// Parse gives every NaN the canonical positive encoding; a signed NaN
	// operand takes its sign through the sign operation, which the sign
	// operations and TotalOrder read.
	if strings.HasPrefix(s, "-") && vecRef.isNaN(f, x) {
		x = vecRef.neg(f, x)
	}
	return x, ""
}

// decRes is the result of a mapped call: a decimal, or text for the
// operations that do not return one.
type decRes struct {
	x    bits
	text string
}

type decOp struct {
	n     int  // operands
	modal bool // takes the context's rounding mode
	text  bool // the result is text rather than a decimal
	run   func(im impl, f *format, a []bits, m bid.Mode) decRes
}

func resBits(x bits) decRes { return decRes{x: x} }

func resCmp(c int, ok bool) decRes {
	if !ok {
		return decRes{text: "NaN"}
	}
	return decRes{text: strconv.Itoa(c)}
}

func resBool(b bool) decRes {
	if b {
		return decRes{text: "1"}
	}
	return decRes{text: "0"}
}

// totalCmp is comparetotal as -1, 0 or 1, derived from the totalOrder
// predicate.
func totalCmp(im impl, f *format, x, y bits) int {
	xy, yx := im.totalOrder(f, x, y), im.totalOrder(f, y, x)
	switch {
	case xy && yx:
		return 0
	case xy:
		return -1
	}
	return 1
}

// decOps maps a decTest operation to a kernel call. The GDA names are:
// remainder is the truncating remainder (Mod), remainderNear the IEEE
// remainder (Rem), tointegralx and tointegral round to an integral value.
var decOps = map[string]decOp{
	"add":      {2, true, false, func(im impl, f *format, a []bits, m bid.Mode) decRes { return resBits(im.add(f, a[0], a[1], m)) }},
	"subtract": {2, true, false, func(im impl, f *format, a []bits, m bid.Mode) decRes { return resBits(im.sub(f, a[0], a[1], m)) }},
	"multiply": {2, true, false, func(im impl, f *format, a []bits, m bid.Mode) decRes { return resBits(im.mul(f, a[0], a[1], m)) }},
	"divide":   {2, true, false, func(im impl, f *format, a []bits, m bid.Mode) decRes { return resBits(im.div(f, a[0], a[1], m)) }},
	"fma": {3, true, false, func(im impl, f *format, a []bits, m bid.Mode) decRes {
		return resBits(im.fma(f, a[0], a[1], a[2], m))
	}},
	"remainder":     {2, false, false, func(im impl, f *format, a []bits, m bid.Mode) decRes { return resBits(im.mod(f, a[0], a[1])) }},
	"remaindernear": {2, false, false, func(im impl, f *format, a []bits, m bid.Mode) decRes { return resBits(im.rem(f, a[0], a[1])) }},
	"tointegralx": {1, true, false, func(im impl, f *format, a []bits, m bid.Mode) decRes {
		return resBits(im.roundToIntegral(f, a[0], m))
	}},
	"tointegral": {1, true, false, func(im impl, f *format, a []bits, m bid.Mode) decRes {
		return resBits(im.roundToIntegral(f, a[0], m))
	}},
	"quantize": {2, true, false, func(im impl, f *format, a []bits, m bid.Mode) decRes {
		return resBits(im.quantize(f, a[0], a[1], m))
	}},
	"reduce": {1, false, false, func(im impl, f *format, a []bits, m bid.Mode) decRes { return resBits(im.reduce(f, a[0])) }},
	"samequantum": {2, false, true, func(im impl, f *format, a []bits, m bid.Mode) decRes {
		return resBool(im.sameQuantum(f, a[0], a[1]))
	}},
	"compare": {2, false, true, func(im impl, f *format, a []bits, m bid.Mode) decRes {
		return resCmp(im.compare(f, a[0], a[1]))
	}},
	// comparesig differs from compare only in what it signals.
	"comparesig": {2, false, true, func(im impl, f *format, a []bits, m bid.Mode) decRes {
		return resCmp(im.compare(f, a[0], a[1]))
	}},
	"comparetotal": {2, false, true, func(im impl, f *format, a []bits, m bid.Mode) decRes {
		return resCmp(totalCmp(im, f, a[0], a[1]), true)
	}},
	"comparetotmag": {2, false, true, func(im impl, f *format, a []bits, m bid.Mode) decRes {
		return resCmp(totalCmp(im, f, im.abs(f, a[0]), im.abs(f, a[1])), true)
	}},
	"max":        {2, false, false, func(im impl, f *format, a []bits, m bid.Mode) decRes { return resBits(im.max(f, a[0], a[1])) }},
	"min":        {2, false, false, func(im impl, f *format, a []bits, m bid.Mode) decRes { return resBits(im.min(f, a[0], a[1])) }},
	"abs":        {1, false, false, func(im impl, f *format, a []bits, m bid.Mode) decRes { return resBits(im.abs(f, a[0])) }},
	"copyabs":    {1, false, false, func(im impl, f *format, a []bits, m bid.Mode) decRes { return resBits(im.abs(f, a[0])) }},
	"copynegate": {1, false, false, func(im impl, f *format, a []bits, m bid.Mode) decRes { return resBits(im.neg(f, a[0])) }},
	"copysign": {2, false, false, func(im impl, f *format, a []bits, m bid.Mode) decRes {
		return resBits(im.copySign(f, a[0], a[1]))
	}},
}

// vecOpReason names why an operation has no mapping.
func vecOpReason(op string) string {
	switch op {
	case "nextplus", "nextminus", "nexttoward", "class", "canonical", "copy",
		"minmag", "maxmag", "toeng", "plus", "minus":
		return "op: IEEE or identity operation outside the kernel API"
	case "?malformed":
		return "malformed line"
	case "divideint", "logb", "scaleb", "shift", "rotate", "and", "or", "xor",
		"invert", "power", "exp", "ln", "log10", "squareroot", "powersqrt",
		"trim", "rescale", "remainder_near":
		return "op: non-IEEE operation"
	}
	return "op: unknown operation"
}

// evalDec runs one decTest vector against im.
func evalDec(im impl, v *decVector) vecOutcome {
	f, why := vecFormat(&v.ctx)
	if why != "" {
		return vecSkip(why)
	}
	if why := vecClampReason(f, v); why != "" {
		return vecSkip(why)
	}
	op := strings.ToLower(v.op)
	if op == "apply" || op == "tosci" {
		return evalDecParse(im, f, v, op == "apply")
	}
	spec, ok := decOps[op]
	if !ok {
		return vecSkip(vecOpReason(op))
	}
	if len(v.args) != spec.n {
		return vecSkip("malformed line")
	}
	mode := bid.ToNearestEven
	if spec.modal {
		if mode, ok = vecMode(v.ctx.rounding); !ok {
			return vecSkip("context: rounding mode outside the contract")
		}
	}
	if v.hasCond("division_impossible") {
		return vecSkip("contract: remainder Division impossible")
	}
	args := make([]bits, spec.n)
	for i, s := range v.args {
		var why string
		if args[i], why = vecOperand(f, s); why != "" {
			return vecSkip(why)
		}
		if (op == "comparetotal" || op == "comparetotmag") && strings.Contains(strings.ToLower(s), "snan") {
			return vecSkip("contract: signaling NaN ordering")
		}
	}
	if v.result == "?" {
		return vecSkip("result: undefined (?)")
	}
	if strings.HasPrefix(v.result, "#") {
		return vecSkip("result: hex encoding")
	}
	wantNaN := vecNaNWord.MatchString(v.result)
	var want bits
	if !spec.text && !wantNaN {
		var st bid.Status
		if want, st = f.parse(v.result, bid.ToNearestEven); st != bid.Exact {
			return vecSkip("result: not representable in the format")
		}
	}

	var got decRes
	implemented, err := try(func() { got = spec.run(im, f, args, mode) })
	switch {
	case !implemented:
		return vecSkip("kernel: not implemented")
	case err != nil:
		return vecOutcome{got: err.Error()}
	}

	if spec.text {
		wantText := v.result
		loose := false
		if wantNaN {
			wantText, loose = "NaN", v.result != "NaN"
		}
		return vecOutcome{pass: got.text == wantText, loose: loose, got: got.text}
	}
	gotText := fmt.Sprintf("%s (%016x%016x)", f.format(got.x), got.x.Hi, got.x.Lo)
	if wantNaN {
		return vecOutcome{pass: vecRef.isNaN(f, got.x), loose: v.result != "NaN", got: gotText}
	}
	out := vecOutcome{pass: got.x == want, got: gotText}
	if !out.pass && (op == "min" || op == "max") {
		if why := minMaxDivergence(f, args[0], args[1], want, got.x); why != "" {
			return vecSkip(why)
		}
	}
	return out
}

// The two divergences of Min and Max from GDA, named in the package
// documentation under "Minimum and maximum".
const (
	skipMinMaxNaN    = "contract: Min/Max return NaN if either operand is NaN (GDA returns the number)"
	skipMinMaxCohort = "contract: Min/Max return x for operands equal in value and sign (GDA chooses by exponent)"
)

// minMaxDivergence reports, for a Min or Max vector that failed, whether
// the failure is exactly one of the divergences the contract documents,
// and returns the name of the divergence, or "" when it is not. It is
// deliberately narrow, so that every other Min or Max failure still fails:
//
//   - NaN: exactly one operand is a NaN, the vector expects the other
//     operand (GDA returns the number) and the target returned a NaN (the
//     contract returns NaN if either operand is NaN).
//   - Cohort: both operands are finite, equal in value and sign (members
//     of one cohort with different exponents, or zeros of one sign), the
//     vector expects y (GDA picks by exponent) and the target returned x
//     (the contract returns x).
//
// want and got are the expected and obtained encodings.
func minMaxDivergence(f *format, x, y, want, got bits) string {
	a, b := f.decode(x), f.decode(y)
	switch {
	case (a.kind == notANumber) != (b.kind == notANumber):
		number := x
		if a.kind == notANumber {
			number = y
		}
		if want == number && f.decode(got).kind == notANumber {
			return skipMinMaxNaN
		}
	case a.kind == finite && b.kind == finite && a.neg == b.neg && cmpValue(a, b) == 0:
		if want == y && got == x && x != y {
			return skipMinMaxCohort
		}
	}
	return ""
}

// TestMinMaxDivergence checks that minMaxDivergence excuses only the two
// documented divergences and nothing wider.
func TestMinMaxDivergence(t *testing.T) {
	d := func(s string) bits {
		x, st := dec64.parse(s, bid.ToNearestEven)
		if st != bid.Exact {
			t.Fatalf("parse %q: %v", s, st)
		}
		return x
	}
	qnan := dec64.encode(nan)
	cases := []struct {
		name          string
		x, y, want, g bits
		reason        string
	}{
		{"nan first, number expected", qnan, d("1"), d("1"), qnan, skipMinMaxNaN},
		{"nan second, number expected", d("7"), qnan, d("7"), qnan, skipMinMaxNaN},
		{"nan, wrong number expected", qnan, d("1"), d("2"), qnan, ""},
		{"nan, target returned the number", qnan, d("1"), d("1"), d("1"), ""},
		{"nan, target returned another number", qnan, d("1"), d("1"), d("2"), ""},
		{"cohort, y expected, x returned", d("1.0"), d("1"), d("1"), d("1.0"), skipMinMaxCohort},
		{"cohort of zeros", d("0E1"), d("0E2"), d("0E2"), d("0E1"), skipMinMaxCohort},
		{"cohort, target returned a third value", d("1.0"), d("1"), d("1"), d("1.00"), ""},
		{"cohort, target returned y", d("1.0"), d("1"), d("1"), d("1"), ""},
		{"opposite zeros", d("0"), d("-0"), d("0"), d("-0"), ""},
		{"unequal values", d("1"), d("2"), d("2"), d("1"), ""},
	}
	for _, c := range cases {
		if got := minMaxDivergence(dec64, c.x, c.y, c.want, c.g); got != c.reason {
			t.Errorf("%s: got %q, want %q", c.name, got, c.reason)
		}
	}
}

// evalDecParse runs apply (which rounds to the context) and toSci (which
// converts exactly) through the target's Parse and Append.
func evalDecParse(im impl, f *format, v *decVector, apply bool) vecOutcome {
	if len(v.args) != 1 {
		return vecSkip("malformed line")
	}
	s := v.args[0]
	mode, ok := vecMode(v.ctx.rounding)
	switch {
	case strings.HasPrefix(s, "#"):
		return vecSkip("operand: hex encoding")
	case vecNaNPayload.MatchString(s):
		return vecSkip("contract: NaN payload operand")
	case v.hasCond("invalid_operation"):
		return vecSkip("contract: signaling NaN reports no Invalid")
	case !ok:
		return vecSkip("context: rounding mode outside the contract")
	case v.result == "?" || strings.HasPrefix(v.result, "#"):
		return vecSkip("result: undefined or hex")
	}
	var x bits
	var st bid.Status
	var text string
	implemented, err := try(func() {
		x, st = im.parse(f, s, mode)
		text = im.appendTo(f, x)
	})
	switch {
	case !implemented:
		return vecSkip("kernel: not implemented")
	case err != nil:
		return vecOutcome{got: err.Error()}
	}
	if !apply && st&^bid.Syntax != 0 {
		return vecSkip("operand: toSci operand not representable")
	}
	want, loose := v.result, false
	if vecNaNWord.MatchString(want) {
		want, loose = "NaN", want != "NaN"
	}
	got := text
	pass := text == want
	if apply {
		var wantSt bid.Status
		for _, c := range v.conds {
			switch c {
			case "inexact":
				wantSt |= bid.Inexact
			case "overflow":
				wantSt |= bid.Inexact | bid.Overflow
			case "underflow":
				wantSt |= bid.Inexact | bid.Underflow
			case "conversion_syntax":
				wantSt |= bid.Syntax
			}
		}
		got = fmt.Sprintf("%s, %s", text, statusString(st))
		pass = pass && st == wantSt
		if !pass {
			got += fmt.Sprintf(" (want %s)", statusString(wantSt))
		}
	}
	return vecOutcome{pass: pass, loose: pass && loose, got: got}
}

// ------------------------------------------------------------------ Intel

// intelVector is one readtest.in line: function, rounding code, and the
// remaining fields (operands, expected result, expected status).
type intelVector struct {
	line   int
	name   string
	round  string
	fields []string
}

// parseIntelLine parses one line of readtest.in. ok is false for blank
// lines, comments and lines of another shape.
func parseIntelLine(text string, n int) (*intelVector, bool) {
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "--") {
		return nil, false
	}
	toks := vecTokens(text)
	if len(toks) < 4 || !strings.HasPrefix(toks[0].s, "bid") {
		return nil, false
	}
	v := &intelVector{line: n, name: toks[0].s, round: toks[1].s}
	for _, tk := range toks[2:] {
		v.fields = append(v.fields, tk.s)
	}
	return v, true
}

// intelHex reads a bracketed hex encoding of the given width.
func intelHex(s string, width int) (bits, bool) {
	s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	if s == "" || len(s) > width/4 {
		return bits{}, false
	}
	for len(s) < width/4 {
		s = "0" + s
	}
	if width == 64 {
		lo, err := strconv.ParseUint(s, 16, 64)
		return bits{Lo: lo}, err == nil
	}
	hi, err1 := strconv.ParseUint(s[:16], 16, 64)
	lo, err2 := strconv.ParseUint(s[16:], 16, 64)
	return bits{Hi: hi, Lo: lo}, err1 == nil && err2 == nil
}

func intelMode(code string) (bid.Mode, bool) {
	switch code {
	case "0":
		return bid.ToNearestEven, true
	case "1":
		return bid.TowardNegative, true
	case "2":
		return bid.TowardPositive, true
	case "3":
		return bid.TowardZero, true
	case "4":
		return bid.ToNearestAway, true
	}
	return 0, false
}

// intelOps maps the function name after the bid64_ or bid128_ prefix to a
// decOp, with the fixed rounding mode for the round_integral variants.
// Function names and argument orders are from the documented format; they
// have not been checked against the real file.
var intelOps = map[string]struct {
	decOp
	fixed *bid.Mode
}{
	"add":                         {decOps["add"], nil},
	"sub":                         {decOps["subtract"], nil},
	"mul":                         {decOps["multiply"], nil},
	"div":                         {decOps["divide"], nil},
	"fma":                         {decOps["fma"], nil},
	"rem":                         {decOps["remaindernear"], nil},
	"quantize":                    {decOps["quantize"], nil},
	"negate":                      {decOps["copynegate"], nil},
	"abs":                         {decOps["abs"], nil},
	"round_integral_exact":        {decOps["tointegralx"], nil},
	"round_integral_nearest_even": {decOps["tointegralx"], modePtr(bid.ToNearestEven)},
	"round_integral_nearest_away": {decOps["tointegralx"], modePtr(bid.ToNearestAway)},
	"round_integral_zero":         {decOps["tointegralx"], modePtr(bid.TowardZero)},
	"round_integral_positive":     {decOps["tointegralx"], modePtr(bid.TowardPositive)},
	"round_integral_negative":     {decOps["tointegralx"], modePtr(bid.TowardNegative)},
	"sameQuantum":                 {decOps["samequantum"], nil},
	"quiet_equal": {decOp{2, false, true, func(im impl, f *format, a []bits, m bid.Mode) decRes {
		return resBool(im.equal(f, a[0], a[1]))
	}}, nil},
	"quiet_less": {decOp{2, false, true, func(im impl, f *format, a []bits, m bid.Mode) decRes {
		return resBool(im.less(f, a[0], a[1]))
	}}, nil},
	"quiet_less_equal": {decOp{2, false, true, func(im impl, f *format, a []bits, m bid.Mode) decRes {
		return resBool(im.lessEqual(f, a[0], a[1]))
	}}, nil},
	"totalOrder": {decOp{2, false, true, func(im impl, f *format, a []bits, m bid.Mode) decRes {
		return resBool(im.totalOrder(f, a[0], a[1]))
	}}, nil},
}

func modePtr(m bid.Mode) *bid.Mode { return &m }

// evalIntel runs one readtest.in vector against im.
func evalIntel(im impl, v *intelVector) vecOutcome {
	var f *format
	var rest string
	switch {
	case strings.HasPrefix(v.name, "bid64_"):
		f, rest = dec64, strings.TrimPrefix(v.name, "bid64_")
	case strings.HasPrefix(v.name, "bid128_"):
		f, rest = dec128, strings.TrimPrefix(v.name, "bid128_")
	default:
		return vecSkip("format: not decimal64 or decimal128")
	}
	switch rest {
	case "to_bid128", "to_bid64":
		return evalIntelConvert(im, f, v, rest)
	}
	spec, ok := intelOps[rest]
	if !ok {
		return vecSkip("op: not mapped to the kernel API")
	}
	// operands, expected result, expected status
	if len(v.fields) != spec.n+2 {
		return vecSkip("malformed line")
	}
	mode, ok := intelMode(v.round)
	if !ok {
		return vecSkip("context: rounding code unknown")
	}
	if spec.fixed != nil {
		mode = *spec.fixed
	}
	args := make([]bits, spec.n)
	for i := range args {
		if args[i], ok = intelHex(v.fields[i], f.width); !ok {
			return vecSkip("malformed line")
		}
	}
	resField := v.fields[spec.n]
	var want bits
	if !spec.text {
		if want, ok = intelHex(resField, f.width); !ok {
			return vecSkip("malformed line")
		}
	}
	var got decRes
	implemented, err := try(func() { got = spec.run(im, f, args, mode) })
	switch {
	case !implemented:
		return vecSkip("kernel: not implemented")
	case err != nil:
		return vecOutcome{got: err.Error()}
	}
	if spec.text {
		return vecOutcome{pass: got.text == resField, got: got.text}
	}
	return intelCompare(f, got.x, want)
}

// intelCompare compares bits, treating any NaN as equal to any NaN.
func intelCompare(f *format, got, want bits) vecOutcome {
	gotText := fmt.Sprintf("%016x%016x", got.Hi, got.Lo)
	if f.width == 64 {
		gotText = fmt.Sprintf("%016x", got.Lo)
	}
	if vecRef.isNaN(f, want) {
		return vecOutcome{pass: vecRef.isNaN(f, got), loose: got != want, got: gotText}
	}
	return vecOutcome{pass: got == want, got: gotText}
}

// evalIntelConvert handles bid64_to_bid128 (Widen) and bid128_to_bid64
// (Narrow with the line's rounding mode).
func evalIntelConvert(im impl, from *format, v *intelVector, rest string) vecOutcome {
	to := dec128
	if rest == "to_bid64" {
		to = dec64
	}
	if len(v.fields) != 3 {
		return vecSkip("malformed line")
	}
	mode, ok := intelMode(v.round)
	if !ok {
		return vecSkip("context: rounding code unknown")
	}
	x, ok1 := intelHex(v.fields[0], from.width)
	want, ok2 := intelHex(v.fields[1], to.width)
	if !ok1 || !ok2 {
		return vecSkip("malformed line")
	}
	var got bits
	implemented, err := try(func() {
		if to == dec128 {
			got = im.widen(x)
		} else {
			got = im.narrow(x, mode)
		}
	})
	switch {
	case !implemented:
		return vecSkip("kernel: not implemented")
	case err != nil:
		return vecOutcome{got: err.Error()}
	}
	return intelCompare(to, got, want)
}

func runIntelFile(t *testing.T, im impl, path string) *vecTally {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		path = filepath.Join(path, "readtest.in")
	}
	fh, err := os.Open(path)
	if err != nil {
		t.Fatalf("%s: %v", envIntel, err)
	}
	defer fh.Close()
	tally := newVecTally()
	shown := 0
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for n := 1; sc.Scan(); n++ {
		v, ok := parseIntelLine(sc.Text(), n)
		if !ok {
			continue
		}
		o := evalIntel(im, v)
		tally.add(v.name, o)
		if o.skip == "" && !o.pass && (*vecAll || shown < *vecShow) {
			shown++
			t.Logf("FAIL readtest.in:%d %s %s %s: got %s", n, v.name, v.round, strings.Join(v.fields, " "), o.got)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	t.Log(tally.summary("readtest.in"))
	if tally.fail > shown {
		t.Logf("    readtest.in: %d failing vectors not shown (-bid.vecshow, -bid.vecall)", tally.fail-shown)
	}
	return tally
}

// ----------------------------------------------------- parser self-checks

// TestDecTestLineParser checks the decTest parser and the mapping on
// hand-written lines, against the reference model. It needs no vectors.
func TestDecTestLineParser(t *testing.T) {
	const src = `
-- a comment line
version: 2.59
Precision:   16
maxExponent: 384
minexponent: -383
extended:    1
clamp:       1
rounding:    half_even

t001 add 1 2 -> 3
t002 add '1.50' "2.5" -> 4.00  -- trailing comment
t003 divide 1 3 -> 0.3333333333333333 Inexact Rounded
t004 compare 1 NaN -> NaN
t005 toSci '' -> NaN Conversion_syntax
t006 apply 1E+400 -> Infinity Overflow Inexact Rounded
t007 add 1 1 -> ?
t008 logb 1 -> 0
t009 add 'it''s' 1 -> NaN
rounding: ceiling
t010 divide 1 3 -> 0.3333333333333334 Inexact Rounded
rounding: up
t011 divide 1 3 -> 0.3333333333333334 Inexact Rounded
rounding: half_even
t013 add 1 2 -> 4
t014 comparetotal -NaN NaN -> -1
t015 copysign 7 -NaN -> -7
precision: 9
t012 add 1 2 -> 3
`
	var got []*decVector
	if err := parseDecTest(strings.NewReader(src), "self.decTest", func(v *decVector) { got = append(got, v) }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 15 {
		t.Fatalf("parsed %d vectors, want 15", len(got))
	}
	if v := got[1]; v.id != "t002" || v.op != "add" || len(v.args) != 2 || v.args[0] != "1.50" || v.args[1] != "2.5" || v.result != "4.00" {
		t.Errorf("t002 parsed as %+v", v)
	}
	if v := got[2]; len(v.conds) != 2 || v.conds[0] != "inexact" {
		t.Errorf("t003 conditions %v", v.conds)
	}
	if v := got[8]; v.args[0] != "it's" {
		t.Errorf("t009 operand %q, want it's", v.args[0])
	}
	if c := got[0].ctx; c.precision != 16 || c.maxExp != 384 || c.minExp != -383 || c.clamp != 1 || c.extended != 1 || c.rounding != "half_even" {
		t.Errorf("context of t001 = %+v", c)
	}
	if c := got[9].ctx; c.rounding != "ceiling" || c.precision != 16 {
		t.Errorf("context of t010 = %+v", c)
	}

	want := map[string]string{ // id -> outcome: pass or the skip reason
		"t001": "pass", "t002": "pass", "t003": "pass", "t004": "pass",
		"t005": "pass", "t006": "pass",
		"t007": "result: undefined (?)",
		"t008": "op: non-IEEE operation",
		"t009": "operand: outside the contract's numeral grammar",
		"t010": "pass",
		"t011": "context: rounding mode outside the contract",
		"t012": "context: precision not 16 or 34",
		"t013": "fail", // a wrong expectation must fail, not skip
		"t014": "pass", "t015": "pass",
	}
	for _, v := range got {
		o := evalDec(reference{}, v)
		switch {
		case o.skip != "":
			if o.skip != want[v.id] {
				t.Errorf("%s skipped: %s; want %s", v.id, o.skip, want[v.id])
			}
		case !o.pass:
			if want[v.id] != "fail" {
				t.Errorf("%s failed against the reference model: got %s", v.id, o.got)
			}
		case want[v.id] != "pass":
			t.Errorf("%s passed; want skip %q", v.id, want[v.id])
		}
	}
}

// TestIntelLineParser checks the readtest.in parser and mapping on
// hand-written lines, against the reference model. These lines were written
// from the documented format, not copied from the real file.
func TestIntelLineParser(t *testing.T) {
	const src = `
-- a comment
bid64_add 0 [31c0000000000001] [31c0000000000002] [31c0000000000003] 00
bid64_sub 0 [31c0000000000003] [31c0000000000001] [31c0000000000002] 00
bid64_mul 0 [31c0000000000002] [31c0000000000003] [31c0000000000006] 00
bid64_div 0 [31c0000000000001] [31c0000000000003] [2fcbd7a625405555] 20
bid64_div 2 [31c0000000000001] [31c0000000000003] [2fcbd7a625405556] 20
bid64_add 0 [7c00000000000000] [31c0000000000001] [7c00000000000000] 00
bid64_add 0 [7e00000000000001] [31c0000000000001] [7c00000000000000] 01
bid128_add 0 [30400000000000000000000000000001] [30400000000000000000000000000002] [30400000000000000000000000000003] 00
bid64_to_bid128 0 [31c0000000000001] [30400000000000000000000000000001] 00
bid128_to_bid64 0 [30400000000000000000000000000003] [31c0000000000003] 00
bid64_quiet_less 0 [31c0000000000001] [31c0000000000002] 1 00
bid64_sqrt 0 [31c0000000000004] [31c0000000000002] 00
bid64_add 9 [31c0000000000001] [31c0000000000002] [31c0000000000003] 00
bid32_add 0 [32800001] [32800002] [32800003] 00
bid64_add 0 [31c0000000000001] [31c0000000000002] [31c0000000000004] 00
`
	skips := map[int]string{
		12: "op: not mapped to the kernel API",
		13: "context: rounding code unknown",
		14: "format: not decimal64 or decimal128",
	}
	n, parsed := 0, 0
	sc := bufio.NewScanner(strings.NewReader(src))
	for line := 1; sc.Scan(); line++ {
		v, ok := parseIntelLine(sc.Text(), line)
		if !ok {
			continue
		}
		parsed++
		n++
		o := evalIntel(reference{}, v)
		switch {
		case n == 15: // deliberately wrong expectation: 1+2 is not 4
			if o.skip != "" || o.pass {
				t.Errorf("line %d: a wrong expectation must fail, got %+v", line, o)
			}
		case skips[n] != "":
			if o.skip != skips[n] {
				t.Errorf("line %d: skip %q, want %q", line, o.skip, skips[n])
			}
		case o.skip != "":
			t.Errorf("line %d (%s) skipped: %s", line, v.name, o.skip)
		case !o.pass:
			t.Errorf("line %d (%s) failed against the reference model: got %s", line, v.name, o.got)
		}
	}
	if parsed != 15 {
		t.Errorf("parsed %d lines, want 15", parsed)
	}
}
