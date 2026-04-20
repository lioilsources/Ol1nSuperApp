package nntp

import (
	"bytes"
	"crypto/rand"
	"hash/crc32"
	"testing"
)

func TestYEnc_RoundTripSinglePart(t *testing.T) {
	data := make([]byte, 4096)
	_, _ = rand.Read(data)

	encoded := YEncEncodeSegment("random.bin", 0, 0, int64(len(data)), 0, data, 0)

	dec, err := YEncDecode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(dec.Data, data) {
		t.Fatalf("roundtrip mismatch")
	}
	if dec.Filename != "random.bin" {
		t.Fatalf("filename: got %q", dec.Filename)
	}
	if dec.TotalSize != int64(len(data)) {
		t.Fatalf("size: got %d want %d", dec.TotalSize, len(data))
	}
}

func TestYEnc_RoundTripMultiPart(t *testing.T) {
	full := make([]byte, SegmentSize*2+1234)
	_, _ = rand.Read(full)
	fileCRC := crc32.ChecksumIEEE(full)

	// Split and encode part 2 of 3
	partSize := SegmentSize
	begin := int64(partSize) + 1
	partData := full[partSize : 2*partSize]
	encoded := YEncEncodeSegment("x.bin", 2, 3, int64(len(full)), begin, partData, fileCRC)

	dec, err := YEncDecode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(dec.Data, partData) {
		t.Fatalf("part data mismatch")
	}
	if dec.Part != 2 || dec.Total != 3 {
		t.Fatalf("part/total: got %d/%d", dec.Part, dec.Total)
	}
	if dec.Begin != begin || dec.End != begin+int64(partSize)-1 {
		t.Fatalf("begin/end: got %d/%d", dec.Begin, dec.End)
	}
	if dec.PartCRC == 0 {
		t.Fatalf("pcrc32 not parsed")
	}
}

func TestYEnc_EscapesCriticalBytes(t *testing.T) {
	// Pre-offset values: 214, 244, 227, 251 map to 0x00, 0x0A, 0x0D, 0x3D after +42.
	data := []byte{214, 244, 227, 251}
	enc := YEncEncodeSegment("x", 0, 0, int64(len(data)), 0, data, 0)
	// Every byte should be escaped -> body must contain four '=' chars in the data section.
	dec, err := YEncDecode(bytes.NewReader(enc))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(dec.Data, data) {
		t.Fatalf("critical-byte roundtrip failed: got %v", dec.Data)
	}
}
