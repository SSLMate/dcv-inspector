package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"time"
)

var ctsearchUserAgent = func() string {
	if info, _ := debug.ReadBuildInfo(); info != nil {
		return info.Main.Path + "/" + info.Main.Version
	} else {
		return "dcv-inspector"
	}
}()

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
	req.Header.Set("User-Agent", ctsearchUserAgent)
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
