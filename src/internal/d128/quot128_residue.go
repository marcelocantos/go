// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go quot128_residue.go.

package d128

type quot128Residue struct {
	quotDw0 int64
	quotDw1 int64
	residue residue
}
