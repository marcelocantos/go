// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go verify.go.

package d128

// Production runtime guard (CrossPlatformArchitecture.md Section 9.2): always
// evaluated; aborts through impossible when cond is false. Message first,
// condition second. An empty msg renders as "demand failed".
func demand(msg string, cond bool) {
	if !cond {
		if msg == "" {
			impossible("demand failed")
		}
		impossible(msg)
	}
}

// Unconditional abort marking unreachable code (CrossPlatformArchitecture.md
// Section 9.3); never returns.
func impossible(msg string) {
	// Go spells "does not return" as panic.
	if msg == "" {
		panic("impossible")
	}
	panic(msg)
}
