// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package types_test

import (
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	. "go/types"
	"testing"
)

// TestMinMaxCohortConstant checks that constant min and max choose among
// members of one cohort as the run-time built-ins do, by IEEE 754-2019's
// totalOrder, whatever the order of the arguments.
func TestMinMaxCohortConstant(t *testing.T) {
	const src = `package p

const (
	a = min(1.0, 1.00)
	b = min(1.00, 1.0)
	c = max(1.0, 1.00)
	d = max(1.00, 1.0)
	e = min(-1.0, -1.00)
	f = min(-1.00, -1.0)
	g = max(-1.0, -1.00, -1.000)
	h = min(0x1p-1, 0.50)
	i = min(0.50, 0x1p-1)
	j decimal64 = min(2.50, 2.5)
	k = max(decimal64(2.5), 2.50)
)
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	conf := Config{Importer: importer.Default(), GoVersion: "go1.28"}
	pkg, err := conf.Check("p", fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]int{"a": -2, "b": -2, "c": -1, "d": -1, "e": -1, "f": -1, "g": -3, "h": -2, "i": -2, "j": -2, "k": -1} {
		v := pkg.Scope().Lookup(name).(*Const).Val()
		if q, ok := constant.Quantum(v); !ok || q != want {
			t.Errorf("%s: quantum %d, %v; want %d", name, q, ok, want)
		}
	}
}
