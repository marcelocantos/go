// -lang=go1.27

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The decimal types require go1.28 or later.

package p

var _ decimal64 /* ERROR "predeclared decimal64 requires go1.28 or later" */
var _ decimal128 /* ERROR "predeclared decimal128 requires go1.28 or later" */

func _(x float64) {
	_ = decimal64 /* ERROR "predeclared decimal64 requires go1.28 or later" */ (x)
}
