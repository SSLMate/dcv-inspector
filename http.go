package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"src.agwa.name/go-dbutil"
)

func getHTTPSConfig(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	if hello.ServerName == domain {
		return &tls.Config{
			GetCertificate: getHTTPSCertificate,
			NextProtos:     []string{"h2", "http/1.1", "acme-tls/1"},
			MinVersion:     tls.VersionTLS13,
		}, nil
	} else if _, ok := parseHostname(hello.ServerName); ok && !strings.HasPrefix(hello.ServerName, "_") {
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
	} else if testID, ok := parseHostname(host); ok && !strings.HasPrefix(host, "_") {
		err = serveTestHTTP(ctx, testID, w, r)
	} else {
		http.Error(w, fmt.Sprintf("unrecognized host name %q", host), 404)
	}
	if err != nil && ctx.Err() == nil {
		log.Print("error while serving HTTP request: ", err)
		http.Error(w, err.Error(), 500)
	}
}

func serveTestHTTP(ctx context.Context, testID testID, w http.ResponseWriter, r *http.Request) error {
	remoteAddr, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		http.Error(w, "error parsing remote address: "+err.Error(), 400)
		return nil
	}

	if ok, err := isRunningTest(ctx, testID); err != nil {
		return fmt.Errorf("serveTestHTTP: error checking if %x is a running test: %w", testID, err)
	} else if !ok {
		http.Error(w, fmt.Sprintf("%x is not a running test", testID), 404)
		return nil
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO http_request (test_id, remote_ip, remote_port, host, method, url, proto, header_json, https) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, testID[:], remoteAddr.Addr().String(), remoteAddr.Port(), r.Host, r.Method, r.URL.String(), r.Proto, dbutil.JSON(r.Header), r.TLS != nil); err != nil {
		return fmt.Errorf("serveTestHTTP: error inserting http_request for test %x: %w", testID, err)
	}

	http.Error(w, "OK", 200)
	return nil
}

func runHTTPServer(l net.Listener) {
	server := http.Server{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  3 * time.Second,
		Handler:      http.HandlerFunc(serveHTTP),
		//ErrorLog:     logfilter.New(log.Default(), logfilter.HTTPServerErrors),
	}
	log.Fatal(server.Serve(l))
}
func runHTTPSServer(l net.Listener) {
	runHTTPServer(tls.NewListener(l, &tls.Config{GetConfigForClient: getHTTPSConfig}))
}
