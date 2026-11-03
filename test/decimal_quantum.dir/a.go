// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package a

const Base, Other = 1.50, 2.00
const Twice = Base * 2
const Third = 1.0 / 3.0
const Rate = 0.0825
const Neg = -Base

// Typed decimal constants fold here and are imported as constants.
const Price decimal64 = 19.99
const Tax = Price * Rate
const Total decimal128 = 1.50 + 1.50
