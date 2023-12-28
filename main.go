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
		httpListener  string
		httpsListener string
		httpsCert     string
		smtpListener  string
		dnsListener   string
		dnsUDP        string
	}
	flag.StringVar(&flags.domain, "domain", "", "Domain name")
	flag.StringVar(&flags.db, "db", "", "Path to database file")
	flag.StringVar(&flags.httpListener, "http-listener", "", "Socket for HTTP server to listen on (go-listener syntax; e.g. tcp:80)")
	flag.StringVar(&flags.httpsListener, "https-listener", "", "Socket for HTTPS server to listen on (go-listener syntax; e.g. tcp:443)")
	flag.StringVar(&flags.httpsCert, "https-cert", "", "HTTPS certificate (default: obtain automatically with ACME)")
	flag.StringVar(&flags.smtpListener, "smtp-listener", "", "Socket for SMTP server to listen on (go-listener syntax; e.g. tcp:25)")
	flag.StringVar(&flags.dnsListener, "dns-listener", "", "TCP socket for DNS server to listen on (go-listener syntax; e.g. tcp:53)")
	flag.StringVar(&flags.dnsUDP, "dns-udp", "", "UDP socket for DNS server (udp:PORTNO or udp:IPADDR:PORTNO or fd:FILDESC)")
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

	var (
		httpListener  net.Listener
		httpsListener net.Listener
		smtpListener  net.Listener
		dnsListener   net.Listener
		dnsUDP        net.PacketConn
	)

	if flags.httpListener != "" {
		if l, err := listener.Open(flags.httpListener); err != nil {
			log.Fatalf("error opening HTTP listener: %s", err)
		} else {
			httpListener = l
		}
	}
	if flags.httpsListener != "" {
		if l, err := listener.Open(flags.httpsListener); err != nil {
			log.Fatalf("error opening HTTPS listener: %s", err)
		} else {
			httpsListener = l
		}
	}
	if flags.smtpListener != "" {
		if l, err := listener.Open(flags.smtpListener); err != nil {
			log.Fatalf("error opening SMTP listener: %s", err)
		} else {
			smtpListener = l
		}
	}
	if flags.dnsListener != "" {
		if l, err := listener.Open(flags.dnsListener); err != nil {
			log.Fatalf("error opening DNS listener: %s", err)
		} else {
			dnsListener = l
		}
	}
	if flags.dnsUDP != "" {
		if p, err := listenUDP(flags.dnsUDP); err != nil {
			log.Fatalf("error opening DNS UDP socket: %s", err)
		} else {
			dnsUDP = p
		}
	}

	if httpListener != nil {
		go runHTTPServer(httpListener)
	}
	if httpsListener != nil {
		go runHTTPSServer(httpsListener)
	}
	if smtpListener != nil {
		go runSMTPServer(smtpListener)
	}
	if dnsListener != nil {
		go runDNSServer(dnsListener, nil)
	}
	if dnsUDP != nil {
		go runDNSServer(nil, dnsUDP)
	}

	select {}
}
