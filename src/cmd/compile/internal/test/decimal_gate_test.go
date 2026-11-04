// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"internal/testenv"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The decimal types are behind GOEXPERIMENT=decimal outside the standard
// library, and require go1.28 or later in go.mod.
func TestDecimalGates(t *testing.T) {
	testenv.MustHaveGoBuild(t)
	t.Parallel()

	const prog = "package main\n\nfunc main() { var d decimal64 = 1.50; println(d) }\n"
	build := func(goVersion, experiment string) (string, error) {
		dir := t.TempDir()
		mod := "module example.com/decimalgate\n\ngo " + goVersion + "\n"
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o666); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(prog), 0o666); err != nil {
			t.Fatal(err)
		}
		cmd := testenv.Command(t, testenv.GoToolPath(t), "build", "-o", os.DevNull, ".")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOEXPERIMENT="+experiment, "GOFLAGS=")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	if out, err := build("1.28", ""); err != nil {
		t.Errorf("go 1.28 with the default experiments: %v\n%s", err, out)
	}
	if out, err := build("1.28", "nodecimal"); err == nil || !strings.Contains(out, "decimal64 requires GOEXPERIMENT=decimal") {
		t.Errorf("GOEXPERIMENT=nodecimal: err = %v, want the experiment error\n%s", err, out)
	}
	if out, err := build("1.27", ""); err == nil || !strings.Contains(out, "predeclared decimal64 requires go1.28 or later") {
		t.Errorf("go 1.27: err = %v, want the version error\n%s", err, out)
	}
}
