// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// quantizeSlow64 is the quantize of decimal64 operands that quantize64
// did not decide: quantizeLong64 is all of it.
func quantizeSlow64(x, y uint64, mode Mode) uint64 {
	return quantizeLong64(x, y, mode)
}
