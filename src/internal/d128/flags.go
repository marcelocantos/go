// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go flags.go.

package d128

type flags struct {
	bits int64
}

func (f *flags) set(exception exception754) { f.bits |= exc754Mask(exception) }

// Raw flag word. Provided for fast "any flags set?" checks
// (`flags.raw() == 0`) and for serialization in test harnesses.
func (f *flags) raw() int64 { return f.bits }
