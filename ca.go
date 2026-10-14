package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/elazarl/goproxy"
)

func loadOrCreateCA(dir string) (tls.Certificate, []byte, error) {
	var ca tls.Certificate
	if err := os.MkdirAll(dir, 0700); err != nil {
		return ca, nil, err
	}
	certPath, keyPath := filepath.Join(dir, "imitm-ca.pem"), filepath.Join(dir, "imitm-ca-key.pem")
	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)
	if os.IsNotExist(certErr) && os.IsNotExist(keyErr) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return ca, nil, err
		}
		serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			return ca, nil, err
		}
		template := &x509.Certificate{
			SerialNumber: serial, Subject: pkix.Name{CommonName: "IMTM Local Proxy CA", Organization: []string{"IMTM"}},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(5, 0, 0),
			IsCA: true, BasicConstraintsValid: true,
			KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		}
		der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		if err != nil {
			return ca, nil, err
		}
		if err := writeNewFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
			return ca, nil, err
		}
		if err := writeNewFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644); err != nil {
			return ca, nil, err
		}
	}
	var err error
	ca, err = tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return ca, nil, fmt.Errorf("load local CA (both certificate and key must exist): %w", err)
	}
	ca.Leaf, err = x509.ParseCertificate(ca.Certificate[0])
	if err != nil {
		return ca, nil, err
	}
	if !ca.Leaf.IsCA || time.Now().Before(ca.Leaf.NotBefore) || time.Now().After(ca.Leaf.NotAfter) {
		return ca, nil, fmt.Errorf("local CA is not a valid, current CA certificate")
	}
	der := ca.Certificate[0]
	if err := os.WriteFile(filepath.Join(dir, "imitm-ca.cer"), der, 0644); err != nil {
		return ca, nil, err
	}
	return ca, der, nil
}

func writeNewFile(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func registerCertificateDownload(proxy *goproxy.ProxyHttpServer, der []byte) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ca.cer" {
			http.Error(w, "IMTM proxy: download the CA at /ca.cer", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/x-x509-ca-cert")
		w.Header().Set("Content-Disposition", `attachment; filename="imitm-ca.cer"`)
		w.Write(der)
	})
	proxy.NonproxyHandler = handler
	proxy.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		if req.URL.Scheme == "http" && req.URL.Hostname() == "imitm.local" && req.URL.Path == "/ca.cer" {
			resp := goproxy.NewResponse(req, "application/x-x509-ca-cert", http.StatusOK, string(der))
			resp.Header.Set("Content-Disposition", `attachment; filename="imitm-ca.cer"`)
			return req, resp
		}
		return req, nil
	})
}
