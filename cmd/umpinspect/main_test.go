package main

import (
	"errors"
	"io"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestUMPIntegerWidths(t *testing.T) {
	for _, tc := range []struct {
		data []byte
		want uint32
	}{
		{[]byte{0x7f}, 127},
		{[]byte{0x81, 0x02}, 129},
		{[]byte{0xc1, 0x02, 0x01}, 8257},
		{[]byte{0xe1, 0x02, 0x01, 0x01}, 1052705},
		{[]byte{0xf0, 0x78, 0x56, 0x34, 0x12}, 0x12345678},
	} {
		got, n, err := readInt(tc.data)
		if err != nil || got != tc.want || n != len(tc.data) {
			t.Fatalf("%x: got %d/%d/%v", tc.data, got, n, err)
		}
		if len(tc.data) > 1 {
			if _, _, err := readInt(tc.data[:len(tc.data)-1]); !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("truncated integer accepted: %x", tc.data)
			}
		}
	}
}

func TestPartialPartAndHeaderAssociation(t *testing.T) {
	// Header type 25, then a complete encrypted-response envelope, then a
	// media part whose declared size exceeds this capture. Do not invent frames.
	header := []byte{8, 25, 34, 2, 56, 0}
	opaque := protowire.AppendBytes(protowire.AppendTag(nil, 1, protowire.BytesType), []byte{1, 2, 3})
	data := append([]byte{10, byte(len(header))}, header...)
	data = append(data, 11, byte(len(opaque)))
	data = append(data, opaque...)
	data = append(data, 21, 10, 0, 1, 2)
	parts, err := split(data)
	if err != nil || len(parts) != 3 {
		t.Fatalf("split: %v/%v", parts, err)
	}
	if parts[1].HeaderType == nil || *parts[1].HeaderType != 25 || !parts[1].Crypto || parts[1].OpaqueFields[1] != 3 {
		t.Fatalf("lost encrypted header: %+v", parts[1])
	}
	if parts[2].Complete || parts[2].Available != 3 || parts[2].Declared != 10 {
		t.Fatalf("partial media misreported: %+v", parts[2])
	}
	if _, err := split([]byte{20, 0xc0}); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("incomplete size: %v", err)
	}
}
