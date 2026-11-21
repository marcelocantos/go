// Code extracted from decimal128-go (MIT) by an AST walk; see doc.go.
// From decimal128-go context.go.

package d128

type context struct {
	round round
	flags *flags
}

// ctxSignalInexact_ctxnull signals inexact and returns d.
func ctxSignalInexact_ctxnull(ctx *context, d d128) d128 {
	ctx.flags.set(const_INEXACT)
	return d
}

// ctxSignalInexactOverflow_ctxnull signals overflow and inexact, and returns d.
func ctxSignalInexactOverflow_ctxnull(ctx *context, d d128) d128 {
	ctx.flags.set(const_OVERFLOW)
	ctx.flags.set(const_INEXACT)
	return d
}

func ctxSignalInexactUnderflow_ctxnull(ctx *context, d d128) d128 {
	ctx.flags.set(const_UNDERFLOW)
	ctx.flags.set(const_INEXACT)
	return d
}

// ctxSignalInvalidOperation_ctxnull signals invalidOperation (when ctxnull is non-nil) and returns result.
func ctxSignalInvalidOperation_ctxnull(ctxnull *context, cause invalidCause, result d128) d128 {
	_ = cause
	if ctxnull != nil {
		ctxnull.flags.set(const_INVALID_OPERATION)
	}
	return result
}

// ctxSignalSNanOperandFound_ctxnull returns nan quieted and signals invalidOperation (when ctxnull is non-nil).
// The operand must be a NaN; callers pass one only when an operand was
// signaling.
func ctxSignalSNanOperandFound_ctxnull(ctxnull *context, nan d128) d128 {

	quietedNaN := nan
	if d128_isSNAN(nan) {
		quietedNaN = d128_quieted(nan)
	}
	if ctxnull != nil {
		ctxnull.flags.set(const_INVALID_OPERATION)
	}
	return quietedNaN
}

// ctxNanOperand2_ctxnull selects and propagates the NaN of a two-operand operation: x's, unless only
// y is a NaN or only y is signaling. If either operand is signaling, the
// result is quieted and invalidOperation is signaled.
func ctxNanOperand2_ctxnull(ctxnull *context, x, y d128) d128 {

	takeY := !d128_isNAN(x) || (d128_isSNAN(y) && !d128_isSNAN(x))
	theNaN := x
	if takeY {
		theNaN = y
	}

	isSignaling := d128_isSNAN(x) || d128_isSNAN(y)
	if !isSignaling {
		return theNaN
	}
	return ctxSignalSNanOperandFound_ctxnull(ctxnull, theNaN)
}
