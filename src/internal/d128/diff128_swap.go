// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go diff128_swap.go.

package d128

type diff128Swap struct {
	diffDw0 int64
	diffDw1 int64
	swap    bool
}
