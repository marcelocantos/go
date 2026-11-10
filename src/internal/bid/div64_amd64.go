// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bid

// div64Native reports whether bits.Div64 is one hardware instruction on
// this target: the 128-by-64 division of amd64. See u256.go.
const div64Native = true
