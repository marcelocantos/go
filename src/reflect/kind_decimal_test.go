// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reflect_test

import (
	"internal/goexperiment"
	"reflect"
	"testing"
)

// TestDecimalKindNames checks that the decimal kinds are named only while
// GOEXPERIMENT=decimal is on. While it is off they read as unknown kinds,
// so a program that checks at start-up for kinds it doesn't know, as
// mongo-driver's bsoncodec does, is unaffected.
func TestDecimalKindNames(t *testing.T) {
	want64, want128 := "decimal64", "decimal128"
	if !goexperiment.Decimal {
		want64, want128 = "kind27", "kind28"
	}
	if got := reflect.Decimal64.String(); got != want64 {
		t.Errorf("Decimal64.String() = %q, want %q", got, want64)
	}
	if got := reflect.Decimal128.String(); got != want128 {
		t.Errorf("Decimal128.String() = %q, want %q", got, want128)
	}
	if got := reflect.Kind(reflect.Decimal128 + 1).String(); got != "kind29" {
		t.Errorf("Kind(Decimal128+1).String() = %q, want %q", got, "kind29")
	}
}
