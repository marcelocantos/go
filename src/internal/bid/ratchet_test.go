// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid_test

// This file holds two ratchets that scan the whole Go tree. They are not
// tests of the package's behaviour; they keep the tree in the shape the
// decimal work promises.
//
//   - TestNoBIDStrawman (target T1.5.8): the BID encoding lives in this
//     package and nowhere else.
//   - TestNoPublishedVectors (target T1.6.2, third criterion): no published
//     test vector, and no mention of the sets they come from, enters the
//     tree, except in the throwaway runner vectors_test.go.
//
// Each has a self-test of its matcher on synthetic input (TestRatchetMatchers
// and TestRatchetVectorMatchers), which always runs, so a matcher that goes
// blind is caught without depending on what the tree happens to contain.

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"internal/bid"
	"internal/testenv"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"unicode"
)

// ---------------------------------------------------------------------------
// Ratchet 1: no BID encoding outside internal/bid (T1.5.8).
// ---------------------------------------------------------------------------

// bidRatchetAllow names the files outside internal/bid that may contain BID
// encoding anyway, each with the reason. T1.5.8 accepts an empty list or one
// that names each exception with a reason. Most entries are false positives
// of the mask rule: a 2^k-1 mask and a two-bit mask are common outside BID,
// and the rule matches by value. Keys are slash paths
// relative to GOROOT. An entry whose file is not flagged fails the test, so
// the list cannot go stale.
var bidRatchetAllow = map[string]string{
	"src/cmd/internal/obj/dwarf.go":                                       "(1<<49)-1 is one of a series of 2^k-1 DWARF line-program opcode thresholds, not a decimal128 coefficient mask",
	"src/crypto/internal/fips140/edwards25519/field/fe.go":                "(1<<51)-1 is the 51-bit limb mask of GF(2^255-19), not a decimal64 coefficient mask",
	"src/crypto/internal/fips140/edwards25519/field/_asm/fe_amd64_asm.go": "(1<<51)-1 is the 51-bit limb mask of GF(2^255-19), not a decimal64 coefficient mask",
	"test/decimal.go":                     "a test program that writes the expected BID encodings by hand, so that they do not come from the kernel it judges",
	"test/decimal_const.go":               "a test program that writes the expected BID encodings by hand, so that they do not come from the kernel it judges",
	"src/encoding/json/jsontext/state.go": "0x6000_0000_0000_0000 is stateNamespaceMask, two bits of a JSON parser state word, not a BID combination field",
}

// What the ratchet flags in a non-test Go file under $GOROOT/src and
// $GOROOT/test (outside internal/bid, testdata and vendor directories). It
// works on the parsed file, so a comment or a string never matches.
//
//  1. Mask. An integer constant expression whose value is one of the BID
//     field masks derived from internal/bid's own constants: the
//     combination-field masks (0x6000000000000000, 0x7800000000000000,
//     0x7c00000000000000, 0x7e00000000000000), the coefficient-field masks
//     of the two encoding forms of decimal64 and of the high word of
//     decimal128 (0x1fffffffffffff, 0x7ffffffffffff, 0x1ffffffffffff,
//     0x7fffffffffff), the largest coefficients (10^16-1 and the two
//     words of 10^34-1) and the encodings of +0 (398<<53, 6176<<49). The
//     expression may be a literal in any radix with any case or digit
//     separators, or a constant expression of literals using << >> + - * | &
//     (so 0xF<<59 and 1<<53-1 are masks). A mask is matched by value, so
//     the 128-bit high-word forms, which have the same values as the 64-bit
//     ones, need no entries of their own.
//
//  2. Bias. The integer literal 398 or 6176 (the exponent biases of
//     decimal64 and decimal128) in a BID context. The context is any of:
//     (a) the literal is in an expression that contains a shift; the
//     expression is the outermost one reached from the literal through
//     parentheses, unary and binary operators, calls, selectors, indexes
//     and composite-literal elements. (b) The innermost simple statement,
//     declaration spec or field around the literal, or the enclosing
//     function's name or receiver type, or the package name, has an
//     identifier with the word "bid" or "decimal" in it, where words are
//     the pieces of the name split at underscores, lower-to-upper case
//     changes, acronym ends and letter/digit changes ("BID64Pack" has
//     bid, 64, pack; "forbid" and "Bidi" have neither).
//
//  3. Name. An identifier, declared or used, matching the names the
//     strawman's encoders had: bid64Pack, bid128Pack, d64Unpack, d128Unpack,
//     bid64ToBID128, any name ending in ToBID64 or ToBID128, and any name
//     starting float64ToBID, literalToBID or packBID.
const (
	bidKindMask = "mask"
	bidKindBias = "bias"
	bidKindName = "name"
)

