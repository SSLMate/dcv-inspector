// Copyright (C) 2024 Opsmate, Inc.
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
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/netip"
	"sync"
	"time"
)

var dtpClient = &http.Client{
	Timeout: 30 * time.Second,
}

var dtpData struct {
	mu              sync.Mutex
	googlePublicDNS *cidrSet
}

type delegatedThirdParty struct {
	Name string
	URL  string
}

var googlePublicDNS = delegatedThirdParty{Name: "Google Public DNS", URL: "https://developers.google.com/speed/public-dns/faq#locations"}

func getDNSDelegatedThirdParty(addrString string) *delegatedThirdParty {
	if addr, err := netip.ParseAddr(addrString); err == nil {
		return getDNSDelegatedThirdPartyForAddress(addr)
	} else {
		return nil
	}
}

func getDNSDelegatedThirdPartyForAddress(addr netip.Addr) *delegatedThirdParty {
	dtpData.mu.Lock()
	defer dtpData.mu.Unlock()

	switch {
	case dtpData.googlePublicDNS.Has(addr):
		return &googlePublicDNS
	default:
		return nil
	}
}

func downloadGooglePublicDNS(ctx context.Context) (*cidrSet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.gstatic.com/ipranges/publicdns.json", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgentString)
	resp, err := dtpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s response from %s: %s", resp.Status, req.URL, string(respBytes))
	}
	var response struct {
		Prefixes []struct {
			IPv4Prefix netip.Prefix `json:"ipv4Prefix"`
			IPv6Prefix netip.Prefix `json:"ipv6Prefix"`
		} `json:"prefixes"`
	}
	if err := json.Unmarshal(respBytes, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling JSON from %s: %w", req.URL, err)
	}
	cidrs := newCidrSet()
	for _, prefix := range response.Prefixes {
		if prefix.IPv4Prefix.IsValid() {
			cidrs.Add(prefix.IPv4Prefix)
		} else if prefix.IPv6Prefix.IsValid() {
			cidrs.Add(prefix.IPv6Prefix)
		}
	}
	return cidrs, nil
}

func refreshGooglePublicDNS(ctx context.Context) error {
	googlePublicDNS, err := downloadGooglePublicDNS(ctx)
	if err != nil {
		return fmt.Errorf("error downloading prefixes for Google Public DNS: %w", err)
	}
	dtpData.mu.Lock()
	defer dtpData.mu.Unlock()
	dtpData.googlePublicDNS = googlePublicDNS
	return nil
}

func refreshGooglePublicDNSPeriodically() {
	for {
		if err := refreshGooglePublicDNS(context.Background()); err != nil {
			log.Print(err)
			time.Sleep(1 * time.Hour)
		} else {
			time.Sleep(24 * time.Hour)
		}
	}
}
