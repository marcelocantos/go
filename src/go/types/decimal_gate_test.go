// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"internal/goexperiment"
	"testing"
)

func TestDecimalUniverseGate(t *testing.T) {
	for _, name := range []string{"decimal64", "decimal128"} {
		if got := types.Universe.Lookup(name) != nil; got != goexperiment.Decimal {
			t.Errorf("Universe.Lookup(%q) present = %v, want %v", name, got, goexperiment.Decimal)
		}
	}
	want := int(types.UntypedNil) + 1
	if goexperiment.Decimal {
		want += 2
	} else if types.IsNumeric != 26 || types.IsOrdered != 42 || types.IsConstType != 59 {
		t.Fatal("ordinary BasicInfo constants changed")
	}
	if len(types.Typ) != want {
		t.Errorf("len(Typ) = %d, want %d", len(types.Typ), want)
	}
	for _, src := range []string{
		"package p; type decimal64 string; type decimal128 int; var x decimal64; var y decimal128",
		"package p; var decimal64 = 3; func decimal128() int { return decimal64 }",
		"package p; var x decimal64",
	} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "p.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, err = new(types.Config).Check("p", fset, []*ast.File{f}, nil)
		wantError := src == "package p; var x decimal64" && !goexperiment.Decimal
		if (err != nil) != wantError {
			t.Errorf("Check(%q): %v, want error %v", src, err, wantError)
		}
	}
}