var bidNamePattern = regexp.MustCompile(
	`^(bid64Pack|bid128Pack|d64Unpack|d128Unpack|bid64ToBID128)$|ToBID(64|128)$|^(float64ToBID|literalToBID|packBID)`)

// bidMasks maps the value of each BID field mask to the name of the
// internal/bid constant it derives from.
var bidMasks = map[uint64]string{
	bid.Form2Mask64:                     "Form2Mask64 (combination field)",
	bid.SpecialMask64:                   "SpecialMask64 (combination field)",
	bid.NaN64:                           "NaN64 (combination field)",
	bid.SNaN64:                          "SNaN64 (combination field)",
	uint64(1)<<bid.CoeffBits64 - 1:      "coefficient field, first form, decimal64",
	uint64(1)<<bid.Form2CoeffBits64 - 1: "coefficient field, second form, decimal64",
	bid.CoeffMask128Hi:                  "CoeffMask128Hi",
	bid.Form2CoeffMask128Hi:             "Form2CoeffMask128Hi",
	bid.MaxCoeff64:                      "MaxCoeff64",
	bid.MaxCoeff128Hi:                   "MaxCoeff128Hi",
	bid.MaxCoeff128Lo:                   "MaxCoeff128Lo",
	bid.Zero64:                          "Zero64",
	bid.Zero128Hi:                       "Zero128Hi",
}

// Biases of the two formats.
var bidBiases = map[int64]string{
	bid.Bias64:  "decimal64 exponent bias",
	bid.Bias128: "decimal128 exponent bias",
}

type ratchetFinding struct {
	file string // as given to bidFileFindings
	line int
	col  int
	kind string
	text string // the matched source text or identifier
	why  string
}

func (f ratchetFinding) String() string {
	return fmt.Sprintf("%s:%d:%d: %s %s: %s", f.file, f.line, f.col, f.kind, f.text, f.why)
}

