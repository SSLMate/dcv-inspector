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
	"bytes"
	"context"
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/miekg/dns"
	"src.agwa.name/go-dbutil"
)

const redirectDashboardToHTTPS = true

//go:embed assets/* templates/*
var content embed.FS

var homeTemplate = template.Must(template.ParseFS(content, "templates/home.html"))
var testTemplate = template.Must(template.ParseFS(content, "templates/test.html"))

type dashboard struct {
	Domain    string
	BuildInfo *debug.BuildInfo
}

func makeDashboard() dashboard {
	var d dashboard
	d.Domain = domain
	d.BuildInfo, _ = debug.ReadBuildInfo()
	return d
}

type testDashboard struct {
	dashboard
	TestID    testID
	StartedAt time.Time
	StoppedAt *time.Time
	DNS       []dnsItem
	HTTP      []httpItem
	SMTP      []smtpItem
}

func (t *testDashboard) IsRunning() bool {
	return t.StoppedAt == nil
}
func (t *testDashboard) TestDomain() string {
	return t.TestID.String() + ".test." + domain
}

var dnsRequestTable = dbutil.Table{Name: "dns_request"}

type dnsItem struct {
	DNSRequestID int       `sql:"dns_request_id"`
	ReceivedAt   time.Time `sql:"received_at"`
	RemoteIP     string    `sql:"remote_ip"`
	RemotePort   string    `sql:"remote_port"`
	FQDN         string    `sql:"fqdn"`
	QType        uint16    `sql:"qtype"`
	Bytes        []byte    `sql:"bytes"`
}

func (i *dnsItem) RemoteAddr() string { return net.JoinHostPort(i.RemoteIP, i.RemotePort) }

func (i *dnsItem) QTypeString() string {
	if str, ok := dns.TypeToString[i.QType]; ok {
		return str
	} else {
		return fmt.Sprintf("TYPE%d", i.QType)
	}
}

func (i *dnsItem) MessageString() string {
	msg := new(dns.Msg)
	if err := msg.Unpack(i.Bytes); err != nil {
		return "error unpacking DNS message: " + err.Error()
	}
	return msg.String()
}

var httpRequestTable = dbutil.Table{Name: "http_request"}

type httpItem struct {
	HTTPRequestID int                 `sql:"http_request_id"`
	ReceivedAt    time.Time           `sql:"received_at"`
	RemoteIP      string              `sql:"remote_ip"`
	RemotePort    string              `sql:"remote_port"`
	Host          string              `sql:"host"`
	Method        string              `sql:"method"`
	URL           string              `sql:"url"`
	Proto         string              `sql:"proto"`
	Header        map[string][]string `sql:"header_json,json"`
	HTTPS         bool                `sql:"https"`
}

func (i *httpItem) IsHTTPS() string { return boolString(i.HTTPS) }

func (i *httpItem) RemoteAddr() string { return net.JoinHostPort(i.RemoteIP, i.RemotePort) }

func (i *httpItem) HeaderString() string {
	var buf strings.Builder
	if err := http.Header(i.Header).Write(&buf); err != nil {
		return "error writing HTTP header: " + err.Error()
	}
	return buf.String()
}

var smtpRequestTable = dbutil.Table{Name: "smtp_request"}

type smtpItem struct {
	SMTPRequestID int       `sql:"smtp_request_id"`
	ReceivedAt    time.Time `sql:"received_at"`
	RemoteIP      string    `sql:"remote_ip"`
	RemotePort    string    `sql:"remote_port"`
	Helo          string    `sql:"helo"`
	MailFrom      string    `sql:"mail_from"`
	RcptTo        []string  `sql:"rcpt_to_json,json"`
	Data          []byte    `sql:"data"`
	STARTTLS      bool      `sql:"starttls"`
}

func (i *smtpItem) IsSTARTTLS() string { return boolString(i.STARTTLS) }

func (i *smtpItem) RemoteAddr() string { return net.JoinHostPort(i.RemoteIP, i.RemotePort) }

func (i *smtpItem) MessageHeader() string {
	if index := bytes.Index(i.Data, []byte("\n\r\n")); index != -1 {
		return string(i.Data[:index+1])
	} else if index := bytes.Index(i.Data, []byte("\n\n")); index != -1 {
		return string(i.Data[:index+1])
	} else {
		return string(i.Data)
	}
}

