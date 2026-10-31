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
// (IntelRDFPMathLib) ships the vectors as TESTS/readtest.in. Source: the
// release IntelRDFPMathLib20U2 (version 2.0 update 2), downloaded from
// https://netlib.org/misc/intel/IntelRDFPMathLib20U2.tar.gz, whose sha256
// is 93c0c78e0989df88f8540bf38d6743734804cef1e40706fd8fe5c6a03f79e173.
// The licence is the file eula.txt at the top of that release, which
// begins
//
//	Copyright (c) 2018, Intel Corp.
//
//	All rights reserved.
//
//	Redistribution and use in source and binary forms, with or without modification,
//	are permitted provided that the following conditions are met:
//
// and lists three conditions: redistributions of source code retain the
// copyright notice, the list of conditions and the disclaimer;
// redistributions in binary form reproduce them in the documentation or
// other materials provided with the distribution; and the name of Intel
// Corporation and the names of its contributors may not be used to endorse
// or promote products derived from the software without specific prior
// written permission. A disclaimer of warranties and of liability follows.
// That is the three-clause BSD licence. readtest.in itself carries no
// notice; the one in eula.txt covers the release. Even so permissive a
// licence obliges the tree to carry Intel's notice with a copy of the
// file, and readtest.in is 127,118 lines (9.6 MB) of generated data, which
// is not wanted in the Go repository.
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
// Intel readtest.in. A line is
//
//	function rounding operand... result status [trailing field...]
//
// separated by white space, where "--" starts a comment (a whole-line
// comment or a remark such as "-- MinDen"). The function is bid32_,
// bid64_ or bid128_ and the operation, or binaryNN_to_bidNN, or bid_ and
// the name of a library function that is not an operation. A mixed-width
// function, such as bid128dd_add or bid64qq_sub, has letters d (decimal64)
// and q (decimal128) after the prefix, one for each operand, and the prefix
// gives the result's format. The rounding code is 0 nearest even, 1 toward
// negative, 2 toward positive, 3 toward zero, 4 nearest away. A decimal
// operand or result is a hex encoding in brackets (16 digits for
// decimal64, 32 digits or two groups of 16 separated by a comma for
// decimal128, in either case), or text that the harness converts to the
// operand's format first: a numeral ("0", "-0", "+8898.E5") or one of the
// words Inf, Infinity, NaN, QNaN and SNaN in any case, each with an
// optional sign. The runner reads text with the reference codec, rounding
// to nearest even whatever the line's code, which is what the harness does
// (the code applies to the operation, not to reading its operands); only
// from_string hands its text to the target's Parse, in the line's mode.
// A predicate's result is 0 or 1, an integer conversion's result a decimal
// integer, a conversion to binary a hex bit pattern in brackets (8 or 16
// digits; the operand of binaryNN_to_bidNN is one too, or text that the
// harness converts to binary first), and to_string's an Intel-style string
// such as "+55E+0" or "-Inf". The status is hex digits with an optional
// 0x: 0x01 invalid, 0x04 division by zero, 0x08 overflow, 0x10 underflow,
// 0x20 inexact (0x02 is a binary-operand flag the contract does not have).
// Trailing fields are "ulp=x" (the error of an approximate result; the
// runner runs a vector with a zero ulp, which is exact, and skips one with
// a non-zero ulp), "underflow_before_only" (a remark on the status: the
// contract detects tininess before rounding) and "longintsize=n" (lrint
// and lround, which are skipped).
//
// The mapping, by Intel function:
//
//	add, sub, mul, div, fma         Add, Sub, Mul, Div, FMA at the line's mode
//	bid128dd_add, bid128dqd_fma ... the decimal64 operands widened (exact),
//	                                then the decimal128 operation: one rounding
//	rem, fmod                       Rem (IEEE remainder), Mod (truncated)
//	quantize                        Quantize
//	round_integral_{nearest_even,   RoundToIntegral in the mode named; _exact
//	  nearest_away,zero,positive,   and nearbyint in the line's mode
//	  negative,exact}, nearbyint
//	negate, abs, copySign           Neg, Abs, CopySign
//	minnum, maxnum                  Min, Max, with the divergences of
//	                                minMaxDivergence
//	quiet_*, signaling_*            Equal, Less, LessEqual and Compare; the two
//	                                prefixes differ only in flags
//	sameQuantum, totalOrder,        SameQuantum, TotalOrder (totalOrderMag on
//	  totalOrderMag                 the absolute values)
//	isCanonical, isFinite, isInf,   IsCanonical, IsFinite, IsInf, IsNaN,
//	  isNaN, isNormal, isSignaling, (finite, non-zero, not subnormal),
//	  isSigned, isSubnormal, isZero IsSignaling, Signbit, IsSubnormal, IsZero
//	class                           the class from the predicates
//	to_int{8,16,32,64}_*,           Int64From, with the rounding the suffix
//	  to_uint{8,16,32,64}_*,        names (int toward zero, rnint nearest
//	  llrint, llround               even, rninta nearest away, floor, ceil;
//	                                an x also expects inexact) and a range
//	                                check; Uint64From for the unsigned
//	from_{int,uint}{32,64}          Int64To, Uint64To; the status through Round
//	to_binary{32,64}                Float32From, Float64From
//	binary{32,64}_to_bid{64,128}    Float32To, Float64To
//	bid64_to_bid128, bid128_to_bid64 Widen, Narrow
//	to_string                       Unpack, written in Intel's notation
//	from_string                     Parse, with its Status
//
// Status. A comparison of flags is made where the kernel returns a Status:
// from_string (Parse), the conversions to integers (Int64From and
// Uint64From), and the conversions from integers (Round). Every other
// operation returns no Status, so its vector passes on the result alone;
// those passes are counted under a note that says so. An integer
// conversion of a value out of range, a NaN or an infinity is expected to
// raise invalid and return the integer indefinite, where the contract
// saturates and reports Overflow, or reports Invalid and returns 0 for a
// NaN: the runner requires the kernel to report one of those and does not
// compare the value, and counts the pass under a note. A conversion whose
// name has no x raises no inexact. A few vectors print a uint32 result of
// 2^31 or more as the negative int32 with the same bits, and the runner
// reads both.
//
// Results are compared bit for bit, except that any NaN expected matches
// any NaN got (counted as "NaN modulo payload"). Min and Max, and
// TotalOrder on NaNs of one sign, are skipped where they diverge from
// minNum and maxNum and from clause 5.10 in the documented way and in no
// other. Each skip has a reason, counted. Not mapped, each with its own
// reason: the C99 functions and transcendental functions, sqrt, lrint,
// lround, scalbn and scalbln, ldexp, fdim, frexp, modf, ilogb, logb, the
// next functions, minNumMag and maxNumMag, quantum and quantexp, copy,
// the inf and nan constructors, the library's flag and rounding-attribute
// functions, DPD and strtod; the bid32 and binary80 and binary128 formats;
// a mixed-width function with a decimal64 result and a decimal128 operand,
// which would need one rounding from two operands of the wider format
// where the kernel would round twice; and the vectors recorded as
// erroneous in the file (the count is in the run's output, with the line
// number and the justification).
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
	"errors"
	"flag"
	"fmt"
	"internal/bid"
	"io"
	"math"
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
	loose bool   // passed by NaN tolerance
	note  string // non-empty: passed, but did not check this
	got   string
}

func vecSkip(reason string) vecOutcome { return vecOutcome{skip: reason} }

// vecTally counts the vectors of one file or of the whole run.
type vecTally struct {
	pass, fail, loose int
	skip              map[string]int
	note              map[string]int // passes by what they did not check
	failByOp          map[string]int
}

