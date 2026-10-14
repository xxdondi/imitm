package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"flag"
	"log"
	"net"
	"net/http"
	"net/url"

	"github.com/elazarl/goproxy"
	"github.com/inconshreveable/go-vhost"
)

type MiddlewareRegisterer interface {
	Register(proxy *goproxy.ProxyHttpServer)
}

func main() {
	verbose := flag.Bool("v", false, "should every proxy request be logged to stdout")
	mode := flag.String("mode", "capture", "capture or youtube (experimental response modification)")
	caDir := flag.String("ca-dir", "certs", "directory for the local CA certificate and private key")
	initCA := flag.Bool("init-ca", false, "create/export the local CA and exit")
	captureDir := flag.String("capture-dir", "data/captures", "directory for capture metadata and bodies")
	bodyLimit := flag.Int64("body-limit", 1024*1024, "maximum captured bytes per body; 0 disables body capture")
	http_addr := flag.String("httpaddr", ":3129", "proxy http listen address")
	https_addr := flag.String("httpsaddr", "", "optional redirected TLS listen address, e.g. :3128")
	flag.Parse()
	if (*mode != "capture" && *mode != "youtube") || *bodyLimit < 0 {
		log.Fatal("mode must be capture or youtube; body-limit must be nonnegative")
	}
	ca, certDER, err := loadOrCreateCA(*caDir)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Install %s/imitm-ca.cer on your phone and enable full trust", *caDir)
	if *initCA {
		return
	}

	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = *verbose
	// Verify the upstream server independently of the phone's trust in our CA.
	proxy.Tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	proxy.OnRequest().HandleConnect(goproxy.FuncHttpsHandler(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		log.Printf("CONNECT %s from %s", host, ctx.Req.RemoteAddr)
		return &goproxy.ConnectAction{Action: goproxy.ConnectMitm, TLSConfig: goproxy.TLSConfigFromCA(&ca)}, host
	}))
	registerCertificateDownload(proxy, certDER)
	recorder, err := newCaptureRecorder(*captureDir, *bodyLimit)
	if err != nil {
		log.Fatal(err)
	}
	defer recorder.Close()
	recorder.Register(proxy)
	if *mode == "youtube" {
		NewYoutubeAdblockMiddleware().Register(proxy)
	}
	log.Printf("%s proxy listening on %s; captures in %s", *mode, *http_addr, recorder.dir)
	if *https_addr == "" {
		log.Fatal(http.ListenAndServe(*http_addr, proxy))
	}

	go func() {
		log.Fatalln(http.ListenAndServe(*http_addr, proxy))
	}()

	// listen to the TLS ClientHello but make it a CONNECT request instead
	ln, err := net.Listen("tcp", *https_addr)
	if err != nil {
		log.Fatalf("Error listening for https connections - %v", err)
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			log.Printf("Error accepting new connection - %v", err)
			continue
		}
		go func(c net.Conn) {
			tlsConn, err := vhost.TLS(c)
			if err != nil {
				log.Printf("Error accepting new connection - %v", err)
				c.Close()
				return
			}
			if tlsConn.Host() == "" {
				log.Printf("Cannot support non-SNI enabled clients")
				c.Close()
				return
			}
			connectReq := &http.Request{
				Method: "CONNECT",
				URL: &url.URL{
					Opaque: tlsConn.Host(),
					Host:   net.JoinHostPort(tlsConn.Host(), "443"),
				},
				Host:       tlsConn.Host(),
				Header:     make(http.Header),
				RemoteAddr: c.RemoteAddr().String(),
			}
			resp := dumbResponseWriter{tlsConn}
			proxy.ServeHTTP(resp, connectReq)
		}(c)
	}
}

type dumbResponseWriter struct {
	net.Conn
}

func (dumb dumbResponseWriter) Header() http.Header {
	panic("Header() should not be called on this ResponseWriter")
}

func (dumb dumbResponseWriter) Write(buf []byte) (int, error) {
	if bytes.Equal(buf, []byte("HTTP/1.0 200 OK\r\n\r\n")) || bytes.Equal(buf, []byte("HTTP/1.0 200 Connection established\r\n\r\n")) {
		return len(buf), nil // throw away the HTTP OK response from the faux CONNECT request
	}
	return dumb.Conn.Write(buf)
}

func (dumb dumbResponseWriter) WriteHeader(code int) {
	panic("WriteHeader() should not be called on this ResponseWriter")
}

func (dumb dumbResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return dumb, bufio.NewReadWriter(bufio.NewReader(dumb), bufio.NewWriter(dumb)), nil
}
