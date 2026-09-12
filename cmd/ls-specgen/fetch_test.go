package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGetBytesRetriesTransientHTTPFailures(t *testing.T) {
	for _, status := range []int{http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.Header.Get("User-Agent") != "krsec-ls-specgen/1.0" || r.Header.Get("Accept") == "" {
					t.Errorf("unexpected document request: %s %v", r.Method, r.Header)
				}
				if calls.Add(1) < 3 {
					w.WriteHeader(status)
					_, _ = fmt.Fprint(w, "temporary error")
					return
				}
				_, _ = fmt.Fprint(w, "document")
			}))
			defer srv.Close()
			data, err := getBytesWithRetry(srv.Client(), srv.URL, 0)
			if err != nil || string(data) != "document" || calls.Load() != 3 {
				t.Fatalf("calls=%d data=%q err=%v", calls.Load(), data, err)
			}
		})
	}
}

func TestGetBytesStopsOnPermanentOrExhaustedHTTPFailures(t *testing.T) {
	for _, tc := range []struct {
		status int
		calls  int32
	}{
		{http.StatusBadRequest, 1},
		{http.StatusForbidden, 1},
		{http.StatusNotFound, 1},
		{http.StatusTooManyRequests, 3},
		{http.StatusServiceUnavailable, 3},
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, "<html>private-session-marker</html>")
			}))
			defer srv.Close()
			data, err := getBytesWithRetry(srv.Client(), srv.URL, 0)
			if err == nil || data != nil || calls.Load() != tc.calls || !strings.Contains(err.Error(), fmt.Sprintf("HTTP %d", tc.status)) {
				t.Fatalf("calls=%d data=%q err=%v", calls.Load(), data, err)
			}
			if strings.Contains(err.Error(), "private-session-marker") {
				t.Fatal("response body leaked into error")
			}
		})
	}
}

type documentRoundTripper func(*http.Request) (*http.Response, error)

func (f documentRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type failedDocumentReader struct{}

func (failedDocumentReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestGetBytesRetriesTransportAndReadFailures(t *testing.T) {
	for _, failure := range []string{"transport", "read"} {
		for _, recover := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/recover=%t", failure, recover), func(t *testing.T) {
				calls := 0
				client := &http.Client{Transport: documentRoundTripper(func(_ *http.Request) (*http.Response, error) {
					calls++
					var body io.Reader = strings.NewReader("document")
					if calls == 1 || !recover {
						if failure == "transport" {
							return nil, errors.New("connection refused")
						}
						body = io.MultiReader(strings.NewReader("partial"), failedDocumentReader{})
					}
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(body), Header: make(http.Header)}, nil
				})}
				data, err := getBytesWithRetry(client, "https://example.invalid/document", 0)
				if recover {
					if err != nil || string(data) != "document" || calls != 2 {
						t.Fatalf("calls=%d data=%q err=%v", calls, data, err)
					}
				} else if err == nil || data != nil || calls != 3 {
					t.Fatalf("calls=%d data=%q err=%v", calls, data, err)
				}
			})
		}
	}
}

func TestGetBytesRejectsOversizedDocumentWithoutRetry(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: documentRoundTripper(func(_ *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", maxDocumentBytes+1))),
			Header:     make(http.Header),
		}, nil
	})}
	data, err := getBytesWithRetry(client, "https://example.invalid/document", 0)
	if err == nil || data != nil || calls != 1 || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("calls=%d data length=%d err=%v", calls, len(data), err)
	}
}

func TestRefreshPreservesFilesOnDocumentCollectionFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "snapshot.json"), filepath.Join(dir, "specs.go")}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("existing content\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := runRefresh([]string{"--portal-url", srv.URL, "--snapshot", paths[0], "--spec-out", paths[1]}); err == nil {
		t.Fatal("refresh succeeded despite failed collection")
	}
	for _, path := range paths {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "existing content\n" {
			t.Fatalf("refresh changed %s: content=%q err=%v", path, got, err)
		}
	}
}
