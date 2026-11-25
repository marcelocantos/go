// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package decimal

import "internal/bid"

// totalOrder of IEEE 754-2019 clause 5.10 and totalOrderMag of clause
// 5.7.2. A decimal128 stored as UBD uses the UBD order, which orders NaNs
// as clause 5.10 d) does; decimal64, and decimal128 on a target that stores
// BID, use internal/bid's TotalOrder64 and TotalOrder128, which order NaNs
// by sign alone.

// TotalOrder reports whether x precedes or is y in the total order.
func TotalOrder[D Decimal](x, y D) bool {
	if is64(x) {
		return bid.TotalOrder64(BID64bits(decimal64(x)), BID64bits(decimal64(y)))
	}
	if bid.NativeUBD128 {
		return bid.UBDTotalOrder128(ubdWords(decimal128(x)), ubdWords(decimal128(y)))
	}
	return bid.TotalOrder128(b128(decimal128(x)), b128(decimal128(y)))
}

// TotalCompare returns -1, 0 or +1 as x precedes, is, or follows y in the
// total order.
func TotalCompare[D Decimal](x, y D) int {
	if !is64(x) && bid.NativeUBD128 {
		return bid.UBDTotalCompare128(ubdWords(decimal128(x)), ubdWords(decimal128(y)))
	}
	xy, yx := TotalOrder(x, y), TotalOrder(y, x)
	switch {
	case xy && yx:
		return 0
	case xy:
		return -1
	}
	return 1
}

// TotalOrderMag reports whether the magnitude of x precedes or is the
// magnitude of y in the total order: TotalOrder(Abs(x), Abs(y)).
func TotalOrderMag[D Decimal](x, y D) bool {
	if !is64(x) && bid.NativeUBD128 {
		return bid.UBDTotalOrderMag128(ubdWords(decimal128(x)), ubdWords(decimal128(y)))
	}
	return TotalOrder(Abs(x), Abs(y))
}
