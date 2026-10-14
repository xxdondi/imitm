package youtube

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/elazarl/goproxy"
	"github.com/xxdondi/imitm/internal/capture"
)

func TestInitplayback502ScopeAndRecording(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.Write([]byte("upstream intact"))
	}))
	defer upstream.Close()
	proxy := goproxy.NewProxyHttpServer()
	proxy.Tr.Proxy = nil
	proxy.Tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, upstream.Listener.Addr().String())
	}
	recorder, err := capture.NewRecorder(t.TempDir(), 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close()
	recorder.Register(proxy)
	New(recorder).Register(proxy)
	server := httptest.NewServer(proxy)
	defer server.Close()
	proxyURL, _ := url.Parse(server.URL)
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	body := []byte("opaque Onesie request")
	for _, tc := range []struct {
		method, host, path string
		blocked            bool
	}{
		{"POST", "rr1.googlevideo.com", "/initplayback", true},
		{"POST", "redirector.googlevideo.com", "/initplayback", true},
		{"GET", "rr1.googlevideo.com", "/initplayback", false},
		{"POST", "rr1.googlevideo.com", "/videoplayback", false},
		{"POST", "googlevideo.com.example.org", "/initplayback", false},
		{"POST", "youtubei.googleapis.com", "/youtubei/v1/get_watch", false},
	} {
		before := upstreamCalls.Load()
		req, _ := http.NewRequest(tc.method, "http://"+tc.host+tc.path, bytes.NewReader(body))
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if tc.blocked {
			if resp.StatusCode != 502 || upstreamCalls.Load() != before || resp.Header.Get("Cache-Control") != "no-store" {
				t.Fatal("blocked request forwarded or wrong response")
			}
		} else if resp.StatusCode != 200 || string(data) != "upstream intact" || upstreamCalls.Load() != before+1 {
			t.Fatal("unrelated request changed")
		}
	}
	got, err := os.ReadFile(filepath.Join(recorder.Dir(), "000001-request.bin"))
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("missing blocked request capture: %v", err)
	}
	metadata, err := os.ReadFile(filepath.Join(recorder.Dir(), "flows.jsonl"))
	if err != nil || !bytes.Contains(metadata, []byte(`"type":"initplayback502"`)) || !bytes.Contains(metadata, []byte(`"status":502`)) {
		t.Fatal("missing experiment/response metadata")
	}
}
