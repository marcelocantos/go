// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"internal/buildcfg"
	"internal/testenv"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Compile the same consumers through both API surfaces. cmd/api separately
// compares the complete ordinary surface with the released API baseline.
func TestDecimalPublicAPIGate(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	for _, tc := range []struct{ pkg, use string }{
		{"math", "p.Decimal64bits"},
		{"reflect", "p.Decimal64"},
		{"reflect", "p.Value.Decimal"},
		{"strconv", "p.ParseDecimal"},
		{"go/constant", "p.Quantum"},
		{"go/types", "p.IsDecimal"},
		{"debug/dwarf", "p.DecimalFloatType{}"},
		{"math/decimal", "p.NativeRepresentation"},
	} {
		t.Run(tc.pkg+"/"+tc.use, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/apigate\n\ngo 1.28\n"), 0o666); err != nil {
				t.Fatal(err)
			}
			src := "package p\nimport p " + `"` + tc.pkg + `"` + "\nvar _ = " + tc.use + "\n"
			if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0o666); err != nil {
				t.Fatal(err)
			}
			// Enabled, default, explicit off, then enabled again also exercises
			// cache separation without clearing or replacing the build cache.
			// The default is the toolchain's, which make.bash may have built
			// with GOEXPERIMENT=decimal.
			for _, mode := range []string{"decimal", "", "nodecimal", "decimal"} {
				goexp := mode
				if goexp == "" {
					goexp = buildcfg.DefaultGOEXPERIMENT
				}
				exp, err := buildcfg.ParseGOEXPERIMENT(runtime.GOOS, runtime.GOARCH, goexp)
				if err != nil {
					t.Fatal(err)
				}
				cmd := testenv.Command(t, testenv.GoToolPath(t), "build", "-o", os.DevNull, ".")
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "GOEXPERIMENT="+mode, "GOWORK=off", "GOFLAGS=")
				out, err := cmd.CombinedOutput()
				if exp.Decimal {
					if err != nil {
						t.Fatalf("enabled API: %v\n%s", err, out)
					}
				} else if err == nil || !(strings.Contains(string(out), "undefined") || tc.pkg == "math/decimal" && strings.Contains(string(out), "build constraints exclude all Go files")) {
					t.Fatalf("GOEXPERIMENT=%q: %v\n%s; want an absent API", mode, err, out)
				}
			}
		})
	}
}
