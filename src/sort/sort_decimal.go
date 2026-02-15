// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !compiler_bootstrap

package sort

import "slices"

// Decimal128Slice implements Interface for a []decimal128, sorting in increasing order,
// with not-a-number (NaN) values ordered before other values.
type Decimal128Slice []decimal128

func (x Decimal128Slice) Len() int           { return len(x) }
func (x Decimal128Slice) Less(i, j int) bool { return x[i] < x[j] || (x[i] != x[i] && x[j] == x[j]) }
func (x Decimal128Slice) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }

// Sort is a convenience method: x.Sort() calls Sort(x).
func (x Decimal128Slice) Sort() { Sort(x) }

// Search returns the result of applying [SearchDecimal128s] to the receiver and x.
func (p Decimal128Slice) Search(x decimal128) int { return SearchDecimal128s(p, x) }

// SearchDecimal128s searches for x in a sorted slice of decimal128s and returns the index
// as specified by [Search]. The return value is the index to insert x if x is
// not present (it could be len(a)).
// The slice must be sorted in ascending order.
func SearchDecimal128s(a []decimal128, x decimal128) int {
	return Search(len(a), func(i int) bool { return a[i] >= x })
}

// Decimal128s sorts a slice of decimal128s in increasing order.
// Not-a-number (NaN) values are ordered before other values.
func Decimal128s(x []decimal128) { slices.Sort(x) }

// Decimal128sAreSorted reports whether the slice x is sorted in increasing order,
// with not-a-number (NaN) values before any other values.
func Decimal128sAreSorted(x []decimal128) bool { return slices.IsSorted(x) }
