// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal && (amd64 || arm64)

package test

import (
	"internal/testenv"
	"testing"
)

// TestDecimalInlineLevels checks the decimal operators at the settings of
// -d=decimalinline that are not the default: every setting gives the same
// bits. The ordinary run of internal/bid's tests covers the default.
func TestDecimalInlineLevels(t *testing.T) {
	testenv.MustHaveGoRun(t)
	for _, level := range []string{"1", "3"} {
		t.Run(level, func(t *testing.T) {
			t.Parallel()
			cmd := testenv.Command(t, testenv.GoToolPath(t), "test", "-short",
				"-gcflags=-d=decimalinline="+level,
				"-run=Operator|AgreeWithCompiler|DecimalRoundingMode", "internal/bid")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("decimalinline=%s: %v\n%s", level, err, out)
			}
		})
	}
}
