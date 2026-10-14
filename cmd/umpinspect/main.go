// umpinspect splits offline YouTube UMP captures into framing and control data.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/protocolbuffers/protoscope"
	"google.golang.org/protobuf/encoding/protowire"
)

// UMP integers are not protobuf varints. See LuanRT/googlevideo UmpReader.ts.
func readInt(data []byte) (uint32, int, error) {
	if len(data) == 0 {
		return 0, 0, io.ErrUnexpectedEOF
	}
	b := data[0]
	n := 1
	switch {
	case b < 128:
		n = 1
	case b < 192:
		n = 2
	case b < 224:
		n = 3
	case b < 240:
		n = 4
	default:
		n = 5
	}
	if len(data) < n {
		return 0, 0, io.ErrUnexpectedEOF
	}
	switch n {
	case 1:
		return uint32(b), n, nil
	case 2:
		return uint32(b&63) | uint32(data[1])<<6, n, nil
	case 3:
		return uint32(b&31) | uint32(data[1])<<5 | uint32(data[2])<<13, n, nil
	case 4:
		return uint32(b&15) | uint32(data[1])<<4 | uint32(data[2])<<12 | uint32(data[3])<<20, n, nil
	default:
		return binary.LittleEndian.Uint32(data[1:5]), n, nil
	}
}

type part struct {
	Index        int                      `json:"index"`
	Offset       int                      `json:"offset"`
	Type         uint32                   `json:"type"`
	Name         string                   `json:"name"`
	Declared     uint32                   `json:"declared_bytes"`
	Available    int                      `json:"available_bytes"`
	Complete     bool                     `json:"complete"`
	HeaderType   *uint64                  `json:"onesie_header_type,omitempty"`
	VideoID      string                   `json:"video_id,omitempty"`
	OpaqueFields map[protowire.Number]int `json:"bytes_field_sizes,omitempty"`
	Crypto       bool                     `json:"has_crypto_params,omitempty"`
	File         string                   `json:"payload_file,omitempty"`
	Wire         bool                     `json:"valid_protobuf_wire,omitempty"`
	Gzip         bool                     `json:"gzip_decoded,omitempty"`
	payload      []byte
}

var names = map[uint32]string{10: "ONESIE_HEADER", 11: "ONESIE_DATA", 12: "ONESIE_ENCRYPTED_MEDIA", 20: "MEDIA_HEADER", 21: "MEDIA", 22: "MEDIA_END", 35: "NEXT_REQUEST_POLICY", 42: "FORMAT_INITIALIZATION_METADATA", 43: "SABR_REDIRECT", 44: "SABR_ERROR", 47: "PLAYBACK_START_POLICY", 55: "TIMELINE_CONTEXT", 57: "SABR_CONTEXT_UPDATE", 58: "STREAM_PROTECTION_STATUS", 59: "SABR_CONTEXT_SENDING_POLICY", 62: "END_OF_TRACK", 69: "CUEPOINT_LIST", 70: "STITCHED_REGIONS_OF_INTEREST", 71: "STITCHED_SEGMENTS_METADATA_LIST"}

func split(data []byte) ([]part, error) {
	var parts []part
	var lastType *uint64
	var crypto bool
	for pos := 0; pos < len(data); {
		start := pos
		typ, n, err := readInt(data[pos:])
		if err != nil {
			return parts, fmt.Errorf("type @ %d: %w", pos, err)
		}
		pos += n
		size, n, err := readInt(data[pos:])
		if err != nil {
			return parts, fmt.Errorf("size @ %d: %w", pos, err)
		}
		pos += n
		available := len(data) - pos
		complete := uint64(size) <= uint64(available)
		if complete {
			available = int(size)
		}
		p := part{Index: len(parts), Offset: start, Type: typ, Name: names[typ], Declared: size, Available: available, Complete: complete, payload: data[pos : pos+available]}
		if p.Name == "" {
			p.Name = "UNKNOWN"
		}
		if typ == 10 && complete {
			lastType = nil
			crypto = false
			walk(p.payload, func(num protowire.Number, typ protowire.Type, v uint64, b []byte) {
				if num == 1 && typ == protowire.VarintType {
					value := v
					lastType = &value
					p.HeaderType = lastType
				}
				if num == 2 && typ == protowire.BytesType {
					p.VideoID = string(b)
				}
				if num == 4 && typ == protowire.BytesType && len(b) > 0 {
					crypto = true
				}
			})
			p.Crypto = crypto
		} else if typ == 11 {
			p.HeaderType = lastType
			p.Crypto = crypto
			lastType = nil
			crypto = false
		}
		if typ == 20 && complete {
			walk(p.payload, func(num protowire.Number, typ protowire.Type, v uint64, b []byte) {
				if num == 2 && typ == protowire.BytesType {
					p.VideoID = string(b)
				}
			})
		}
		if typ == 11 && complete && p.HeaderType != nil && *p.HeaderType == 25 {
			p.OpaqueFields = map[protowire.Number]int{}
			walk(p.payload, func(num protowire.Number, typ protowire.Type, v uint64, b []byte) {
				if typ == protowire.BytesType {
					p.OpaqueFields[num] = len(b)
				}
			})
		}
		parts = append(parts, p)
		pos += available
		if !complete {
			break
		}
	}
	return parts, nil
}

