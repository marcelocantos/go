// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binary

import (
	"bytes"
	"testing"
	"unsafe"
)

func TestDecimalWriteRead(t *testing.T) {
	// Test decimal64 write and read with both endiannesses.
	var d64 decimal64 = 3.14
	d64Bits := *(*uint64)(unsafe.Pointer(&d64))

	for _, order := range []ByteOrder{LittleEndian, BigEndian} {
		t.Run("decimal64/"+order.String(), func(t *testing.T) {
			buf := new(bytes.Buffer)
			if err := Write(buf, order, d64); err != nil {
				t.Fatalf("Write decimal64: %v", err)
			}
			if buf.Len() != 8 {
				t.Fatalf("Write decimal64: got %d bytes, want 8", buf.Len())
			}

			var got decimal64
			if err := Read(bytes.NewReader(buf.Bytes()), order, &got); err != nil {
				t.Fatalf("Read decimal64: %v", err)
			}
			gotBits := *(*uint64)(unsafe.Pointer(&got))
			if gotBits != d64Bits {
				t.Errorf("decimal64 round trip: got bits %#x, want %#x", gotBits, d64Bits)
			}
		})
	}

	// Test decimal128 write and read with both endiannesses.
	var d128 decimal128 = 2.718
	d128Bits := *(*[2]uint64)(unsafe.Pointer(&d128))

	for _, order := range []ByteOrder{LittleEndian, BigEndian} {
		t.Run("decimal128/"+order.String(), func(t *testing.T) {
			buf := new(bytes.Buffer)
			if err := Write(buf, order, d128); err != nil {
				t.Fatalf("Write decimal128: %v", err)
			}
			if buf.Len() != 16 {
				t.Fatalf("Write decimal128: got %d bytes, want 16", buf.Len())
			}

			var got decimal128
			if err := Read(bytes.NewReader(buf.Bytes()), order, &got); err != nil {
				t.Fatalf("Read decimal128: %v", err)
			}
			gotBits := *(*[2]uint64)(unsafe.Pointer(&got))
			if gotBits != d128Bits {
				t.Errorf("decimal128 round trip: got bits %v, want %v", gotBits, d128Bits)
			}
		})
	}
}

func TestDecimalZero(t *testing.T) {
	// Test that zero decimal values write and read correctly.
	var d64 decimal64 = 0
	var d128 decimal128 = 0

	for _, order := range []ByteOrder{LittleEndian, BigEndian} {
		t.Run("decimal64_zero/"+order.String(), func(t *testing.T) {
			buf := new(bytes.Buffer)
			if err := Write(buf, order, d64); err != nil {
				t.Fatalf("Write: %v", err)
			}
			var got decimal64
			if err := Read(bytes.NewReader(buf.Bytes()), order, &got); err != nil {
				t.Fatalf("Read: %v", err)
			}
			gotBits := *(*uint64)(unsafe.Pointer(&got))
			wantBits := *(*uint64)(unsafe.Pointer(&d64))
			if gotBits != wantBits {
				t.Errorf("got bits %#x, want %#x", gotBits, wantBits)
			}
		})

		t.Run("decimal128_zero/"+order.String(), func(t *testing.T) {
			buf := new(bytes.Buffer)
			if err := Write(buf, order, d128); err != nil {
				t.Fatalf("Write: %v", err)
			}
			var got decimal128
			if err := Read(bytes.NewReader(buf.Bytes()), order, &got); err != nil {
				t.Fatalf("Read: %v", err)
			}
			gotBits := *(*[2]uint64)(unsafe.Pointer(&got))
			wantBits := *(*[2]uint64)(unsafe.Pointer(&d128))
			if gotBits != wantBits {
				t.Errorf("got bits %v, want %v", gotBits, wantBits)
			}
		})
	}
}

