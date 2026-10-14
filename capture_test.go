package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/elazarl/goproxy"
)

func TestCaptureHTTPAndHTTPS(t *testing.T) {
	caDir := t.TempDir()
	ca, der, err := loadOrCreateCA(caDir)
	if err != nil {
		t.Fatal(err)
	}
	_, secondDER, err := loadOrCreateCA(caDir)
	if err != nil || !bytes.Equal(der, secondDER) {
		t.Fatalf("CA changed on reload: %v", err)
	}
	info, err := os.Stat(filepath.Join(caDir, "imitm-ca-key.pem"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private key permissions: %v", err)
	}
	payload := []byte("\x00protobuf-like body /pagead/ intact\xff")
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(body, payload) {
			t.Errorf("upstream body changed: %q, %v", body, err)
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.Write(payload)
	})
	httpOrigin := httptest.NewServer(handler)
	defer httpOrigin.Close()
	tlsOrigin := httptest.NewTLSServer(handler)
	defer tlsOrigin.Close()
	proxy := goproxy.NewProxyHttpServer()
	upstreamRoots := x509.NewCertPool()
	upstreamRoots.AddCert(tlsOrigin.Certificate())
	proxy.Tr.TLSClientConfig = &tls.Config{RootCAs: upstreamRoots}
	proxy.OnRequest().HandleConnect(goproxy.FuncHttpsHandler(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		return &goproxy.ConnectAction{Action: goproxy.ConnectMitm, TLSConfig: goproxy.TLSConfigFromCA(&ca)}, host
	}))
	registerCertificateDownload(proxy, der)
	recorder, err := newCaptureRecorder(t.TempDir(), 7)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close()
	recorder.Register(proxy)
	server := httptest.NewServer(proxy)
	defer server.Close()
	proxyURL, _ := url.Parse(server.URL)
	roots := x509.NewCertPool()
	roots.AddCert(ca.Leaf)
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL), TLSClientConfig: &tls.Config{RootCAs: roots}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	for _, origin := range []string{httpOrigin.URL, tlsOrigin.URL} {
		resp, err := client.Post(origin+"/capture", "application/x-protobuf", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || !bytes.Equal(body, payload) {
			t.Fatalf("downstream body changed: %q, %v", body, err)
		}
	}
	for _, address := range []string{server.URL + "/ca.cer", "http://imitm.local/ca.cer"} {
		resp, err := client.Get(address)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !bytes.Equal(body, der) {
			t.Fatalf("certificate download mismatch at %s", address)
		}
	}
	files, err := filepath.Glob(filepath.Join(recorder.dir, "*-*.bin"))
	if err != nil || len(files) < 4 {
		t.Fatalf("missing captures: %v, %v", files, err)
	}
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil || len(body) > 7 {
			t.Fatalf("capture limit failed: %s, %v", file, err)
		}
	}
	metadata, err := os.ReadFile(filepath.Join(recorder.dir, "flows.jsonl"))
	if err != nil || !bytes.Contains(metadata, []byte(`"status":200`)) || !bytes.Contains(metadata, []byte("https://")) {
		t.Fatalf("missing metadata: %s, %v", metadata, err)
	}
}