// Walking wire fields does not guess the numerical or message schema.
func walk(data []byte, visit func(protowire.Number, protowire.Type, uint64, []byte)) bool {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 || num < 1 || num > (1<<29)-1 {
			return false
		}
		data = data[n:]
		size := protowire.ConsumeFieldValue(num, typ, data)
		if size < 0 {
			return false
		}
		var v uint64
		var b []byte
		if typ == protowire.VarintType {
			v, _ = protowire.ConsumeVarint(data)
		}
		if typ == protowire.BytesType {
			b, _ = protowire.ConsumeBytes(data)
		}
		if visit != nil {
			visit(num, typ, v, b)
		}
		data = data[size:]
	}
	return true
}

func inspect(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	parts, parseErr := split(data)
	dir := path + ".ump"
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for i := range parts {
		p := &parts[i]
		// Keep media bytes in the original capture; extract control payloads only.
		if !p.Complete || p.Type == 21 || p.Type == 12 || p.Type == 22 {
			continue
		}
		p.File = fmt.Sprintf("%03d-type-%d.bin", p.Index, p.Type)
		if err := os.WriteFile(filepath.Join(dir, p.File), p.payload, 0600); err != nil {
			return err
		}
		decoded := p.payload
		if bytes.HasPrefix(decoded, []byte{0x1f, 0x8b}) {
			gz, err := gzip.NewReader(bytes.NewReader(decoded))
			if err != nil {
				return err
			}
			decoded, err = io.ReadAll(io.LimitReader(gz, 32*1024*1024+1))
			gz.Close()
			if err != nil {
				return err
			}
			if len(decoded) > 32*1024*1024 {
				return fmt.Errorf("oversized gzip control payload")
			}
			p.Gzip = true
			if err := os.WriteFile(filepath.Join(dir, p.File+".gunzip.bin"), decoded, 0600); err != nil {
				return err
			}
		}
		p.Wire = walk(decoded, nil)
		if p.Wire {
			text := protoscope.Write(decoded, protoscope.WriterOptions{NoGroups: true})
			if err := os.WriteFile(filepath.Join(dir, p.File+".pscope.txt"), []byte(text), 0600); err != nil {
				return err
			}
		}
	}
	r := map[string]interface{}{"file": path, "bytes": len(data), "parts": parts, "note": "Incomplete parts may reflect protocol continuation, interrupted capture, or the capture limit; original capture has no EOF marker. Part names and Onesie header interpretation use LuanRT/googlevideo community schemas."}
	if parseErr != nil {
		r["framing_error"] = parseErr.Error()
	}
	encoded, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "parts.json"), append(encoded, '\n'), 0600); err != nil {
		return err
	}
	counts := map[string]int{}
	videoIDs := map[string]bool{}
	for _, p := range parts {
		counts[p.Name]++
		if p.VideoID != "" {
			videoIDs[p.VideoID] = true
		}
	}
	summary, _ := json.Marshal(counts)
	fmt.Printf("%s: %d bytes, %d parts %s\n", path, len(data), len(parts), summary)
	if len(videoIDs) > 0 {
		ids, _ := json.Marshal(videoIDs)
		fmt.Printf("  video IDs: %s\n", ids)
	}
	for _, p := range parts {
		if p.Type == 10 || p.Type == 11 || !p.Complete {
			var ht interface{}
			if p.HeaderType != nil {
				ht = *p.HeaderType
			}
			fmt.Printf("  part %d %s declared=%d available=%d complete=%t header=%v crypto=%t wire=%t gzip=%t\n", p.Index, p.Name, p.Declared, p.Available, p.Complete, ht, p.Crypto, p.Wire, p.Gzip)
		}
	}
	if parseErr != nil {
		fmt.Printf("  framing: %v\n", parseErr)
	}
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: go run ./cmd/umpinspect FILE.bin [FILE.bin ...]")
		os.Exit(2)
	}
	for _, path := range os.Args[1:] {
		if err := inspect(path); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