func loadTestDashboard(ctx context.Context, testID testID) (*testDashboard, error) {
	dashboard := &testDashboard{dashboard: makeDashboard(), TestID: testID}
	if err := db.QueryRowContext(ctx, `SELECT started_at, stopped_at FROM test WHERE test_id = ?`, testID[:]).Scan(&dashboard.StartedAt, &dashboard.StoppedAt); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("error querying test table: %w", err)
	}
	if err := dbutil.QueryStructs(ctx, db, dnsRequestTable, &dashboard.DNS, `WHERE test_id = ? ORDER BY received_at, dns_request_id`, testID[:]); err != nil {
		return nil, fmt.Errorf("error querying dns_request table: %w", err)
	}
	if err := dbutil.QueryStructs(ctx, db, httpRequestTable, &dashboard.HTTP, `WHERE test_id = ? ORDER BY received_at, http_request_id`, testID[:]); err != nil {
		return nil, fmt.Errorf("error querying http_request table: %w", err)
	}
	if err := dbutil.QueryStructs(ctx, db, smtpRequestTable, &dashboard.SMTP, `WHERE test_id = ? ORDER BY received_at, smtp_request_id`, testID[:]); err != nil {
		return nil, fmt.Errorf("error querying smtp_request table: %w", err)
	}
	return dashboard, nil
}

func parseTestPath(path string) (testID, bool) {
	testIDStr, ok := strings.CutPrefix(path, "/test/")
	if !ok {
		return testID{}, false
	}
	return parseTestID(testIDStr)
}

func serveDashboard(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	if redirectDashboardToHTTPS && r.TLS == nil {
		newURL := &url.URL{
			Scheme:     "https",
			Host:       r.Host,
			Path:       r.URL.Path,
			RawPath:    r.URL.RawPath,
			ForceQuery: r.URL.ForceQuery,
			RawQuery:   r.URL.RawQuery,
		}
		http.Redirect(w, r, newURL.String(), http.StatusPermanentRedirect)
		return nil
	}
	if r.URL.Path == "/" {
		return serveHome(ctx, w, r)
	} else if strings.HasPrefix(r.URL.Path, "/assets/") {
		http.FileServer(http.FS(content)).ServeHTTP(w, r)
		return nil
	} else if r.URL.Path == "/test" && r.Method == http.MethodPost {
		return startTest(ctx, w, r)
	} else if testID, ok := parseTestPath(r.URL.Path); ok {
		return serveTest(ctx, w, r, testID)
	} else {
		http.Error(w, "Unrecognized path", 400)
		return nil
	}
}

func serveHome(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Xss-Protection", "0")
	w.WriteHeader(http.StatusOK)
	homeTemplate.Execute(w, makeDashboard())
	return nil
}

func startTest(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	testID := generateTestID()
	if _, err := db.ExecContext(ctx, `INSERT INTO test (test_id) VALUES(?)`, testID[:]); err != nil {
		return fmt.Errorf("startTest: error inserting test: %w", err)
	}
	http.Redirect(w, r, "/test/"+testID.String(), http.StatusSeeOther)
	return nil
}

func serveTest(ctx context.Context, w http.ResponseWriter, r *http.Request, testID testID) error {
	if r.PostFormValue("stop") != "" {
		if _, err := db.ExecContext(ctx, `UPDATE test SET stopped_at = CURRENT_TIMESTAMP WHERE test_id = ? AND stopped_at IS NULL`, testID[:]); err != nil {
			return fmt.Errorf("serveTest: error updating test: %w", err)
		}
		http.Redirect(w, r, "/test/"+testID.String(), http.StatusSeeOther)
		return nil
	}
	dashboard, err := loadTestDashboard(ctx, testID)
	if err != nil {
		return fmt.Errorf("error loading dashboard for test %x: %w", testID, err)
	}
	if dashboard == nil {
		http.Error(w, fmt.Sprintf("test %x not found", testID), 404)
		return nil
	}
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Xss-Protection", "0")
	w.WriteHeader(http.StatusOK)
	testTemplate.Execute(w, dashboard)
	return nil
}

func boolString(v bool) string {
	if v {
		return "Yes"
	} else {
		return "No"
	}
}
