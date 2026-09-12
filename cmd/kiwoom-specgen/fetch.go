package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	apiListAttempts = 3
	maxAPIListBytes = 16 << 20
)

// The public documentation endpoint can briefly return an HTML page with HTTP
// 200. Retry collection only; a failed collection must never replace a snapshot.
func fetchAPIList(client *http.Client, listURL string, retryDelay time.Duration) (apiListResponse, error) {
	for attempt := 1; ; attempt++ {
		payload, retry, err := fetchAPIListOnce(client, listURL)
		if err == nil {
			return payload, nil
		}
		if !retry || attempt == apiListAttempts {
			return apiListResponse{}, fmt.Errorf("API list collection failed after %d attempt(s): %w", attempt, err)
		}
		time.Sleep(retryDelay * time.Duration(attempt))
	}
}

func fetchAPIListOnce(client *http.Client, listURL string) (apiListResponse, bool, error) {
	var payload apiListResponse
	req, err := http.NewRequest(http.MethodPost, listURL, strings.NewReader("apiId="))
	if err != nil {
		return payload, false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "krsec-kiwoom-specgen/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return payload, true, fmt.Errorf("request API list: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	responseInfo := fmt.Sprintf("HTTP %d, Content-Type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retry := resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return payload, retry, fmt.Errorf("fetch API list: %s", responseInfo)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIListBytes+1))
	if err != nil {
		return payload, true, fmt.Errorf("read API list (%s): %w", responseInfo, err)
	}
	if len(data) > maxAPIListBytes {
		return payload, false, fmt.Errorf("API list exceeds %d bytes (%s)", maxAPIListBytes, responseInfo)
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		// Do not log response bodies: even documentation error pages can carry
		// session identifiers. Status and content type identify HTML responses.
		return payload, true, fmt.Errorf("decode API list response (%s): %w", responseInfo, err)
	}
	if strings.TrimSpace(payload.RespCode) != "0" {
		return payload, false, fmt.Errorf("API list response error: code=%s msg=%s", strings.TrimSpace(payload.RespCode), strings.TrimSpace(payload.RespMsg))
	}
	return payload, false, nil
}
