// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

TEXT errors(SB),$0
	DQUA F2, F1, $4, F3             // ERROR "RMC field 4 out of range"
	DQUAI $16, F1, $0, F2           // ERROR "TE field 16 out of range"
	DQUAI $-17, F1, $0, F2          // ERROR "TE field -17 out of range"
	DRINTN $2, F1, $0, F2           // ERROR "R field 2 out of range"
	DRINTXQ $0, F2, $7, F4          // ERROR "RMC field 7 out of range"
	DDEDPD $4, F1, F2               // ERROR "SP field 4 out of range"
	DENBCD $2, F1, F2               // ERROR "S field 2 out of range"
	RET