// bidWords splits an identifier into lower-case words: at underscores, at a
// lower-case letter followed by upper case, at the last capital of an
// acronym followed by a lower-case letter, and between letters and digits.
func bidWords(name string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	rs := []rune(name)
	for i, r := range rs {
		if r == '_' {
			flush()
			continue
		}
		if len(cur) > 0 {
			prev := rs[i-1]
			switch {
			case unicode.IsLower(prev) && unicode.IsUpper(r):
				flush()
			case unicode.IsUpper(prev) && unicode.IsUpper(r) && i+1 < len(rs) && unicode.IsLower(rs[i+1]):
				flush()
			case unicode.IsDigit(prev) != unicode.IsDigit(r):
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	return words
}

func hasBIDWord(name string) bool {
	for _, w := range bidWords(name) {
		if w == "bid" || w == "decimal" {
			return true
		}
	}
	return false
}

// foldConst evaluates e if it is an integer constant expression made of
// integer literals, parentheses and the operators << >> + - * | &.
func foldConst(e ast.Expr) (constant.Value, bool) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind != token.INT {
			return nil, false
		}
		v := constant.MakeFromLiteral(e.Value, token.INT, 0)
		return v, v.Kind() == constant.Int
	case *ast.ParenExpr:
		return foldConst(e.X)
	case *ast.BinaryExpr:
		x, ok := foldConst(e.X)
		if !ok {
			return nil, false
		}
		y, ok := foldConst(e.Y)
		if !ok {
			return nil, false
		}
		switch e.Op {
		case token.SHL, token.SHR:
			s, ok := constant.Uint64Val(y)
			if !ok || s > 256 {
				return nil, false
			}
			return constant.Shift(x, e.Op, uint(s)), true
		case token.ADD, token.SUB, token.MUL, token.OR, token.AND:
			return constant.BinaryOp(x, e.Op, y), true
		}
	}
	return nil, false
}

func isShift(e ast.Node) bool {
	b, ok := e.(*ast.BinaryExpr)
	return ok && (b.Op == token.SHL || b.Op == token.SHR)
}

// isCompoundStmt reports whether s encloses other statements, so that its
// whole subtree is too wide to count as "near" a literal.
func isCompoundStmt(s ast.Stmt) bool {
	switch s.(type) {
	case *ast.BlockStmt, *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt,
		*ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt,
		*ast.CaseClause, *ast.CommClause, *ast.LabeledStmt:
		return true
	}
	return false
}

// biasContext reports whether the literal at the top of stack is in a BID
// context (see the list at bidKindMask) and says which. The stack holds the
// ancestors of the literal, outermost first, ending with the literal.
func biasContext(pkgName string, stack []ast.Node) (string, bool) {
	// The outermost expression around the literal.
	root := len(stack) - 1
	for root > 0 {
		switch stack[root-1].(type) {
		case *ast.ParenExpr, *ast.BinaryExpr, *ast.UnaryExpr, *ast.CallExpr,
			*ast.SelectorExpr, *ast.IndexExpr, *ast.KeyValueExpr,
			*ast.CompositeLit, *ast.StarExpr:
			root--
			continue
		}
		break
	}
	shift := false
	ast.Inspect(stack[root], func(n ast.Node) bool {
		if n != nil && isShift(n) {
			shift = true
		}
		return !shift
	})
	if shift {
		return "in an expression with a shift", true
	}

	// The innermost simple statement, spec or field.
	scope := stack[root]
	for i := root - 1; i >= 0; i-- {
		switch n := stack[i].(type) {
		case ast.Stmt:
			if isCompoundStmt(n) {
				continue
			}
			scope = n
		case *ast.ValueSpec, *ast.TypeSpec, *ast.Field:
			scope = n
		default:
			continue
		}
		break
	}
	found := ""
	ast.Inspect(scope, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && found == "" && hasBIDWord(id.Name) {
			found = id.Name
		}
		return found == ""
	})
	if found != "" {
		return "near the identifier " + found, true
	}

	// The enclosing function's name and receiver type, and the package name.
	for i := root - 1; i >= 0; i-- {
		if fd, ok := stack[i].(*ast.FuncDecl); ok {
			if hasBIDWord(fd.Name.Name) {
				return "in function " + fd.Name.Name, true
			}
			if fd.Recv != nil {
				ast.Inspect(fd.Recv, func(n ast.Node) bool {
					if id, ok := n.(*ast.Ident); ok && found == "" && hasBIDWord(id.Name) {
						found = id.Name
					}
					return found == ""
				})
				if found != "" {
					return "in a method of " + found, true
				}
			}
			break
		}
	}
	if hasBIDWord(pkgName) {
		return "in package " + pkgName, true
	}
	return "", false
}

// bidFileFindings returns what the ratchet flags in one parsed file. name is
// used only in the findings.
func bidFileFindings(fset *token.FileSet, f *ast.File, name string) []ratchetFinding {
	var out []ratchetFinding
	add := func(n ast.Node, kind, text, why string) {
		p := fset.Position(n.Pos())
		out = append(out, ratchetFinding{name, p.Line, p.Column, kind, text, why})
	}
	var stack []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		switch n := n.(type) {
		case *ast.Ident:
			if bidNamePattern.MatchString(n.Name) {
				add(n, bidKindName, n.Name, "named like a strawman BID encoder")
			}
		case *ast.BasicLit, *ast.BinaryExpr, *ast.ParenExpr:
			v, ok := foldConst(n.(ast.Expr))
			if !ok {
				break
			}
			if u, ok := constant.Uint64Val(v); ok {
				if why, ok := bidMasks[u]; ok {
					add(n, bidKindMask, fmt.Sprintf("%#x", u), "BID field mask "+why)
					// Do not report the parts of the expression too.
					stack = stack[:len(stack)-1]
					return false
				}
			}
			if lit, ok := n.(*ast.BasicLit); ok {
				if i, ok := constant.Int64Val(v); ok {
					if why, ok := bidBiases[i]; ok {
						if ctx, ok := biasContext(f.Name.Name, stack); ok {
							add(n, bidKindBias, lit.Value, why+" "+ctx)
						}
					}
				}
			}
		}
		return true
	})
	return out
}

