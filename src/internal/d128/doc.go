// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package d128 is the arithmetic of decimal128-go
// (github.com/migueldecimal128/decimal128-go, MIT licence, Copyright the
// decimal128-go authors), which works on the UBD layout itself (see
// internal/bid/ubd.go), with no conversion to BID. internal/bid's UBD kernel
// entries call it through entry.go.
//
// Every other file is extracted from decimal128-go, commit 71e34e9, by an
// AST walk from the entry points' roots: it copies the package-level
// declarations they reach, transitively, unchanged, one file per source
// file, and drops the verify(...) assertions, which compile to nothing in
// decimal128-go's default build. Changes belong in decimal128-go, followed
// by a new extraction, not here.
//
// Extracted so far: add, subtract and multiply (d128_add_rnd_ctxnull,
// d128_subtract_rnd_ctxnull, d128_multiply_rnd_ctxnull). Quantize was
// tried and measured slower than internal/bid's on money-sized operands
// (about 20 ns against 12 ns a call, conversions included): its coarser
// case divides a word-sized coefficient by 10^k with the general 128-bit
// reciprocal and a digit count; see internal/bid/ubd.go.
//
// This package is imported by internal/bid, which the runtime imports, so it
// may import only math/bits and unsafe, and must not allocate.
package d128
