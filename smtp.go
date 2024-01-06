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
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/emersion/go-smtp"
	"src.agwa.name/go-dbutil"
	"src.agwa.name/go-listener/cert"
)

type smtpBackend struct{}

func (smtpBackend) NewSession(conn *smtp.Conn) (smtp.Session, error) {
	return &smtpSession{conn: conn}, nil
}

type smtpSession struct {
	conn     *smtp.Conn
	mailFrom string
	rcptTo   []string
	testIDs  []testID
}

func (s *smtpSession) Reset() {
	s.mailFrom = ""
	s.rcptTo = nil
	s.testIDs = nil
}
func (s *smtpSession) Logout() error {
	return nil
}
func (s *smtpSession) AuthPlain(username, password string) error {
	return nil
}
func (s *smtpSession) Mail(from string, opts *smtp.MailOptions) error {
	s.mailFrom = from
	return nil
}
func (s *smtpSession) Rcpt(to string, opts *smtp.RcptOptions) error {
	testID, ok := parseEmailAddress(to)
	if !ok {
		return &smtp.SMTPError{Code: 554, EnhancedCode: [3]int{5, 7, 1}, Message: "Relay access denied"}
	}
	s.testIDs = append(s.testIDs, testID)
	s.rcptTo = append(s.rcptTo, to)
	return nil
}
func (s *smtpSession) Data(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	remoteAddr := s.conn.Conn().RemoteAddr()
	if remoteAddr == nil {
		return fmt.Errorf("SMTP connection has unknown remote address")
	}
	addrPort, err := netip.ParseAddrPort(remoteAddr.String())
	if err != nil {
		return fmt.Errorf("error parsing SMTP client's remote address: %w", err)
	}
	helo := s.conn.Hostname()
	_, starttls := s.conn.TLSConnectionState()
	for _, testID := range s.testIDs {
		if err := recordSMTPRequest(context.Background(), testID, addrPort, helo, starttls, s.mailFrom, s.rcptTo, data); err != nil {
			log.Printf("smtp: error recording request for test %x: %s", testID, err)
		}
	}
	return nil
}

func recordSMTPRequest(ctx context.Context, testID testID, remoteAddr netip.AddrPort, helo string, starttls bool, mailFrom string, rcptTo []string, data []byte) error {
	if ok, err := isRunningTest(ctx, testID); err != nil {
		return fmt.Errorf("error checking if test is running: %w", err)
	} else if !ok {
		return nil
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO smtp_request (test_id, remote_ip, remote_port, helo, mail_from, rcpt_to_json, data, starttls) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, testID[:], remoteAddr.Addr().String(), remoteAddr.Port(), helo, mailFrom, dbutil.JSON(rcptTo), data, starttls); err != nil {
		return fmt.Errorf("error inserting smtp_request: %w", err)
	}
	return nil
}

func runSMTPServer(l net.Listener) {
	server := smtp.NewServer(smtpBackend{})
	server.TLSConfig = &tls.Config{
		GetCertificate: cert.GetCertificateDefaultServerName(domain, getSelfSignedCert),
		MinVersion:     tls.VersionTLS10,
	}
	server.Domain = domain
	server.MaxRecipients = 20
	server.MaxMessageBytes = 1 * 1024 * 1024
	server.ReadTimeout = 5 * time.Second
	server.WriteTimeout = 15 * time.Second
	server.EnableSMTPUTF8 = true
	server.AuthDisabled = true
	log.Fatal(server.Serve(l))
}

func parseEmailAddress(addr string) (testID, bool) {
	at := strings.LastIndexByte(addr, '@')
	if at == -1 {
		return testID{}, false
	}
	id, _, ok := parseHostname(addr[at+1:])
	return id, ok
}
