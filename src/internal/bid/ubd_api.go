// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// The UBD kernel's decimal128 operations for code compiled for the target,
// the runtime above all, where the compiler calls a runtime function rather
// than the kernel (every target but amd64 and arm64). Each takes and returns
// UBD words. The arithmetic rounds to nearest with ties to even, as the
// language's operators do.

// UBDAdd128 returns x + y.
func UBDAdd128(x, y UBD128) UBD128 { return ubdAddTTE128(x, y) }

// UBDSub128 returns x - y.
func UBDSub128(x, y UBD128) UBD128 { return ubdSubTTE128(x, y) }

// UBDMul128 returns x × y.
func UBDMul128(x, y UBD128) UBD128 { return ubdMulTTE128(x, y) }

// UBDDiv128 returns x / y.
func UBDDiv128(x, y UBD128) UBD128 { return ubdDivTTE128(x, y) }

// UBDMin128 and UBDMax128 are Min128 and Max128 on UBD.
func UBDMin128(x, y UBD128) UBD128 { return ubdMin128(x, y) }
func UBDMax128(x, y UBD128) UBD128 { return ubdMax128(x, y) }

// UBDLess128 and UBDLessEqual128 are Less128 and LessEqual128 on UBD.
func UBDLess128(x, y UBD128) bool      { return ubdLess128(x, y) }
func UBDLessEqual128(x, y UBD128) bool { return ubdLessEqual128(x, y) }

// UBDInt64From128 and UBDUint64From128 are the language's conversions to
// int64 and uint64: Int64From128 and Uint64From128 with TowardZero,
// without the status.
func UBDInt64From128(x UBD128) int64   { return ubdInt64From128(x) }
func UBDUint64From128(x UBD128) uint64 { return ubdUint64From128(x) }
