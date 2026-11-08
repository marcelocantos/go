// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package dwarf

// A DecimalFloatType represents a decimal floating point type.
type DecimalFloatType struct{ BasicType }

func newDecimalFloatType() Type { return new(DecimalFloatType) }
