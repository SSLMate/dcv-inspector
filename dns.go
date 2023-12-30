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
	"fmt"
	"github.com/miekg/dns"
	"log"
	"net"
	"net/netip"
	"strings"
)

func serveDNS(w dns.ResponseWriter, req *dns.Msg) {
	testDomain := "test." + domain + "."

	if len(req.Question) == 0 || req.Question[0].Qclass != dns.ClassINET ||
		req.Question[0].Qtype == dns.TypeIXFR || req.Question[0].Qtype == dns.TypeAXFR {
		sendRefused(w, req)
		return
	}

	fqdn := strings.ToLower(req.Question[0].Name)
	qtype := req.Question[0].Qtype

	if !dns.IsSubDomain(testDomain, fqdn) {
		sendRefused(w, req)
		return
	}

	var answers []dns.RR

	if fqdn == testDomain {
		answers = []dns.RR{}
		if qtype == dns.TypeNS || qtype == dns.TypeANY {
			answers = append(answers, &dns.NS{
				Hdr: dns.RR_Header{Name: testDomain, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 86400},
				Ns:  domain + ".",
			})
		}
		if qtype == dns.TypeSOA || qtype == dns.TypeANY {
			answers = append(answers, makeSOA())
		}
	} else if testID, ok := parseHostname(fqdn); ok {
		if !strings.HasPrefix(fqdn, "_") {
			answers = []dns.RR{}
			if qtype == dns.TypeA || qtype == dns.TypeANY {
				for _, addr := range v4address {
					answers = append(answers, &dns.A{
						Hdr: dns.RR_Header{Name: fqdn, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 3600},
						A:   addr.AsSlice(),
					})
				}
			}
			if qtype == dns.TypeAAAA || qtype == dns.TypeANY {
				for _, addr := range v6address {
					answers = append(answers, &dns.AAAA{
						Hdr:  dns.RR_Header{Name: fqdn, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 3600},
						AAAA: addr.AsSlice(),
					})
				}
			}
			if qtype == dns.TypeMX || qtype == dns.TypeANY {
				answers = append(answers, &dns.MX{
					Hdr:        dns.RR_Header{Name: fqdn, Rrtype: dns.TypeMX, Class: dns.ClassINET, Ttl: 86400},
					Preference: 10,
					Mx:         domain + ".",
				})
			}
		}
		if err := recordDNSRequest(context.Background(), testID, w.RemoteAddr(), req); err != nil {
			log.Printf("error recording DNS request: %s", err)
		}
	}

	resp := new(dns.Msg)
	resp.SetReply(req)
	resp.Authoritative = true
	resp.Compress = true
	resp.Answer = answers
	if len(answers) == 0 {
		if answers == nil {
			resp.Rcode = dns.RcodeNameError
		}
		resp.Ns = []dns.RR{makeSOA()}
	}
	w.WriteMsg(resp)
}

func recordDNSRequest(ctx context.Context, testID testID, remoteAddr net.Addr, req *dns.Msg) error {
	if ok, err := isRunningTest(ctx, testID); err != nil {
		return fmt.Errorf("error checking if %x is a running test: %w", testID, err)
	} else if !ok {
		return nil
	}

	addrPort, err := netip.ParseAddrPort(remoteAddr.String())
	if err != nil {
		return fmt.Errorf("error parsing DNS remote address: %w", err)
	}

	reqBytes, err := req.Pack()
	if err != nil {
		return fmt.Errorf("error packing DNS message: %w", err)
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO dns_request (test_id, remote_ip, remote_port, fqdn, qtype, bytes) VALUES (?, ?, ?, ?, ?, ?)`, testID[:], addrPort.Addr().String(), addrPort.Port(), req.Question[0].Name, req.Question[0].Qtype, reqBytes); err != nil {
		return fmt.Errorf("error inserting dns_request: %w", err)
	}

	return nil
}

func sendRefused(w dns.ResponseWriter, req *dns.Msg) {
	resp := new(dns.Msg)
	resp.SetRcode(req, dns.RcodeRefused)
	w.WriteMsg(resp)
}

func makeSOA() *dns.SOA {
	return &dns.SOA{
		Hdr:     dns.RR_Header{Name: "test." + domain + ".", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 86400},
		Ns:      domain + ".",
		Mbox:    "hostmaster." + domain + ".",
		Serial:  1,
		Refresh: 86400,
		Retry:   86400,
		Expire:  86400,
		Minttl:  86400,
	}
}

func runDNSServer(l net.Listener, p net.PacketConn) {
	log.Fatal(dns.ActivateAndServe(l, p, dns.HandlerFunc(serveDNS)))
}
