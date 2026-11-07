// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package fmtsort_test

import (
	"math/decimal"
	"reflect"
)

// The decimal cases of the comparison and sorting tables. The external
// test package is not compiled as part of the standard library, so it can
// name decimal64 only while GOEXPERIMENT=decimal is on.

func init() {
	compareTests = append(compareTests,
		ct(reflect.TypeOf(decimal64(0)), decimal.NaN[decimal64](), decimal.Inf[decimal64](-1), decimal64(-1e10), decimal64(0), decimal64(1e10), decimal.Inf[decimal64](1)),
	)
	sortTests = append(sortTests,
		sortTest{
			map[decimal64]string{7: "bar", -3: "foo", decimal.NaN[decimal64](): "nan", decimal.Inf[decimal64](1): "inf"},
			"NaN:nan -3:foo 7:bar +Inf:inf",
		},
	)
}
