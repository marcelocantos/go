// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package decimal

// pow10Recip[k-1] is the first 192 bits of 10^-k, least significant word
// first, for k from 1 to maxPow10Recip: the integer part of 2^(t+191) /
// 10^k, where 10^k has t bits. At three words, the precision of a first
// evaluation, a product by one of them divides by a power of ten, which
// is how a coefficient with digits after the point becomes a binary
// number. TestPow10Recip checks them against that quotient.
var pow10Recip = [maxPow10Recip][3]uint64{
	{0xcccccccccccccccc, 0xcccccccccccccccc, 0xcccccccccccccccc}, // 10^-1
	{0xd70a3d70a3d70a3d, 0x3d70a3d70a3d70a3, 0xa3d70a3d70a3d70a}, // 10^-2
	{0x78d4fdf3b645a1ca, 0x645a1cac083126e9, 0x83126e978d4fdf3b}, // 10^-3
	{0xc154c985f06f6944, 0xd3c36113404ea4a8, 0xd1b71758e219652b}, // 10^-4
	{0xcddd6e04c0592103, 0x0fcf80dc33721d53, 0xa7c5ac471b478423}, // 10^-5
	{0xd7e45803cd141a69, 0xa63f9a49c2c1b10f, 0x8637bd05af6c69b5}, // 10^-6
	{0x8ca08cd2e1b9c3db, 0x3d32907604691b4c, 0xd6bf94d5e57a42bc}, // 10^-7
	{0x3d4d3d758161697c, 0xfdc20d2b36ba7c3d, 0xabcc77118461cefc}, // 10^-8
	{0xfdd7645e011abac9, 0x31680a88f8953030, 0x89705f4136b4a597}, // 10^-9
	{0x2fbf06fcce912adc, 0xb573440e5a884d1b, 0xdbe6fecebdedd5be}, // 10^-10
	{0xf2ff38ca3eda88b0, 0xf78f69a51539d748, 0xafebff0bcb24aafe}, // 10^-11
	{0xf598fa3b657ba08d, 0xf93f87b7442e45d3, 0x8cbccc096f5088cb}, // 10^-12
	{0x88f4c3923bf900e2, 0x2865a5f206b06fb9, 0xe12e13424bb40e13}, // 10^-13
	{0x6d909c74fcc733e8, 0x538484c19ef38c94, 0xb424dc35095cd80f}, // 10^-14
	{0x57a6e390ca38f653, 0x0f9d37014bf60a10, 0x901d7cf73ab0acd9}, // 10^-15
	{0xbf716c1add27f085, 0x4c2ebe687989a9b3, 0xe69594bec44de15b}, // 10^-16
	{0xff8df0157db98d37, 0x09befeb9fad487c2, 0xb877aa3236a4b449}, // 10^-17
	{0x32d7f344649470f9, 0x3aff322e62439fcf, 0x9392ee8e921d5d07}, // 10^-18
	{0x1e2652070753e7f4, 0x2b31e9e3d06c32e5, 0xec1e4a7db69561a5}, // 10^-19
	{0x181ea8059f76532a, 0x88f4bb1ca6bcf584, 0xbce5086492111aea}, // 10^-20
	{0x467eecd14c5ea8ee, 0xd3f6fc16ebca5e03, 0x971da05074da7bee}, // 10^-21
	{0x70cb148213caa7e4, 0x5324c68b12dd6338, 0xf1c90080baf72cb1}, // 10^-22
	{0x8d6f439b43088650, 0x75b7053c0f178293, 0xc16d9a0095928a27}, // 10^-23
	{0xd78c3615cf3a050c, 0xc4926a9672793542, 0x9abe14cd44753b52}, // 10^-24
	{0x8c1389bc7ec33b47, 0x3a83ddbd83f52204, 0xf79687aed3eec551}, // 10^-25
	{0x3cdc6e306568fc39, 0x95364afe032a819d, 0xc612062576589dda}, // 10^-26
	{0xca49f1c05120c9c7, 0x775ea264cf55347d, 0x9e74d1b791e07e48}, // 10^-27
	{0x76dcb60081ce0fa5, 0x8bca9d6e188853fc, 0xfd87b5f28300ca0d}, // 10^-28
	{0x5f16f80067d80c84, 0x096ee45813a04330, 0xcad2f7f5359a3b3e}, // 10^-29
	{0x18df2ccd1fe00a03, 0xa1258379a94d028d, 0xa2425ff75e14fc31}, // 10^-30
	{0x4718f0a419800802, 0x80eacf948770ced7, 0x81ceb32c4b43fcf4}, // 10^-31
	{0x0b5b1aa028ccd99e, 0x67de18eda5814af2, 0xcfb11ead453994ba}, // 10^-32
	{0x6f7c154ced70ae18, 0xecb1ad8aeacdd58e, 0xa6274bbdd0fadd61}, // 10^-33
	{0xbf967770bdf3be79, 0xbd5af13bef0b113e, 0x84ec3c97da624ab4}, // 10^-34
	{0x65bd8be79652ca5c, 0x955e4ec64b44e864, 0xd4ad2dbfc3d07787}, // 10^-35
	{0xeafe098611dbd516, 0xdde50bd1d5d0b9e9, 0xaa242499697392d2}, // 10^-36
	{0xbbfe6e04db164412, 0x7e50d64177da2e54, 0x881cea14545c7575}, // 10^-37
	{0x2cca49a15e8a0683, 0x96e7bd358c904a21, 0xd9c7dced53c72255}, // 10^-38
}

// maxPow10Recip is the largest k of pow10Recip: every power of ten that
// a 128-bit integer holds.
const maxPow10Recip = 38

// mpPow10Recip returns 10^-k in three words, for k in [1,
// maxPow10Recip], to within one unit in the last place. Its words are
// the table's, and are not to be written.
func mpPow10Recip(k int) mpf {
	return mpf{m: pow10Recip[k-1][:], exp: -(lenU(pow10U(k)) + 191)}
}
