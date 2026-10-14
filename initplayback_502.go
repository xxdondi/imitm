package main

import (
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/elazarl/goproxy"
)

func registerInitplayback502Experiment(proxy *goproxy.ProxyHttpServer, recorder *captureRecorder) {
	proxy.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		if req.Method != http.MethodPost || req.URL.Path != "/initplayback" || !strings.HasSuffix(strings.ToLower(req.URL.Hostname()), ".googlevideo.com") {
			return req, nil
		}
		// Read through the recorder so blocked requests remain inspectable.
		// Bound draining even though the observed Onesie bodies are small.
		var drained int64
		var err error
		if req.Body != nil {
			drained, err = io.Copy(io.Discard, io.LimitReader(req.Body, 1024*1024+1))
			req.Body.Close()
		}
		event := map[string]interface{}{"id": ctx.Session, "type": "initplayback502", "host": req.URL.Hostname(), "status": http.StatusBadGateway, "request_bytes_read": drained, "drain_limit_reached": drained > 1024*1024}
		if err != nil {
			event["read_error"] = err.Error()
		}
		recorder.record(event)
		resp := goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusBadGateway, "initplayback blocked by local proxy experiment\n")
		resp.Header.Set("Cache-Control", "no-store")
		log.Printf("[%d] INITPLAYBACK502: blocked POST /initplayback on %s", ctx.Session, req.URL.Hostname())
		return req, resp
	})
}