// bidSkipDir reports whether the walk skips the directory rel (a slash path
// relative to GOROOT).
func bidSkipDir(rel string) bool {
	base := rel[strings.LastIndex(rel, "/")+1:]
	return rel == "src/internal/bid" || base == "testdata" || base == "vendor" || base == ".git"
}

// scanBIDTree parses every non-test Go file of root/src and root/test and
// returns the findings, sorted. Files with syntax errors (the test directory
// has many on purpose) are scanned as far as they parse.
func scanBIDTree(root string) ([]ratchetFinding, int, error) {
	var files []string
	for _, top := range []string{"src", "test"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if d.IsDir() {
				if bidSkipDir(rel) {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go") && d.Type().IsRegular() {
				files = append(files, rel)
			}
			return nil
		})
		if err != nil {
			return nil, 0, err
		}
	}

	var (
		mu       sync.Mutex
		findings []ratchetFinding
		wg       sync.WaitGroup
		next     = make(chan string)
	)
	for range runtime.GOMAXPROCS(0) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fset := token.NewFileSet()
			for rel := range next {
				src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
				if err != nil {
					continue
				}
				f, _ := parser.ParseFile(fset, rel, src, parser.SkipObjectResolution)
				if f == nil {
					continue
				}
				if fs := bidFileFindings(fset, f, rel); len(fs) > 0 {
					mu.Lock()
					findings = append(findings, fs...)
					mu.Unlock()
				}
			}
		}()
	}
	for _, rel := range files {
		next <- rel
	}
	close(next)
	wg.Wait()
	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.file != b.file {
			return a.file < b.file
		}
		if a.line != b.line {
			return a.line < b.line
		}
		return a.col < b.col
	})
	return findings, len(files), nil
}

// TestNoBIDStrawman is the ratchet of target T1.5.8: it fails if a non-test
// Go file of GOROOT outside internal/bid contains a BID field mask, an
// exponent bias of a BID format or the name of a strawman encoder (see the
// comment above bidKindMask for the exact rules), unless the file is in
// bidRatchetAllow.
func TestNoBIDStrawman(t *testing.T) {
	testenv.MustHaveSource(t)
	root := testenv.GOROOT(t)
	findings, nfiles, err := scanBIDTree(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("scanned %d files", nfiles)

	flagged := map[string]bool{}
	for _, f := range findings {
		flagged[f.file] = true
		if _, ok := bidRatchetAllow[f.file]; !ok {
			t.Errorf("%s", f)
		}
	}
	for file, reason := range bidRatchetAllow {
		if reason == "" {
			t.Errorf("bidRatchetAllow[%q] has no reason", file)
		}
		if !flagged[file] {
			t.Errorf("bidRatchetAllow[%q] is stale: the file is not flagged; delete the entry", file)
		}
	}
}

// bidParse parses src as a file and returns what the ratchet flags in it.
func bidParse(t *testing.T, src string) []ratchetFinding {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, src)
	}
	return bidFileFindings(fset, f, "x.go")
}

