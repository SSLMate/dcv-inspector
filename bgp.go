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
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kentik/patricia"
	"github.com/kentik/patricia/uint32_tree"
)

var bgptoolsClient = &http.Client{
	Timeout: 30 * time.Second,
}

type autonomousSystem struct {
	Number uint32
	Name   string
}

func (as *autonomousSystem) String() string {
	str := fmt.Sprintf("AS%d", as.Number)
	if as.Name != "" {
		str += fmt.Sprintf(" (%s)", as.Name)
	}
	return str
}

func (as *autonomousSystem) HTML() template.HTML {
	str := fmt.Sprintf(`<a href="https://bgp.tools/as/%d">AS%d</a>`, as.Number, as.Number)
	if as.Name != "" {
		str += fmt.Sprintf(" (%s)", template.HTMLEscapeString(as.Name))
	}
	return template.HTML(str)
}

var bgpData struct {
	mu         sync.Mutex
	asNames    map[uint32]string
	v4prefixes *uint32_tree.TreeV4
	v6prefixes *uint32_tree.TreeV6
}

func getAutonomousSystems(addrString string) []autonomousSystem {
	if addr, err := netip.ParseAddr(addrString); err == nil {
		return getAutonomousSystemsForAddr(addr)
	} else {
		return nil
	}
}

func getAutonomousSystemsForAddr(addr netip.Addr) []autonomousSystem {
	bgpData.mu.Lock()
	defer bgpData.mu.Unlock()

	var asns []uint32
	if addr.Is4() && bgpData.v4prefixes != nil {
		_, asns = bgpData.v4prefixes.FindDeepestTags(patricia.NewIPv4AddressFromBytes(addr.AsSlice(), 32))
	} else if addr.Is6() && bgpData.v6prefixes != nil {
		_, asns = bgpData.v6prefixes.FindDeepestTags(patricia.NewIPv6Address(addr.AsSlice(), 128))
	}
	ases := make([]autonomousSystem, len(asns))
	for i, asn := range asns {
		ases[i].Number = asn
		ases[i].Name = bgpData.asNames[asn]
	}
	return ases
}

func downloadASNames(ctx context.Context) (map[uint32]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://bgp.tools/asns.csv", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgentString)
	resp, err := bgptoolsClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s response from %s: %s", resp.Status, req.URL, string(respBytes))
	}
	reader := csv.NewReader(resp.Body)
	reader.FieldsPerRecord = 3
	reader.ReuseRecord = true

	if _, err := reader.Read(); err != nil {
		return nil, fmt.Errorf("error reading CSV header from %s: %w", req.URL, err)
	}
	names := make(map[uint32]string, 100000)
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading CSV response from %s: %w", req.URL, err)
		}
		asn, err := strconv.ParseUint(strings.TrimPrefix(record[0], "AS"), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("error parsing ASN in CSV from %s: %w", req.URL, err)
		}
		names[uint32(asn)] = record[1]
	}
	return names, nil
}

func refreshASNames(ctx context.Context) error {
	names, err := downloadASNames(ctx)
	if err != nil {
		return fmt.Errorf("error downloading autonomous system names: %w", err)
	}
	bgpData.mu.Lock()
	defer bgpData.mu.Unlock()
	bgpData.asNames = names
	return nil
}

func refreshASNamesPeriodically() {
	for {
		if err := refreshASNames(context.Background()); err != nil {
			log.Print(err)
			time.Sleep(1 * time.Hour)
		} else {
			time.Sleep(24 * time.Hour)
		}
	}
}

func downloadPrefixes(ctx context.Context) (*uint32_tree.TreeV4, *uint32_tree.TreeV6, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://bgp.tools/table.jsonl", nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", userAgentString)
	resp, err := bgptoolsClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("%s response from %s: %s", resp.Status, req.URL, string(respBytes))
	}
	decoder := json.NewDecoder(resp.Body)
	v4prefixes := uint32_tree.NewTreeV4()
	v6prefixes := uint32_tree.NewTreeV6()
	for {
		var record struct {
			CIDR netip.Prefix
			ASN  uint32
		}
		if err := decoder.Decode(&record); err == io.EOF {
			break
		} else if err != nil {
			return nil, nil, fmt.Errorf("error decoding JSON from %s: %w", req.URL, err)
		}
		if record.CIDR.Addr().Is4() {
			addr := patricia.NewIPv4AddressFromBytes(record.CIDR.Addr().AsSlice(), uint(record.CIDR.Bits()))
			v4prefixes.Add(addr, record.ASN, nil)
		} else if record.CIDR.Addr().Is6() {
			addr := patricia.NewIPv6Address(record.CIDR.Addr().AsSlice(), uint(record.CIDR.Bits()))
			v6prefixes.Add(addr, record.ASN, nil)
		}
	}
	return v4prefixes, v6prefixes, nil
}

func refreshPrefixes(ctx context.Context) error {
	v4prefixes, v6prefixes, err := downloadPrefixes(ctx)
	if err != nil {
		return fmt.Errorf("error downloading BGP prefixes: %w", err)
	}
	bgpData.mu.Lock()
	defer bgpData.mu.Unlock()
	bgpData.v4prefixes = v4prefixes
	bgpData.v6prefixes = v6prefixes
	return nil
}

func refreshPrefixesPeriodically() {
	for {
		if err := refreshPrefixes(context.Background()); err != nil {
			log.Print(err)
			time.Sleep(30 * time.Minute)
		} else {
			time.Sleep(2 * time.Hour)
		}
	}
}
