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
	"testing"

	"github.com/elazarl/goproxy"
	"github.com/xxdondi/imitm/internal/capture"
	"github.com/xxdondi/imitm/internal/modules"
)

func TestPlayerAdsResponseForwarding(t *testing.T) {
	original, want := playerAdsFixture()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.Header().Set("ETag", "original")
		if r.URL.Path == "/youtubei/v1/player" {
			w.Write(fixturePlayer(original))
		} else {
			w.Write(original)
		}
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
	selected, err := modules.Select("youtube", New(recorder))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range selected {
		m.Register(proxy)
	}
	server := httptest.NewServer(proxy)
	defer server.Close()
	proxyURL, _ := url.Parse(server.URL)
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	for _, tc := range []struct {
		host, path string
		modified   bool
	}{
		{"youtubei.googleapis.com", "/youtubei/v1/get_watch", true},
		{"youtubei.googleapis.com", "/youtubei/v1/player", true},
		{"youtubei.googleapis.com", "/youtubei/v1/next", false},
		{"example.com", "/youtubei/v1/get_watch", false},
	} {
		resp, err := client.Get("http://" + tc.host + tc.path)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		expected := original
		if tc.modified {
			expected = want
			if tc.path == "/youtubei/v1/player" {
				expected = fixturePlayer(want)
			}
			if resp.Header.Get("ETag") != "" {
				t.Fatal("stale validator")
			}
		}
		if !bytes.Equal(data, expected) || resp.ContentLength != int64(len(expected)) {
			t.Fatal("incorrect body/length")
		}
	}
	resp, err := client.Post("http://rr1.googlevideo.com/initplayback", "application/x-protobuf", bytes.NewReader([]byte("opaque request")))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 502 {
		t.Fatal("combined mode did not block Onesie")
	}
	for name, want := range map[string][]byte{"000001-response.bin": original, "000001-playerads-original.bin": original, "000001-playerads-modified.bin": want} {
		got, err := os.ReadFile(filepath.Join(recorder.Dir(), name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("capture %s: %v", name, err)
		}
	}
}
