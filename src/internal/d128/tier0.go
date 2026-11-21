// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go tier0.go.

package d128

// Logical right shift; `n` in `0..<64`.
func ushr_64_64(a int64, n int) int64 {

	return int64(uint64(a) >> uint(n))
}
