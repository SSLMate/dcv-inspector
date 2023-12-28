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
	server.ReadTimeout = 50 * time.Second // TODO: reduce
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
	return parseHostname(addr[at+1:])
}
