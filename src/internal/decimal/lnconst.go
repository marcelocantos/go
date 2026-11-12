// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package decimal

// ln 2 and ln 10 truncated to 576 bits, least significant word first:
// ln 2 is ln2Words × 2^ln2Exp, and likewise ln 10. They serve the
// first precisions the elementary functions work at; beyond nine words
// the constants are computed. TestLnConstants checks them against that
// computation.

const (
	ln2Exp  = -576
	ln10Exp = -574
)

var ln2Words = []uint64{
	0x3e96ca16224ae8c5,
	0x27573b291169b825,
	0xed2eae35c1382144,
	0x559552fb4afa1b10,
	0xe7b876206debac98,
	0x8a0d175b8baafa2b,
	0x40f343267298b62d,
	0xc9e3b39803f2f6af,
	0xb17217f7d1cf79ab,
}

var ln10Words = []uint64{
	0x782cf8a28a8c911e,
	0xfb8f788402e516d6,
	0x2c622418410be2da,
	0xcc70cbc02c5f0d68,
	0x962f02d7b1a8105c,
	0x83c61e8201f02d72,
	0xe28fecf9da5df90e,
	0xea56d62b82d30a28,
	0x935d8dddaaa8ac16,
}
