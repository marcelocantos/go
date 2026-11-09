// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !goexperiment.decimal

package dwarf

// decimalFloatType represents a DW_ATE_decimal_float base type when the
// exported DecimalFloatType is not part of the API. Decoding the attribute
// does not depend on the language experiment: the linker emits it in every
// mode, and other producers may emit it too.
type decimalFloatType struct{ BasicType }

func newDecimalFloatType() Type { return new(decimalFloatType) }
