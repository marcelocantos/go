// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ssagen

import (
	"fmt"
	"internal/goexperiment"
	"internal/testenv"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"cmd/compile/internal/base"
	"cmd/internal/obj"
	"cmd/internal/sys"
)

func TestBIDPowerTables(t *testing.T) {
	oldContext, oldArch := base.Ctxt, Arch.LinkArch
	oldAdd, oldCompare, oldRound := bidAddPowers, bidComparePowers, bidRoundPowers
	defer func() {
		base.Ctxt, Arch.LinkArch = oldContext, oldArch
		bidAddPowers, bidComparePowers, bidRoundPowers = oldAdd, oldCompare, oldRound
	}()
	for _, arch := range []*sys.Arch{sys.ArchARM64, sys.ArchPPC64} {
		Arch.LinkArch = &obj.LinkArch{Arch: arch}
		base.Ctxt = obj.Linknew(Arch.LinkArch)
		initBIDPowerTables()
		initBIDPowerTables() // No duplicate declaration on serial reuse.
		if len(base.Ctxt.Data) != 3 {
			t.Fatalf("%s: got %d declarations, want 3", arch.Name, len(base.Ctxt.Data))
		}
		for _, table := range []struct {
			sym         *obj.LSym
			first, last int
		}{{bidAddPowers, 0, 14}, {bidComparePowers, 0, 19}, {bidRoundPowers, 1, 19}} {
			if !table.sym.OnList() || len(table.sym.P) != 8*(table.last-table.first+1) {
				t.Fatalf("%s: undeclared or wrong-size table %s", arch.Name, table.sym.Name)
			}
			for exponent := table.first; exponent <= table.last; exponent++ {
				got := arch.ByteOrder.Uint64(table.sym.P[8*(exponent-table.first):])
				want := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil).Uint64()
				if got != want {
					t.Fatalf("%s: table %s power %d = %d, want %d", arch.Name, table.sym.Name, exponent, got, want)
				}
			}
		}
	}
}

func TestDecimalParallelPowerTables(t *testing.T) {
	if !goexperiment.Decimal || runtime.GOARCH != "arm64" && runtime.GOARCH != "amd64" {
		t.Skip("requires decimal integer lowering on ARM64 or AMD64")
	}
	testenv.MustHaveGoBuild(t)
	var source strings.Builder
	source.WriteString("package p\n")
	// Many equal-sized functions cause workers to reach first table use together.
	for i := 0; i < 128; i++ {
		fmt.Fprintf(&source, "func f%d(a,b decimal128) decimal128 { if a < b { return a+b }; return a-b }\n", i)
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "powers.go")
	if err := os.WriteFile(file, []byte(source.String()), 0600); err != nil {
		t.Fatal(err)
	}
	// Each direct compiler invocation bypasses the build cache. Repeat to expose
	// scheduling-sensitive first declarations; the old lowering races here.
	for i := 0; i < 32; i++ {
		cmd := testenv.Command(t, testenv.GoToolPath(t), "tool", "compile", "-c=64", "-p", "p", "-o", filepath.Join(dir, "powers.o"), file)
		cmd.Env = append(cmd.Environ(), "GOEXPERIMENT=decimal")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("attempt %d: %v\n%s", i+1, err, out)
		}
	}
}