// TestRatchetMatchers checks the matchers of TestNoBIDStrawman on synthetic
// files. It always runs.
func TestRatchetMatchers(t *testing.T) {
	const pre = "package p\nvar bid, x, y, z uint64\nvar _, _ = x, y\n"
	for _, c := range []struct {
		name string
		src  string // body after pre
		want []string
	}{
		{"mask shifted", "func f() { x := bid >> 53 & 0x7c00000000000000; _ = x }", []string{bidKindMask}},
		{"mask upper case hex", "var m = 0X7E00000000000000", []string{bidKindMask}},
		{"mask lower case hex", "var m = 0x7e00000000000000", []string{bidKindMask}},
		{"mask 6", "var m = 0x6000000000000000", []string{bidKindMask}},
		{"mask 78", "var m = 0x7800000000000000", []string{bidKindMask}},
		{"mask coeff 53", "var m = 0x1fffffffffffff", []string{bidKindMask}},
		{"mask coeff 53 padded", "var m = 0x001fffffffffffff", []string{bidKindMask}},
		{"mask coeff 128 hi", "var m = 0x0001ffffffffffff", []string{bidKindMask}},
		{"mask coeff 51", "var m = 0x7ffffffffffff", []string{bidKindMask}},
		{"mask digit separators", "var m = 0x7c00_0000_0000_0000", []string{bidKindMask}},
		{"mask by shifts", "var m = 0xF << 59", []string{bidKindMask}},
		{"mask by shift minus one", "var m = 1<<53 - 1", []string{bidKindMask}},
		{"mask by octal-free binary", "var m = 0b11 << 61", []string{bidKindMask}},
		{"mask max coefficient", "var m = 9999999999999999", []string{bidKindMask}},
		{"other hex is fine", "var m = 0x7c00000000000001", nil},
		{"mask in comment", "// 0x7c00000000000000\nvar m = 1", nil},
		{"mask in string", `var m = "0x7c00000000000000"`, nil},
		{"mask in raw string", "var m = `0x7c00000000000000`", nil},
		{"bias with shift", "var m = uint64(398) << 20", []string{bidKindBias}},
		{"bias 6176 with shift", "func f() uint64 { return uint64(6176+x) << 49 }", []string{bidKindBias}},
		{"bias near bid ident", "func f() { e := bid - 398; _ = e }", []string{bidKindBias}},
		{"bias near decimal ident", "func f() { decimalExp := 6176; _ = decimalExp }", []string{bidKindBias}},
		{"bias in bid function", "func bid64Exp(a, b int) int {\n\tc := a\n\treturn c + 398\n}", []string{bidKindBias}},
		{"bias in comment", "// the bias is 398\nvar m = 1", nil},
		{"bias in string", `var m = "6176"`, nil},
		{"bias alone", "func f() int { return 398 }", nil},
		{"bias near forbid", "func f() { forbidden := 6176; _ = forbidden }", nil},
		{"bias near bidi", "func f() { bidi := 398; _ = bidi }", nil},
		{"bias in other statement", "func f() { _ = 398\n\tvar decimal int; _ = decimal }", nil},
		{"name bid64Pack", "func bid64Pack(a uint64) uint64 { return a }", []string{bidKindName}},
		{"name bid128Pack", "func bid128Pack() {}", []string{bidKindName}},
		{"name d64Unpack", "func d64Unpack() {}", []string{bidKindName}},
		{"name d128Unpack", "func d128Unpack() {}", []string{bidKindName}},
		{"name ToBID64", "func float64ToBID64() {}", []string{bidKindName}},
		{"name suffix ToBID128", "func stringToBID128() {}", []string{bidKindName}},
		{"name bid64ToBID128", "func bid64ToBID128() {}", []string{bidKindName}},
		{"name float64ToBID", "func float64ToBIDx() {}", []string{bidKindName}},
		{"name literalToBID", "func literalToBID() {}", []string{bidKindName}},
		{"name packBID", "func packBID64() {}", []string{bidKindName}},
		{"use of name", "func f() { _ = bid64Pack }", []string{bidKindName}},
		{"similar name", "func decode64(a int) int { return a }", nil},
		{"call into the kernel", "func f() { _ = bidpkg.Pack64 }", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, f := range bidParse(t, pre+c.src) {
				got = append(got, f.kind)
			}
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("flagged %v, want %v:\n%v", got, c.want, bidParse(t, pre+c.src))
			}
		})
	}

	// A package named for BID puts every bias in context.
	if fs := bidParse(t, "package decimal\nfunc f() int { return 398 }"); len(fs) != 1 {
		t.Errorf("bias in package decimal: %v", fs)
	}

	// Words of identifiers.
	for name, want := range map[string]string{
		"BID64Pack":   "bid 64 pack",
		"IsBIDNaN":    "is bid na n",
		"forbid":      "forbid",
		"Bidi":        "bidi",
		"decimal_exp": "decimal exp",
		"toDecimal64": "to decimal 64",
	} {
		if got := strings.Join(bidWords(name), " "); got != want {
			t.Errorf("bidWords(%q) = %q, want %q", name, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Ratchet 2: no published test vectors in the tree (T1.6.2, third criterion).
// ---------------------------------------------------------------------------

// vectorsRunner is the one file allowed to name the two published sets or to
// carry a vector: the throwaway runner that reads the published vector sets
// from the developer's own disk.
//
// This exception is removed, with the runner (vectors_test.go), before the CL
// series is cut. After that this test has no exceptions.
const vectorsRunner = "src/internal/bid/vectors_test.go"

// The tree that TestNoPublishedVectors walks, besides regular files in the
// root of GOROOT.
// The names the ratchet looks for, assembled so that this file does not spell
// them.
const (
	vectorIBM   = "dec" + "Test"
	vectorIntel = "read" + "test"
)

var vectorsDirs = []string{"src", "test", "misc", "lib", "doc", "api"}

// What counts as a published vector. A file is flagged for a line that
// matches any of:
//
//   - names a set: the IBM set's name as a whole word (so a file name ending
//     in it matches, a plural or a longer identifier does not), or the
//     Intel set's file name as a whole word, case-sensitively. Both are
//     assembled in vectorIBM and vectorIntel below, because this file is
//     not the runner and must not spell them;
//   - an IBM vector line: an id, an operation of the IBM set
//     (case-insensitive: the files mix cases), operands, "->" and a result;
//   - an Intel vector line: a function bid32_, bid64_ or bid128_ (or the
//     dpd equivalent), a rounding code 0 to 4, and a hex word in brackets.
var (
	vectorNameRE = regexp.MustCompile(`\b(` + vectorIBM + `|` + vectorIntel + `)\b`)

	// The operations of the IBM set (GDA test-case file format) as far
	// as the General and decQuad/decDouble files use them.
	vectorDecRE = regexp.MustCompile(`(?i)^\s*[a-z0-9_.+-]+\s+(` +
		`abs|add|and|apply|canonical|class|compare|comparesig|comparetotal|comparetotmag|` +
		`copy|copyabs|copynegate|copysign|d2x|divide|divideint|exp|fma|invert|ln|log10|logb|` +
		`max|maxmag|min|minmag|minus|multiply|nextminus|nextplus|nexttoward|or|plus|power|` +
		`quantize|reduce|remainder|remaindernear|rescale|rotate|samequantum|scaleb|shift|` +
		`squareroot|subtract|toeng|tointegral|tointegralx|tosci|trim|x2d|xor` +
		`)\s+.*->\s*\S`)

	vectorIntelRE = regexp.MustCompile(`^\s*(bid|dpd)(32|64|128)_[a-z0-9_]+\s+[0-4]\s+.*\[[0-9a-fA-F]{2,32}\]`)
)

// vectorHits returns the 1-based numbers of the lines of data that name a
// published set or are a vector from one, with a reason for each.
func vectorHits(data []byte) (lines []int, why []string) {
	if bytes.IndexByte(data[:min(len(data), 8192)], 0) >= 0 {
		return nil, nil // binary
	}
	names := bytes.Contains(data, []byte(vectorIBM)) || bytes.Contains(data, []byte(vectorIntel))
	arrow := bytes.Contains(data, []byte("->"))
	intel := bytes.Contains(data, []byte("bid")) || bytes.Contains(data, []byte("dpd"))
	if !names && !arrow && !intel {
		return nil, nil
	}
	n := 0
	for len(data) > 0 {
		var line []byte
		line, data, _ = bytes.Cut(data, []byte("\n"))
		n++
		switch {
		case names && vectorNameRE.Match(line):
			lines, why = append(lines, n), append(why, "names a published vector set")
		case arrow && bytes.Contains(line, []byte("->")) && vectorDecRE.Match(line):
			lines, why = append(lines, n), append(why, "IBM vector line")
		case intel && vectorIntelRE.Match(line):
			lines, why = append(lines, n), append(why, "Intel vector line")
		}
	}
	return lines, why
}

// TestNoPublishedVectors is the third criterion of target T1.6.2: it fails if
// any file of the tree (src, test, misc, lib, doc, api and the files in the
// root, testdata and _test.go files included, binary files skipped) other
// than the runner, vectorsRunner, names a published vector set or contains a
// line of a published vector set. The suite's own files, src/internal/bid/
// testdata/*.txt, are in a different format and are not flagged.
func TestNoPublishedVectors(t *testing.T) {
	testenv.MustHaveSource(t)
	root := testenv.GOROOT(t)

	var files []string
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Type().IsRegular() {
			files = append(files, e.Name())
		}
	}
	for _, top := range vectorsDirs {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if d.IsDir() && d.Name() == ".git" {
				return filepath.SkipDir
			}
			if d.Type().IsRegular() {
				rel, _ := filepath.Rel(root, path)
				files = append(files, filepath.ToSlash(rel))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	var (
		mu   sync.Mutex
		msgs []string
		wg   sync.WaitGroup
		next = make(chan string)
	)
	for range runtime.GOMAXPROCS(0) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rel := range next {
				if rel == vectorsRunner {
					continue
				}
				data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
				if err != nil {
					continue
				}
				lines, why := vectorHits(data)
				if len(lines) == 0 {
					continue
				}
				mu.Lock()
				for i, n := range lines {
					msgs = append(msgs, fmt.Sprintf("%s:%d: %s", rel, n, why[i]))
				}
				mu.Unlock()
			}
		}()
	}
	for _, rel := range files {
		next <- rel
	}
	close(next)
	wg.Wait()

	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(vectorsRunner))); err != nil {
		t.Logf("%s is gone: delete the vectorsRunner exception from this test", vectorsRunner)
	}
	sort.Strings(msgs)
	for _, m := range msgs {
		t.Error(m)
	}
	t.Logf("scanned %d files", len(files))
}

