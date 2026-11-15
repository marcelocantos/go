// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package strconv

var (
	BaseError    = baseError
	BitSizeError = bitSizeError
)

// The routes of the 'x' format of decimals, for SetHexPathHook.
const (
	HexPathProduct = hexPathProduct
	HexPathBinary  = hexPathBinary
	HexPathExact   = hexPathExact
)

// SetHexPathHook sets the hook that the 'x' format of decimals calls with
// the route that decided a value, and returns a function that clears it.
func SetHexPathHook(f func(path int)) func() {
	hexPathHook = f
	return func() { hexPathHook = nil }
}
