package youtube

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strconv"

	"github.com/elazarl/goproxy"
	"github.com/xxdondi/imitm/internal/modules"
	"google.golang.org/protobuf/encoding/protowire"
)

type playerAdRemoval struct {
	Field protowire.Number `json:"field"`
	Bytes int              `json:"wire_bytes_removed"`
}

// Strip only direct fields 7/68 of the observed get_watch content player at
// 1/2. Preserve the account's own content data and the separate UI envelope.
func removePlayerAds(original []byte) ([]byte, []playerAdRemoval, error) {
	return removePlayerAdsAt(original, 0)
}

// /player returns the player directly; /get_watch wraps it at 1/2.
func removePlayerAdsAt(original []byte, startDepth int) ([]byte, []playerAdRemoval, error) {
	var changes []playerAdRemoval
	var rewrite func([]byte, int) ([]byte, error)
	rewrite = func(data []byte, depth int) ([]byte, error) {
		fields, valid := wireFields(data)
		if !valid {
			return nil, fmt.Errorf("invalid message at depth %d", depth)
		}
		if depth == 2 {
			hasContent, hasFormats, hasAdSlot := false, false, false
			for _, f := range fields {
				if f.typ != protowire.BytesType {
					continue
				}
				payload := data[f.payloadStart:f.end]
				if f.num == 4 {
					_, hasFormats = wireFields(payload)
				}
				if f.num == 11 {
					details, valid := wireFields(payload)
					if valid {
						for _, d := range details {
							if d.num == 1 && d.typ == protowire.BytesType && d.end-d.payloadStart == 11 {
								hasContent = true
							}
						}
					}
				}
				if f.num == 68 {
					slots, valid := wireFields(payload)
					if !valid {
						return nil, fmt.Errorf("invalid ad slot")
					}
					for _, slot := range slots {
						if slot.num == 424701016 && slot.typ == protowire.BytesType && bytes.Contains(payload, []byte("/pagead/")) {
							hasAdSlot = true
						}
					}
				}
			}
			if !hasContent || !hasFormats || !hasAdSlot {
				return bytes.Clone(data), nil
			}
			var out []byte
			for _, f := range fields {
				if f.num == 7 || f.num == 68 {
					if f.typ != protowire.BytesType {
						return nil, fmt.Errorf("unexpected ad field type")
					}
					changes = append(changes, playerAdRemoval{Field: f.num, Bytes: f.end - f.start})
				} else {
					out = append(out, data[f.start:f.end]...)
				}
			}
			return out, nil
		}
		var out []byte
		for _, f := range fields {
			target := protowire.Number(1)
			if depth == 1 {
				target = 2
			}
			if f.num != target || f.typ != protowire.BytesType {
				out = append(out, data[f.start:f.end]...)
				continue
			}
			payload := data[f.payloadStart:f.end]
			modified, err := rewrite(payload, depth+1)
			if err != nil {
				return nil, err
			}
			if bytes.Equal(payload, modified) {
				out = append(out, data[f.start:f.end]...)
				continue
			}
			_, _, tagLen := protowire.ConsumeTag(data[f.start:])
			out = append(out, data[f.start:f.start+tagLen]...)
			out = protowire.AppendBytes(out, modified)
		}
		return out, nil
	}
	out, err := rewrite(original, startDepth)
	if err != nil {
		return bytes.Clone(original), nil, err
	}
	return out, changes, nil
}

func registerPlayerAds(proxy *goproxy.ProxyHttpServer, evidence modules.Evidence) {
	proxy.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		if resp == nil || resp.Body == nil || resp.StatusCode != http.StatusOK || ctx.Req.URL.Hostname() != "youtubei.googleapis.com" || (ctx.Req.URL.Path != "/youtubei/v1/get_watch" && ctx.Req.URL.Path != "/youtubei/v1/player") {
			return resp
		}
		mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/x-protobuf" {
			return resp
		}
		if encoding := resp.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
			return resp
		}
		body := resp.Body
		const maxBody = 16 * 1024 * 1024
		original, err := io.ReadAll(io.LimitReader(body, maxBody+1))
		if err != nil || len(original) > maxBody {
			resp.Body = replayBody{Reader: io.MultiReader(bytes.NewReader(original), body), Closer: body}
			log.Printf("[%d] PLAYERADS skipped oversized/unreadable response: %v", ctx.Session, err)
			return resp
		}
		body.Close()
		depth := 0
		if ctx.Req.URL.Path == "/youtubei/v1/player" {
			depth = 2
		}
		modified, changes, err := removePlayerAdsAt(original, depth)
		resp.Body = io.NopCloser(bytes.NewReader(modified))
		if err != nil {
			log.Printf("[%d] PLAYERADS skipped: %v", ctx.Session, err)
			return resp
		}
		if len(changes) == 0 {
			return resp
		}
		resp.ContentLength = int64(len(modified))
		resp.TransferEncoding = nil
		resp.Header.Set("Content-Length", strconv.Itoa(len(modified)))
		resp.Header.Del("Transfer-Encoding")
		resp.Header.Del("ETag")
		resp.Header.Del("Content-MD5")
		resp.Header.Del("Digest")
		prefix := fmt.Sprintf("%06d-playerads", ctx.Session)
		for suffix, data := range map[string][]byte{"original.bin": original, "modified.bin": modified} {
			if err := evidence.SaveArtifact(prefix+"-"+suffix, data); err != nil {
				log.Printf("Player ads sample: %v", err)
			}
		}
		evidence.Record(map[string]interface{}{"id": ctx.Session, "type": "playerads", "endpoint": ctx.Req.URL.Path, "changes": changes, "original_bytes": len(original), "modified_bytes": len(modified), "sample_prefix": prefix})
		log.Printf("[%d] PLAYERADS: removed %d fields; bytes %d -> %d", ctx.Session, len(changes), len(original), len(modified))
		return resp
	})
}
