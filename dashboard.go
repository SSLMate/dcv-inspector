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
	"encoding/base64"
	"fmt"
	"github.com/mattn/go-sqlite3"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
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
	TestID     testID
	StartedAt  time.Time
	StoppedAt  *time.Time
	DNS        []dnsItem
	DNSRecords []dnsRecord
	HTTP       []httpItem
	HTTPFiles  []httpFile
	SMTP       []smtpItem
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

func (i *dnsItem) DelegatedThirdParty() *delegatedThirdParty { return getDNSDelegatedThirdParty(i.RemoteIP) }

func (i *dnsItem) AutonomousSystems() []autonomousSystem { return getAutonomousSystems(i.RemoteIP) }

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

var dnsRecordTable = dbutil.Table{Name: "dns_record"}

type dnsRecord struct {
	DNSRecordID int            `sql:"dns_record_id"`
	Subdomain   string         `sql:"subdomain"`
	Type        uint16         `sql:"type"`
	Data        map[string]any `sql:"data_json,json"`
}

func (r *dnsRecord) TypeString() string {
	if str, ok := dns.TypeToString[r.Type]; ok {
		return str
	} else {
		return fmt.Sprintf("TYPE%d", r.Type)
	}
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

func (i *httpItem) AutonomousSystems() []autonomousSystem { return getAutonomousSystems(i.RemoteIP) }

func (i *httpItem) RemoteAddr() string { return net.JoinHostPort(i.RemoteIP, i.RemotePort) }

func (i *httpItem) HeaderString() string {
	var buf strings.Builder
	if err := http.Header(i.Header).Write(&buf); err != nil {
		return "error writing HTTP header: " + err.Error()
	}
	return buf.String()
}

var httpFileTable = dbutil.Table{Name: "http_file"}

type httpFile struct {
	HTTPFileID int    `sql:"http_file_id"`
	Scheme     string `sql:"scheme"`
	Subdomain  string `sql:"subdomain"`
	Path       string `sql:"path"`
	Content    string `sql:"content"`
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

func (i *smtpItem) AutonomousSystems() []autonomousSystem { return getAutonomousSystems(i.RemoteIP) }

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
	if err := dbutil.QueryStructs(ctx, db, dnsRecordTable, &dashboard.DNSRecords, `WHERE test_id = ? ORDER BY subdomain, dns_record_id`, testID[:]); err != nil {
		return nil, fmt.Errorf("error querying dns_record table: %w", err)
	}
	if err := dbutil.QueryStructs(ctx, db, httpRequestTable, &dashboard.HTTP, `WHERE test_id = ? ORDER BY received_at, http_request_id`, testID[:]); err != nil {
		return nil, fmt.Errorf("error querying http_request table: %w", err)
	}
	if err := dbutil.QueryStructs(ctx, db, httpFileTable, &dashboard.HTTPFiles, `WHERE test_id = ? ORDER BY scheme, subdomain, path`, testID[:]); err != nil {
		return nil, fmt.Errorf("error querying http_file table: %w", err)
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
	} else if r.URL.Path == "/view_issuance" {
		return serveViewIssuance(ctx, w, r)
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

func validateCAATag(tag string) error {
	if len(tag) == 0 {
		return fmt.Errorf("tag cannot be empty")
	}
	if len(tag) > 255 {
		return fmt.Errorf("tag is too long")
	}
	return nil
}

func decodePostedDNSRecord(r *http.Request) (string, uint16, map[string]any, error) {
	switch r.PostFormValue("add_dns_record") {
	case "TXT":
		subdomain := strings.ToLower(r.PostFormValue("txt_subdomain"))
		txt := r.PostFormValue("txt_data")
		if len(txt) > 255 {
			return "", 0, nil, fmt.Errorf("TXT record is too long")
		}
		return subdomain, dns.TypeTXT, map[string]any{"Txt": []string{txt}}, nil
	case "CAA":
		subdomain := strings.ToLower(r.PostFormValue("caa_subdomain"))
		flag, err := strconv.ParseUint(r.PostFormValue("caa_flag"), 10, 8)
		if err != nil {
			return "", 0, nil, fmt.Errorf("Invalid CAA flag: %w", err)
		}
		tag := r.PostFormValue("caa_tag")
		if err := validateCAATag(tag); err != nil {
			return "", 0, nil, fmt.Errorf("Invalid CAA tag: %w", err)
		}
		value := r.PostFormValue("caa_value")
		return subdomain, dns.TypeCAA, map[string]any{"Flag": flag, "Tag": tag, "Value": value}, nil
	default:
		return "", 0, nil, fmt.Errorf("invalid record type")
	}
}

func serveTest(ctx context.Context, w http.ResponseWriter, r *http.Request, testID testID) error {
	dashboard, err := loadTestDashboard(ctx, testID)
	if err != nil {
		return fmt.Errorf("error loading dashboard for test %v: %w", testID, err)
	}
	if dashboard == nil {
		http.Error(w, fmt.Sprintf("test %v not found", testID), 404)
		return nil
	}
	if dashboard.IsRunning() && r.Method == http.MethodPost {
		if r.PostFormValue("stop") != "" {
			if _, err := db.ExecContext(ctx, `UPDATE test SET stopped_at = CURRENT_TIMESTAMP WHERE test_id = ? AND stopped_at IS NULL`, testID[:]); err != nil {
				return fmt.Errorf("serveTest: error updating test: %w", err)
			}
		} else if r.PostFormValue("add_dns_record") != "" {
			subdomain, rrType, rrData, err := decodePostedDNSRecord(r)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return nil
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO dns_record (test_id, subdomain, type, data_json) VALUES(?,?,?,?)`, testID[:], subdomain, rrType, dbutil.JSON(rrData)); err != nil {
				return fmt.Errorf("serveTest: error inserting dns_record: %w", err)
			}
		} else if dnsRecordID := r.PostFormValue("rm_dns_record"); dnsRecordID != "" {
			if _, err := db.ExecContext(ctx, `DELETE FROM dns_record WHERE test_id = ? AND dns_record_id = ?`, testID[:], dnsRecordID); err != nil {
				return fmt.Errorf("serveTest: error deleting dns_record: %w", err)
			}
		} else if httpFileID := r.PostFormValue("rm_http_file"); httpFileID != "" {
			if _, err := db.ExecContext(ctx, `DELETE FROM http_file WHERE test_id = ? AND http_file_id = ?`, testID[:], httpFileID); err != nil {
				return fmt.Errorf("serveTest: error deleting http_file: %w", err)
			}
		} else if r.PostFormValue("add_http_file") != "" {
			var (
				scheme    = r.PostFormValue("file_scheme")
				subdomain = r.PostFormValue("file_subdomain")
				path      = r.PostFormValue("file_path")
				content   = r.PostFormValue("file_content")
			)
			subdomain = strings.ToLower(subdomain)
			if !strings.HasPrefix(path, "/.well-known/pki-validation/") && !strings.HasPrefix(path, "/.well-known/acme-challenge/") {
				http.Error(w, "Path must start with /.well-known/pki-validation/ or /.well-known/acme-challenge/", 400)
				return nil
			}
			if len(content) > 512 {
				http.Error(w, "Content must not be longer than 512 bytes", 400)
				return nil
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO http_file (test_id, scheme, subdomain, path, content) VALUES(?,?,?,?,?)`, testID[:], scheme, subdomain, path, content); err != nil {
				if isUniqueViolation(err) {
					http.Error(w, "There is already a file at this subdomain and path", 400)
					return nil
				}
				return fmt.Errorf("serveTest: error inserting http_file: %w", err)
			}
		}
		http.Redirect(w, r, "/test/"+testID.String(), http.StatusSeeOther)
		return nil
	}
	if r.FormValue("ctsearch") != "" {
		resp, err := ctsearch(ctx, "issuances", url.Values{
			"domain":             {makeHostname(testID, "")},
			"include_subdomains": {"true"},
			"expand":             {"dns_names", "issuer.operator"},
			"after":              {r.FormValue("ctsearch_after")},
		})
		if err != nil {
			return err
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		w.Write(resp)
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

func isUniqueViolation(err error) bool {
	sqliteErr, ok := err.(sqlite3.Error)
	return ok && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique
}

func serveViewIssuance(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	issuanceID := r.FormValue("id")
	certDER, err := ctsearch(ctx, "issuances/"+url.PathEscape(issuanceID+".der"), nil)
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "public, max-age=3600, must-revalidate")
	http.Redirect(w, r, "https://x509.io/?"+url.Values{"cert": {base64.StdEncoding.EncodeToString(certDER)}}.Encode(), http.StatusSeeOther)
	return nil
}
