// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import "internal/bid"

// ddiv64 computes x / y through the shared BID kernel. The language
// operators always round to nearest, ties to even.
func ddiv64(x, y decimal64) decimal64 {
	return decimal64frombits(bid.Div64(decimal64bits(x), decimal64bits(y), bid.ToNearestEven))
}

// ddiv128 computes x / y through the shared BID kernel.
func ddiv128(x, y decimal128) decimal128 {
	xhi, xlo := decimal128bits(x)
	yhi, ylo := decimal128bits(y)
	z := bid.Div128(
		bid.Uint128{Hi: xhi, Lo: xlo},
		bid.Uint128{Hi: yhi, Lo: ylo},
		bid.ToNearestEven,
	)
	return decimal128frombits(z.Hi, z.Lo)
}