func newVecTally() *vecTally {
	return &vecTally{skip: map[string]int{}, note: map[string]int{}, failByOp: map[string]int{}}
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
		if o.note != "" {
			t.note[o.note]++
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
	for k, n := range u.note {
		t.note[k] += n
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
	notes := make([]string, 0, len(t.note))
	for r := range t.note {
		notes = append(notes, r)
	}
	sort.Slice(notes, func(i, j int) bool {
		if t.note[notes[i]] != t.note[notes[j]] {
			return t.note[notes[i]] > t.note[notes[j]]
		}
		return notes[i] < notes[j]
	})
	for _, r := range notes {
		fmt.Fprintf(&sb, "\n    note %-6d %s", t.note[r], r)
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

// The divergences of Min and Max from GDA and from IEEE 754-2008 minNum and
// maxNum, named in the package documentation under "Minimum and maximum".
const (
	skipMinMaxNaN    = "contract: Min/Max return NaN if either operand is NaN (GDA returns the number)"
	skipMinMaxCohort = "contract: Min/Max return x for operands equal in value and sign (GDA chooses by exponent)"
	skipMinMaxZero   = "contract: Min/Max order -0 before +0 (IEEE minNum and maxNum leave the zero's sign open)"
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
//   - Zero: the operands are zeros of opposite signs and the vector expects
//     one of them where the target returned the other (the contract puts -0
//     before +0; IEEE 754-2008 clause 5.3.1 allows either result).
//
// The operand returned is canonicalized (see "Minimum and maximum"), so the
// expected value is compared with the canonical form of the operand. want
// and got are the expected and obtained encodings.
func minMaxDivergence(f *format, x, y, want, got bits) string {
	canon := func(b bits) bits { return f.encode(f.decode(b)) }
	a, b := f.decode(x), f.decode(y)
	switch {
	case (a.kind == notANumber) != (b.kind == notANumber):
		number := x
		if a.kind == notANumber {
			number = y
		}
		if want == canon(number) && f.decode(got).kind == notANumber {
			return skipMinMaxNaN
		}
	case a.kind == finite && b.kind == finite && a.neg == b.neg && cmpValue(a, b) == 0:
		if want == canon(y) && got == canon(x) && canon(x) != canon(y) {
			return skipMinMaxCohort
		}
	case a.kind == finite && b.kind == finite && a.neg != b.neg && a.coeff.Sign() == 0 && b.coeff.Sign() == 0:
		if (want == canon(x) && got == canon(y)) || (want == canon(y) && got == canon(x)) {
			return skipMinMaxZero
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
		{"opposite zeros, the other zero expected", d("0"), d("-0"), d("0"), d("-0"), skipMinMaxZero},
		{"opposite zeros, a third value", d("0"), d("-0"), d("1"), d("-0"), ""},
		{"opposite zeros, the target returned a third value", d("0"), d("-0"), d("0"), d("0.0"), ""},
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

// The status flags of the Intel library (the BID_*_EXCEPTION bits). The
// denormal bit, 0x02, is a binary-operand flag that the contract does not
// have.
const (
	intelInvalid   = 0x01
	intelOverflow  = 0x08
	intelUnderflow = 0x10
	intelInexact   = 0x20
)

// The class codes of the Intel library's class function, which are the
// classes of IEEE 754-2008 clause 5.7.2 numbered from the signaling NaN.
const (
	intelClassSNaN = iota
	intelClassQNaN
	intelClassNegInf
	intelClassNegNormal
	intelClassNegSubnormal
	intelClassNegZero
	intelClassPosZero
	intelClassPosSubnormal
	intelClassPosNormal
	intelClassPosInf
)

// Notes on passes: what a passing vector did not check, counted by name.
const (
	noteNoStatus = "result matches; status flags not compared (the kernel operation returns no Status)"
	noteIntInv   = "result matches; the value of an out-of-range integer conversion not compared (Intel: the integer indefinite; contract: saturation)"
	noteSyntax   = "result matches; Syntax status not compared (Intel reports no flag for a malformed numeral)"
	noteString   = "result matches through the sign, coefficient and exponent (Intel's notation differs from Append's)"
)

// intelVector is one readtest.in line: the function name, the rounding code
// and every field after them (operands, expected result, expected status and
// optional trailing fields). The fields were split at white space; a "--"
// starts a comment.
type intelVector struct {
	line   int
	name   string
	round  string
	fields []string
}

// parseIntelLine parses one line of readtest.in. ok is false for blank lines
// and comment lines.
func parseIntelLine(text string, n int) (*intelVector, bool) {
	toks := vecTokens(text)
	if len(toks) == 0 {
		return nil, false
	}
	v := &intelVector{line: n, name: toks[0].s}
	if len(toks) > 1 {
		v.round = toks[1].s
		for _, tk := range toks[2:] {
			v.fields = append(v.fields, tk.s)
		}
	}
	return v, true
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

// intelHex reads a bracketed hex encoding of the given width: exactly
// width/4 digits, or for 128 bits two groups of 16 digits separated by a
// comma. Digits may be of either case.
func intelHex(s string, width int) (bits, bool) {
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return bits{}, false
	}
	s = s[1 : len(s)-1]
	if width == 128 {
		if hi, lo, ok := strings.Cut(s, ","); ok {
			s = hi + lo
			if len(hi) != 16 || len(lo) != 16 {
				return bits{}, false
			}
		}
	}
	if len(s) != width/4 {
		return bits{}, false
	}
	if width == 64 {
		lo, err := strconv.ParseUint(s, 16, 64)
		return bits{Lo: lo}, err == nil
	}
	hi, err1 := strconv.ParseUint(s[:16], 16, 64)
	lo, err2 := strconv.ParseUint(s[16:], 16, 64)
	return bits{Hi: hi, Lo: lo}, err1 == nil && err2 == nil
}

// intelSignalingNaN is the positive signaling NaN with a zero payload.
func intelSignalingNaN(f *format) bits {
	if f.width == 64 {
		return bits{Lo: 0x7e << 56}
	}
	return bits{Hi: 0x7e << 56}
}

// intelDecimal reads a decimal operand or expected result of format f. The
// readtest file writes one either as a bracketed hex encoding, or as text
// that the harness converts with the library's own string conversion, to
// nearest even whatever the line's rounding code: a numeral, or one of the
// words Inf, Infinity, NaN, QNaN and SNaN in either case, each with an
// optional sign. The numeral is read with the reference model's codec.
// (A directed mode here made the operand of a directed-mode vector differ
// from the library's by one unit in the last place, and overflow where the
// library gets an infinity.)
func intelDecimal(f *format, tok string) (bits, string) {
	if strings.HasPrefix(tok, "[") {
		x, ok := intelHex(tok, f.width)
		switch {
		case ok:
			return x, ""
		case !strings.HasSuffix(tok, "]"):
			return bits{}, "malformed line: hex literal with no closing bracket"
		}
		return bits{}, "malformed line: hex literal is not of the operand's width"
	}
	body := strings.TrimLeft(tok, "+-")
	neg := strings.HasPrefix(tok, "-")
	if len(tok)-len(body) > 1 {
		return bits{}, "operand: text outside the contract's numeral grammar"
	}
	var x bits
	switch strings.ToLower(body) {
	case "nan", "qnan":
		x = f.encode(nan)
	case "snan":
		x = intelSignalingNaN(f)
	case "inf", "infinity":
		x = f.encode(value{kind: infinite})
	default:
		var st bid.Status
		if x, st = f.parse(tok, bid.ToNearestEven); st&bid.Syntax != 0 {
			return bits{}, "operand: text outside the contract's numeral grammar"
		}
		return x, ""
	}
	if neg {
		x = vecRef.neg(f, x)
	}
	return x, ""
}

// intelFlags renders a Status as the Intel flags it corresponds to. Syntax
// has no flag.
func intelFlags(st bid.Status) uint64 {
	var m uint64
	if st&bid.Invalid != 0 {
		m |= intelInvalid
	}
	if st&bid.Overflow != 0 {
		m |= intelOverflow
	}
	if st&bid.Underflow != 0 {
		m |= intelUnderflow
	}
	if st&bid.Inexact != 0 {
		m |= intelInexact
	}
	return m
}

// intelSkipOps are the functions of the file that have no counterpart in
// the kernel API, each with the reason that is counted.
var intelSkipOps = map[string]string{
	"sqrt":    "op: square root (not in the contract)",
	"atan2":   "op: atan2 (not in the contract)",
	"lrint":   "op: lrint (depends on the C long size; the 64-bit form is llrint)",
	"lround":  "op: lround (depends on the C long size; the 64-bit form is llround)",
	"scalbn":  "op: scalbn (not in the contract)",
	"scalbln": "op: scalbln (not in the contract)",
	"ldexp":   "op: ldexp (not in the contract)",

	"acos": "op: transcendental function (not in the contract)", "acosh": "op: transcendental function (not in the contract)",
	"asin": "op: transcendental function (not in the contract)", "asinh": "op: transcendental function (not in the contract)",
	"atan": "op: transcendental function (not in the contract)", "atanh": "op: transcendental function (not in the contract)",
	"cos": "op: transcendental function (not in the contract)", "cosh": "op: transcendental function (not in the contract)",
	"sin": "op: transcendental function (not in the contract)", "sinh": "op: transcendental function (not in the contract)",
	"tan": "op: transcendental function (not in the contract)", "tanh": "op: transcendental function (not in the contract)",
	"exp": "op: transcendental function (not in the contract)", "exp2": "op: transcendental function (not in the contract)",
	"exp10": "op: transcendental function (not in the contract)", "expm1": "op: transcendental function (not in the contract)",
	"log": "op: transcendental function (not in the contract)", "log2": "op: transcendental function (not in the contract)",
	"log10": "op: transcendental function (not in the contract)", "log1p": "op: transcendental function (not in the contract)",
	"pow": "op: transcendental function (not in the contract)", "cbrt": "op: transcendental function (not in the contract)",
	"hypot": "op: transcendental function (not in the contract)", "erf": "op: transcendental function (not in the contract)",
	"erfc": "op: transcendental function (not in the contract)", "lgamma": "op: transcendental function (not in the contract)",
	"tgamma": "op: transcendental function (not in the contract)",

	"fdim":       "op: fdim (not in the contract)",
	"frexp":      "op: frexp (not in the contract)",
	"modf":       "op: modf (not in the contract)",
	"ilogb":      "op: ilogb (not in the contract)",
	"logb":       "op: logb (not in the contract)",
	"nextafter":  "op: nextUp, nextDown, nextAfter (not in the contract)",
	"nexttoward": "op: nextUp, nextDown, nextAfter (not in the contract)",
	"nextup":     "op: nextUp, nextDown, nextAfter (not in the contract)",
	"nextdown":   "op: nextUp, nextDown, nextAfter (not in the contract)",

	"minnum_mag": "op: minNumMag, maxNumMag (not in the contract)",
	"maxnum_mag": "op: minNumMag, maxNumMag (not in the contract)",
	"quantexp":   "op: quantum, quantexp, radix (inspection of the format, not in the contract)",
	"llquantexp": "op: quantum, quantexp, radix (inspection of the format, not in the contract)",
	"quantum":    "op: quantum, quantexp, radix (inspection of the format, not in the contract)",
	"radix":      "op: quantum, quantexp, radix (inspection of the format, not in the contract)",
	"copy":       "op: copy (the identity; the kernel has no function for it)",
	"inf":        "op: inf and nan constructors (no kernel counterpart)",
	"nan":        "op: inf and nan constructors (no kernel counterpart)",
}

var (
	intelName     = regexp.MustCompile(`^bid(32|64|128)([dq]*)_(\w+)$`)
	intelBinary   = regexp.MustCompile(`^binary(32|64|80|128)_to_bid(32|64|128)$`)
	intelToInt    = regexp.MustCompile(`^to_(u?)int(8|16|32|64)_(x?)(int|rnint|rninta|floor|ceil)$`)
	intelFromInt  = regexp.MustCompile(`^from_(u?)int(32|64)$`)
	intelToBinary = regexp.MustCompile(`^to_binary(32|64|80|128)$`)
)

// intelIntModes maps the rounding part of a conversion to integer's name to
// the kernel's mode.
var intelIntModes = map[string]bid.Mode{
	"int":    bid.TowardZero,
	"rnint":  bid.ToNearestEven,
	"rninta": bid.ToNearestAway,
	"floor":  bid.TowardNegative,
	"ceil":   bid.TowardPositive,
}

// intelPredicates are the comparison predicates of clause 5.11, named as the
// file names them after "quiet_" and "signaling_". The two prefixes differ
// only in the flags they raise.
var intelPredicates = map[string]func(im impl, f *format, x, y bits) bool{
	"equal":             func(im impl, f *format, x, y bits) bool { return im.equal(f, x, y) },
	"not_equal":         func(im impl, f *format, x, y bits) bool { return !im.equal(f, x, y) },
	"greater":           func(im impl, f *format, x, y bits) bool { return im.less(f, y, x) },
	"greater_equal":     func(im impl, f *format, x, y bits) bool { return im.lessEqual(f, y, x) },
	"greater_unordered": func(im impl, f *format, x, y bits) bool { return !im.lessEqual(f, x, y) },
	"less":              func(im impl, f *format, x, y bits) bool { return im.less(f, x, y) },
	"less_equal":        func(im impl, f *format, x, y bits) bool { return im.lessEqual(f, x, y) },
	"less_unordered":    func(im impl, f *format, x, y bits) bool { return !im.lessEqual(f, y, x) },
	"not_greater":       func(im impl, f *format, x, y bits) bool { return !im.less(f, y, x) },
	"not_less":          func(im impl, f *format, x, y bits) bool { return !im.less(f, x, y) },
	"ordered": func(im impl, f *format, x, y bits) bool {
		_, ok := im.compare(f, x, y)
		return ok
	},
	"unordered": func(im impl, f *format, x, y bits) bool {
		_, ok := im.compare(f, x, y)
		return !ok
	},
}

// intelClassOps are the one-operand predicates of the file.
var intelClassOps = map[string]func(im impl, f *format, x bits) bool{
	"isCanonical": func(im impl, f *format, x bits) bool { return im.isCanonical(f, x) },
	"isFinite":    func(im impl, f *format, x bits) bool { return im.isFinite(f, x) },
	"isInf":       func(im impl, f *format, x bits) bool { return im.isInf(f, x) },
	"isNaN":       func(im impl, f *format, x bits) bool { return im.isNaN(f, x) },
	"isSignaling": func(im impl, f *format, x bits) bool { return im.isSignaling(f, x) },
	"isSigned":    func(im impl, f *format, x bits) bool { return im.signbit(f, x) },
	"isSubnormal": func(im impl, f *format, x bits) bool { return im.isSubnormal(f, x) },
	"isZero":      func(im impl, f *format, x bits) bool { return im.isZero(f, x) },
	"isNormal": func(im impl, f *format, x bits) bool {
		return im.isFinite(f, x) && !im.isZero(f, x) && !im.isSubnormal(f, x)
	},
}

// intelClass is the class function of the file for x.
func intelClass(im impl, f *format, x bits) int {
	neg := im.signbit(f, x)
	pick := func(pos, negative int) int {
		if neg {
			return negative
		}
		return pos
	}
	switch {
	case im.isSignaling(f, x):
		return intelClassSNaN
	case im.isNaN(f, x):
		return intelClassQNaN
	case im.isInf(f, x):
		return pick(intelClassPosInf, intelClassNegInf)
	case im.isZero(f, x):
		return pick(intelClassPosZero, intelClassNegZero)
	case im.isSubnormal(f, x):
		return pick(intelClassPosSubnormal, intelClassNegSubnormal)
	}
	return pick(intelClassPosNormal, intelClassNegNormal)
}

// intelCall is one vector in the course of being evaluated.
type intelCall struct {
	im   impl
	v    *intelVector
	op   string    // the function name after the format prefix
	f    *format   // the format of the result (the prefix of the name)
	args []*format // the format of each operand: f unless the name has d and q letters
	mode bid.Mode
}

// split divides the fields of the vector into n operands, the expected
// result, the expected status flags and the trailing fields, and checks the
// trailing fields. A non-empty skip is the reason the vector cannot run.
func (c *intelCall) split(n int) (ops []string, res string, want uint64, skip string) {
	fields := c.v.fields
	if len(fields) < n+2 {
		return nil, "", 0, "malformed line: fewer fields than the function takes"
	}
	st, err := strconv.ParseUint(strings.TrimPrefix(fields[n+1], "0x"), 16, 16)
	if err != nil {
		return nil, "", 0, "malformed line: the status field is not hexadecimal"
	}
	for _, e := range fields[n+2:] {
		switch {
		case strings.HasPrefix(e, "ulp="):
			if u, err := strconv.ParseFloat(e[len("ulp="):], 64); err != nil {
				return nil, "", 0, "field: unreadable ulp column"
			} else if u != 0 {
				return nil, "", 0, "field: non-zero ulp column (the expected result is an approximation)"
			}
		case e == "underflow_before_only":
			// A note on the status flags: tininess is judged before
			// rounding, as the contract does.
		case e == "undefrlow_before_only":
			return nil, "", 0, fmt.Sprintf("erroneous vector readtest.in:%d (%s): the trailing field is a misspelling of underflow_before_only", c.v.line, c.v.name)
		default:
			return nil, "", 0, "field: unrecognized trailing field"
		}
	}
	return fields[:n], fields[n], st, ""
}

// operands reads the operands in their own formats and widens them to the
// result's format. A decimal128 operand of a decimal64 result would need a
// second rounding in the kernel, so the vector is skipped.
func (c *intelCall) operands(ops []string) ([]bits, string) {
	out := make([]bits, len(ops))
	for i, tok := range ops {
		x, why := intelDecimal(c.args[i], tok)
		if why != "" {
			return nil, why
		}
		switch {
		case c.args[i] == c.f:
		case c.args[i] == dec64 && c.f == dec128:
			x = c.im.widen(x)
		default:
			return nil, "kernel: mixed-width form with a decimal64 result needs one rounding from decimal128 operands (the kernel would round twice)"
		}
		out[i] = x
	}
	return out, ""
}

// run calls fn, turning a kernel that is not implemented into a skip and a
// panic into a failure. ok is false when o is the outcome to return.
func intelRun(fn func()) (o vecOutcome, ok bool) {
	implemented, err := try(fn)
	switch {
	case !implemented:
		return vecSkip("kernel: not implemented"), false
	case err != nil:
		return vecOutcome{got: err.Error()}, false
	}
	return vecOutcome{}, true
}

// noted marks a passing outcome with what it did not check.
func (o vecOutcome) noted(note string) vecOutcome {
	if o.pass {
		o.note = note
	}
	return o
}

// evalIntel runs one readtest.in vector against im.
func evalIntel(im impl, v *intelVector) vecOutcome {
	if v.round == "" {
		return vecSkip("malformed line: no rounding code")
	}
	if m := intelBinary.FindStringSubmatch(v.name); m != nil {
		return evalIntelFromBinary(im, v, m[1], m[2])
	}
	switch {
	case strings.HasPrefix(v.name, "bid_"):
		return vecSkip("op: library state, rounding attribute, flag, DPD or strtod function (not a decimal operation)")
	case v.name == "str64":
		return vecSkip("op: str64 (a string round trip of the C library)")
	}
	m := intelName.FindStringSubmatch(v.name)
	if m == nil {
		return vecSkip("op: unknown function")
	}
	width, letters, op := m[1], m[2], m[3]
	switch {
	case width == "32":
		return vecSkip("format: bid32 (not decimal64 or decimal128)")
	case intelToBinary.MatchString(op) && (strings.HasSuffix(op, "80") || strings.HasSuffix(op, "128")):
		return vecSkip("format: binary" + strings.TrimPrefix(op, "to_binary") + " (not a binary format of the contract)")
	case op == "to_bid32":
		return vecSkip("format: bid32 (not decimal64 or decimal128)")
	}
	if why, ok := intelSkipOps[op]; ok {
		return vecSkip(why)
	}
	mode, ok := intelMode(v.round)
	if !ok {
		return vecSkip("context: rounding code unknown")
	}
	c := &intelCall{im: im, v: v, op: op, f: dec64, mode: mode}
	if width == "128" {
		c.f = dec128
	}
	for _, l := range letters {
		if l == 'd' {
			c.args = append(c.args, dec64)
		} else {
			c.args = append(c.args, dec128)
		}
	}
	arity := map[string]int{"add": 2, "sub": 2, "mul": 2, "div": 2, "fma": 3}[op]
	switch {
	case letters != "" && (arity == 0 || len(letters) != arity):
		return vecSkip("op: mixed-width form not mapped")
	case letters == "":
		for range 3 {
			c.args = append(c.args, c.f)
		}
	}
	return c.eval()
}

// eval dispatches on the function name.
func (c *intelCall) eval() vecOutcome {
	im, f, mode := c.im, c.f, c.mode
	op := c.op
	switch op {
	case "add":
		return c.arith(2, func(a []bits) bits { return im.add(f, a[0], a[1], mode) })
	case "sub":
		return c.arith(2, func(a []bits) bits { return im.sub(f, a[0], a[1], mode) })
	case "mul":
		return c.arith(2, func(a []bits) bits { return im.mul(f, a[0], a[1], mode) })
	case "div":
		return c.arith(2, func(a []bits) bits { return im.div(f, a[0], a[1], mode) })
	case "fma":
		return c.arith(3, func(a []bits) bits { return im.fma(f, a[0], a[1], a[2], mode) })
	case "rem":
		return c.arith(2, func(a []bits) bits { return im.rem(f, a[0], a[1]) })
	case "fmod":
		return c.arith(2, func(a []bits) bits { return im.mod(f, a[0], a[1]) })
	case "quantize":
		return c.arith(2, func(a []bits) bits { return im.quantize(f, a[0], a[1], mode) })
	case "negate":
		return c.arith(1, func(a []bits) bits { return im.neg(f, a[0]) })
	case "abs":
		return c.arith(1, func(a []bits) bits { return im.abs(f, a[0]) })
	case "copySign":
		return c.arith(2, func(a []bits) bits { return im.copySign(f, a[0], a[1]) })
	case "round_integral_exact", "nearbyint":
		return c.arith(1, func(a []bits) bits { return im.roundToIntegral(f, a[0], mode) })
	case "round_integral_nearest_even":
		return c.arith(1, func(a []bits) bits { return im.roundToIntegral(f, a[0], bid.ToNearestEven) })
	case "round_integral_nearest_away":
		return c.arith(1, func(a []bits) bits { return im.roundToIntegral(f, a[0], bid.ToNearestAway) })
	case "round_integral_zero":
		return c.arith(1, func(a []bits) bits { return im.roundToIntegral(f, a[0], bid.TowardZero) })
	case "round_integral_positive":
		return c.arith(1, func(a []bits) bits { return im.roundToIntegral(f, a[0], bid.TowardPositive) })
	case "round_integral_negative":
		return c.arith(1, func(a []bits) bits { return im.roundToIntegral(f, a[0], bid.TowardNegative) })
	case "minnum":
		return c.minMax(false)
	case "maxnum":
		return c.minMax(true)
	case "sameQuantum":
		return c.predicate(2, func(a []bits) bool { return im.sameQuantum(f, a[0], a[1]) })
	case "totalOrder":
		return c.totalOrder(false)
	case "totalOrderMag":
		return c.totalOrder(true)
	case "class":
		return c.class()
	case "to_bid128", "to_bid64":
		return c.convertWidth()
	case "to_string":
		return c.toString()
	case "from_string":
		return c.fromString()
	case "llrint":
		return c.toInt(false, 64, true, mode)
	case "llround":
		return c.toInt(false, 64, false, bid.ToNearestAway)
	}
	if fn, ok := intelClassOps[op]; ok {
		return c.predicate(1, func(a []bits) bool { return fn(im, f, a[0]) })
	}
	for _, prefix := range []string{"quiet_", "signaling_"} {
		if rest, ok := strings.CutPrefix(op, prefix); ok {
			if fn, ok := intelPredicates[rest]; ok {
				return c.predicate(2, func(a []bits) bool { return fn(im, f, a[0], a[1]) })
			}
		}
	}
	if m := intelToInt.FindStringSubmatch(op); m != nil {
		n, _ := strconv.Atoi(m[2])
		return c.toInt(m[1] == "u", n, m[3] == "x", intelIntModes[m[4]])
	}
	if m := intelFromInt.FindStringSubmatch(op); m != nil {
		n, _ := strconv.Atoi(m[2])
		return c.fromInt(m[1] == "u", n)
	}
	if m := intelToBinary.FindStringSubmatch(op); m != nil {
		return c.toBinary(m[1])
	}
	return vecSkip("op: not mapped to the kernel API")
}

// arith runs a function of n decimal operands with a decimal result.
func (c *intelCall) arith(n int, fn func(a []bits) bits) vecOutcome {
	ops, res, _, skip := c.split(n)
	if skip != "" {
		return vecSkip(skip)
	}
	a, why := c.operands(ops)
	if why != "" {
		return vecSkip(why)
	}
	want, why := intelDecimal(c.f, res)
	if why != "" {
		return vecSkip(why)
	}
	var got bits
	if o, ok := intelRun(func() { got = fn(a) }); !ok {
		return o
	}
	return intelCompare(c.f, got, want).noted(noteNoStatus)
}

// minMax runs minnum and maxnum, which differ from the contract's Min and
// Max on a NaN operand and on a cohort (see minMaxDivergence).
func (c *intelCall) minMax(max bool) vecOutcome {
	ops, res, _, skip := c.split(2)
	if skip != "" {
		return vecSkip(skip)
	}
	a, why := c.operands(ops)
	if why != "" {
		return vecSkip(why)
	}
	want, why := intelDecimal(c.f, res)
	if why != "" {
		return vecSkip(why)
	}
	var got bits
	if o, ok := intelRun(func() {
		if max {
			got = c.im.max(c.f, a[0], a[1])
		} else {
			got = c.im.min(c.f, a[0], a[1])
		}
	}); !ok {
		return o
	}
	out := intelCompare(c.f, got, want)
	if !out.pass {
		if why := minMaxDivergence(c.f, a[0], a[1], want, got); why != "" {
			return vecSkip(why)
		}
	}
	return out.noted(noteNoStatus)
}

// predicate runs a function of n operands with a 0 or 1 result.
func (c *intelCall) predicate(n int, fn func(a []bits) bool) vecOutcome {
	ops, res, _, skip := c.split(n)
	if skip != "" {
		return vecSkip(skip)
	}
	a, why := c.operands(ops)
	if why != "" {
		return vecSkip(why)
	}
	if res != "0" && res != "1" {
		return vecSkip("malformed line: a predicate's result is not 0 or 1")
	}
	var got bool
	if o, ok := intelRun(func() { got = fn(a) }); !ok {
		return o
	}
	gotText := "0"
	if got {
		gotText = "1"
	}
	return vecOutcome{pass: gotText == res, got: gotText}.noted(noteNoStatus)
}

// totalOrder runs totalOrder and totalOrderMag. The contract orders a NaN by
// its sign alone, so two NaNs of one sign are ordered both ways; the
// standard also orders by signaling bit and payload. A vector that expects
// that order is the documented divergence.
func (c *intelCall) totalOrder(mag bool) vecOutcome {
	ops, res, _, skip := c.split(2)
	if skip != "" {
		return vecSkip(skip)
	}
	a, why := c.operands(ops)
	if why != "" {
		return vecSkip(why)
	}
	if res != "0" && res != "1" {
		return vecSkip("malformed line: a predicate's result is not 0 or 1")
	}
	if mag {
		a[0], a[1] = c.im.abs(c.f, a[0]), c.im.abs(c.f, a[1])
	}
	var got bool
	if o, ok := intelRun(func() { got = c.im.totalOrder(c.f, a[0], a[1]) }); !ok {
		return o
	}
	gotText := "0"
	if got {
		gotText = "1"
	}
	out := vecOutcome{pass: gotText == res, got: gotText}
	if !out.pass && vecRef.isNaN(c.f, a[0]) && vecRef.isNaN(c.f, a[1]) &&
		vecRef.signbit(c.f, a[0]) == vecRef.signbit(c.f, a[1]) {
		return vecSkip("contract: TotalOrder ignores the signaling bit and the payload of NaNs of one sign")
	}
	return out.noted(noteNoStatus)
}

// class runs class, whose result is a class code.
func (c *intelCall) class() vecOutcome {
	ops, res, _, skip := c.split(1)
	if skip != "" {
		return vecSkip(skip)
	}
	a, why := c.operands(ops)
	if why != "" {
		return vecSkip(why)
	}
	var got int
	if o, ok := intelRun(func() { got = intelClass(c.im, c.f, a[0]) }); !ok {
		return o
	}
	return vecOutcome{pass: strconv.Itoa(got) == res, got: strconv.Itoa(got)}.noted(noteNoStatus)
}

// convertWidth runs bid64_to_bid128 (Widen) and bid128_to_bid64 (Narrow in
// the line's mode). The name's prefix is the operand's format.
func (c *intelCall) convertWidth() vecOutcome {
	to := dec128
	if c.op == "to_bid64" {
		to = dec64
	}
	if to == c.f {
		return vecSkip("op: not mapped to the kernel API")
	}
	ops, res, _, skip := c.split(1)
	if skip != "" {
		return vecSkip(skip)
	}
	x, why := intelDecimal(c.f, ops[0])
	if why != "" {
		return vecSkip(why)
	}
	want, why := intelDecimal(to, res)
	if why != "" {
		return vecSkip(why)
	}
	var got bits
	if o, ok := intelRun(func() {
		if to == dec128 {
			got = c.im.widen(x)
		} else {
			got = c.im.narrow(x, c.mode)
		}
	}); !ok {
		return o
	}
	return intelCompare(to, got, want).noted(noteNoStatus)
}

// toInt runs the conversions to an integer of the given width, signedness
// and rounding. A conversion whose name has an x raises inexact; the others
// do not, so their inexact flag is not expected.
func (c *intelCall) toInt(unsigned bool, n int, inexact bool, mode bid.Mode) vecOutcome {
	ops, res, want, skip := c.split(1)
	if skip != "" {
		return vecSkip(skip)
	}
	x, why := intelDecimal(c.f, ops[0])
	if why != "" {
		return vecSkip(why)
	}
	var (
		sv  int64
		uv  uint64
		st  bid.Status
		out bool // the rounded value is outside the integer type
	)
	if o, ok := intelRun(func() {
		if unsigned {
			uv, st = c.im.uint64From(c.f, x, mode)
			out = st&(bid.Invalid|bid.Overflow) != 0 || n < 64 && uv>>n != 0
		} else {
			sv, st = c.im.int64From(c.f, x, mode)
			out = st&(bid.Invalid|bid.Overflow) != 0 || n < 64 && (sv < -1<<(n-1) || sv > 1<<(n-1)-1)
		}
	}); !ok {
		return o
	}
	gotText := fmt.Sprintf("%d %s", sv, statusString(st))
	if unsigned {
		gotText = fmt.Sprintf("%d %s", uv, statusString(st))
	}
	if out || want&intelInvalid != 0 {
		// Intel returns the integer indefinite for every operand out of
		// range; the contract saturates, or returns 0 for a NaN. The value
		// is not comparable, so only the report is.
		return vecOutcome{pass: out && want&intelInvalid != 0, got: gotText}.noted(noteIntInv)
	}
	if want&^intelInexact != 0 {
		return vecOutcome{got: gotText + " (Intel status has other flags)"}
	}
	var same bool
	if unsigned {
		w, err := strconv.ParseUint(res, 10, 64)
		if i, err2 := strconv.ParseInt(res, 10, 64); err != nil && err2 == nil && n < 64 && i >= -1<<(n-1) {
			// Some vectors print a result of 2^(n-1) or more as the
			// negative integer with the same n bits.
			w, err = uint64(i)&(1<<n-1), nil
		}
		if err != nil {
			return vecSkip("malformed line: the integer result is unreadable")
		}
		same = w == uv
	} else {
		w, err := strconv.ParseInt(res, 10, 64)
		if err != nil {
			return vecSkip("malformed line: the integer result is unreadable")
		}
		same = w == sv
	}
	wantInexact := want&intelInexact != 0
	gotInexact := st&bid.Inexact != 0
	if !inexact && wantInexact {
		return vecOutcome{got: gotText + " (Intel raised inexact for a conversion without x)"}
	}
	return vecOutcome{pass: same && (!inexact || wantInexact == gotInexact), got: gotText}
}

// fromInt runs the conversions from an integer type to decimal; the status
// is compared through Round, which reports one.
func (c *intelCall) fromInt(unsigned bool, n int) vecOutcome {
	ops, res, want, skip := c.split(1)
	if skip != "" {
		return vecSkip(skip)
	}
	var (
		neg bool
		mag uint64
	)
	if unsigned {
		u, err := strconv.ParseUint(ops[0], 10, n)
		if err != nil {
			return vecSkip("malformed line: the integer operand is unreadable or out of its type")
		}
		mag = u
	} else {
		i, err := strconv.ParseInt(ops[0], 10, n)
		if err != nil {
			return vecSkip("malformed line: the integer operand is unreadable or out of its type")
		}
		neg = i < 0
		mag = uint64(i)
		if neg {
			mag = -mag
		}
	}
	wantBits, why := intelDecimal(c.f, res)
	if why != "" {
		return vecSkip(why)
	}
	var (
		got bits
		st  bid.Status
	)
	if o, ok := intelRun(func() {
		switch {
		case unsigned:
			got = c.im.uint64To(c.f, mag, c.mode)
		default:
			i := int64(mag)
			if neg {
				i = -int64(mag)
			}
			got = c.im.int64To(c.f, i, c.mode)
		}
		_, st = c.im.round(c.f, neg, bid.Uint128{Lo: mag}, 0, false, c.mode)
	}); !ok {
		return o
	}
	out := intelCompare(c.f, got, wantBits)
	if out.pass && intelFlags(st) != want {
		return vecOutcome{got: out.got + " " + statusString(st)}
	}
	return out
}

// toBinary runs the conversions to binary32 and binary64.
func (c *intelCall) toBinary(width string) vecOutcome {
	ops, res, _, skip := c.split(1)
	if skip != "" {
		return vecSkip(skip)
	}
	x, why := intelDecimal(c.f, ops[0])
	if why != "" {
		return vecSkip(why)
	}
	n := 64
	if width == "32" {
		n = 32
	}
	want, ok := intelBinaryBits(res, n)
	if !ok {
		return vecSkip("malformed line: the binary result is not a hex literal of its width")
	}
	var got uint64
	if o, ok := intelRun(func() {
		if n == 32 {
			got = uint64(math.Float32bits(c.im.float32From(c.f, x, c.mode)))
		} else {
			got = math.Float64bits(c.im.float64From(c.f, x, c.mode))
		}
	}); !ok {
		return o
	}
	return intelCompareBinary(n, got, want).noted(noteNoStatus)
}

// intelBinaryBits reads a bracketed hex literal of a binary format of n bits.
func intelBinaryBits(s string, n int) (uint64, bool) {
	if len(s) != n/4+2 || s[0] != '[' || s[len(s)-1] != ']' {
		return 0, false
	}
	b, err := strconv.ParseUint(s[1:len(s)-1], 16, 64)
	return b, err == nil
}

// intelCompareBinary compares binary bit patterns, any NaN matching any NaN.
func intelCompareBinary(n int, got, want uint64) vecOutcome {
	gotText := fmt.Sprintf("%0*x", n/4, got)
	isNaN := func(b uint64) bool {
		if n == 32 {
			return math.IsNaN(float64(math.Float32frombits(uint32(b))))
		}
		return math.IsNaN(math.Float64frombits(b))
	}
	if isNaN(want) {
		return vecOutcome{pass: isNaN(got), loose: got != want, got: gotText}
	}
	return vecOutcome{pass: got == want, got: gotText}
}

// evalIntelFromBinary runs binary32_to_bid64 and its relatives. The
// operand is the bit pattern of the binary value.
func evalIntelFromBinary(im impl, v *intelVector, from, to string) vecOutcome {
	switch {
	case from == "80" || from == "128":
		return vecSkip("format: binary" + from + " (not a binary format of the contract)")
	case to == "32":
		return vecSkip("format: bid32 (not decimal64 or decimal128)")
	}
	mode, ok := intelMode(v.round)
	if !ok {
		return vecSkip("context: rounding code unknown")
	}
	f := dec64
	if to == "128" {
		f = dec128
	}
	c := &intelCall{im: im, v: v, op: v.name, f: f, mode: mode}
	ops, res, _, skip := c.split(1)
	if skip != "" {
		return vecSkip(skip)
	}
	n := 64
	if from == "32" {
		n = 32
	}
	// The operand is the bit pattern, or text that the harness converts to
	// the binary format first, to nearest even.
	var x float64
	if strings.HasPrefix(ops[0], "[") {
		b, ok := intelBinaryBits(ops[0], n)
		if !ok {
			return vecSkip("malformed line: the binary operand is not a hex literal of its width")
		}
		x = math.Float64frombits(b)
		if n == 32 {
			x = float64(math.Float32frombits(uint32(b)))
		}
	} else {
		var err error
		if x, err = strconv.ParseFloat(ops[0], n); err != nil && !errors.Is(err, strconv.ErrRange) {
			return vecSkip("operand: text outside the contract's numeral grammar")
		}
	}
	want, why := intelDecimal(f, res)
	if why != "" {
		return vecSkip(why)
	}
	var got bits
	if o, ok := intelRun(func() {
		if n == 32 {
			got = im.float32To(f, float32(x), mode)
		} else {
			got = im.float64To(f, x, mode)
		}
	}); !ok {
		return o
	}
	return intelCompare(f, got, want).noted(noteNoStatus)
}

// toString runs to_string. Intel writes a sign, the coefficient, E and a
// signed exponent ("+55E+0"), "+Inf", "+NaN" and "+SNaN", where the
// contract's Append writes the GDA form, so the check is on the operand's
// sign, coefficient and exponent.
func (c *intelCall) toString() vecOutcome {
	ops, res, _, skip := c.split(1)
	if skip != "" {
		return vecSkip(skip)
	}
	x, why := intelDecimal(c.f, ops[0])
	if why != "" {
		return vecSkip(why)
	}
	var got string
	if o, ok := intelRun(func() {
		sign := "+"
		if c.im.signbit(c.f, x) {
			sign = "-"
		}
		switch {
		case c.im.isSignaling(c.f, x):
			got = sign + "SNaN"
		case c.im.isNaN(c.f, x):
			got = sign + "NaN"
		case c.im.isInf(c.f, x):
			got = sign + "Inf"
		default:
			_, _, coeff, exp := c.im.unpack(c.f, x)
			got = fmt.Sprintf("%s%sE%+d", sign, fromUint128(coeff), exp)
		}
	}); !ok {
		return o
	}
	return vecOutcome{pass: got == res, got: got}.noted(noteString)
}

// fromString runs from_string through the target's Parse in the line's
// mode. Intel reports nothing for a malformed numeral, which the contract
// reports as Syntax, and returns NaN for it; bid128 also accepts a numeral
// with a trailing letter E, which bid64 and the contract refuse.
func (c *intelCall) fromString() vecOutcome {
	ops, res, want, skip := c.split(1)
	if skip != "" {
		return vecSkip(skip)
	}
	if c.f == dec128 && ops[0] == "1.1E-2E" {
		return vecSkip(fmt.Sprintf("erroneous vector readtest.in:%d (%s): the numeral ends in a stray E, which bid64_from_string of the same file rejects with NaN and clause 5.12.1 does not allow", c.v.line, c.v.name))
	}
	wantBits, why := intelDecimal(c.f, res)
	if why != "" {
		return vecSkip(why)
	}
	var (
		got bits
		st  bid.Status
	)
	if o, ok := intelRun(func() { got, st = c.im.parse(c.f, ops[0], c.mode) }); !ok {
		return o
	}
	out := intelCompare(c.f, got, wantBits)
	if st&bid.Syntax != 0 {
		return out.noted(noteSyntax)
	}
	if out.pass && intelFlags(st) != want {
		return vecOutcome{got: out.got + " " + statusString(st)}
	}
	return out
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
	lines, vectors := 0, 0
	for n := 1; sc.Scan(); n++ {
		lines++
		v, ok := parseIntelLine(sc.Text(), n)
		if !ok {
			continue
		}
		vectors++
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
	t.Logf("readtest.in: %d lines, %d vectors, %d blank or comment", lines, vectors, lines-vectors)
	if got := tally.pass + tally.fail + tally.skipped(); got != vectors {
		t.Errorf("readtest.in: %d vectors but %d accounted for", vectors, got)
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

// TestIntelLineParser checks the readtest.in parser and the mapping, on
// lines of each shape the runner handles, against the reference model and
// the kernel. The lines have the shapes of the real file, but their values
// are the trivial ones that any set of decimal vectors starts with (zero,
// infinities, NaNs, small integers) or ones worked out by hand from the
// contract, not copied from the file, because the file's licence notice
// must travel with any copy. It needs no vectors.
func TestIntelLineParser(t *testing.T) {
	const (
		pass = "pass"
		fail = "fail" // a wrong expectation must fail, not skip
	)
	cases := []struct {
		line string
		want string // pass, fail, or the prefix of the skip reason
	}{
		// Arithmetic: hex operands, text operands, comma form, words.
		{"bid64_add 0 [31c0000000000001] [31c0000000000002] [31c0000000000003] 00", pass},
		{"bid64_add 0 [31C0000000000001] [31C0000000000002] [31C0000000000003] 00", pass},
		{"bid64_add 0 1 2 [31c0000000000003] 00", pass},
		{"bid64_add 0 -0.5 +1.5E0 [31a000000000000a] 00", pass},
		{"bid64_add 0 1 2 [31c0000000000004] 00", fail},
		{"bid64_add 0 -0 0 [31c0000000000000] 00", pass},
		{"bid64_add 1 -0 0 [b1c0000000000000] 00", pass},
		{"bid64_add 1 -0 0 [31c0000000000000] 00", fail},
		{"bid64_add 0 QNaN 1 [7c00000000000000] 00", pass},
		{"bid64_add 0 SNaN 1 [7c00000000000000] 01", pass},
		{"bid64_add 0 Infinity -Infinity [7c00000000000000] 01", pass},
		{"bid64_add 0 +Inf 1 [7800000000000000] 00", pass},
		{"bid64_add 0 inf -inf [7c00000000000000] 01", pass},
		{"bid64_sub 0 0 0 [31c0000000000000] 00", pass},
		{"bid64_sub 1 0 0 [b1c0000000000000] 00", pass},
		{"bid64_mul 0 3 -0 [b1c0000000000000] 00", pass},
		{"bid64_div 0 [31c0000000000001] [31c0000000000003] [2fcbd7a625405555] 20", pass},
		{"bid64_div 2 [31c0000000000001] [31c0000000000003] [2fcbd7a625405556] 20", pass},
		{"bid64_div 1 [31c0000000000001] [31c0000000000003] [2fcbd7a625405556] 20", fail},
		{"bid64_div 0 1 3 [2fcbd7a625405556] 20", fail},
		{"bid64_fma 0 [31c0000000000002] [31c0000000000003] [31c0000000000004] [31c000000000000a] 00", pass},
		{"bid64_fma 0 2 3 4 10 00", pass},
		{"bid128_add 0 [30400000000000000000000000000001] [3040000000000000,0000000000000002] [30400000000000000000000000000003] 00", pass},
		{"bid128_add 0 1 2 [3040000000000000,0000000000000003] 00", pass},
		{"bid128_mul 0 0 Infinity [7c000000000000000000000000000000] 01", pass},
		{"bid128_div 0 1 3 [2ffe314dc6448d9338c15b0a00000000] 20", fail},
		// Mixed-width forms: decimal64 operands widened, one rounding.
		{"bid128dd_add 0 [31c0000000000001] [31c0000000000002] [30400000000000000000000000000003] 00", pass},
		{"bid128ddd_fma 0 2 3 4 [3040000000000000000000000000000a] 00", pass},
		{"bid128dq_mul 0 [31c0000000000002] [30400000000000000000000000000003] [30400000000000000000000000000006] 00", pass},
		{"bid128qd_div 0 [30400000000000000000000000000001] [31c0000000000004] [303c0000000000000000000000000019] 00", pass},
		{"bid64qq_add 0 [30400000000000000000000000000001] [30400000000000000000000000000002] [31c0000000000003] 00", "kernel: mixed-width"},
		{"bid64dq_sub 0 [31c0000000000001] [30400000000000000000000000000002] [b1c0000000000001] 00", "kernel: mixed-width"},
		// Remainders, quantize, rounding to an integral value.
		{"bid64_rem 0 7 4 [b1c0000000000001] 00", pass},
		{"bid64_fmod 0 7 4 [31c0000000000003] 00 ulp=0.0000000000", pass},
		{"bid64_fmod 0 7 4 [31c0000000000003] 00 ulp=1.0000000000e+00", "field: non-zero ulp"},
		{"bid64_fmod 0 7 4 [31c0000000000003] 00 -- x=7,y=4", pass},
		{"bid64_quantize 0 [31a000000000000a] [31c0000000000000] [31c0000000000001] 20", pass},
		{"bid64_round_integral_nearest_even 0 2.5 [31c0000000000002] 00", pass},
		{"bid64_round_integral_nearest_away 0 2.5 [31c0000000000003] 00", pass},
		{"bid64_round_integral_zero 0 -2.5 [b1c0000000000002] 00", pass},
		{"bid64_round_integral_negative 2 2.5 [31c0000000000002] 00", pass},
		{"bid64_round_integral_positive 1 2.5 [31c0000000000003] 00", pass},
		{"bid64_round_integral_exact 1 -2.5 [b1c0000000000003] 20", pass},
		{"bid64_round_integral_exact 2 -2.5 [b1c0000000000002] 20", pass},
		{"bid64_nearbyint 2 2.1 [31c0000000000003] 00", pass},
		{"bid128_round_integral_exact 0 -Infinity [f8000000000000000000000000000000] 00", pass},
		// Sign operations, minnum and maxnum.
		{"bid64_negate 0 -0 [31c0000000000000] 00", pass},
		{"bid64_abs 0 -Infinity [7800000000000000] 00", pass},
		{"bid128_abs 0 -0 [30400000000000000000000000000000] 00", pass},
		{"bid64_copySign 0 5 -2 [b1c0000000000005] 00", pass},
		{"bid64_minnum 0 1 2 [31c0000000000001] 00", pass},
		{"bid64_maxnum 0 1 2 [31c0000000000001] 00", fail},
		{"bid64_maxnum 0 QNaN 3 [31c0000000000003] 00", "contract: Min/Max return NaN"},
		{"bid64_maxnum 0 QNaN 3 [31c0000000000004] 00", fail},
		{"bid64_maxnum 0 1.0 1 [31c0000000000001] 00", "contract: Min/Max return x"},
		{"bid64_minnum 0 -0 0 [31c0000000000000] 00", "contract: Min/Max order -0"},
		{"bid64_minnum_mag 0 -1 2 [b1c0000000000001] 00", "op: minNumMag"},
		// Predicates, ordering, class.
		{"bid64_quiet_less 0 1 2 1 00", pass},
		{"bid64_quiet_less 0 2 1 1 00", fail},
		{"bid64_quiet_equal 0 1.0 1 1 00", pass},
		{"bid64_quiet_not_equal 0 NaN 1 1 00", pass},
		{"bid64_quiet_unordered 0 NaN 1 1 00", pass},
		{"bid64_quiet_ordered 0 NaN 1 0 00", pass},
		{"bid64_quiet_greater_unordered 0 NaN 1 1 00", pass},
		{"bid64_quiet_less_unordered 0 2 1 0 00", pass},
		{"bid64_quiet_not_greater 0 NaN 1 1 00", pass},
		{"bid64_quiet_not_less 0 NaN 1 1 00", pass},
		{"bid64_quiet_greater_equal 0 1 1 1 00", pass},
		{"bid128_signaling_less_equal 0 SNaN 1 0 01", pass},
		{"bid64_sameQuantum 0 1.0 2.0 1 00", pass},
		{"bid64_sameQuantum 0 1.0 2 1 00", fail},
		{"bid64_totalOrder 0 -0 0 1 00", pass},
		{"bid64_totalOrder 0 0 -0 0 00", pass},
		{"bid64_totalOrder 0 [7c00000000000000] [7e00000000000000] 0 00", "contract: TotalOrder ignores"},
		{"bid64_totalOrder 0 -NaN NaN 1 00", pass},
		{"bid64_totalOrderMag 0 -1 -2 1 00", pass},
		{"bid64_class 0 -Infinity 2 00", pass},
		{"bid64_class 0 SNaN 0 00", pass},
		{"bid64_class 0 QNaN 1 00", pass},
		{"bid64_class 0 [0000000000000001] 7 00", pass},
		{"bid64_class 0 1 8 00", pass},
		{"bid64_class 0 -1 3 00", pass},
		{"bid64_class 0 -0 5 00", pass},
		{"bid64_class 0 Infinity 9 00", pass},
		{"bid64_class 0 1 3 00", fail},
		{"bid64_isSigned 0 -0 1 00", pass},
		{"bid64_isNormal 0 1 1 00", pass},
		{"bid64_isNormal 0 [0000000000000001] 0 00", pass},
		{"bid64_isSubnormal 0 [0000000000000001] 1 00", pass},
		{"bid64_isZero 0 -0 1 00", pass},
		{"bid64_isInf 0 -Infinity 1 00", pass},
		{"bid64_isNaN 0 SNaN 1 00", pass},
		{"bid64_isSignaling 0 SNaN 1 00", pass},
		{"bid64_isSignaling 0 QNaN 0 00", pass},
		{"bid64_isFinite 0 Infinity 0 00", pass},
		{"bid128_isCanonical 0 [7c000000000000000000000000000000] 1 00", pass},
		{"bid64_isZero 0 1 1 00", fail},
		// Conversions to integers: rounding, the x variants, range.
		{"bid64_to_int32_int 0 -2.5 -2 00", pass},
		{"bid64_to_int32_int 0 -2.5 -3 00", fail},
		{"bid64_to_int32_xint 0 -2.5 -2 20", pass},
		{"bid64_to_int32_xint 0 -2.5 -2 00", fail},
		{"bid64_to_int32_xint 0 -2 -2 00", pass},
		{"bid64_to_int32_floor 0 -2.5 -3 00", pass},
		{"bid64_to_int32_xceil 0 -2.5 -2 20", pass},
		{"bid64_to_int32_rnint 0 2.5 2 00", pass},
		{"bid64_to_int32_xrninta 0 2.5 3 20", pass},
		{"bid64_to_int64_int 0 1E+3 1000 00", pass},
		{"bid64_to_int8_ceil 0 2.5 3 00", pass},
		{"bid64_to_int8_int 0 127.9 127 00", pass},
		{"bid64_to_int8_int 0 128 -128 01", pass},
		{"bid64_to_int8_ceil 0 127.5 -128 01", pass},
		{"bid64_to_int16_int 0 -32768 -32768 00", pass},
		{"bid64_to_int32_int 0 3000000000 -2147483648 01", pass},
		{"bid64_to_int32_int 0 3000000000 -2147483648 00", fail},
		{"bid64_to_int32_int 0 7 -2147483648 01", fail},
		{"bid64_to_int32_int 0 NaN -2147483648 01", pass},
		{"bid64_to_int64_int 0 -Infinity -9223372036854775808 01", pass},
		{"bid128_to_int64_int 0 1E+30 -9223372036854775808 01", pass},
		{"bid64_to_uint32_int 0 3000000000 3000000000 00", pass},
		{"bid64_to_uint32_xint 0 3000000000 -1294967296 00", pass},
		{"bid64_to_uint32_int 0 -1 4294967295 01", pass},
		{"bid64_to_uint32_int 0 -0.5 0 00", pass},
		{"bid64_to_uint32_xint 0 -0.5 0 20", pass},
		{"bid64_to_uint32_int 0 4294967296 4294967295 01", pass},
		{"bid128_to_uint64_int 0 18446744073709551615 18446744073709551615 00", pass},
		{"bid128_to_uint64_int 0 18446744073709551615 -1 00", "malformed line: the integer result"},
		{"bid128_to_uint64_int 0 18446744073709551616 18446744073709551615 01", pass},
		{"bid64_to_uint64_int 0 18446744073709551615 18446744073709551615 00", fail},
		{"bid64_to_uint8_floor 0 -0.5 0 00", "fail"},
		{"bid64_llrint 2 2.1 3 20", pass},
		{"bid64_llrint 0 2.5 2 20", pass},
		{"bid64_llround 0 2.5 3 00", pass},
		// Conversions from integers: the status is compared.
		{"bid64_from_int32 0 -5 [b1c0000000000005] 00", pass},
		{"bid64_from_int32 0 -5 [b1c0000000000005] 20", fail},
		{"bid64_from_int32 0 2147483648 [31c0000000000000] 00", "malformed line: the integer operand"},
		{"bid64_from_uint32 0 4294967295 [31c00000ffffffff] 00", pass},
		{"bid64_from_uint64 0 18446744073709551615 [32468db8bac710cb] 20", pass},
		{"bid64_from_uint64 0 18446744073709551615 [32468db8bac710cb] 00", fail},
		{"bid64_from_int64 0 123 [31c000000000007b] 00", pass},
		{"bid128_from_int64 0 -1 [b0400000000000000000000000000001] 00", pass},
		{"bid128_from_uint64 0 18446744073709551615 [3040000000000000ffffffffffffffff] 00", pass},
		// Conversions between formats and with binary.
		{"bid64_to_bid128 0 [31c0000000000001] [30400000000000000000000000000001] 00", pass},
		{"bid128_to_bid64 0 [30400000000000000000000000000003] [31c0000000000003] 00", pass},
		{"bid128_to_bid64 0 [30400000000000000000000000000003] [31c0000000000004] 00", fail},
		{"bid64_to_binary64 0 [31c0000000000001] [3ff0000000000000] 00", pass},
		{"bid64_to_binary32 0 1 [3f800000] 00", pass},
		{"bid64_to_binary64 0 0.1 [3fb999999999999a] 0x20", pass},
		{"bid64_to_binary64 1 0.1 [3fb9999999999999] 0x20", pass},
		{"bid64_to_binary64 1 0.1 [3fb999999999999a] 0x20", fail},
		{"bid64_to_binary64 0 NaN [7ff8000000000000] 00", pass},
		{"bid128_to_binary32 0 Infinity [7f800000] 00", pass},
		{"binary64_to_bid64 0 [3ff0000000000000] [31c0000000000001] 00", pass},
		{"binary32_to_bid64 0 [3f800000] [31c0000000000001] 00", pass},
		{"binary32_to_bid128 0 [3fc00000] [3040000000000000,000000000000000f] 00", fail},
		{"binary32_to_bid64 0 1.5 [31a000000000000f] 00", pass},
		{"binary64_to_bid128 0 [7ff8000000000000] [7c000000000000000000000000000000] 00", pass},
		// Strings.
		{"bid64_to_string 0 [31c0000000000037] +55E+0 00", pass},
		{"bid64_to_string 0 -Infinity -Inf 00", pass},
		{"bid64_to_string 0 SNaN +SNaN 00", pass},
		{"bid64_to_string 0 -NaN -NaN 00", pass},
		{"bid128_to_string 0 -0 -0E+0 00", pass},
		{"bid128_to_string 0 -0 -0 00", fail},
		{"bid64_from_string 0 1. [31c0000000000001] 00", pass},
		{"bid64_from_string 2 12345678901234565 1234567890123457e1 20", pass},
		{"bid64_from_string 0 12345678901234565 1234567890123457e1 20", fail},
		{"bid64_from_string 0 1.5 [31a000000000000f] 20", fail},
		{"bid64_from_string 0 1.1P2 [7c00000000000000] 00", pass},
		{"bid64_from_string 0 +SNAN [7e00000000000000] 00", pass},
		{"bid64_from_string 0 +INFINITY [7800000000000000] 00", pass},
		{"bid64_from_string 0 1E-399 [0000000000000000] 30", pass},
		{"bid64_from_string 0 1E+385 [7800000000000000] 28", pass},
		{"bid128_from_string 0 0e6176 0e6111 00", pass},
		{"bid128_from_string 0 1.1E-2E [303a000000000000000000000000000b] 00", "erroneous vector"},
		// Skipped: functions, formats and fields outside the contract.
		{"bid32_add 0 [32800001] [32800002] [32800003] 00", "format: bid32"},
		{"bid64_to_bid32 0 [31c0000000000001] [32800001] 00", "format: bid32"},
		{"bid64_to_binary80 0 [31c0000000000001] [3fff8000000000000000] 00", "format: binary80"},
		{"bid128_to_binary128 0 1 [3fff0000000000000000000000000000] 00", "format: binary128"},
		{"binary80_to_bid64 0 [3fff8000000000000000] [31c0000000000001] 00", "format: binary80"},
		{"binary128_to_bid128 0 [3fff0000000000000000000000000000] [30400000000000000000000000000001] 00", "format: binary128"},
		{"binary32_to_bid32 0 [3f800000] [32800001] 00", "format: bid32"},
		{"bid64_sqrt 0 [31c0000000000004] [31c0000000000002] 00", "op: square root"},
		{"bid64_atan2 0 1 1 [2fe1cd1f7ceee7ab] 20 ulp=3.2e-01", "op: atan2"},
		{"bid64_sin 0 1 [2fe1cd1f7ceee7ab] 20 ulp=3.2e-01", "op: transcendental"},
		{"bid64_lrint 0 2.5 2 20 longintsize=32", "op: lrint"},
		{"bid64_lround 0 2.5 3 00 longintsize=64", "op: lround"},
		{"bid64_scalbn 0 1 2 [31c0000000000064] 00", "op: scalbn"},
		{"bid64_nextup 0 1 [31c0000000000002] 00", "op: nextUp"},
		{"bid64_quantexp 0 [0000000000000001] -398 00 -- MinDen", "op: quantum"},
		{"bid64_copy 0 0 [31c0000000000000] 00", "op: copy"},
		{"bid64_inf 0 0 [7800000000000000] 00", "op: inf and nan"},
		{"bid_setDecimalRoundingDirection 0 0 0 00", "op: library state"},
		{"bid_to_dpd64 0 [0020002800000000] [0004003c7dec6c4c] 00", "op: library state"},
		{"bid_strtod64 0 1 1 0", "op: library state"},
		{"str64 0 [4cb1bcece758ed60] [4cb1bcece758ed60] 00", "op: str64"},
		{"bid64_frobnicate 0 1 1 00", "op: not mapped to the kernel API"},
		{"frobnicate_64 0 1 1 00", "op: unknown function"},
		{"bid64_add 9 [31c0000000000001] [31c0000000000002] [31c0000000000003] 00", "context: rounding code unknown"},
		// Skipped: lines the runner cannot read, each for its own reason.
		{"bid64_add 0 [31c0000000000001] [31c0000000000002]", "malformed line: fewer fields"},
		{"bid64_add 0 1 2 3 zz", "malformed line: the status field"},
		{"bid64_add 0 [31c0000000000001 [31c0000000000002] [31c0000000000003] 00", "malformed line: hex literal with no closing bracket"},
		{"bid64_add 0 [31c00001] [31c0000000000002] [31c0000000000003] 00", "malformed line: hex literal is not of"},
		{"bid64_add 0 1 2 [31c0000000000003] 00 whatever", "field: unrecognized trailing field"},
		{"bid64_add 0 1 2 [31c0000000000003] 00 ulp=zz", "field: unreadable ulp"},
		{"bid64_add 0 1 2 [31c0000000000003] 00 underflow_before_only", pass},
		{"bid64_add 0 1 2 [31c0000000000003] 00 undefrlow_before_only", "erroneous vector"},
		{"bid64_add 0 1x 2 [31c0000000000003] 00", "operand: text outside"},
		{"bid64_add 0 --1 2 [31c0000000000003] 00", "malformed line"},
		{"bid64_quiet_less 0 1 2 yes 00", "malformed line: a predicate"},
		{"bid64_to_int32_int 0 2 two 00", "malformed line: the integer result"},
		{"bid64_to_binary64 0 1 [3ff00000] 00", "malformed line: the binary result"},
		{"binary64_to_bid64 0 [3ff00000] [31c0000000000001] 00", "malformed line: the binary operand"},
		{"bid64_add", "malformed line: no rounding code"},
	}
	// A "--" comment line, a blank line and a line with a trailing remark.
	for _, text := range []string{"", "   ", "-- a comment", "--- underflow before round cases"} {
		if v, ok := parseIntelLine(text, 1); ok {
			t.Errorf("parseIntelLine(%q) = %+v, want no vector", text, v)
		}
	}
	v, ok := parseIntelLine("bid64_isZero 0 -0 1 00 -- remark", 7)
	if !ok || v.line != 7 || v.name != "bid64_isZero" || v.round != "0" || len(v.fields) != 3 || v.fields[0] != "-0" {
		t.Errorf("parseIntelLine with a remark = %+v, %v", v, ok)
	}

	for _, im := range []struct {
		name string
		im   impl
	}{{"reference", reference{}}, {"kernel", kernel{}}} {
		for i, c := range cases {
			v, ok := parseIntelLine(c.line, i+1)
			if !ok {
				t.Errorf("%s: %q: no vector", im.name, c.line)
				continue
			}
			o := evalIntel(im.im, v)
			switch {
			case o.skip == "kernel: not implemented":
			case c.want == pass:
				if o.skip != "" || !o.pass {
					t.Errorf("%s: %q: want pass, got skip %q pass %v got %s", im.name, c.line, o.skip, o.pass, o.got)
				}
			case c.want == fail:
				if o.skip != "" || o.pass {
					t.Errorf("%s: %q: want fail, got skip %q pass %v", im.name, c.line, o.skip, o.pass)
				}
			case !strings.HasPrefix(o.skip, c.want):
				t.Errorf("%s: %q: want skip %q, got skip %q pass %v got %s", im.name, c.line, c.want, o.skip, o.pass, o.got)
			}
		}
	}
}