// TestRatchetVectorMatchers checks the matchers of TestNoPublishedVectors on
// synthetic lines, and on every line of the suite's own testdata, which must
// not be flagged. It always runs.
func TestRatchetVectorMatchers(t *testing.T) {
	for _, c := range []struct {
		line string
		want bool
	}{
		{"addx001 add 1 2 -> 3", true},
		{"dqadd001 ADD '1.5' \"2.5\" -> 4.0 Inexact Rounded", true},
		{"ddfma0001 fma 1 2 3 -> 5", true},
		{"basx001 toSci '123' -> '123'", true},
		{"  rotx001 rotate 12 3 -> 12000", true},
		{"-- " + vectorIBM + " comment", true},
		{"see ddAdd." + vectorIBM + " for details", true},
		{"INTEL points at " + vectorIntel + ".in", true},
		{"bid64_add 0 [31c0000000000001] [31c0000000000002] [31c0000000000003] 00", true},
		{"bid128_to_bid64 2 [30400000000000000000000000000003] [31c0000000000003] 00", true},
		{vectorIBM + "s := []int{1}", false},
		{"type encDecTest struct{}", false},
		{"go readTest(t)", false},
		{"Add64 even exact 1.0 2.00 -> 3.00", false},
		{"Compare64 - fin 1 2 -> -1", false},
		{"Max64 even fin 1 2 -> 2", false},
		{"a -> b", false},
		{"x add y", false},
		{"bid64_add is the name of a function", false},
		{"x := bid64_add(0, [31c0])", false},
		{"func f(x int) -> int", false},
	} {
		lines, _ := vectorHits([]byte(c.line))
		if got := len(lines) > 0; got != c.want {
			t.Errorf("flagged(%q) = %v, want %v", c.line, got, c.want)
		}
	}
	if lines, _ := vectorHits([]byte("addx001 add 1 2 -> 3\x00")); len(lines) != 0 {
		t.Error("a file with a NUL byte is binary and must be skipped")
	}
	if lines, _ := vectorHits([]byte("a\nb\naddx001 add 1 2 -> 3\n")); len(lines) != 1 || lines[0] != 3 {
		t.Errorf("line numbers: %v, want [3]", lines)
	}

	// The suite's own files are not IBM or Intel lines.
	suite, err := filepath.Glob("testdata/*.txt")
	if err != nil || len(suite) == 0 {
		t.Fatalf("no suite files in testdata (err %v)", err)
	}
	for _, name := range suite {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if lines, why := vectorHits(data); len(lines) > 0 {
			t.Errorf("%s:%d: suite file flagged: %s", name, lines[0], why[0])
		}
	}
}
