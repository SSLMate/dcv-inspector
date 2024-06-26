// Copyright (C) 2023 Opsmate, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.
//
// Except as contained in this notice, the name(s) of the above copyright
// holders shall not be used in advertising or otherwise to promote the
// sale, use or other dealings in this Software without prior written
// authorization.

package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"src.agwa.name/go-dbutil"
)

func isHTTPLogNoise(message string) bool {
	return strings.HasPrefix(message, "http: TLS handshake error")
}

type httpServerLogWriter struct{}

func (httpServerLogWriter) Write(p []byte) (int, error) {
	if message := string(p); !isHTTPLogNoise(message) {
		log.Print(message)
	}
	return len(p), nil
}

func getHTTPSConfig(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	if hello.ServerName == domain {
		return &tls.Config{
			GetCertificate: getHTTPSCertificate,
			NextProtos:     []string{"h2", "http/1.1", "acme-tls/1"},
			MinVersion:     tls.VersionTLS13,
		}, nil
	} else if _, _, ok := parseHostname(hello.ServerName); ok && !strings.HasPrefix(hello.ServerName, "_") {
		return &tls.Config{
			GetCertificate: getSelfSignedCert,
			NextProtos:     []string{"h2", "http/1.1"},
			MinVersion:     tls.VersionTLS10,
		}, nil
	} else {
		return nil, fmt.Errorf("unrecognized server name")
	}
}

func getHTTPHost(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.Host); err == nil {
		return host
	} else {
		return r.Host
	}
}

func serveHTTP(w http.ResponseWriter, r *http.Request) {
	host := getHTTPHost(r)
	ctx := r.Context()

	var err error

	if host == domain {
		err = serveDashboard(ctx, w, r)
	} else if testID, subdomain, ok := parseHostname(host); ok && !strings.HasPrefix(host, "_") {
		err = serveTestHTTP(ctx, testID, subdomain, w, r)
	} else {
		http.Error(w, fmt.Sprintf("unrecognized host name %q", host), 404)
	}
	if err != nil && ctx.Err() == nil {
		log.Print("error while serving HTTP request: ", err)
		http.Error(w, err.Error(), 500)
	}
}

func requestScheme(r *http.Request) string {
	if r.TLS == nil {
		return "http"
	} else {
		return "https"
	}
}

func serveTestHTTP(ctx context.Context, testID testID, subdomain string, w http.ResponseWriter, r *http.Request) error {
	remoteAddr, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		http.Error(w, "error parsing remote address: "+err.Error(), 400)
		return nil
	}

	if ok, err := isRunningTest(ctx, testID); err != nil {
		return fmt.Errorf("serveTestHTTP: error checking if %v is a running test: %w", testID, err)
	} else if !ok {
		http.Error(w, fmt.Sprintf("%v is not a running test", testID), 404)
		return nil
	}

	var content string
	if err := db.QueryRowContext(ctx, `SELECT content FROM http_file WHERE test_id = ? AND scheme = ? AND subdomain = ? AND path = ?`, testID[:], requestScheme(r), subdomain, r.URL.Path).Scan(&content); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("serveTestHTTP: error querying http_file row: %w", err)
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO http_request (test_id, remote_ip, remote_port, host, method, url, proto, header_json, https) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, testID[:], remoteAddr.Addr().String(), remoteAddr.Port(), r.Host, r.Method, r.URL.String(), r.Proto, dbutil.JSON(r.Header), r.TLS != nil); err != nil {
		return fmt.Errorf("serveTestHTTP: error inserting http_request for test %v: %w", testID, err)
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(content))
	return nil
}

func runHTTPServer(l net.Listener) {
	server := http.Server{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  3 * time.Second,
		Handler:      http.HandlerFunc(serveHTTP),
		ErrorLog:     log.New(httpServerLogWriter{}, "", 0),
	}
	log.Fatal(server.Serve(l))
}
func runHTTPSServer(l net.Listener) {
	runHTTPServer(tls.NewListener(l, &tls.Config{GetConfigForClient: getHTTPSConfig}))
}
