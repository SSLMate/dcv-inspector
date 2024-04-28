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
	"database/sql"
	"flag"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"net"
	"net/netip"
	"net/url"
	"runtime/debug"
	"src.agwa.name/go-dbutil/dbschema"
	"src.agwa.name/go-listener"
	"src.agwa.name/go-listener/cert"
	"time"

	"software.sslmate.com/src/dcv-inspector/schema"
)

var (
	domain              string
	v4address           []netip.Addr
	v6address           []netip.Addr
	db                  *sql.DB
	getHTTPSCertificate cert.GetCertificateFunc
	userAgentString     string
)

func main() {
	var flags struct {
		domain      string
		db          string
		httpListen  []string
		httpsListen []string
		httpsCert   string
		smtpListen  []string
		dnsListen   []string
		dnsUDP      []string
	}
	flag.StringVar(&flags.domain, "domain", "", "Domain name")
	flag.StringVar(&flags.db, "db", "", "Path to database file")
	flag.Func("http-listen", "Socket for HTTP server to listen on (go-listener syntax; e.g. tcp:80)", func(arg string) error {
		flags.httpListen = append(flags.httpListen, arg)
		return nil
	})
	flag.Func("https-listen", "Socket for HTTPS server to listen on (go-listener syntax; e.g. tcp:443)", func(arg string) error {
		flags.httpsListen = append(flags.httpsListen, arg)
		return nil
	})
	flag.StringVar(&flags.httpsCert, "https-cert", "", "HTTPS certificate (default: obtain automatically with ACME)")
	flag.Func("smtp-listen", "Socket for SMTP server to listen on (go-listener syntax; e.g. tcp:25)", func(arg string) error {
		flags.smtpListen = append(flags.smtpListen, arg)
		return nil
	})
	flag.Func("dns-listen", "TCP socket for DNS server to listen on (go-listener syntax; e.g. tcp:53)", func(arg string) error {
		flags.dnsListen = append(flags.dnsListen, arg)
		return nil
	})
	flag.Func("dns-udp", "UDP socket for DNS server (udp:PORTNO or udp:IPADDR:PORTNO or fd:FILDESC)", func(arg string) error {
		flags.dnsUDP = append(flags.dnsUDP, arg)
		return nil
	})
	flag.Parse()

	if flags.domain == "" {
		log.Fatal("-domain not specified")
	}
	domain = flags.domain
	if addr, err := net.DefaultResolver.LookupNetIP(context.Background(), "ip4", domain); err != nil {
		log.Fatal(err)
	} else {
		v4address = addr
	}
	if addr, err := net.DefaultResolver.LookupNetIP(context.Background(), "ip6", domain); err != nil {
		log.Fatal(err)
	} else {
		v6address = addr
	}

	userAgentString = "DCV Inspector"
	if info, _ := debug.ReadBuildInfo(); info != nil {
		userAgentString += " (" + info.Main.Path + "@" + info.Main.Version + ")"
	}
	userAgentString += " running on " + domain

	if flags.db == "" {
		log.Fatal("-db not specified")
	}
	if ret, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_busy_timeout=5000&_foreign_keys=ON&_txlock=immediate&_journal_mode=WAL&_synchronous=FULL", url.PathEscape(flags.db))); err != nil {
		log.Fatalf("error opening database: %s", err)
	} else {
		db = ret
	}

	if err := dbschema.Build(context.Background(), db, schema.Files); err != nil {
		log.Fatalf("error building database schema: %s", err)
	}

	if flags.httpsCert == "" {
		getHTTPSCertificate = cert.GetCertificateAutomatically([]string{domain})
	} else {
		getHTTPSCertificate = cert.GetCertificateFromFile(flags.httpsCert)
	}

	httpListeners, err := listener.OpenAll(flags.httpListen)
	if err != nil {
		log.Fatalf("error opening HTTP listeners: %s", err)
	}
	httpsListeners, err := listener.OpenAll(flags.httpsListen)
	if err != nil {
		log.Fatalf("error opening HTTPS listeners: %s", err)
	}
	smtpListeners, err := listener.OpenAll(flags.smtpListen)
	if err != nil {
		log.Fatalf("error opening SMTP listeners: %s", err)
	}
	dnsListeners, err := listener.OpenAll(flags.dnsListen)
	if err != nil {
		log.Fatalf("error opening DNS listeners: %s", err)
	}
	dnsUDP, err := listenAllUDP(flags.dnsUDP)
	if err != nil {
		log.Fatalf("error opening DNS UDP sockets: %s", err)
	}

	go cleanupTestsPeriodically()
	go refreshPrefixesPeriodically()
	go refreshASNamesPeriodically()
	go refreshGooglePublicDNSPeriodically()

	for _, l := range httpListeners {
		l := l
		go runHTTPServer(l)
	}
	for _, l := range httpsListeners {
		l := l
		go runHTTPSServer(l)
	}
	for _, l := range smtpListeners {
		l := l
		go runSMTPServer(l)
	}
	for _, l := range dnsListeners {
		l := l
		go runDNSServer(l, nil)
	}
	for _, u := range dnsUDP {
		u := u
		go runDNSServer(nil, u)
	}

	select {}
}

func cleanupTestsPeriodically() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if err := cleanupTests(); err != nil {
			log.Printf("error cleaning up tests: %s", err)
		}
		<-ticker.C
	}
}

func cleanupTests() error {
	_, err := db.Exec(`UPDATE test SET stopped_at = CURRENT_TIMESTAMP WHERE stopped_at IS NULL AND started_at < ?`, time.Now().Add(-6*time.Hour))
	return err
}
