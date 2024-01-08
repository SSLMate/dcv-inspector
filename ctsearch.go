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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

var ctsearchClient = &http.Client{
	Timeout: 30 * time.Second,
}

func ctsearch(ctx context.Context, path string, query url.Values) ([]byte, error) {
	const varName = "CT_SEARCH_API_KEY"
	apiKey := os.Getenv(varName)
	if apiKey == "" {
		return nil, fmt.Errorf("$%s environment variable not set; get an API key at https://sslmate.com/ct_search_api", varName)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.certspotter.com/v1/"+path+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgentString)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := ctsearchClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading %s response from %s: %w", resp.Status, req.URL, err)
	}
	if resp.StatusCode == 200 {
		return respBytes, nil
	} else {
		return nil, fmt.Errorf("%s response from %s: %s", resp.Status, req.URL, string(respBytes))
	}
}
