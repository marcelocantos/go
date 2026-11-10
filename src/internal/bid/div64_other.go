// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !amd64

package bid

// div64Native reports whether bits.Div64 is one hardware instruction on
// this target. Only amd64 has a 128-by-64 division; everywhere else
// bits.Div64 is a routine. See u256.go.
const div64Native = false
