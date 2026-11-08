// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !goexperiment.decimal

package types

const (
	decimalInfo    BasicInfo = 0
	decimalEnabled           = false
)
