// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types2

const (
	// Decimal64 and Decimal128 are the decimal floating-point basic kinds.
	Decimal64  BasicKind = decimal64Kind
	Decimal128 BasicKind = decimal128Kind
	// IsDecimal identifies decimal floating-point types.
	IsDecimal   BasicInfo = isDecimalInfo
	decimalInfo           = isDecimalInfo
)

const decimalEnabled = true
