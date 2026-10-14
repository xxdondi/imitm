// protoinspect decodes captures offline and locates ad markers in wire fields.
package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/protocolbuffers/protoscope"
	"google.golang.org/protobuf/encoding/protowire"
)

type field struct {
	Number        protowire.Number `json:"number"`
	Type          protowire.Type   `json:"wire_type"`
	Offset        int              `json:"tag_offset"`
	End           int              `json:"end_offset"`
	PayloadOffset int              `json:"payload_offset,omitempty"`
	PayloadSize   int              `json:"payload_size,omitempty"`
}

type marker struct {
	Text   string  `json:"marker"`
	Offset int     `json:"offset"`
	Path   []field `json:"enclosing_fields"`
}

type report struct {
	File          string   `json:"file"`
	Size          int      `json:"bytes"`
	CapturedBytes int      `json:"captured_bytes"`
	Encoding      string   `json:"input_encoding"`
	SHA256        string   `json:"sha256"`
	ValidWire     bool     `json:"valid_top_level_wire"`
	Roundtrip     bool     `json:"byte_identical_protoscope_roundtrip"`
	Markers       []marker `json:"markers"`
	Note          string   `json:"note"`
}

// Validate the entire candidate message before descending into it. A bytes field
// can also be a string, packed numbers, or opaque data: nesting remains inferred.
func parseFields(data []byte, base int) ([]field, bool) {
	var fields []field
	for pos := 0; pos < len(data); {
		num, typ, n := protowire.ConsumeTag(data[pos:])
		if n < 0 || num < 1 || num > (1<<29)-1 {
			return nil, false
		}
		start := pos
		pos += n
		size := protowire.ConsumeFieldValue(num, typ, data[pos:])
		if size < 0 {
			return nil, false
		}
		f := field{Number: num, Type: typ, Offset: base + start, End: base + pos + size}
		if typ == protowire.BytesType {
			payload, consumed := protowire.ConsumeBytes(data[pos:])
			if consumed < 0 {
				return nil, false
			}
			f.PayloadOffset = base + pos + consumed - len(payload)
			f.PayloadSize = len(payload)
		}
		fields = append(fields, f)
		pos += size
	}
	return fields, true
}

func enclosingPath(data []byte, base, target, depth int) []field {
	if depth > 64 {
		return nil
	}
	fields, valid := parseFields(data, base)
	if !valid {
		return nil
	}
	for _, f := range fields {
		if target < f.Offset || target >= f.End {
			continue
		}
		path := []field{f}
		if f.Type == protowire.BytesType && target >= f.PayloadOffset {
			start := f.PayloadOffset - base
			path = append(path, enclosingPath(data[start:start+f.PayloadSize], f.PayloadOffset, target, depth+1)...)
		}
		return path
	}
	return nil
}

func inspect(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	capturedSize := len(data)
	capturedHash := fmt.Sprintf("%x", sha256.Sum256(data))
	encoding := "identity"
	if bytes.HasPrefix(data, []byte{0x1f, 0x8b}) {
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return err
		}
		data, err = io.ReadAll(io.LimitReader(reader, 32*1024*1024+1))
		reader.Close()
		if err != nil {
			return fmt.Errorf("gunzip %s: %w", path, err)
		}
		if len(data) > 32*1024*1024 {
			return fmt.Errorf("gunzip %s: exceeds 32 MiB inspection limit", path)
		}
		encoding = "gzip"
		if err := os.WriteFile(path+".gunzip.bin", data, 0600); err != nil {
			return err
		}
	}
	text := protoscope.Write(data, protoscope.WriterOptions{NoGroups: true})
	reencoded, err := protoscope.NewScanner(text).Exec()
	if err != nil {
		return fmt.Errorf("roundtrip %s: %w", path, err)
	}
	_, valid := parseFields(data, 0)
	r := report{File: path, Size: len(data), CapturedBytes: capturedSize, Encoding: encoding, SHA256: capturedHash, ValidWire: valid,
		Roundtrip: bytes.Equal(data, reencoded), Markers: []marker{},
		Note: "SHA256 covers the captured input; bytes, wire checks, roundtrip and marker offsets cover decompressed data when input_encoding is gzip. Field nesting is inferred without a schema. Ad markers identify candidates, not proof that a field is safe to remove. A valid wire message alone does not prove the HTTP body was fully captured."}
	for _, signature := range []string{"/pagead/", "googleadservices.com", "Visit advertiser", "adPlacements", "adSlots"} {
		for start := 0; start < len(data); {
			i := bytes.Index(data[start:], []byte(signature))
			if i < 0 {
				break
			}
			i += start
			r.Markers = append(r.Markers, marker{Text: signature, Offset: i, Path: enclosingPath(data, 0, i, 0)})
			start = i + len(signature)
		}
	}
	encoded, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path+".pscope.txt", []byte(text), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(path+".analysis.json", append(encoded, '\n'), 0600); err != nil {
		return err
	}
	fmt.Printf("%s: %d bytes, wire=%t, exact roundtrip=%t, markers=%d\n", path, r.Size, r.ValidWire, r.Roundtrip, len(r.Markers))
	for _, m := range r.Markers {
		var parts []string
		for _, f := range m.Path {
			parts = append(parts, fmt.Sprint(f.Number))
		}
		fmt.Printf("  %s @ %d: %s\n", m.Text, m.Offset, strings.Join(parts, "/"))
	}
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: go run ./cmd/protoinspect FILE.bin [FILE.bin ...]")
		os.Exit(2)
	}
	for _, path := range os.Args[1:] {
		if err := inspect(path); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