func TestDecimalSlice(t *testing.T) {
	// Test writing and reading slices of decimal values.
	d64s := []decimal64{1.0, 2.5, -3.14}
	d128s := []decimal128{1.0, 2.5, -3.14}

	for _, order := range []ByteOrder{LittleEndian, BigEndian} {
		t.Run("decimal64_slice/"+order.String(), func(t *testing.T) {
			buf := new(bytes.Buffer)
			if err := Write(buf, order, d64s); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if buf.Len() != 8*len(d64s) {
				t.Fatalf("got %d bytes, want %d", buf.Len(), 8*len(d64s))
			}
			got := make([]decimal64, len(d64s))
			if err := Read(bytes.NewReader(buf.Bytes()), order, got); err != nil {
				t.Fatalf("Read: %v", err)
			}
			for i := range d64s {
				wantBits := *(*uint64)(unsafe.Pointer(&d64s[i]))
				gotBits := *(*uint64)(unsafe.Pointer(&got[i]))
				if gotBits != wantBits {
					t.Errorf("[%d]: got bits %#x, want %#x", i, gotBits, wantBits)
				}
			}
		})

		t.Run("decimal128_slice/"+order.String(), func(t *testing.T) {
			buf := new(bytes.Buffer)
			if err := Write(buf, order, d128s); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if buf.Len() != 16*len(d128s) {
				t.Fatalf("got %d bytes, want %d", buf.Len(), 16*len(d128s))
			}
			got := make([]decimal128, len(d128s))
			if err := Read(bytes.NewReader(buf.Bytes()), order, got); err != nil {
				t.Fatalf("Read: %v", err)
			}
			for i := range d128s {
				wantBits := *(*[2]uint64)(unsafe.Pointer(&d128s[i]))
				gotBits := *(*[2]uint64)(unsafe.Pointer(&got[i]))
				if gotBits != wantBits {
					t.Errorf("[%d]: got bits %v, want %v", i, gotBits, wantBits)
				}
			}
		})
	}
}

func TestDecimalSize(t *testing.T) {
	var d64 decimal64
	if got := Size(d64); got != 8 {
		t.Errorf("Size(decimal64) = %d, want 8", got)
	}
	var d128 decimal128
	if got := Size(d128); got != 16 {
		t.Errorf("Size(decimal128) = %d, want 16", got)
	}
	d64s := make([]decimal64, 3)
	if got := Size(d64s); got != 24 {
		t.Errorf("Size([]decimal64) = %d, want 24", got)
	}
	d128s := make([]decimal128, 3)
	if got := Size(d128s); got != 48 {
		t.Errorf("Size([]decimal128) = %d, want 48", got)
	}
}

// The bytes are written by hand: the IEEE 754 interchange encoding of the
// decimal, in the requested byte order, as one number at each width.
func TestDecimalWireBytes(t *testing.T) {
	d64 := decimal64(1.50) // coefficient 150, exponent -2
	big64 := []byte{0x31, 0x80, 0, 0, 0, 0, 0, 0x96}
	little64 := []byte{0x96, 0, 0, 0, 0, 0, 0x80, 0x31}

	d128 := decimal128(1.50)
	big128 := []byte{0x30, 0x3c, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x96}
	little128 := []byte{0x96, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x3c, 0x30}

	type named decimal128
	tests := []struct {
		name  string
		order ByteOrder
		v     any
		want  []byte
	}{
		{"decimal64 big", BigEndian, d64, big64},
		{"decimal64 little", LittleEndian, d64, little64},
		{"decimal128 big", BigEndian, d128, big128},
		{"decimal128 little", LittleEndian, d128, little128},
		{"pointer big", BigEndian, &d128, big128},
		{"slice big", BigEndian, []decimal128{d128}, big128},
		{"slice little", LittleEndian, []decimal128{d128}, little128},
		{"struct big", BigEndian, struct{ D decimal128 }{d128}, big128},
		{"struct little", LittleEndian, struct{ D decimal128 }{d128}, little128},
		{"named big", BigEndian, struct{ D named }{named(d128)}, big128},
		{"struct64 big", BigEndian, struct{ D decimal64 }{d64}, big64},
	}
	for _, tt := range tests {
		var buf bytes.Buffer
		if err := Write(&buf, tt.order, tt.v); err != nil {
			t.Errorf("%s: Write: %v", tt.name, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), tt.want) {
			t.Errorf("%s: Write = % x, want % x", tt.name, buf.Bytes(), tt.want)
		}
	}

	var got128 decimal128
	if err := Read(bytes.NewReader(big128), BigEndian, &got128); err != nil || got128 != 1.5 {
		t.Errorf("Read big decimal128 = %v, %v", got128, err)
	}
	if err := Read(bytes.NewReader(little128), LittleEndian, &got128); err != nil || got128 != 1.5 {
		t.Errorf("Read little decimal128 = %v, %v", got128, err)
	}
	var st struct{ D decimal128 }
	if err := Read(bytes.NewReader(big128), BigEndian, &st); err != nil || st.D != 1.5 {
		t.Errorf("Read big struct = %v, %v", st.D, err)
	}
	var sl = make([]decimal128, 1)
	if err := Read(bytes.NewReader(big128), BigEndian, sl); err != nil || sl[0] != 1.5 {
		t.Errorf("Read big slice = %v, %v", sl[0], err)
	}
}
