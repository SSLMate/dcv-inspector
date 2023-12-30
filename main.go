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
	"src.agwa.name/go-listener"
	"src.agwa.name/go-listener/cert"
)

var (
	domain              string
	v4address           []netip.Addr
	v6address           []netip.Addr
	db                  *sql.DB
	getHTTPSCertificate cert.GetCertificateFunc
)

func main() {
	var flags struct {
		domain        string
		db            string
		httpListener  []string
		httpsListener []string
		httpsCert     string
		smtpListener  []string
		dnsListener   []string
		dnsUDP        []string
	}
	flag.StringVar(&flags.domain, "domain", "", "Domain name")
	flag.StringVar(&flags.db, "db", "", "Path to database file")
	flag.Func("http-listener", "Socket for HTTP server to listen on (go-listener syntax; e.g. tcp:80)", func(arg string) error {
		flags.httpListener = append(flags.httpListener, arg)
		return nil
	})
	flag.Func("https-listener", "Socket for HTTPS server to listen on (go-listener syntax; e.g. tcp:443)", func(arg string) error {
		flags.httpsListener = append(flags.httpsListener, arg)
		return nil
	})
	flag.StringVar(&flags.httpsCert, "https-cert", "", "HTTPS certificate (default: obtain automatically with ACME)")
	flag.Func("smtp-listener", "Socket for SMTP server to listen on (go-listener syntax; e.g. tcp:25)", func(arg string) error {
		flags.smtpListener = append(flags.smtpListener, arg)
		return nil
	})
	flag.Func("dns-listener", "TCP socket for DNS server to listen on (go-listener syntax; e.g. tcp:53)", func(arg string) error {
		flags.dnsListener = append(flags.dnsListener, arg)
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

	if flags.db == "" {
		log.Fatal("-db not specified")
	}
	if ret, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_busy_timeout=5000", url.PathEscape(flags.db))); err != nil {
		log.Fatalf("error opening database: %w", err)
	} else {
		db = ret
	}

	if flags.httpsCert == "" {
		getHTTPSCertificate = cert.GetCertificateAutomatically([]string{domain})
	} else {
		getHTTPSCertificate = cert.GetCertificateFromFile(flags.httpsCert)
	}

	httpListeners, err := listener.OpenAll(flags.httpListener)
	if err != nil {
		log.Fatalf("error opening HTTP listeners: %s", err)
	}
	httpsListeners, err := listener.OpenAll(flags.httpsListener)
	if err != nil {
		log.Fatalf("error opening HTTPS listeners: %s", err)
	}
	smtpListeners, err := listener.OpenAll(flags.smtpListener)
	if err != nil {
		log.Fatalf("error opening SMTP listeners: %s", err)
	}
	dnsListeners, err := listener.OpenAll(flags.dnsListener)
	if err != nil {
		log.Fatalf("error opening DNS listeners: %s", err)
	}
	dnsUDP, err := listenAllUDP(flags.dnsUDP)
	if err != nil {
		log.Fatalf("error opening DNS UDP sockets: %s", err)
	}

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
