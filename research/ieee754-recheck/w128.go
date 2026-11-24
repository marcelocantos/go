//go:build !w64

package main

// T is the width under test: decimal128 unless built with -tags w64.
type T = decimal128

const bits = 128
