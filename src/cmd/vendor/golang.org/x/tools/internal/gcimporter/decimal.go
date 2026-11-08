// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !compiler_bootstrap

package gcimporter

import (
	"go/types"
	_ "unsafe"
)

//go:linkname decimalTypes go/types.decimalTypes
func decimalTypes() (*types.Basic, *types.Basic)
func decimalPredeclared() []types.Type { a, b := decimalTypes(); return []types.Type{a, b} }
