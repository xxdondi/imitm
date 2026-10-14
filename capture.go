package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/elazarl/goproxy"
)

type captureRecorder struct {
	dir   string
	limit int64
	file  *os.File
	mu    sync.Mutex
}

func newCaptureRecorder(root string, limit int64) (*captureRecorder, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(root, time.Now().Format("20060102-150405")+"-")
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "flows.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return &captureRecorder{dir: dir, limit: limit, file: f}, nil
}

func (r *captureRecorder) Close() error { return r.file.Close() }

func (r *captureRecorder) record(event map[string]interface{}) {
	event["time"] = time.Now().UTC().Format(time.RFC3339Nano)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := json.NewEncoder(r.file).Encode(event); err != nil {
		log.Printf("Capture metadata: %v", err)
	}
}

func (r *captureRecorder) Register(proxy *goproxy.ProxyHttpServer) {
	proxy.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		log.Printf("[%d] %s %s", ctx.Session, req.Method, req.URL)
		event := map[string]interface{}{"id": ctx.Session, "type": "request", "method": req.Method, "url": req.URL.String(), "headers": req.Header.Clone(), "client": req.RemoteAddr}
		req.Body = r.wrapBody(req.Body, ctx.Session, "request", event)
		r.record(event)
		return req, nil
	})
	proxy.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		event := map[string]interface{}{"id": ctx.Session, "type": "response"}
		if resp == nil {
			event["error"] = fmt.Sprint(ctx.Error)
			r.record(event)
			return nil
		}
		log.Printf("[%d] %d %s (%s)", ctx.Session, resp.StatusCode, ctx.Req.URL, resp.Header.Get("Content-Type"))
		event["status"], event["headers"] = resp.StatusCode, resp.Header.Clone()
		resp.Body = r.wrapBody(resp.Body, ctx.Session, "response", event)
		r.record(event)
		return resp
	})
}

func (r *captureRecorder) wrapBody(body io.ReadCloser, id int64, kind string, event map[string]interface{}) io.ReadCloser {
	if body == nil || body == http.NoBody || r.limit == 0 {
		return body
	}
	name := fmt.Sprintf("%06d-%s.bin", id, kind)
	f, err := os.OpenFile(filepath.Join(r.dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		log.Printf("Capture body: %v", err)
		return body
	}
	event["body_file"], event["body_limit"] = name, r.limit
	return &capturedBody{ReadCloser: body, file: f, remaining: r.limit}
}

// Capture while forwarding: large bodies and streams are never buffered in memory.
// Disk failures must not alter the bytes returned to the proxy.
type capturedBody struct {
	io.ReadCloser
	file      *os.File
	remaining int64
}

func (b *capturedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	count := int64(n)
	if count > b.remaining {
		count = b.remaining
	}
	if count > 0 {
		if _, writeErr := b.file.Write(p[:count]); writeErr != nil {
			log.Printf("Capture body: %v", writeErr)
			b.remaining = 0
		} else {
			b.remaining -= count
		}
	}
	return n, err
}

func (b *capturedBody) Close() error {
	b.file.Close()
	return b.ReadCloser.Close()
}
