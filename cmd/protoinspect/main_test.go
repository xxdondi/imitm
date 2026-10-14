package main

import (
	"bytes"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func bytesField(num protowire.Number, payload []byte) []byte {
	return protowire.AppendBytes(protowire.AppendTag(nil, num, protowire.BytesType), payload)
}

func TestEnclosingPathUsesWireBoundaries(t *testing.T) {
	// A tag-like sequence inside the URL must not become a message ancestor.
	url := append([]byte("https://example.com/pagead/"), protowire.AppendTag(nil, 49399797, protowire.BytesType)...)
	data := bytesField(14, bytesField(62960614, bytesField(1, url)))
	offset := bytes.Index(data, []byte("/pagead/"))
	path := enclosingPath(data, 0, offset, 0)
	want := []protowire.Number{14, 62960614, 1}
	if len(path) != len(want) {
		t.Fatalf("unexpected path: %+v", path)
	}
	for i, num := range want {
		if path[i].Number != num || offset < path[i].PayloadOffset || offset >= path[i].End {
			t.Fatalf("wrong ancestor: %+v", path[i])
		}
	}
}

func TestRejectTruncatedMessages(t *testing.T) {
	data := bytesField(42, bytesField(49399797, []byte("/pagead/")))
	if _, valid := parseFields(data[:len(data)-1], 0); valid {
		t.Fatal("truncated message accepted")
	}
	if path := enclosingPath(data[:len(data)-1], 0, 8, 0); len(path) != 0 {
		t.Fatalf("invented truncated ancestry: %+v", path)
	}
	if _, valid := parseFields([]byte{0}, 0); valid {
		t.Fatal("field zero accepted")
	}
}
