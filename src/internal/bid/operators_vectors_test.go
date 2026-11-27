// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package bid_test

import (
	"errors"
	"fmt"
	"internal/bid"
	dec "math/decimal"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"
)

// operatorsImpl runs add, subtract, multiply, divide and FMA the way a
// program does: each operand read with math/decimal's BID64frombits or
// BID128frombits, the operation done by Go's operators + - * / on
// decimal64 or decimal128 (FMA by math/decimal's FMA), and the result
// written with BID64bits or BID128bits. The compiler lowers the operators
// as it does in any program, so on a UBD target the run covers the inline
// paths of the decimalinline setting the test was built at (go test
// -gcflags=all=-d=decimalinline=N) as well as the kernel entries behind
// them.
// -bid.vectors=operators runs the published vectors against it.
//
// The operators round TiesToEven and have no other rounding direction, so a
// vector in another rounding direction is skipped. Every other operation is
// the ubd target's.
//
// A result of this target means nothing without the level, so the run
// names it ("operators at decimalinline setting 3"), read from the -gcflags
// the test binary records in its build information, and refuses to run
// when it cannot tell.
type operatorsImpl struct{ ubdKernel }

func init() {
	vecTargets["operators"] = operatorsImpl{}
}

// decimalInlineFlag finds the decimalinline setting in the compiler's -d flags.
var decimalInlineFlag = regexp.MustCompile(`(?:^|[\s=,])decimalinline=([0-9]+)`)

// describe names the decimalinline setting the operators were compiled at.
func (operatorsImpl) describe() (string, error) {
	if !bid.NativeUBD128 {
		return "operators (decimal128 is not stored as UBD on this target)", nil
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "", errors.New("the test binary has no build information, so its decimalinline setting is unknown")
	}
	level := 0
	for _, s := range bi.Settings {
		if s.Key == "-gcflags" {
			l, err := decimalInlineFromGcflags(s.Value)
			if err != nil {
				return "", err
			}
			level = l
		}
	}
	if level == 0 {
		level = 2 // the compiler's default (cmd/compile/internal/ssagen.ubdLevel)
	}
	return fmt.Sprintf("operators at decimalinline setting %d", level), nil
}

// decimalInlineFromGcflags returns the decimalinline setting that the
// -gcflags value v gives
// internal/bid's test, 0 for none (the compiler's default). v is flags, or
// pattern=flags; a pattern that may not cover internal/bid is an error.
func decimalInlineFromGcflags(v string) (int, error) {
	flags := v
	if !strings.HasPrefix(flags, "-") {
		pattern, rest, _ := strings.Cut(flags, "=")
		switch pattern {
		case "all", "std", "internal/bid", "internal/...":
		default:
			return 0, fmt.Errorf("cannot tell whether -gcflags=%s covers internal/bid; use -gcflags=all=-d=decimalinline=N", v)
		}
		flags = rest
	}
	m := decimalInlineFlag.FindStringSubmatch(flags)
	if m == nil {
		return 0, nil
	}
	return strconv.Atoi(m[1])
}

// skipOperatorsDirection is the skip reason for a vector in a rounding
// direction other than TiesToEven.
const skipOperatorsDirection = implSkip("operators: Go's operators round TiesToEven only")

// opAdd64 and the other operators are operators_test.go's.

//go:noinline
func opFMA64(x, y, z decimal64) decimal64 { return dec.FMA(x, y, z) }

//go:noinline
func opFMA128(x, y, z decimal128) decimal128 { return dec.FMA(x, y, z) }

func opIn64(x bits) decimal64  { return dec.BID64frombits(x.Lo) }
func opOut64(d decimal64) bits { return bits{Lo: dec.BID64bits(d)} }

func opIn128(x bits) decimal128 { return dec.BID128frombits(x.Hi, x.Lo) }
func opOut128(d decimal128) bits {
	hi, lo := dec.BID128bits(d)
	return bits{Hi: hi, Lo: lo}
}

func opBinary(f *format, x, y bits, m bid.Mode, op64 func(x, y decimal64) decimal64, op128 func(x, y decimal128) decimal128) bits {
	if m != bid.ToNearestEven {
		panic(skipOperatorsDirection)
	}
	if w64(f) {
		return opOut64(op64(opIn64(x), opIn64(y)))
	}
	return opOut128(op128(opIn128(x), opIn128(y)))
}

func (operatorsImpl) add(f *format, x, y bits, m bid.Mode) bits {
	return opBinary(f, x, y, m, opAdd64, opAdd128)
}
func (operatorsImpl) sub(f *format, x, y bits, m bid.Mode) bits {
	return opBinary(f, x, y, m, opSub64, opSub128)
}
func (operatorsImpl) mul(f *format, x, y bits, m bid.Mode) bits {
	return opBinary(f, x, y, m, opMul64, opMul128)
}
func (operatorsImpl) div(f *format, x, y bits, m bid.Mode) bits {
	return opBinary(f, x, y, m, opDiv64, opDiv128)
}
func (operatorsImpl) fma(f *format, x, y, z bits, m bid.Mode) bits {
	if m != bid.ToNearestEven {
		panic(skipOperatorsDirection)
	}
	if w64(f) {
		return opOut64(opFMA64(opIn64(x), opIn64(y), opIn64(z)))
	}
	return opOut128(opFMA128(opIn128(x), opIn128(y), opIn128(z)))
}

func TestUBDLevelFromGcflags(t *testing.T) {
	for _, c := range []struct {
		v     string
		level int
		err   bool
	}{
		{"-d=decimalinline=1", 1, false},
		{"all=-d=decimalinline=3", 3, false},
		{"internal/bid=-d=decimalinline=2", 2, false},
		{"std=-N -l -d=checkptr=0,decimalinline=3", 3, false},
		{"all=-N -l", 0, false},
		{"-d=nodecimalinline=3", 0, false},
		{"math/decimal=-d=decimalinline=1", 0, true},
	} {
		level, err := decimalInlineFromGcflags(c.v)
		if level != c.level || (err != nil) != c.err {
			t.Errorf("decimalInlineFromGcflags(%q) = %d, %v; want %d, error %v", c.v, level, err, c.level, c.err)
		}
	}
}
