// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build goexperiment.decimal

package bid_test

import (
	"reflect"
	"runtime"
	"testing"
	"unsafe"
)

// dfpInsn names a decimal floating-point instruction by its primary opcode
// and its 10-bit extended opcode (bits 21-30; Power ISA 3.1 section 5.6).
type dfpInsn struct {
	name    string
	primary uint32
	xo      uint32
}

var (
	iDADD    = dfpInsn{"DADD", 59, 2}
	iDSUB    = dfpInsn{"DSUB", 59, 514}
	iDMUL    = dfpInsn{"DMUL", 59, 34}
	iDDIV    = dfpInsn{"DDIV", 59, 546}
	iDADDQ   = dfpInsn{"DADDQ", 63, 2}
	iDSUBQ   = dfpInsn{"DSUBQ", 63, 514}
	iDMULQ   = dfpInsn{"DMULQ", 63, 34}
	iDDIVQ   = dfpInsn{"DDIVQ", 63, 546}
	iDCMPU   = dfpInsn{"DCMPU", 59, 642}
	iDCMPUQ  = dfpInsn{"DCMPUQ", 63, 642}
	iDCTQPQ  = dfpInsn{"DCTQPQ", 63, 258}
	iDRDPQ   = dfpInsn{"DRDPQ", 63, 770}
	iDCFFIX  = dfpInsn{"DCFFIX", 59, 802}
	iDCFFIXQ = dfpInsn{"DCFFIXQ", 63, 802}
	iDCTFIX  = dfpInsn{"DCTFIX", 59, 290}
	iDCTFIXQ = dfpInsn{"DCTFIXQ", 63, 290}
	iDRINTN  = dfpInsn{"DRINTN", 59, 227 | 1<<8} // the Z23 form's RMC=01 (toward zero) in bits 21-22
	iDRINTNQ = dfpInsn{"DRINTNQ", 63, 227 | 1<<8}
)

// TestOperatorsUseHardware reads the machine code of the operator
// functions of TestOperators and checks that each contains its decimal
// floating-point instructions and calls nothing, so that TestOperators
// measures the hardware path and not the kernel.
func TestOperatorsUseHardware(t *testing.T) {
	for _, c := range []struct {
		fn   any
		want []dfpInsn
	}{
		{opAdd64, []dfpInsn{iDADD}},
		{opSub64, []dfpInsn{iDSUB}},
		{opMul64, []dfpInsn{iDMUL}},
		{opDiv64, []dfpInsn{iDDIV}},
		{opAdd128, []dfpInsn{iDADDQ}},
		{opSub128, []dfpInsn{iDSUBQ}},
		{opMul128, []dfpInsn{iDMULQ}},
		{opDiv128, []dfpInsn{iDDIVQ}},
		{opCmp64, []dfpInsn{iDCMPU}},
		{opCmp128, []dfpInsn{iDCMPUQ}},
		{opMin64, []dfpInsn{iDCMPU, iDADD}},
		{opMax128, []dfpInsn{iDCMPUQ, iDADDQ}},
		{opWiden, []dfpInsn{iDCTQPQ}},
		{opNarrow, []dfpInsn{iDRDPQ}},
		{opFromInt64, []dfpInsn{iDCFFIX}},
		{opFromInt64To128, []dfpInsn{iDCFFIXQ}},
		{opFromUint32, []dfpInsn{iDCFFIX}},
		{opFromUint64, []dfpInsn{iDCFFIXQ, iDADDQ, iDRDPQ}},
		{opFromUint64To128, []dfpInsn{iDCFFIXQ, iDADDQ}},
		{opToInt64, []dfpInsn{iDRINTN, iDCTFIX}},
		{opToInt64From128, []dfpInsn{iDRINTNQ, iDCTFIXQ}},
	} {
		name, code := machineCode(t, c.fn)
		found := map[dfpInsn]bool{}
		for _, w := range code {
			if w>>26 == 18 && w&1 != 0 { // bl
				t.Errorf("%s calls a function (%08x)", name, w)
			}
			for _, in := range c.want {
				if w>>26 == in.primary && w>>1&0x3FF == in.xo {
					found[in] = true
				}
			}
		}
		for _, in := range c.want {
			if !found[in] {
				t.Errorf("%s has no %s instruction", name, in.name)
			}
		}
	}
}

// machineCode returns the name of the function fn and its instructions:
// the words from its entry to the first address that belongs to another
// function.
func machineCode(t *testing.T, fn any) (string, []uint32) {
	pc := reflect.ValueOf(fn).Pointer()
	f := runtime.FuncForPC(pc)
	if f == nil || f.Entry() != pc {
		t.Fatalf("no function at %#x", pc)
	}
	var code []uint32
	for i := uintptr(0); i < 1<<16; i += 4 {
		if g := runtime.FuncForPC(pc + i); g == nil || g.Entry() != pc {
			return f.Name(), code
		}
		code = append(code, *(*uint32)(unsafe.Pointer(pc + i))) // little-endian, as the target
	}
	t.Fatalf("%s is longer than 64KB", f.Name())
	return "", nil
}
